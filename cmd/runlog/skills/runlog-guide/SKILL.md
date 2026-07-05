---
name: runlog-guide
description: Comprehensive reference for the runlog Go library and CLI. Covers architecture, all three API tiers (RunLog/TestContext/Fixture), chainable assertions, event model, SQLite schema, config file, CLI commands, and common workflows. Load this when writing tests, inspecting runs, auditing quality, or extending runlog.
metadata:
  author: emergent
  version: "1.0"
---

# runlog — LLM Reference Guide

`github.com/emergent-company/runlog` — terminal-native test observability for Go.

Structured logging + SQLite run history + interactive TUI + LLM-powered analysis. Zero CGO (pure-Go SQLite).

---

## Architecture Overview

```
Your Go test
  └─ runlog.NewRunLog(t)           ← Go library (github.com/emergent-company/runlog)
       ├─ logs/<ts>-<TestName>/    ← flat log files (TEST_LOG_DIR or source file dir)
       │    └─ run.log             ← chronological plain-text log
       └─ .runlog/runs.db          ← SQLite database (WAL mode)
            ├─ test_runs           ← one row per test run
            └─ run_events          ← one row per event (sections, CLI, failures, ...)

runlog (binary)                    ← TUI + CLI (cmd/runlog)
  ├─ TUI          interactive browser of runs.db
  ├─ runlog runs  list recent runs
  ├─ runlog show  dump a run
  ├─ runlog inspect  full event timeline
  ├─ runlog analyze  LLM analysis via Gemini
  └─ runlog test  run tests with env profile support
```

**DB path resolution** (priority order):
1. `RUNLOG_DB` environment variable
2. `.runlog/config.yaml` → `db:` key
3. Walk up from cwd to find an existing `.runlog/` directory
4. Walk up from cwd to find `.git/` — place `.runlog/` there
5. Fallback: `.runlog/runs.db` in cwd

---

## Three API Tiers

| API | Entry point | When to use |
|-----|-------------|-------------|
| **RunLog** | `runlog.NewRunLog(t)` | Simple structured logging; no server/CLI dependency |
| **TestContext** | `runlog.NewTest(t, opts)` | Multi-step tests with CLI + HTTP calls; automatic project lifecycle |
| **Fixture** | `runlog.Use(t, opts...)` | Declarative minimal-boilerplate; automatic server check + project |

---

## 1. RunLog — Structured Logging

### Construction and lifecycle

```go
rl := runlog.NewRunLog(t)   // opens logs/<ts>-<TestName>/run.log + registers DB row
defer rl.Close()            // preferred: flushes, closes file, writes outcome to DB
// OR: t.Cleanup(rl.Close)  // more robust (runs even on panic)
```

`NewRunLog` automatically:
- Creates `logs/<timestamp>-<TestName>/run.log`
- Inserts a `test_runs` row in `runs.db`
- Computes SHA-256 of the test source file → `test_version` column
- Captures git commit hash of the test file
- Reads `EXPERIMENT` env var → `experiment` column
- Captures tracked env vars (API keys, server URLs) → `env_vars` column
- Auto-derives `test_type` from source file path (`e2e`, `integration`, `benchmark`, `unit`, `other`)

If `RUNLOG_RUN_ID` is set, `NewRunLog` reuses that existing DB row instead of creating a new one (used by `runlog test` to link test process runs to the daemon run).

### All RunLog methods

