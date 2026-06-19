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
