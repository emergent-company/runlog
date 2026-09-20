package form

import "github.com/emergent-company/go-daisy/shared"

// SelectOption represents a single option in a select input.
type SelectOption struct {
	Value string
	Label string
}

func ternary(cond bool, a, b string) string { return shared.Ternary(cond, a, b) }

func entriesToMaps(entries []StructuredEntry) []map[string]string {
	if entries == nil {
		return nil
	}
	out := make([]map[string]string, len(entries))
	for i, e := range entries {
		out[i] = map[string]string(e)
	}
	return out
}