```go
// Description (call once, immediately after NewRunLog)
rl.Describe(summary string, bullets ...string)

// Logging
rl.Section(name string)                          // collapsible section header in TUI
rl.Printf(format string, args ...any)            // timestamped log line
rl.LogStep(label string, details map[string]any) // log line with structured key-value details
rl.AssertionStep(label string, expected, actual any, extra map[string]any) // assertion event

// CLI logging
rl.CLI(invocation, output string)                         // "$ <invocation>" message
rl.CLIErr(invocation, output string, err error)           // like CLI, records exit code
rl.CLIStep(desc, invocation, output string)               // custom description (not "$ ...")
rl.CLIStepErr(desc, invocation, output string, err error) // custom desc + error
rl.MustRunCLI(t *testing.T, args ...string) string        // runs "memory <args>" + logs
rl.MustRunCLIInDir(t *testing.T, dir string, args ...string) string

// Events
rl.Event(kind, message string, details any)   // custom structured event (any JSON-serializable details)
rl.Group(kind, title string, fn func(g *GroupLogger)) // collapsible group with child lines

// Failure / skip
rl.Failf(format string, args ...any)   // logs failure event + t.Fatal (use instead of t.Fatalf)
rl.Skipf(format string, args ...any)   // logs skip event + t.Skip
runlog.DoSkipf(t, rl, format, args...) // nil-safe: uses rl.Skipf if rl != nil, else t.Skipf

// Metadata
rl.Tag(tags ...string)                            // "key:value" variant tags
rl.SetExperiment(name string)                     // assign to experiment batch
rl.SetAppVersion(version string)                  // application-under-test version
rl.SetTestVersion(version string)                 // override auto-detected test file SHA256
rl.SetCategory(category string)                   // self-declare category (overrides config)
rl.SetTestType(testType string)                   // override auto-derived test type
rl.SetTimeout(d time.Duration)                    // per-test timeout for daemon monitor
rl.SetCoverage(pct float64, data string)          // coverage % + per-function JSON

// Token / cost tracking
rl.RecordTokenUsage(inputTokens, outputTokens int64, costUSD float64)
rl.PrintTokenSummary(intervals []runlog.AgentRunInterval) // writes token_summary event
// package-level helpers:
runlog.PrintGantt(rl *RunLog, intervals []AgentRunInterval)
runlog.PrintTokenSummary(rl *RunLog, intervals []AgentRunInterval)

// Tracing
rl.StartTracePoller(serverURL, token, projectID string) // polls Tempo proxy for trace spans

// Accessor
rl.Dir() string  // path to the log directory (for sibling detail files)
```

### Canonical test structure

```go
func TestFeature_Scenario(t *testing.T) {
    // 1. RunLog FIRST — always, before any skip guards
    rl := runlog.NewRunLog(t)
    t.Cleanup(rl.Close)
    rl.Describe("One-line summary",
        "Bullet: what this test sets up",
        "Bullet: what it asserts",
    )

    // 2. Skip guards (after RunLog so the run appears in DB even if skipped)
    // rl.Skipf("GOOGLE_AI_API_KEY not set") — if LLM-dependent

    // 3. Test logic in sections
    rl.Section("Setup")
    rl.Printf("setting up test...")

    rl.Section("Execute")
    out := runSomeCommand()
    rl.CLI("myapp do-thing", out)

    rl.Section("Verify")
    if !strings.Contains(out, "expected") {
        rl.Failf("expected 'expected' in output, got: %s", out)
    }
    rl.Printf("verification passed")
}
```

**Rules:**
- `rl.Failf` not `t.Fatalf` — failures must appear in `runlog inspect`
- `t.Cleanup(rl.Close)` not `defer` — more robust (runs on panic)
- `rl.Describe` immediately after `NewRunLog`
- Every section must have at least one logged event
- Every CLI call must be logged via `rl.CLI` / `rl.CLIStep`

---

## 2. TestContext — Multi-Step Tests

### Construction

```go
tc := runlog.NewTest(t, runlog.TestOpts{
    Describe:   "Full lifecycle: create, verify, delete",
    Bullets:    []string{"Creates via CLI", "Verifies in list", "Deletes and confirms"},
    Project:    "e2e-myfeature",    // creates project + registers cleanup
    Tags:       []string{"model:gemini-2.0-flash", "blueprint:v2"},
    AppVersion: "v2.1.0",
    Binary:     "memory",          // CLI binary; defaults to "memory"
    TestType:   "e2e",             // overrides auto-derived type
})
defer tc.Done()
```

