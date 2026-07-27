package runlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// DaemonClient is a test helper that communicates with a runlog daemon via HTTP.
// It replaces all direct DB writes from tests.
type DaemonClient struct {
	baseURL string
	client  *http.Client
}

// NewDaemonClient creates a DaemonClient talking to the given daemon base URL.
func NewDaemonClient(baseURL string) *DaemonClient {
	return &DaemonClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// CreateRunOpts holds all fields for creating a test run via POST /runs.
type CreateRunOpts struct {
	PID            int
	EnvProfile     string
	ServerURL      string
	Token          string
	Category       string
	Tags           []string
	Description    string
	Experiment     string
	Runner         string
	AppVersion     string
	TestVersion    string
	EnvVars        map[string]string
	StartedAt      string
	TimeoutSeconds float64
}

// CreateRunResult holds the response from creating a run.
type CreateRunResult struct {
	DaemonID  string // daemon_runs UUID
	TestRunID int64  // test_runs auto-increment ID (0 for batch creation only)
}

// CreateRunNF is the non-fatal variant of CreateRun. Returns (CreateRunResult, error).
func (c *DaemonClient) CreateRunNF(t *testing.T, opts CreateRunOpts) (CreateRunResult, error) {
	t.Helper()
	pid := opts.PID
	if pid <= 0 {
		pid = 12345
	}
	body := map[string]any{
		"pid":             pid,
		"env_profile":     opts.EnvProfile,
		"server_url":      opts.ServerURL,
		"token":           opts.Token,
		"category":        opts.Category,
		"tags":            opts.Tags,
		"description":     opts.Description,
		"experiment":      opts.Experiment,
		"runner":          opts.Runner,
		"app_version":     opts.AppVersion,
		"test_version":    opts.TestVersion,
		"env_vars":        opts.EnvVars,
		"started_at":      opts.StartedAt,
		"timeout_seconds": opts.TimeoutSeconds,
	}
	b, _ := json.Marshal(body)
	resp, err := c.client.Post(c.baseURL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("POST /runs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return CreateRunResult{}, fmt.Errorf("POST /runs → %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CreateRunResult{}, fmt.Errorf("decode response: %w", err)
	}
	return CreateRunResult{DaemonID: result.ID}, nil
}

// CreateRun creates a daemon batch run via POST /runs and returns the result.
// Creates only a daemon_runs row (no test_runs). Use RegisterFunction to create
// per-test-function test_runs rows within the batch.
func (c *DaemonClient) CreateRun(t *testing.T, opts CreateRunOpts) CreateRunResult {
	t.Helper()
	pid := opts.PID
	if pid <= 0 {
		pid = 12345
	}
	body := map[string]any{
		"pid":             pid,
		"env_profile":     opts.EnvProfile,
		"server_url":      opts.ServerURL,
		"token":           opts.Token,
		"category":        opts.Category,
		"tags":            opts.Tags,
		"description":     opts.Description,
		"experiment":      opts.Experiment,
		"runner":          opts.Runner,
		"app_version":     opts.AppVersion,
		"test_version":    opts.TestVersion,
		"env_vars":        opts.EnvVars,
		"started_at":      opts.StartedAt,
		"timeout_seconds": opts.TimeoutSeconds,
	}
	b, _ := json.Marshal(body)
	resp, err := c.client.Post(c.baseURL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("DaemonClient.CreateRun: POST /runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.CreateRun: POST /runs → %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("DaemonClient.CreateRun: decode response: %v", err)
	}
	return CreateRunResult{DaemonID: result.ID}
}

// RegisterFunction creates a per-test-function test_runs row within a daemon
// batch via POST /runs/:id/functions. Returns the new test_runs.id.
func (c *DaemonClient) RegisterFunction(batchID, testName, testType, experiment string) (int64, error) {
	body := map[string]string{
		"test_name":  testName,
		"test_type":  testType,
		"experiment": experiment,
	}
	b, _ := json.Marshal(body)
	url := c.baseURL + "/runs/" + batchID + "/functions"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("POST %s → %d: %s", url, resp.StatusCode, string(respBody))
	}
	var result struct {
		TestRunID int64 `json:"test_run_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	return result.TestRunID, nil
}

// CreateTestRun creates a batch + registers a test function in one call.
// AfterCreateRun, calls RegisterFunction with testName. Returns the
// daemon batch ID and the test_runs.id for subsequent AddEvent/MarkDone calls.
func (c *DaemonClient) CreateTestRun(t *testing.T, opts CreateRunOpts, testName string) CreateRunResult {
	t.Helper()
	r := c.CreateRun(t, opts)
	id, err := c.RegisterFunction(r.DaemonID, testName, "", "")
	if err != nil {
		t.Fatalf("DaemonClient.CreateTestRun: RegisterFunction: %v", err)
	}
	return CreateRunResult{DaemonID: r.DaemonID, TestRunID: id}
}

// MarkDoneOpts holds fields for completing a run via PUT /test-runs/:id/done.
type MarkDoneOpts struct {
	Passed       *bool
	Skipped      *bool
	Reason       string
	FinishedAt   string
	InputTokens  *int64
	OutputTokens *int64
	CostUSD      *float64
}

// MarkDone marks a run as done via PUT /test-runs/:id/done.
func (c *DaemonClient) MarkDone(t *testing.T, runID int64, opts MarkDoneOpts) {
	t.Helper()
	body := map[string]any{}
	if opts.Passed != nil {
		body["passed"] = *opts.Passed
	}
	if opts.Skipped != nil {
		body["skipped"] = *opts.Skipped
	}
	if opts.Reason != "" {
		body["reason"] = opts.Reason
	}
	if opts.FinishedAt != "" {
		body["finished_at"] = opts.FinishedAt
	}
	if opts.InputTokens != nil {
		body["input_tokens"] = *opts.InputTokens
	}
	if opts.OutputTokens != nil {
		body["output_tokens"] = *opts.OutputTokens
	}
	if opts.CostUSD != nil {
		body["cost_usd"] = *opts.CostUSD
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/done", c.baseURL, runID)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("DaemonClient.MarkDone: PUT /test-runs/%d/done: %v", runID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.MarkDone: PUT /test-runs/%d/done → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// AddEvent adds an event to a run via POST /test-runs/:id/events.
func (c *DaemonClient) AddEvent(t *testing.T, runID int64, kind, message string) {
	t.Helper()
	body := map[string]any{
		"kind":      kind,
		"message":   message,
		"elapsed_s": 0.5,
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/events", c.baseURL, runID)
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("DaemonClient.AddEvent: POST /test-runs/%d/events: %v", runID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.AddEvent: POST /test-runs/%d/events → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// SetMetadata updates a string field on the test_runs row via PUT /test-runs/:id/metadata/:field.
func (c *DaemonClient) SetMetadata(t *testing.T, runID int64, field, value string) { //nolint:deadcode
	t.Helper()
	body := map[string]string{"value": value}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/metadata/%s", c.baseURL, runID, field)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("DaemonClient.SetMetadata: PUT /test-runs/%d/metadata/%s: %v", runID, field, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.SetMetadata: PUT /test-runs/%d/metadata/%s → %d: %s", runID, field, resp.StatusCode, string(respBody))
	}
}

// ListTestRuns returns all test_runs via GET /test-runs.
func (c *DaemonClient) ListTestRuns(t *testing.T) []map[string]any {
	t.Helper()
	resp, err := c.client.Get(c.baseURL + "/test-runs")
	if err != nil {
		t.Fatalf("DaemonClient.ListTestRuns: GET /test-runs: %v", err)
	}
	defer resp.Body.Close()
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("DaemonClient.ListTestRuns: decode: %v", err)
	}
	return result
}

// RunCount returns the number of test_runs via GET /test-runs.
func (c *DaemonClient) RunCount(t *testing.T) int { //nolint:deadcode
	t.Helper()
	return len(c.ListTestRuns(t))
}

// MustGetTestRun fetches a single test_run by ID via GET /test-runs/:id.
func (c *DaemonClient) MustGetTestRun(t *testing.T, id int64) map[string]any {
	t.Helper()
	url := fmt.Sprintf("%s/test-runs/%d", c.baseURL, id)
	resp, err := c.client.Get(url)
	if err != nil {
		t.Fatalf("DaemonClient.MustGetTestRun: GET /test-runs/%d: %v", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.MustGetTestRun: GET /test-runs/%d → %d: %s", id, resp.StatusCode, string(respBody))
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("DaemonClient.MustGetTestRun: decode: %v", err)
	}
	return result
}

// MustGetEvents fetches events for a test_run via GET /test-runs/:id/events.
func (c *DaemonClient) MustGetEvents(t *testing.T, id int64) []map[string]any { //nolint:deadcode
	t.Helper()
	url := fmt.Sprintf("%s/test-runs/%d/events", c.baseURL, id)
	resp, err := c.client.Get(url)
	if err != nil {
		t.Fatalf("DaemonClient.MustGetEvents: GET /test-runs/%d/events: %v", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.MustGetEvents: GET /test-runs/%d/events → %d: %s", id, resp.StatusCode, string(respBody))
	}
	var result []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("DaemonClient.MustGetEvents: decode: %v", err)
	}
	return result
}

// addEventNF is the non-fatal variant of AddEvent for use inside RunLog.
func (c *DaemonClient) addEventNF(t *testing.T, runID int64, kind, message string, details any, elapsedSec float64, durationMs float64) { //nolint:deadcode
	t.Helper()
	body := map[string]any{
		"kind":      kind,
		"message":   message,
		"elapsed_s": elapsedSec,
	}
	if details != nil {
		body["details"] = details
	}
	if durationMs > 0 {
		body["duration_ms"] = durationMs
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/events", c.baseURL, runID)
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Logf("warn: DaemonClient.addEventNF: POST /test-runs/%d/events: %v", runID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("warn: DaemonClient.addEventNF: POST /test-runs/%d/events → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// markDoneNF is the non-fatal variant of MarkDone for use inside RunLog.Close().
func (c *DaemonClient) markDoneNF(t *testing.T, runID int64, outcome RunOutcome, reason string, inputTokens, outputTokens int64, costUSD float64) {
	t.Helper()
	body := map[string]any{}
	switch outcome {
	case OutcomePass:
		body["passed"] = true
	case OutcomeFail:
		body["passed"] = false
	case OutcomeSkip:
		body["skipped"] = true
	}
	if reason != "" {
		body["reason"] = reason
	}
	if inputTokens > 0 {
		body["input_tokens"] = inputTokens
	}
	if outputTokens > 0 {
		body["output_tokens"] = outputTokens
	}
	if costUSD > 0 {
		body["cost_usd"] = costUSD
	}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/done", c.baseURL, runID)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Logf("warn: DaemonClient.markDoneNF: PUT /test-runs/%d/done: %v", runID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("warn: DaemonClient.markDoneNF: PUT /test-runs/%d/done → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// putRawOutputNF uploads raw log content to the test_runs row via PUT
// /test-runs/:id/raw_output. Non-fatal (logs warning on failure).
func (c *DaemonClient) putRawOutputNF(t *testing.T, runID int64, output string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"output": output})
	url := fmt.Sprintf("%s/test-runs/%d/raw_output", c.baseURL, runID)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Logf("warn: DaemonClient.putRawOutputNF: PUT /test-runs/%d/raw_output: %v", runID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("warn: DaemonClient.putRawOutputNF: PUT /test-runs/%d/raw_output → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// setMetadataNF is the non-fatal variant of SetMetadata for use inside RunLog.
func (c *DaemonClient) setMetadataNF(t *testing.T, runID int64, field, value string) { //nolint:deadcode
	t.Helper()
	body := map[string]string{"value": value}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/test-runs/%d/metadata/%s", c.baseURL, runID, field)
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Logf("warn: DaemonClient.setMetadataNF: PUT /test-runs/%d/metadata/%s: %v", runID, field, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("warn: DaemonClient.setMetadataNF: PUT /test-runs/%d/metadata/%s → %d: %s", runID, field, resp.StatusCode, string(respBody))
	}
}
