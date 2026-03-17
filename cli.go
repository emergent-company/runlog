// Package e2eframework — cli.go
//
// Helpers for invoking the memory CLI binary in tests.
package runlog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	// CLITimeout is the per-command timeout for CLI invocations.
	CLITimeout = 30 * time.Second
)

// MustRunCLI runs `memory <args>` from the current directory and fails the
// test if the command exits non-zero.  Returns combined stdout+stderr.
func MustRunCLI(t *testing.T, args ...string) string {
	t.Helper()
	return MustRunCLIInDir(t, "", args...)
}

// MustRunCLIInDir runs `memory <args>` from dir (empty = inherit) with a
// per-test isolated HOME directory derived from t.TempDir().
// It fails the test if the command exits non-zero and logs all output.
func MustRunCLIInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return MustRunCLIInDirWithHome(t, dir, t.TempDir(), args...)
}

// MustRunCLIInDirWithHome runs `memory <args>` from dir with the given home
// directory injected as HOME in the subprocess environment.
// Use this when you need to inspect the credentials written to a specific home.
func MustRunCLIInDirWithHome(t *testing.T, dir, home string, args ...string) string {
	t.Helper()
	return MustRunBinaryInDirWithHome(t, "memory", dir, home, args...)
}

// RunCLIInDirWithHome is like MustRunCLIInDirWithHome but returns an error
// instead of failing the test — used for polling where transient failures are OK.
func RunCLIInDirWithHome(t *testing.T, dir, home string, args ...string) (string, error) {
	t.Helper()
	return RunBinaryInDirWithHome(t, "memory", dir, home, args...)
}

// ─────────────────────────────────────────────────────────────────────────────
// Generic binary helpers (configurable binary name)
// ─────────────────────────────────────────────────────────────────────────────

// MustRunBinaryInDirWithHome runs `<binary> <args>` from dir with the given
// home directory injected as HOME in the subprocess environment.
// It fails the test if the command exits non-zero.  Returns combined stdout+stderr.
// This is the generic form of MustRunCLIInDirWithHome — pass any binary name.
func MustRunBinaryInDirWithHome(t *testing.T, binary, dir, home string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), CLITimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	env := FilteredEnv()
	env = append(env, "HOME="+home)
	env = append(env, "PATH="+home+"/.memory/bin:"+os.Getenv("PATH"))
	cmd.Env = env

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()

	invocation := fmt.Sprintf("%s %s", binary, strings.Join(args, " "))

	if err != nil {
		t.Fatalf("CLI command failed: %s\nerror: %v\noutput:\n%s", invocation, err, out)
	}
	return out
}

// RunBinaryInDirWithHome is like MustRunBinaryInDirWithHome but returns an
// error instead of failing the test — used when non-zero exit is expected or
// for polling where transient failures are OK.
func RunBinaryInDirWithHome(t *testing.T, binary, dir, home string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), CLITimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	env := FilteredEnv()
	env = append(env, "HOME="+home)
	env = append(env, "PATH="+home+"/.memory/bin:"+os.Getenv("PATH"))
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// SetupCLIAuth configures authentication in the isolated home directory for a
// CLI test.  The exact steps depend on MEMORY_AUTH_MODE:
//
//	standalone (default):
//	  memory config set server_url <MEMORY_TEST_SERVER>
//	  memory config set api_key    <MEMORY_TEST_TOKEN>
//	  The CLI sends the key as X-API-Key — correct for standalone/Docker servers.
//
//	account:
//	  memory set-token             <MEMORY_SET_TOKEN>   (writes credentials.json)
//	  memory config set server_url <MEMORY_TEST_SERVER>
//	  The CLI reads credentials.json and sends Bearer <token> — required for
//	  Zitadel-backed servers such as mcj-emergent and local dev.
//
//	  IMPORTANT: api_key must NOT be set in account mode.  If api_key is
//	  present in config.yaml the CLI takes the apikey branch in client.New()
//	  and sends only X-API-Key, bypassing credentials.json entirely.
//	  Non-standalone servers do not check X-API-Key, so the request fails
//	  with 401 missing_token.
//
// Call this once per test after creating the isolated home with t.TempDir().
func SetupCLIAuth(t *testing.T, home string) {
	t.Helper()

	srv := ServerURL()

	if AuthMode() == "account" {
		// Write credentials.json so the CLI considers itself authenticated.
		// Do NOT set api_key — that would make the CLI skip credentials.json.
		MustRunCLIInDirWithHome(t, "", home, "set-token", SetToken(), "--server", srv)
		MustRunCLIInDirWithHome(t, "", home, "config", "set", "server_url", srv)
		// Some servers don't expose the org-list endpoint for synthetic tokens,
		// so auto-detection fails.  Set org_id explicitly when provided.
		if id := OrgID(); id != "" {
			MustRunCLIInDirWithHome(t, "", home, "config", "set", "org_id", id)
		}
	} else {
		// Standalone: server accepts a plain API key via X-API-Key header.
		MustRunCLIInDirWithHome(t, "", home, "config", "set", "server_url", srv)
		MustRunCLIInDirWithHome(t, "", home, "config", "set", "api_key", E2ETestToken())
	}
}

