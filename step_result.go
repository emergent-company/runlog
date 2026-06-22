// Package e2eframework — step_result.go
//
// CLIResult and HTTPResult: chainable assertion types returned by Step actions.
// Each assertion logs its check to RunLog and calls rl.Failf on failure.
package runlog

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// CLIResult
// ─────────────────────────────────────────────────────────────────────────────

// CLIResult holds the output of a CLI invocation and provides chainable
// assertion methods.  Each assertion logs to RunLog; failures call rl.Failf.
type CLIResult struct {
	rl       *RunLog
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// newCLIResult constructs a CLIResult from a command's combined output and error.
// If combinedOutput is true, both stdout and stderr are in the stdout field
// (matching the current MustRunBinaryInDirWithHome which uses CombinedOutput).
func newCLIResult(rl *RunLog, stdout, stderr string, exitCode int, err error) *CLIResult {  //nolint:deadcode
	return &CLIResult{
		rl:       rl,
		stdout:   stdout,
		stderr:   stderr,
		exitCode: exitCode,
		err:      err,
	}
}

// newCLIResultFromCombined constructs a CLIResult from combined output (stdout+stderr
// merged) and the command error.
func newCLIResultFromCombined(rl *RunLog, combined string, err error) *CLIResult {  //nolint:deadcode
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return &CLIResult{
		rl:       rl,
		stdout:   combined,
		stderr:   "", // combined mode — stderr is interleaved in stdout
		exitCode: code,
		err:      err,
	}
}

// Contains asserts that stdout contains ALL of the given substrings.
// The first missing substring triggers rl.Failf.
func (r *CLIResult) Contains(substrs ...string) *CLIResult {  //nolint:deadcode
	for _, sub := range substrs {
		if strings.Contains(r.stdout, sub) {
			r.rl.Printf("assert: output contains %q ✓", sub)
		} else {
			r.rl.Failf("assert: output does not contain %q\noutput:\n%s", sub, Truncate(r.stdout, 500))
		}
	}
	return r
}

// ContainsAny asserts that stdout contains at least one of the given substrings.
func (r *CLIResult) ContainsAny(substrs ...string) *CLIResult {  //nolint:deadcode
	for _, sub := range substrs {
		if strings.Contains(r.stdout, sub) {
			r.rl.Printf("assert: output contains one of %v ✓ (matched %q)", substrs, sub)
			return r
		}
	}
	r.rl.Failf("assert: output does not contain any of %v\noutput:\n%s", substrs, Truncate(r.stdout, 500))
	return r
}

// NotContains asserts that stdout does NOT contain any of the given substrings.
func (r *CLIResult) NotContains(substrs ...string) *CLIResult {  //nolint:deadcode
	for _, sub := range substrs {
		if strings.Contains(r.stdout, sub) {
			r.rl.Failf("assert: output should not contain %q but does\noutput:\n%s", sub, Truncate(r.stdout, 500))
		}
	}
	r.rl.Printf("assert: output does not contain %v ✓", substrs)
	return r
}

// Matches asserts that stdout matches the given regexp pattern.
func (r *CLIResult) Matches(pattern string) *CLIResult {  //nolint:deadcode
	re, err := regexp.Compile(pattern)
	if err != nil {
		r.rl.Failf("assert: invalid regex %q: %v", pattern, err)
		return r
	}
	if re.MatchString(r.stdout) {
		r.rl.Printf("assert: output matches /%s/ ✓", pattern)
	} else {
		r.rl.Failf("assert: output does not match /%s/\noutput:\n%s", pattern, Truncate(r.stdout, 500))
	}
	return r
}

// Empty asserts that stdout is empty or whitespace-only.
func (r *CLIResult) Empty() *CLIResult {  //nolint:deadcode
	if strings.TrimSpace(r.stdout) == "" {
		r.rl.Printf("assert: output is empty ✓")
	} else {
		r.rl.Failf("assert: expected empty output, got:\n%s", Truncate(r.stdout, 500))
	}
	return r
}

// ParseID extracts a UUID (36-char, 4 hyphens) from stdout and stores it in *dst.
// If no UUID is found, rl.Failf is called.
func (r *CLIResult) ParseID(dst *string) *CLIResult {  //nolint:deadcode
	// UUID regex: 8-4-4-4-12 hex digits.
	re := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	match := re.FindString(r.stdout)
	if match == "" {
		r.rl.Failf("assert: no UUID found in output\noutput:\n%s", Truncate(r.stdout, 500))
		return r
	}
	*dst = match
	r.rl.Printf("assert: parsed ID %s ✓", match)
	return r
}

// JSONField parses stdout as JSON and extracts the named top-level field into *dst.
func (r *CLIResult) JSONField(field string, dst *string) *CLIResult {  //nolint:deadcode
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		r.rl.Failf("assert: cannot parse output as JSON: %v\noutput:\n%s", err, Truncate(r.stdout, 500))
		return r
	}
	raw, ok := m[field]
	if !ok {
		r.rl.Failf("assert: JSON field %q not found in output\noutput:\n%s", field, Truncate(r.stdout, 500))
		return r
	}
	// Try to unquote a string value; otherwise use the raw JSON.
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		*dst = string(raw)
	} else {
		*dst = s
	}
	r.rl.Printf("assert: JSON field %q = %q ✓", field, Truncate(*dst, 100))
	return r
}

