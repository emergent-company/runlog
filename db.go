// Package e2eframework — db.go
//
// RunDB: a SQLite-backed structured event log for test runs.
//
// Every test that creates a RunLog automatically gets its events persisted to
// .runlog/runs.db (or $TEST_RUNS_DB if set).  Both Docker and host runs share
// the same database so `runlog` shows a unified view.  The schema is
// intentionally minimal: one row per test run, one row per event.  All
// event-specific structure lives in the `details` TEXT column as a JSON blob
// so that new event kinds never require a schema migration.
//
// Schema versioning uses a schema_migrations table; migrations are applied
// once at DB open time and are idempotent.
package runlog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// ─────────────────────────────────────────────────────────────────────────────
// Migrations
// ─────────────────────────────────────────────────────────────────────────────

// migration is a single versioned SQL statement.
type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS test_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    test_name   TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    finished_at TEXT,
    passed      INTEGER
);

CREATE TABLE IF NOT EXISTS run_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      INTEGER NOT NULL REFERENCES test_runs(id),
    seq         INTEGER NOT NULL,
    occurred_at TEXT NOT NULL,
    elapsed_s   REAL NOT NULL,
    kind        TEXT NOT NULL,
    message     TEXT,
    details     TEXT
);

CREATE INDEX IF NOT EXISTS idx_run_events_run_id ON run_events(run_id);
CREATE INDEX IF NOT EXISTS idx_test_runs_started_at ON test_runs(started_at);
`,
	},
	{
		version: 2,
		sql: `
-- parent_id links a child event to its parent group event.
-- children stores a JSON array of {elapsed_s, kind, message, details} objects
-- on the parent row so the TUI never needs a second query.
ALTER TABLE run_events ADD COLUMN parent_id INTEGER REFERENCES run_events(id);
ALTER TABLE run_events ADD COLUMN children  TEXT;
CREATE INDEX IF NOT EXISTS idx_run_events_parent_id ON run_events(parent_id);
`,
	},
	{
		version: 3,
		sql: `
-- description stores a JSON object {"summary":"...","bullets":["...",...]} set
-- by RunLog.Describe() so the TUI can display it in the run-list inspector.
ALTER TABLE test_runs ADD COLUMN description TEXT;
`,
	},
	{
		version: 4,
		sql: `
-- tags stores a JSON array of "key:value" strings set by RunLog.Tag() so that
-- test runs that override settings (model, blueprint, etc.) can be compared.
-- experiment stores an optional experiment name/id set by RunLog.SetExperiment()
-- or automatically from the EXPERIMENT env var.
ALTER TABLE test_runs ADD COLUMN tags       TEXT;
ALTER TABLE test_runs ADD COLUMN experiment TEXT;
`,
	},
	{
		version: 5,
		sql: `
-- experiment_suggestions holds LLM-generated improvement suggestions for a
-- named experiment.  Each row is one suggestion with a priority, category,
-- title, body, and an optional list of run IDs that are most relevant.
CREATE TABLE IF NOT EXISTS experiment_suggestions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment     TEXT    NOT NULL,
    generated_at   TEXT    NOT NULL,
    category       TEXT    NOT NULL,
    priority       TEXT    NOT NULL,
    title          TEXT    NOT NULL,
    body           TEXT    NOT NULL,
    run_ids        TEXT    NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_exp_suggestions_experiment
    ON experiment_suggestions(experiment);
`,
	},
	{
		version: 6,
		sql: `
-- test_launchers tracks processes started by the runlog TUI via ./test.
-- launcher_pid is the OS PID of the go test process (bash execs into it).
-- finished_at is set when the process exits or is killed via the TUI.
-- Rows are never deleted so the TUI can show historical launch info.
CREATE TABLE IF NOT EXISTS test_launchers (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    test_name    TEXT    NOT NULL,
    env          TEXT    NOT NULL,
    launched_at  TEXT    NOT NULL,
    launcher_pid INTEGER NOT NULL,
    finished_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_test_launchers_test_name
    ON test_launchers(test_name);
`,
	},
	{
		version: 7,
		sql: `
-- runner records the execution environment: "host" (./test), "docker"
-- (docker-compose / run_tests.sh), or empty for older rows.
ALTER TABLE test_runs ADD COLUMN runner TEXT;
`,
	},
	{
		version: 8,
		sql: `
-- analyzer_traces stores one row per analysis run so we can replay
-- the full LLM conversation later without re-running the agent.
CREATE TABLE IF NOT EXISTS analyzer_traces (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    suggestion_key TEXT    NOT NULL,
    run_id         INTEGER,
    started_at     TEXT    NOT NULL,
    finished_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_analyzer_traces_run_id
    ON analyzer_traces(run_id);

-- analyzer_trace_events stores the individual conversation events
-- (system prompt, user message, thoughts, tool calls, etc.).
CREATE TABLE IF NOT EXISTS analyzer_trace_events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id  INTEGER NOT NULL REFERENCES analyzer_traces(id),
    seq       INTEGER NOT NULL,
    kind      TEXT    NOT NULL,
    author    TEXT,
    content   TEXT,
    details   TEXT
);
CREATE INDEX IF NOT EXISTS idx_analyzer_trace_events_trace_id
    ON analyzer_trace_events(trace_id);
`,
	},
	{
		version: 9,
		sql: `
-- reason stores a human-readable explanation of why the test was skipped or
-- failed.  For skips the message comes from rl.Skipf(); for failures it comes
-- from the last rl.Failf() event.  NULL for passes and older rows.
ALTER TABLE test_runs ADD COLUMN reason TEXT;
`,
	},
	{
		version: 10,
		sql: `
-- env_name stores the test environment profile name from MEMORY_TEST_ENV.
-- Used to distinguish runs executed in different environments (e.g. localhost,
-- mcj-emergent, etc.).  NULL for older rows.
ALTER TABLE test_runs ADD COLUMN env_name TEXT;
`,
	},
	{
		version: 11,
		sql: `
-- Token usage and cost tracking for LLM operations within tests.
-- These columns store aggregated token counts and estimated costs for all
-- LLM operations performed during a test run (e.g. memory ask, agent triggers).
-- NULL for tests that don't make LLM calls or older rows.
ALTER TABLE test_runs ADD COLUMN input_tokens  INTEGER;
ALTER TABLE test_runs ADD COLUMN output_tokens INTEGER;
ALTER TABLE test_runs ADD COLUMN cost_usd      REAL;
`,
	},
	{
		version: 12,
		sql: `
-- Environment variables used during the test run.
-- Stored as JSON object with key-value pairs (e.g. {"GOOGLE_AI_API_KEY": "AIza..."}).
-- Useful for tracking which API keys, server URLs, and other config was used.
ALTER TABLE test_runs ADD COLUMN env_vars TEXT;
`,
	},
	{
		version: 13,
		sql: `
-- daemon_runs tracks active test runs registered by the local runlog daemon.
-- Each row represents one "runlog test" invocation, keyed by PID.
-- status: active | done | dead
CREATE TABLE IF NOT EXISTS daemon_runs (
    id          TEXT    PRIMARY KEY,
    pid         INTEGER NOT NULL,
    env_profile TEXT,
    server_url  TEXT,
    token       TEXT,
    status      TEXT    NOT NULL DEFAULT 'active',
    started_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    finished_at TEXT
);

-- daemon_resources tracks server-side projects registered by test runs.
-- When a run dies without cleaning up, the sweeper deletes these resources.
-- status: active | deleted
CREATE TABLE IF NOT EXISTS daemon_resources (
    id          TEXT    PRIMARY KEY,
    run_id      TEXT    NOT NULL REFERENCES daemon_runs(id),
    resource_id TEXT    NOT NULL,
    resource_type TEXT  NOT NULL DEFAULT 'project',
    server_url  TEXT    NOT NULL,
    token       TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'active',
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    deleted_at  TEXT
);

CREATE INDEX IF NOT EXISTS idx_daemon_resources_run_id ON daemon_resources(run_id);
CREATE INDEX IF NOT EXISTS idx_daemon_resources_status ON daemon_resources(status);
`,
	},
	{
		version: 14,
		sql: `
-- skipped column was missing from the initial test_runs schema.
ALTER TABLE test_runs ADD COLUMN skipped INTEGER NOT NULL DEFAULT 0;
`,
	},
	{
		version: 15,
		sql: `
-- app_version stores the version of the application-under-test (set by test).
-- test_version stores a unique identifier for the test file contents (SHA256).
ALTER TABLE test_runs ADD COLUMN app_version  TEXT;
ALTER TABLE test_runs ADD COLUMN test_version TEXT;
`,
	},
	{
		version: 16,
		sql: `
-- daemon_run_id links test_runs to daemon_runs so that dogfood runs registered
-- via POST /runs appear in both the daemon monitor and the test dashboard.
ALTER TABLE test_runs ADD COLUMN daemon_run_id TEXT;
`,
	},
	{
		version: 17,
		sql: `
-- composite index on (test_name, started_at) to speed up tests list page,
-- which GROUP BY test_name and ORDER BY started_at DESC per test.
CREATE INDEX IF NOT EXISTS idx_test_runs_name_started ON test_runs(test_name, started_at DESC);
`,
	},
	{
		version: 18,
		sql: `
-- timeout_seconds stores the per-test timeout duration set by the test via
-- RunLog.SetTimeout().  NULL means no timeout.  The background timeout worker
-- compares started_at + timeout_seconds against the current time to detect
-- runs that have exceeded their deadline and marks them as timed out.
ALTER TABLE test_runs ADD COLUMN timeout_seconds REAL;
`,
	},
	{
		version: 19,
		sql: `
-- category stores the test's self-declared category set via RunLog.SetCategory().
-- NULL means no category was declared.  Used as the primary source for test
-- categorization in the web UI, falling back to config categories or directory name.
ALTER TABLE test_runs ADD COLUMN category TEXT;
`,
	},
	{
		version: 20,
		sql: `
CREATE TABLE IF NOT EXISTS linter_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    linter_name  TEXT NOT NULL,
    command      TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'running',
    exit_code    INTEGER,
    output       TEXT,
    started_at   TEXT NOT NULL,
    finished_at  TEXT
);
CREATE INDEX IF NOT EXISTS idx_linter_runs_name_started
    ON linter_runs(linter_name, started_at DESC);