// log entry for the test.  It is called at the top of every test so that each
// log file starts with the current authentication / server state, making it
// easy to diagnose failures without context-switching to a separate run.
// The command is allowed to fail (e.g. no credentials yet).
//
// home is an optional isolated HOME directory to inject.  Pass "" to use a
// fresh TempDir (safe for tests that haven't set up their own home yet).
func LogStatusPreamble(t *testing.T, home ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h := ""
	if len(home) > 0 && home[0] != "" {
		h = home[0]
	} else {
		h = t.TempDir()
	}

	args := []string{"status", "--server", ServerURL()}
	cmd := exec.CommandContext(ctx, "memory", args...)
	env := FilteredEnv()
	env = append(env, "HOME="+h)
	env = append(env, "PATH="+h+"/.memory/bin:"+os.Getenv("PATH"))
	cmd.Env = env

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	_ = cmd.Run() // intentionally ignore exit code — status may fail with no token
	out := buf.String()

	t.Logf("memory status:\n%s", strings.TrimSpace(out))
}

// logDir returns the directory to write session logs.
// Resolution order:
//  1. TEST_LOG_DIR env var
//  2. logs/ sibling to the calling source file
//  3. /test-logs (fallback inside Docker)
func logDir() string {
	if d := os.Getenv("TEST_LOG_DIR"); d != "" {
		return d
	}
	_, srcFile, _, ok := runtime.Caller(2)
	if ok {
		return filepath.Join(filepath.Dir(srcFile), "logs")
	}
	if _, err := os.Stat("/test-logs"); err == nil {
		return "/test-logs"
	}
	return filepath.Join(os.TempDir(), "memory-cli-docker-tests")
}

// LogSession writes a structured log of the CLI invocation and its output to
// the session log directory. It is a best-effort operation; failures are
// logged via t.Log but do not fail the test.
func LogSession(t *testing.T, invocation, output string) {
	t.Helper()

	d := logDir()
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Logf("warn: could not create log directory %s: %v", d, err)
		return
	}

	now := time.Now().UTC()
	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "-").Replace(t.Name())
	timestamp := now.Format("2006-01-02T15-04-05Z")
	logFile := filepath.Join(d, fmt.Sprintf("%s-%s.log", timestamp, safeName))

	var sb strings.Builder
	sb.WriteString("=== SESSION LOG ===\n")
	sb.WriteString(fmt.Sprintf("test:      %s\n", t.Name()))
	sb.WriteString(fmt.Sprintf("time:      %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("invoked:   %s\n", invocation))
	sb.WriteString("--- output ---\n")
	sb.WriteString(output)
	if !strings.HasSuffix(output, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("--- end ---\n")

	_ = os.WriteFile(logFile, []byte(sb.String()), 0o644)
	t.Logf("session log: %s", logFile)
}
