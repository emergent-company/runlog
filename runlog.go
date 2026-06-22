// Package e2eframework — runlog.go
//
// RunLog: persistent single-file log for a whole test run, plus Gantt/token
// rendering utilities for multi-agent orchestrator tests.
//
// Every RunLog also writes structured events to the process-wide SQLite
// RunDB (.runlog/runs.db) via SharedDB().  All DB writes are best-effort:
// failures are logged via t.Log but never fail the test.
package runlog

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// RunLog writes a chronological, timestamped record of every significant event
// in a test run to a folder that is always created regardless of whether the
// test passes or is run with -v.
//
// Layout:
//
//	logs/<timestamp>-<TestName>/
//	    run.log                   ← main chronological log (rl.Printf / rl.CLI)
//	    <agent>-<runID>.log       ← per-agent-run detail files (caller-written)
//
// Usage:
//
//	rl := NewRunLog(t)          // creates logs/<timestamp>-<TestName>/run.log
//	defer rl.Close()
//	rl.Section("Step 1")
//	rl.Printf("did thing %s", x)
//	rl.CLI("memory agents list", output)
//	rl.Event("state_change", "wp abc: pending→running", map[string]any{...})
//	rl.Dir()                    // returns the folder path for sibling files
type RunLog struct {
	t         *testing.T
	mu        sync.Mutex
	f         *os.File
	path      string
	dir       string
	StartedAt time.Time

	// DB integration (best-effort; both may be zero if DB is unavailable)
	db    *RunDB
	runID int64
	seq   atomic.Int64 // monotonically increasing event sequence number

	// Section-as-collapsible-parent tracking.
	// Every call to Section() starts a new group event; subsequent dbEvent
	// calls are stored as children of that group until the next Section() or Close().
	currentSectionID int64
	sectionChildren  []ChildEvent

	// Variant tagging: "key:value" strings stored as a JSON array in the DB.
	tags []string
	// Experiment name/id — auto-populated from the EXPERIMENT env var in NewRunLog.
	experiment string
	// App version — set by SetAppVersion (manual, test-writer responsibility).
	appVersion string
	// Test version — SHA256 of the test file, auto-detected in NewRunLog.
	testVersion string
	// Timeout duration set by SetTimeout.
	timeoutSeconds float64
	// Category set by SetCategory.
	category string

	// Outcome reason tracking.
	// skipReason is set by Skipf() before calling t.Skip().
	// lastFailMsg is updated by Failf() on every call (last one wins).
	skipReason  string
	lastFailMsg string

	// Token usage and cost tracking.
	// Updated via RecordTokenUsage() when tests make LLM calls.
	inputTokens  int64
	outputTokens int64
	costUSD      float64

	// TracePoller lifecycle (optional; started via StartTracePoller).
	tracePoller *TracePoller
}

// ActiveRunLogs maps t.Name() → *RunLog for tests that have an active run log.
var ActiveRunLogs sync.Map

// NewRunLog creates a per-test log folder and opens run.log inside it.
// The folder is placed under the logs/ directory used by LogSession.
// It is safe to call even if the directory cannot be created — all writes
// become no-ops and the path is logged via t.Log.
func NewRunLog(t *testing.T) *RunLog { //nolint:deadcode
	t.Helper()
	rl := &RunLog{t: t, StartedAt: time.Now()}

	_, srcFile, _, _ := runtime.Caller(1) // test source file for version detection

	logDir := os.Getenv("TEST_LOG_DIR")
	if logDir == "" {
		if srcFile != "" {
			logDir = filepath.Join(filepath.Dir(srcFile), "logs")
		} else if _, err := os.Stat("/test-logs"); err == nil {
			logDir = "/test-logs"
		} else {
			logDir = filepath.Join(os.TempDir(), "memory-cli-docker-tests")
		}
	}

	safeName := strings.NewReplacer("/", "_", " ", "_", ":", "-").Replace(t.Name())
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	runDir := filepath.Join(logDir, fmt.Sprintf("%s-%s", timestamp, safeName))

	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Logf("warn: RunLog: could not create log directory %s: %v", runDir, err)
		return rl
	}
	rl.dir = runDir
	rl.path = filepath.Join(runDir, "run.log")

	f, err := os.OpenFile(rl.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Logf("warn: RunLog: could not open log file %s: %v", rl.path, err)
		return rl
	}
	rl.f = f

	// Register so LogSession routes into this file.
	ActiveRunLogs.Store(t.Name(), rl)

	rl.writef("=== RUN LOG: %s ===\n", t.Name())
	rl.writef("started: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	t.Logf("run log: %s", rl.path)

	// Wire up the shared DB (best-effort).
	if db, err := SharedDB(); err == nil && db != nil {
		envName := os.Getenv("MEMORY_TEST_ENV")
		envVars := captureEnvVars()

		// If RUNLOG_RUN_ID is set, use the existing row instead of creating a new one.
		if ridStr := os.Getenv("RUNLOG_RUN_ID"); ridStr != "" {
			if rid, err := strconv.ParseInt(ridStr, 10, 64); err == nil {
				rl.db = db
				rl.runID = rid
				t.Logf("runlog: using existing run ID %d from RUNLOG_RUN_ID", rid)
			} else {
				t.Logf("warn: RunLog: invalid RUNLOG_RUN_ID %q: %v", ridStr, err)
			}
		} else if id, err := db.InsertRun(t.Name(), rl.StartedAt, Runner(), envName, envVars); err == nil {
			rl.db = db
			rl.runID = id
		} else {
			t.Logf("warn: RunLog: DB InsertRun: %v", err)
		}
	} else if err != nil {
		t.Logf("warn: RunLog: SharedDB: %v", err)
	}

	// Auto-populate experiment from the EXPERIMENT env var.
	if exp := os.Getenv("EXPERIMENT"); exp != "" {
		rl.SetExperiment(exp)
	}

	// Auto-detect test version from the test source file.
	if srcFile != "" {
		sha := FileSHA256(srcFile)
		gitHash := GitCommitHash(srcFile)
		testVer := sha
		if testVer == "" {
			testVer = gitHash
		}
		if testVer != "" {
			details := map[string]any{"sha256": sha}
			if gitHash != "" {
				details["git_commit"] = gitHash
			}
			rl.testVersion = testVer
			rl.writef("test_version: %s\n", testVer)
			if rl.db != nil && rl.runID != 0 {
				if err := rl.db.UpdateRunTestVersion(rl.runID, testVer); err != nil {
					rl.t.Logf("warn: RunLog: DB UpdateRunTestVersion: %v", err)
				}
			}
			rl.dbEvent("test_version", testVer, details)
		}
	}

	return rl
}

