package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runlog "github.com/emergent-company/runlog"
)

// setupSkillsTestDir creates a temporary project root with an opencode marker
// directory and returns the root path. Skills come from the embedded FS, so no
// source directories are needed.
func setupSkillsTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Create opencode marker dir so detectTools finds it.
	if err := os.MkdirAll(filepath.Join(root, ".opencode"), 0755); err != nil {
		t.Fatal(err)
	}

	return root
}

// TestDiscoverEmbeddedSkills verifies that the embedded skills are discoverable.
// TestDiscoverEmbeddedSkills verifies all embedded skill directories are discoverable and have the runlog- prefix.
func TestDiscoverEmbeddedSkills(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("discoverembeddedskills")
	df.Event("log", "discoverembeddedskills")

	skills, err := discoverEmbeddedSkills()
	if err != nil {
		t.Fatalf("discoverEmbeddedSkills: %v", err)
	}
	if len(skills) != 5 {
		df.Event("assertion", "FAIL: expected 5 embedded skills")
		t.Errorf("expected 5 embedded skills, got %d", len(skills))
	} else {
		df.Event("assertion", "found 5 embedded skills")
	}

	// All skills must have runlog- prefix.
	for _, s := range skills {
		if !strings.HasPrefix(s.Name, "runlog-") {
			t.Errorf("skill %q does not have runlog- prefix", s.Name)
		}
	}

	// Check for expected skill names.
	expected := map[string]bool{
		"runlog-test-designer":      false,
		"runlog-verify-e2e-changes": false,
		"runlog-verify-runs":        false,
		"runlog-clear":              false,
		"runlog-install-skills":     false,
	}
	for _, s := range skills {
		if _, ok := expected[s.Name]; ok {
			expected[s.Name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected embedded skill %q not found", name)
		}
	}
}

// TestEmbeddedSkillsHaveSKILLMD verifies each embedded skill directory
// contains a SKILL.md file.
// TestEmbeddedSkillsHaveSKILLMD verifies each embedded skill directory contains a SKILL.md file with valid frontmatter.
func TestEmbeddedSkillsHaveSKILLMD(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("embeddedskillshaveskillmd")
	df.Event("log", "embeddedskillshaveskillmd")

	skills, err := discoverEmbeddedSkills()
	if err != nil {
		t.Fatalf("discoverEmbeddedSkills: %v", err)
	}

	for _, s := range skills {
		path := "skills/" + s.Name + "/SKILL.md"
		data, err := fs.ReadFile(embeddedSkills, path)
		if err != nil {
			t.Errorf("skill %s: missing SKILL.md: %v", s.Name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("skill %s: SKILL.md is empty", s.Name)
			continue
		}
		// Verify frontmatter contains the skill name.

		content := string(data)
		if !strings.Contains(content, "name: "+s.Name) {
			t.Errorf("skill %s: SKILL.md frontmatter missing 'name: %s'", s.Name, s.Name)
		}
	}
}

// TestSkillsInstall_HappyPath verifies a clean install copies embedded skills.
// TestSkillsInstall_HappyPath verifies a clean install copies all embedded skills to the target directory.
func TestSkillsInstall_HappyPath(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall happypath")
	df.Event("log", "skillsinstall happypath")

	root := setupSkillsTestDir(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := cmdSkillsInstall([]string{"--tools", "opencode"}); err != nil {
		t.Fatalf("cmdSkillsInstall: %v", err)
	}

	// Verify all 5 skills were installed.
	skills, _ := discoverEmbeddedSkills()
	for _, s := range skills {
		dst := filepath.Join(root, ".opencode", "skills", s.Name, "SKILL.md")
		if _, err := os.Stat(dst); err != nil {
			t.Errorf("expected installed file %s, got: %v", dst, err)
		}
	}
}

// TestSkillsInstall_SkipExisting verifies that an existing install is skipped
// without --force.
// TestSkillsInstall_SkipExisting verifies existing skill installations are not overwritten without the --force flag.
func TestSkillsInstall_SkipExisting(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall skipexisting")
	df.Event("log", "skillsinstall skipexisting")

	root := setupSkillsTestDir(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Pre-create one destination skill with a sentinel file.
	dst := filepath.Join(root, ".opencode", "skills", "runlog-clear")
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
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
// TestSkillsInstall_ForceOverwrite verifies the --force flag removes and replaces existing skill installations.
func TestSkillsInstall_ForceOverwrite(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall forceoverwrite")
	df.Event("log", "skillsinstall forceoverwrite")

	root := setupSkillsTestDir(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Pre-create a destination with a stale file.
	dst := filepath.Join(root, ".opencode", "skills", "runlog-clear")
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
	// The actual SKILL.md from the embedded source should be present.
	installed := filepath.Join(dst, "SKILL.md")
	if _, err := os.Stat(installed); err != nil {
		t.Errorf("SKILL.md not installed after --force: %v", err)
	}
}

// TestSkillsInstall_DryRun verifies that --dry-run prints actions without
// writing any files.
// TestSkillsInstall_DryRun verifies --dry-run prints the installation plan without writing any files.
func TestSkillsInstall_DryRun(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall dryrun")
	df.Event("log", "skillsinstall dryrun")

	root := setupSkillsTestDir(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := cmdSkillsInstall([]string{"--tools", "opencode", "--dry-run"}); err != nil {
		t.Fatalf("cmdSkillsInstall --dry-run: %v", err)
	}

	// No skill directories should have been created.
	skills, _ := discoverEmbeddedSkills()
	for _, s := range skills {
		dst := filepath.Join(root, ".opencode", "skills", s.Name)

		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Errorf("--dry-run should not have created %s", dst)
		}
	}
}

// TestSkillsInstall_UnknownTool verifies that an unknown --tools value returns
// an error.
// TestSkillsInstall_UnknownTool verifies an invalid --tools value returns an error.
func TestSkillsInstall_UnknownTool(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall unknowntool")
	df.Event("log", "skillsinstall unknowntool")

	root := setupSkillsTestDir(t)

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	err := cmdSkillsInstall([]string{"--tools", "notarealtool"})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "unknown tool ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestSkillsList verifies the list subcommand runs without error.
// TestSkillsList verifies the skills list subcommand runs without error and prints skill names.
func TestSkillsList(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillslist")
	df.Event("log", "skillslist")

	root := setupSkillsTestDir(t)
	err := cmdSkillsList(root)
	if err != nil {
		t.Fatalf("cmdSkillsList: %v", err)
	}
}

// TestSkillsInstall_MultipleTools verifies installing to multiple tools at once.
// TestSkillsInstall_MultipleTools verifies installing skills to multiple tool directories at once.
func TestSkillsInstall_MultipleTools(t *testing.T) {

	df := runlog.NewDogfoodRun(t, "skills")
	defer df.Done()
	df.Describe("skillsinstall multipletools")
	df.Event("log", "skillsinstall multipletools")

	root := setupSkillsTestDir(t)

	// Also create agents marker.
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0755); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := cmdSkillsInstall([]string{"--tools", "opencode,agents"}); err != nil {
		t.Fatalf("cmdSkillsInstall: %v", err)
	}

	// Verify skills installed to both directories.
	skills, _ := discoverEmbeddedSkills()
	for _, s := range skills {
		for _, dir := range []string{".opencode/skills", ".agents/skills"} {
			dst := filepath.Join(root, dir, s.Name, "SKILL.md")
			if _, err := os.Stat(dst); err != nil {
				t.Errorf("expected installed file %s, got: %v", dst, err)
			}
		}
	}
}