`,
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// RunDB
// ─────────────────────────────────────────────────────────────────────────────

// RunDB is a handle to the SQLite runs database.
// All methods are safe for concurrent use from multiple goroutines.
type RunDB struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

// Path returns the filesystem path to the SQLite database file.
func (rdb *RunDB) Path() string {
	return rdb.path
}

// OpenDB opens (or creates) the SQLite database at path and applies any
// pending migrations.  The caller is responsible for closing it.
func OpenDB(path string) (*RunDB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("rundb: mkdir %s: %w", filepath.Dir(path), err)
	}
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("rundb: open %s: %w", path, err)
	}
	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)

	rdb := &RunDB{db: db, path: path}
	if err := rdb.applyMigrations(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return rdb, nil
}

// Close releases the database connection.
func (rdb *RunDB) Close() error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	return rdb.db.Close()
}

// RawDB returns the underlying *sql.DB handle.
// Use sparingly — caller is responsible for safe concurrent access.
// This is used by the daemon to execute queries directly against the DB.
func (rdb *RunDB) RawDB() *sql.DB {
	return rdb.db
}

// applyMigrations runs any migrations whose version is not yet in
// schema_migrations.  Each migration is applied in a single transaction.
func (rdb *RunDB) applyMigrations() error {
	// Bootstrap: create schema_migrations if it doesn't exist yet.
	_, err := rdb.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version    INTEGER PRIMARY KEY,
        applied_at TEXT NOT NULL
    )`)
	if err != nil {
		return fmt.Errorf("rundb: bootstrap migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		row := rdb.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version)
		if err := row.Scan(&exists); err != nil {
			return fmt.Errorf("rundb: check migration %d: %w", m.version, err)
		}
		if exists > 0 {
			continue
		}
		tx, err := rdb.db.Begin()
		if err != nil {
			return fmt.Errorf("rundb: begin migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("rundb: apply migration %d: %w", m.version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			m.version, time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("rundb: record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("rundb: commit migration %d: %w", m.version, err)
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Write helpers
// ─────────────────────────────────────────────────────────────────────────────

// InsertRun inserts a new test_runs row and returns its auto-assigned ID.
// runner identifies the execution environment (e.g. "host", "docker").
// Pass "" to leave the runner column NULL.
func (rdb *RunDB) InsertRun(testName string, startedAt time.Time, runner string, envName string, envVars map[string]string) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	var runnerVal any
	if runner != "" {
		runnerVal = runner
	}
	var envNameVal any
	if envName != "" {
		envNameVal = envName
	}
	var envVarsVal any
	if len(envVars) > 0 {
		b, err := json.Marshal(envVars)
		if err == nil {
			envVarsVal = string(b)
		}
	}
	res, err := rdb.db.Exec(
		`INSERT INTO test_runs(test_name, started_at, runner, env_name, env_vars) VALUES (?, ?, ?, ?, ?)`,
		testName, startedAt.UTC().Format(time.RFC3339Nano), runnerVal, envNameVal, envVarsVal,
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertRun: %w", err)
	}
	return res.LastInsertId()
}

// RunDescription is the structured description stored on a test_runs row.
type RunDescription struct {
	Summary string   `json:"summary"`
	Bullets []string `json:"bullets,omitempty"`
}

// UpdateRunDescription serialises desc as JSON and stores it on the test_runs
// row identified by id.  Best-effort: callers should log but not fail on error.
func (rdb *RunDB) UpdateRunDescription(id int64, desc RunDescription) error {
	b, err := json.Marshal(desc)
	if err != nil {
		return fmt.Errorf("rundb: UpdateRunDescription marshal: %w", err)
	}
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err = rdb.db.Exec(
		`UPDATE test_runs SET description = ? WHERE id = ?`, string(b), id,
	)
	return err
}

// UpdateRunTags serialises tags as a JSON array and stores it on the
// test_runs row identified by id.  Best-effort: callers should log but not
// fail on error.
func (rdb *RunDB) UpdateRunTags(id int64, tags []string) error {
	b, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("rundb: UpdateRunTags marshal: %w", err)
	}
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err = rdb.db.Exec(
		`UPDATE test_runs SET tags = ? WHERE id = ?`, string(b), id,
	)
	return err
}

// UpdateRunExperiment stores experiment on the test_runs row identified by id.
// Best-effort: callers should log but not fail on error.
func (rdb *RunDB) UpdateRunExperiment(id int64, experiment string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET experiment = ? WHERE id = ?`, experiment, id,
	)
	return err
}

// UpdateRunAppVersion stores app_version on the test_runs row identified by id.
func (rdb *RunDB) UpdateRunAppVersion(id int64, version string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET app_version = ? WHERE id = ?`, version, id,
	)
	return err
}

// UpdateRunTestVersion stores test_version on the test_runs row identified by id.
func (rdb *RunDB) UpdateRunTestVersion(id int64, version string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET test_version = ? WHERE id = ?`, version, id,
	)
	return err
}

// UpdateRunCategory stores the category on the test_runs row identified by id.
func (rdb *RunDB) UpdateRunCategory(id int64, category string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET category = ? WHERE id = ?`, category, id,
	)
	return err
}

