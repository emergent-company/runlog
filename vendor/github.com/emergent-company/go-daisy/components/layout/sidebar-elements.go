package layout

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

type SidebarProjectItem struct {
	Name string
	Icon string
	Href string
}

type SidebarBottomNavItem struct {
	Label  string
	Icon   string
	Href   string
	Active bool
}

func firstItem(items []SidebarProjectItem) SidebarProjectItem {
	if len(items) > 0 {
		return items[0]
	}
	return SidebarProjectItem{Name: "Select project"}
}

// children combines multiple templ.Component into one that renders them sequentially.
func children(components ...templ.Component) templ.Component {
	if len(components) == 1 {
		return components[0]
	}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, c := range components {
			if err := c.Render(ctx, w); err != nil {
				return err
			}
		}
		return nil
	})
}

func checkedAttr(open bool) templ.SafeCSS {
	if open {
		return templ.SafeCSS("checked")
	}
	return templ.SafeCSS("")
}

func initialsFrom(name string) string {
	if len(name) == 0 {
		return "?"
	}
	runes := []rune(name)
	if len(runes) >= 2 {
		return string(runes[:2])
	}
	return string(runes[0])
}
