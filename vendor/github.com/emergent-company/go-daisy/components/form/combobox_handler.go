package form

import (
	"net/http"
	"net/url"
	"strings"
)

// OptionsRequest holds extracted combobox server-mode request params.
type OptionsRequest struct {
	Search string
	Deps   map[string]string
}

// ParseOptionsRequest extracts search and dependency params from a combobox
// options endpoint request. Reads "search" and any DependsOn field names.
func ParseOptionsRequest(r *http.Request, dependsOn ...string) OptionsRequest {
	q := r.URL.Query()
	req := OptionsRequest{
		Search: strings.TrimSpace(q.Get("search")),
		Deps:   make(map[string]string),
	}
	for _, dep := range dependsOn {
		if v := q.Get(dep); v != "" {
			req.Deps[dep] = v
		}
	}
	return req
}

// ToggleRequest holds extracted combobox toggle request params.
type ToggleRequest struct {
	Value string
	Deps  map[string]string
}

// ParseToggleRequest extracts the value being toggled and dependencies.
func ParseToggleRequest(r *http.Request, dependsOn ...string) ToggleRequest {
	r.ParseForm()
	req := ToggleRequest{
		Value: r.FormValue("value"),
		Deps:  make(map[string]string),
	}
	for _, dep := range dependsOn {
		if v := r.FormValue(dep); v != "" {
			req.Deps[dep] = v
		}
	}
	return req
}

// CloneParams copies url.Values without mutating the original.
func CloneParams(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for k, vv := range src {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		dst[k] = vv2
	}
	return dst
}
