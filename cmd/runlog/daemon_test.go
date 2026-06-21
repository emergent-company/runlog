package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// ── Helpers ────────────────────────────────────────────────────────────────

// newDaemonTest creates a DaemonServer + RunDB with a temp SQLite DB for testing.
func newDaemonTest(t *testing.T) (*DaemonServer, *runlog.RunDB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	srv := newDaemonServer(db, 0, 30*time.Minute)
	return srv, db
}

// registerTestRun posts a test run to the daemon and returns the run UUID.
func registerTestRun(t *testing.T, baseURL string, pid int, profile string) string {
	t.Helper()
	body := map[string]any{
		"pid":         pid,
		"env_profile": profile,
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(baseURL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("registerTestRun POST /runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("registerTestRun: want 201, got %d", resp.StatusCode)
	}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	return result["id"]
}

// ── Tests ──────────────────────────────────────────────────────────────────

// TestDaemon_Health verifies GET /health returns 200 OK when the daemon is running.
func TestDaemon_Health(t *testing.T) {
	df := NewDogfoodRun(t, "daemon")
	defer df.Done()
	df.Event("log", "Verify GET /health returns 200 OK")

	srv, _ := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	var bodyBuf bytes.Buffer
	bodyBuf.ReadFrom(resp.Body)
	bodyStr := bodyBuf.String()

	df.HTTPCall("GET", "/health", resp.StatusCode, bodyStr)
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

// TestDaemon_RegisterRun verifies POST /runs creates daemon_runs + test_runs rows with runner=dogfood.
func TestDaemon_RegisterRun(t *testing.T) {
	t.Log("=== TestDaemon_RegisterRun ===")
	t.Log("Purpose: Verify POST /runs creates daemon_runs + test_runs rows")

	srv, db := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Sending POST /runs with pid=12345, env_profile=test-profile")
	body := map[string]any{
		"pid":         12345,
		"env_profile": "test-profile",
		"server_url":  "http://localhost:3002",
		"token":       "test-token",
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(server.URL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /runs: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("Step 2: Verifying status code = 201 (got %d)", resp.StatusCode)
	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	runID := result["id"]
	if runID == "" {
		t.Fatal("response missing 'id' field")
	}
	t.Logf("Step 3: Got run ID = %s", runID)

	t.Log("Step 4: Verifying daemon_runs table has 1 row")
	var daemonCount int
	db.RawDB().QueryRow("SELECT COUNT(*) FROM daemon_runs").Scan(&daemonCount)
	if daemonCount != 1 {
		t.Errorf("want 1 daemon run, got %d", daemonCount)
	}

	t.Log("Step 5: Verifying test_runs table has 1 row linked via daemon_run_id")
	var testCount int
	db.RawDB().QueryRow("SELECT COUNT(*) FROM test_runs WHERE daemon_run_id = ?", runID).Scan(&testCount)
	if testCount != 1 {
		t.Errorf("want 1 test run, got %d", testCount)
	}

	var testName, runner string
	db.RawDB().QueryRow("SELECT test_name, runner FROM test_runs WHERE daemon_run_id = ?", runID).Scan(&testName, &runner)
	t.Logf("  test_name=%q, runner=%q", testName, runner)
	if runner != "dogfood" {
		t.Errorf("want runner='dogfood', got %q", runner)
	}
	t.Log("✓ POST /runs creates rows in both daemon_runs and test_runs")
}

// TestDaemon_RegisterRun_MissingPID verifies POST /runs without pid returns 400.
func TestDaemon_RegisterRun_MissingPID(t *testing.T) {
	t.Log("=== TestDaemon_RegisterRun_MissingPID ===")
	t.Log("Purpose: Verify POST /runs with empty pid returns 400")

	srv, _ := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	body := map[string]any{"env_profile": "no-pid"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(server.URL+"/runs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /runs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("want 400 for missing pid, got %d", resp.StatusCode)
	}
	t.Logf("✓ POST /runs with empty pid returns 400 (got %d)", resp.StatusCode)
}

// TestDaemon_InsertEvent verifies POST /runs/:id/events inserts a row in run_events with correct kind, message, and details JSON.
func TestDaemon_InsertEvent(t *testing.T) {
	t.Log("=== TestDaemon_InsertEvent ===")
	t.Log("Purpose: Verify POST /runs/:id/events inserts into run_events with correct kind/details")

	srv, db := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Registering a run first")
	runID := registerTestRun(t, server.URL, 12346, "test-events")
	t.Logf("  run ID = %s", runID)

	t.Log("Step 2: Sending POST /runs/:id/events with kind='log', message='hello from daemon test'")
	eventBody := map[string]any{
		"kind":    "log",
		"message": "hello from daemon test",
		"details": map[string]any{"key": "value"},
	}
	b, _ := json.Marshal(eventBody)
	url := fmt.Sprintf("%s/runs/%s/events", server.URL, runID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /runs/%s/events: %v", runID, err)
	}
	defer resp.Body.Close()

	t.Logf("Step 3: Verifying status code = 201 (got %d)", resp.StatusCode)
	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}

	var eventResult map[string]any
	json.NewDecoder(resp.Body).Decode(&eventResult)
	seq := eventResult["seq"]
	t.Logf("  event seq = %v", seq)

	t.Log("Step 4: Querying run_events table directly")
	// Find the test_runs ID linked by daemon_run_id
	var testRunID int64
	db.RawDB().QueryRow("SELECT id FROM test_runs WHERE daemon_run_id = ?", runID).Scan(&testRunID)

	var kind, message string
	var details *string
	err = db.RawDB().QueryRow(
		"SELECT kind, message, details FROM run_events WHERE run_id = ? AND seq = ?",
		testRunID, seq,
	).Scan(&kind, &message, &details)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}

	t.Logf("Step 5: Verifying event fields")
	if kind != "log" {
		t.Errorf("want kind='log', got %q", kind)
	}
	if !strings.Contains(message, "hello from daemon test") {
		t.Errorf("message missing expected text, got %q", message)
	}
	if details == nil || !strings.Contains(*details, "value") {
		t.Errorf("details should contain the details map")
	}
	t.Log("✓ Event inserted with correct kind, message, and details")
}

// TestDaemon_MarkRunDone verifies PUT /runs/:id/done sets daemon_runs.status=done and test_runs.finished_at.
func TestDaemon_MarkRunDone(t *testing.T) {
	t.Log("=== TestDaemon_MarkRunDone ===")
	t.Log("Purpose: Verify PUT /runs/:id/done sets finished_at and status='done'")

	srv, db := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Registering a run first")
	runID := registerTestRun(t, server.URL, 12347, "test-done")
	t.Logf("  run ID = %s", runID)

	t.Logf("Step 2: Sending PUT /runs/%s/done with passed=true", runID)
	body := map[string]bool{"passed": true}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/runs/%s/done", server.URL, runID), bytes.NewReader(b))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /runs/%s/done: %v", runID, err)
	}
	defer resp.Body.Close()

	t.Logf("Step 3: Verifying status code = 200 (got %d)", resp.StatusCode)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	t.Log("Step 4: Verifying daemon_runs.status = 'done'")
	var status string
	db.RawDB().QueryRow("SELECT status FROM daemon_runs WHERE id = ?", runID).Scan(&status)
	if status != "done" {
		t.Errorf("want status='done', got %q", status)
	}

	t.Log("Step 5: Verifying test_runs has finished_at and passed=1")
	var finishedStr *string
	var passedInt *int
	db.RawDB().QueryRow(
		"SELECT finished_at, passed FROM test_runs WHERE daemon_run_id = ?", runID,
	).Scan(&finishedStr, &passedInt)
	if finishedStr == nil || *finishedStr == "" {
		t.Errorf("finished_at should be set, got nil/empty")
	} else {
		t.Logf("  finished_at = %s", *finishedStr)
	}
	if passedInt == nil || *passedInt != 1 {
		t.Errorf("want passed=1, got %v", passedInt)
	}
	t.Log("✓ PUT /runs/:id/done marks run as done in both tables")
}

// TestDaemon_Reap verifies POST /reap catches unfinished runs older than the daemon timeout and marks them with passed=3 (timeout).
func TestDaemon_Reap(t *testing.T) {
	t.Log("=== TestDaemon_Reap ===")
	t.Log("Purpose: Verify POST /reap catches unfinished runs older than timeout")

	srv, db := newDaemonTest(t)
	srv.timeout = 1 * time.Millisecond // very short timeout for testing
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Inserting a stale test_runs row (finished_at = NULL, started_at = 1 hour ago)")
	_, err := db.RawDB().Exec(
		`INSERT INTO test_runs (test_name, started_at) VALUES ('stale-reap-test', datetime('now', '-1 hour'))`,
	)
	if err != nil {
		t.Fatalf("insert stale run: %v", err)
	}

	t.Log("Step 2: Sending POST /reap")
	resp, err := http.Post(server.URL+"/reap", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /reap: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	t.Log("  /reap returned 200 OK")

	t.Log("Step 3: Verifying the stale run now has finished_at, passed=3 (timeout), reason='timed out'")
	var finishedStr *string
	var passed *int
	var reason *string
	db.RawDB().QueryRow(
		"SELECT finished_at, passed, reason FROM test_runs WHERE test_name = 'stale-reap-test'",
	).Scan(&finishedStr, &passed, &reason)

	if finishedStr == nil || *finishedStr == "" {
		t.Errorf("finished_at should be set after reap")
	} else {
		t.Logf("  finished_at = %s", *finishedStr)
	}
	if passed == nil || *passed != 3 {
		t.Errorf("want passed=3 (timeout), got %v", passed)
	}
	if reason == nil || *reason != "timed out" {
		t.Errorf("want reason='timed out', got %v", reason)
	}
	t.Log("✓ Stale run was reaped with passed=3, reason='timed out'")
}

// TestDaemon_Reap_SkipsRecentRuns verifies POST /reap does NOT touch runs started within the timeout window.
func TestDaemon_Reap_SkipsRecentRuns(t *testing.T) {
	t.Log("=== TestDaemon_Reap_SkipsRecentRuns ===")
	t.Log("Purpose: Verify POST /reap does NOT touch runs started within the timeout window")

	srv, db := newDaemonTest(t)
	srv.timeout = 1 * time.Hour // 1 hour timeout
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Inserting a recent run using RFC3339 format (started 1 minute ago)")
	now := time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano)
	_, err := srv.db.RawDB().Exec(
		`INSERT INTO test_runs (test_name, started_at) VALUES ('recent-run', ?)`, now,
	)
	if err != nil {
		t.Fatalf("insert recent run: %v", err)
	}

	t.Log("Step 2: Sending POST /reap")
	resp, err := http.Post(server.URL+"/reap", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /reap: %v", err)
	}
	defer resp.Body.Close()

	t.Log("Step 3: Verifying the recent run was NOT touched (finished_at is still NULL)")
	var finishedStr *string
	db.RawDB().QueryRow(
		"SELECT finished_at FROM test_runs WHERE test_name = 'recent-run'",
	).Scan(&finishedStr)
	if finishedStr != nil {
		t.Errorf("recent run should NOT be reaped (finished_at should be nil), got %s", *finishedStr)
	} else {
		t.Log("  finished_at is NULL — recent run correctly skipped")
	}
	t.Log("✓ POST /reap skips runs within the timeout window")
}

// TestDaemon_Status verifies GET /status returns JSON with active_runs and tracked_resources counts.
func TestDaemon_Status(t *testing.T) {
	t.Log("=== TestDaemon_Status ===")
	t.Log("Purpose: Verify GET /status returns JSON with run counts")

	srv, _ := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	t.Log("Step 1: Registering 2 test runs")
	registerTestRun(t, server.URL, 12348, "status-test-1")
	registerTestRun(t, server.URL, 12349, "status-test-2")

	t.Log("Step 2: Sending GET /status")
	resp, err := http.Get(server.URL + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("Step 3: Verifying status code = 200 (got %d)", resp.StatusCode)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var status struct {
		Status           string `json:"status"`
		ActiveRuns       int    `json:"active_runs"`
		TrackedResources int    `json:"tracked_resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode status JSON: %v", err)
	}
	t.Logf("  status=%q, active_runs=%d, tracked_resources=%d", status.Status, status.ActiveRuns, status.TrackedResources)
	if status.Status != "running" {
		t.Errorf("want status='running', got %q", status.Status)
	}
	if status.ActiveRuns != 2 {
		t.Errorf("want 2 active runs, got %d", status.ActiveRuns)
	}
	t.Log("✓ GET /status returns daemon status with run counts")
}

// TestDaemon_InsertEvent_RequiresKind verifies POST /runs/:id/events without kind field returns 400.
func TestDaemon_InsertEvent_RequiresKind(t *testing.T) {
	t.Log("=== TestDaemon_InsertEvent_RequiresKind ===")
	t.Log("Purpose: Verify POST /runs/:id/events without 'kind' returns 400")

	srv, _ := newDaemonTest(t)
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	runID := registerTestRun(t, server.URL, 12350, "test-no-kind")

	t.Log("Sending event without 'kind' field")
	body := map[string]any{"message": "no kind here"}
	b, _ := json.Marshal(body)
	resp, err := http.Post(fmt.Sprintf("%s/runs/%s/events", server.URL, runID), "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST /runs/%s/events: %v", runID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("want 400 for missing kind, got %d", resp.StatusCode)
	}
	t.Logf("✓ Missing kind returns 400 (got %d)", resp.StatusCode)
}
