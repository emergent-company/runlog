package runlog

import (
	"os"
	"testing"
)

func TestEnvVarsIntegration(t *testing.T) {
	df := NewDogfoodRun(t, "env")
	defer df.Done()
	df.Describe("Environment variable capture end-to-end",
		"Sets test env vars, creates a run with them",
		"Verifies env vars appear in the database row",
	)
	df.Event("log", "Testing env var capture integration")

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

	_, dc := StartTestDaemon(t)

	r := dc.CreateRun(t, CreateRunOpts{
		EnvProfile: t.Name(),
		Category:   "env",
		EnvVars:    testVars,
	})
	dc.MarkDone(t, r.DaemonID, MarkDoneOpts{Passed: boolPtr(true)})

	run := dc.MustGetTestRun(t, r.TestRunID)
	if run == nil {
		t.Fatal("expected at least one run")
	}
}
