package nav

import (
	"context"
	"io"

	"github.com/a-h/templ"
	"github.com/emergent-company/go-daisy/devmode"
)

// PageHeaderWithBoundary wraps PageHeader with a dev-mode component boundary annotation.
// gallery:token steps
// gallery:hint steps:slice(3)
func PageHeaderWithBoundary(steps []BreadcrumbStep) templ.Component {
	return devmode.ComponentBoundary("PageHeader", PageHeader(steps, nil), map[string]any{"stepCount": len(steps)})
}

// TabMenuWithBoundary wraps TabMenu with a dev-mode component boundary annotation.
// gallery:token tabs
// gallery:hint tabs:slice(3)
func TabMenuWithBoundary(tabs []Tab, target ...string) templ.Component {
	return devmode.ComponentBoundary("TabMenu", TabMenu(tabs, nil, target...), map[string]any{"tabCount": len(tabs)})
}

// SimpleTabsWithBoundary wraps SimpleTabs with a dev-mode component boundary annotation.
// gallery:token tabs
// gallery:hint tabs:slice(3)
func SimpleTabsWithBoundary(tabs []Tab) templ.Component {
	return devmode.ComponentBoundary("SimpleTabs", SimpleTabs(tabs), map[string]any{"tabCount": len(tabs)})
}

// TopBarWithBoundary wraps TopBar with a dev-mode component boundary annotation.
// gallery:token title
// gallery:hint title:default(My Application)
func TopBarWithBoundary(title string) templ.Component {
	return devmode.ComponentBoundary("TopBar", TopBar(title, nil), map[string]any{"title": title})
}

// MenuWithBoundary wraps Menu with a dev-mode component boundary annotation.
// gallery:token size,items
// gallery:hint items:slice(4)
func MenuWithBoundary(size MenuSize, items []MenuItem) templ.Component {
	return devmode.ComponentBoundary("Menu", Menu(size, items), map[string]any{
		"size":      string(size),
		"itemCount": len(items),
	})
}

// BreadcrumbsWithBoundary wraps Breadcrumbs with a dev-mode component boundary annotation.
// gallery:token items
// gallery:hint items:slice(3)
func BreadcrumbsWithBoundary(items []BreadcrumbItem) templ.Component {
	return devmode.ComponentBoundary("Breadcrumbs", Breadcrumbs(items), map[string]any{"itemCount": len(items)})
}

// DockWithBoundary wraps Dock with a dev-mode component boundary annotation.
// gallery:token items
// gallery:hint items:slice(4)
func DockWithBoundary(items []DockItem) templ.Component {
	return devmode.ComponentBoundary("Dock", Dock(items), map[string]any{"itemCount": len(items)})
}

// LinkWithBoundary wraps Link with a dev-mode component boundary annotation.
// gallery:token variant
// gallery:hint variant:default(link)
func LinkWithBoundary(href string, variant LinkVariant, label string) templ.Component {
	child := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, label)
		return err
	})
	inner := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return Link(href, variant, "", nil).Render(templ.WithChildren(ctx, child), w)
	})
	return devmode.ComponentBoundary("Link", inner, map[string]any{
		"href":    href,
		"variant": string(variant),
		"label":   label,
	})
}

// PageTitleMinimalWithBoundary wraps PageTitleMinimal with a dev-mode component boundary annotation.
func PageTitleMinimalWithBoundary(title string, steps []PageTitleStep) templ.Component {
	return devmode.ComponentBoundary("PageTitleMinimal", PageTitleMinimal(title, steps), map[string]any{
		"title":     title,
		"stepCount": len(steps),
	})
}

// PageTitleEditorWithBoundary wraps PageTitleEditor with a dev-mode component boundary annotation.
func PageTitleEditorWithBoundary(steps []BreadcrumbStep, title, subtitle string, actions []PageTitleEditorAction) templ.Component {
	return devmode.ComponentBoundary("PageTitleEditor", PageTitleEditor(steps, title, subtitle, actions), map[string]any{
		"title":       title,
		"subtitle":    subtitle,
		"stepCount":   len(steps),
		"actionCount": len(actions),
	})
}

// FooterMinimalWithBoundary wraps FooterMinimal with a dev-mode component boundary annotation.
func FooterMinimalWithBoundary(copyright string, links []FooterLink) templ.Component {
	return devmode.ComponentBoundary("FooterMinimal", FooterMinimal(copyright, links), map[string]any{
		"copyright": copyright,
		"linkCount": len(links),
	})
}

// ProfileMenuWithBoundary wraps ProfileMenu with a dev-mode component boundary annotation.
func ProfileMenuWithBoundary(name, email, initials string, items []ProfileMenuItem, signOutHref string) templ.Component {
	return devmode.ComponentBoundary("ProfileMenu", ProfileMenu(name, email, initials, items, signOutHref), map[string]any{
		"name":        name,
		"email":       email,
		"initials":    initials,
		"itemCount":   len(items),
		"signOutHref": signOutHref,
	})
}

// FooterVariantWithBoundary wraps FooterVariant with a dev-mode boundary.
func FooterVariantWithBoundary(style string, opts FooterVariantOpts) templ.Component {
	return devmode.ComponentBoundary("FooterVariant", FooterVariant(style, opts), map[string]any{
		"style":   style,
		"version": 1,
	})
}

// NotificationVariantWithBoundary wraps NotificationVariant with a dev-mode boundary.
func NotificationVariantWithBoundary(style string, opts NotificationVariantOpts) templ.Component {
	return devmode.ComponentBoundary("NotificationVariant", NotificationVariant(style, opts), map[string]any{
		"style": style,
	})
}

// SearchModalWithBoundary wraps SearchModal with a dev-mode boundary.
func SearchModalWithBoundary(style string, opts SearchModalOpts) templ.Component {
	return devmode.ComponentBoundary("SearchModal", SearchModal(style, opts), map[string]any{
		"style": style,
	})
}

// ProfileMenuVariantWithBoundary wraps ProfileMenuVariant with a dev-mode boundary.
func ProfileMenuVariantWithBoundary(style string, opts ProfileMenuVariantOpts) templ.Component {
	return devmode.ComponentBoundary("ProfileMenuVariant", ProfileMenuVariant(style, opts), map[string]any{
		"style": style,
	})
}

// PageTitleVariantWithBoundary wraps PageTitleVariant with a dev-mode boundary.
func PageTitleVariantWithBoundary(style string, opts PageTitleVariantOpts) templ.Component {
	return devmode.ComponentBoundary("PageTitleVariant", PageTitleVariant(style, opts), map[string]any{
		"style": style,
	})
}

// ScrollTopbarWithBoundary wraps ScrollTopbar with a dev-mode component boundary annotation.
// gallery:token title
// gallery:hint title:default(Dashboard)
func ScrollTopbarWithBoundary(title string) templ.Component {
	return devmode.ComponentBoundary("ScrollTopbar", ScrollTopbar(title, nil), map[string]any{
		"title": title,
	})
}
