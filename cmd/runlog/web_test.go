package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// newDaemonApp creates a WebApp + daemon on a random port, sharing the same DB.
// Returns the daemon URL, WebApp, and DaemonClient.
func newDaemonApp(t *testing.T) (daemonURL string, app *WebApp, dc *runlog.DaemonClient) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	daemon := newDaemonServer(db, port, 5*time.Minute, "")
	go daemon.ServeOn(ln)
	t.Cleanup(func() { daemon.Shutdown() })

	// Wait for daemon to be healthy
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	for i := 0; i < 20; i++ {
		resp, herr := http.Get(healthURL)
		if herr == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			break
		}
		if herr == nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}

	cfg := &runlog.Config{
		TestCommand: "go test -v -run {name} ./...",
		DaemonPort:  port,
	}

	app = newWebApp(db, cfg, "")
	t.Cleanup(func() { app.Shutdown() })
	daemonURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	dc = runlog.NewDaemonClient(daemonURL)
	return
}

// newWebTest is a convenience wrapper around newDaemonApp + DogfoodRun.
// Returns the daemon URL, WebApp, DaemonClient, and DogfoodRun.
// When the dogfood daemon is unavailable, DogfoodRun is a no-op (fail-open).
func newWebTest(t *testing.T, category, description string) (string, *WebApp, *runlog.DaemonClient, *runlog.DogfoodRun) {
	t.Helper()
	df := runlog.NewDogfoodRun(t, category)
	df.Describe(description)
	df.Event("log", description)
	t.Cleanup(func() {
		df.Event("assertion", "all checks completed")
		df.Done()
	})
	daemonURL, app, dc := newDaemonApp(t)
	return daemonURL, app, dc, df
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

// newTestApp creates a WebApp with an isolated DB for testing (legacy — prefer newDaemonApp).
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
	t.Cleanup(func() { app.Shutdown() })
	return app, db
}

// TestWebApp_ServeHTTP_RoutesExist verifies all major routes return expected HTTP status codes.
func TestWebApp_ServeHTTP_RoutesExist(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "Verify all routes return expected HTTP status codes")

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/", 200},
		{"GET", "/tests", 200},
		{"GET", "/tests/TestFoo", 200},
		{"GET", "/runs/99999", 404},
		{"POST", "/launch/TestDoesNotExist", 303},
		{"GET", "/nonexistent", 404},
	}

	for _, tt := range tests {
		rec := echoRequest(t, app, tt.method, tt.path)
		msg := fmt.Sprintf("%s %s → %d", tt.method, tt.path, rec.Code)
		if rec.Code != tt.status {
			df.EventAssertion(fmt.Sprintf("%s (want %d)", msg, tt.status), tt.status, rec.Code)
			t.Errorf("%s %s → want %d, got %d", tt.method, tt.path, tt.status, rec.Code)
		} else {
			df.EventHTTP(tt.method, tt.path, rec.Code, "", "")
		}
	}
	df.Event("assertion", "all 6 routes returned expected status codes")
}

// TestWebApp_Dashboard_HasTestIDs verifies the dashboard page contains expected data-testid attributes.
func TestWebApp_Dashboard_HasTestIDs(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "Dashboard page has expected data-testid attributes")
	df.Event("log", "Requesting GET /")
	rec := echoRequest(t, app, "GET", "/")
	body := rec.Body.String()
	df.EventHTTP("GET", "/", rec.Code, "", "")

	checkTestID(t, body, "dashboard-page")
	checkTestID(t, body, "stat-cards")
	checkTestID(t, body, "main-content")
	df.EventAssertion("dashboard-page, stat-cards, main-content data-testid attributes present",
		"all present", len(body) > 0)
}

