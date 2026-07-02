package form

// SelectOption represents a single option in a select input.
type SelectOption struct {
	Value string
	Label string
}

// ternary returns a if cond is true, else b.
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
