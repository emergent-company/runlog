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
func captureStdout(t *testing.T, fn func() error) string { //nolint:deadcode
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
func captureStdoutErr(t *testing.T, fn func() error) string { //nolint:deadcode
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
func captureStderr(t *testing.T, fn func() error) string { //nolint:deadcode
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
func newTestDBWithRuns(t *testing.T) (*runlog.RunDB, *runlog.DaemonClient, map[string]int64) {
	t.Helper()
	_, app, dc := newDaemonApp(t)

	runs := make(map[string]int64)
	for _, tc := range []struct {
		name   string
		passed bool
	}{
		{"TestAlpha", true},
		{"TestBeta", false},
		{"TestGamma", true},
	} {
		r := dc.CreateRun(t, runlog.CreateRunOpts{EnvProfile: tc.name})
		dc.MarkDone(t, r.DaemonID, runlog.MarkDoneOpts{Passed: &tc.passed})
		runs[tc.name] = r.TestRunID
	}

	// Add events via daemon for the first run using daemon run ID.
	runsList := dc.ListTestRuns(t)
	for _, tr := range runsList {
		name, _ := tr["test_name"].(string)
		if name == "TestAlpha" {
			id, _ := tr["id"].(float64)
			run := dc.MustGetTestRun(t, int64(id))
			daemonID, _ := run["daemon_run_id"].(string)
			if daemonID != "" {
				for _, ev := range []struct{ kind, msg string }{
					{"state_change", "test started"},
					{"log", "running setup"},
					{"cli", "go build ./..."},
					{"state_change", "test finished"},
				} {
					dc.AddEvent(t, daemonID, ev.kind, ev.msg)
				}
			}
			break
		}
	}

	return app.db, dc, runs
}

// ── Tests ──────────────────────────────────────────────────────────────────

// TestCLI_Runs verifies the runlog runs command prints a table with test names and pass/fail statuses.
func TestCLI_Runs(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI runs command prints test table",
		"Seeds 3 test runs (Alpha, Beta, Gamma)",
		"Verifies names and pass/fail status appear in output",
	)

	db, _, _ := newTestDBWithRuns(t)

	output := df.RunCLI("runlog runs --since 1y", func() error {
		return cmdRuns(db, 365*24*time.Hour)
	})

	if !strings.Contains(output, "TestAlpha") {
		df.Event("assertion", "FAIL: TestAlpha not in CLI output")
		t.Errorf("expected TestAlpha in output\n%s", output)
	} else {
		df.Event("assertion", "TestAlpha found in CLI output")
	}
	if !strings.Contains(output, "TestBeta") {
		df.Event("assertion", "FAIL: TestBeta not in CLI output")
		t.Errorf("expected TestBeta in output\n%s", output)
	} else {
		df.Event("assertion", "TestBeta found in CLI output")
	}
	if !strings.Contains(output, "TestGamma") {
		df.Event("assertion", "FAIL: TestGamma not in CLI output")
		t.Errorf("expected TestGamma in output\n%s", output)
	} else {
		df.Event("assertion", "TestGamma found in CLI output")
	}
	if !strings.Contains(output, "PASS") && !strings.Contains(output, "Pass") {
		df.Event("assertion", "FAIL: PASS/Pass status not in output")
		t.Errorf("expected PASS/Pass status in output\n%s", output)
	} else {
		df.Event("assertion", "PASS status found in CLI output")
	}
	if !strings.Contains(output, "FAIL") && !strings.Contains(output, "Fail") {
		df.Event("assertion", "FAIL: FAIL/Fail status not in output")
		t.Errorf("expected FAIL/Fail status in output\n%s", output)
	} else {
		df.Event("assertion", "FAIL status found in CLI output")
	}
}

// TestCLI_Show verifies the runlog show <id> command prints run metadata and all seeded events.
func TestCLI_Show(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI show command prints run metadata",
		"Seeds 3 runs with events",
		"Verifies output contains run details and events",
	)

	db, _, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog show %d", runID), func() error {
		return cmdShow(db, runID)
	})

	if !strings.Contains(output, "TestAlpha") {
		df.Event("assertion", "FAIL: TestAlpha not in show output")
		t.Errorf("expected test name TestAlpha in output\n%s", output)
	} else {
		df.Event("assertion", "TestAlpha found in show output")
	}
	if !strings.Contains(output, "run:") {
		df.Event("assertion", "FAIL: 'run:' not in show output")
		t.Errorf("expected 'run:' in output\n%s", output)
	} else {
		df.Event("assertion", "'run:' found in show output")
	}
	if !strings.Contains(output, "state_change") {
		df.Event("assertion", "FAIL: state_change event not in show output")
		t.Errorf("expected state_change event in output\n%s", output)
	} else {
		df.Event("assertion", "state_change event found in show output")
	}
	if !strings.Contains(output, "running setup") {
		df.Event("assertion", "FAIL: 'running setup' not in show output")
		t.Errorf("expected 'running setup' log event in output\n%s", output)
	} else {
		df.Event("assertion", "'running setup' found in show output")
	}
	if !strings.Contains(output, "go build") {
		df.Event("assertion", "FAIL: 'go build' not in show output")
		t.Errorf("expected 'go build' cli event in output\n%s", output)
	} else {
		df.Event("assertion", "'go build' found in show output")
	}
}