`NewTest` automatically:
1. Creates `RunLog` + registers `rl.Close` via `t.Cleanup`
2. Creates isolated temp dir → `tc.Home`
3. Calls `SkipIfServerDown` (skips test if server unreachable)
4. Calls `SetupCLIAuth` in isolated home
5. Sets description from opts
6. Applies tags
7. If `opts.Project != ""`, creates the project and registers cleanup

### TestContext fields (exported)

```go
tc.T          *testing.T
tc.RunLog     *runlog.RunLog
tc.Home       string      // isolated temp HOME for CLI invocations
tc.Server     string      // MEMORY_TEST_SERVER
tc.Token      string      // MEMORY_TEST_TOKEN
tc.ProjectID  string      // set when opts.Project != ""
tc.Binary     string      // CLI binary name
```

### TestContext methods

```go
tc.Step(name string, fn func(s *runlog.Step))  // named step block
tc.Done()                                       // idempotent finalizer (safe for defer)
tc.Log(format string, args ...any)              // delegates to RunLog.Printf
tc.Tag(tags ...string)                          // delegates to RunLog.Tag
tc.Skip(reason string)                          // delegates to RunLog.Skipf
```

### Steps and actions

```go
tc.Step("Create agent", func(s *runlog.Step) {
    var agentID string
    s.CLI("agents", "create", "--name", "test-bot").
        Contains("created").
        ParseID(&agentID)
})

tc.Step("Expect error", func(s *runlog.Step) {
    s.CLIExpectError("tokens", "revoke", "--id", "bad").
        ExitCode(1).
        Contains("not found")
})

tc.Step("HTTP call", func(s *runlog.Step) {
    body, _ := json.Marshal(map[string]string{"key": "val"})
    s.HTTP("POST", "/api/resource", body).
        Status(201).
        JSONField("id", &resourceID)
})

tc.Step("Write file", func(s *runlog.Step) {
    s.WriteFile("config.yaml", "key: value")
    s.Log("wrote config file")
})
```

### Step methods

```go
s.CLI(args ...string) *CLIResult            // fatal on non-zero exit
s.CLIExpectError(args ...string) *CLIResult // never fails; assert exit code manually
s.HTTP(method, path string, body ...[]byte) *HTTPResult
s.Log(format string, args ...any)
s.WriteFile(path, content string)           // writes to tc.Home/path
```

---

## 3. Fixture — Declarative Setup

```go
fx := runlog.Use(t,
    runlog.WithProject("e2e-docs"),          // create project + register cleanup
    runlog.WithSchema("testdata/schema.yaml"), // upload schema (requires WithProject first)
    runlog.WithDocument("testdata/doc.pdf"),   // upload document (requires WithProject first)
    runlog.WithBinary("mycli"),              // override binary (default: "memory")
)

fx.Section("Verify document")
fx.CLI("documents", "list").Contains("doc.pdf")
```

`Use` automatically: creates RunLog, isolated home, checks server readiness, applies options.

### Fixture fields

```go
fx.T         *testing.T
fx.Home      string
fx.Server    string
fx.Token     string
fx.ProjectID string
fx.Binary    string
fx.RunLog    *runlog.RunLog
```

### Fixture methods

```go
fx.CLI(args ...string) *CLIResult            // fatal on error + logged
fx.CLIExpectError(args ...string) *CLIResult
fx.TempFile(name, content string) string     // writes to a fresh t.TempDir(), returns path
fx.Log(format string, args ...any)
fx.Section(name string)
```

---

## 4. CLIResult Assertions (chainable)

All assertions log the check result to RunLog. Failure calls `rl.Failf`.

```go
.Contains(substrs ...string) *CLIResult    // stdout contains ALL substrings
.ContainsAny(substrs ...string) *CLIResult // stdout contains at least one
.NotContains(substrs ...string) *CLIResult // stdout contains none
.Matches(pattern string) *CLIResult        // stdout matches regex
.Empty() *CLIResult                        // stdout is empty/whitespace
.ExitCode(n int) *CLIResult                // exit code equals n
.ParseID(dst *string) *CLIResult           // extract UUID (8-4-4-4-12) from stdout
.JSONField(field string, dst *string) *CLIResult  // parse JSON, extract top-level field
.JSON(dst any) *CLIResult                  // unmarshal full stdout into dst
.Output() string                           // raw stdout
.StderrOutput() string                     // raw stderr (empty in combined mode)
```

