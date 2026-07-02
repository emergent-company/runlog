// Package nav provides navigation Templ components (tabs, top bar, breadcrumbs).
package nav

// BreadcrumbStep is a single step in a page breadcrumb trail.
// If URL is empty the step renders as plain text (current page).
type BreadcrumbStep struct {
	Label string
	URL   string
}

// Crumbs builds a breadcrumb step slice for use with PageHeader.
// Alternate label/URL pairs: label first, then optional URL (must start with /).
// Example — single step:  nav.Crumbs("Cases")
// Example — two steps:    nav.Crumbs("Cases", "/app/cases", "Test 3")
func Crumbs(args ...string) []BreadcrumbStep {
	var steps []BreadcrumbStep
	for i := 0; i < len(args); i++ {
		step := BreadcrumbStep{Label: args[i]}
		if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] == '/' {
			step.URL = args[i+1]
			i++
		}
		steps = append(steps, step)
	}
	return steps
}
