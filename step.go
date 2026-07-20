// Package e2eframework — step.go
//
// Step: a scoped test action block created by TestContext.Step.
// Each Step has a name, a reference to its parent TestContext, and provides
// typed action methods (CLI, CLIExpectError, HTTP, Log, WriteFile) that
// automatically record events to RunLog.
package runlog

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Step represents a single named action block within a test.  It is created
// by TestContext.Step and passed to the step closure.  All actions executed
// through a Step are automatically logged to the parent TestContext's RunLog.
type Step struct {
	tc      *TestContext
	name    string
	startAt time.Time
}

// CLI executes the configured binary (tc.Binary) with the given args, using
// the TestContext's Home directory and environment.  The invocation and output
// are automatically logged to RunLog as a "cli" event with the execution
// duration tracked.
//
// If the command exits non-zero, the test fails via rl.Failf.
// Returns a *CLIResult for chainable assertions.
func (s *Step) CLI(args ...string) *CLIResult { //nolint:deadcode
	s.tc.T.Helper()
	binary := s.tc.Binary
	invocation := formatInvocation(binary, args)

	start := time.Now()
	out, err := RunBinaryInDirWithHome(s.tc.T, binary, "", s.tc.Home, args...)
	elapsed := time.Since(start)

	// Log to RunLog regardless of outcome — with measured duration.
	s.tc.RunLog.CLIStepErr(s.name+": "+invocation, invocation, strings.TrimSpace(out), err, elapsed)

	if err != nil {
		s.tc.RunLog.Failf("CLI command failed: %s\nerror: %v\noutput:\n%s", invocation, err, out)
	}

	return newCLIResultFromCombined(s.tc.RunLog, out, nil) // err is nil here (we fatalf'd above)
}

// CLIExpectError executes the configured binary with the given args, but does
// NOT fail the test on non-zero exit.  The exit code, stdout, and stderr are
// captured for assertion via the returned *CLIResult.
func (s *Step) CLIExpectError(args ...string) *CLIResult { //nolint:deadcode
	s.tc.T.Helper()
	binary := s.tc.Binary
	invocation := formatInvocation(binary, args)

	start := time.Now()
	out, err := RunBinaryInDirWithHome(s.tc.T, binary, "", s.tc.Home, args...)
	elapsed := time.Since(start)

	// Log to RunLog — include error info if present.
	s.tc.RunLog.CLIStepErr(s.name+": "+invocation, invocation, strings.TrimSpace(out), err, elapsed)

	return newCLIResultFromCombined(s.tc.RunLog, out, err)
}

// HTTP makes an authenticated HTTP request to tc.Server + path and returns
// an *HTTPResult for chainable assertions.  The request uses the auth token
// from tc.Token and the project ID from tc.ProjectID.
//
// The request round-trip duration is measured, and the result is logged as an
// http_call event (not a cli event). Pass nil for body on GET/DELETE requests.
// If body is non-nil, Content-Type is set to application/json.
//
// Optional expects run inline after the call; failures call rl.Failf.
//
//	s.HTTP("GET", "/api/health", nil, ExpectStatus(200), ExpectBodyContains("ok"))
func (s *Step) HTTP(method, path string, body []byte, expects ...HTTPExpect) *HTTPResult { //nolint:deadcode
	s.tc.T.Helper()
	url := s.tc.Server + path

	var reqBody io.Reader
	var reqBodyBytes []byte
	if body != nil {
		reqBodyBytes = body
		reqBody = bytes.NewReader(reqBodyBytes)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		s.tc.RunLog.Failf("HTTP: cannot build request %s %s: %v", method, url, err)
		return newHTTPResult(s.tc.RunLog, 0, "", nil)
	}
	if len(reqBodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	SetAuthHeader(req, s.tc.Token)
	if s.tc.ProjectID != "" {
		req.Header.Set("X-Project-ID", s.tc.ProjectID)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		s.tc.RunLog.HTTPCall(method, path, 0, string(reqBodyBytes), "", duration)
		s.tc.RunLog.Failf("HTTP: request failed %s %s: %v", method, url, err)
		result := newHTTPResult(s.tc.RunLog, 0, "", nil)
		result.Expect(expects...)
		return result
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.tc.RunLog.HTTPCall(method, path, resp.StatusCode, string(reqBodyBytes), "", duration)
		s.tc.RunLog.Failf("HTTP: cannot read response body %s %s: %v", method, url, err)
		result := newHTTPResult(s.tc.RunLog, resp.StatusCode, "", nil)
		result.Expect(expects...)
		return result
	}

	bodyStr := string(respBody)
	s.tc.RunLog.HTTPCall(method, path, resp.StatusCode, string(reqBodyBytes), bodyStr, duration)

	result := newHTTPResult(s.tc.RunLog, resp.StatusCode, bodyStr, resp.Header)
	result.Expect(expects...)
	return result
}

// Log writes a scoped log message to RunLog under the current step's section.
func (s *Step) Log(format string, args ...any) { //nolint:deadcode
	s.tc.RunLog.Printf(format, args...)
}

// WriteFile creates a file at path (relative to tc.Home) with the given
// content, and logs the action to RunLog.
func (s *Step) WriteFile(path, content string) { //nolint:deadcode
	s.tc.T.Helper()
	fullPath := filepath.Join(s.tc.Home, path)

	// Ensure the parent directory exists.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.tc.RunLog.Failf("WriteFile: cannot create directory %s: %v", dir, err)
		return
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		s.tc.RunLog.Failf("WriteFile: cannot write %s: %v", fullPath, err)
		return
	}

	s.tc.RunLog.Printf("wrote file %s (%d bytes)", path, len(content))
}
