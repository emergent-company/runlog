package runlog

import (
	"bufio"
	"os"
	"strings"
)

// DiscoverLintersFromLefthook parses lefthook.yml from workDir and extracts
// jobs from the given groupName (e.g. "lint", "pre-commit") as LinterDef entries.
// Returns empty slice if lefthook.yml is not found or has no matching group.
func DiscoverLintersFromLefthook(workDir, groupName string) ([]LinterDef, error) {
	path := workDir + "/lefthook.yml"
	if workDir == "" {
		path = "lefthook.yml"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var linters []LinterDef
	inTargetGroup := false
	inLintJobs := false
	var current *LinterDef
	var runBuffer strings.Builder
	inRunBlock := false
	nameIndent := 0
	targetPrefix := groupName + ":"

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Detect target group (top-level, no leading space)
		if trimmed == targetPrefix && !isIndented(line) {
			inTargetGroup = true
			continue
		}

		if !inTargetGroup {
			continue
		}

		// Non-indented line means we've left the group block
		if !isIndented(line) {
			break
		}

		// Inside lint group
		if trimmed == "jobs:" {
			inLintJobs = true
			continue
		}

		if !inLintJobs {
			continue
		}

		// Handle multi-line run block
		if inRunBlock {
			// If this line is indented MORE than the - name: line, it's block content
			indent := countIndent(line)
			if indent > nameIndent {
				runBuffer.WriteString(strings.TrimLeft(line, " \t") + "\n")
				continue
			}
			// Line is at or above the name level — end the block
			if current != nil {
				cmd := strings.TrimSpace(runBuffer.String())
				cmd = strings.TrimRight(cmd, "\n")
				current.Command = cmd
			}
			runBuffer.Reset()
			inRunBlock = false
		}

		if strings.HasPrefix(trimmed, "- name:") {
			// Flush previous linter
			if current != nil {
				linters = append(linters, *current)
			}
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
			name = strings.Trim(name, "\"'")
			nameIndent = countIndent(line)
			current = &LinterDef{Name: name}
			continue
		}

		if strings.HasPrefix(trimmed, "run:") && current != nil {
			runVal := strings.TrimSpace(strings.TrimPrefix(trimmed, "run:"))
			if runVal == "|" {
				inRunBlock = true
				runBuffer.Reset()
			} else {
				runVal = strings.Trim(runVal, "\"'")
				current.Command = runVal
			}
			continue
		}
	}

	// Flush last linter
	if current != nil {
		if inRunBlock {
			cmd := strings.TrimSpace(runBuffer.String())
			cmd = strings.TrimRight(cmd, "\n")
			current.Command = cmd
		}
		linters = append(linters, *current)
	}

	return linters, scanner.Err()
}

func isIndented(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func countIndent(line string) int {
	n := 0
	for _, c := range line {
		if c == ' ' {
			n++
		} else if c == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}

// MergeLinters merges config linters with lefthook-discovered linters.
// Config linters take precedence (override same-name lefthook linters).
func MergeLinters(cfg []LinterDef, lefthook []LinterDef) []LinterDef {
	seen := make(map[string]bool, len(cfg))
	for _, l := range cfg {
		seen[l.Name] = true
	}
	result := make([]LinterDef, 0, len(cfg)+len(lefthook))
	result = append(result, cfg...)
	for _, l := range lefthook {
		if !seen[l.Name] {
			result = append(result, l)
			seen[l.Name] = true
		}
	}
	return result
}

// EnsureRunAllCommand returns a command suitable for "Run All" mode:
// strips {staged_files} placeholders since Run All operates on all files.
func EnsureRunAllCommand(cmd string) string {
	s := cmd
	s = strings.ReplaceAll(s, "{staged_files}", "")
	s = strings.TrimSpace(s)
	return s
}
