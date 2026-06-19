// Package staticfs embeds the static/ directory so that it can be served from
// the binary without a runtime filesystem dependency.
package staticfs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:static
var files embed.FS

// FS returns the embedded static file system, rooted at "static/".
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic("staticfs: " + err.Error())
	}
	return sub
}

// Handler returns an http.Handler that serves the embedded static files,
// stripping the given prefix (e.g. "/static/").
func Handler(prefix string) http.Handler {
	return http.StripPrefix(prefix, http.FileServer(http.FS(FS())))
}
