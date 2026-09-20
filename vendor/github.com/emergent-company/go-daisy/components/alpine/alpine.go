// Package alpine provides Go helpers for generating Alpine.js attributes and
// pre-built data state objects for common interaction patterns. Use these in
// Templ components instead of hand-writing x-data, x-show, x-transition, etc.
//
// Alpine.js is optional — components that don't use alpine helpers render
// standard DaisyUI CSS-interactive elements. Add alpine attrs only when
// smooth transitions, focus traps, or keyboard navigation are needed.
package alpine

import (
	"context"
	"encoding/json"
	"io"

	"github.com/a-h/templ"
)

// State is a JSON-serializable value used as x-data.
type State any

// Tag returns the Alpine.js script tag to include in the page <head> or <body>.
func Tag() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte(`<script defer src="/static/js/alpine.js"></script>`))
		return err
	})
}

// TagDefer returns the Alpine.js script tag that defers loading until
// after the initial page paint.
func TagDefer() templ.Component {
	return Tag()
}

// MorphTag returns the idiomorph + HTMX morph extension script tag.
// Include this when using morph swaps via render.MorphSwap().
func MorphTag() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte(`<script src="/static/js/morph.js"></script>`))
		return err
	})
}

// XData returns {"x-data": json} for spreading into a Templ element.
// Use with a pre-built State from this package or a custom JSON-marshalable value.
func XData(s State) templ.Attributes {
	data, err := json.Marshal(s)
	if err != nil {
		return templ.Attributes{}
	}
	return templ.Attributes{"x-data": string(data)}
}

// Show returns {"x-show": expr} for conditional visibility.
func Show(expr string) templ.Attributes {
	return templ.Attributes{"x-show": expr}
}

// Transition returns {"x-transition": ""} for smooth enter/leave animations.
func Transition() templ.Attributes {
	return templ.Attributes{"x-transition": ""}
}

// TransitionDuration returns {"x-transition.duration.200ms": ""} with a custom duration.
func TransitionDuration(duration string) templ.Attributes {
	return templ.Attributes{"x-transition.duration." + duration: ""}
}

// Collapse returns {"x-collapse": ""} for smooth height animation on show/hide.
// Requires the Alpine Collapse plugin.
func Collapse() templ.Attributes {
	return templ.Attributes{"x-collapse": ""}
}

// On returns {"@event": handler} for event handling.
func On(event, handler string) templ.Attributes {
	return templ.Attributes{"@" + event: handler}
}

// Escape returns {"@keydown.escape": handler} for Escape key handling.
func Escape(handler string) templ.Attributes {
	return On("keydown.escape", handler)
}

// Outside returns {"@click.outside": handler} for outside-click handling.
func Outside(handler string) templ.Attributes {
	return On("click.outside", handler)
}

// Bind returns {"x-bind:attr": expr} for attribute binding.
func Bind(attr, expr string) templ.Attributes {
	return templ.Attributes{"x-bind:" + attr: expr}
}

// Model returns {"x-model": field} for two-way input binding.
func Model(field string) templ.Attributes {
	return templ.Attributes{"x-model": field}
}

// Trap returns {"x-trap": expr} for focus trapping (requires Alpine Focus plugin).
func Trap(expr string) templ.Attributes {
	return templ.Attributes{"x-trap": expr}
}

// Ref returns {"x-ref": name} for element references.
func Ref(name string) templ.Attributes {
	return templ.Attributes{"x-ref": name}
}

// Cloak returns {"x-cloak": ""} to hide element until Alpine initializes.
func Cloak() templ.Attributes {
	return templ.Attributes{"x-cloak": ""}
}

// Init returns {"x-init": expr} for initialization expressions.
func Init(expr string) templ.Attributes {
	return templ.Attributes{"x-init": expr}
}

// Text returns {"x-text": expr} for setting text content.
func Text(expr string) templ.Attributes {
	return templ.Attributes{"x-text": expr}
}

// HTML returns {"x-html": expr} for setting innerHTML.
func HTML(expr string) templ.Attributes {
	return templ.Attributes{"x-html": expr}
}

// If returns {"x-if": expr} for conditional rendering (element removed from DOM).
func If(expr string) templ.Attributes {
	return templ.Attributes{"x-if": expr}
}

// For returns {"x-for": expr} for looping.
func For(expr string) templ.Attributes {
	return templ.Attributes{"x-for": expr}
}

// ClassBinding returns {":class": expr} for dynamic CSS class binding.
func ClassBinding(expr string) templ.Attributes {
	return templ.Attributes{":class": expr}
}

// BooleanBinding returns {":attr": expr} for boolean attribute binding.
func BooleanBinding(attr, expr string) templ.Attributes {
	return templ.Attributes{":" + attr: expr}
}

// Effect returns {"x-effect": expr} for reactive effects.
func Effect(expr string) templ.Attributes {
	return templ.Attributes{"x-effect": expr}
}

// Merge merges multiple templ.Attributes maps together.
// Later entries override earlier ones for matching keys.
func Merge(attrs ...templ.Attributes) templ.Attributes {
	result := templ.Attributes{}
	for _, a := range attrs {
		for k, v := range a {
			result[k] = v
		}
	}
	return result
}
