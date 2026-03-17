# runlog

Terminal-native test observability for Go projects. Structured logging, SQLite-backed run history, interactive TUI, Gantt charts, and LLM-powered analysis.

## Features

- **Structured test logging** — `RunLog` provides sections, groups, key-value pairs, and Gantt chart timing for Go tests
- **SQLite run database** — Every test run is stored with events, durations, and outcomes for historical analysis
- **Interactive TUI** — Browse runs, drill into events, search tests, and launch tests from the terminal
- **Test launcher** — Start tests directly from the TUI with configurable commands
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

### Browse results in the TUI

```bash
runlog                  # interactive TUI
runlog runs             # list recent runs
runlog tests            # list all tests with last status
runlog show 42          # full detail dump
runlog analyze 42       # LLM analysis of a failure
```

### TUI keyboard navigation

| Key | Action |
|---|---|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `Enter` | Drill into run / event |
| `Esc` / `Backspace` | Go back |
| `/` | Search |
| `r` | Refresh |
| `L` | Launch selected test |
| `q` / `Ctrl+C` | Quit |

## Configuration

Create a `.runlog.yaml` in your project root or next to `runs.db`:

```yaml
# Command template for launching tests from the TUI.
# Placeholders: {name} = test function name, {env} = test environment
testCommand: "go test -v -run {name} ./..."

# Explicit database path (optional).
# Default search: $RUNLOG_DB → ./runs.db → ./logs/runs.db
db: logs/runs.db

# Group tests by category in the TUI.
categories:
  api/users:
    - TestUserCreation
    - TestUserDeletion
    - TestUserUpdate
  api/auth:
    - TestLogin
    - TestTokenRefresh
```

## CLI Reference

```
runlog [flags]                        open interactive TUI
runlog runs [flags]                   list recent runs
runlog events [flags] <run-id>        list events for a run
runlog show [flags] <run-id>          full detail dump of a run
runlog tail [flags]                   stream new events as they arrive
runlog tests [flags]                  list all known tests with last status
runlog tests [flags] <test-name>      list recent runs for a specific test
runlog inspect [flags] <run-id>       full inspector dump of a run
runlog analyze [flags] <run-id>       LLM analysis with full trace
runlog trace [flags] <run-id>         show stored analysis trace
runlog clear [--db <path>]            delete runs.db and log files
runlog version                        print version and exit

Flags:
  --db <path>      path to runs.db (default: auto-resolved)
  --since <dur>    time window, e.g. 5m, 1h, 24h (default: 24h)
  --json           (analyze only) output as JSON
```

## Environment Variables

| Variable | Description |
|---|---|
| `RUNLOG_DB` | Explicit path to `runs.db` |
| `RUNLOG_CONFIG` | Explicit path to `.runlog.yaml` |
| `TEST_LOG_DIR` | Directory for run log files |
| `GOOGLE_AI_API_KEY` | API key for LLM analyzer (Gemini) |

## License

MIT
