package main

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
// The test name is stored as-is — categories come from rl.SetCategory(), not from prefixes.
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
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(daemonURL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil || resp.StatusCode != 201 {
		return df // fail-open
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
func (df *DogfoodRun) Event(kind, message string) {
	if !df.active {
		return
	}
	df.seq++
	body := map[string]any{
		"kind":      kind,
		"message":   message,
		"elapsed_s": 0.5,
	}
	b, _ := json.Marshal(body)
	http.Post(fmt.Sprintf("%s/runs/%s/events", df.url, df.runID), "application/json", bytes.NewReader(b))
}

// Eventf formats and emits an event.
func (df *DogfoodRun) Eventf(kind, format string, args ...any) {
	df.Event(kind, fmt.Sprintf(format, args...))
}

// RunCLI executes a CLI command function, captures its stdout+stderr, and emits a cli event
// with the full command, output (truncated), and exit code. Returns the combined output.
//
//	output := df.RunCLI("runlog runs --since 24h", func() error {
//	    return cmdRuns(db, 24*time.Hour)
//	})
//	if !strings.Contains(output, "TestFoo") { t.Error(...) }
func (df *DogfoodRun) RunCLI(commandDesc string, fn func() error) string {
	if !df.active {
		// When daemon is unavailable, still run the command and return output
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

	// Capture stdout + stderr
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

	// Truncate output for the event
	outputForEvent := output
	if len(outputForEvent) > 4096 {
		outputForEvent = outputForEvent[:4096] + "\n... (truncated)"
	}

	// Emit cli event with full details
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
// This is the canonical way to record HTTP interactions — never use kind="log" for HTTP calls.
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
	body := map[string]bool{"passed": passed}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", df.url, df.runID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
}

// Fail marks the run as failed with a reason (no-op if daemon not available).
func (df *DogfoodRun) Fail(reason string) {
	if !df.active {
		return
	}
	df.Event("failure", reason)

	body := map[string]bool{"passed": false}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", df.url, df.runID), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
}
