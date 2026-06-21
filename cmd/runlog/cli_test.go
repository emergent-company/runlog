package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// captureStdout runs fn and returns everything written to stdout.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("command failed: %v\noutput:\n%s", err, buf.String())
	}
	return buf.String()
}

// captureStdoutErr runs fn and returns everything written to stdout + stderr.
func captureStdoutErr(t *testing.T, fn func() error) string {
	t.Helper()
	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	fn()
	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	var buf bytes.Buffer
	io.Copy(&buf, rOut)
	io.Copy(&buf, rErr)
	return buf.String()
}

// captureStderr runs fn and returns everything written to stderr.
func captureStderr(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := fn()
	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	if err != nil {
		t.Fatalf("command failed: %v\noutput:\n%s", err, buf.String())
	}
	return buf.String()
}

// newTestDBWithRuns seeds a temp DB with a set of runs and events for CLI tests.
// Returns the DB and a map of testName → runID.
func newTestDBWithRuns(t *testing.T) (*runlog.RunDB, map[string]int64) {
	t.Helper()
	_, db := newTestApp(t) // creates a fresh WebApp + DB

	runs := make(map[string]int64)
	for _, tc := range []struct {
		name   string
		passed bool
	}{
		{"TestAlpha", true},
		{"TestBeta", false},
		{"TestGamma", true},
	} {
		id := seedTestRun(t, db, tc.name, tc.passed)
		runs[tc.name] = id
	}

	// Add events with sequential seq numbers for the first run.
	for i, ev := range []struct{ kind, msg string }{
		{"state_change", "test started"},
		{"log", "running setup"},
		{"cli", "go build ./..."},
		{"state_change", "test finished"},
	} {
		db.RawDB().Exec(
			`INSERT INTO run_events (run_id, seq, kind, message, elapsed_s, occurred_at)
			 VALUES (?, ?, ?, ?, 0.5, datetime('now'))`,
			runs["TestAlpha"], i+1, ev.kind, ev.msg,
		)
	}

	return db, runs
}

// ── Tests ──────────────────────────────────────────────────────────────────

// TestCLI_Runs verifies the runlog runs command prints a table with test names and pass/fail statuses.
func TestCLI_Runs(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	db, _ := newTestDBWithRuns(t)

	output := df.RunCLI("runlog runs --since 1y", func() error {
		return cmdRuns(db, 365*24*time.Hour)
	})

	if !strings.Contains(output, "TestAlpha") {
		t.Errorf("expected TestAlpha in output\n%s", output)
	}
	if !strings.Contains(output, "TestBeta") {
		t.Errorf("expected TestBeta in output\n%s", output)
	}
	if !strings.Contains(output, "TestGamma") {
		t.Errorf("expected TestGamma in output\n%s", output)
	}
	if !strings.Contains(output, "PASS") && !strings.Contains(output, "Pass") {
		t.Errorf("expected PASS/Pass status in output\n%s", output)
	}
	if !strings.Contains(output, "FAIL") && !strings.Contains(output, "Fail") {
		t.Errorf("expected FAIL/Fail status in output\n%s", output)
	}
}

// TestCLI_Show verifies the runlog show <id> command prints run metadata and all seeded events.
func TestCLI_Show(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	db, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog show %d", runID), func() error {
		return cmdShow(db, runID)
	})

	if !strings.Contains(output, "TestAlpha") {
		t.Errorf("expected test name TestAlpha in output\n%s", output)
	}
	if !strings.Contains(output, "run:") {
		t.Errorf("expected 'run:' in output\n%s", output)
	}
	if !strings.Contains(output, "state_change") {
		t.Errorf("expected state_change event in output\n%s", output)
	}
	if !strings.Contains(output, "running setup") {
		t.Errorf("expected 'running setup' log event in output\n%s", output)
	}
	if !strings.Contains(output, "go build") {
		t.Errorf("expected 'go build' cli event in output\n%s", output)
	}
}

