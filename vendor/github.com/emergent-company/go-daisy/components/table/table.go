package table

import (
	"github.com/emergent-company/go-daisy/components/ui"
	"github.com/emergent-company/go-daisy/shared"
)

func ternary(cond bool, a, b string) string { return shared.Ternary(cond, a, b) }

// ternaryInt returns a when cond is true, otherwise b.
func ternaryInt(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}

// ternaryVariant returns the error button variant when danger is set, else the zero value.
func ternaryVariant(danger bool) ui.ButtonVariant {
	if danger {
		return ui.ButtonError
	}
	return ""
}

// dataTablePageSize returns the page size with default fallback.
func dataTablePageSize(n int) int {
	if n == 0 {
		return 10
	}
	return n
}
