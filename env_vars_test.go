package runlog

import (
	"os"
	"testing"
)

// TestCaptureEnvVars verifies that captureEnvVars captures the expected
// environment variables when they are set.
func TestCaptureEnvVars(t *testing.T) {
	// Set up test environment variables.
	testVars := map[string]string{
		"GOOGLE_AI_API_KEY":    "AIzaSyC9WZcHy0ytAistKvwGcTZ9MjiiTpTOrFo",
		"MEMORY_TEST_SERVER":   "http://172.26.0.3:3002",
		"MEMORY_AUTH_MODE":     "standalone",
		"MEMORY_TEST_TOKEN":    "e2e-test-user",
		"MEMORY_ORG_ID":        "test-org-123",
		"BRAVE_SEARCH_API_KEY": "brave-test-key",
		"OPENAI_API_KEY":       "sk-test-key",
		"ANTHROPIC_API_KEY":    "sk-ant-test-key",
	}

	// Set environment variables for the test.
	for k, v := range testVars {
		oldVal := os.Getenv(k)
		os.Setenv(k, v)
		defer func(key, old string) {
			if old == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, old)
			}
		}(k, oldVal)
	}

	// Capture environment variables.
	captured := captureEnvVars()

	// Verify all expected variables were captured.
	for k, expected := range testVars {
		actual, found := captured[k]
		if !found {
			t.Errorf("expected env var %q to be captured, but it wasn't", k)
			continue
		}
		if actual != expected {
			t.Errorf("env var %q: expected %q, got %q", k, expected, actual)
		}
	}

	// Verify no extra variables were captured.
	if len(captured) != len(testVars) {
		t.Errorf("expected %d env vars, got %d: %v", len(testVars), len(captured), captured)
	}
}

// TestCaptureEnvVarsEmpty verifies that captureEnvVars returns an empty map
// when none of the tracked environment variables are set.
func TestCaptureEnvVarsEmpty(t *testing.T) {
	// Clear all tracked environment variables.
	trackedVars := []string{
		"GOOGLE_AI_API_KEY",
		"MEMORY_TEST_SERVER",
		"MEMORY_TEST_TOKEN",
		"MEMORY_AUTH_MODE",
		"MEMORY_ORG_ID",
		"BRAVE_SEARCH_API_KEY",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	}

	oldVals := make(map[string]string)
	for _, k := range trackedVars {
		oldVals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range oldVals {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	// Capture environment variables.
	captured := captureEnvVars()

	// Verify empty result.
	if len(captured) != 0 {
		t.Errorf("expected empty map when no env vars set, got: %v", captured)
	}
}

// TestCaptureEnvVarsPartial verifies that captureEnvVars only captures
// variables that are actually set.
func TestCaptureEnvVarsPartial(t *testing.T) {
	// Clear all tracked environment variables first.
	trackedVars := []string{
		"GOOGLE_AI_API_KEY",
		"MEMORY_TEST_SERVER",
		"MEMORY_TEST_TOKEN",
		"MEMORY_AUTH_MODE",
		"MEMORY_ORG_ID",
		"BRAVE_SEARCH_API_KEY",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	}

	oldVals := make(map[string]string)
	for _, k := range trackedVars {
		oldVals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range oldVals {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	// Set only a few variables.
	os.Setenv("GOOGLE_AI_API_KEY", "test-key-123")
	os.Setenv("MEMORY_TEST_SERVER", "http://localhost:3002")

	// Capture environment variables.
	captured := captureEnvVars()

	// Verify only the set variables were captured.
	if len(captured) != 2 {
		t.Errorf("expected 2 env vars, got %d: %v", len(captured), captured)
	}

	if captured["GOOGLE_AI_API_KEY"] != "test-key-123" {
		t.Errorf("expected GOOGLE_AI_API_KEY=test-key-123, got %q", captured["GOOGLE_AI_API_KEY"])
	}

	if captured["MEMORY_TEST_SERVER"] != "http://localhost:3002" {
		t.Errorf("expected MEMORY_TEST_SERVER=http://localhost:3002, got %q", captured["MEMORY_TEST_SERVER"])
	}
}
