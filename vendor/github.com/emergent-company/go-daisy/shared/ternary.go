package shared

// Ternary implements the ternary conditional operator for strings.
// If cond is true, returns a; otherwise returns b.
func Ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
