// Package e2eframework — step.go
//
// Step: a scoped test action block created by TestContext.Step.
// Each Step has a name, a reference to its parent TestContext, and provides
// typed action methods (CLI, CLIExpectError, HTTP, Log, WriteFile) that
// automatically record events to RunLog.
package runlog

import (
	"bytes"
	"fmt"
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
// are automatically logged to RunLog as a "cli" event.
//
// If the command exits non-zero, the test fails via rl.Failf.
// Returns a *CLIResult for chainable assertions.
func (s *Step) CLI(args ...string) *CLIResult {
	s.tc.T.Helper()
	binary := s.tc.Binary
	invocation := formatInvocation(binary, args)

	out, err := RunBinaryInDirWithHome(s.tc.T, binary, "", s.tc.Home, args...)

	// Log to RunLog regardless of outcome.
	s.tc.RunLog.CLIStepErr(s.name+": "+invocation, invocation, strings.TrimSpace(out), err)

	if err != nil {
		s.tc.RunLog.Failf("CLI command failed: %s\nerror: %v\noutput:\n%s", invocation, err, out)
	}

	return newCLIResultFromCombined(s.tc.RunLog, out, nil) // err is nil here (we fatalf'd above)
}

// CLIExpectError executes the configured binary with the given args, but does
// NOT fail the test on non-zero exit.  The exit code, stdout, and stderr are
// captured for assertion via the returned *CLIResult.
func (s *Step) CLIExpectError(args ...string) *CLIResult {
	s.tc.T.Helper()
	binary := s.tc.Binary
	invocation := formatInvocation(binary, args)

	out, err := RunBinaryInDirWithHome(s.tc.T, binary, "", s.tc.Home, args...)

	// Log to RunLog — include error info if present.
	s.tc.RunLog.CLIStepErr(s.name+": "+invocation, invocation, strings.TrimSpace(out), err)

	return newCLIResultFromCombined(s.tc.RunLog, out, err)
}

// HTTP makes an authenticated HTTP request to tc.Server + path and returns
// an *HTTPResult for chainable assertions.  The request uses the auth token
// from tc.Token and the project ID from tc.ProjectID.
//
// An optional body may be provided (at most one); if present, Content-Type
// is set to application/json.
func (s *Step) HTTP(method, path string, body ...[]byte) *HTTPResult {
	s.tc.T.Helper()
	url := s.tc.Server + path

	var reqBody io.Reader
	if len(body) > 0 && body[0] != nil {
		reqBody = bytes.NewReader(body[0])
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		s.tc.RunLog.Failf("HTTP: cannot build request %s %s: %v", method, url, err)
		return newHTTPResult(s.tc.RunLog, 0, "", nil)
	}
	if len(body) > 0 && body[0] != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	SetAuthHeader(req, s.tc.Token)
	if s.tc.ProjectID != "" {
		req.Header.Set("X-Project-ID", s.tc.ProjectID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.tc.RunLog.Failf("HTTP: request failed %s %s: %v", method, url, err)
		return newHTTPResult(s.tc.RunLog, 0, "", nil)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		s.tc.RunLog.Failf("HTTP: cannot read response body %s %s: %v", method, url, err)
		return newHTTPResult(s.tc.RunLog, resp.StatusCode, "", nil)
	}

	bodyStr := string(respBody)

	// Log to RunLog.
	desc := fmt.Sprintf("%s: %s %s → %d", s.name, method, path, resp.StatusCode)
	details := fmt.Sprintf("%s %s\nStatus: %d\nBody: %s", method, url, resp.StatusCode, Truncate(bodyStr, 500))
	s.tc.RunLog.CLIStep(desc, fmt.Sprintf("%s %s", method, url), details)

	return newHTTPResult(s.tc.RunLog, resp.StatusCode, bodyStr, resp.Header)
}

// Log writes a scoped log message to RunLog under the current step's section.
func (s *Step) Log(format string, args ...any) {
	s.tc.RunLog.Printf(format, args...)
}

// WriteFile creates a file at path (relative to tc.Home) with the given
// content, and logs the action to RunLog.
func (s *Step) WriteFile(path, content string) {
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
