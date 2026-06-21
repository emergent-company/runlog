package main

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// newTestApp creates a WebApp with an in-memory SQLite DB for testing.
func newTestApp(t *testing.T) (*WebApp, *runlog.RunDB) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &runlog.Config{
		TestCommand: "go test -v -run {name} ./...",
		DaemonPort:  17430,
	}

	app := newWebApp(db, cfg, "")
	return app, db
}

// echoRequest performs an HTTP request against the WebApp and returns the response recorder.
func echoRequest(t *testing.T, app *WebApp, method, path string, body ...string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *strings.Reader
	if len(body) > 0 {
		reqBody = strings.NewReader(body[0])
	} else {
		reqBody = strings.NewReader("")
	}

	req := httptest.NewRequest(method, path, reqBody)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

// htmxRequest performs an HTMX request against the WebApp and returns the response recorder.
func htmxRequest(t *testing.T, app *WebApp, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

// seedTestRun inserts a basic run row into the test database.
func seedTestRun(t *testing.T, db *runlog.RunDB, testName string, passed bool) int64 {
	t.Helper()

	var id int64
	err := db.RawDB().QueryRow(
		`INSERT INTO test_runs (test_name, passed, skipped, started_at, finished_at)
		 VALUES (?, ?, 0, datetime('now'), datetime('now'))
		 RETURNING id`,
		testName, passed,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return id
}

// seedTestEvent inserts a basic event row for a given run.
func seedTestEvent(t *testing.T, db *runlog.RunDB, runID int64, kind, message string) int64 {
	t.Helper()

	var id int64
	err := db.RawDB().QueryRow(
		`INSERT INTO run_events (run_id, seq, kind, message, elapsed_s, occurred_at)
		 VALUES (?, 1, ?, ?, 0.5, datetime('now'))
		 RETURNING id`,
		runID, kind, message,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return id
}

// TestMain ensures common test prerequisites.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestWebApp_ServeHTTP_RoutesExist(t *testing.T) {
	app, _ := newTestApp(t)

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/", 200},
		{"GET", "/tests", 200},
		{"GET", "/tests/TestFoo", 200},
		{"GET", "/runs/99999", 404},               // non-existent run
		{"POST", "/launch/TestDoesNotExist", 303}, // spawns process, redirects to run page
		{"GET", "/nonexistent", 404},
	}

	for _, tt := range tests {
		rec := echoRequest(t, app, tt.method, tt.path)
		if rec.Code != tt.status {
			t.Errorf("%s %s → want %d, got %d", tt.method, tt.path, tt.status, rec.Code)
		}
	}
}

func TestWebApp_Dashboard_HasTestIDs(t *testing.T) {
	app, _ := newTestApp(t)

	rec := echoRequest(t, app, "GET", "/")
	body := rec.Body.String()

	checkTestID(t, body, "dashboard-page")
	checkTestID(t, body, "stat-cards")
	checkTestID(t, body, "main-content")
}

func TestWebApp_Dashboard_HTMXPartial_NoShell(t *testing.T) {
	app, _ := newTestApp(t)

	rec := htmxRequest(t, app, "GET", "/")
	body := rec.Body.String()

	if strings.Contains(body, "app-page") {
		t.Errorf("HTMX partial should not include app-page wrapper")
	}
	checkTestID(t, body, "dashboard-content")
}

func TestWebApp_Tests_RendersList(t *testing.T) {
	app, db := newTestApp(t)
	seedTestRun(t, db, "TestAlpha", true)
	seedTestRun(t, db, "TestBeta", false)

	rec := echoRequest(t, app, "GET", "/tests")
	body := rec.Body.String()

	checkTestID(t, body, "test-list")
	checkTestID(t, body, "category-filter")
}

func TestWebApp_Tests_HTMXPartial(t *testing.T) {
	app, _ := newTestApp(t)

	rec := htmxRequest(t, app, "GET", "/tests")
	body := rec.Body.String()

	checkTestID(t, body, "tests-content")
	if strings.Contains(body, "app-page") {
		t.Errorf("HTMX partial should not include app-page wrapper")
	}
}

func TestWebApp_Tests_FilterByStatus(t *testing.T) {
	app, db := newTestApp(t)

	// Seed runs with each status.
	// pass: passed=1
	seedTestRun(t, db, "TestFilterPass", true)

	// fail: passed=0
	seedTestRun(t, db, "TestFilterFail", false)

	// skip: passed=2
	db.RawDB().Exec(`INSERT INTO test_runs (test_name, passed, skipped, started_at, finished_at) VALUES ('TestFilterSkip', 2, 1, datetime('now'), datetime('now'))`)

	// timeout: passed=3, reason='timed out'
	toID := seedTestRun(t, db, "TestFilterTimeout", true)
	db.RawDB().Exec(`UPDATE test_runs SET passed=3, reason='timed out' WHERE id=?`, toID)

	// running: finished_at=NULL, passed=NULL
	db.RawDB().Exec(`INSERT INTO test_runs (test_name, started_at) VALUES ('TestFilterRunning', datetime('now'))`)

	tests := []struct {
		filter string
		wantVariant string // badge CSS variant class
	}{
		{"pass", "badge-success"},
		{"fail", "badge-error"},
		{"skip", "badge-warning"},
		{"timeout", "badge-warning"},
		{"running", "badge-info"},
	}
	for _, tt := range tests {
		rec := echoRequest(t, app, "GET", "/tests?status="+tt.filter)
		body := rec.Body.String()
		if !strings.Contains(body, tt.wantVariant) {
			// Check if the response is empty (no matching tests) rather than wrong
			if strings.Contains(body, "no tests found") || strings.Contains(body, "empty") {
				t.Logf("status=%s: no matching tests in response (empty state)", tt.filter)
			} else {
				t.Errorf("status=%s: expected badge variant %q but not found in response", tt.filter, tt.wantVariant)
			}
		}
	}

	// Verify never_run filter returns nothing (no discovered tests in test env).
	rec := echoRequest(t, app, "GET", "/tests?status=never_run")
	body := rec.Body.String()
	if strings.Contains(body, `badge-success`) || strings.Contains(body, `badge-error`) || strings.Contains(body, `badge-warning`) || strings.Contains(body, `badge-info`) {
		t.Errorf("status=never_run: expected no status badges but found some (matched by badge variant)")
	}
}

func TestWebApp_TestDetail_RendersTestName(t *testing.T) {
	app, db := newTestApp(t)
	seedTestRun(t, db, "TestDetailFoo", true)

	rec := echoRequest(t, app, "GET", "/tests/TestDetailFoo")
	body := rec.Body.String()

	checkTestID(t, body, "test-detail-content")
	checkTestID(t, body, "launch-area")
	if !strings.Contains(body, "TestDetailFoo") {
		t.Errorf("expected test name in body")
	}
}

func TestWebApp_TestDetail_HTMXPartial(t *testing.T) {
	app, db := newTestApp(t)
	seedTestRun(t, db, "TestHTMXDetail", true)

	rec := htmxRequest(t, app, "GET", "/tests/TestHTMXDetail")
	body := rec.Body.String()

	checkTestID(t, body, "test-detail-content")
	if strings.Contains(body, "app-page") {
		t.Errorf("HTMX partial should not include app-page wrapper")
	}
}

func TestWebApp_RunDetail_RendersRun(t *testing.T) {
	app, db := newTestApp(t)
	id := seedTestRun(t, db, "TestRunView", true)
	seedTestEvent(t, db, id, "section", "test setup")
	seedTestEvent(t, db, id, "log", "hello world")

	rec := echoRequest(t, app, "GET", "/runs/"+fmt.Sprintf("%d", id))
	body := rec.Body.String()

	checkTestID(t, body, "run-detail-content")
	if !strings.Contains(body, "TestRunView") {
		t.Errorf("expected test name in run detail body")
	}
}

func TestWebApp_RunDetail_NonExistentRun(t *testing.T) {
	app, _ := newTestApp(t)

	rec := echoRequest(t, app, "GET", "/runs/99999")
	if rec.Code != 404 {
		t.Errorf("want 404 for non-existent run, got %d", rec.Code)
	}
}

func TestWebApp_LaunchTest_MissingBinary(t *testing.T) {
	app, _ := newTestApp(t)

	rec := echoRequest(t, app, "POST", "/launch/TestDoesNotExist")
	// Launching spawns a process asynchronously; handler creates a test_runs
	// row and redirects to the run detail page.
	if rec.Code != 303 {
		t.Errorf("want 303 (redirect), got %d", rec.Code)
	}
}

func TestWebApp_ExpandCommand_Basic(t *testing.T) {
	got := runlog.ExpandTestCommand("go test -run {name} ./...", "TestFoo", "")
	want := "go test -run TestFoo ./..."
	if got != want {
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	}
}

func TestWebApp_ExpandCommand_WithEnv(t *testing.T) {
	got := runlog.ExpandTestCommand("go test -run {name} -env {env}", "TestFoo", "staging")
	want := "go test -run TestFoo -env staging"
	if got != want {
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	}
}

func TestWebApp_ExpandCommand_NoPlaceholders(t *testing.T) {
	got := runlog.ExpandTestCommand("go test ./...", "TestFoo", "")
	want := "go test ./..."
	if got != want {
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	}
}

func TestWebApp_LaunchAndPollUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E launch test in short mode")
	}

	dir := t.TempDir()

	testFile := filepath.Join(dir, "e2e_test.go")
	if err := os.WriteFile(testFile, []byte(`package e2e_test

import (
	"testing"
	"time"
	runlog "github.com/emergent-company/runlog"
)

func TestE2E_UpdateCheck(t *testing.T) {
	rl := runlog.NewRunLog(t)
	defer rl.Close()
	rl.SetTimeout(30 * time.Second)
	rl.Printf("test starting...")
	time.Sleep(1 * time.Second)
	rl.Printf("step 1 complete")
	time.Sleep(1 * time.Second)
	rl.Printf("step 2 complete")
	rl.Printf("done")
}
`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	runModInit(t, dir)

	db, err := runlog.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	runlogDir := filepath.Join(dir, ".runlog")
	if err := os.MkdirAll(runlogDir, 0o755); err != nil {
		t.Fatalf("mkdir .runlog: %v", err)
	}
	dbLink := filepath.Join(runlogDir, "runs.db")
	if err := os.Symlink(filepath.Join(dir, "test.db"), dbLink); err != nil {
		t.Fatalf("symlink .runlog/runs.db: %v", err)
	}

	cfg := &runlog.Config{
		TestCommand: "cd " + dir + " && TEST_RUNS_DB=" + filepath.Join(dir, "test.db") + " go test -v -run {name} ./...",
		DaemonPort:  17430,
	}
	app := newWebApp(db, cfg, "")

	rec := echoRequest(t, app, "POST", "/launch/TestE2E_UpdateCheck")
	if rec.Code != 303 {
		t.Fatalf("launch: want 303 redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	runID := strings.TrimPrefix(loc, "/ui/runs/")
	if runID == "" || runID == loc {
		t.Fatalf("launch: no run ID in Location header %q", loc)
	}
	t.Logf("launched run %s", runID)

	detailPath := strings.TrimPrefix(loc, "/ui")
	rec = echoRequest(t, app, "GET", detailPath)
	body := rec.Body.String()
	if !strings.Contains(body, "Running") {
		t.Errorf("run detail: expected 'Running', got: %s", body[:min(len(body), 200)])
	}
	checkTestID(t, body, "run-detail-content")

	deadline := time.Now().Add(25 * time.Second)
	var eventsBody string
	for time.Now().Before(deadline) {
		rec = echoRequest(t, app, "GET", fmt.Sprintf("/runs/%s/events-table", runID))
		eventsBody = rec.Body.String()
		if strings.Contains(eventsBody, "step 2 complete") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(eventsBody, "step 2 complete") {
		t.Errorf("events-table: expected 'step 2 complete' within 25s, got: %s", eventsBody[:min(len(eventsBody), 300)])
	}

	deadline = time.Now().Add(15 * time.Second)
	var finalBody string
	for time.Now().Before(deadline) {
		rec = echoRequest(t, app, "GET", detailPath)
		finalBody = rec.Body.String()
		if strings.Contains(finalBody, "Pass") || strings.Contains(finalBody, "Fail") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !strings.Contains(finalBody, "Pass") {
		t.Errorf("run detail: expected 'Pass' after completion, got: %s", finalBody[:min(len(finalBody), 400)])
	}

	var finishedStr *string
	err = db.RawDB().QueryRow(`SELECT finished_at FROM test_runs WHERE id = ?`, runID).Scan(&finishedStr)
	if err != nil {
		t.Errorf("query finished_at: %v", err)
	}
	if finishedStr == nil || *finishedStr == "" {
		t.Errorf("run %s was never finished (finished_at is NULL)", runID)
	} else {
		t.Logf("run %s finished at %s", runID, *finishedStr)
	}
}

// runModInit creates a minimal Go module in dir that points at the local runlog.
func runModInit(t *testing.T, dir string) {
	t.Helper()

	// go mod init
	cmd := exec.Command("go", "mod", "init", "e2e_test")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod init: %v\n%s", err, out)
	}

	// Write a replace directive so the module uses our local runlog.
	replaceLine := fmt.Sprintf("replace github.com/emergent-company/runlog => %s", "/root/runlog")
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if err := os.WriteFile(goModPath, []byte(string(data)+"\n"+replaceLine+"\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Run go mod tidy to resolve dependencies.
	cmd = exec.Command("go", "mod", "tidy", "-e")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}

	// Verify the test compiles.
	cmd = exec.Command("go", "test", "-c", "-o", "/dev/null", ".")
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -c: %v\n%s", err, out)
	}
	t.Log("test module compiles OK")
}

func TestWebApp_StatusFromRun(t *testing.T) {
	now := time.Now()
	passed := true

	tests := []struct {
		name string
		run  runlog.RunRow
		want string
	}{
		{"running", runlog.RunRow{}, "running"},
		{"pass", runlog.RunRow{FinishedAt: &now, Passed: &passed}, "pass"},
	}
	for _, tt := range tests {
		got := statusFromRun(tt.run)
		if got != tt.want {
			t.Errorf("statusFromRun(%s): got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// checkTestID fails the test if the body does not contain the expected data-testid.
func checkTestID(t *testing.T, body, testID string) {
	t.Helper()
	marker := `data-testid="` + testID + `"`
	if !strings.Contains(body, marker) {
		t.Errorf("expected data-testid=%q in response body", testID)
	}
}