// JSON unmarshals the full stdout into dst.
func (r *CLIResult) JSON(dst any) *CLIResult {  //nolint:deadcode
	if err := json.Unmarshal([]byte(r.stdout), dst); err != nil {
		r.rl.Failf("assert: cannot unmarshal output as JSON: %v\noutput:\n%s", err, Truncate(r.stdout, 500))
	} else {
		r.rl.Printf("assert: JSON unmarshal ✓")
	}
	return r
}

// ExitCode asserts that the command exited with the expected code.
func (r *CLIResult) ExitCode(expected int) *CLIResult {  //nolint:deadcode
	if r.exitCode == expected {
		r.rl.Printf("assert: exit code %d ✓", expected)
	} else {
		r.rl.Failf("assert: expected exit code %d, got %d\noutput:\n%s", expected, r.exitCode, Truncate(r.stdout, 500))
	}
	return r
}

// Output returns the raw stdout string.
func (r *CLIResult) Output() string {  //nolint:deadcode
	return r.stdout
}

// StderrOutput returns the raw stderr string.  When combined output mode was
// used (the default for CLI steps), stderr is interleaved in Output() and
// this returns an empty string.
func (r *CLIResult) StderrOutput() string {  //nolint:deadcode
	return r.stderr
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTPResult
// ─────────────────────────────────────────────────────────────────────────────

// HTTPResult holds the outcome of an HTTP request and provides chainable
// assertion methods.  Each assertion logs to RunLog; failures call rl.Failf.
type HTTPResult struct {
	rl         *RunLog
	statusCode int
	body       string
	headers    map[string][]string
}

// newHTTPResult constructs an HTTPResult.
func newHTTPResult(rl *RunLog, statusCode int, body string, headers map[string][]string) *HTTPResult {  //nolint:deadcode
	return &HTTPResult{
		rl:         rl,
		statusCode: statusCode,
		body:       body,
		headers:    headers,
	}
}

// Status asserts the response status code matches expected.
func (r *HTTPResult) Status(expected int) *HTTPResult {  //nolint:deadcode
	if r.statusCode == expected {
		r.rl.Printf("assert: HTTP status %d ✓", expected)
	} else {
		r.rl.Failf("assert: expected HTTP status %d, got %d\nbody:\n%s", expected, r.statusCode, Truncate(r.body, 500))
	}
	return r
}

// JSONField parses the response body as JSON and extracts a top-level field into *dst.
func (r *HTTPResult) JSONField(field string, dst *string) *HTTPResult {  //nolint:deadcode
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.body), &m); err != nil {
		r.rl.Failf("assert: cannot parse response body as JSON: %v\nbody:\n%s", err, Truncate(r.body, 500))
		return r
	}
	raw, ok := m[field]
	if !ok {
		r.rl.Failf("assert: JSON field %q not found in response\nbody:\n%s", field, Truncate(r.body, 500))
		return r
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		*dst = string(raw)
	} else {
		*dst = s
	}
	r.rl.Printf("assert: HTTP JSON field %q = %q ✓", field, Truncate(*dst, 100))
	return r
}

// JSONContains asserts that a top-level JSON field in the response body equals expected.
func (r *HTTPResult) JSONContains(field, expected string) *HTTPResult {  //nolint:deadcode
	var got string
	r.JSONField(field, &got)
	// JSONField already called Failf if parsing failed; if we get here, check value.
	if got != expected {
		r.rl.Failf("assert: HTTP JSON field %q = %q, expected %q", field, got, expected)
	} else {
		r.rl.Printf("assert: HTTP JSON field %q == %q ✓", field, expected)
	}
	return r
}

// BodyContains asserts that the response body contains the given substring.
func (r *HTTPResult) BodyContains(substr string) *HTTPResult {  //nolint:deadcode
	if strings.Contains(r.body, substr) {
		r.rl.Printf("assert: HTTP body contains %q ✓", substr)
	} else {
		r.rl.Failf("assert: HTTP body does not contain %q\nbody:\n%s", substr, Truncate(r.body, 500))
	}
	return r
}

// JSON unmarshals the full response body into dst.
func (r *HTTPResult) JSON(dst any) *HTTPResult {  //nolint:deadcode
	if err := json.Unmarshal([]byte(r.body), dst); err != nil {
		r.rl.Failf("assert: cannot unmarshal response body as JSON: %v\nbody:\n%s", err, Truncate(r.body, 500))
	} else {
		r.rl.Printf("assert: HTTP JSON unmarshal ✓")
	}
	return r
}

// Body returns the raw response body string.
func (r *HTTPResult) Body() string {  //nolint:deadcode
	return r.body
}

// Header returns the first value for the named response header, or "" if absent.
func (r *HTTPResult) Header(name string) string {  //nolint:deadcode
	vals := r.headers[name]
	if len(vals) > 0 {
		return vals[0]
	}
	// Try case-insensitive lookup.
	lower := strings.ToLower(name)
	for k, v := range r.headers {
		if strings.ToLower(k) == lower && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// StatusCode returns the raw HTTP status code.
func (r *HTTPResult) StatusCode() int {  //nolint:deadcode
	return r.statusCode
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// formatInvocation builds a human-readable CLI invocation string.
func formatInvocation(binary string, args []string) string {  //nolint:deadcode
	return fmt.Sprintf("%s %s", binary, strings.Join(args, " "))
}