---

## 5. HTTPResult Assertions (chainable)

```go
.Status(n int) *HTTPResult                        // status code equals n
.BodyContains(substr string) *HTTPResult          // body contains substring
.JSONField(field string, dst *string) *HTTPResult // parse JSON body, extract field
.JSONContains(field, expected string) *HTTPResult // JSON field equals expected
.JSON(dst any) *HTTPResult                        // unmarshal body into dst
.Body() string                                    // raw body string
.Header(name string) string                       // first response header value
.StatusCode() int                                 // raw status code
```

---

## 6. Event Model

All events stored in `run_events`. The `details` column is a JSON blob — no schema migration needed for new event kinds.

### Built-in event kinds

| Kind | Emitted by | TUI display |
|------|-----------|-------------|
| `section` | `rl.Section()` | Collapsible group header (timeline) |
| `log` | `rl.Printf()`, `rl.LogStep()` | Timeline entry |
| `cli` | `rl.CLI()`, `rl.CLIStep()`, `s.CLI()`, `fx.CLI()` | Timeline (with invocation + output in details) |
| `assertion` | `rl.AssertionStep()` | Timeline (expected/actual comparison layout) |
| `failure` | `rl.Failf()` | Timeline (highlighted red) |
| `skip` | `rl.Skipf()` | Timeline |
| `tag` | `rl.Tag()` | Metadata panel |
| `state_change` | Auto at run start/finish | Metadata panel |
| `token_usage` | `rl.RecordTokenUsage()` | Metadata panel |
| `token_summary` | `rl.PrintTokenSummary()` | Metadata panel (aggregated table) |
| `gantt` | `runlog.PrintGantt()` | Timeline (rendered as Gantt chart) |
| `app_version` | `rl.SetAppVersion()` | Metadata panel |
| `test_version` | Auto in `NewRunLog` | Metadata panel |
| `trace_span` | `StartTracePoller` | Metadata panel |

### Custom events

```go
rl.Event("deployment", "Deployed to staging", map[string]any{
    "environment": "staging",
    "version":     "v2.1.0",
    "duration_ms": 3420,
})
```

### Section children model

`Section()` inserts a group event. Subsequent `Printf`/`CLI`/`Event` calls are buffered as children and flushed to the `children` JSON column when the next `Section()` or `Close()` is called. Children are never separate `run_events` rows — they live in the parent's `children` column.

### CLI event details shape

```json
{
  "invocation": "memory agents list --project abc",
  "output": "agent-1  active\nagent-2  idle",
  "error_msg": "exit status 1",   // only on error
  "exit_code": 1                  // only on error
}
```

---

## 7. SQLite Schema

Database: `.runlog/runs.db` (WAL mode, single writer connection).

### `test_runs` table

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | auto-increment |
| `test_name` | TEXT | Go test function name |
| `started_at` | TEXT | RFC3339Nano |
| `finished_at` | TEXT | RFC3339Nano; NULL if still running |
| `passed` | INTEGER | 0=fail, 1=pass, 2=skip, 3=timeout; NULL if not finished |
| `skipped` | INTEGER | 1 when passed=2 |
| `reason` | TEXT | skip reason or last rl.Failf message; NULL for passes |
| `description` | TEXT | JSON: `{"summary":"...","bullets":["...",...]}` |
| `tags` | TEXT | JSON array: `["model:gemini","blueprint:v2"]` |
| `experiment` | TEXT | experiment name/id; NULL if not set |
| `runner` | TEXT | "host" or "docker"; NULL for older rows |
| `env_name` | TEXT | MEMORY_TEST_ENV value; NULL if not set |
| `env_vars` | TEXT | JSON object: captured env vars |
| `app_version` | TEXT | application-under-test version; NULL if not set |
| `test_version` | TEXT | SHA-256 of test source file; NULL if not set |
| `input_tokens` | INTEGER | total LLM input tokens; NULL if no LLM calls |
| `output_tokens` | INTEGER | total LLM output tokens; NULL if no LLM calls |
| `cost_usd` | REAL | estimated cost; NULL if no LLM calls |
| `timeout_seconds` | REAL | per-test timeout; NULL if not set |
| `category` | TEXT | self-declared category; NULL if not set |
| `coverage_pct` | REAL | code coverage 0.0–100.0; NULL if not measured |
| `coverage_data` | TEXT | per-function coverage JSON; NULL if not measured |
| `test_type` | TEXT | classification: unit/integration/e2e/benchmark/docs/other |
| `daemon_run_id` | TEXT | links to daemon_runs.id; NULL if not daemon-managed |

