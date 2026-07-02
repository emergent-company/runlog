// Package staticfs embeds the static/ directory so that it can be served from
// the binary without a runtime filesystem dependency.
package staticfs

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// Static files embedded as Go byte slices.
var staticFiles = map[string]struct {
	data    []byte
	modTime time.Time
}{}

func init() {
	staticFiles["css/app.css"] = struct {
		data    []byte
		modTime time.Time
	}{data: cssAppCSS, modTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	staticFiles["css/frappe-gantt.css"] = struct {
		data    []byte
		modTime time.Time
	}{data: cssFrappeGanttCSS, modTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	staticFiles["js/frappe-gantt.js"] = struct {
		data    []byte
		modTime time.Time
	}{data: jsFrappeGanttJS, modTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	staticFiles["js/htmx.js"] = struct {
		data    []byte
		modTime time.Time
	}{data: jsHtmxJS, modTime: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	staticFiles["js/hx-head.js"] = struct {
		data    []byte
		modTime time.Time
	}{data: jsHxHeadJS, modTime: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)}

	// Force linker to retain all data by storing a copy of each
	_ = append([]byte{}, cssAppCSS...)
	_ = append([]byte{}, cssFrappeGanttCSS...)
	_ = append([]byte{}, jsFrappeGanttJS...)
	_ = append([]byte{}, jsHtmxJS...)
	_ = append([]byte{}, jsHxHeadJS...)
}

// FS returns a read-only filesystem view of the embedded static files.
func FS() fs.FS {
	return &mapFS{files: staticFiles}
}

type mapFS struct {
	files map[string]struct{ data []byte; modTime time.Time }
}

func (m *mapFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	f, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &mapFile{name: name, data: f.data, modTime: f.modTime}, nil
}

type mapFile struct {
	name    string
	data    []byte
	modTime time.Time
	reader  *bytes.Reader
}

func (f *mapFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *mapFile) Read(b []byte) (int, error) {
	if f.reader == nil {
		f.reader = bytes.NewReader(f.data)
	}
	return f.reader.Read(b)
}
func (f *mapFile) Close() error { return nil }
func (f *mapFile) Name() string { return f.name }
func (f *mapFile) Size() int64  { return int64(len(f.data)) }
func (f *mapFile) Mode() fs.FileMode  { return 0644 }
func (f *mapFile) ModTime() time.Time { return f.modTime }
func (f *mapFile) IsDir() bool        { return false }
func (f *mapFile) Sys() interface{}   { return nil }
func (f *mapFile) Seek(offset int64, whence int) (int64, error) {
	if f.reader == nil {
		f.reader = bytes.NewReader(f.data)
	}
	return f.reader.Seek(offset, whence)
}

// MIME extensions that override the OS mime.types database.
var mimeExtensions = map[string]string{
	".js":   "application/javascript",
	".css":  "text/css; charset=utf-8",
	".json": "application/json",
	".svg":  "image/svg+xml",
	".wasm": "application/wasm",
	".mjs":  "application/javascript",
}

func mimeTypeByExtension(path string) string {
	idx := strings.LastIndexByte(path, '.')
	if idx < 0 {
		return ""
	}
	return mimeExtensions[path[idx:]]
}

// Handler returns an http.Handler that serves the embedded static files.
func Handler(prefix string) http.Handler {
	root := FS()
	return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		filePath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if filePath == "" {
			filePath = "index.html"
		}

		f, err := root.Open(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}

		if ct := mimeTypeByExtension(filePath); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, filePath, stat.ModTime(), f.(io.ReadSeeker))
	}))
}