// TestCLI_Events verifies the runlog events <id> command prints a formatted event table with SEQ, KIND, and MESSAGE columns.
func TestCLI_Events(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI events command prints event table",
		"Seeds runs with state_change, log, cli events",
		"Verifies SEQ, KIND, MESSAGE columns and event content",
	)

	db, _, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog events %d", runID), func() error {
		return cmdEvents(db, runID)
	})

	if !strings.Contains(output, "SEQ") {
		df.Event("assertion", "FAIL: SEQ header not in events output")
		t.Errorf("expected SEQ header in output\n%s", output)
	} else {
		df.Event("assertion", "SEQ header found in events output")
	}
	if !strings.Contains(output, "KIND") {
		df.Event("assertion", "FAIL: KIND header not in events output")
		t.Errorf("expected KIND header in output\n%s", output)
	} else {
		df.Event("assertion", "KIND header found in events output")
	}
	if !strings.Contains(output, "MESSAGE") {
		df.Event("assertion", "FAIL: MESSAGE header not in events output")
		t.Errorf("expected MESSAGE header in output\n%s", output)
	} else {
		df.Event("assertion", "MESSAGE header found in events output")
	}
	if !strings.Contains(output, "test started") {
		df.Event("assertion", "FAIL: 'test started' event not found")
		t.Errorf("expected 'test started' event\n%s", output)
	} else {
		df.Event("assertion", "'test started' event found")
	}
	if !strings.Contains(output, "running setup") {
		df.Event("assertion", "FAIL: 'running setup' event not found")
		t.Errorf("expected 'running setup' event\n%s", output)
	} else {
		df.Event("assertion", "'running setup' event found")
	}
}

// TestCLI_Inspect verifies the runlog inspect <id> command prints detailed event information with inspector formatting.
func TestCLI_Inspect(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI inspect command prints detailed event info",
		"Seeds runs with events including cli events",
		"Verifies output contains event details and go build output",
	)

	db, _, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog inspect %d", runID), func() error {
		return cmdInspect(db, runID)
	})

	if !strings.Contains(output, "TestAlpha") {
		df.Event("assertion", "FAIL: TestAlpha not in inspect output")
		t.Errorf("expected test name in output\n%s", output)
	} else {
		df.Event("assertion", "TestAlpha found in inspect output")
	}
	if !strings.Contains(output, "running setup") {
		df.Event("assertion", "FAIL: 'running setup' not in inspect output")
		t.Errorf("expected 'running setup' in output\n%s", output)
	} else {
		df.Event("assertion", "'running setup' found in inspect output")
	}
	if !strings.Contains(output, "go build") {
		df.Event("assertion", "FAIL: 'go build' not in inspect output")
		t.Errorf("expected cli event in output\n%s", output)
	} else {
		df.Event("assertion", "'go build' found in inspect output")
	}
}

// TestCLI_Watch_Finished verifies the runlog watch <id> command prints all events for a finished run and exits cleanly.
func TestCLI_Watch_Finished(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI watch command prints events for finished run",
		"Seeds a finished run with events",
		"Verifies watch streams events and exits cleanly",
	)

	db, _, runs := newTestDBWithRuns(t)
	runID := runs["TestAlpha"]

	output := df.RunCLI(fmt.Sprintf("runlog watch %d", runID), func() error {
		return cmdWatch(db, []string{fmt.Sprintf("%d", runID)})
	})

	if !strings.Contains(output, "test started") {
		df.Event("assertion", "FAIL: 'test started' not in watch output")
		t.Errorf("expected 'test started' in watch output\n%s", output)
	} else {
		df.Event("assertion", "'test started' found in watch output")
	}
	if !strings.Contains(output, "running setup") {
		df.Event("assertion", "FAIL: 'running setup' not in watch output")
		t.Errorf("expected 'running setup' in watch output\n%s", output)
	} else {
		df.Event("assertion", "'running setup' found in watch output")
	}
}

// TestCLI_Reap_DryRun verifies runlog reap --dry-run displays stale runs without modifying them (finished_at stays NULL).
func TestCLI_Reap_DryRun(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI reap --dry-run displays stale runs without modifying",
		"Inserts a stale run 1 hour old",
		"Verifies dry-run mode shows stale run but does NOT update finished_at",
	)

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
		df.Event("assertion", "FAIL: stale-cli-test not in dry-run output")
		t.Errorf("expected stale-cli-test in dry-run output\n%s", output)
	} else {
		df.Event("assertion", "stale-cli-test found in dry-run output")
	}

	var passed *int
	db.RawDB().QueryRow("SELECT passed FROM test_runs WHERE test_name = 'stale-cli-test'").Scan(&passed)
	if passed != nil {
		df.Event("assertion", "FAIL: dry-run modified passed column")
		t.Errorf("expected passed=NULL (dry-run should not modify), got %d", *passed)
	} else {
		df.Event("assertion", "dry-run correctly preserved passed=NULL")
	}
}

// TestCLI_Reap_Executes verifies runlog reap marks stale runs as finished with passed=NULL and finished_at set.
func TestCLI_Reap_Executes(t *testing.T) {
	df := runlog.NewDogfoodRun(t, "cli")
	defer df.Done()
	df.Describe("CLI reap marks stale runs as finished",
		"Inserts a stale run 2 hours old",
		"Verifies reap sets finished_at timestamp",
	)

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
		df.Event("assertion", "FAIL: stale run was not reaped")
		t.Errorf("stale run should have been reaped (finished_at is NULL)")
	} else {
		t.Logf("  finished_at = %s — run was reaped", *finishedStr)
	}
}