### `run_events` table

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | auto-increment |
| `run_id` | INTEGER | FK → test_runs.id |
| `seq` | INTEGER | monotonically increasing per run |
| `occurred_at` | TEXT | RFC3339Nano |
| `elapsed_s` | REAL | seconds since run started |
| `kind` | TEXT | event kind (section/log/cli/failure/…) |
| `message` | TEXT | short human-readable message |
| `details` | TEXT | JSON blob; NULL if no details |
| `parent_id` | INTEGER | FK → run_events.id (for child events) |
| `children` | TEXT | JSON array of `{elapsed_s,kind,message,details}` for group events |

### Other tables (summary)

| Table | Purpose |
|-------|---------|
| `schema_migrations` | Migration tracking (version + applied_at) |
| `experiment_suggestions` | LLM-generated suggestions per experiment |
| `analyzer_traces` | Per-analysis run metadata |
| `analyzer_trace_events` | Full LLM conversation events for replay |
| `test_launchers` | Test processes started via TUI launcher |
| `daemon_runs` | Active test runs registered with local daemon |
| `daemon_resources` | Server-side projects tracked by daemon for orphan cleanup |
| `linter_runs` | Linter execution results |

---

## 8. Configuration: `.runlog/config.yaml`

All fields optional. File is searched in priority order:
1. `$RUNLOG_CONFIG` env var (exact path)
2. `.runlog/config.yaml` (project root, walked up from cwd)
3. `.runlog.yaml` in cwd (backward compat)

```yaml
# Human-readable app name shown in the web UI header.
name: "My Project"

# Command template for launching tests from TUI.
# {name} = test function name, {env} = environment profile.
# Default: "go test -v -count=1 -run {name} ./..."
testCommand: "go test -v -run {name} ./tests/..."

# Explicit DB path (default: auto-resolved .runlog/runs.db)
db: .runlog/runs.db

# Daemon port (default: 7430)
daemon_port: 7430

# Working directory for test execution
work_dir: /path/to/project

# Artifacts directory (screenshots, traces served at /artifact/)
artifacts_dir: .runlog/artifacts

# Environment variables set for every test run
env:
  MEMORY_TEST_SERVER: http://localhost:3002
  GOOGLE_AI_API_KEY: "AIza..."

# Go packages to test (default: ["./..."])
test_packages:
  - ./tests/api/...
  - ./tests/integration/...

# Linters shown in the web UI linters panel
linters:
  - name: golangci-lint
    command: golangci-lint run ./...
  - name: go vet
    command: go vet ./...

# Multi-project workspace (each project has its own work_dir)
projects:
  - name: backend
    work_dir: ./backend
  - name: frontend
    work_dir: ./frontend

# Named test environments for runlog test / runlog env
environments:
  - name: localhost
    env:
      MEMORY_TEST_SERVER: http://localhost:3002
    requires:
      MEMORY_TEST_TOKEN:
        check: nonempty
        hint: "Run: memory set-token"
      MEMORY_TEST_SERVER:
        check: reachable
        hint: "Start the server first"

# Categorize tests for the web UI tests list
# Priority: rl.SetCategory() > categories map > directory name
categories:
  api/users:
    - TestUserCreation
    - TestUserDeletion
  api/auth:
    - TestLogin
    - TestTokenRefresh
```

