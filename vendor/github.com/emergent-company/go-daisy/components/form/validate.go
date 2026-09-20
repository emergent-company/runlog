package form

import (
	"fmt"
	"net/http"
	"strings"
)

// Errors maps field names to validation error messages.
type Errors map[string]string

// HasErrors returns true when there are validation errors.
func (e Errors) HasErrors() bool {
	return len(e) > 0
}

// DecodeAndValidate parses form values and runs validation functions.
// Returns the raw Form values and any validation errors.
//
// Usage:
//
//	r.ParseForm()
//	values, errs := form.DecodeAndValidate(r)
//	if errs.HasErrors() {
//	    return render.RenderPartial(w, r, MyForm(values, errs))
//	}
func DecodeAndValidate(r *http.Request) (Values, Errors) {
	if r.Form == nil {
		r.ParseForm()
	}
	values := make(Values)
	errs := make(Errors)
	for key, vv := range r.Form {
		if len(vv) > 0 {
			values[key] = strings.TrimSpace(vv[0])
		}
	}
	return values, errs
}

// Values is a map of form field names to values.
type Values map[string]string

// Get returns the value for a key, or defaultVal if absent.
func (v Values) Get(key, defaultVal string) string {
	if val, ok := v[key]; ok && val != "" {
		return val
	}
	return defaultVal
}

// Require validates that required fields are non-empty. Mutates the errors map.
func Require(v Values, errs Errors, keys ...string) {
	for _, key := range keys {
		if strings.TrimSpace(v[key]) == "" {
			errs[key] = fmt.Sprintf("%s is required", key)
		}
	}
}

// RequireMin validates that a field has at least min length. Mutates errors.
func RequireMin(v Values, errs Errors, key string, min int) {
	if len(strings.TrimSpace(v[key])) < min {
		if _, exists := errs[key]; !exists {
			errs[key] = fmt.Sprintf("%s must be at least %d characters", key, min)
		}
	}
}

// RequireEmail validates a field looks like an email. Mutates errors.
func RequireEmail(v Values, errs Errors, key string) {
	val := strings.TrimSpace(v[key])
	if val == "" {
		return
	}
	if !strings.Contains(val, "@") || !strings.Contains(val, ".") {
		errs[key] = fmt.Sprintf("%s must be a valid email", key)
	}
}

// GetStrings extracts multiple values for a field name (e.g. "tags[0]", "tags[1]").
func GetStrings(r *http.Request, base string) []string {
	if r.Form == nil {
		r.ParseForm()
	}
	var result []string
	prefix := base + "["
	for key, vv := range r.Form {
		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, "]") {
			for _, v := range vv {
				if v := strings.TrimSpace(v); v != "" {
					result = append(result, v)
				}
			}
		}
	}
	return result
}

// GetNestedStrings extracts nested form values like "name[0][key]".
// Returns a slice of maps keyed by the inner key.
func GetNestedStrings(r *http.Request, base string) []map[string]string {
	if r.Form == nil {
		r.ParseForm()
	}
	rows := make(map[string]map[string]string)
	prefix := base + "["
	for key, vv := range r.Form {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := key[len(prefix):]
		closeBracket := strings.Index(rest, "]")
		if closeBracket < 0 {
			continue
		}
		idx := rest[:closeBracket]
		innerKey := ""
		if len(rest) > closeBracket+3 && rest[closeBracket+1] == '[' {
			innerEnd := strings.Index(rest[closeBracket+2:], "]")
			if innerEnd >= 0 {
				innerKey = rest[closeBracket+2 : closeBracket+2+innerEnd]
			}
		}
		if _, ok := rows[idx]; !ok {
			rows[idx] = make(map[string]string)
		}
		if len(vv) > 0 && innerKey != "" {
			rows[idx][innerKey] = strings.TrimSpace(vv[0])
		} else if len(vv) > 0 {
			rows[idx]["value"] = strings.TrimSpace(vv[0])
		}
	}
	result := make([]map[string]string, 0, len(rows))
	for i := 0; ; i++ {
		idx := fmt.Sprintf("%d", i)
		if row, ok := rows[idx]; ok {
			result = append(result, row)
		} else {
			break
		}
	}
	return result
}