// UpdateRunTimeout stores the timeout duration (in seconds) on the test_runs row.
func (rdb *RunDB) UpdateRunTimeout(id int64, timeoutSeconds float64) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET timeout_seconds = ? WHERE id = ?`, timeoutSeconds, id,
	)
	return err
}

// RunOutcome represents the final state of a test run.
type RunOutcome int

const (
	OutcomeFail    RunOutcome = 0 // test failed
	OutcomePass    RunOutcome = 1 // test passed
	OutcomeSkip    RunOutcome = 2 // test was skipped (t.Skip)
	OutcomeTimeout RunOutcome = 3 // test timed out
)

// FinishRun sets finished_at, passed, and optionally reason on an existing
// test_runs row.  outcome encodes the result: 0=fail, 1=pass, 2=skip.
// reason is a human-readable explanation (e.g. skip reason or last failure
// message); pass "" to leave the column NULL.
func (rdb *RunDB) FinishRun(id int64, finishedAt time.Time, outcome RunOutcome, reason string) error {
	return rdb.FinishRunWithCost(id, finishedAt, outcome, reason, 0, 0, 0)
}

// FinishRunWithCost is like FinishRun but also records token usage and cost.
// Pass 0 for inputTokens, outputTokens, costUSD to leave those columns NULL.
// If the run has no events, synthetic state_change events ("test started" /
// "test finished") are automatically inserted so the run always has a
// meaningful timeline.
func (rdb *RunDB) FinishRunWithCost(id int64, finishedAt time.Time, outcome RunOutcome, reason string, inputTokens, outputTokens int64, costUSD float64) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	// Check if run has any events — if not, insert synthetic ones.
	var eventCount int
	_ = rdb.db.QueryRow(`SELECT COUNT(*) FROM run_events WHERE run_id = ?`, id).Scan(&eventCount)
	if eventCount == 0 {
		var startedStr string
		if err := rdb.db.QueryRow(`SELECT started_at FROM test_runs WHERE id = ?`, id).Scan(&startedStr); err == nil {
			if started, err := time.Parse(time.RFC3339Nano, startedStr); err == nil {
				elapsed := finishedAt.Sub(started).Seconds()
				rdb.insertEventLocked(id, 1, started, 0, "state_change", "test started", nil, nil)
				rdb.insertEventLocked(id, 2, finishedAt, elapsed, "state_change", "test finished", nil, nil)
			}
		}
	}

	var reasonVal any
	if reason != "" {
		reasonVal = reason
	}
	var inputTokVal, outputTokVal, costVal any
	if inputTokens > 0 {
		inputTokVal = inputTokens
	}
	if outputTokens > 0 {
		outputTokVal = outputTokens
	}
	if costUSD > 0 {
		costVal = costUSD
	}
	_, err := rdb.db.Exec(
		`UPDATE test_runs SET finished_at = ?, passed = ?, reason = ?, input_tokens = ?, output_tokens = ?, cost_usd = ? WHERE id = ?`,
		finishedAt.UTC().Format(time.RFC3339Nano), int(outcome), reasonVal, inputTokVal, outputTokVal, costVal, id,
	)
	return err
}

// ListStaleRuns returns all runs where finished_at IS NULL — i.e. runs that
// were never properly closed, typically because the test process was killed.
func (rdb *RunDB) ListStaleRuns() ([]RunRow, error) {
	return rdb.ListRuns(time.Time{}, 0) // we filter in the caller
}

// ReapStaleRuns marks all unfinished runs (finished_at IS NULL) as failed with
// the given reason. It returns the number of rows updated.
func (rdb *RunDB) ReapStaleRuns(reason string) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	var reasonVal any
	if reason != "" {
		reasonVal = reason
	}
	res, err := rdb.db.Exec(
		`UPDATE test_runs SET finished_at = started_at, passed = ?, reason = ? WHERE finished_at IS NULL`,
		int(OutcomeFail), reasonVal,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertEvent appends one run_events row.
// details may be any JSON-serialisable value (struct, map, nil).
// If details is already a []byte or string it is stored verbatim as JSON.
func (rdb *RunDB) InsertEvent(
	runID int64,
	seq int,
	occurredAt time.Time,
	elapsedS float64,
	kind, message string,
	details any,
) error {
	return rdb.insertEvent(runID, seq, occurredAt, elapsedS, kind, message, details, nil)
}

// marshalDetailsJSON converts an arbitrary details value to a *string of JSON.
// Returns nil for nil or non-marshalable values.
func marshalDetailsJSON(details any) *string {
	if details == nil {
		return nil
	}
	var b []byte
	var err error
	switch v := details.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		b, err = json.Marshal(details)
		if err != nil {
			return nil
		}
	}
	s := string(b)
	return &s
}

// insertEvent is the internal implementation shared by InsertEvent and
// InsertChildEvent.  It acquires rdb.mu before inserting.
func (rdb *RunDB) insertEvent(
	runID int64,
	seq int,
	occurredAt time.Time,
	elapsedS float64,
	kind, message string,
	details any,
	parentID *int64,
) error {
	detailsJSON := marshalDetailsJSON(details)
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	return rdb.insertEventLocked(runID, seq, occurredAt, elapsedS, kind, message, detailsJSON, parentID)
}

// insertEventLocked is like insertEvent but assumes rdb.mu is already held.
// Callers that already hold the lock (e.g. FinishRunWithCost) must use this
// instead of insertEvent to avoid deadlock.
func (rdb *RunDB) insertEventLocked(
	runID int64,
	seq int,
	occurredAt time.Time,
	elapsedS float64,
	kind, message string,
	detailsJSON *string,
	parentID *int64,
) error {
	_, err := rdb.db.Exec(
		`INSERT INTO run_events(run_id, seq, occurred_at, elapsed_s, kind, message, details, parent_id)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID,
		seq,
		occurredAt.UTC().Format(time.RFC3339Nano),
		elapsedS,
		kind,
		message,
		detailsJSON,
		parentID,
	)
	return err
}

// InsertGroupEvent inserts a parent "group" event and returns its row ID.
// Callers call AppendGroupChildren once all children have been collected.
func (rdb *RunDB) InsertGroupEvent(
	runID int64,
	seq int,
	occurredAt time.Time,
	elapsedS float64,
	kind, message string,
) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	res, err := rdb.db.Exec(
		`INSERT INTO run_events(run_id, seq, occurred_at, elapsed_s, kind, message)
         VALUES (?, ?, ?, ?, ?, ?)`,
		runID, seq,
		occurredAt.UTC().Format(time.RFC3339Nano),
		elapsedS, kind, message,
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertGroupEvent: %w", err)
	}
	return res.LastInsertId()
}