// Dir returns the folder that contains run.log and any sibling detail files.
// Returns "" if the log directory could not be created.
func (rl *RunLog) Dir() string { //nolint:deadcode
	return rl.dir
}

// Close flushes and closes the log file and deregisters from LogSession routing.
// It records the test outcome (passed/failed) in the DB.
func (rl *RunLog) Close() { //nolint:deadcode
	// Stop trace poller before acquiring the log mutex (poller may call dbEvent).
	if rl.tracePoller != nil {
		rl.tracePoller.Stop()
		rl.tracePoller = nil
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.f == nil {
		return
	}

	// Flush any buffered section children to the DB.
	rl.flushSectionLocked()

	now := time.Now()
	rl.writeLocked(fmt.Sprintf("\nfinished: %s\n", now.UTC().Format(time.RFC3339)))
	_ = rl.f.Close()
	rl.f = nil
	ActiveRunLogs.Delete(rl.t.Name())
	if rl.dir != "" {
		rl.t.Logf("run log written: %s", rl.dir)
	} else {
		rl.t.Logf("run log written: %s", rl.path)
	}

	// Persist run outcome to DB.
	if rl.db != nil && rl.runID != 0 {
		var outcome RunOutcome
		var reason string
		switch {
		case rl.t.Skipped():
			outcome = OutcomeSkip
			reason = rl.skipReason
		case rl.t.Failed():
			outcome = OutcomeFail
			reason = rl.lastFailMsg
		default:
			outcome = OutcomePass
		}
		if err := rl.db.FinishRunWithCost(rl.runID, now, outcome, reason, rl.inputTokens, rl.outputTokens, rl.costUSD); err != nil {
			rl.t.Logf("warn: RunLog: DB FinishRunWithCost: %v", err)
		}
	}
}

// Failf writes a "failure" event to the run log and DB, then calls t.Fatalf.
// Use this instead of bare t.Fatalf/t.Fatal at meaningful failure points so
// the reason is recorded in the log before the test exits.
//
// Because t.Fatal calls runtime.Goexit, all deferred functions (including
// defer rl.Close()) still execute, which means finished_at and passed=false
// are correctly written to the DB even when the test fails mid-run.
func (rl *RunLog) Failf(format string, args ...any) { //nolint:deadcode
	rl.t.Helper()
	msg := fmt.Sprintf(format, args...)
	rl.lastFailMsg = msg
	rl.dbEvent("failure", msg, nil)
	rl.writef("[FAIL] %s\n", msg)
	rl.t.Fatal(msg)
}

// Skipf writes a "skip" event to the run log and DB, then calls t.Skipf.
// Use this instead of bare t.Skip/t.Skipf so the skip reason is recorded
// in the run row and visible in `runlog inspect`.
//
// Because t.Skip calls runtime.Goexit, all deferred functions (including
// defer rl.Close()) still execute, which means finished_at and passed=2
// are correctly written to the DB.
func (rl *RunLog) Skipf(format string, args ...any) { //nolint:deadcode
	rl.t.Helper()
	msg := fmt.Sprintf(format, args...)
	rl.skipReason = msg
	rl.dbEvent("skip", msg, nil)
	rl.writef("[SKIP] %s\n", msg)
	rl.t.Skip(msg)
}

// DoSkipf records a skip reason via rl.Skipf when rl is non-nil, otherwise
// falls through to t.Skipf.  Use this in skip-guard helpers that accept an
// optional *RunLog so the reason is captured in the runs DB.
//
// Because both rl.Skipf and t.Skipf call runtime.Goexit, DoSkipf never returns.
func DoSkipf(t *testing.T, rl *RunLog, format string, args ...any) { //nolint:deadcode
	t.Helper()
	if rl != nil {
		rl.Skipf(format, args...) // calls t.Skip internally — never returns
	}
	t.Skipf(format, args...)
}

// Describe sets a human-readable description for this test run that is stored
// in the DB and shown in the run-list inspector panel of the TUI.
//
// summary is a one-line explanation of what the test verifies.
// bullets are optional detail points (e.g. preconditions, key assertions).
//
// Call it once, right after NewRunLog:
//
//	rl := NewRunLog(t)
//	defer rl.Close()
//	rl.Describe("Verifies the full blueprint install + task-cli workflow",
//	    "Installs workspace-memory-blueprint into a fresh project",
//	    "Creates WorkPackage and Task graph objects",
//	    "Runs task-cli list and asserts all titles appear",
//	)
func (rl *RunLog) Describe(summary string, bullets ...string) { //nolint:deadcode
	rl.t.Helper()
	rl.t.Log(summary)
	for _, b := range bullets {
		rl.t.Log("  • " + b)
	}
	// Write a human-readable block to the flat log so it appears at the top.
	rl.writef("description: %s\n", summary)
	for _, b := range bullets {
		rl.writef("  • %s\n", b)
	}
	rl.writef("\n")

	// Persist to the DB run row (best-effort).
	if rl.db != nil && rl.runID != 0 {
		desc := RunDescription{Summary: summary, Bullets: bullets}
		if err := rl.db.UpdateRunDescription(rl.runID, desc); err != nil {
			rl.t.Logf("warn: RunLog.Describe: DB UpdateRunDescription: %v", err)
		}
	}
}

// Tag appends one or more variant tags to this test run and persists them to
// the DB.  Tags should be "key:value" strings, e.g. "model:gemini-2.0-flash"
// or "blueprint:v2".  They are stored as a JSON array in test_runs.tags so
// runs with different settings can be filtered and compared in the TUI.
//
// Tag is safe to call multiple times; each call appends to the existing set.
// It also emits a "tag" event to the flat log so tags are visible in the
// chronological record.
func (rl *RunLog) Tag(tags ...string) { //nolint:deadcode
	if len(tags) == 0 {
		return
	}

	rl.mu.Lock()
	rl.tags = append(rl.tags, tags...)
	snapshot := make([]string, len(rl.tags))
	copy(snapshot, rl.tags)
	rl.mu.Unlock()

	// Write a human-readable line to the flat log.
	rl.writef("tags: %s\n", strings.Join(tags, ", "))

	// Persist the full updated tag list to the DB.
	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunTags(rl.runID, snapshot); err != nil {
			rl.t.Logf("warn: RunLog.Tag: DB UpdateRunTags: %v", err)
		}
	}

	// Emit a "tag" event so tags appear in the structured event log.
	rl.dbEvent("tag", strings.Join(tags, ", "), map[string]any{"tags": tags})
}

