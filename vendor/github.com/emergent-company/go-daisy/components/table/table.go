package table

// ternary returns a if cond is true, else b.
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// dataTablePageSize returns the page size with default fallback.
func dataTablePageSize(n int) int {
	if n == 0 {
		return 10
	}
	return n
}
