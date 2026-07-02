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
	LoaderCentered LoaderVariant = "centered"
	LoaderInline   LoaderVariant = "inline"
	LoaderOverlay  LoaderVariant = "overlay"
)

// ternary returns a if cond is true, else b.
func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
