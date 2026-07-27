# runlog

Structured test observability for Go projects. Structured logging, SQLite-backed run history, web UI, Gantt charts, and LLM-powered analysis.

📖 **[Writing Tests Guide](GUIDE.md)** — full API reference, patterns, and examples.

## Features

- **Structured test logging** — `RunLog` provides sections, groups, key-value pairs, and Gantt chart timing for Go tests
- **SQLite run database** — Every test run is stored with events, durations, and outcomes for historical analysis
- **CLI + Web UI** — Browse runs, drill into events, and search tests via CLI or web browser
- **Test launcher** — Start tests from the web UI or CLI
- **LLM analyzer** — AI-powered analysis of test failures with full conversation traces
- **Step-based API** — `TestContext` with `Step()`, `CLIResult`, and `HTTPResult` for structured test workflows
- **Zero CGO** — Pure Go SQLite driver, cross-compiles to all platforms

## Installation

### Go install (recommended)

```bash
go install github.com/emergent-company/runlog/cmd/runlog@latest
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/emergent-company/runlog/releases).

### Install script

```bash
curl -fsSL https://raw.githubusercontent.com/emergent-company/runlog/main/install.sh | sh
```

### As a library

```bash
go get github.com/emergent-company/runlog
```

## Quick Start

### In your tests

```go
import "github.com/emergent-company/runlog"

func TestUserCreation(t *testing.T) {
    rl := runlog.NewRunLog(t)
    rl.Describe("Create a new user and verify the response")

    rl.Section("Setup")
    rl.Printf("Creating test user...")

    rl.Section("API Call")
    rl.Printf("POST /api/users → 201 Created")

    rl.Section("Verification")
    rl.Printf("User ID: %s", userID)
    rl.Printf("Email verified: true")
}
```

## Version Tracking

### Test Version (auto)

Every test file gets a unique version identifier — the **SHA-256 hash** of the test file — recorded automatically at test start. This catches local edits that aren't committed to git. The git commit hash of the last change to the file is also included in the event details when available.

```go
// Auto-detected in NewRunLog:
//   test_version: <SHA256> + event with {"sha256":..., "git_commit":...}
```

Override the auto-detected version:
```go
rl.SetTestVersion("my-custom-label")
```

### App Version (manual)

Record which version of the application-under-test was used. Test writer decides when and what to record.

```go
// Directly on RunLog:
rl := runlog.NewRunLog(t)
rl.SetAppVersion("v2.1.0")

// Or via TestContext:
tc := runlog.NewTest(t, runlog.TestOpts{
    AppVersion: "v2.1.0",
})
```

Both values appear in the run inspector panel and are persisted to the SQLite database for filtering and historical queries.

### Browse results

```bash
runlog runs              # list recent runs
runlog runs             # list recent runs
runlog tests            # list all tests with last status
runlog show 42          # full detail dump
runlog analyze 42       # LLM analysis of a failure
```

