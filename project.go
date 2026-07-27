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
func CreateProject(t *testing.T, home, srv, name string) string { //nolint:deadcode
	t.Helper()
	args := append([]string{"projects", "create", "--name", name}, ProjectCreateOrgArgs()...)
	out := MustRunCLIInDirWithHome(t, "", home, args...)
	logCLISuccessIfActive(t, "memory "+strings.Join(args, " "), out)
	t.Logf("projects create:\n%s", out)

	projectID := ParseProjectID(out)
	if projectID == "" {
		t.Fatalf("could not parse project ID from: %q", out)
	}
	t.Logf("project: %s (%s)", name, projectID)

	// Set project_id in config so commands that don't honour --project find it.
	setArgs := []string{"config", "set", "project_id", projectID}
	setOut := MustRunCLIInDirWithHome(t, "", home, setArgs...)
	logCLISuccessIfActive(t, "memory "+strings.Join(setArgs, " "), setOut)

	return projectID
}

// ProjectCreateOrgArgs returns ["--org-id", "<id>"] when MEMORY_ORG_ID is set,
// otherwise returns an empty slice.  Append to any `projects create` CLI call.
func ProjectCreateOrgArgs() []string { //nolint:deadcode
	return OrgIDArgs()
}

// OrgIDArgs returns ["--org-id", "<id>"] when MEMORY_ORG_ID is set,
// otherwise returns an empty slice.  Append to any CLI call that requires
// --org-id (provider configure, blueprints, agent-definitions, etc.).
func OrgIDArgs() []string { //nolint:deadcode
	if id := OrgID(); id != "" {
		return []string{"--org-id", id}
	}
	return nil
}

// DeleteProjectOnCleanup registers a t.Cleanup function that deletes the
// project when the test finishes.  Non-fatal: if deletion fails, it is logged.
// After successful deletion, the project is deregistered from the daemon
// (best-effort, fail-open).
func DeleteProjectOnCleanup(t *testing.T, home, projectID string) { //nolint:deadcode
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "memory", "projects", "delete", projectID)
		cmd.Env = append(FilteredEnv(), "HOME="+home, "PATH="+home+"/.memory/bin:"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("warn: failed to delete project %s: %v\n%s", projectID, err, out)
		} else {
			t.Logf("deleted project %s", projectID)
		}
	})
}

// RevokeTokenOnCleanup registers a t.Cleanup that runs `memory tokens revoke
// <tokenID>` using the isolated home directory.  If rl is non-nil, the cleanup
// opens a "Cleanup" section and records the invocation and outcome as structured
// RunLog events; otherwise it falls back to t.Logf.  Non-fatal: failures are
// logged but never fail the test.
func RevokeTokenOnCleanup(t *testing.T, rl *RunLog, home, tokenID string) { //nolint:deadcode
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
//	OPENAI_API_KEY    → provider "openai",  model from OPENAI_MODEL
//	DEEPSEEK_API_KEY  → provider "deepseek", model from DEEPSEEK_MODEL
//	GOOGLE_AI_API_KEY → provider "google",   model from GOOGLE_AI_MODEL
//
// model is empty when the corresponding *_MODEL variable is not set; callers
// should then omit --generative-model so the server auto-selects from its
// model catalog.  All three strings are empty when no provider is configured.
func ProviderFromEnv() (provider, apiKey, model string) { //nolint:deadcode
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return "openai", key, os.Getenv("OPENAI_MODEL")
	}
	if key := os.Getenv("DEEPSEEK_API_KEY"); key != "" {
		return "deepseek", key, os.Getenv("DEEPSEEK_MODEL")
	}
	if key := os.Getenv("GOOGLE_AI_API_KEY"); key != "" {
		return "google", key, os.Getenv("GOOGLE_AI_MODEL")
	}
	return "", "", ""
}

// ConfigureProvider configures an LLM provider at the project level via
// `memory provider configure-project`. provider is the provider type
// (e.g. "openai-compatible", "google", "google-vertex"). model is the generative
// model name; when empty --generative-model is omitted and the server picks
// from its catalog automatically.
func ConfigureProvider(t *testing.T, home, projectID, provider, apiKey, model string) { //nolint:deadcode
	t.Helper()
	args := []string{"provider", "configure-project", provider, "--project", projectID, "--api-key", apiKey}
	if baseURL := providerBaseURL(); baseURL != "" {
		args = append(args, "--base-url", baseURL)
	}
	if model != "" {
		args = append(args, "--generative-model", model)
	}
	out := MustRunCLIInDirWithHome(t, "", home, args...)
	t.Logf("provider configure-project %s:\n%s", provider, out)
}

