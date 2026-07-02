package layout

import (
	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/devmode"
)

// AppShellWithBoundary wraps AppShell with a dev-mode component boundary annotation.
func AppShellWithBoundary(appName string) templ.Component {
	return devmode.ComponentBoundary("AppShell", AppShell(appName), map[string]any{"appName": appName})
}

// SidebarWithBoundary wraps Sidebar with a dev-mode component boundary annotation.
func SidebarWithBoundary(appName string, groups []SidebarGroup) templ.Component {
	return devmode.ComponentBoundary("Sidebar", Sidebar(appName, groups), map[string]any{
		"appName":    appName,
		"groupCount": len(groups),
	})
}

// NavbarWithBoundary wraps Navbar with a dev-mode component boundary annotation.
// gallery:token appName
// gallery:hint appName:default(MyApp)
func NavbarWithBoundary(appName string) templ.Component {
	return devmode.ComponentBoundary("Navbar", Navbar(appName), map[string]any{"appName": appName})
}

// SidebarVariantWithBoundary wraps SidebarVariant with a dev-mode boundary.
func SidebarVariantWithBoundary(variant string, opts SidebarVariantOpts) templ.Component {
	return devmode.ComponentBoundary("SidebarVariant", SidebarVariant(variant, opts), map[string]any{
		"variant": variant,
		"appName": opts.AppName,
	})
}

// TopbarVariantWithBoundary wraps TopbarVariant with a dev-mode boundary.
func TopbarVariantWithBoundary(style string, opts TopbarVariantOpts) templ.Component {
	return devmode.ComponentBoundary("TopbarVariant", TopbarVariant(style, opts), map[string]any{
		"style":   style,
		"appName": opts.AppName,
	})
}

// LayoutBuilderWithBoundary wraps LayoutBuilder with a dev-mode component boundary annotation.
func LayoutBuilderWithBoundary() templ.Component {
	return devmode.ComponentBoundary("LayoutBuilder", LayoutBuilder(), map[string]any{})
}

// SidebarDenseWithBoundary wraps SidebarDense with a dev-mode component boundary annotation.
func SidebarDenseWithBoundary(props SidebarDenseProps) templ.Component {
	return devmode.ComponentBoundary("SidebarDense", SidebarDense(props), map[string]any{
		"appName": props.AppName,
	})
}
