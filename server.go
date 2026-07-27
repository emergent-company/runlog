// Package e2eframework — server.go
//
// Helpers for locating and health-checking the Memory test server.
package runlog

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// ServerURL returns the server URL from the MEMORY_TEST_SERVER or RUNLOG_TEST_SERVER
// environment variable, falling back to an empty string.  An empty value
// causes SkipIfServerDown to skip server-dependent tests rather than hitting
// a wrong address.
func ServerURL() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_TEST_SERVER"); v != "" {
		return v
	}
	return os.Getenv("RUNLOG_TEST_SERVER")
}

// E2ETestToken returns the static API key for the test server.
// Reads MEMORY_TEST_TOKEN or RUNLOG_TEST_TOKEN; falls back to the
// default value used by the Docker Compose stack.
func E2ETestToken() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_TEST_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("RUNLOG_TEST_TOKEN"); v != "" {
		return v
	}
	return "e2e-test-user"
}

// AuthMode returns the authentication mode for the current test environment.
// Reads MEMORY_AUTH_MODE or RUNLOG_AUTH_MODE; defaults to "standalone".
func AuthMode() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_AUTH_MODE"); v != "" {
		return v
	}
	if v := os.Getenv("RUNLOG_AUTH_MODE"); v != "" {
		return v
	}
	return "standalone"
}

// SetToken returns the Bearer token to write into credentials.json when
// MEMORY_AUTH_MODE=account.  Reads MEMORY_SET_TOKEN or RUNLOG_SET_TOKEN;
// defaults to "all-scopes".
func SetToken() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_SET_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("RUNLOG_SET_TOKEN"); v != "" {
		return v
	}
	return "all-scopes"
}

// ─────────────────────────────────────────────────────────────────────────────
// Org ID resolution — thread-safe, lazy, retry-backed.
// ─────────────────────────────────────────────────────────────────────────────

var (
	orgIDOnce  sync.Once
	orgIDValue string
)

// OrgID returns the organization ID for the test server.
// Checks MEMORY_ORG_ID first, then RUNLOG_ORG_ID (set by parent process).
// If not set, lazily discovers it from the server with retry (cached via
// sync.Once so each test binary queries the server at most once).
//
// Returns an empty string when the org cannot be determined — callers
// should handle the empty case gracefully.
func OrgID() string { //nolint:deadcode
	if id := os.Getenv("MEMORY_ORG_ID"); id != "" {
		return id
	}
	if id := os.Getenv("RUNLOG_ORG_ID"); id != "" {
		return id
	}
	orgIDOnce.Do(func() {
		orgIDValue = discoverOrgID()
	})
	return orgIDValue
}

// discoverOrgID queries the server for the first org ID with retry.
// Uses a fresh http.Client with 15s timeout per attempt. Includes
// 0-2s random jitter before first attempt so parallel test binaries
// stagger their requests.
func discoverOrgID() string {
	srv := ServerURL()
	if srv == "" {
		fmt.Fprintf(os.Stderr, "[setup] discoverOrgID: MEMORY_TEST_SERVER not set — org-dependent tests will skip\n")
		return ""
	}
	token := E2ETestToken()
	client := &http.Client{Timeout: 15 * time.Second}

	// Random jitter so parallel test binaries don't hammer the server simultaneously.
	time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, _ := http.NewRequest("GET", srv+"/api/user/orgs-and-projects", nil)
		SetAuthHeader(req, token)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[setup] discoverOrgID attempt %d/5: %v (server=%s)\n", attempt+1, err, srv)
			continue
		}
		var orgs []struct {
			ID string `json:"id"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&orgs)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || decodeErr != nil || len(orgs) == 0 {
			fmt.Fprintf(os.Stderr, "[setup] discoverOrgID attempt %d/5: status=%d decode=%v orgs=%d\n",
				attempt+1, resp.StatusCode, decodeErr, len(orgs))
			continue
		}
		fmt.Fprintf(os.Stderr, "[setup] discovered org ID: %s\n", orgs[0].ID)
		return orgs[0].ID
	}
	fmt.Fprintf(os.Stderr, "[setup] discoverOrgID: all 5 attempts failed — org-dependent tests will skip\n")
	return ""
}

// DiscoverOrgID is a no-op — org discovery is now lazy via OrgID().
// Kept for backward compatibility with TestMain files; safe to remove.
func DiscoverOrgID() {} //nolint:deadcode

// DiscoverOrgIDForce queries the server for the first org ID synchronously
// (bypasses the internal sync.Once cache).  Use this in the parent process
// (e.g. `runlog test`) to set MEMORY_ORG_ID before spawning child test
// binaries so every binary inherits the value without hitting the server.
// Returns the org ID or empty string on failure (logged to stderr).
func DiscoverOrgIDForce() string { //nolint:deadcode
	return discoverOrgID()
}

// SkipIfServerDown skips t if the Emergent server at ServerURL() is unreachable.
// If rl is non-nil the skip reason is recorded in the runs DB.
func SkipIfServerDown(t *testing.T, rl ...*RunLog) { //nolint:deadcode
	t.Helper()

	var runlog *RunLog
	if len(rl) > 0 {
		runlog = rl[0]
	}

	srv := ServerURL()
	ctx, cancel := cancelCtx(5 * time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv+"/health", nil)
	if err != nil {
		DoSkipf(t, runlog, "cannot build health request for %s: %v", srv, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		DoSkipf(t, runlog, "server unreachable (%s): %v — is MEMORY_TEST_SERVER set?", srv, err)
	}
	resp.Body.Close()
	// Server responded — it's reachable. Health endpoint may return
	// non-200 (e.g. 503 when optional services like storage aren't
	// configured) but the API is still operational.
}

// SkipIfEndpointMissing skips t if a GET/HEAD to the given path returns 404.
// Use this when a test requires a server-side feature that may not be present in
// older deployed versions (e.g. account-level token routes added after v0.30).
//
// path is relative to ServerURL(), e.g. "/api/tokens".
// auth is an optional bearer token to include so auth errors (401) are not
// confused with routing errors (404).
// If rl is non-nil the skip reason is recorded in the runs DB.
func SkipIfEndpointMissing(t *testing.T, path string, bearerToken string, rl ...*RunLog) { //nolint:deadcode
	t.Helper()

	var runlog *RunLog
	if len(rl) > 0 {
		runlog = rl[0]
	}

	srv := ServerURL()
	ctx, cancel := cancelCtx(5 * time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv+path, nil)
	if err != nil {
		DoSkipf(t, runlog, "cannot build request for %s%s: %v", srv, path, err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		DoSkipf(t, runlog, "request to %s%s failed: %v", srv, path, err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		DoSkipf(t, runlog, "endpoint %s not available on this server version (404) — skipping", path)
	}
}

// FilteredEnv returns os.Environ() with project-scoped variables stripped.
// HOME and PATH are also stripped so callers can re-inject isolated values.
func FilteredEnv() []string { //nolint:deadcode
	filtered := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		switch {
		case hasPrefix(kv, "MEMORY_PROJECT_TOKEN="),
			hasPrefix(kv, "MEMORY_PROJECT="),
			hasPrefix(kv, "MEMORY_PROJECT_ID="),
			hasPrefix(kv, "MEMORY_API_KEY="),
			hasPrefix(kv, "HOME="),
			hasPrefix(kv, "PATH="):
			// skip — HOME and PATH are re-injected by the caller
		default:
			filtered = append(filtered, kv)
		}
	}
	return filtered
}