### `EnvCheck` types

| `check` value | Validates |
|---------------|-----------|
| `nonempty` | env var is set and non-empty |
| `reachable` | URL is reachable (HTTP GET, 200–499) |
| `port_open` | TCP port is open |
| `executable` | binary is on PATH |
| `file_exists` | file path exists on disk |

---

## 9. CLI Reference

```
runlog [flags]                      open interactive TUI
runlog runs [flags]                 list recent runs (default: --since 24h)
runlog events [flags] <run-id>      list events for a run
runlog show [flags] <run-id>        full detail dump of a run
runlog tail [flags]                 stream new events as they arrive
runlog tests [flags]                list all tests with last status
runlog tests [flags] <test-name>    list recent runs for a specific test
runlog inspect [flags] <run-id>     full inspector dump with events
runlog analyze [flags] <run-id>     LLM analysis with full trace (requires GOOGLE_AI_API_KEY)
runlog trace [flags] <run-id>       show stored analysis trace
runlog clear [--db <path>]          delete runs.db and all log files
runlog version                      print version and exit
runlog skills install [flags]       install embedded skills into AI agent tool dirs
runlog test <profile> [filter] [-- flags]  run tests with env profile
runlog env test <name>              validate an environment's requirements
runlog env setup <name>             run the environment's setup_script
runlog daemon start / stop / status  manage the background daemon

Global flags:
  --db <path>       path to runs.db (default: auto-resolved)
  --since <dur>     time window, e.g. 5m, 1h, 24h (default: 24h)
  --json            (analyze only) output as JSON
```

### `runlog test` — environment-aware test runner

```bash
# Run with environment profile (loads .env + .env.<profile>)
runlog test localhost TestCLI_Auth
runlog test mcj-emergent TestBlueprints
runlog test production -- -v -count=1

# Without profile (uses .env only)
runlog test TestCLI_Auth
```

What it does:
1. Loads `.env` from test directory
2. Overlays `.env.<profile>` if profile specified → sets `MEMORY_TEST_ENV=<profile>`
3. Sets `RUNLOG_RUN_ID` so test process reuses the daemon's run row
4. Execs `go test` with enriched environment

### TUI keyboard navigation

| Key | Action |
|-----|--------|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` | Drill into run / event |
| `Esc` / `Backspace` | Go back |
| `/` | Search |
| `r` | Refresh |
| `L` | Launch selected test |
| `q` / `Ctrl+C` | Quit |

---

## 10. Environment Variables

| Variable | Description |
|----------|-------------|
| `RUNLOG_DB` | Explicit path to `runs.db` |
| `RUNLOG_CONFIG` | Explicit path to `.runlog/config.yaml` |
| `RUNLOG_RUN_ID` | Existing run ID to reuse (set by `runlog test`) |
| `RUNLOG_APP_TITLE` | App title shown in web UI header |
| `TEST_LOG_DIR` | Directory for per-run log files |
| `GOOGLE_AI_API_KEY` | API key for LLM analyzer (Gemini) |
| `EXPERIMENT` | Auto-assigned experiment name for `NewRunLog` |
| `MEMORY_TEST_SERVER` | Server URL (captured in env_vars) |
| `MEMORY_TEST_TOKEN` | Auth token (captured in env_vars) |
| `MEMORY_AUTH_MODE` | Auth mode (captured in env_vars) |
| `MEMORY_ORG_ID` | Org ID (captured in env_vars) |
| `MEMORY_TEST_ENV` | Environment profile name |
| `BRAVE_SEARCH_API_KEY` | Brave API key (captured in env_vars) |
| `OPENAI_API_KEY` | OpenAI key (captured in env_vars) |
| `ANTHROPIC_API_KEY` | Anthropic key (captured in env_vars) |

---

## 11. Installation

```bash
# As a CLI binary (TUI + runlog test + skills)
go install github.com/emergent-company/runlog/cmd/runlog@latest

