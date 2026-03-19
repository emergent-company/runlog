# Migration Guide

Migrating from `github.com/emergent-company/emergent.memory.e2e/framework` to `github.com/emergent-company/runlog`.

## Overview

The `runlog` package is a standalone extraction of the `e2eframework` package. All types, functions, and the TUI binary are identical — only the import path and package name have changed.

## Import Path Changes

```go
// Before
import framework "github.com/emergent-company/emergent.memory.e2e/framework"

// After
import "github.com/emergent-company/runlog"
```

## Package Name

The package name changed from `e2eframework` (typically imported as `framework`) to `runlog`:

```go
// Before
rl := framework.NewRunLog(t)
db := framework.SharedDB()
framework.MustRunCLI(t, rl, "memory", "version")

// After
rl := runlog.NewRunLog(t)
db := runlog.SharedDB()
runlog.MustRunCLI(t, rl, "memory", "version")
```

## TUI Binary

The TUI binary is now installed separately:

```bash
# Before: built from cmd/runlog/ in the e2e repo
go run ./cmd/runlog

# After: install the standalone binary
go install github.com/emergent-company/runlog/cmd/runlog@latest
runlog
```

## Configuration

Create a `.runlog/config.yaml` in your project root to configure the test launcher and categories:

```yaml
testCommand: "./test mcj-emergent {name}"

categories:
  cli/install:
    - TestCLIInstalled_Version
    - TestCLIInstalled_Help
    # ... add your test categories
```

This replaces the hardcoded `knownTests` slice that was previously in the TUI binary.

## Database Path

The DB path resolution has been simplified. If your setup relied on Docker-specific paths or `runtime.Caller`-based resolution, set one of:

```bash
export RUNLOG_DB=/path/to/runs.db      # new env var
export TEST_LOG_DIR=/path/to/logs      # still supported for backward compat
```

Or configure in `.runlog/config.yaml`:

```yaml
db: /path/to/runs.db
```

## Step-by-Step Migration

1. Add the new module: `go get github.com/emergent-company/runlog`
2. Replace import paths: `framework "github.com/emergent-company/emergent.memory.e2e/framework"` → `"github.com/emergent-company/runlog"`
3. Replace `framework.` references with `runlog.` in your code
4. Create `.runlog/config.yaml` if you want test categories in the TUI
5. Install the standalone TUI: `go install github.com/emergent-company/runlog/cmd/runlog@latest`
6. Remove the old `cmd/runlog/` directory from your repo
7. Run `go mod tidy` to clean up
