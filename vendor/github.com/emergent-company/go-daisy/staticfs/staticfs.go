// Package staticfs embeds the static/ directory so that it can be served from
// the binary without a runtime filesystem dependency.
//
// When static files change, run:  go generate ./staticfs/
// This updates staticfs_hash.go so the Go compiler recompiles the package.
//go:generate sh -c "find static -type f -exec sha256sum {} + | sha256sum | cut -d' ' -f1 | xargs -I{} printf 'package staticfs\n\nconst staticFilesHash = \"%s\"\n' {} > staticfs_hash.go"
package staticfs

import (
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var files embed.FS

// UseUnminified controls whether Script() serves unminified variants of
// go-daisy-owned JS files. When true, the ".min" suffix is stripped from
// the path when an unminified variant exists in the embedded FS.
//
// Default false — always serves minified production files.
var UseUnminified bool

// Hash returns the current content hash of all static files.
// Use for cache-busting query params: /static/js/htmx.js?v=abc123
func Hash() string {
	return staticFilesHash[:12]
}

// Stylesheet returns a <link> tag for a CSS file with cache-busting hash.
// path is relative to static/, e.g. "/css/app.css"
func Stylesheet(p string) string {
	return fmt.Sprintf(`<link rel="stylesheet" href="/static%s?v=%s" data-asset-track="reload">`,
		html.EscapeString(p), staticFilesHash[:12])
}

// Script returns a <script> tag for a JS file with cache-busting hash.
// path is relative to static/, e.g. "/js/htmx.js"
//
// When UseUnminified is true and path contains ".min.js", the ".min" suffix
// is stripped to serve the unminified variant (e.g. "file.min.js" → "file.js").
// Falls back to the original path if the unminified variant doesn't exist.
func Script(p string) string {
	resolved := p
	if UseUnminified {
		unmin := strings.Replace(p, ".min.js", ".js", 1)
		if unmin != p {
			if _, err := fs.Stat(FS(), unmin[1:]); err == nil {
				resolved = unmin
			}
		}
	}
	return fmt.Sprintf(`<script src="/static%s?v=%s" data-asset-track="reload"></script>`,
		html.EscapeString(resolved), staticFilesHash[:12])
}

// AssetPath returns the full URL path for a static file with cache-busting hash.
// When UseUnminified is true, strips ".min" suffix same as Script().
func AssetPath(p string) string {
	resolved := p
	if UseUnminified {
		unmin := strings.Replace(p, ".min.js", ".js", 1)
		if unmin != p {
			if _, err := fs.Stat(FS(), unmin[1:]); err == nil {
				resolved = unmin
			}
		}
	}
	return fmt.Sprintf("/static%s?v=%s", resolved, staticFilesHash[:12])
}

// FS returns the embedded static file system, rooted at "static/".
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic("staticfs: " + err.Error())
	}
	return sub
}

// MIME extensions that override the OS mime.types database.
// Arch Linux and some containers map .js to text/plain, which breaks
// script execution in browsers (CORB/MIME type checking).
var mimeExtensions = map[string]string{
	".js":   "application/javascript",
	".css":  "text/css; charset=utf-8",
	".json": "application/json",
	".svg":  "image/svg+xml",
	".wasm": "application/wasm",
	".mjs":  "application/javascript",
}

func mimeTypeByExtension(p string) string {
	idx := strings.LastIndexByte(p, '.')
	if idx < 0 {
		return ""
	}
	return mimeExtensions[p[idx:]]
}

// Handler returns an http.Handler that serves the embedded static files,
// stripping the given prefix (e.g. "/static/").
// Content-Type is set explicitly for known extensions to override buggy
// OS-level MIME databases (e.g. Arch Linux maps .js → text/plain).
func Handler(prefix string) http.Handler {
	fileServer := http.FileServer(http.FS(FS()))
	return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := mimeTypeByExtension(path.Clean(r.URL.Path))
		if ct == "" {
			fileServer.ServeHTTP(w, r)
			return
		}
		lw := &mimeWriter{ResponseWriter: w, ct: ct}
		fileServer.ServeHTTP(lw, r)
	}))
}

// mimeWriter forces Content-Type before Go's http.FileServer can set it
// based on the OS mime.types (which may be wrong for .js on some systems).
type mimeWriter struct {
	http.ResponseWriter
	ct    string
	wrote bool
}

func (w *mimeWriter) WriteHeader(code int) {
	if !w.wrote {
		w.Header().Set("Content-Type", w.ct)
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *mimeWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
