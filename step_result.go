// Package e2eframework — step_result.go
//
// CLIResult and HTTPResult: chainable assertion types returned by Step actions.
// Each assertion logs to RunLog via AssertionStep and calls rl.Failf on failure.
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
func newCLIResult(rl *RunLog, stdout, stderr string, exitCode int, err error) *CLIResult { //nolint:deadcode
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
func newCLIResultFromCombined(rl *RunLog, combined string, err error) *CLIResult { //nolint:deadcode
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

// Expect runs each CLIExpect against this result. Use for grouped assertions:
//
//	r.Expect(runlog.ExpectContains("Created"), runlog.ExpectExitCode(0))
func (r *CLIResult) Expect(expects ...CLIExpect) *CLIResult { //nolint:deadcode
	for _, e := range expects {
		e(r)
	}
	return r
}

// Contains asserts that stdout contains ALL of the given substrings.
func (r *CLIResult) Contains(substrs ...string) *CLIResult { //nolint:deadcode
	for _, sub := range substrs {
		found := strings.Contains(r.stdout, sub)
		r.rl.AssertionStep("output contains", sub, found, map[string]any{
			"context": Truncate(r.stdout, 200),
		})
		if !found {
			r.rl.Failf("output does not contain %q\noutput:\n%s", sub, Truncate(r.stdout, 500))
		}
	}
	return r
}

// ContainsAny asserts that stdout contains at least one of the given substrings.
func (r *CLIResult) ContainsAny(substrs ...string) *CLIResult { //nolint:deadcode
	matched := ""
	for _, sub := range substrs {
		if strings.Contains(r.stdout, sub) {
			matched = sub
			break
		}
	}
	r.rl.AssertionStep("output contains any", fmt.Sprintf("%v", substrs), matched != "",
		map[string]any{"matched": matched, "context": Truncate(r.stdout, 200)})
	if matched == "" {
		r.rl.Failf("output does not contain any of %v\noutput:\n%s", substrs, Truncate(r.stdout, 500))
	} else {
		r.rl.Printf("assert: output contains one of %v ✓ (matched %q)", substrs, matched)
	}
	return r
}

// NotContains asserts that stdout does NOT contain any of the given substrings.
func (r *CLIResult) NotContains(substrs ...string) *CLIResult { //nolint:deadcode
	for _, sub := range substrs {
		found := strings.Contains(r.stdout, sub)
		r.rl.AssertionStep("output not contains", sub, !found, nil)
		if found {
			r.rl.Failf("output should not contain %q but does\noutput:\n%s", sub, Truncate(r.stdout, 500))
		}
	}
	return r
}

// Matches asserts that stdout matches the given regexp pattern.
func (r *CLIResult) Matches(pattern string) *CLIResult { //nolint:deadcode
	re, err := regexp.Compile(pattern)
	if err != nil {
		r.rl.Failf("invalid regex %q: %v", pattern, err)
		return r
	}
	matched := re.MatchString(r.stdout)
	r.rl.AssertionStep("output matches", pattern, matched, map[string]any{
		"context": Truncate(r.stdout, 200),
	})
	if !matched {
		r.rl.Failf("output does not match /%s/\noutput:\n%s", pattern, Truncate(r.stdout, 500))
	}
	return r
}

// Empty asserts that stdout is empty or whitespace-only.
func (r *CLIResult) Empty() *CLIResult { //nolint:deadcode
	empty := strings.TrimSpace(r.stdout) == ""
	r.rl.AssertionStep("output empty", true, empty, map[string]any{
		"actual": Truncate(r.stdout, 200),
	})
	if !empty {
		r.rl.Failf("expected empty output, got:\n%s", Truncate(r.stdout, 500))
	}
	return r
}

// ParseID extracts a UUID from stdout and stores it in *dst.
func (r *CLIResult) ParseID(dst *string) *CLIResult { //nolint:deadcode
	re := regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	match := re.FindString(r.stdout)
	r.rl.AssertionStep("parse UUID", "non-empty UUID", match != "", map[string]any{
		"extracted": match, "context": Truncate(r.stdout, 200),
	})
	if match == "" {
		r.rl.Failf("no UUID found in output\noutput:\n%s", Truncate(r.stdout, 500))
		return r
	}
	*dst = match
	return r
}

// JSONField parses stdout as JSON and extracts the named top-level field into *dst.
func (r *CLIResult) JSONField(field string, dst *string) *CLIResult { //nolint:deadcode
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &m); err != nil {
		r.rl.AssertionStep("JSON parse", "valid JSON", false, map[string]any{
			"error": err.Error(), "context": Truncate(r.stdout, 200),
		})
		r.rl.Failf("cannot parse output as JSON: %v\noutput:\n%s", err, Truncate(r.stdout, 500))
		return r
	}
	raw, ok := m[field]
	r.rl.AssertionStep(fmt.Sprintf("JSON field %q", field), "exists", ok, nil)
	if !ok {
		r.rl.Failf("JSON field %q not found in output\noutput:\n%s", field, Truncate(r.stdout, 500))
		return r
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		*dst = string(raw)
	} else {
		*dst = s
	}
	return r
}

// JSON unmarshals the full stdout into dst.
func (r *CLIResult) JSON(dst any) *CLIResult { //nolint:deadcode
	err := json.Unmarshal([]byte(r.stdout), dst)
	r.rl.AssertionStep("JSON unmarshal", "valid JSON", err == nil, map[string]any{
		"error": func() string { if err != nil { return err.Error() }; return "" }(),
	})
	if err != nil {
		r.rl.Failf("cannot unmarshal output as JSON: %v\noutput:\n%s", err, Truncate(r.stdout, 500))
	}
	return r
}

// ExitCode asserts that the command exited with the expected code.
func (r *CLIResult) ExitCode(expected int) *CLIResult { //nolint:deadcode
	r.rl.AssertionStep("exit code", expected, r.exitCode, nil)
	if r.exitCode != expected {
		r.rl.Failf("expected exit code %d, got %d\noutput:\n%s", expected, r.exitCode, Truncate(r.stdout, 500))
	}
	return r
}

// Output returns the raw stdout string.
func (r *CLIResult) Output() string { //nolint:deadcode
	return r.stdout
}

// StderrOutput returns the raw stderr string.
func (r *CLIResult) StderrOutput() string { //nolint:deadcode
	return r.stderr
}

// ─────────────────────────────────────────────────────────────────────────────
// CLIExpect — inline assertion type for CLI helpers
// ─────────────────────────────────────────────────────────────────────────────

// CLIExpect is a function that asserts one property of a CLI execution result.
// Use constructors like ExpectContains, ExpectExitCode, etc. and pass them to
// CLIResult.Expect() or Fixture.CLIAssert().
type CLIExpect func(r *CLIResult)

// ExpectContains returns a CLIExpect that asserts stdout contains all substrings.
func ExpectContains(substrs ...string) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) { r.Contains(substrs...) }
}

