package runlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RunRecorder records CLI commands and HTTP calls as structured events in the
// runlog database. When RUNLOG_DAEMON_URL is set, it also registers runs with
// the daemon so they appear in the web UI automatically.
//
// Usage:
//
//	r := NewRunRecorder()
//	runID, _ := r.RegisterRun("test_suite_name")
//	defer r.MarkDone(runID, !t.Failed())
//
//	output, err := r.CLICapture("go test -v -run TestFoo", func() error {
//	    return cmd.Run()
//	})
//
//	r.HTTPCall("GET", "/api/health", 200, "", `{"status":"ok"}`)
type RunRecorder struct {
	daemonURL string
	runID     string
	active    bool
	seq       int
	startedAt time.Time
	db        *RunDB
}

// NewRunRecorder creates a RunRecorder. When RUNLOG_DAEMON_URL is set, it
// connects to the daemon for automatic run registration. The optional *RunDB
// is used for direct DB writes when no daemon is available.
func NewRunRecorder(db *RunDB) *RunRecorder { //nolint:deadcode
	r := &RunRecorder{
		startedAt: time.Now(),
		db:        db,
	}
	if u := os.Getenv("RUNLOG_DAEMON_URL"); u != "" {
		r.daemonURL = u
	}
	return r
}

// RegisterRun creates a test run via the daemon API (or directly in the DB)
// and returns the run ID. The event stream starts with a state_change event.
func (r *RunRecorder) RegisterRun(testName string) (string, error) { //nolint:deadcode
	r.runID = ""
	r.active = false
	r.seq = 0

	if r.daemonURL != "" {
		body := map[string]any{
			"pid":         os.Getpid(),
			"env_profile": testName,
		}
		b, _ := json.Marshal(body)
		resp, err := http.Post(r.daemonURL+"/runs", "application/json", bytes.NewReader(b))
		if err == nil && resp.StatusCode == 201 {
			defer resp.Body.Close()
			var result map[string]string
			json.NewDecoder(resp.Body).Decode(&result)
			r.runID = result["id"]
			r.active = true
		}
	}

	if !r.active && r.db != nil {
		// Fallback: insert directly into DB
		id, err := r.db.InsertRun(testName, time.Now(), "runrecorder", "", nil)
		if err == nil {
			r.runID = fmt.Sprintf("%d", id)
			r.active = true
		}
	}

	if r.active {
		r.emit("state_change", "test started", nil)
	}
	return r.runID, nil
}

// MarkDone marks the run as finished with the given pass/fail status.
func (r *RunRecorder) MarkDone(runID string, passed bool) error { //nolint:deadcode
	if !r.active {
		return nil
	}
	r.emit("state_change", "test finished", nil)

	if r.daemonURL != "" {
		body := map[string]bool{"passed": passed}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", r.daemonURL, runID), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		http.DefaultClient.Do(req)
	}
	if r.db != nil {
		var tid int64
		fmt.Sscanf(runID, "%d", &tid)
		if tid > 0 {
			outcome := OutcomePass
			if !passed {
				outcome = OutcomeFail
			}
			r.db.FinishRun(tid, time.Now(), outcome, "")
		}
	}
	return nil
}

// Event emits an arbitrary event with the given kind, message, and optional
// structured details. Details should be a JSON-serializable map.
func (r *RunRecorder) Event(kind, message string, details map[string]any) error { //nolint:deadcode
	return r.emit(kind, message, details)
}

// CLICapture runs fn, captures its combined stdout+stderr, and emits a cli
// event with the full command description, output, and exit code.
// Returns the captured output. When fn returns an error, the output is still
// returned (the error is included in the cli event's exit_code).
//
//	output := r.CLICapture("go test -v -run TestFoo", func() error {
//	    cmd := exec.Command("go", "test", "-v", "-run", "TestFoo")
//	    cmd.Stdout = os.Stdout
//	    cmd.Stderr = os.Stderr
//	    return cmd.Run()
//	})
func (r *RunRecorder) CLICapture(description string, fn func() error) string { //nolint:deadcode
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

	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	output := buf.String()

	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	// Truncate output for the event details
	outputBrief := output
	if len(outputBrief) > 4096 {
		outputBrief = outputBrief[:4096] + "\n... (truncated)"
	}

	r.emit("cli", fmt.Sprintf("$ %s", description), map[string]any{
		"command":    description,
		"exit_code":  exitCode,
		"output":     outputBrief,
		"stderr":     "",
		"stdout_len": len(output),
	})

	return output
}

// HTTPCall emits an http_call event with full request/response details.
// method, url, and statusCode are required. requestBody and responseBody
// are optional and will be truncated to 2048 bytes in the event details.
func (r *RunRecorder) HTTPCall(method, url string, statusCode int, requestBody, responseBody string) { //nolint:deadcode
	details := map[string]any{
		"method":      method,
		"url":         url,
		"status_code": statusCode,
	}
	if requestBody != "" {
		if len(requestBody) > 2048 {
			requestBody = requestBody[:2048] + "..."
		}
		details["request_body"] = requestBody
	}
	if responseBody != "" {
		if len(responseBody) > 2048 {
			responseBody = responseBody[:2048] + "..."
		}
		details["response_body"] = responseBody
	}

	msg := fmt.Sprintf("%s %s → %d", method, url, statusCode)
	r.emit("http_call", msg, details)
}

// emit sends an event to the daemon or writes it directly to the DB.
func (r *RunRecorder) emit(kind, message string, details map[string]any) error { //nolint:deadcode
	if !r.active {
		return nil
	}
	r.seq++

	if r.daemonURL != "" && r.runID != "" {
		body := map[string]any{
			"kind":      kind,
			"message":   message,
			"elapsed_s": time.Since(r.startedAt).Seconds(),
			"details":   details,
		}
		b, _ := json.Marshal(body)
		http.Post(fmt.Sprintf("%s/runs/%s/events", r.daemonURL, r.runID),
			"application/json", bytes.NewReader(b))
		return nil
	}

	if r.db != nil {
		var tid int64
		if _, err := fmt.Sscanf(r.runID, "%d", &tid); err != nil || tid == 0 {
			return nil
		}
		var detailsJSON *string
		if len(details) > 0 {
			b, _ := json.Marshal(details)
			s := string(b)
			detailsJSON = &s
		}
		_ = r.db.InsertEvent(tid, r.seq, time.Now(), time.Since(r.startedAt).Seconds(),
			kind, message, detailsJSON)
	}
	return nil
}

// ExtractOutput is a helper that extracts a specific line from CLI output.
// Useful for test assertions.
func ExtractOutput(output, prefix string) string { //nolint:deadcode
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}
