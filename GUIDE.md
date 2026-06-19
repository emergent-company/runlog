# Writing Tests with runlog — Good Practices Guide

## Choosing an API

runlog offers three APIs for test authors. Pick the one that fits your test style.

| API | Entry point | Best for |
|-----|-------------|----------|
| **Raw RunLog** | `runlog.NewRunLog(t)` | Simple logging, no server/CLI dependency |
| **TestContext** | `runlog.NewTest(t, opts)` | Structured multi-step tests with CLI + HTTP |
| **Fixture** | `runlog.Use(t, opts...)` | Declarative setup, minimal boilerplate |

## Raw RunLog — Lightweight structured logging

Use when you just need structured logging with no server, CLI, or project setup.

```go
func TestParseConfig(t *testing.T) {
    rl := runlog.NewRunLog(t)
    defer rl.Close()

    rl.Describe("Validate configuration parsing",
        "Loads a YAML config file",
        "Verifies all fields are read correctly",
    )

    rl.Section("Load config")
    cfg, err := loadConfig("testdata/sample.yaml")
    if err != nil {
        rl.Failf("load config: %v", err)
    }
    rl.Printf("Config loaded: %+v", cfg)

    rl.Section("Verify fields")
    rl.Printf("Server: %s", cfg.Server)
    rl.Printf("Timeout: %s", cfg.Timeout)
}
```

**Key patterns:**
- `rl.Describe(summary, bullets...)` — top-level test description shown in TUI
- `rl.Section(name)` — creates a collapsible group in TUI
- `rl.Printf(format, args...)` — log a timestamped message
- `rl.Failf(format, args...)` — log a failure and fail the test
- `rl.Event(kind, message, details)` — custom structured event (any JSON-serializable details)

## TestContext — Structured multi-step tests with CLI + HTTP

Use when your test runs a CLI binary and/or makes HTTP requests against a server. This is the primary API for end-to-end tests.

### Basic structure

```go
func TestAgentLifecycle(t *testing.T) {
    tc := runlog.NewTest(t, runlog.TestOpts{
        Describe: "Full agent lifecycle: create, verify, delete",
        Bullets: []string{
            "Creates an agent via CLI",
            "Lists agents and verifies the new agent appears",
            "Deletes the agent and confirms removal",
        },
        Project: "e2e-agents",
        Tags:    []string{"model:gemini-2.0-flash", "blueprint:v2"},
    })
    defer tc.Done()

    var agentID string

    tc.Step("Create agent", func(s *runlog.Step) {
        s.CLI("agents", "create", "--name", "test-bot", "--model", "gemini").
            Contains("created").
            ParseID(&agentID)
    })

    tc.Step("List agents", func(s *runlog.Step) {
        s.CLI("agents", "list").
            Contains("test-bot")
    })

    tc.Step("Delete agent", func(s *runlog.Step) {
        s.CLI("agents", "delete", "--id", agentID).
            Contains("deleted")
    })
}
```

### Step actions

Each `tc.Step` gives you `*Step` with these action methods:

| Method | Behavior on non-zero/error |
|--------|---------------------------|
| `s.CLI(args...)` | Fails test via `rl.Failf` |
| `s.CLIExpectError(args...)` | Returns result without failing (assert exit code manually) |
| `s.HTTP(method, path, body...)` | Fails test on connection error |
| `s.Log(format, args...)` | Writes a timestamped log line |
| `s.WriteFile(path, content)` | Writes file into `tc.Home` directory |

### Chainable assertions

Every action returns a result object with chainable assertions:

**`*CLIResult`:**
```go
s.CLI("agents", "list").
    Contains("bot-1", "bot-2").     // stdout contains ALL substrings
    ContainsAny("active", "idle").  // stdout contains AT LEAST ONE
    NotContains("error").           // stdout does NOT contain any
    Matches(`bot-\d+`).             // stdout matches regex
    ExitCode(0).                    // assert exit code
    ParseID(&id).                   // extract UUID from stdout
    JSONField("name", &name).       // parse JSON output, extract field
    JSON(&myStruct)                 // unmarshal full stdout into struct
```

**`*HTTPResult`:**
```go
s.HTTP("GET", "/api/projects/"+projectID+"/agents").
    Status(200).
    BodyContains("test-bot").
    JSONField("id", &agentID).
    JSONContains("status", "active").
    JSON(&responseStruct)
```

### Expected-failure patterns

```go
tc.Step("Revoke with bad token", func(s *runlog.Step) {
    s.CLIExpectError("tokens", "revoke", "--id", "nonexistent").
        ExitCode(1).
        Contains("not found")
})
```

### App version tracking

```go
tc := runlog.NewTest(t, runlog.TestOpts{
    AppVersion: "v2.1.0", // recorded in test_runs.app_version + app_version event
    // ...
})
```

## Fixture — Declarative, minimal-boilerplate tests

Use when you want the shortest possible test body with automatic server checks and project lifecycle.

