package runlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

// DogfoodRun represents a single test run registered with the dogfood daemon.
// Usage:
//
//	df := NewDogfoodRun(t)
//	defer df.Done()
//	df.Event("log", "step 1 complete")
//	df.HTTPCall("GET", "/api/health", 200, `{"status":"ok"}`)
type DogfoodRun struct {
	t      *testing.T
	url    string
	runID  string
	seq    int
	active bool
}

// NewDogfoodRun registers a test run with the dogfood daemon (if RUNLOG_DAEMON_URL is set).
func NewDogfoodRun(t *testing.T, category string) *DogfoodRun {
	t.Helper()
	df := &DogfoodRun{t: t, active: false}

	daemonURL := os.Getenv("RUNLOG_DAEMON_URL")
	if daemonURL == "" {
		return df
	}
	df.url = daemonURL

	testName := t.Name()
	body := map[string]any{
		"pid":         os.Getpid(),
		"env_profile": testName,
		"category":    category,
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(daemonURL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != 201 {
		return df
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	df.runID = result["id"]
	df.active = true

	df.Event("state_change", "test started")
	return df
}

// Event emits an event for this run (no-op if daemon not available).
func (df *DogfoodRun) Event(kind, message string, details ...map[string]any) {
	if !df.active {
		return
	}
	df.seq++
	body := map[string]any{
		"kind":      kind,
		"message":   message,
		"elapsed_s": 0.5,
	}
	if len(details) > 0 && details[0] != nil {
		body["details"] = details[0]
	}
	b, _ := json.Marshal(body)
	http.Post(fmt.Sprintf("%s/runs/%s/events", df.url, df.runID), "application/json", bytes.NewReader(b))
}

// Eventf formats and emits an event.
func (df *DogfoodRun) Eventf(kind, format string, args ...any) { //nolint:deadcode
	df.Event(kind, fmt.Sprintf(format, args...))
}

// EventHTTP records an http_call with full request/response details.
func (df *DogfoodRun) EventHTTP(method, url string, statusCode int, reqBody, respBody string) {
	details := map[string]any{
		"method":      method,
		"url":         url,
		"status_code": statusCode,
	}
	if reqBody != "" {
		details["request_body"] = reqBody
	}
	if respBody != "" {
		details["response_body"] = respBody
	}
	msg := fmt.Sprintf("%s %s → %d", method, url, statusCode)
	df.Event("http_call", msg, details)
}

// EventAssertion records an assertion with expected/actual values.
func (df *DogfoodRun) EventAssertion(msg string, expected, actual any, extra ...map[string]any) {
	details := map[string]any{
		"expected": expected,
		"actual":   actual,
	}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			details[k] = v
		}
	}
	df.Event("assertion", msg, details)
}

// RunCLI executes a CLI command function, captures its stdout+stderr, and emits a cli event.
func (df *DogfoodRun) RunCLI(commandDesc string, fn func() error) string {
	if !df.active {
		var buf bytes.Buffer
		oldOut := os.Stdout
		oldErr := os.Stderr
		rOut, wOut, _ := os.Pipe()
		rErr, wErr, _ := os.Pipe()
		os.Stdout = wOut
		os.Stderr = wErr

		err := fn()
		wOut.Close()
		wErr.Close()
		os.Stdout = oldOut
		os.Stderr = oldErr
		io.Copy(&buf, rOut)
		io.Copy(&buf, rErr)
		if err != nil {
			buf.WriteString(fmt.Sprintf("\n⚠ exit code: %v", err))
		}
		return buf.String()
	}

	df.seq++

	var buf bytes.Buffer
	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	err := fn()
	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)

	output := buf.String()

	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	outputForEvent := output
	if len(outputForEvent) > 4096 {
		outputForEvent = outputForEvent[:4096] + "\n... (truncated)"
	}

	details := map[string]any{
		"command":    commandDesc,
		"exit_code":  exitCode,
		"output":     outputForEvent,
		"stdout_len": len(output),
	}
	body := map[string]any{
		"kind":      "cli",
		"message":   fmt.Sprintf("$ %s", commandDesc),
		"elapsed_s": 0.5,
		"details":   details,
	}
	b, _ := json.Marshal(body)
	http.Post(fmt.Sprintf("%s/runs/%s/events", df.url, df.runID), "application/json", bytes.NewReader(b))

	return output
}

// HTTPCall emits an http_call event with method, url, status code and optional response body.
func (df *DogfoodRun) HTTPCall(method, url string, statusCode int, responseBody string) {
	if !df.active {
		return
	}
	df.seq++
	msg := fmt.Sprintf("%s %s → %d", method, url, statusCode)
	details := map[string]any{
		"method":      method,
		"url":         url,
		"status_code": statusCode,
	}
	if responseBody != "" {
		truncated := responseBody
		if len(truncated) > 1024 {
			truncated = truncated[:1024] + "..."
		}
		details["response_body"] = truncated
	}
	body := map[string]any{
		"kind":      "http_call",
		"message":   msg,
		"elapsed_s": 0.5,
		"details":   details,
	}
	b, _ := json.Marshal(body)
	http.Post(fmt.Sprintf("%s/runs/%s/events", df.url, df.runID), "application/json", bytes.NewReader(b))
}

// Done marks the run as passed (no-op if daemon not available).
func (df *DogfoodRun) Done() {
	if !df.active {
		return
	}
	df.Event("state_change", "test finished")

	passed := !df.t.Failed()
	body := map[string]any{"passed": passed}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", df.url, df.runID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		df.t.Logf("dogfood: Done: PUT failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		df.t.Logf("dogfood: Done: PUT returned %d", resp.StatusCode)
	}
}

// Describe sets the run description (no-op if daemon not available).
func (df *DogfoodRun) Describe(summary string, bullets ...string) {
	if !df.active {
		return
	}
	desc := RunDescription{Summary: summary}
	if len(bullets) > 0 {
		desc.Bullets = bullets
	}
	descJSON, _ := json.Marshal(desc)
	body := map[string]string{"value": string(descJSON)}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/description", df.url, df.runID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
}

// Fail marks the run as failed with a reason (no-op if daemon not available).
func (df *DogfoodRun) Fail(reason string) { //nolint:deadcode
	if !df.active {
		return
	}
	df.Event("failure", reason)

	body := map[string]any{"passed": false}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", df.url, df.runID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		df.t.Logf("dogfood: Fail: PUT failed: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		df.t.Logf("dogfood: Fail: PUT returned %d", resp.StatusCode)
	}
}