// AppendGroupChildren serialises children as a JSON array into the parent's
// `children` column.  children is a []ChildEvent value.
func (rdb *RunDB) AppendGroupChildren(parentID int64, children []ChildEvent) error {
	if len(children) == 0 {
		return nil
	}
	b, err := json.Marshal(children)
	if err != nil {
		return fmt.Errorf("rundb: AppendGroupChildren marshal: %w", err)
	}
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err = rdb.db.Exec(
		`UPDATE run_events SET children = ? WHERE id = ?`, string(b), parentID,
	)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Read helpers (used by the TUI browser)
// ─────────────────────────────────────────────────────────────────────────────

// RunTokenSummary holds aggregated token usage for a test run, derived from
// the most recent "token_summary" event emitted by PrintTokenSummary.
type RunTokenSummary struct {
	TotalRuns    int
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// RunRow is a single row from test_runs.
type RunRow struct {
	ID           int64
	TestName     string
	StartedAt    time.Time
	FinishedAt   *time.Time // nil if still running
	Passed       *bool      // nil if not yet determined
	Skipped      bool       // true when the test called t.Skip() (passed column = 2)
	EventCount   int
	Description  *RunDescription   // nil if not set
	TokenSummary *RunTokenSummary  // nil if PrintTokenSummary was never called
	Tags         []string          // nil if no tags set; decoded from JSON array
	Experiment   *string           // nil if not set
	AppVersion   *string           // nil if not set; application-under-test version
	TestVersion  *string           // nil if not set; unique test file identifier
	Runner       *string           // nil for older rows; "host" or "docker"
	Reason       *string           // nil for passes and older rows; skip/fail reason
	EnvName      *string           // nil for older rows; environment profile name from MEMORY_TEST_ENV
	InputTokens  *int64            // nil if no LLM calls made; total input tokens consumed
	OutputTokens *int64            // nil if no LLM calls made; total output tokens generated
	CostUSD      *float64          // nil if no LLM calls made; estimated cost in USD
	EnvVars      map[string]string // nil if not set; environment variables used during test
	Category     *string           // nil if not set; self-declared via RunLog.SetCategory()
}

// ChildEvent is one entry in the `children` JSON array stored on a group event.
type ChildEvent struct {
	ElapsedS float64 `json:"elapsed_s"`
	Kind     string  `json:"kind"`
	Message  string  `json:"message"`
	Details  string  `json:"details,omitempty"` // raw JSON, empty if absent
}

// EventRow is a single row from run_events.
type EventRow struct {
	ID         int64
	RunID      int64
	Seq        int
	OccurredAt time.Time
	ElapsedS   float64
	Kind       string
	Message    string
	Details    *string      // raw JSON, nil if absent
	ParentID   *int64       // non-nil for child events
	Children   []ChildEvent // populated for group events
}

// ListRuns returns test_runs rows with started_at >= since, newest first.
// It also populates EventCount via a JOIN. If limit > 0, at most limit rows are returned.
func (rdb *RunDB) ListRuns(since time.Time, limit int) ([]RunRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	q := `SELECT
            r.id,
            r.test_name,
            r.started_at,
            r.finished_at,
            r.passed,
            COUNT(e.id) AS event_count,
            r.description,
            r.tags,
            r.experiment,
            r.runner,
            r.reason,
            r.env_name,
            r.input_tokens,
            r.output_tokens,
            r.cost_usd,
            r.env_vars,
            r.app_version,
            r.test_version,
            r.category
        FROM test_runs r
        LEFT JOIN run_events e ON e.run_id = r.id
        WHERE r.started_at >= ?
        GROUP BY r.id
        ORDER BY r.started_at DESC`
	args := []any{since.UTC().Format(time.RFC3339)}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := rdb.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListRuns: %w", err)
	}
	defer rows.Close()

	var result []RunRow
	for rows.Next() {
		var row RunRow
		var startedStr string
		var finishedStr *string
		var passedInt *int
		var descJSON *string
		var tagsJSON *string
		var experiment *string
		var runner *string
		var reason *string
		var envName *string
		var inputTokens *int64
		var outputTokens *int64
		var costUSD *float64
		var envVarsJSON *string
		var appVersion, testVersion *string
		if err := rows.Scan(
			&row.ID,
			&row.TestName,
			&startedStr,
			&finishedStr,
			&passedInt,
			&row.EventCount,
			&descJSON,
			&tagsJSON,
			&experiment,
			&runner,
			&reason,
			&envName,
			&inputTokens,
			&outputTokens,
			&costUSD,
			&envVarsJSON,
			&appVersion,
			&testVersion,
			&row.Category,
		); err != nil {
			return nil, fmt.Errorf("rundb: ListRuns scan: %w", err)
		}
		row.StartedAt, _ = time.Parse(time.RFC3339Nano, startedStr)
		if finishedStr != nil {
			t, _ := time.Parse(time.RFC3339Nano, *finishedStr)
			row.FinishedAt = &t
		}
		if passedInt != nil {
			if *passedInt == 2 {
				// skip: Passed stays nil (not determined as pass/fail), Skipped=true
				row.Skipped = true
			} else {
				p := *passedInt == 1
				row.Passed = &p
			}
		}
		if descJSON != nil && *descJSON != "" {
			var d RunDescription
			if json.Unmarshal([]byte(*descJSON), &d) == nil {
				row.Description = &d
			}
		}
		if tagsJSON != nil && *tagsJSON != "" {
			var tags []string
			if json.Unmarshal([]byte(*tagsJSON), &tags) == nil {
				row.Tags = tags
			}
		}
		row.Experiment = experiment
		row.Runner = runner
		row.Reason = reason
		row.EnvName = envName
		row.InputTokens = inputTokens
		row.OutputTokens = outputTokens
		row.CostUSD = costUSD
		row.AppVersion = appVersion
		row.TestVersion = testVersion
		if envVarsJSON != nil && *envVarsJSON != "" {
			var envVars map[string]string
			if json.Unmarshal([]byte(*envVarsJSON), &envVars) == nil {
				row.EnvVars = envVars
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Second pass: fetch the latest token_summary event details for each run.
	if len(result) > 0 {
		// Build a placeholder list for the IN clause.
		ids := make([]any, len(result))
		for i, r := range result {
			ids[i] = r.ID
		}
		placeholders := strings.Repeat("?,", len(ids))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

		tsRows, err := rdb.db.Query(
			`SELECT run_id, details
			 FROM run_events
			 WHERE kind = 'token_summary'
			   AND run_id IN (`+placeholders+`)
			 ORDER BY run_id, seq DESC`,
			ids...,
		)
		if err == nil {
			// Keep only the first (latest by DESC) row per run_id.
			seen := map[int64]bool{}
			for tsRows.Next() {
				var runID int64
				var detailsJSON *string
				if tsRows.Scan(&runID, &detailsJSON) == nil && !seen[runID] && detailsJSON != nil {
					seen[runID] = true
					var payload struct {
						TotalRuns    int     `json:"total_runs"`
						InputTokens  int64   `json:"input_tokens"`
						OutputTokens int64   `json:"output_tokens"`
						CostUSD      float64 `json:"cost_usd"`
					}
					if json.Unmarshal([]byte(*detailsJSON), &payload) == nil {
						ts := &RunTokenSummary{
							TotalRuns:    payload.TotalRuns,
							InputTokens:  payload.InputTokens,
							OutputTokens: payload.OutputTokens,
							CostUSD:      payload.CostUSD,
						}
						for i := range result {
							if result[i].ID == runID {
								result[i].TokenSummary = ts
								break
							}
						}
					}
				}
			}
			tsRows.Close()
		}
	}

	return result, nil
}

// ExperimentSummary holds aggregated information about one experiment.
type ExperimentSummary struct {
	Name         string    // experiment name
	RunCount     int       // total number of runs in this experiment
	PassCount    int       // number of passed runs
	FailCount    int       // number of failed runs
	SkipCount    int       // number of skipped runs
	LastRunAt    time.Time // most recent started_at across all runs
	Tags         []string  // union of all distinct tags across runs (sorted)
	Runs         []RunRow  // all runs belonging to this experiment (newest first)
	TotalCostUSD float64   // sum of TokenSummary.CostUSD across all runs
}

// DiscoverTests returns all distinct test names from the database, ordered
// alphabetically. This is used by the TUI to auto-populate the test list
// without requiring a hardcoded registry.
func (rdb *RunDB) DiscoverTests() ([]string, error) {
	rows, err := rdb.db.Query(`SELECT DISTINCT test_name FROM test_runs ORDER BY test_name`)
	if err != nil {
		return nil, fmt.Errorf("rundb: DiscoverTests: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("rundb: DiscoverTests scan: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// FailingTestRow holds the last-run info for a test whose most recent
// completed run within the query window was a failure.
type FailingTestRow struct {
	TestName   string
	LastRunAt  time.Time
	Reason     *string
	FailStreak int // consecutive failures (newest-first) within the window
}

// ListFailingTests returns one row per test whose most recent completed run
// (within the since window) was a failure, sorted by FailStreak DESC then
// LastRunAt DESC.  Streak is capped to runs within the since window.
func (rdb *RunDB) ListFailingTests(since time.Time) ([]FailingTestRow, error) {
	// Step 1: find tests whose last completed run is a failure.
	rdb.mu.Lock()
	rows, err := rdb.db.Query(`
		WITH ranked AS (
			SELECT test_name, started_at, passed, reason,
			       ROW_NUMBER() OVER (PARTITION BY test_name ORDER BY started_at DESC) AS rn
			FROM test_runs
			WHERE finished_at IS NOT NULL
			  AND started_at >= ?
		)
		SELECT test_name, started_at, reason
		FROM ranked
		WHERE rn = 1 AND passed = 0
		ORDER BY started_at DESC
	`, since.UTC().Format(time.RFC3339))
	rdb.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("rundb: ListFailingTests: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		testName string
		lastAt   time.Time
		reason   *string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var startedStr string
		if err := rows.Scan(&c.testName, &startedStr, &c.reason); err != nil {
			return nil, fmt.Errorf("rundb: ListFailingTests scan: %w", err)
		}
		c.lastAt, _ = time.Parse(time.RFC3339, startedStr)
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(candidates) == 0 {
		return nil, nil
	}

	// Step 2: for each failing test, count consecutive failures from newest.
	result := make([]FailingTestRow, 0, len(candidates))
	for _, c := range candidates {
		rdb.mu.Lock()
		srows, err := rdb.db.Query(`
			SELECT passed FROM test_runs
			WHERE test_name = ? AND finished_at IS NOT NULL AND started_at >= ?
			ORDER BY started_at DESC
		`, c.testName, since.UTC().Format(time.RFC3339))
		rdb.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("rundb: ListFailingTests streak %s: %w", c.testName, err)
		}
		streak := 0
		for srows.Next() {
			var passed int
			if err := srows.Scan(&passed); err != nil {
				srows.Close()
				return nil, fmt.Errorf("rundb: ListFailingTests streak scan: %w", err)
			}
			if passed != 0 { // pass or skip breaks the streak
				break
			}
			streak++
		}
		srows.Close()

		result = append(result, FailingTestRow{
			TestName:   c.testName,
			LastRunAt:  c.lastAt,
			Reason:     c.reason,
			FailStreak: streak,
		})
	}

	// Sort by streak DESC, then LastRunAt DESC.
	for i := 1; i < len(result); i++ {
		for j := i; j > 0; j-- {
			a, b := result[j-1], result[j]
			if a.FailStreak < b.FailStreak ||
				(a.FailStreak == b.FailStreak && a.LastRunAt.Before(b.LastRunAt)) {
				result[j-1], result[j] = result[j], result[j-1]
			} else {
				break
			}
		}
	}

	return result, nil
}

// TestStatRow holds aggregated run statistics for a single test.
type TestStatRow struct {
	TestName     string
	TotalRuns    int
	PassCount    int
	FailCount    int
	SkipCount    int
	AvgDurationS float64
	MinDurationS float64
	MaxDurationS float64
	LastRunAt    time.Time
	LastPassed   *bool // nil = last run was a skip or outcome unknown
}

// TestStats returns per-test aggregated statistics for runs within the since
// window, ordered alphabetically by test name.
func (rdb *RunDB) TestStats(since time.Time) ([]TestStatRow, error) {
	rdb.mu.Lock()
	rows, err := rdb.db.Query(`
		SELECT
			test_name,
			COUNT(*) AS total_runs,
			SUM(CASE WHEN passed = 1 THEN 1 ELSE 0 END) AS pass_count,
			SUM(CASE WHEN passed = 0 THEN 1 ELSE 0 END) AS fail_count,
			SUM(CASE WHEN passed = 2 THEN 1 ELSE 0 END) AS skip_count,
			AVG((julianday(finished_at) - julianday(started_at)) * 86400.0) AS avg_dur,
			MIN((julianday(finished_at) - julianday(started_at)) * 86400.0) AS min_dur,
			MAX((julianday(finished_at) - julianday(started_at)) * 86400.0) AS max_dur,
			MAX(started_at) AS last_run_at
		FROM test_runs
		WHERE finished_at IS NOT NULL
		  AND started_at >= ?
		GROUP BY test_name
		ORDER BY test_name
	`, since.UTC().Format(time.RFC3339))
	rdb.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("rundb: TestStats: %w", err)
	}
	defer rows.Close()

	var result []TestStatRow
	for rows.Next() {
		var r TestStatRow
		var lastRunStr string
		var avgDur, minDur, maxDur *float64
		if err := rows.Scan(
			&r.TestName,
			&r.TotalRuns,
			&r.PassCount,
			&r.FailCount,
			&r.SkipCount,
			&avgDur,
			&minDur,
			&maxDur,
			&lastRunStr,
		); err != nil {
			return nil, fmt.Errorf("rundb: TestStats scan: %w", err)
		}
		r.LastRunAt, _ = time.Parse(time.RFC3339, lastRunStr)
		if avgDur != nil {
			r.AvgDurationS = *avgDur
		}
		if minDur != nil {
			r.MinDurationS = *minDur
		}
		if maxDur != nil {
			r.MaxDurationS = *maxDur
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Determine LastPassed for each test via a second query.
	for i := range result {
		rdb.mu.Lock()
		row := rdb.db.QueryRow(`
			SELECT passed FROM test_runs
			WHERE test_name = ? AND finished_at IS NOT NULL AND started_at >= ?
			ORDER BY started_at DESC LIMIT 1
		`, result[i].TestName, since.UTC().Format(time.RFC3339))
		rdb.mu.Unlock()
		var passed int
		if err := row.Scan(&passed); err == nil {
			if passed == 1 {
				v := true
				result[i].LastPassed = &v
			} else if passed == 0 {
				v := false
				result[i].LastPassed = &v
			}
			// passed==2 (skip) → leave nil
		}
	}

	return result, nil
}

// ListExperiments returns one ExperimentSummary per distinct non-null
// experiment value across all test_runs rows, sorted newest-first by LastRunAt.
// It re-uses ListRuns with a zero time to get every run, then groups in memory.
func (rdb *RunDB) ListExperiments() ([]ExperimentSummary, error) {
	rows, err := rdb.ListRuns(time.Time{}, 0)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListExperiments: %w", err)
	}

	// group by experiment name, preserving insertion (newest-first) order for
	// the first occurrence of each name.
	order := []string{}
	byName := map[string]*ExperimentSummary{}
	for _, r := range rows {
		if r.Experiment == nil {
			continue
		}
		name := *r.Experiment
		if _, exists := byName[name]; !exists {
			byName[name] = &ExperimentSummary{Name: name}
			order = append(order, name)
		}
		s := byName[name]
		s.RunCount++
		if r.Skipped {
			s.SkipCount++
		} else if r.Passed != nil {
			if *r.Passed {
				s.PassCount++
			} else {
				s.FailCount++
			}
		}
		if r.StartedAt.After(s.LastRunAt) {
			s.LastRunAt = r.StartedAt
		}
		s.Runs = append(s.Runs, r)
		if r.TokenSummary != nil {
			s.TotalCostUSD += r.TokenSummary.CostUSD
		}
		// collect distinct tags
		for _, t := range r.Tags {
			found := false
			for _, existing := range s.Tags {
				if existing == t {
					found = true
					break
				}
			}
			if !found {
				s.Tags = append(s.Tags, t)
			}
		}
	}

	result := make([]ExperimentSummary, 0, len(order))
	for _, name := range order {
		result = append(result, *byName[name])
	}
	return result, nil
}

// SuggestionRow is a single row from experiment_suggestions.
type SuggestionRow struct {
	ID          int64
	Experiment  string
	GeneratedAt time.Time
	Category    string
	Priority    string
	Title       string
	Body        string
	RunIDs      []int64
}

// InsertSuggestion appends one suggestion row for the given experiment and
// returns its auto-assigned ID.  runIDs is serialised as a JSON array.
func (rdb *RunDB) InsertSuggestion(experiment, title, body, category, priority string, runIDs []int64) (int64, error) {
	if runIDs == nil {
		runIDs = []int64{}
	}
	runIDsJSON, err := json.Marshal(runIDs)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertSuggestion marshal run_ids: %w", err)
	}
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	res, err := rdb.db.Exec(
		`INSERT INTO experiment_suggestions(experiment, generated_at, category, priority, title, body, run_ids)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		experiment,
		time.Now().UTC().Format(time.RFC3339),
		category,
		priority,
		title,
		body,
		string(runIDsJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertSuggestion: %w", err)
	}
	return res.LastInsertId()
}

// ListSuggestions returns all suggestion rows for the given experiment,
// ordered by id ASC (insertion order).
func (rdb *RunDB) ListSuggestions(experiment string) ([]SuggestionRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	rows, err := rdb.db.Query(`
		SELECT id, experiment, generated_at, category, priority, title, body, run_ids
		FROM experiment_suggestions
		WHERE experiment = ?
		ORDER BY id ASC
	`, experiment)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListSuggestions: %w", err)
	}
	defer rows.Close()

	var result []SuggestionRow
	for rows.Next() {
		var row SuggestionRow
		var generatedStr string
		var runIDsJSON string
		if err := rows.Scan(
			&row.ID,
			&row.Experiment,
			&generatedStr,
			&row.Category,
			&row.Priority,
			&row.Title,
			&row.Body,
			&runIDsJSON,
		); err != nil {
			return nil, fmt.Errorf("rundb: ListSuggestions scan: %w", err)
		}
		row.GeneratedAt, _ = time.Parse(time.RFC3339, generatedStr)
		_ = json.Unmarshal([]byte(runIDsJSON), &row.RunIDs)
		if row.RunIDs == nil {
			row.RunIDs = []int64{}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// DeleteSuggestions removes all suggestion rows for the given experiment.
func (rdb *RunDB) DeleteSuggestions(experiment string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(`DELETE FROM experiment_suggestions WHERE experiment = ?`, experiment)
	if err != nil {
		return fmt.Errorf("rundb: DeleteSuggestions: %w", err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Analyzer trace persistence
// ─────────────────────────────────────────────────────────────────────────────

// AnalyzerTraceRow is a single row from analyzer_traces.
type AnalyzerTraceRow struct {
	ID            int64
	SuggestionKey string
	RunID         *int64
	StartedAt     time.Time
	FinishedAt    *time.Time
}

// InsertAnalyzerTrace creates a new trace row and returns its ID.
func (rdb *RunDB) InsertAnalyzerTrace(suggestionKey string, runID *int64) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	var runIDVal interface{}
	if runID != nil {
		runIDVal = *runID
	}
	res, err := rdb.db.Exec(
		`INSERT INTO analyzer_traces(suggestion_key, run_id, started_at) VALUES (?, ?, ?)`,
		suggestionKey, runIDVal, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertAnalyzerTrace: %w", err)
	}
	return res.LastInsertId()
}

// FinishAnalyzerTrace sets the finished_at timestamp on a trace row.
func (rdb *RunDB) FinishAnalyzerTrace(traceID int64) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE analyzer_traces SET finished_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), traceID,
	)
	if err != nil {
		return fmt.Errorf("rundb: FinishAnalyzerTrace: %w", err)
	}
	return nil
}

// traceEventDetails is the JSON blob stored in the details column.
type traceEventDetails struct {
	ToolName      string         `json:"tool_name,omitempty"`
	ToolArgs      map[string]any `json:"tool_args,omitempty"`
	ToolResponse  map[string]any `json:"tool_response,omitempty"`
	PromptTokens  int32          `json:"prompt_tokens,omitempty"`
	OutputTokens  int32          `json:"output_tokens,omitempty"`
	ThoughtTokens int32          `json:"thought_tokens,omitempty"`
	TotalTokens   int32          `json:"total_tokens,omitempty"`
	ErrorCode     string         `json:"error_code,omitempty"`
	ErrorMessage  string         `json:"error_message,omitempty"`
}

// InsertAnalyzerTraceEvent appends one event to a trace.
func (rdb *RunDB) InsertAnalyzerTraceEvent(traceID int64, seq int, ev AnalyzerEvent) error {
	det := traceEventDetails{
		ToolName:      ev.ToolName,
		ToolArgs:      ev.ToolArgs,
		ToolResponse:  ev.ToolResponse,
		PromptTokens:  ev.PromptTokens,
		OutputTokens:  ev.OutputTokens,
		ThoughtTokens: ev.ThoughtTokens,
		TotalTokens:   ev.TotalTokens,
		ErrorCode:     ev.ErrorCode,
		ErrorMessage:  ev.ErrorMessage,
	}
	detJSON, err := json.Marshal(det)
	if err != nil {
		return fmt.Errorf("rundb: InsertAnalyzerTraceEvent marshal: %w", err)
	}
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err = rdb.db.Exec(
		`INSERT INTO analyzer_trace_events(trace_id, seq, kind, author, content, details) VALUES (?, ?, ?, ?, ?, ?)`,
		traceID, seq, string(ev.Kind), ev.Author, ev.Content, string(detJSON),
	)
	if err != nil {
		return fmt.Errorf("rundb: InsertAnalyzerTraceEvent: %w", err)
	}
	return nil
}

// GetLatestTraceForRun returns the most recent trace ID for a given run ID,
// or 0 if none exists.
func (rdb *RunDB) GetLatestTraceForRun(runID int64) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	var traceID int64
	err := rdb.db.QueryRow(
		`SELECT id FROM analyzer_traces WHERE run_id = ? ORDER BY id DESC LIMIT 1`,
		runID,
	).Scan(&traceID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("rundb: GetLatestTraceForRun: %w", err)
	}
	return traceID, nil
}

// ListTracesForRun returns all traces for a given run ID, ordered newest first.
func (rdb *RunDB) ListTracesForRun(runID int64) ([]AnalyzerTraceRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	rows, err := rdb.db.Query(`
		SELECT id, suggestion_key, run_id, started_at, finished_at
		FROM analyzer_traces
		WHERE run_id = ?
		ORDER BY id DESC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListTracesForRun: %w", err)
	}
	defer rows.Close()

	var result []AnalyzerTraceRow
	for rows.Next() {
		var tr AnalyzerTraceRow
		var startedStr string
		var finishedStr *string
		if err := rows.Scan(&tr.ID, &tr.SuggestionKey, &tr.RunID, &startedStr, &finishedStr); err != nil {
			return nil, fmt.Errorf("rundb: ListTracesForRun scan: %w", err)
		}
		if t, err := time.Parse(time.RFC3339Nano, startedStr); err == nil {
			tr.StartedAt = t
		}
		if finishedStr != nil {
			if t, err := time.Parse(time.RFC3339Nano, *finishedStr); err == nil {
				tr.FinishedAt = &t
			}
		}
		result = append(result, tr)
	}
	return result, rows.Err()
}

// ListAnalyzerTraceEvents returns all events for a trace, ordered by seq.
func (rdb *RunDB) ListAnalyzerTraceEvents(traceID int64) ([]AnalyzerEvent, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	rows, err := rdb.db.Query(`
		SELECT kind, author, content, details
		FROM analyzer_trace_events
		WHERE trace_id = ?
		ORDER BY seq ASC
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListAnalyzerTraceEvents: %w", err)
	}
	defer rows.Close()

	var result []AnalyzerEvent
	for rows.Next() {
		var kindStr, author, content, detailsJSON string
		if err := rows.Scan(&kindStr, &author, &content, &detailsJSON); err != nil {
			return nil, fmt.Errorf("rundb: ListAnalyzerTraceEvents scan: %w", err)
		}
		ev := AnalyzerEvent{
			Kind:    AnalyzerEventKind(kindStr),
			Author:  author,
			Content: content,
		}
		if detailsJSON != "" {
			var det traceEventDetails
			if err := json.Unmarshal([]byte(detailsJSON), &det); err == nil {
				ev.ToolName = det.ToolName
				ev.ToolArgs = det.ToolArgs
				ev.ToolResponse = det.ToolResponse
				ev.PromptTokens = det.PromptTokens
				ev.OutputTokens = det.OutputTokens
				ev.ThoughtTokens = det.ThoughtTokens
				ev.TotalTokens = det.TotalTokens
				ev.ErrorCode = det.ErrorCode
				ev.ErrorMessage = det.ErrorMessage
			}
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

// DeleteAnalyzerTraces removes all traces and their events for a given
// suggestion key (e.g. "run:42").
func (rdb *RunDB) DeleteAnalyzerTraces(suggestionKey string) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	// Delete events first (foreign key).
	_, err := rdb.db.Exec(`
		DELETE FROM analyzer_trace_events
		WHERE trace_id IN (SELECT id FROM analyzer_traces WHERE suggestion_key = ?)
	`, suggestionKey)
	if err != nil {
		return fmt.Errorf("rundb: DeleteAnalyzerTraces events: %w", err)
	}
	_, err = rdb.db.Exec(`DELETE FROM analyzer_traces WHERE suggestion_key = ?`, suggestionKey)
	if err != nil {
		return fmt.Errorf("rundb: DeleteAnalyzerTraces: %w", err)
	}
	return nil
}

// HasSkipEvent returns true if the run has at least one 'skip' event.
func (rdb *RunDB) HasSkipEvent(runID int64) bool {
	var count int
	_ = rdb.db.QueryRow(`SELECT COUNT(*) FROM run_events WHERE run_id = ? AND kind = 'skip'`, runID).Scan(&count)
	return count > 0
}

// ListEvents returns top-level run_events for a given run (parent_id IS NULL),
// ordered by seq.  Children are decoded from the parent's `children` column.
func (rdb *RunDB) ListEvents(runID int64) ([]EventRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	rows, err := rdb.db.Query(`
        SELECT id, run_id, seq, occurred_at, elapsed_s, kind, message, details, parent_id, children
        FROM run_events
        WHERE run_id = ? AND parent_id IS NULL
        ORDER BY seq
    `, runID)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListEvents: %w", err)
	}
	defer rows.Close()

	var result []EventRow
	for rows.Next() {
		var row EventRow
		var occurredStr string
		var childrenJSON *string
		if err := rows.Scan(
			&row.ID,
			&row.RunID,
			&row.Seq,
			&occurredStr,
			&row.ElapsedS,
			&row.Kind,
			&row.Message,
			&row.Details,
			&row.ParentID,
			&childrenJSON,
		); err != nil {
			return nil, fmt.Errorf("rundb: ListEvents scan: %w", err)
		}
		row.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredStr)
		if childrenJSON != nil && *childrenJSON != "" {
			_ = json.Unmarshal([]byte(*childrenJSON), &row.Children)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Launcher helpers (test_launchers table)
// ─────────────────────────────────────────────────────────────────────────────

// LauncherRow is a single row from test_launchers.
type LauncherRow struct {
	ID          int64
	TestName    string
	Env         string
	LaunchedAt  time.Time
	LauncherPID int
	FinishedAt  *time.Time // nil if the process has not yet finished
}

// InsertLauncher records a newly-started ./test process and returns its row ID.
func (rdb *RunDB) InsertLauncher(testName, env string, launchedAt time.Time, pid int) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	res, err := rdb.db.Exec(
		`INSERT INTO test_launchers(test_name, env, launched_at, launcher_pid)
		 VALUES (?, ?, ?, ?)`,
		testName, env,
		launchedAt.UTC().Format(time.RFC3339Nano),
		pid,
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertLauncher: %w", err)
	}
	return res.LastInsertId()
}

// FinishLauncher marks a launcher row as finished (process exited or was killed).
func (rdb *RunDB) FinishLauncher(id int64, finishedAt time.Time) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE test_launchers SET finished_at = ? WHERE id = ?`,
		finishedAt.UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// ListLaunchers returns launcher rows for the given test name, most-recent first.
// If testName is empty, all rows are returned.
func (rdb *RunDB) ListLaunchers(testName string) ([]LauncherRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()

	var (
		q    string
		args []any
	)
	if testName == "" {
		q = `SELECT id, test_name, env, launched_at, launcher_pid, finished_at
		     FROM test_launchers ORDER BY launched_at DESC`
	} else {
		q = `SELECT id, test_name, env, launched_at, launcher_pid, finished_at
		     FROM test_launchers WHERE test_name = ? ORDER BY launched_at DESC`
		args = []any{testName}
	}

	rows, err := rdb.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListLaunchers: %w", err)
	}
	defer rows.Close()

	var result []LauncherRow
	for rows.Next() {
		var r LauncherRow
		var launchedStr string
		var finishedStr *string
		if err := rows.Scan(&r.ID, &r.TestName, &r.Env, &launchedStr, &r.LauncherPID, &finishedStr); err != nil {
			return nil, fmt.Errorf("rundb: ListLaunchers scan: %w", err)
		}
		r.LaunchedAt, _ = time.Parse(time.RFC3339Nano, launchedStr)
		if finishedStr != nil && *finishedStr != "" {
			t, _ := time.Parse(time.RFC3339Nano, *finishedStr)
			r.FinishedAt = &t
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Linter runs
// ─────────────────────────────────────────────────────────────────────────────

// LinterRow is a single row from linter_runs.
type LinterRow struct {
	ID         int64
	LinterName string
	Command    string
	Status     string // running / passed / failed / error
	ExitCode   *int
	Output     string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// InsertLinterRun inserts a new linter_runs row and returns its ID.
func (rdb *RunDB) InsertLinterRun(linterName, command string, startedAt time.Time) (int64, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	res, err := rdb.db.Exec(
		`INSERT INTO linter_runs(linter_name, command, started_at) VALUES (?, ?, ?)`,
		linterName, command, startedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("rundb: InsertLinterRun: %w", err)
	}
	return res.LastInsertId()
}

// UpdateLinterRunResult sets the final result of a linter run.
// status: "passed", "failed", or "error".
func (rdb *RunDB) UpdateLinterRunResult(id int64, status string, exitCode int, output string, finishedAt time.Time) error {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	_, err := rdb.db.Exec(
		`UPDATE linter_runs SET status = ?, exit_code = ?, output = ?, finished_at = ? WHERE id = ?`,
		status, exitCode, output, finishedAt.UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

// ScanLinterRow scans a linter_runs row from the current cursor position.
// Columns in order: id, linter_name, command, status, exit_code, output, started_at, finished_at.
func scanLinterRow(scanner interface{ Scan(...any) error }) (LinterRow, error) {
	var r LinterRow
	var startedStr, status string
	var finishedStr *string
	err := scanner.Scan(&r.ID, &r.LinterName, &r.Command, &status, &r.ExitCode, &r.Output, &startedStr, &finishedStr)
	if err != nil {
		return r, err
	}
	r.Status = status
	r.StartedAt, _ = time.Parse(time.RFC3339Nano, startedStr)
	if finishedStr != nil && *finishedStr != "" {
		t, _ := time.Parse(time.RFC3339Nano, *finishedStr)
		r.FinishedAt = &t
	}
	return r, nil
}

// ListLinterRuns returns the latest run for each unique linter name.
func (rdb *RunDB) ListLinterRuns() ([]LinterRow, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	rows, err := rdb.db.Query(`
		SELECT id, linter_name, command, status, exit_code, output, started_at, finished_at
		FROM linter_runs
		WHERE id IN (SELECT MAX(id) FROM linter_runs GROUP BY linter_name)
		ORDER BY linter_name`)
	if err != nil {
		return nil, fmt.Errorf("rundb: ListLinterRuns: %w", err)
	}
	defer rows.Close()
	var result []LinterRow
	for rows.Next() {
		r, err := scanLinterRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListLinterRunHistory returns paginated run history for a specific linter.
func (rdb *RunDB) ListLinterRunHistory(name string, offset, limit int) ([]LinterRow, int, error) {
	rdb.mu.Lock()
	defer rdb.mu.Unlock()
	var total int
	_ = rdb.db.QueryRow(`SELECT COUNT(*) FROM linter_runs WHERE linter_name = ?`, name).Scan(&total)
	rows, err := rdb.db.Query(`
		SELECT id, linter_name, command, status, exit_code, output, started_at, finished_at
		FROM linter_runs
		WHERE linter_name = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?`, name, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("rundb: ListLinterRunHistory: %w", err)
	}
	defer rows.Close()
	var result []LinterRow
	for rows.Next() {
		r, err := scanLinterRow(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, r)
	}
	return result, total, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Runner detection
// ─────────────────────────────────────────────────────────────────────────────

// Runner returns the test execution environment label.
// Resolution:
//  1. $TEST_RUNNER env var (e.g. "docker", "ci")
//  2. "docker" if running inside Docker (CI=true && /test-logs exists)
//  3. "host"
func Runner() string {
	if r := os.Getenv("TEST_RUNNER"); r != "" {
		return r
	}
	// Heuristic: inside Docker the Dockerfile creates /test-logs and sets CI=true.
	if os.Getenv("CI") == "true" {
		if _, err := os.Stat("/test-logs"); err == nil {
			return "docker"
		}
	}
	return "host"
}

// ─────────────────────────────────────────────────────────────────────────────
// Process-wide singleton
// ─────────────────────────────────────────────────────────────────────────────

var (
	globalDBOnce sync.Once
	globalDB     *RunDB
	globalDBErr  error
	globalDBMu   sync.Mutex // protects reset
)

// resetSharedDB resets the singleton so the next SharedDB() call re-opens the
// DB.  Only intended for use in tests that need to redirect the DB path via
// TEST_RUNS_DB between calls.
func resetSharedDB() {
	globalDBMu.Lock()
	defer globalDBMu.Unlock()
	if globalDB != nil {
		_ = globalDB.Close()
		globalDB = nil
	}
	globalDBErr = nil
	globalDBOnce = sync.Once{}
}

// SharedDB returns the process-wide RunDB, opening it lazily on first call.
// The path resolves via dbPath():
//  1. $TEST_RUNS_DB         (explicit override)
//  2. <repo_root>/.runlog/runs.db  (via runtime.Caller)
//  3. $TMPDIR/.../runs.db   (last-resort fallback)
//
// If the DB cannot be opened, a nil handle is returned and the error is
// stored; subsequent calls return the same nil + error.
func SharedDB() (*RunDB, error) {
	globalDBOnce.Do(func() {
		path := dbPath()
		globalDB, globalDBErr = OpenDB(path)
	})
	return globalDB, globalDBErr
}

// dbPath resolves the path for runs.db.
//
// Resolution order:
//  1. TEST_RUNS_DB env var            — explicit override (absolute path)
//  2. config file db: field           — from config.yaml in .runlog directory
//  3. .runlog/runs.db                 — in the project's .runlog directory
//  4. $TMPDIR/.../runs.db             — last-resort fallback
//
// NOTE: TEST_LOG_DIR is intentionally NOT consulted here.  Flat log files
// (run.log, session-*.log) live under TEST_LOG_DIR, but the DB is always
// at the project root so Docker and host runs share a single database.
func dbPath() string {
	if d := os.Getenv("TEST_RUNS_DB"); d != "" {
		return d
	}
	// Check config for an explicit db: field.
	if cfg, err := LoadConfig(""); err == nil && cfg.DBPath != "" {
		return cfg.DBPath
	}

	// Default: create in .runlog/ under the project root.
	runlogDir := RunlogDir()
	if runlogDir != "" {
		return filepath.Join(runlogDir, "runs.db")
	}

	return filepath.Join(os.TempDir(), "memory-cli-docker-tests", "runs.db")
}
