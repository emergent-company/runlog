// Package layout provides the top-level shell Templ components (Page, AppShell, Sidebar, Navbar).
package layout

// PageProps configures the full HTML document shell.
type PageProps struct {
	Title           string
	ThemeAttr       string
	Alpine          bool // include Alpine.js <script> tag
	Morph           bool // include idiomorph + HTMX morph extension
	Stimulus        bool // include Stimulus.js + controllers
	Prefetch        bool // include link prefetch on hover
	ViewTransitions bool // enable View Transitions API (browser-native crossfade)
	ProgressBar     bool // include auto-show progress bar during slow HTMX requests
	SSE             bool // include htmx SSE extension
	WS              bool // include htmx WebSocket extension
}
