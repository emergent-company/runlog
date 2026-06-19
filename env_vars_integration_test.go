package runlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvVarsIntegration(t *testing.T) {
	testVars := map[string]string{
		"GOOGLE_AI_API_KEY":  "AIzaSyC9WZcHy0ytAistKvwGcTZ9MjiiTpTOrFo",
		"MEMORY_TEST_SERVER": "http://172.26.0.3:3002",
		"MEMORY_AUTH_MODE":   "standalone",
		"MEMORY_TEST_TOKEN":  "e2e-test-user",
	}

	for k, v := range testVars {
		oldVal := os.Getenv(k)
		os.Setenv(k, v)
		t.Cleanup(func() {
			if oldVal == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, oldVal)
			}
		})
	}

	tmpDir := newTestDB(t)

	rl := NewRunLog(t)
	rl.Describe("Environment variable capture end-to-end",
		"Sets test env vars before creating RunLog",
		"RunLog auto-captures tracked env vars via NewRunLog",
		"Verifies exactly the set vars appear in the database row",
	)
	rl.Close()

	db, err := OpenDB(filepath.Join(tmpDir, "runs.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	runs, err := db.ListRuns(time.Time{}, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one run")
	}

	run := runs[len(runs)-1] // last run (ours)

	if len(run.EnvVars) == 0 {
		t.Fatal("expected env vars to be captured, but got empty map")
	}

	for k, expected := range testVars {
		actual, found := run.EnvVars[k]
		if !found {
			t.Errorf("expected env var %q to be in database, but it wasn't", k)
			continue
		}
		if actual != expected {
			t.Errorf("env var %q: expected %q, got %q", k, expected, actual)
		}
	}

	t.Logf("Captured %d env vars:", len(run.EnvVars))
	for k, v := range run.EnvVars {
		displayVal := v
		if strings.Contains(strings.ToLower(k), "key") || strings.Contains(strings.ToLower(k), "token") {
			if len(v) > 12 {
				displayVal = v[:8] + "..." + v[len(v)-4:]
			}
		}
		t.Logf("  - %s: %s", k, displayVal)
	}
}
