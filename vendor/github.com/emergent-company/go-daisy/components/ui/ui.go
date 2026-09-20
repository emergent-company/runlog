package ui

import (
	"io/fs"
	"net/http"

	"github.com/emergent-company/go-daisy/shared"
)

// StaticHandlerFS returns an http.Handler that serves static assets from fsys,
// stripping the given URL prefix. Pass staticfs.FS() from the staticfs package.
func StaticHandlerFS(prefix string, fsys fs.FS) http.Handler {
	return http.StripPrefix(prefix, http.FileServer(http.FS(fsys)))
}

// AlertType maps to a DaisyUI alert modifier class.
type AlertType string

const (
	AlertSuccess AlertType = "alert-success"
	AlertError   AlertType = "alert-error"
	AlertWarning AlertType = "alert-warning"
	AlertInfo    AlertType = "alert-info"
)

// AlertStyle controls the visual style variant of an alert.
type AlertStyle string

const (
	AlertStyleDefault AlertStyle = ""             // solid
	AlertStyleSoft    AlertStyle = "alert-soft"
	AlertStyleOutline AlertStyle = "alert-outline"
	AlertStyleDash    AlertStyle = "alert-dash"
)

// ToastType is an alias for AlertType. Alerts and toasts share the same
// DaisyUI modifier classes (alert-success, alert-error, etc.).
type ToastType = AlertType

const (
	ToastSuccess = AlertSuccess
	ToastError   = AlertError
	ToastWarning = AlertWarning
	ToastInfo    = AlertInfo
)

// LoaderVariant controls how a Loader spinner is presented.
type LoaderVariant string

const (
	LoaderCentered LoaderVariant = "centered"
	LoaderInline   LoaderVariant = "inline"
	LoaderOverlay  LoaderVariant = "overlay"
)

func ternary(cond bool, a, b string) string     { return shared.Ternary(cond, a, b) }
func twmerge(classes ...string) string       { return shared.TwMerge(classes...) }
