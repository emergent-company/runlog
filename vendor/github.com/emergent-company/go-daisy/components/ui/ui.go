// Package ui provides reusable primitive Templ components (button, badge, avatar, etc.).
package ui

import (
	"io/fs"
	"net/http"
)

// StaticHandlerFS returns an http.Handler that serves static assets from fsys,
// stripping the given URL prefix. Pass staticfs.FS() from the staticfs package.
func StaticHandlerFS(prefix string, fsys fs.FS) http.Handler {
	return http.StripPrefix(prefix, http.FileServer(http.FS(fsys)))
}

// LoaderVariant controls how a Loader spinner is presented.
type LoaderVariant string

const (
	// LoaderCentered renders a large spinner centred in its container (default).
	LoaderCentered LoaderVariant = "centered"
	// LoaderInline renders a small inline spinner (no wrapper div).
	LoaderInline LoaderVariant = "inline"
	// LoaderOverlay renders a full-area semi-transparent overlay with a spinner.
	LoaderOverlay LoaderVariant = "overlay"
)
