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
	TestRunID int64  // test_runs auto-increment ID (0 if not returned)
}

// CreateRun creates a run via POST /runs and returns the result.
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
		ID        string `json:"id"`
		TestRunID int64  `json:"test_run_id,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("DaemonClient.CreateRun: decode response: %v", err)
	}
	return CreateRunResult{DaemonID: result.ID, TestRunID: result.TestRunID}
}

// MarkDoneOpts holds fields for completing a run via PUT /runs/:id/done.
type MarkDoneOpts struct {
	Passed       *bool
	Skipped      *bool
	Reason       string
	FinishedAt   string
	InputTokens  *int64
	OutputTokens *int64
	CostUSD      *float64
}

// MarkDone marks a run as done via PUT /runs/:id/done.
func (c *DaemonClient) MarkDone(t *testing.T, runID string, opts MarkDoneOpts) {
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
	req, _ := http.NewRequest("PUT", c.baseURL+"/runs/"+runID+"/done", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("DaemonClient.MarkDone: PUT /runs/%s/done: %v", runID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.MarkDone: PUT /runs/%s/done → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// AddEvent adds an event to a run via POST /runs/:id/events.
func (c *DaemonClient) AddEvent(t *testing.T, runID, kind, message string) {
	t.Helper()
	body := map[string]any{
		"kind":      kind,
		"message":   message,
		"elapsed_s": 0.5,
	}
	b, _ := json.Marshal(body)
	resp, err := c.client.Post(c.baseURL+"/runs/"+runID+"/events", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("DaemonClient.AddEvent: POST /runs/%s/events: %v", runID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.AddEvent: POST /runs/%s/events → %d: %s", runID, resp.StatusCode, string(respBody))
	}
}

// SetMetadata updates a string field on the test_runs row via PUT /runs/:id/<field>.
func (c *DaemonClient) SetMetadata(t *testing.T, runID, field, value string) { //nolint:deadcode
	t.Helper()
	body := map[string]string{"value": value}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", c.baseURL+"/runs/"+runID+"/"+field, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("DaemonClient.SetMetadata: PUT /runs/%s/%s: %v", runID, field, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("DaemonClient.SetMetadata: PUT /runs/%s/%s → %d: %s", runID, field, resp.StatusCode, string(respBody))
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
