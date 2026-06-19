// Package e2eframework — project.go
//
// Helpers for creating, configuring, and tearing down Memory projects in tests.
package runlog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// CreateProject creates an ephemeral project and returns its ID.
// It fails the test if the project cannot be created or the ID cannot be parsed.
// When MEMORY_ORG_ID is set, --org-id is appended automatically.
// If RUNLOG_RUN_ID and RUNLOG_DAEMON_URL are set, the project is registered
// with the local daemon for orphan tracking (best-effort, fail-open).
func CreateProject(t *testing.T, home, srv, name string) string {
	t.Helper()
	args := append([]string{"projects", "create", "--name", name}, ProjectCreateOrgArgs()...)
	out := MustRunCLIInDirWithHome(t, "", home, args...)
	t.Logf("projects create:\n%s", out)

	projectID := ParseProjectID(out)
	if projectID == "" {
		t.Fatalf("could not parse project ID from: %q", out)
	}
	t.Logf("project: %s (%s)", name, projectID)

	// Set project_id in config so commands that don't honour --project find it.
	MustRunCLIInDirWithHome(t, "", home, "config", "set", "project_id", projectID)

	// Register with daemon for orphan tracking (best-effort).
	daemonRegisterResource(projectID)

	return projectID
}

// ProjectCreateOrgArgs returns ["--org-id", "<id>"] when MEMORY_ORG_ID is set,
// otherwise returns an empty slice.  Append to any `projects create` CLI call.
func ProjectCreateOrgArgs() []string {
	return OrgIDArgs()
}

// OrgIDArgs returns ["--org-id", "<id>"] when MEMORY_ORG_ID is set,
// otherwise returns an empty slice.  Append to any CLI call that requires
// --org-id (provider configure, blueprints, agent-definitions, etc.).
func OrgIDArgs() []string {
	if id := OrgID(); id != "" {
		return []string{"--org-id", id}
	}
	return nil
}

// DeleteProjectOnCleanup registers a t.Cleanup function that deletes the
// project when the test finishes.  Non-fatal: if deletion fails, it is logged.
// After successful deletion, the project is deregistered from the daemon
// (best-effort, fail-open).
func DeleteProjectOnCleanup(t *testing.T, home, projectID string) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "memory", "projects", "delete", projectID)
		cmd.Env = append(FilteredEnv(), "HOME="+home, "PATH="+home+"/.memory/bin:"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("warn: failed to delete project %s: %v\n%s", projectID, err, out)
		} else {
			t.Logf("deleted project %s", projectID)
			// Deregister from daemon — best-effort, failure is non-fatal.
			daemonDeregisterResource(projectID)
		}
	})
}

// RevokeTokenOnCleanup registers a t.Cleanup that runs `memory tokens revoke
// <tokenID>` using the isolated home directory.  If rl is non-nil, the cleanup
// opens a "Cleanup" section and records the invocation and outcome as structured
// RunLog events; otherwise it falls back to t.Logf.  Non-fatal: failures are
// logged but never fail the test.
func RevokeTokenOnCleanup(t *testing.T, rl *RunLog, home, tokenID string) {
	t.Cleanup(func() {
		if rl != nil {
			rl.Section("Cleanup")
		}
		out, err := RunCLIInDirWithHome(t, "", home, "tokens", "revoke", tokenID)
		if rl != nil {
			rl.CLIStep("Revoke token "+tokenID, "memory tokens revoke "+tokenID, strings.TrimSpace(out))
		}
		if err != nil {
			if rl != nil {
				rl.Printf("warn: failed to revoke account token %s: %v", tokenID, err)
			} else {
				t.Logf("warn: failed to revoke account token %s: %v\noutput: %s", tokenID, err, out)
			}
			return
		}
		if rl == nil {
			t.Logf("revoked account token %s", tokenID)
		}
	})
}

// ProviderFromEnv returns the LLM provider type, API key, and generative model
// name by inspecting environment variables in priority order:
//
//	DEEPSEEK_API_KEY  → provider "deepseek", model from DEEPSEEK_MODEL
//	GOOGLE_AI_API_KEY → provider "google",   model from GOOGLE_AI_MODEL
//	OPENAI_API_KEY    → provider "openai",   model from OPENAI_MODEL
//
// model is empty when the corresponding *_MODEL variable is not set; callers
// should then omit --generative-model so the server auto-selects from its
// model catalog.  All three strings are empty when no provider is configured.
func ProviderFromEnv() (provider, apiKey, model string) {
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		return "deepseek", key, os.Getenv("DEEPSEEK_MODEL")
	}
	if key := os.Getenv("GOOGLE_AI_API_KEY"); key != "" {
		return "google", key, os.Getenv("GOOGLE_AI_MODEL")
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return "openai", key, os.Getenv("OPENAI_MODEL")
	}
	return "", "", ""
}

// ConfigureProvider configures an LLM provider via the memory CLI.
// provider is the provider type (e.g. "deepseek", "google", "openai").
// model is the generative model name; when empty --generative-model is omitted
// and the server picks from its catalog automatically.
func ConfigureProvider(t *testing.T, home, provider, apiKey, model string) {
	t.Helper()
	args := []string{"provider", "configure", provider, "--api-key", apiKey}
	if model != "" {
		args = append(args, "--generative-model", model)
	}
	out := MustRunCLIInDirWithHome(t, "", home, args...)
	t.Logf("provider configure %s:\n%s", provider, out)
}