// SetExperiment assigns an experiment name or ID to this test run.  It is
// persisted to test_runs.experiment so that multiple runs belonging to the
// same experiment batch can be grouped and compared in the TUI.
//
// SetExperiment is called automatically by NewRunLog when the EXPERIMENT
// environment variable is set.  Tests may also call it explicitly.
func (rl *RunLog) SetExperiment(name string) { //nolint:deadcode
	if name == "" {
		return
	}

	rl.mu.Lock()
	rl.experiment = name
	rl.mu.Unlock()

	rl.writef("experiment: %s\n", name)

	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunExperiment(rl.runID, name); err != nil {
			rl.t.Logf("warn: RunLog.SetExperiment: DB UpdateRunExperiment: %v", err)
		}
	}
}

// SetCategory declares the test's category for the web UI tests list.
// Call this at the start of a test to group it under a named category
// instead of relying on config-file category patterns or directory names.
func (rl *RunLog) SetCategory(category string) { //nolint:deadcode
	if category == "" {
		return
	}

	rl.mu.Lock()
	rl.category = category
	rl.mu.Unlock()

	rl.writef("category: %s\n", category)

	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunCategory(rl.runID, category); err != nil {
			rl.t.Logf("warn: RunLog.SetCategory: DB UpdateRunCategory: %v", err)
		}
	}
}

// SetTimeout registers a per-test timeout duration.  The background timeout
// worker will mark the run as timed out if it exceeds this duration.
// Call this at the start of a test to prevent stuck runs.
func (rl *RunLog) SetTimeout(d time.Duration) { //nolint:deadcode
	if d <= 0 {
		return
	}
	sec := d.Seconds()
	rl.mu.Lock()
	rl.timeoutSeconds = sec
	rl.mu.Unlock()

	rl.writef("timeout: %.0fs\n", sec)

	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunTimeout(rl.runID, sec); err != nil {
			rl.t.Logf("warn: RunLog.SetTimeout: DB UpdateRunTimeout: %v", err)
		}
	}
}

// SetAppVersion records the version of the application-under-test.  Call this
// at the start of a test to document which version of the app was tested.
// It is stored in test_runs.app_version and emitted as an "app_version" event.
//
// Unlike SetTestVersion, this is always manual — the test writer decides what
// version to record and when.
func (rl *RunLog) SetAppVersion(version string) { //nolint:deadcode
	if version == "" {
		return
	}

	rl.mu.Lock()
	rl.appVersion = version
	rl.mu.Unlock()

	rl.writef("app_version: %s\n", version)

	// Persist to the DB run row (best-effort).
	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunAppVersion(rl.runID, version); err != nil {
			rl.t.Logf("warn: RunLog.SetAppVersion: DB UpdateRunAppVersion: %v", err)
		}
	}

	// Emit an app_version event.
	rl.Event("app_version", version, map[string]any{"version": version})
}

// SetTestVersion records a unique identifier for the test file (SHA256 by
// default, set automatically in NewRunLog).  Call this to override the
// auto-detected version with a custom label.
//
// It is stored in test_runs.test_version and emitted as a "test_version" event.
func (rl *RunLog) SetTestVersion(version string) { //nolint:deadcode
	if version == "" {
		return
	}

	rl.mu.Lock()
	rl.testVersion = version
	rl.mu.Unlock()

	rl.writef("test_version: %s\n", version)

	// Persist to the DB run row (best-effort).
	if rl.db != nil && rl.runID != 0 {
		if err := rl.db.UpdateRunTestVersion(rl.runID, version); err != nil {
			rl.t.Logf("warn: RunLog.SetTestVersion: DB UpdateRunTestVersion: %v", err)
		}
	}

	// Emit a test_version event.
	rl.Event("test_version", version, map[string]any{"version": version})
}

