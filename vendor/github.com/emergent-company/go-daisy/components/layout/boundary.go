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

// AppShellWithNavWithBoundary wraps AppShellWithNav with a dev-mode component boundary annotation.
// gallery:token appName,groups
// gallery:hint appName:default(MyApp)
func AppShellWithNavWithBoundary(appName string, groups []SidebarGroup) templ.Component {
	return devmode.ComponentBoundary("AppShellWithNav", AppShellWithNav(appName, groups), map[string]any{
		"appName":    appName,
		"groupCount": len(groups),
	})
}

// AppShellContentWithBoundary wraps AppShellContent with a dev-mode component boundary annotation.
func AppShellContentWithBoundary() templ.Component {
	return devmode.ComponentBoundary("AppShellContent", AppShellContent())
}

// PageWithBoundary wraps Page with a dev-mode component boundary annotation.
// gallery:token title,themeAttr
// gallery:hint title:default(go-daisy)
func PageWithBoundary(title string, themeAttr string) templ.Component {
	return devmode.ComponentBoundary("Page", Page(title, themeAttr), map[string]any{
		"title":     title,
		"themeAttr": themeAttr,
	})
}

// ViewMenuWithBoundary wraps ViewMenu with a dev-mode component boundary annotation.
func ViewMenuWithBoundary() templ.Component {
	return devmode.ComponentBoundary("ViewMenu", ViewMenu())
}

// ViewMenuLabelWithBoundary wraps ViewMenuLabel with a dev-mode component boundary annotation.
// gallery:token label
// gallery:hint label:default(Navigation)
func ViewMenuLabelWithBoundary(label string) templ.Component {
	return devmode.ComponentBoundary("ViewMenuLabel", ViewMenuLabel(label), map[string]any{"label": label})
}