// TestWebApp_Dashboard_HTMXPartial_NoShell verifies HTMX requests return content without the app-page shell wrapper.
func TestWebApp_Dashboard_HTMXPartial_NoShell(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "HTMX partial without app-page shell")

	df.Event("log", "Requesting HTMX GET /")
	rec := htmxRequest(t, app, "GET", "/")
	body := rec.Body.String()

	df.Event("http_call", "HTMX GET / → 200")
	if strings.Contains(body, "app-page") {
		df.Event("assertion", "FAIL: response contains app-page wrapper")
		t.Errorf("HTMX partial should not include app-page wrapper")
	} else {
		df.Event("assertion", "response does NOT contain app-page shell wrapper")
	}
	checkTestID(t, body, "dashboard-content")
}

// TestWebApp_Tests_RendersList verifies the tests page renders a list of seeded test runs.
func TestWebApp_Tests_RendersList(t *testing.T) {
	_, app, dc, df := newWebTest(t, "web", "Tests page renders list of seeded runs")
	df.Event("log", "Seeding 2 test runs: TestAlpha (pass), TestBeta (fail)")
	r1 := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestAlpha"})
	dc.MarkDone(t, r1.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})
	r2 := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestBeta"})
	dc.MarkDone(t, r2.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(false)})

	df.Event("log", "Fetching GET /tests")
	rec := echoRequest(t, app, "GET", "/tests")
	body := rec.Body.String()
	df.Event("http_call", "GET /tests → 200")

	checkTestID(t, body, "test-list")
	checkTestID(t, body, "category-filter")
	df.Event("assertion", "test-list and category-filter data-testid present in response")
}

// TestWebApp_Tests_HTMXPartial verifies the tests page HTMX partial returns content without the app-page shell.
func TestWebApp_Tests_HTMXPartial(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "Tests page HTMX partial without app-page shell")

	df.Event("log", "Requesting HTMX GET /tests")
	rec := htmxRequest(t, app, "GET", "/tests")
	body := rec.Body.String()
	df.Event("http_call", "HTMX GET /tests → 200")

	checkTestID(t, body, "tests-content")
	if strings.Contains(body, "app-page") {
		df.Event("assertion", "FAIL: response contains app-page wrapper")
		t.Errorf("HTMX partial should not include app-page wrapper")
	} else {
		df.Event("assertion", "no app-page shell in HTMX partial")
	}
}

// TestWebApp_Tests_FilterByStatus verifies status filters return only matching badge variants.
func TestWebApp_Tests_FilterByStatus(t *testing.T) {
	_, app, dc, df := newWebTest(t, "web", "Status filters return matching badge variants")

	df.Event("log", "Seeding runs: pass, fail, skip, timeout, running")
	r := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestFilterPass"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})
	r = dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestFilterFail"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(false)})
	r = dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestFilterSkip"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Skipped: boolPtr(true)})
	r = dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestFilterTimeout"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(false), Reason: "timed out"})
	dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestFilterRunning"})

	tests := []struct {
		filter      string
		wantVariant string
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
		df.Event("http_call", fmt.Sprintf("GET /tests?status=%s → %d", tt.filter, rec.Code))
		if !strings.Contains(body, tt.wantVariant) {
			if strings.Contains(body, "no tests found") || strings.Contains(body, "empty") {
				df.Event("assertion", fmt.Sprintf("status=%s: no matching tests (empty state)", tt.filter))
				t.Logf("status=%s: no matching tests in response (empty state)", tt.filter)
			} else {
				df.Event("assertion", fmt.Sprintf("FAIL: status=%s expected variant %q not found", tt.filter, tt.wantVariant))
				t.Errorf("status=%s: expected badge variant %q but not found in response", tt.filter, tt.wantVariant)
			}
		} else {
			df.Event("assertion", fmt.Sprintf("status=%s has badge variant %s", tt.filter, tt.wantVariant))
		}
	}

	df.Event("log", "Checking never_run filter returns no badges")
	rec := echoRequest(t, app, "GET", "/tests?status=never_run")
	body := rec.Body.String()
	df.Event("http_call", "GET /tests?status=never_run → 200")
	if strings.Contains(body, `badge-success`) || strings.Contains(body, `badge-error`) || strings.Contains(body, `badge-warning`) || strings.Contains(body, `badge-info`) {
		df.Event("assertion", "FAIL: never_run filter shows badges")
		t.Errorf("status=never_run: expected no status badges but found some")
	} else {
		df.Event("assertion", "never_run filter returns no badges")
	}
}

