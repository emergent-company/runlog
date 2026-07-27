package shared

// Ternary returns a if cond is true, else b.
// Used by .templ files via thin package-level wrappers in each component package.
func Ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