// SetupTestProvider configures whichever LLM provider is available from
// environment variables (see ProviderFromEnv for priority order) at the
// project level via `memory provider configure-project`.
// projectID is required — callers must create the project first.
// If no provider env vars are set the test is skipped via t.Skip.
// If rl is non-nil the configure step is recorded in the RunLog.
func SetupTestProvider(t *testing.T, rl *RunLog, home, projectID string) { //nolint:deadcode
	t.Helper()

	if projectID == "" {
		DoSkipf(t, rl, "no project ID for provider setup — create project first")
	}

	provider, apiKey, model := ProviderFromEnv()
	if provider == "" {
		DoSkipf(t, rl,
			"no LLM provider configured — set DEEPSEEK_API_KEY, GOOGLE_AI_API_KEY, or OPENAI_API_KEY")
	}

	label := "memory provider configure-project " + provider
	if rl != nil {
		rl.Section("Configure LLM provider")
	}

	// When OPENAI_BASE_URL is set, use HTTP API directly because the CLI
	// `configure-project openai` does not accept --base-url.
	baseURL := providerBaseURL()
	if baseURL != "" && provider == "openai" {
		err := configureProviderHTTP(t, projectID, provider, apiKey, model, baseURL, rl)
		if err == nil {
			return
		}
		// Fall through to CLI or test.
	}

	args := []string{"provider", "configure-project", provider, "--project", projectID, "--api-key", apiKey}
	if baseURL != "" {
		args = append(args, "--base-url", baseURL)
	}
	if model != "" {
		args = append(args, "--generative-model", model)
	}

	out, err := RunCLIInDirWithHome(t, "", home, args...)
	if rl != nil {
		rl.CLIErr(label, out, err, 0)
	} else {
		t.Logf("%s:\n%s", label, out)
	}

	if err != nil {
		testArgs := []string{"provider", "test", "--project", projectID}
		testOut, testErr := RunCLIInDirWithHome(t, "", home, testArgs...)
		if rl != nil {
			rl.CLIErr("memory provider test", testOut, testErr, 0)
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

// configureProviderHTTP configures a provider via the HTTP API directly.
// Used when the CLI cannot express the full config (e.g. openai with custom base URL).
func configureProviderHTTP(t *testing.T, projectID, provider, apiKey, model, baseURL string, rl *RunLog) error {
	t.Helper()
	srv := os.Getenv("MEMORY_TEST_SERVER")
	token := os.Getenv("MEMORY_TEST_TOKEN")
	if srv == "" || token == "" {
		return fmt.Errorf("MEMORY_TEST_SERVER or MEMORY_TEST_TOKEN not set")
	}
	orgID := OrgID()
	if orgID == "" {
		return fmt.Errorf("no org ID for provider config")
	}

	provCfg := map[string]any{"apiKey": apiKey}
	if baseURL != "" {
		provCfg["baseUrl"] = baseURL
	}
	if model != "" {
		provCfg["generativeModel"] = model
	}
	body, _ := json.Marshal(provCfg)
	req, err := http.NewRequest("PUT",
		srv+"/api/v1/projects/"+projectID+"/providers/"+provider,
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Configure model-config.
	modelCfg, _ := json.Marshal(map[string]any{"generativeModel": provider + "/" + model})
	req2, _ := http.NewRequest("PUT",
		srv+"/api/v1/projects/"+projectID+"/model-config",
		bytes.NewReader(modelCfg))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("X-Org-ID", orgID)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return err
	}
	resp2.Body.Close()

	if rl != nil {
		rl.Printf("configured provider %s via HTTP for project %s", provider, projectID)
	} else {
		t.Logf("configured provider %s via HTTP for project %s", provider, projectID)
	}
	return nil
}

// ProviderFromEnvBaseURL returns the base URL for OpenAI-compatible providers
// from environment variables.
func ProviderFromEnvBaseURL() string { //nolint:deadcode
	return providerBaseURL()
}

// providerBaseURL returns the base URL for OpenAI-compatible providers.
// Uses DEEPSEEK_BASE_URL or OPENAI_BASE_URL env vars. Does not return a URL
// for "deepseek" — the server knows the default DeepSeek API endpoint.
func providerBaseURL() string {
	if u := os.Getenv("DEEPSEEK_BASE_URL"); u != "" {
		return u
	}
	if u := os.Getenv("OPENAI_BASE_URL"); u != "" {
		return u
	}
	return ""
}

// ConfigureGoogleProvider configures the Google AI provider at the project level.
// Deprecated: use ConfigureProvider(t, home, projectID, "google", apiKey, model) instead.
func ConfigureGoogleProvider(t *testing.T, home, projectID, apiKey, model string) { //nolint:deadcode
	t.Helper()
	ConfigureProvider(t, home, projectID, "google", apiKey, model)
}

// InstallBlueprint runs `memory blueprints <blueprintURL> --project <name> --upgrade`
// and fails the test if the output indicates errors.
func InstallBlueprint(t *testing.T, home, blueprintURL, projectName string) string { //nolint:deadcode
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
func UniqueProjectName(prefix string) string { //nolint:deadcode
	return fmt.Sprintf("%s-%s-%d", prefix, machineID(), time.Now().UnixMilli())
}

// machineID returns the first 8 hex characters of the SHA-256 hash of the
// system hostname. Falls back to "00000000" if the hostname cannot be retrieved.
func machineID() string { //nolint:deadcode
	host, err := os.Hostname()
	if err != nil {
		return "00000000"
	}
	sum := sha256.Sum256([]byte(host))
	return fmt.Sprintf("%x", sum[:4]) // 4 bytes = 8 hex chars
}

// containsErrors returns true when output contains "errors" but not "0 errors".
func containsErrors(out string) bool { //nolint:deadcode
	return contains(out, "errors") && !contains(out, "0 errors")
}

// contains is a thin wrapper used internally.
func contains(s, substr string) bool { //nolint:deadcode
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool { //nolint:deadcode
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