```go
func TestDocumentUpload(t *testing.T) {
    fx := runlog.Use(t,
        runlog.WithProject("e2e-docs"),
        runlog.WithDocument("testdata/contract.pdf"),
    )

    fx.Section("Verify document")
    fx.CLI("documents", "list").
        Contains("contract.pdf")

    fx.Section("Search content")
    fx.CLI("documents", "search", "confidential").
        Contains("contract.pdf")
}
```

**`Fixture` methods:**
- `fx.CLI(args...)` — run CLI, fail on error
- `fx.CLIExpectError(args...)` — run CLI, return result regardless
- `fx.TempFile(name, content)` — write a temp file, return path
- `fx.Log(format, args...)` — log a message
- `fx.Section(name)` — start a named section

**`Fixture` options:**
- `runlog.WithProject(prefix)` — create ephemeral project, register cleanup
- `runlog.WithSchema(path)` — upload schema file (requires WithProject first)
- `runlog.WithDocument(path)` — upload document file (requires WithProject first)
- `runlog.WithBinary(name)` — override CLI binary (default: "memory")

## Test organization patterns

### 1. One scenario per test function

Each `tc.Step` maps to one phase of the scenario. Steps are executed sequentially.

```go
func TestFullWorkflow(t *testing.T) {
    tc := runlog.NewTest(t, runlog.TestOpts{...})
    defer tc.Done()

    tc.Step("Bootstrap project", func(s *runlog.Step) { ... })
    tc.Step("Upload schema", func(s *runlog.Step) { ... })
    tc.Step("Ingest documents", func(s *runlog.Step) { ... })
    tc.Step("Query with agent", func(s *runlog.Step) { ... })
    tc.Step("Verify results", func(s *runlog.Step) { ... })
}
```

### 2. Shared setup via `TestOpts`

Prefer `TestOpts.Describe` + `Bullets` over inline comments. They render in the TUI.

```go
tc := runlog.NewTest(t, runlog.TestOpts{
    Describe: "Regenerate summary for a document",
    Bullets: []string{
        "Document exists with an existing summary",
        "Regenerate triggers a new agent run",
        "Summary is replaced with fresh content",
    },
    Project: "e2e-summary",
    Tags:    []string{"feature:summary"},
})
```

### 3. Early skip on unavailable server

`NewTest` and `Use` both call `RequireServerReady` internally. If the server is down or auth fails, the test is skipped (not failed) with a diagnostic message. No need to add your own server checks.

### 4. Isolated state per test

`NewTest` and `Use` create an isolated temp `HOME` directory for each test. CLI invocations inside `tc.Step` or `fx.CLI` use this directory automatically — no `--home` or `--server` flags needed.

## Events — custom structured data

### Built-in event kinds

| Kind | When emitted | TUI visibility |
|------|-------------|----------------|
| `section` | `rl.Section()` | Timeline (collapsible) |
| `log` | `rl.Printf()` | Timeline |
| `cli` | `rl.CLI()`, `s.CLI()`, `fx.CLI()` | Timeline |
| `failure` | `rl.Failf()` | Timeline (highlighted) |
| `skip` | `rl.Skipf()` | Timeline |
| `tag` | `rl.Tag()` | Metadata (debug mode) |
| `state_change` | Start/finish of run | Metadata (debug mode) |
| `token_usage` | `rl.RecordTokenUsage()` | Metadata (debug mode) |
| `token_summary` | `rl.PrintTokenSummary()` | Metadata (debug mode) |
| `metric` | `rl.Event("metric", ...)` | Metadata (debug mode) |
| `gantt` | `rl.PrintGantt()` | Timeline (rendered as Gantt chart) |
| `app_version` | `rl.SetAppVersion()` | Metadata (debug mode) |
| `test_version` | Auto in `NewRunLog` | Metadata (debug mode) |

### Custom events

```go
rl.Event("deployment", "Deployed to staging", map[string]any{
    "environment": "staging",
    "version":     "v2.1.0",
    "duration_ms": 3420,
})
```

Custom events appear in the timeline with their kind as the badge. They accept any JSON-serializable `details` value.

### Section groups with children

```go
rl.Section("API Results")
for _, item := range results {
    rl.Printf("  %s → %s", item.ID, item.Status)
}
// The section is collapsible in TUI; all Printf calls become children.
```

## Cost tracking for LLM operations

When your test triggers LLM calls (agent runs, memory ask calls), record token usage:

```go
// After each LLM operation:
inputTok, outputTok, costUSD := fetchTokenUsage(...)
rl.RecordTokenUsage(inputTok, outputTok, costUSD)

// At the end, print a token summary table:
rl.PrintTokenSummary(intervals)
```

Totals are auto-persisted to the database on `rl.Close()` and displayed in the TUI run inspector.

## Gantt charts for multi-agent tests

```go
intervals := buildAgentRunIntervals(t, rl, srv, token, projectID, agents)
rl.PrintGantt(intervals)
```

The Gantt chart shows each agent's execution timeline as a horizontal bar with token usage annotated. Emitted as a `gantt` event that the TUI renders interactively.

## Version tracking

### Test version (auto)

`NewRunLog` computes the SHA-256 hash of the test source file and records it as `test_version`. Also captures the git commit hash of the last change to the file (if git is available).

Override when needed:
```go
rl.SetTestVersion("my-custom-label")
```