// RecordTokenUsage accumulates token usage and cost for LLM operations performed
// during this test run.  Call this after each `memory ask` or agent operation
// that consumes tokens.  The accumulated totals are written to the DB when the
// run finishes (in Close).
//
// Usage:
//
//	inputTok, outputTok, cost := FetchRunTokenUsage(t, srv, token, projectID, runID)
//	rl.RecordTokenUsage(inputTok, outputTok, cost)
func (rl *RunLog) RecordTokenUsage(inputTokens, outputTokens int64, costUSD float64) { //nolint:deadcode
	if inputTokens == 0 && outputTokens == 0 && costUSD == 0 {
		return
	}

	rl.mu.Lock()
	rl.inputTokens += inputTokens
	rl.outputTokens += outputTokens
	rl.costUSD += costUSD
	rl.mu.Unlock()

	// Emit a token_usage event so it appears in the chronological log.
	rl.Event("token_usage", fmt.Sprintf("%s in / %s out  $%.6f", FormatInt(inputTokens), FormatInt(outputTokens), costUSD), map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"cost_usd":      costUSD,
	})
}

// Section writes a prominent section header to the log file and starts a new
// collapsible group in the DB.  All subsequent Printf/CLI/Event calls are
// stored as children of this section until the next Section() or Close().
func (rl *RunLog) Section(name string) { //nolint:deadcode
	rl.t.Helper()
	rl.t.Log("── " + name + " ──")
	rl.writef("\n%s\n%s\n", name, strings.Repeat("─", 72))

	rl.mu.Lock()
	// Flush children from the previous section before starting the new one.
	rl.flushSectionLocked()
	rl.mu.Unlock()

	// Insert the new section as a group event and remember its DB id.
	if rl.db != nil && rl.runID != 0 {
		seq := int(rl.seq.Add(1))
		elapsed := time.Since(rl.StartedAt).Seconds()
		id, err := rl.db.InsertGroupEvent(rl.runID, seq, time.Now(), elapsed, "section", name)
		if err != nil {
			rl.t.Logf("warn: RunLog: DB Section InsertGroupEvent: %v", err)
		} else {
			rl.mu.Lock()
			rl.currentSectionID = id
			rl.sectionChildren = rl.sectionChildren[:0]
			rl.mu.Unlock()
		}
	}
}

// Printf writes a timestamped line to the log file and also calls t.Log.
func (rl *RunLog) Printf(format string, args ...any) { //nolint:deadcode
	rl.t.Helper()
	msg := fmt.Sprintf(format, args...)
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())
	rl.writef("[%s] %s\n", ts, msg)
	rl.t.Log(msg)
	rl.dbEvent("log", msg, nil)
}

// LogStep writes a labelled log event with structured details.
// The label is shown in the TUI event table; details are shown as a
// key-value table when the user clicks to expand the event row.
// Example:
//
//	rl.LogStep("setting up project", map[string]any{"project_id": id})
func (rl *RunLog) LogStep(label string, details map[string]any) { //nolint:deadcode
	rl.t.Helper()
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())
	rl.writef("[%s] %s\n", ts, label)
	rl.t.Log(label)
	rl.dbEvent("log", label, details)
}

// AssertionStep records a test assertion with structured expected/actual values.
// The label describes the comparison (e.g. "response status == 404").
// Details include expected, actual, and optional extra fields.
// The TUI renders these in a dedicated comparison layout.
// Example:
//
//	rl.AssertionStep("status == 404", 404, resp.StatusCode, nil)
func (rl *RunLog) AssertionStep(label string, expected, actual any, extra map[string]any) { //nolint:deadcode
	rl.t.Helper()
	details := map[string]any{
		"expected": expected,
		"actual":   actual,
	}
	for k, v := range extra {
		details[k] = v
	}
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())
	rl.writef("[%s] ASSERT %s\n", ts, label)
	rl.t.Log(label)
	rl.dbEvent("assertion", label, details)
}

// CLI writes a CLI invocation header and its full output to the log file
// and stdout (via t.Log). The full invocation and output are available in the
// event details.
//
// The event message shown in the run list is "$ <invocation>".
// Use CLIStep when you want a short human-readable description as the message
// instead (e.g. "Revoke token") while still recording the full command and
// output in the inspector details.
func (rl *RunLog) CLI(invocation, output string) { //nolint:deadcode
	rl.CLIStepErr("$ "+invocation, invocation, output, nil)
}

// CLIErr is like CLI but also records the command error (exit code) in the
// event details so the TUI can highlight the row in red on failure.
func (rl *RunLog) CLIErr(invocation, output string, err error) { //nolint:deadcode
	rl.CLIStepErr("$ "+invocation, invocation, output, err)
}

// MustRunCLI runs `memory <args>` and emits a CLI event with the full
// invocation and output. Fails the test on non-zero exit.
// Equivalent to calling runlog.MustRunCLI then rl.CLI, but in one step.
func (rl *RunLog) MustRunCLI(t *testing.T, args ...string) string { //nolint:deadcode
	t.Helper()
	out := MustRunCLI(t, args...)
	invocation := "memory " + strings.Join(args, " ")
	rl.CLI(invocation, out)
	return out
}

// MustRunCLIInDir runs `memory <args>` from dir and emits a CLI event.
func (rl *RunLog) MustRunCLIInDir(t *testing.T, dir string, args ...string) string { //nolint:deadcode
	t.Helper()
	out := MustRunCLIInDir(t, dir, args...)
	invocation := "memory " + strings.Join(args, " ")
	rl.CLI(invocation, out)
	return out
}

