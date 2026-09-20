package ui

import (
	"strings"

	"github.com/a-h/templ"
)

// iconNameToSpan converts a colon-format icon name (e.g. "lucide:search")
// to the Iconify class format ("lucide--search").
func iconNameToSpan(name string) string {
	return strings.ReplaceAll(name, ":", "--")
}

// IconProps configures an accessible Iconify icon.
type IconProps struct {
	Name  string
	Size  string
	Label string
	Class string
	Attrs templ.Attributes
}

func iconSizeClass(size string) string {
	switch size {
	case "xs":
		return "size-3"
	case "sm":
		return "size-4"
	case "md":
		return "size-5"
	case "lg":
		return "size-6"
	case "xl":
		return "size-8"
	default:
		if size != "" {
			return size
		}
		return "size-5"
	}
}