### App version (manual)

```go
// Via RunLog:
rl.SetAppVersion("v2.1.0")

// Via TestContext:
tc := runlog.NewTest(t, runlog.TestOpts{
    AppVersion: "v2.1.0",
})
```

Both versions appear in the TUI run inspector panel and are persisted to the database.

## Environment variables

### What gets captured

`NewRunLog` automatically captures these env vars and persists them to the database:

- `GOOGLE_AI_API_KEY`
- `MEMORY_TEST_SERVER`
- `MEMORY_TEST_TOKEN`
- `MEMORY_AUTH_MODE`
- `MEMORY_ORG_ID`
- `BRAVE_SEARCH_API_KEY`
- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`

### Test environment profiles

The `runlog test` command loads `.env` (base) + `.env.<profile>` (overlay) from the working directory:

```bash
runlog test localhost TestAgentLifecycle
# Loads .env + .env.localhost, sets MEMORY_TEST_ENV=localhost
```

## Complete reference: all exported symbols

### RunLog methods

| Method | Description |
|--------|-------------|
| `NewRunLog(t)` | Create new run log (opens file, wires DB, auto-detects test_version) |
| `Close()` | Flush, close log file, persist outcome to DB |
| `Describe(summary, bullets...)` | Set test description |
| `Printf(format, args...)` | Log a timestamped message |
| `Section(name)` | Start a collapsible section |
| `Event(kind, msg, details)` | Emit a custom event |
| `Group(kind, title, fn)` | Emit a group event with child lines |
| `CLI(invocation, output)` | Log a CLI invocation and output |
| `CLIErr(invocation, output, err)` | Like CLI, with error for exit code |
| `CLIStep(desc, invocation, output)` | Like CLI with custom description |
| `CLIStepErr(desc, invocation, output, err)` | Like CLIStep with error |
| `Failf(format, args...)` | Log failure + call `t.Fatal` |
| `Skipf(format, args...)` | Log skip + call `t.Skip` |
| `Tag(tags...)` | Add variant tags (e.g. "model:gemini") |
| `SetExperiment(name)` | Assign experiment name |
| `SetAppVersion(version)` | Record app version |
| `SetTestVersion(version)` | Override auto-detected test version |
| `RecordTokenUsage(in, out, cost)` | Accumulate LLM token usage |
| `PrintTokenSummary(intervals)` | Print token summary table |
| `PrintGantt(intervals)` | Print Gantt chart |
| `StartTracePoller(srv, token, projectID)` | Start trace span poller |
| `Dir()` | Get log directory path |
| `MustRunCLI(t, args...)` | Run CLI + log + fail on error |

### TestContext methods

| Method | Description |
|--------|-------------|
| `NewTest(t, opts)` | Create initialized test context |
| `Done()` | Idempotent finalizer (safe for defer) |
| `Step(name, fn)` | Run a named step block |
| `Log(format, args...)` | Log a message |
| `Tag(tags...)` | Add variant tags |
| `Skip(reason)` | Skip the test with reason |

### Step methods

| Method | Description |
|--------|-------------|
| `s.CLI(args...)` | Run CLI, fail on error → `*CLIResult` |
| `s.CLIExpectError(args...)` | Run CLI, no fail → `*CLIResult` |
| `s.HTTP(method, path, body...)` | Make HTTP request → `*HTTPResult` |
| `s.Log(format, args...)` | Log a message |
| `s.WriteFile(path, content)` | Write file to test home |

### Fixture methods

| Method | Description |
|--------|-------------|
| `Use(t, opts...)` | Create initialized fixture |
| `fx.CLI(args...)` | Run CLI, fail on error → `*CLIResult` |
| `fx.CLIExpectError(args...)` | Run CLI, no fail → `*CLIResult` |
| `fx.TempFile(name, content)` | Create temp file, return path |
| `fx.Log(format, args...)` | Log a message |
| `fx.Section(name)` | Start a named section |

### CLIResult assertions

| Method | Description |
|--------|-------------|
| `.Contains(substrs...)` | stdout contains ALL substrings |
| `.ContainsAny(substrs...)` | stdout contains at least one |
| `.NotContains(substrs...)` | stdout contains none |
| `.Matches(regex)` | stdout matches regex |
| `.Empty()` | stdout is empty/whitespace |
| `.ExitCode(n)` | exit code equals n |
| `.ParseID(dst)` | extract UUID from stdout |
| `.JSONField(field, dst)` | parse JSON, extract field |
| `.JSON(dst)` | unmarshal stdout into struct |
| `.Output()` | raw stdout |
| `.StderrOutput()` | raw stderr |

### HTTPResult assertions

| Method | Description |
|--------|-------------|
| `.Status(n)` | status code equals n |
| `.BodyContains(substr)` | body contains substring |
| `.JSONField(field, dst)` | parse JSON body, extract field |
| `.JSONContains(field, value)` | JSON field equals value |
| `.JSON(dst)` | unmarshal body into struct |
| `.Body()` | raw body string |
| `.Header(name)` | response header value |
| `.StatusCode()` | raw status code |

### CLIResult assertions