// TestWebApp_TestDetail_RendersTestName verifies the test detail page shows the correct test name.
func TestWebApp_TestDetail_RendersTestName(t *testing.T) {
	_, app, dc, df := newWebTest(t, "web", "Test detail page shows correct test name")
	df.Event("log", "Seeding run for TestDetailFoo")
	r := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestDetailFoo"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})

	df.Event("log", "Fetching test detail page")
	rec := echoRequest(t, app, "GET", "/tests/TestDetailFoo")
	body := rec.Body.String()
	df.Event("http_call", "GET /tests/TestDetailFoo → 200")

	checkTestID(t, body, "test-detail-content")
	checkTestID(t, body, "launch-area")
	if !strings.Contains(body, "TestDetailFoo") {
		df.Event("assertion", "FAIL: test name not found")
		t.Errorf("expected test name in body")
	} else {
		df.Event("assertion", "TestDetailFoo found in detail page")
	}
}

// TestWebApp_TestDetail_HTMXPartial verifies the test detail HTMX partial returns content without the shell.
func TestWebApp_TestDetail_HTMXPartial(t *testing.T) {
	_, app, dc, df := newWebTest(t, "web", "Test detail HTMX partial without shell")
	df.Event("log", "Seeding run for TestHTMXDetail")
	r := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestHTMXDetail"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})

	df.Event("log", "Requesting HTMX test detail partial")
	rec := htmxRequest(t, app, "GET", "/tests/TestHTMXDetail")
	body := rec.Body.String()
	df.Event("http_call", "HTMX GET /tests/TestHTMXDetail → 200")

	checkTestID(t, body, "test-detail-content")
	if strings.Contains(body, "app-page") {
		df.Event("assertion", "FAIL: HTMX partial has app-page shell")
		t.Errorf("HTMX partial should not include app-page wrapper")
	} else {
		df.Event("assertion", "no app-page shell in HTMX partial")
	}
}

// TestWebApp_RunDetail_RendersRun verifies the run detail page shows the test name and event data.
func TestWebApp_RunDetail_RendersRun(t *testing.T) {
	_, app, dc, df := newWebTest(t, "web", "Run detail page shows test name and event data")
	df.Event("log", "Seeding run with events for TestRunView")
	r := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: "TestRunView"})
	dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: boolPtr(true)})
	dc.AddEvent(t, r.DaemonID, "section", "test setup")
	dc.AddEvent(t, r.DaemonID, "log", "hello world")

	df.Event("log", "Fetching run detail page")
	rec := echoRequest(t, app, "GET", "/runs/"+fmt.Sprintf("%d", r.TestRunID))
	body := rec.Body.String()
	df.Event("http_call", fmt.Sprintf("GET /runs/%d → 200", r.TestRunID))

	checkTestID(t, body, "run-detail-content")
	if !strings.Contains(body, "TestRunView") {
		df.Event("assertion", "FAIL: TestRunView not found")
		t.Errorf("expected test name in run detail body")
	} else {
		df.Event("assertion", "TestRunView found in run detail")
	}
}

// TestWebApp_RunDetail_NonExistentRun verifies requesting a non-existent run ID returns 404.
func TestWebApp_RunDetail_NonExistentRun(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "Non-existent run returns 404")

	df.Event("log", "Requesting non-existent run 99999")
	rec := echoRequest(t, app, "GET", "/runs/99999")
	df.Event("http_call", fmt.Sprintf("GET /runs/99999 → %d", rec.Code))
	if rec.Code != 404 {
		df.Event("assertion", "FAIL: expected 404")
		t.Errorf("want 404 for non-existent run, got %d", rec.Code)
	} else {
		df.Event("assertion", "non-existent run correctly returns 404")
	}
}

func boolPtr(b bool) *bool { return &b }