// ExpectContainsAny returns a CLIExpect that asserts stdout contains at least one substring.
func ExpectContainsAny(substrs ...string) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) { r.ContainsAny(substrs...) }
}

// ExpectNotContains returns a CLIExpect that asserts stdout does not contain substrings.
func ExpectNotContains(substrs ...string) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) { r.NotContains(substrs...) }
}

// ExpectMatches returns a CLIExpect that asserts stdout matches a regexp.
func ExpectMatches(pattern string) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) { r.Matches(pattern) }
}

// ExpectExitCode returns a CLIExpect that asserts the command exit code.
func ExpectExitCode(code int) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) { r.ExitCode(code) }
}

// ExpectOutputJSONField returns a CLIExpect that asserts a JSON field value in output.
func ExpectOutputJSONField(key, expected string) CLIExpect { //nolint:deadcode
	return func(r *CLIResult) {
		var got string
		r.JSONField(key, &got)
		r.rl.AssertionStep(fmt.Sprintf("JSON field %q", key), expected, got, nil)
		if got != expected {
			r.rl.Failf("JSON field %q = %q, expected %q", key, got, expected)
		}
	}
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
func newHTTPResult(rl *RunLog, statusCode int, body string, headers map[string][]string) *HTTPResult { //nolint:deadcode
	return &HTTPResult{
		rl:         rl,
		statusCode: statusCode,
		body:       body,
		headers:    headers,
	}
}

// NewHTTPResult constructs an HTTPResult for manual assertion use in low-level
// test code that calls RunLog.HTTPCall directly.
func NewHTTPResult(rl *RunLog, statusCode int, body string, headers map[string][]string) *HTTPResult { //nolint:deadcode
	return newHTTPResult(rl, statusCode, body, headers)
}

// Expect runs each HTTPExpect against this result. Use for grouped assertions:
//
//	r.Expect(runlog.ExpectStatus(200), runlog.ExpectBodyContains("ok"))
func (r *HTTPResult) Expect(expects ...HTTPExpect) *HTTPResult { //nolint:deadcode
	for _, e := range expects {
		e(r)
	}
	return r
}

// Status asserts the response status code matches expected.
func (r *HTTPResult) Status(expected int) *HTTPResult { //nolint:deadcode
	r.rl.AssertionStep("HTTP status", expected, r.statusCode, nil)
	if r.statusCode != expected {
		r.rl.Failf("expected HTTP status %d, got %d\nbody:\n%s", expected, r.statusCode, Truncate(r.body, 500))
	}
	return r
}

// JSONField parses the response body as JSON and extracts a top-level field into *dst.
func (r *HTTPResult) JSONField(field string, dst *string) *HTTPResult { //nolint:deadcode
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.body), &m); err != nil {
		r.rl.AssertionStep("JSON parse", "valid JSON", false, map[string]any{
			"error": err.Error(), "context": Truncate(r.body, 200),
		})
		r.rl.Failf("cannot parse response body as JSON: %v\nbody:\n%s", err, Truncate(r.body, 500))
		return r
	}
	raw, ok := m[field]
	r.rl.AssertionStep(fmt.Sprintf("JSON field %q", field), "exists", ok, nil)
	if !ok {
		r.rl.Failf("JSON field %q not found in response\nbody:\n%s", field, Truncate(r.body, 500))
		return r
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		*dst = string(raw)
	} else {
		*dst = s
	}
	return r
}

