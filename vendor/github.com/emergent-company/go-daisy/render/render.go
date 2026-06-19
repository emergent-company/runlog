// Package render provides HTMX-aware rendering helpers for Templ components.
package render

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"
)

// contextKey is used for request-scoped values in context.Context.
type contextKey string

const (
	// HistoryRestoreKey is set on the request context when rendering for an HTMX
	// history restore request (browser back/forward). Templates can check this to
	// skip the outer HTML shell and render only the body content, preserving CSS
	// <link> elements already in document.head from the initial page load.
	HistoryRestoreKey contextKey = "htmx_history_restore"
)

// IsHistoryRestore returns true when HTMX is restoring history (browser back/forward).
func IsHistoryRestore(r *http.Request) bool {
	return r.Header.Get("HX-History-Restore-Request") == "true"
}

// IsHistoryRestoreFromContext returns true when the context has the history
// restore flag set (by RenderAuto). Templates use this to conditionally skip
// the outer HTML shell.
func IsHistoryRestoreFromContext(ctx context.Context) bool {
	v := ctx.Value(HistoryRestoreKey)
	return v != nil && v.(bool)
}

// IsPartial returns true when the request was made by HTMX as an inline partial
// swap (not a full-page navigation). Uses the HX-Request-Type header from HTMX v4.
func IsPartial(r *http.Request) bool {
	if IsHistoryRestore(r) {
		return false
	}
	ht := r.Header.Get("HX-Request-Type")
	if ht == "partial" {
		return true
	}
	return r.Header.Get("HX-Request") == "true" && ht != "full"
}

// IsHTMX returns true for any HTMX request (partial or full navigation).
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// HXTarget returns the value of the HX-Target header, or empty string.
func HXTarget(r *http.Request) string {
	return r.Header.Get("HX-Target")
}

// IsMainContentTarget returns true when the HTMX request targets the main
// content area (sidebar navigation). HTMX v4 sends HX-Target as
// "<tagname>#<id>" so we check for the id substring.
func IsMainContentTarget(r *http.Request) bool {
	return strings.Contains(r.Header.Get("HX-Target"), "main-content")
}

// IsScrollLoad returns true when the request is an HTMX scroll-load fetch
// (triggered by a ListArea sentinel element via scroll=1 query param).
func IsScrollLoad(r *http.Request) bool {
	return r.URL.Query().Get("scroll") == "1"
}

// RenderPage renders a full HTML document.
func RenderPage(w http.ResponseWriter, r *http.Request, content templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := content.Render(r.Context(), w); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// RenderPartial renders only a content fragment. Adds Vary headers so caches
// do not serve fragments in response to full-page requests.
func RenderPartial(w http.ResponseWriter, r *http.Request, content templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Add("Vary", "HX-Request-Type, HX-Request")
	if err := content.Render(r.Context(), w); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// RenderAuto renders the full page or a partial depending on HTMX state.
// For history restore requests (browser back/forward), it sets HistoryRestoreKey
// in the request context and renders the page template as a partial, allowing
// templates to skip the outer HTML shell and preserve CSS in the original <head>.
func RenderAuto(w http.ResponseWriter, r *http.Request, page, partial templ.Component) {
	if IsHistoryRestore(r) {
		ctx := context.WithValue(r.Context(), HistoryRestoreKey, true)
		r = r.WithContext(ctx)
		RenderPartial(w, r, page)
		return
	}
	if IsPartial(r) {
		RenderPartial(w, r, partial)
	} else {
		RenderPage(w, r, page)
	}
}

// RenderTriple handles three tiers of rendering for pages with sidebar nav and
// in-page tab/filter levels:
//
//   - Direct browser load (no HTMX)                  → page         (full HTML shell)
//   - Sidebar nav (HX-Target contains "main-content") → pageContent  (header + content, no shell)
//   - Tab/filter swap (any other HTMX partial)        → partial      (just the tab/table area)
func RenderTriple(w http.ResponseWriter, r *http.Request, page, pageContent, partial templ.Component) {
	if !IsPartial(r) {
		RenderPage(w, r, page)
		return
	}
	if IsMainContentTarget(r) {
		RenderPartial(w, r, pageContent)
		return
	}
	RenderPartial(w, r, partial)
}

// RedirectAfterMutation issues an HX-Redirect for HTMX requests or a 303 for plain requests.
func RedirectAfterMutation(w http.ResponseWriter, r *http.Request, path string) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

// AppendToast writes an hx-swap-oob fragment that appends a toast notification
// to #toast-container. toastType should be one of: success, error, warning, info.
func AppendToast(w http.ResponseWriter, toastType, message string) {
	alertClass := "alert-info"
	switch toastType {
	case "success":
		alertClass = "alert-success"
	case "error":
		alertClass = "alert-error"
	case "warning":
		alertClass = "alert-warning"
	}
	id := fmt.Sprintf("toast-%s", toastType)
	fmt.Fprintf(w, `<div id="%s" hx-swap-oob="beforeend:#toast-container">`+
		`<div class="alert %s shadow-lg w-80">`+
		`<span>%s</span>`+
		`<script>setTimeout(()=>{const e=document.getElementById('%s');if(e)e.remove()},4000)</script>`+
		`</div></div>`,
		id, alertClass, message, id)
}