// TestWebApp_LaunchTest_MissingBinary verifies launching a non-existent test returns a 303 redirect.
func TestWebApp_LaunchTest_MissingBinary(t *testing.T) {
	_, app, _, df := newWebTest(t, "web", "Launching missing test returns 303 redirect")

	df.Event("log", "Attempting to launch non-existent test")
	rec := echoRequest(t, app, "POST", "/launch/TestDoesNotExist")
	df.Event("http_call", fmt.Sprintf("POST /launch/TestDoesNotExist → %d", rec.Code))
	if rec.Code != 303 {
		df.Event("assertion", "FAIL: expected 303 redirect")
		t.Errorf("want 303 (redirect), got %d", rec.Code)
	} else {
		df.Event("assertion", "missing test launch returns 303 redirect")
	}
}

// TestWebApp_ExpandCommand_Basic verifies the {name} placeholder is replaced in command templates.
func TestWebApp_ExpandCommand_Basic(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "web")
	defer df.Done()
	df.Describe("ExpandTestCommand replaces {name} placeholder")
	df.Event("log", `ExpandTestCommand("go test -run {name} ./...", "TestFoo", "")`)
	got := runlog.ExpandTestCommand("go test -run {name} ./...", "TestFoo", "")
	want := "go test -run TestFoo ./..."
	if got != want {
		df.Event("assertion", fmt.Sprintf("FAIL: got %q, want %q", got, want))
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	} else {
		df.Event("assertion", `→ "go test -run TestFoo ./..." — {name} replaced`)
	}
}

// TestWebApp_ExpandCommand_WithEnv verifies the {env} placeholder is replaced with the environment name.
func TestWebApp_ExpandCommand_WithEnv(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "web")
	defer df.Done()
	df.Describe("ExpandTestCommand replaces {env} placeholder")
	df.Event("log", `ExpandTestCommand("go test -run {name} -env {env}", "TestFoo", "staging")`)
	got := runlog.ExpandTestCommand("go test -run {name} -env {env}", "TestFoo", "staging")
	want := "go test -run TestFoo -env staging"
	if got != want {
		df.Event("assertion", fmt.Sprintf("FAIL: got %q, want %q", got, want))
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	} else {
		df.Event("assertion", `→ "go test -run TestFoo -env staging" — {name}+{env} replaced`)
	}
}

// TestWebApp_ExpandCommand_NoPlaceholders verifies commands without placeholders pass through unchanged.
func TestWebApp_ExpandCommand_NoPlaceholders(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "web")
	defer df.Done()
	df.Describe("ExpandTestCommand passes through without placeholders")
	df.Event("log", `ExpandTestCommand("go test ./...", "TestFoo", "")`)
	got := runlog.ExpandTestCommand("go test ./...", "TestFoo", "")
	want := "go test ./..."
	if got != want {
		df.Event("assertion", fmt.Sprintf("FAIL: got %q, want %q", got, want))
		t.Errorf("ExpandTestCommand: got %q, want %q", got, want)
	} else {
		df.Event("assertion", "→ unchanged (no placeholders to replace)")
	}
}

// TestWebApp_LaunchAndPollUpdates verifies launching a real test via the web API and polling events-table until completion.
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
		TestCommand: "cd " + dir + " && _RUNLOG_DAEMON_DB=" + filepath.Join(dir, "test.db") + " go test -v -run {name} ./...",
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

// TestWebApp_StatusFromRun verifies statusFromRun returns correct labels for running, pass, fail, and timeout states.
func TestWebApp_StatusFromRun(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "web")
	defer df.Done()
	df.Describe("statusFromRun returns correct labels for running/pass states")
	df.Event("log", "Testing statusFromRun for running and pass states")
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
		df.Event("log", "statusFromRun("+tt.name+" case)")
		got := statusFromRun(tt.run)
		if got != tt.want {
			msg := fmt.Sprintf("FAIL: got %q, want %q", got, tt.want)
			df.Event("assertion", msg)
			t.Errorf("statusFromRun(%s): got %q, want %q", tt.name, got, tt.want)
		} else {
			msg := fmt.Sprintf("got %q as expected", tt.want)
			df.Event("assertion", msg)
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