// CLIStep is like CLI but uses desc as the short message shown in the run list
// (e.g. "Revoke token") instead of the raw invocation string.  The full
// invocation and output are still recorded in the inspector details.
func (rl *RunLog) CLIStep(desc, invocation, output string) { //nolint:deadcode
	rl.CLIStepErr(desc, invocation, output, nil)
}

// CLIStepErr is like CLIStep but also records the command error (exit code) in
// the event details so the TUI can highlight the row in red on failure.
func (rl *RunLog) CLIStepErr(desc, invocation, output string, err error) { //nolint:deadcode
	rl.t.Helper()
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())
	rl.writef("[%s] $ %s\n%s\n", ts, invocation, strings.TrimRight(output, "\n"))
	rl.t.Logf("$ %s", invocation)
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line != "" {
			rl.t.Log("  " + line)
		}
	}
	details := map[string]any{
		"invocation": invocation,
		"output":     output,
	}
	if err != nil {
		details["error_msg"] = err.Error()
		details["exit_code"] = exitCode(err)
	}
	rl.dbEvent("cli", desc, details)
}

// exitCode extracts the integer exit code from a command error.
// Returns 1 for generic errors, 0 if err is nil.
func exitCode(err error) int { //nolint:deadcode
	if err == nil {
		return 0
	}
	// exec.ExitError carries the real exit code.
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return 1
}

// Event emits a structured event of any kind with an arbitrary JSON-
// serialisable details value.  It writes a human-readable line to the flat
// log and a structured row to the DB.
//
// kind should be a short snake_case string, e.g. "state_change", "metric",
// "gantt_row".  details may be any JSON-serialisable value or nil.
// This is the general-purpose escape hatch for tests that need custom events.
func (rl *RunLog) Event(kind, message string, details any) { //nolint:deadcode
	rl.t.Helper()
	rl.t.Logf("[%s] %s", kind, message)
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())
	rl.writef("[%s] [%s] %s\n", ts, kind, message)
	rl.dbEvent(kind, message, details)
}

// Group emits a single parent event in the DB whose children are the lines
// logged inside fn.  In the flat log file each child is written as an indented
// sub-line under the parent header so the file stays readable.
//
// Usage:
//
//	rl.Group("log", "Tasks created: 5", func(g *GroupLogger) {
//	    for _, task := range tasks {
//	        g.Printf("  %d. [%s] %s", i+1, task.Status, task.Title)
//	    }
//	})
//
// In the TUI the group appears as one collapsed row; pressing Enter expands
// it to show the child lines.
func (rl *RunLog) Group(kind, title string, fn func(g *GroupLogger)) { //nolint:deadcode
	ts := fmt.Sprintf("%.1fs", time.Since(rl.StartedAt).Seconds())

	// Write the parent header to the flat log.
	rl.writef("[%s] [%s] %s\n", ts, kind, title)
	rl.t.Log(title)

	// Insert the parent event row and obtain its DB id.
	var parentDBID int64
	if rl.db != nil && rl.runID != 0 {
		seq := int(rl.seq.Add(1))
		elapsed := time.Since(rl.StartedAt).Seconds()
		id, err := rl.db.InsertGroupEvent(rl.runID, seq, time.Now(), elapsed, kind, title)
		if err != nil {
			rl.t.Logf("warn: RunLog: DB InsertGroupEvent: %v", err)
		} else {
			parentDBID = id
		}
	}

	// Run the caller's function with a GroupLogger.
	gl := &GroupLogger{rl: rl, ts: ts}
	fn(gl)

	// Flush child lines to flat log (already written inline) and to DB.
	if parentDBID != 0 && len(gl.children) > 0 {
		if err := rl.db.AppendGroupChildren(parentDBID, gl.children); err != nil {
			rl.t.Logf("warn: RunLog: DB AppendGroupChildren: %v", err)
		}
	}
}

// GroupLogger is passed to the function given to RunLog.Group.
// Its methods capture sub-lines for the parent group event.
type GroupLogger struct {
	rl       *RunLog
	ts       string
	children []ChildEvent
}

// Printf writes one child line under the current group.
func (g *GroupLogger) Printf(format string, args ...any) { //nolint:deadcode
	msg := fmt.Sprintf(format, args...)
	g.rl.writef("  %s\n", msg) // indented in flat log
	g.children = append(g.children, ChildEvent{
		ElapsedS: time.Since(g.rl.StartedAt).Seconds(),
		Kind:     "log",
		Message:  msg,
	})
}

// Event writes one structured child event under the current group.
func (g *GroupLogger) Event(kind, message string, details any) { //nolint:deadcode
	g.rl.writef("  [%s] %s\n", kind, message)
	var detJSON string
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detJSON = string(b)
		}
	}
	g.children = append(g.children, ChildEvent{
		ElapsedS: time.Since(g.rl.StartedAt).Seconds(),
		Kind:     kind,
		Message:  message,
		Details:  detJSON,
	})
}

