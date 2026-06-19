// Package devmode provides component boundary annotation for gallery dev tooling.
//
// In dev mode, [ComponentBoundary] wraps any [templ.Component] output in a
// display:contents <div> annotated with data-component and data-props attributes.
// This makes the component hierarchy visible in DevTools and enables the gallery's
// hover overlay, component tree panel, and annotated source view.
//
// For structural HTML elements that cannot legally contain a <div> wrapper
// (e.g. <thead>, <tbody>, <tr>, <td>, <th>), use [ElementBoundary] instead.
// It injects the data-component/data-props attributes directly onto the first
// opening tag emitted by the inner component rather than wrapping it.
//
// Usage in the gallery server:
//
//	ctx = devmode.WithDevMode(ctx)   // inject once per request
//	comp = devmode.ComponentBoundary("Button", ui.Button(props), props)     // with props
//	comp = devmode.ComponentBoundary("Button", ui.Button(props))            // without props
//	comp = devmode.ElementBoundary("TableRow", table.TableRow(id, hover), props)
//
// In production (when [IsDevMode] returns false), both functions are a
// zero-overhead passthrough — no wrapper element or extra attributes are emitted.
package devmode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/a-h/templ"
)

// contextKey is the unexported typed key used to store devmode state in context.
// Using a named type prevents collisions with any other package's context keys.
type contextKey struct{}

// emptyAttrs is returned by [Attrs] in production to avoid per-call allocations.
var emptyAttrs = templ.Attributes{}

// WithDevMode returns a new context with dev mode enabled.
// Pass this context (or a context derived from it) to [templ.Component.Render]
// to activate component boundary annotations.
func WithDevMode(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, true)
}

// IsDevMode reports whether dev mode is active in the given context.
func IsDevMode(ctx context.Context) bool {
	v, _ := ctx.Value(contextKey{}).(bool)
	return v
}

// Attrs returns a [templ.Attributes] map containing data-component="name" when
// dev mode is active in ctx. In production (when [IsDevMode] returns false) it
// returns a shared empty map — no allocation, no HTML output.
//
// Spread the result into any templ element to annotate it with its component
// name, making it easy to identify components in browser DevTools:
//
//	templ Button(...) {
//	    <button { devmode.Attrs(ctx, "ui/Button")... }>
//	        { children... }
//	    </button>
//	}
func Attrs(ctx context.Context, name string) templ.Attributes {
	if !IsDevMode(ctx) {
		return emptyAttrs
	}
	return templ.Attributes{"data-component": name}
}

// ComponentBoundary wraps inner with a display:contents <div> annotated with
// data-component (the component name) and optionally data-props (the props as JSON).
//
// The wrapper is only emitted when [IsDevMode] returns true for the render
// context — in all other cases the inner component is rendered unchanged.
//
// The optional props argument can be any JSON-serialisable value. When omitted,
// no data-props attribute is emitted. For best results pass a map[string]any or
// the component's props struct.
func ComponentBoundary(name string, inner templ.Component, props ...any) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !IsDevMode(ctx) {
			return inner.Render(ctx, w)
		}

		// Emit the opening wrapper. display:contents makes the div invisible to
		// layout engines (flexbox, grid) while preserving its presence in the DOM
		// so DevTools and JavaScript can query [data-component] freely.
		if len(props) > 0 {
			propsJSON, err := json.Marshal(props[0])
			if err != nil {
				propsJSON = []byte("null")
			}
			// Use HTML-escaped attribute values so JSON containing " characters is
			// preserved correctly when read back via getAttribute() in the browser.
			if _, err := fmt.Fprintf(w,
				`<div data-component="%s" data-props="%s" style="display:contents">`,
				html.EscapeString(name), html.EscapeString(string(propsJSON)),
			); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w,
				`<div data-component="%s" style="display:contents">`,
				html.EscapeString(name),
			); err != nil {
				return err
			}
		}

		if err := inner.Render(ctx, w); err != nil {
			return err
		}

		_, err := io.WriteString(w, `</div>`)
		return err
	})
}

// ElementBoundary annotates the first opening tag emitted by inner with
// data-component and optionally data-props attributes, without adding any wrapper element.
//
// Use this for structural HTML elements that cannot legally contain a <div>
// wrapper, such as <thead>, <tbody>, <tr>, <td>, and <th>. The browser's
// HTML parser will strip a <div> placed directly inside a <table> or <tr>,
// making [ComponentBoundary] ineffective for these elements.
//
// The optional props argument can be any JSON-serialisable value. When omitted,
// no data-props attribute is emitted.
//
// Like [ComponentBoundary], this is a zero-overhead passthrough in production
// (when [IsDevMode] returns false).
func ElementBoundary(name string, inner templ.Component, props ...any) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !IsDevMode(ctx) {
			return inner.Render(ctx, w)
		}

		// Use HTML-escaped attribute values so JSON containing " characters is
		// preserved correctly when read back via getAttribute() in the browser.
		var attrs string
		if len(props) > 0 {
			propsJSON, err := json.Marshal(props[0])
			if err != nil {
				propsJSON = []byte("null")
			}
			attrs = fmt.Sprintf(` data-component="%s" data-props="%s"`,
				html.EscapeString(name), html.EscapeString(string(propsJSON)))
		} else {
			attrs = fmt.Sprintf(` data-component="%s"`, html.EscapeString(name))
		}

		// Render the inner component into a buffer so we can inject attributes
		// into the first opening tag before forwarding to the real writer.
		var buf bytes.Buffer
		if err := inner.Render(ctx, &buf); err != nil {
			return err
		}

		// Find the first '>' and insert the attributes just before it,
		// after any existing attributes on the tag.
		raw := buf.String()
		idx := strings.Index(raw, ">")
		if idx < 0 {
			// Shouldn't happen for well-formed templ output; fall through.
			_, writeErr := io.WriteString(w, raw)
			return writeErr
		}
		// Handle self-closing tags: don't inject before '/>'
		insert := idx
		if insert > 0 && raw[insert-1] == '/' {
			insert--
		}
		annotated := raw[:insert] + attrs + raw[insert:]
		_, writeErr := io.WriteString(w, annotated)
		return writeErr
	})
}
