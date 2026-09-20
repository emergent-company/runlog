// Package stimulus provides Go helpers for generating Stimulus.js data
// attributes. Use as an alternative to the alpine package when you prefer
// Stimulus's explicit controller/action/target pattern.
//
// Compare:
//
//	Alpine:  <div x-data="{ open: false }"> <button @click="open = !open">
//	Stimulus: <div data-controller="modal"> <button data-action="click->modal#open">
//
// Stimulus is ~5KB gzipped vs Alpine's ~15KB. Choose based on team preference.
package stimulus

import (
	"context"
	"io"

	"github.com/a-h/templ"
)

// Tag returns the Stimulus script tag to include in the page.
func Tag() templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte(`<script src="/static/js/stimulus.js"></script>`))
		return err
	})
}

// Controller returns {"data-controller": name} for spreading into a Templ element.
func Controller(name string) templ.Attributes {
	return templ.Attributes{"data-controller": name}
}

// Action returns {"data-action": "event->controller#method"} for event binding.
// event: click, keydown.escape, submit, etc.
// controller: the controller name (without trailing #).
// method: the controller method name.
func Action(event, controller, method string) templ.Attributes {
	return templ.Attributes{"data-action": event + "->" + controller + "#" + method}
}

// Actions returns {"data-action": actions} for multiple event bindings
// separated by spaces.
func Actions(actions ...string) templ.Attributes {
	joined := ""
	for i, a := range actions {
		if i > 0 {
			joined += " "
		}
		joined += a
	}
	return templ.Attributes{"data-action": joined}
}

// Target returns {"data-<controller>-target": name} for element references.
func Target(controller, name string) templ.Attributes {
	return templ.Attributes{"data-" + controller + "-target": name}
}

// Targets returns {"data-<controller>-targets": name} for multiple element references.
func Targets(controller, name string) templ.Attributes {
	return templ.Attributes{"data-" + controller + "-targets": name}
}

// Param returns {"data-<controller>-<name>-param": value} for typed parameters.
func Param(controller, name, value string) templ.Attributes {
	return templ.Attributes{"data-" + controller + "-" + name + "-param": value}
}

// Value returns {"data-<controller>-<name>-value": value} for controller values.
func Value(controller, name, value string) templ.Attributes {
	return templ.Attributes{"data-" + controller + "-" + name + "-value": value}
}

// Outlet returns {"data-<controller>-<name>-outlet": selector} for cross-controller communication.
func Outlet(controller, name, selector string) templ.Attributes {
	return templ.Attributes{"data-" + controller + "-" + name + "-outlet": selector}
}