// StartTracePoller starts a background goroutine that polls the Memory server's
// Tempo proxy for agent.run traces associated with projectID and writes new
// spans to the DB as "trace_span" events.
//
// It is safe to call even when tracing is disabled on the server — all errors
// are non-fatal.  The poller is stopped automatically in Close().
//
// serverURL is the base URL of the Memory server (e.g. "http://localhost:3002").
// token is the API key or Bearer token.
// projectID is the Memory project to watch.
func (rl *RunLog) StartTracePoller(serverURL, token, projectID string) { //nolint:deadcode
	if rl.db == nil || rl.runID == 0 || serverURL == "" || projectID == "" {
		return
	}
	if rl.tracePoller != nil {
		// Already running; ignore.
		return
	}
	p := NewTracePoller(serverURL, token, projectID, rl.runID, rl.db)
	rl.tracePoller = p
	p.Start()
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (rl *RunLog) writef(format string, args ...any) { //nolint:deadcode
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.writeLocked(fmt.Sprintf(format, args...))
}

func (rl *RunLog) writeLocked(s string) { //nolint:deadcode
	if rl.f == nil {
		return
	}
	_, _ = rl.f.WriteString(s)
}

// dbEvent persists one event to the DB.  Always best-effort.
// When a section is active the event is stored as a child of that section;
// otherwise it is inserted as a top-level row.
func (rl *RunLog) dbEvent(kind, message string, details any) { //nolint:deadcode
	if rl.db == nil || rl.runID == 0 {
		return
	}

	rl.mu.Lock()
	sectionID := rl.currentSectionID
	rl.mu.Unlock()

	if sectionID != 0 {
		// Buffer as a child of the current section.
		var detJSON string
		if details != nil {
			if b, err := json.Marshal(details); err == nil {
				detJSON = string(b)
			}
		}
		child := ChildEvent{
			ElapsedS: time.Since(rl.StartedAt).Seconds(),
			Kind:     kind,
			Message:  message,
			Details:  detJSON,
		}
		rl.mu.Lock()
		// Re-check: still the same section (not flushed by a concurrent Section() call).
		if rl.currentSectionID == sectionID {
			rl.sectionChildren = append(rl.sectionChildren, child)
			rl.mu.Unlock()
			return
		}
		rl.mu.Unlock()
		// Section changed between our check and now — fall through to top-level insert.
	}

	seq := int(rl.seq.Add(1))
	elapsed := time.Since(rl.StartedAt).Seconds()
	if err := rl.db.InsertEvent(rl.runID, seq, time.Now(), elapsed, kind, message, details); err != nil {
		rl.t.Logf("warn: RunLog: DB InsertEvent(%s): %v", kind, err)
	}
}

// flushSectionLocked writes the buffered section children to the DB and resets
// the section tracking state.  Must be called with rl.mu held.
func (rl *RunLog) flushSectionLocked() { //nolint:deadcode
	if rl.currentSectionID == 0 || len(rl.sectionChildren) == 0 {
		rl.currentSectionID = 0
		rl.sectionChildren = rl.sectionChildren[:0]
		return
	}
	id := rl.currentSectionID
	children := make([]ChildEvent, len(rl.sectionChildren))
	copy(children, rl.sectionChildren)
	rl.currentSectionID = 0
	rl.sectionChildren = rl.sectionChildren[:0]

	// Release the lock while doing the DB write to avoid holding it too long.
	rl.mu.Unlock()
	if err := rl.db.AppendGroupChildren(id, children); err != nil {
		rl.t.Logf("warn: RunLog: DB flushSection AppendGroupChildren: %v", err)
	}
	rl.mu.Lock()
}

// ─────────────────────────────────────────────────────────────────────────────
// AgentRunInterval and Gantt / token-usage helpers
// ─────────────────────────────────────────────────────────────────────────────

// AgentRunInterval records one agent run's timing and token usage for Gantt rendering.
type AgentRunInterval struct {
	AgentName        string
	RunID            string // used to fetch token usage
	Start            time.Time
	End              time.Time // zero means still running
	DurationMs       int
	InputTokens      int64
	OutputTokens     int64
	EstimatedCostUSD float64
}

// AgentInfo is the minimal shape passed to BuildAgentRunIntervals and
// createRuntimeAgents. Cron is optional — when empty, helpers use the default
// dormant cron "0 0 0 1 1 *".
type AgentInfo struct {
	Name string
	ID   string
	Cron string
}

// FetchRunTokenUsage fetches token usage for a single agent run via
// GET /api/projects/:projectID/agent-runs/:runID.
// Returns zeros gracefully when the endpoint returns 404 or the tokenUsage
// field is absent.
func FetchRunTokenUsage(t *testing.T, srv, token, projectID, runID string) (inputTokens, outputTokens int64, estimatedCostUSD float64) { //nolint:deadcode
	t.Helper()
	url := fmt.Sprintf("%s/api/projects/%s/agent-runs/%s", srv, projectID, runID)
	resp := DoJSON(t, "GET", url, token, projectID, nil)
	if resp.StatusCode != 200 {
		_ = ReadBody(t, resp)
		return 0, 0, 0
	}
	body := ReadBody(t, resp)
	var apiResp struct {
		Data struct {
			TokenUsage *struct {
				TotalInputTokens  int64   `json:"totalInputTokens"`
				TotalOutputTokens int64   `json:"totalOutputTokens"`
				EstimatedCostUSD  float64 `json:"estimatedCostUsd"`
			} `json:"tokenUsage"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &apiResp); err != nil {
		return 0, 0, 0
	}
	if apiResp.Data.TokenUsage == nil {
		return 0, 0, 0
	}
	tu := apiResp.Data.TokenUsage
	return tu.TotalInputTokens, tu.TotalOutputTokens, tu.EstimatedCostUSD
}

// BuildAgentRunIntervals queries the runs API for each agent and returns a
// slice of AgentRunInterval sorted by start time.  Token usage is fetched
// per run via FetchRunTokenUsage.
func BuildAgentRunIntervals(t *testing.T, rl *RunLog, srv, token, projectID string, agents []AgentInfo) []AgentRunInterval { //nolint:deadcode
	t.Helper()
	var intervals []AgentRunInterval
	for _, agent := range agents {
		runsURL := fmt.Sprintf("%s/api/projects/%s/agents/%s/runs?limit=10", srv, projectID, agent.ID)
		runsResp := DoJSON(t, "GET", runsURL, token, projectID, nil)
		if runsResp.StatusCode != 200 {
			body := ReadBody(t, runsResp)
			rl.Printf("warn: agent runs API for %s returned %d: %s", agent.Name, runsResp.StatusCode, body)
			continue
		}
		runsBody := ReadBody(t, runsResp)
		var apiResp struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(runsBody)), &apiResp); err != nil {
			continue
		}
		for _, run := range apiResp.Data {
			runID, _ := run["id"].(string)
			startStr, _ := run["startedAt"].(string)
			start, err := time.Parse(time.RFC3339Nano, startStr)
			if err != nil {
				continue
			}
			var end time.Time
			if endStr, ok := run["completedAt"].(string); ok && endStr != "" {
				end, _ = time.Parse(time.RFC3339Nano, endStr)
			}
			durMs := 0
			if d, ok := run["durationMs"].(float64); ok {
				durMs = int(d)
			}
			var inTok, outTok int64
			var costUSD float64
			if runID != "" {
				inTok, outTok, costUSD = FetchRunTokenUsage(t, srv, token, projectID, runID)
			}
			iv := AgentRunInterval{
				AgentName:        agent.Name,
				RunID:            runID,
				Start:            start,
				End:              end,
				DurationMs:       durMs,
				InputTokens:      inTok,
				OutputTokens:     outTok,
				EstimatedCostUSD: costUSD,
			}
			intervals = append(intervals, iv)

			// Emit a metric event to the DB for this agent run.
			rl.Event("metric", fmt.Sprintf("%s: %s in / %s out", agent.Name, FormatInt(inTok), FormatInt(outTok)), map[string]any{
				"agent_name":    agent.Name,
				"run_id":        runID,
				"input_tokens":  inTok,
				"output_tokens": outTok,
				"cost_usd":      costUSD,
				"duration_ms":   durMs,
			})
		}
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].Start.Before(intervals[j].Start)
	})
	return intervals
}

const ganttBarWidth = 40 // characters wide for the Gantt bar

// GanttRow is the serialisable form of one agent run's timing, stored in the
// "gantt" event's details JSON so the TUI can re-render the chart at any width.
type GanttRow struct {
	AgentName        string  `json:"agent_name"`
	RunID            string  `json:"run_id"`
	StartS           float64 `json:"start_s"` // seconds from earliest start
	EndS             float64 `json:"end_s"`   // seconds from earliest start
	DurationMs       int     `json:"duration_ms"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	EstimatedCostUSD float64 `json:"cost_usd"`
}

// GanttData is stored as the details JSON of a "gantt" event.
type GanttData struct {
	TotalS float64    `json:"total_s"` // full span of the chart in seconds
	Rows   []GanttRow `json:"rows"`
}

// PrintGantt writes the Gantt-style timeline to rl, one row per
// agent run interval.  Each row includes a token/cost suffix when available.
// It also emits a single "gantt" event to the DB with the full GanttData so
// the TUI can re-render the chart at any terminal width.
func PrintGantt(rl *RunLog, intervals []AgentRunInterval) { //nolint:deadcode
	if len(intervals) == 0 {
		rl.Printf("(no agent run intervals found - API may have been unreachable)")
		return
	}

	var earliest, latest time.Time
	for _, iv := range intervals {
		if earliest.IsZero() || iv.Start.Before(earliest) {
			earliest = iv.Start
		}
		endTime := iv.End
		if endTime.IsZero() {
			endTime = time.Now()
		}
		if latest.IsZero() || endTime.After(latest) {
			latest = endTime
		}
	}
	totalDur := latest.Sub(earliest)
	if totalDur <= 0 {
		totalDur = time.Second
	}

	var ganttRows []GanttRow

	for _, iv := range intervals {
		startOff := iv.Start.Sub(earliest)
		endTime := iv.End
		if endTime.IsZero() {
			endTime = time.Now()
		}
		endOff := endTime.Sub(earliest)

		startCell := int(float64(startOff) / float64(totalDur) * ganttBarWidth)
		endCell := int(math.Round(float64(endOff) / float64(totalDur) * ganttBarWidth))
		if endCell <= startCell {
			endCell = startCell + 1
		}
		if endCell > ganttBarWidth {
			endCell = ganttBarWidth
		}

		bar := make([]byte, ganttBarWidth)
		for i := range bar {
			bar[i] = ' '
		}
		for i := startCell; i < endCell; i++ {
			bar[i] = '='
		}

		fmtHMS := func(d time.Duration) string {
			t := int(d.Seconds())
			if t < 0 {
				t = 0
			}
			h := t / 3600
			m := (t % 3600) / 60
			s := t % 60
			return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		}

		startLabel := fmtHMS(startOff)
		endLabel := fmtHMS(endOff)

		durLabel := ""
		if iv.DurationMs > 0 {
			durLabel = fmt.Sprintf(" (%dms)", iv.DurationMs)
		}

		tokenLabel := ""
		if iv.InputTokens > 0 || iv.OutputTokens > 0 {
			tokenLabel = fmt.Sprintf("  %s in / %s out  $%.6f",
				FormatInt(iv.InputTokens), FormatInt(iv.OutputTokens), iv.EstimatedCostUSD)
		}

		agentStartElapsed := iv.Start.Sub(rl.StartedAt).Seconds()
		rl.writef("[%.1fs] %s-%s  %-20s [%s]%s%s\n",
			agentStartElapsed, startLabel, endLabel, iv.AgentName, string(bar), durLabel, tokenLabel)

		ganttRows = append(ganttRows, GanttRow{
			AgentName:        iv.AgentName,
			RunID:            iv.RunID,
			StartS:           startOff.Seconds(),
			EndS:             endOff.Seconds(),
			DurationMs:       iv.DurationMs,
			InputTokens:      iv.InputTokens,
			OutputTokens:     iv.OutputTokens,
			EstimatedCostUSD: iv.EstimatedCostUSD,
		})
	}

	// Emit a single "gantt" event with full data so the TUI can render it.
	rl.Event("gantt", fmt.Sprintf("%d agents  %.0fs total", len(intervals), totalDur.Seconds()), GanttData{
		TotalS: totalDur.Seconds(),
		Rows:   ganttRows,
	})
}

// PrintTokenSummary writes a summary table of token usage aggregated
// by agent name after the Gantt chart.
func PrintTokenSummary(rl *RunLog, intervals []AgentRunInterval) { //nolint:deadcode
	type agentSummary struct {
		runs             int
		inputTokens      int64
		outputTokens     int64
		estimatedCostUSD float64
	}

	order := []string{}
	seen := map[string]bool{}
	byAgent := map[string]*agentSummary{}
	for _, iv := range intervals {
		if !seen[iv.AgentName] {
			order = append(order, iv.AgentName)
			seen[iv.AgentName] = true
			byAgent[iv.AgentName] = &agentSummary{}
		}
		s := byAgent[iv.AgentName]
		s.runs++
		s.inputTokens += iv.InputTokens
		s.outputTokens += iv.OutputTokens
		s.estimatedCostUSD += iv.EstimatedCostUSD
	}

	var totalIn, totalOut int64
	var totalCost float64
	var totalRuns int
	for _, name := range order {
		s := byAgent[name]
		totalRuns += s.runs
		totalIn += s.inputTokens
		totalOut += s.outputTokens
		totalCost += s.estimatedCostUSD
	}
	if totalIn == 0 && totalOut == 0 {
		rl.Printf("(no token usage data available)")
		return
	}

	const sep = "────────────────────────────────────────────────────────────────────────"
	rl.writef("%s\n", sep)
	rl.writef("%-24s  %4s   %13s   %13s   %10s\n", "Agent", "Runs", "Input Tokens", "Output Tokens", "Est. Cost")
	rl.writef("%s\n", sep)
	for _, name := range order {
		s := byAgent[name]
		inStr := FormatInt(s.inputTokens)
		outStr := FormatInt(s.outputTokens)
		if s.inputTokens == 0 && s.outputTokens == 0 {
			inStr = "—"
			outStr = "—"
		}
		costStr := "—"
		if s.estimatedCostUSD > 0 {
			costStr = fmt.Sprintf("$%.6f", s.estimatedCostUSD)
		}
		rl.writef("%-24s  %4d   %13s   %13s   %10s\n", name, s.runs, inStr, outStr, costStr)
	}
	rl.writef("%s\n", sep)
	rl.writef("%-24s  %4d   %13s   %13s   $%.6f\n",
		"TOTAL", totalRuns, FormatInt(totalIn), FormatInt(totalOut), totalCost)

	// Emit a token_summary event to the DB.
	summaryData := map[string]any{
		"total_runs":    totalRuns,
		"input_tokens":  totalIn,
		"output_tokens": totalOut,
		"cost_usd":      totalCost,
	}
	byAgentData := make(map[string]any, len(order))
	for _, name := range order {
		s := byAgent[name]
		byAgentData[name] = map[string]any{
			"runs":          s.runs,
			"input_tokens":  s.inputTokens,
			"output_tokens": s.outputTokens,
			"cost_usd":      s.estimatedCostUSD,
		}
	}
	summaryData["by_agent"] = byAgentData
	rl.Event("token_summary", fmt.Sprintf("total: %s in / %s out  $%.6f", FormatInt(totalIn), FormatInt(totalOut), totalCost), summaryData)
}

// FormatInt formats an int64 with comma thousands separators.
func FormatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	result := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	if neg {
		return "-" + string(result)
	}
	return string(result)
}

// FileSHA256 computes the SHA-256 hex digest of the file at path.
// Returns empty string if the file cannot be read.
func FileSHA256(path string) string { //nolint:deadcode
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// GitCommitHash returns the full commit hash of the last commit that touched
// the given file.  Returns empty string if git is unavailable, not a repo, or
// the file has no commits.
func GitCommitHash(filePath string) string { //nolint:deadcode
	dir := filepath.Dir(filePath)
	cmd := exec.Command("git", "log", "-1", "--format=%H", "--", filepath.Base(filePath))
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// captureEnvVars returns a map of important environment variables that should
// be recorded with the test run.  This includes API keys, server URLs, and
// other configuration that affects test behavior.
//
// The returned map may be empty if no tracked variables are set.
func captureEnvVars() map[string]string { //nolint:deadcode
	// List of environment variables to capture.
	// Add any variable you want to see in runlog output here.
	trackedVars := []string{
		"GOOGLE_AI_API_KEY",
		"MEMORY_TEST_SERVER",
		"MEMORY_TEST_TOKEN",
		"MEMORY_AUTH_MODE",
		"MEMORY_ORG_ID",
		"BRAVE_SEARCH_API_KEY",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	}

	result := make(map[string]string)
	for _, key := range trackedVars {
		if val := os.Getenv(key); val != "" {
			result[key] = val
		}
	}
	return result
}