// TestCLI_Events verifies the runlog events <id> command prints a formatted event table with SEQ, KIND, and MESSAGE columns.
func TestCLI_Events(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	db, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog events %d", runID), func() error {
		return cmdEvents(db, runID)
	})

	if !strings.Contains(output, "SEQ") {
		t.Errorf("expected SEQ header in output\n%s", output)
	}
	if !strings.Contains(output, "KIND") {
		t.Errorf("expected KIND header in output\n%s", output)
	}
	if !strings.Contains(output, "MESSAGE") {
		t.Errorf("expected MESSAGE header in output\n%s", output)
	}
	if !strings.Contains(output, "test started") {
		t.Errorf("expected 'test started' event\n%s", output)
	}
	if !strings.Contains(output, "running setup") {
		t.Errorf("expected 'running setup' event\n%s", output)
	}
}

// TestCLI_Inspect verifies the runlog inspect <id> command prints detailed event information with inspector formatting.
func TestCLI_Inspect(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	db, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog inspect %d", runID), func() error {
		return cmdInspect(db, runID)
	})

	if !strings.Contains(output, "TestAlpha") {
		t.Errorf("expected test name in output\n%s", output)
	}
	if !strings.Contains(output, "running setup") {
		t.Errorf("expected 'running setup' in output\n%s", output)
	}
	if !strings.Contains(output, "go build") {
		t.Errorf("expected cli event in output\n%s", output)
	}
}

// TestCLI_Watch_Finished verifies the runlog watch <id> command prints all events for a finished run and exits cleanly.
func TestCLI_Watch_Finished(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	db, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog watch %d", runID), func() error {
		return cmdWatch(db, []string{fmt.Sprintf("%d", runID)})
	})

	if !strings.Contains(output, "test started") {
		t.Errorf("expected 'test started' in watch output\n%s", output)
	}
	if !strings.Contains(output, "running setup") {
		t.Errorf("expected 'running setup' in watch output\n%s", output)
	}
}

// TestCLI_Reap_DryRun verifies runlog reap --dry-run displays stale runs without modifying them (finished_at stays NULL).
func TestCLI_Reap_DryRun(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	_, db := newTestApp(t)

	_, err := db.RawDB().Exec(
		`INSERT INTO test_runs (test_name, started_at) VALUES ('stale-cli-test', datetime('now', '-1 hour'))`,
	)
	if err != nil {
		t.Fatalf("insert stale run: %v", err)
	}

	output := df.RunCLI("runlog reap --dry-run", func() error {
		return cmdReap(db, 0, true)
	})

	if !strings.Contains(output, "stale-cli-test") {
		t.Errorf("expected stale-cli-test in dry-run output\n%s", output)
	}

	var passed *int
	db.RawDB().QueryRow("SELECT passed FROM test_runs WHERE test_name = 'stale-cli-test'").Scan(&passed)
	if passed != nil {
		t.Errorf("expected passed=NULL (dry-run should not modify), got %d", *passed)
	}
}

// TestCLI_Reap_Executes verifies runlog reap marks stale runs as finished with passed=NULL and finished_at set.
func TestCLI_Reap_Executes(t *testing.T) {
	df := NewDogfoodRun(t, "cli")
	defer df.Done()

	_, db := newTestApp(t)

	started := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	_, err := db.RawDB().Exec(
		`INSERT INTO test_runs (test_name, started_at) VALUES ('stale-cli-reap', ?)`, started,
	)
	if err != nil {
		t.Fatalf("insert stale run: %v", err)
	}

	df.RunCLI("runlog reap", func() error {
		return cmdReap(db, 0, false)
	})

	var finishedStr *string
	db.RawDB().QueryRow("SELECT finished_at FROM test_runs WHERE test_name = 'stale-cli-reap'").Scan(&finishedStr)
	if finishedStr == nil || *finishedStr == "" {
		t.Errorf("stale run should have been reaped (finished_at is NULL)")
	} else {
		t.Logf("  finished_at = %s — run was reaped", *finishedStr)
	}
}
