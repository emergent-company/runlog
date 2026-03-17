// Package e2eframework — project.go
//
// Helpers for creating, configuring, and tearing down Memory projects in tests.
package runlog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// CreateProject creates an ephemeral project and returns its ID.
// It fails the test if the project cannot be created or the ID cannot be parsed.
// When MEMORY_ORG_ID is set, --org-id is appended automatically.
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

// ConfigureGoogleProvider configures the Google AI provider for a project.
func ConfigureGoogleProvider(t *testing.T, home, apiKey, model string) {
	t.Helper()
	out := MustRunCLIInDirWithHome(t, "", home,
		"provider", "configure", "google",
		"--api-key", apiKey,
		"--generative-model", model,
	)
	t.Logf("provider configure:\n%s", out)
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

// UniqueProjectName returns a unique project name using the given prefix and
// current time in milliseconds, suitable for ephemeral test projects.
func UniqueProjectName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
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
