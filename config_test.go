package runlog

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestConfig writes content to <dir>/config.yaml and returns the path.
func writeTestConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestParseConfigFile_Categories(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
db: .runlog/runs.db
daemon_port: 7431

categories:
  auth:
    - unauthenticated request redirects to login
    - login page shows email error and retry button
  policy-export:
    - 'T1: export policy via GQL produces v2 bundle with partialTemplates'
`)

	cfg, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("parseConfigFile returned nil cfg with nil error")
	}
	if cfg.DaemonPort != 7431 {
		t.Errorf("DaemonPort = %d, want 7431", cfg.DaemonPort)
	}

	wantAuth := []string{
		"unauthenticated request redirects to login",
		"login page shows email error and retry button",
	}
	if got := cfg.Categories["auth"]; !equalStrings(got, wantAuth) {
		t.Errorf("Categories[auth] = %v, want %v", got, wantAuth)
	}

	// Quoted item (contains ':') must have quotes stripped and the colon preserved.
	wantPolicy := []string{"T1: export policy via GQL produces v2 bundle with partialTemplates"}
	if got := cfg.Categories["policy-export"]; !equalStrings(got, wantPolicy) {
		t.Errorf("Categories[policy-export] = %v, want %v", got, wantPolicy)
	}
}

func TestParseConfigFile_UnknownKey_NeverReturnsNilConfig(t *testing.T) {
	dir := t.TempDir()
	// "daemonPort" (camelCase) is not a recognized key — must error, but
	// must NOT return a nil *Config (callers commonly do `cfg, _ := ...`
	// and then dereference cfg fields; a nil cfg would panic).
	path := writeTestConfig(t, dir, `
db: .runlog/runs.db
daemonPort: 7431
`)

	cfg, err := parseConfigFile(path)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if cfg == nil {
		t.Fatal("parseConfigFile returned nil cfg on error — this panics any caller doing cfg, _ := parseConfigFile(...)")
	}
	// Fields parsed before the error was hit should still be populated.
	if cfg.DBPath != ".runlog/runs.db" {
		t.Errorf("DBPath = %q, want .runlog/runs.db (partial parse should be preserved)", cfg.DBPath)
	}
}

func TestParseConfigFile_MissingFile_NeverReturnsNilConfig(t *testing.T) {
	cfg, err := parseConfigFile("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if cfg == nil {
		t.Fatal("parseConfigFile returned nil cfg on error")
	}
}

func TestParseConfigFile_ArtifactsDir(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir, `
artifacts_dir: /tmp/runlog-artifacts
`)
	cfg, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile returned error: %v", err)
	}
	if cfg.ArtifactsDir != "/tmp/runlog-artifacts" {
		t.Errorf("ArtifactsDir = %q, want /tmp/runlog-artifacts", cfg.ArtifactsDir)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
