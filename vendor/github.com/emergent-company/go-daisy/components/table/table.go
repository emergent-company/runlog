package table

import "github.com/emergent-company/go-daisy/shared"

func ternary(cond bool, a, b string) string { return shared.Ternary(cond, a, b) }

// dataTablePageSize returns the page size with default fallback.
func dataTablePageSize(n int) int {
	if n == 0 {
		return 10
	}
	return n
}
