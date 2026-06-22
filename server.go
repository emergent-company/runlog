// Package e2eframework — server.go
//
// Helpers for locating and health-checking the Memory test server.
package runlog

import (
	"net/http"
	"os"
	"testing"
	"time"
)

// ServerURL returns the Emergent server URL from the MEMORY_TEST_SERVER
// environment variable, falling back to an empty string.  An empty value
// causes SkipIfServerDown to skip server-dependent tests rather than hitting
// a wrong address.
func ServerURL() string { //nolint:deadcode
	return os.Getenv("MEMORY_TEST_SERVER")
}

// E2ETestToken returns the static API key for the test server.
// It reads MEMORY_TEST_TOKEN from the environment, falling back to the
// default value used by the Docker Compose stack.
func E2ETestToken() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_TEST_TOKEN"); v != "" {
		return v
	}
	return "e2e-test-user"
}

// AuthMode returns the authentication mode for the current test environment.
// Reads MEMORY_AUTH_MODE; defaults to "standalone".
//
//	standalone — plain API key sent as X-API-Key (Docker Compose / local dev standalone)
//	account    — Bearer token from credentials.json only (mcj-emergent, local Zitadel-backed dev)
//	             api_key must NOT be set in this mode — see SetupCLIAuth for details.
func AuthMode() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_AUTH_MODE"); v != "" {
		return v
	}
	return "standalone"
}

// SetToken returns the Bearer token to write into credentials.json when
// MEMORY_AUTH_MODE=account.  Reads MEMORY_SET_TOKEN; defaults to "all-scopes".
func SetToken() string { //nolint:deadcode
	if v := os.Getenv("MEMORY_SET_TOKEN"); v != "" {
		return v
	}
	return "all-scopes"
}

// OrgID returns the organization ID to set in config when MEMORY_ORG_ID is
// provided.  An empty return value means auto-detection should be relied upon.
func OrgID() string { //nolint:deadcode
	return os.Getenv("MEMORY_ORG_ID")
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
	if resp.StatusCode >= 400 {
		DoSkipf(t, runlog, "server health check returned %d (%s)", resp.StatusCode, srv)
	}
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