// SetupTestProvider configures whichever LLM provider is available from
// environment variables (see ProviderFromEnv for priority order).
// If no provider env vars are set the test is skipped via t.Skip.
// If rl is non-nil the configure step is recorded in the RunLog.
// The function falls back to "provider test" if configure fails, which is
// useful when credentials are already stored on the server.
func SetupTestProvider(t *testing.T, rl *RunLog, home string) {
	t.Helper()

	provider, apiKey, model := ProviderFromEnv()
	if provider == "" {
		DoSkipf(t, rl,
			"no LLM provider configured — set DEEPSEEK_API_KEY, GOOGLE_AI_API_KEY, or OPENAI_API_KEY")
	}

	label := "memory provider configure " + provider
	if rl != nil {
		rl.Section("Configure LLM provider")
	}

	args := []string{"provider", "configure", provider, "--api-key", apiKey}
	if model != "" {
		args = append(args, "--generative-model", model)
	}
	args = append(args, OrgIDArgs()...)

	out, err := RunCLIInDirWithHome(t, "", home, args...)
	if rl != nil {
		rl.CLIErr(label, out, err)
	} else {
		t.Logf("%s:\n%s", label, out)
	}

	if err != nil {
		// Configure may fail if credentials are already stored; verify via test.
		testArgs := append([]string{"provider", "test"}, OrgIDArgs()...)
		testOut, testErr := RunCLIInDirWithHome(t, "", home, testArgs...)
		if rl != nil {
			rl.CLIErr("memory provider test", testOut, testErr)
		}
		if testErr != nil {
			if rl != nil {
				rl.Failf("provider configure failed and provider test also failed: %v\n%s", testErr, testOut)
			} else {
				t.Fatalf("provider configure failed and provider test also failed: %v\n%s", testErr, testOut)
			}
		}
	}
}

// ConfigureGoogleProvider configures the Google AI provider.
// Deprecated: use ConfigureProvider(t, home, "google", apiKey, model) instead.
func ConfigureGoogleProvider(t *testing.T, home, apiKey, model string) {
	t.Helper()
	ConfigureProvider(t, home, "google", apiKey, model)
}

// InstallBlueprint runs `memory blueprints <blueprintURL> --project <name> --upgrade`
// and fails the test if the output indicates errors.
func InstallBlueprint(t *testing.T, home, blueprintURL, projectName string) string {
	t.Helper()
	out := MustRunCLIInDirWithHome(t, "", home,
		"blueprints", blueprintURL,
		"--project", projectName,
		"--upgrade",
	)
	t.Logf("blueprint install:\n%s", out)

	if containsErrors(out) {
		t.Fatalf("blueprint install reported errors:\n%s", out)
	}
	return out
}

// UniqueProjectName returns a unique project name using the given prefix,
// a short machine ID derived from the hostname, and current time in milliseconds.
// Format: <prefix>-<mid8>-<timestamp_ms>
func UniqueProjectName(prefix string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, machineID(), time.Now().UnixMilli())
}

// machineID returns the first 8 hex characters of the SHA-256 hash of the
// system hostname. Falls back to "00000000" if the hostname cannot be retrieved.
func machineID() string {
	host, err := os.Hostname()
	if err != nil {
		return "00000000"
	}
	sum := sha256.Sum256([]byte(host))
	return fmt.Sprintf("%x", sum[:4]) // 4 bytes = 8 hex chars
}

// containsErrors returns true when output contains "errors" but not "0 errors".
func containsErrors(out string) bool {
	return contains(out, "errors") && !contains(out, "0 errors")
}

// contains is a thin wrapper used internally.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Daemon integration helpers (best-effort, fail-open)
// ─────────────────────────────────────────────────────────────────────────────

// daemonRegisterResource registers a project with the local runlog daemon.
// Called after CreateProject succeeds. No-op if RUNLOG_RUN_ID or
// RUNLOG_DAEMON_URL are not set. All errors are silently ignored.
func daemonRegisterResource(projectID string) {
	runID := os.Getenv("RUNLOG_RUN_ID")
	dURL := os.Getenv("RUNLOG_DAEMON_URL")
	if runID == "" || dURL == "" {
		return
	}

	body, _ := json.Marshal(map[string]string{
		"resource_id":   projectID,
		"resource_type": "project",
	})

	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := strings.TrimRight(dURL, "/") + "/runs/" + runID + "/resources"
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// daemonDeregisterResource deregisters a project from the local runlog daemon
// after it has been successfully deleted from the server.
// Called after DeleteProjectOnCleanup succeeds. All errors are silently ignored.
func daemonDeregisterResource(projectID string) {
	runID := os.Getenv("RUNLOG_RUN_ID")
	dURL := os.Getenv("RUNLOG_DAEMON_URL")
	if runID == "" || dURL == "" {
		return
	}

	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := strings.TrimRight(dURL, "/") + "/runs/" + runID + "/resources/" + projectID
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
