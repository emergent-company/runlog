// Package nav provides navigation Templ components (tabs, top bar, breadcrumbs).
package nav

// BreadcrumbItem is a single breadcrumb entry.
// If Href is empty the item renders as plain text (no link).
type BreadcrumbItem struct {
	Label  string
	Href   string // empty = no link, plain text
	Active bool   // current-page styling
	Icon   string // optional Iconify icon name
}

// BreadcrumbsDivider controls the separator between breadcrumb items.
type BreadcrumbsDivider string

const (
	BreadcrumbsDividerDefault BreadcrumbsDivider = ""  // DaisyUI default ">"
	BreadcrumbsDividerSlash   BreadcrumbsDivider = "/" // slash separator
)

// Crumbs builds a breadcrumb item slice for use with PageHeader.
// Alternate label/URL pairs: label first, then optional URL (must start with /).
// Example — single step:  nav.Crumbs("Cases")
// Example — two steps:    nav.Crumbs("Cases", "/app/cases", "Test 3")
func Crumbs(args ...string) []BreadcrumbItem {
	var items []BreadcrumbItem
	for i := 0; i < len(args); i++ {
		item := BreadcrumbItem{Label: args[i]}
		if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] == '/' {
			item.Href = args[i+1]
			i++
		}
		items = append(items, item)
	}
	return items
}