// JSONContains asserts that a top-level JSON field in the response body equals expected.
func (r *HTTPResult) JSONContains(field, expected string) *HTTPResult { //nolint:deadcode
	var got string
	r.JSONField(field, &got)
	r.rl.AssertionStep(fmt.Sprintf("HTTP JSON field %q", field), expected, got, nil)
	if got != expected {
		r.rl.Failf("HTTP JSON field %q = %q, expected %q", field, got, expected)
	}
	return r
}

// BodyContains asserts that the response body contains the given substring.
func (r *HTTPResult) BodyContains(substr string) *HTTPResult { //nolint:deadcode
	found := strings.Contains(r.body, substr)
	r.rl.AssertionStep("body contains", substr, found, map[string]any{
		"context": Truncate(r.body, 200),
	})
	if !found {
		r.rl.Failf("HTTP body does not contain %q\nbody:\n%s", substr, Truncate(r.body, 500))
	}
	return r
}

// JSON unmarshals the full response body into dst.
func (r *HTTPResult) JSON(dst any) *HTTPResult { //nolint:deadcode
	err := json.Unmarshal([]byte(r.body), dst)
	r.rl.AssertionStep("HTTP JSON unmarshal", "valid JSON", err == nil, map[string]any{
		"error": func() string { if err != nil { return err.Error() }; return "" }(),
	})
	if err != nil {
		r.rl.Failf("cannot unmarshal response body as JSON: %v\nbody:\n%s", err, Truncate(r.body, 500))
	}
	return r
}

// Body returns the raw response body string.
func (r *HTTPResult) Body() string { //nolint:deadcode
	return r.body
}

// Header returns the first value for the named response header, or "" if absent.
func (r *HTTPResult) Header(name string) string { //nolint:deadcode
	vals := r.headers[name]
	if len(vals) > 0 {
		return vals[0]
	}
	lower := strings.ToLower(name)
	for k, v := range r.headers {
		if strings.ToLower(k) == lower && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// StatusCode returns the raw HTTP status code.
func (r *HTTPResult) StatusCode() int { //nolint:deadcode
	return r.statusCode
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTPExpect — inline assertion type for HTTP helpers
// ─────────────────────────────────────────────────────────────────────────────

// HTTPExpect is a function that asserts one property of an HTTP response.
// Use constructors like ExpectStatus, ExpectBodyContains, etc. and pass them to
// HTTPResult.Expect() or directly to RunLog.HTTPGet, Step.HTTP, etc.
type HTTPExpect func(r *HTTPResult)

// ExpectStatus returns an HTTPExpect that asserts the response status code.
func ExpectStatus(code int) HTTPExpect { //nolint:deadcode
	return func(r *HTTPResult) { r.Status(code) }
}

// ExpectBodyContains returns an HTTPExpect that asserts the body contains substr.
func ExpectBodyContains(substr string) HTTPExpect { //nolint:deadcode
	return func(r *HTTPResult) { r.BodyContains(substr) }
}

// ExpectBodyNotContains returns an HTTPExpect that asserts the body does NOT contain substr.
func ExpectBodyNotContains(substr string) HTTPExpect { //nolint:deadcode
	return func(r *HTTPResult) {
		found := strings.Contains(r.body, substr)
		r.rl.AssertionStep("body not contains", substr, !found, nil)
		if found {
			r.rl.Failf("HTTP body should not contain %q\nbody:\n%s", substr, Truncate(r.body, 500))
		}
	}
}

// ExpectJSONField returns an HTTPExpect that asserts a JSON field equals expected.
func ExpectJSONField(key, expected string) HTTPExpect { //nolint:deadcode
	return func(r *HTTPResult) { r.JSONContains(key, expected) }
}

// ExpectHeader returns an HTTPExpect that asserts a response header value.
func ExpectHeader(name, expected string) HTTPExpect { //nolint:deadcode
	return func(r *HTTPResult) {
		got := r.Header(name)
		r.rl.AssertionStep(fmt.Sprintf("header %q", name), expected, got, nil)
		if got != expected {
			r.rl.Failf("expected header %q = %q, got %q", name, expected, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// formatInvocation builds a human-readable CLI invocation string.
func formatInvocation(binary string, args []string) string { //nolint:deadcode
	return fmt.Sprintf("%s %s", binary, strings.Join(args, " "))
}
