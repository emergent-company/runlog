// Package e2eframework — skills.go
//
// Helpers for verifying installed memory skills in tests.
package runlog

import (
	"os"
	"path/filepath"
	"testing"
)

// VerifySkillInstalled asserts that the named skill directory contains a valid
// SKILL.md with non-empty name and description fields and that the name matches
// the directory name.
//
// If rl is non-nil, a "skill" event is emitted under the current section with
// the message "Verify <skillName> skill", recording the parsed name,
// description, and whether the name matched the directory.  All assertion
// failures are reported via t.Errorf (non-fatal).
func VerifySkillInstalled(t *testing.T, rl *RunLog, skillsDir, skillName string) {  //nolint:deadcode
	t.Helper()

	skillMDPath := filepath.Join(skillsDir, skillName, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Errorf("SKILL.md not found for skill %s: %v", skillName, err)
		if rl != nil {
			rl.Event("skill", "Verify "+skillName+" skill", map[string]any{
				"name":             "",
				"desc":             "",
				"name_matches_dir": false,
				"error":            err.Error(),
			})
		}
		return
	}

	name, desc := ParseFrontmatterFields(string(data))

	if name == "" {
		t.Errorf("SKILL.md for %s has empty 'name' field", skillName)
	}
	if desc == "" {
		t.Errorf("SKILL.md for %s has empty 'description' field", skillName)
	}
	nameMatchesDir := name == skillName
	if !nameMatchesDir {
		t.Errorf("SKILL.md name %q does not match directory name %q", name, skillName)
	}

	if rl != nil {
		rl.Event("skill", "Verify "+skillName+" skill", map[string]any{
			"name":             name,
			"desc":             desc,
			"name_matches_dir": nameMatchesDir,
		})
	}
}