# As a Go library in your project
go get github.com/emergent-company/runlog

# Binary download
curl -fsSL https://raw.githubusercontent.com/emergent-company/runlog/main/install.sh | sh
```

---

## 12. Embedded Skills System

Five skills are bundled inside the `runlog` binary and can be installed into AI agent tool directories:

```bash
runlog skills install --all           # install for all detected tools
runlog skills install --dry-run --all # preview
runlog skills install --tools opencode
runlog skills install --all --force   # overwrite existing
```

**Bundled skills:**

| Skill | Purpose |
|-------|---------|
| `runlog-test-designer` | Design and write high-quality e2e tests |
| `runlog-verify-e2e-changes` | Compile + smoke-test after changes |
| `runlog-verify-runs` | Audit run log quality |
| `runlog-clear` | Wipe DB + log files for fresh runs |
| `runlog-install-skills` | Install skills into agent tool dirs |
| `runlog-guide` | This skill — comprehensive LLM reference |

**Tool detection** (checks for config directory marker):

| Tool | Marker | Install path |
|------|--------|-------------|
| opencode | `.opencode/` | `.opencode/skills/` |
| claude | `.claude/` | `.claude/skills/` |
| cursor | `.cursor/` | `.cursor/skills/` |
| agents | `.agents/` | `.agents/skills/` |
| windsurf | `.windsurf/` | `.windsurf/skills/` |

---

## 13. Key Go Types (quick reference)

```go
// RunLog — core structured logger
type RunLog struct { /* opaque */ }

// TestOpts — configures NewTest
type TestOpts struct {
    Describe   string
    Bullets    []string
    Project    string
    Tags       []string
    AppVersion string
    Binary     string   // default: "memory"
    TestType   string
}

// TestContext — multi-step test handle
type TestContext struct {
    T         *testing.T
    RunLog    *RunLog
    Home      string
    Server    string
    Token     string
    ProjectID string
    Binary    string
}

// Fixture — declarative setup handle
type Fixture struct {
    T         *testing.T
    Home      string
    Server    string
    Token     string
    ProjectID string
    Binary    string
    RunLog    *RunLog
}

// RunOutcome — passed column values
const (
    OutcomeFail    RunOutcome = 0
    OutcomePass    RunOutcome = 1
    OutcomeSkip    RunOutcome = 2
    OutcomeTimeout RunOutcome = 3
)

// RunRow — one row from test_runs
type RunRow struct {
    ID, TestName, StartedAt, FinishedAt, Passed, Skipped ...
    EventCount   int
    Description  *RunDescription
    TokenSummary *RunTokenSummary
    Tags         []string
    Experiment   *string
    AppVersion   *string; TestVersion *string
    Runner       *string; Reason *string; EnvName *string
    InputTokens  *int64; OutputTokens *int64; CostUSD *float64
    EnvVars      map[string]string
    Category     *string
    CoveragePct  *float64; CoverageData *string
    TestType     string
}

// ChildEvent — child of a group/section event
type ChildEvent struct {
    ElapsedS float64 `json:"elapsed_s"`
    Kind     string  `json:"kind"`
    Message  string  `json:"message"`
    Details  string  `json:"details,omitempty"` // raw JSON
}

// AgentRunInterval — for Gantt charts
type AgentRunInterval struct {
    AgentName        string
    RunID            string
    Start, End       time.Time
    DurationMs       int
    InputTokens      int64
    OutputTokens     int64
    EstimatedCostUSD float64
}

