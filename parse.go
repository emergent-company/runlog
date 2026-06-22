// Package e2eframework — parse.go
//
// Helpers for extracting structured values from CLI output and JSON strings.
package runlog

import (
	"encoding/json"
	"strings"
)

// ParseProjectID extracts a UUID from `memory projects create` output.
// The CLI prints the project name with its ID in parentheses, e.g.:
//
//	Created project "my-project" (a1b2c3d4-…)
func ParseProjectID(output string) string {  //nolint:deadcode
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "(") {
			continue
		}
		start := strings.LastIndex(line, "(")
		end := strings.LastIndex(line, ")")
		if start < 0 || end <= start {
			continue
		}
		candidate := strings.TrimSpace(line[start+1 : end])
		// Minimal UUID check: 36 chars, 4 hyphens.
		if len(candidate) == 36 && strings.Count(candidate, "-") == 4 {
			return candidate
		}
	}
	return ""
}

// ParseAgentDefsModel extracts the first model name from `memory agent-definitions list` output.
// The CLI prints one block per definition; each block contains a line like:
//
//	Model:      gemini-2.5-flash
//
// Returns the first model name found, or "" if none is present.
// When a test overrides all agents to the same model, this returns that model.
// Used to tag test runs with the actual model in use rather than a hardcoded constant.
func ParseAgentDefsModel(output string) string {  //nolint:deadcode
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Model:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "Model:"))
			if val != "" {
				return val
			}
		}
	}
	return ""
}

// ParseAgentID extracts a UUID from `memory agents create` output.
// Format:   ID:   <uuid>
func ParseAgentID(output string) string {  //nolint:deadcode
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID:") {
			candidate := strings.TrimSpace(strings.TrimPrefix(line, "ID:"))
			if len(candidate) == 36 && strings.Count(candidate, "-") == 4 {
				return candidate
			}
		}
	}
	return ""
}

// ParseJSONField extracts a top-level string field from a JSON object string.
func ParseJSONField(jsonStr, field string) string {  //nolint:deadcode
	var obj map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonStr)), &obj); err != nil {
		return ""
	}
	v, _ := obj[field].(string)
	return v
}

// CompactRunsOutput converts the multi-line human-readable output of
// `memory agents runs` into one compact line per run, suitable for poll logs.
//
// Input (example):
//
//	Found 2 run(s):
//
//	1. Run c0f5e9d1-4020-423f-a2e7-de239a1de5ef
//	   Status:    success
//	   Started:   2026-03-10 11:06:27
//	   Completed: 2026-03-10 11:06:30
//	   Duration:  3496ms
//
// Output (example):
//
//	run 1: c0f5e9d1 status=success started=11:06:27 completed=11:06:30 dur=3496ms
//
// Returns "(no runs)" when the output is empty or contains no run blocks.
func CompactRunsOutput(runsOut string) string {  //nolint:deadcode
	type runEntry struct {
		num       string
		id        string
		status    string
		started   string
		completed string
		duration  string
		errMsg    string
		skipped   string
	}

	var runs []runEntry
	var cur *runEntry

	for _, raw := range strings.Split(runsOut, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		// "1. Run <uuid>"
		if len(line) > 5 && line[1] == '.' && strings.HasPrefix(line[3:], "Run ") {
			if cur != nil {
				runs = append(runs, *cur)
			}
			parts := strings.SplitN(line, " ", 3)
			num := strings.TrimSuffix(parts[0], ".")
			id := ""
			if len(parts) == 3 {
				id = parts[2]
			}
			cur = &runEntry{num: num, id: id}
			continue
		}

		if cur == nil {
			continue
		}

		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "Status":
			cur.status = val
		case "Started":
			// keep only HH:MM:SS portion
			if len(val) >= 19 {
				cur.started = val[11:19]
			} else {
				cur.started = val
			}
		case "Completed":
			if len(val) >= 19 {
				cur.completed = val[11:19]
			} else {
				cur.completed = val
			}
		case "Duration":
			cur.duration = val
		case "Error":
			cur.errMsg = val
		case "Skipped":
			cur.skipped = val
		}
	}
	if cur != nil {
		runs = append(runs, *cur)
	}

	if len(runs) == 0 {
		return "(no runs)"
	}

	lines := make([]string, 0, len(runs))
	for _, r := range runs {
		// Truncate UUID to first 8 chars for brevity.
		id := r.id
		if len(id) > 8 {
			id = id[:8]
		}

		var sb strings.Builder
		sb.WriteString("run " + r.num + ": " + id + " status=" + r.status)
		if r.started != "" {
			sb.WriteString(" started=" + r.started)
		}
		if r.completed != "" {
			sb.WriteString(" completed=" + r.completed)
		}
		if r.duration != "" {
			sb.WriteString(" dur=" + r.duration)
		}
		if r.errMsg != "" {
			sb.WriteString(" error=" + r.errMsg)
		}
		if r.skipped != "" {
			sb.WriteString(" skipped=" + r.skipped)
		}
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}

// AllRunsTerminal returns true when runsOut contains at least one run and every
// run is in a terminal status (success, error, failed, skipped).
// Used in poll loops to detect a stuck state.
func AllRunsTerminal(runsOut string) bool {  //nolint:deadcode
	terminalStatuses := map[string]bool{
		"success": true,
		"error":   true,
		"failed":  true,
		"skipped": true,
	}
	count := 0
	for _, raw := range strings.Split(runsOut, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "Status:") {
			continue
		}
		status := strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
		if !terminalStatuses[status] {
			return false
		}
		count++
	}
	return count > 0
}

// ParseFrontmatterFields extracts the `name:` and `description:` values from
// a SKILL.md YAML frontmatter block.  It intentionally avoids importing a YAML
// library so the test module stays dependency-free.
func ParseFrontmatterFields(content string) (name, description string) {  //nolint:deadcode
	const delim = "---"
	first := strings.Index(content, delim)
	if first < 0 {
		return
	}
	rest := content[first+len(delim):]
	second := strings.Index(rest, delim)
	if second < 0 {
		return
	}
	frontmatter := rest[:second]

	for _, line := range strings.Split(frontmatter, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			switch k {
			case "name":
				name = v
			case "description":
				description = v
			}
		}
	}
	return
}

// PrettyJSONOutput detects whether output is JSON (object or array), and if so
// re-indents it and removes the noisy "content_hash" field before returning.
// Non-JSON output is returned unchanged.
func PrettyJSONOutput(output string) string {  //nolint:deadcode
	trimmed := strings.TrimSpace(output)
	if len(trimmed) == 0 {
		return output
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return output
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return output
	}
	removeContentHash(v)
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return output
	}
	return string(b)
}

// removeContentHash recursively removes "content_hash" keys from maps decoded
// from JSON. The field contains raw binary bytes that render as garbage.
func removeContentHash(v any) {  //nolint:deadcode
	switch val := v.(type) {
	case map[string]any:
		delete(val, "content_hash")
		for _, child := range val {
			removeContentHash(child)
		}
	case []any:
		for _, item := range val {
			removeContentHash(item)
		}
	}
}

// ParseLineField finds the first line in s that contains key and returns the
// trimmed value after it.  Useful for extracting labelled fields from CLI
// table output, e.g.:
//
//	"  ID:      abc123"  with key "ID:"  →  "abc123"
func ParseLineField(s, key string) string {  //nolint:deadcode
	for _, line := range strings.Split(s, "\n") {
		idx := strings.Index(line, key)
		if idx == -1 {
			continue
		}
		val := strings.TrimSpace(line[idx+len(key):])
		if val != "" {
			return val
		}
	}
	return ""
}
