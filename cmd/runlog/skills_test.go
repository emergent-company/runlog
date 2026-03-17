package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupSkillsTestDir creates a temporary project root with skill source dirs
// and returns the root path. The caller is responsible for cleanup.
func setupSkillsTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Create source skill dirs.
	srcSkill := filepath.Join(root, ".agents", "skills", "my-skill")
	if err := os.MkdirAll(srcSkill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcSkill, "SKILL.md"), []byte("---\nname: my-skill\ndescription: test\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create opencode marker dir so detectTools finds it.
	if err := os.MkdirAll(filepath.Join(root, ".opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	return root
}

// TestSkillsInstall_HappyPath verifies a clean install copies skill dirs.
func TestSkillsInstall_HappyPath(t *testing.T) {
	root := setupSkillsTestDir(t)

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := cmdSkillsInstall([]string{"--tools", "opencode"}); err != nil {
		t.Fatalf("cmdSkillsInstall: %v", err)
	}

	dst := filepath.Join(root, ".opencode", "skills", "my-skill", "SKILL.md")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected installed file %s, got: %v", dst, err)
	}
}

// TestSkillsInstall_SkipExisting verifies that an existing install is skipped
// without --force.
func TestSkillsInstall_SkipExisting(t *testing.T) {
	root := setupSkillsTestDir(t)

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	// Pre-create the destination.
	dst := filepath.Join(root, ".opencode", "skills", "my-skill")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a sentinel file that should NOT be overwritten.
	sentinel := filepath.Join(dst, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := cmdSkillsInstall([]string{"--tools", "opencode"}); err != nil {
		t.Fatalf("cmdSkillsInstall: %v", err)
	}

	// Sentinel should still be intact (not overwritten).
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel disappeared: %v", err)
	}
	if string(data) != "original" {
		t.Errorf("sentinel was overwritten; got %q", data)
	}
}

// TestSkillsInstall_ForceOverwrite verifies --force removes and replaces an
// existing install.
func TestSkillsInstall_ForceOverwrite(t *testing.T) {
	root := setupSkillsTestDir(t)

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	// Pre-create a destination with a stale file.
	dst := filepath.Join(root, ".opencode", "skills", "my-skill")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dst, "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := cmdSkillsInstall([]string{"--tools", "opencode", "--force"}); err != nil {
		t.Fatalf("cmdSkillsInstall --force: %v", err)
	}

	// Stale file should be gone.
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file still exists after --force install")
	}
	// The actual SKILL.md from the source should be present.
	installed := filepath.Join(dst, "SKILL.md")
	if _, err := os.Stat(installed); err != nil {
		t.Errorf("SKILL.md not installed after --force: %v", err)
	}
}

// TestSkillsInstall_DryRun verifies that --dry-run prints actions without
// writing any files.
func TestSkillsInstall_DryRun(t *testing.T) {
	root := setupSkillsTestDir(t)

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := cmdSkillsInstall([]string{"--tools", "opencode", "--dry-run"}); err != nil {
		t.Fatalf("cmdSkillsInstall --dry-run: %v", err)
	}

	// Nothing should have been written.
	dst := filepath.Join(root, ".opencode", "skills", "my-skill")
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("--dry-run should not have created %s", dst)
	}
}

// TestSkillsInstall_UnknownTool verifies that an unknown --tools value returns
// an error.
func TestSkillsInstall_UnknownTool(t *testing.T) {
	root := setupSkillsTestDir(t)

	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	err := cmdSkillsInstall([]string{"--tools", "notarealtool"})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if !containsStr(err.Error(), "unknown tool ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStrHelper(s, sub))
}

func containsStrHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