// Config — .runlog/config.yaml
type Config struct {
    Name         string
    TestCommand  string
    DBPath       string
    DaemonPort   int
    WorkDir      string
    ArtifactsDir string
    Env          map[string]string
    TestPackages []string
    Linters      []LinterDef
    Projects     []ProjectConfig
    Environments []EnvironmentConfig
    Categories   map[string][]string
}
```

---

## 14. Common Workflows

### Write a new test

```go
func TestMyFeature(t *testing.T) {
    rl := runlog.NewRunLog(t)
    t.Cleanup(rl.Close)
    rl.Describe("Describe what this test verifies",
        "Step: preconditions",
        "Step: actions",
        "Step: assertions",
    )

    rl.Section("Setup")
    // ... setup code ...
    rl.Printf("setup complete")

    rl.Section("Execute")
    // ... run the feature under test ...
    rl.CLI("myapp do-thing --arg value", output)

    rl.Section("Verify")
    if !strings.Contains(output, "expected string") {
        rl.Failf("expected string not found in: %s", output)
    }
    rl.Printf("all assertions passed")
}
```

### Write a structured multi-step test

```go
func TestAgentLifecycle(t *testing.T) {
    tc := runlog.NewTest(t, runlog.TestOpts{
        Describe: "Full agent lifecycle: create → list → delete",
        Project:  "e2e-agents",
    })
    defer tc.Done()

    var agentID string

    tc.Step("Create agent", func(s *runlog.Step) {
        s.CLI("agents", "create", "--name", "test-bot", "--model", "gemini").
            Contains("created").
            ParseID(&agentID)
    })

    tc.Step("List agents", func(s *runlog.Step) {
        s.CLI("agents", "list").Contains("test-bot")
    })

    tc.Step("Delete agent", func(s *runlog.Step) {
        s.CLI("agents", "delete", "--id", agentID).Contains("deleted")
    })
}
```

### Inspect recent runs

```bash
runlog runs --since 2h               # list recent runs
runlog inspect <run-id>              # full event timeline for one run
runlog show <run-id>                 # summary dump
runlog tests TestMyFeature           # all runs for a specific test
```

### Audit run quality

Look for:
- Empty sections (section event with 0 children)
- CLI calls not logged (`rl.CLI` / `rl.CLIStep` after every `exec.Command`)
- `t.Fatalf` used instead of `rl.Failf` after RunLog is active
- `rl.Describe` missing (test purpose invisible in TUI)
- Orphaned events (Printf/Event outside any Section)

### Run tests with environment

```bash
runlog test localhost TestMyFeature            # loads .env + .env.localhost
runlog test production -- -v -count=1          # loads .env + .env.production
```

### Clear stale runs

```bash
runlog clear                                   # wipe .runlog/runs.db + log files
runlog clear --db /path/to/custom/runs.db
```

### LLM analysis

```bash
# Requires GOOGLE_AI_API_KEY
runlog analyze <run-id>                        # interactive analysis
runlog analyze --json <run-id>                 # JSON output
runlog trace <run-id>                          # replay stored conversation
```

---

## 15. Anti-Patterns

| Anti-pattern | Fix |
|-------------|-----|
| `t.Fatalf` after RunLog created | `rl.Failf` — failures must appear in `runlog inspect` |
| `defer rl.Close()` | `t.Cleanup(rl.Close)` — runs even on panic |
| Empty section: `rl.Section("X")` with no child events | Add at least one `rl.Printf` inside the section |
| HTTP call without logging | `rl.CLIStep("description", "POST /api/...", body)` after every HTTP call |
| Missing `rl.Describe` | Add immediately after `NewRunLog`, before any other code |
| `t.Skip(...)` instead of `rl.Skipf(...)` | `rl.Skipf` records the reason in the DB |
| Logs outside any section | Wrap in `rl.Section("section name")` |

---

## 16. Web UI (built into `runlog` binary)

The `runlog` binary serves a web UI at port 4099 (configurable). Access via `http://<hostname>:4099`.

Pages:
- **Dashboard** — failing tests, recent activity, system status
- **Tests** — all known tests with pass/fail/skip counts, last run, categories
- **Test detail** — run history for one test with sparklines
- **Runs** — all recent runs with filter/pagination
- **Run detail** — full event timeline with expandable sections
- **Experiments** — grouped runs by experiment name, LLM suggestions
- **Linters** — run linters and view output in real-time

The server is managed by the daemon (`runlog daemon start/stop/status`). Do not start it manually.
