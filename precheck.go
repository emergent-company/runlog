// Package e2eframework — precheck.go
//
// RequireServerReady is a single call that every server-dependent test should
// make at its top, before any CLI or API calls.  It runs three checks in order:
//
//  1. Health     — is the server reachable and healthy?
//  2. Auth       — can we authenticate with the configured credentials?
//  3. (proceed)  — both passed, test may continue.
//
// If either check fails the test is immediately skipped (not failed) with a
// clear diagnostic message so the operator knows exactly what to fix.
//
// Usage:
//
//	func TestFoo(t *testing.T) {
//	    home := t.TempDir()
//	    framework.RequireServerReady(t, home)
//	    ...
//	}
package runlog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// RequireServerReady skips t with a diagnostic message if the server is not
// reachable or if the configured credentials do not authenticate successfully.
//
// home is the isolated HOME directory that SetupCLIAuth will use.  Pass
// t.TempDir() — RequireServerReady calls SetupCLIAuth internally so the caller
// does NOT need to call it again.
//
// If rl is non-nil the skip reason is recorded in the runs DB.
//
// Typical usage — replace the old SkipIfServerDown + setupCLIAuth pair:
//
//	func TestFoo(t *testing.T) {
//	    home := t.TempDir()
//	    framework.RequireServerReady(t, home)
//	    // home is now fully authenticated; use mustRunCLIInDirWithHome(t, "", home, ...)
//	}
func RequireServerReady(t *testing.T, home string, rl ...*RunLog) {
	t.Helper()
	var runlog *RunLog
	if len(rl) > 0 {
		runlog = rl[0]
	}
	checkHealth(t, runlog)
	SetupCLIAuth(t, home)
	checkAuth(t, home, runlog)
}

// checkHealth verifies the server /health endpoint is reachable and returns
// status "healthy".  Skips (not fails) the test on any problem.
func checkHealth(t *testing.T, rl *RunLog) {
	t.Helper()

	srv := ServerURL()
	if srv == "" {
		DoSkipf(t, rl, "MEMORY_TEST_SERVER is not set — skipping server-dependent test")
	}

	ctx, cancel := cancelCtx(5 * time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv+"/health", nil)
	if err != nil {
		DoSkipf(t, rl, "pre-check: cannot build health request for %s: %v", srv, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		DoSkipf(t, rl, "pre-check: server unreachable at %s: %v\nIs MEMORY_TEST_SERVER correct?", srv, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		DoSkipf(t, rl, "pre-check: health check returned HTTP %d at %s\nbody: %s", resp.StatusCode, srv, body)
	}

	// Parse the health response to confirm status == "healthy".
	var h struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &h); err == nil && h.Status != "" && h.Status != "healthy" {
		DoSkipf(t, rl, "pre-check: server reports status %q (not healthy) at %s", h.Status, srv)
	}

	t.Logf("pre-check: server healthy at %s", srv)
}

// checkAuth verifies that the credentials written into home by SetupCLIAuth
// can successfully reach an authenticated endpoint.
//
// It hits GET /api/projects with the Bearer token (account mode) or X-API-Key
// (standalone mode).  A 2xx response means auth works.  Any 401/403 causes the
// test to be skipped with an actionable message.
func checkAuth(t *testing.T, home string, rl *RunLog) {
	t.Helper()

	srv := ServerURL()

	// Build the auth header the same way the CLI would.
	authHeader, apiKey := authHeadersForHome(t, home)

	ctx, cancel := cancelCtx(5 * time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv+"/api/projects", nil)
	if err != nil {
		DoSkipf(t, rl, "pre-check: cannot build auth probe request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		DoSkipf(t, rl, "pre-check: auth probe request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		t.Logf("pre-check: auth OK (%s, mode=%s)", srv, AuthMode())
	case http.StatusUnauthorized, http.StatusForbidden:
		DoSkipf(t, rl,
			"pre-check: auth failed (HTTP %d) at %s\n"+
				"  mode:   %s\n"+
				"  body:   %s\n"+
				"  hint:   check MEMORY_AUTH_MODE, MEMORY_SET_TOKEN, MEMORY_TEST_TOKEN",
			resp.StatusCode, srv, AuthMode(), body,
		)
	default:
		// Unexpected status — don't skip, let the test surface the issue.
		t.Logf("pre-check: auth probe returned HTTP %d (expected 200): %s", resp.StatusCode, body)
	}
}

// authHeadersForHome returns the Authorization header value and X-API-Key value
// that correspond to the credentials written into home by SetupCLIAuth.
//
// account mode  → (Bearer <token>, "")
// standalone    → ("", <api_key>)
func authHeadersForHome(t *testing.T, home string) (authorizationHeader, xAPIKey string) {
	t.Helper()

	if AuthMode() == "account" {
		tok := SetToken()
		if tok == "" {
			t.Logf("pre-check warn: MEMORY_SET_TOKEN is empty in account mode")
		}
		return fmt.Sprintf("Bearer %s", tok), ""
	}

	// standalone
	return "", E2ETestToken()
}
