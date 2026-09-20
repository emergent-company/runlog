package render

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// CSRFKey is the context key for the CSRF token.
const CSRFKey = "csrf_token"

// CSRFMiddleware generates and validates CSRF tokens.
//
// Usage with Echo:
//
//	e.Use(render.CSRFMiddleware(render.CSRFConfig{
//	    CookieName: "__csrf",
//	    HeaderName: "X-CSRF-TOKEN",
//	    SkipMethods: []string{"GET", "HEAD", "OPTIONS"},
//	}))
//
// In your page template, render the CSRF meta tag:
//
//	@ui.CSRFMeta(csrf.FromContext(c))
//
// HTMX will auto-inject X-CSRF-TOKEN into all non-GET requests when the
// CSRF meta tag is present (requires the bundled CSRF JS in staticfs).
type CSRFConfig struct {
	CookieName  string
	HeaderName  string
	SkipMethods []string
}

// Default CSRF config.
func defaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookieName:  "__csrf",
		HeaderName:  "X-CSRF-TOKEN",
		SkipMethods: []string{"GET", "HEAD", "OPTIONS"},
	}
}

// CSRFMiddleware returns Echo middleware for CSRF protection.
func CSRFMiddleware(cfg ...CSRFConfig) echo.MiddlewareFunc {
	c := defaultCSRFConfig()
	if len(cfg) > 0 {
		if cfg[0].CookieName != "" {
			c.CookieName = cfg[0].CookieName
		}
		if cfg[0].HeaderName != "" {
			c.HeaderName = cfg[0].HeaderName
		}
		if len(cfg[0].SkipMethods) > 0 {
			c.SkipMethods = cfg[0].SkipMethods
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			token := getOrCreateToken(ctx, c.CookieName)

			if !skipMethod(ctx.Request().Method, c.SkipMethods) {
				submitted := extractSubmittedToken(ctx.Request(), c.HeaderName)
				if submitted == "" || submitted != token {
					return ctx.String(http.StatusForbidden, "invalid CSRF token")
				}
			}

			ctx.Set(CSRFKey, token)
			return next(ctx)
		}
	}
}

func getOrCreateToken(ctx echo.Context, cookieName string) string {
	cookie, err := ctx.Cookie(cookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := generateToken()
	ctx.SetCookie(&http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func skipMethod(method string, skipMethods []string) bool {
	for _, m := range skipMethods {
		if strings.EqualFold(method, m) {
			return true
		}
	}
	return false
}

func extractSubmittedToken(r *http.Request, headerName string) string {
	if t := r.Header.Get(headerName); t != "" {
		return t
	}
	return r.FormValue("csrf_token")
}

// FromContext extracts the CSRF token from Echo context.
// Use with ui.CSRFMeta(csrf.FromContext(c)).
func CSRFFromContext(ctx echo.Context) string {
	t, _ := ctx.Get(CSRFKey).(string)
	return t
}
