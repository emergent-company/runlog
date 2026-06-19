## Why

When `runlog test` runs, it replaces itself with `go test` via `syscall.Exec` — there is no persistent process tracking which test runs are active or what server resources they own. If a run is killed, crashes, or a test's `t.Cleanup` fails to fire, projects created on the remote test server are silently orphaned. Over time these accumulate, cause naming collisions, and — when agents are attached — can trigger runaway queue explosions. We need a persistent local daemon that tracks active runs, owns their server resources, and cleans up automatically when runs finish or die.

## What Changes

- **New**: `runlog daemon` subcommand — starts a background daemon process (`runlogd`) that persists across test runs
- **New**: `runlog daemon stop` / `runlog daemon status` — lifecycle commands for the daemon
- **New**: Daemon HTTP API (localhost, configurable port) — runs, resources, and cleanup endpoints
- **New**: `RUNLOG_RUN_ID` environment variable — injected by `runlog test` before exec'ing `go test`, threads run identity into the test process
- **New**: `RUNLOG_DAEMON_URL` environment variable — injected alongside run ID so the framework knows where to reach the daemon
- **Modified**: `runlog test` — registers the run with the daemon before exec, passes run ID and daemon URL via env vars; marks run done if daemon was pre-exec (not possible post-Exec — daemon detects PID death instead)
- **Modified**: `framework.CreateProject` — registers created project as a resource against the active run (best-effort, fail-open when daemon not running)
- **Modified**: `framework.DeleteProjectOnCleanup` — deregisters the resource from the daemon on successful cleanup
- **New**: Machine-scoped project naming — `UniqueProjectName` prefixes names with a short machine ID so each machine owns its own namespace on the server
- **New**: Daemon orphan sweeper — background goroutine that runs every 60 seconds, finds resources whose owning run's process is no longer alive, deletes them from the server
- **New**: Daemon run reaper — detects runs that are registered but whose PID has died without a clean finish signal; marks them done and triggers resource sweep
- **New**: `runlog cleanup` — manual trigger that asks the daemon for orphaned resources and deletes them (safe: checks active runs first)
- **New**: Daemon config — port (default 7430) configurable in runlog config file

## Capabilities

### New Capabilities

- `daemon`: The runlogd process — lifecycle (start, stop, status), PID file management, daemonization, HTTP API surface
- `run-registry`: Tracking active and completed runs — run records with PID, status, start time, environment; detecting dead PIDs
- `resource-registry`: Mapping runs to server resources (project IDs) — registration, deregistration, orphan detection
- `orphan-sweeper`: Background cleanup goroutine — periodic scan, PID liveness check, server-side deletion of orphaned resources
- `machine-scoped-naming`: Short machine ID prefix on all e2e project names — ensures each machine owns a non-overlapping namespace

### Modified Capabilities

- `test-runner`: `runlog test` gains pre-exec daemon registration and run ID injection into the child process environment

## Impact

- `cmd/runlog/main.go` — new `daemon` subcommand and `cleanup` subcommand
- `cmd/runlog/test.go` — inject `RUNLOG_RUN_ID` + `RUNLOG_DAEMON_URL` before exec
- New `cmd/runlog/daemon.go` — daemon start/stop/status implementation
- New `daemon/` package (or internal) — HTTP server, run registry, resource registry, sweeper goroutines
- `project.go` — `CreateProject` and `DeleteProjectOnCleanup` gain best-effort daemon registration
- `project.go` — `UniqueProjectName` gains machine ID prefix
- `config.go` — daemon port added to runlog config
- No new external Go dependencies — standard library `net/http` for daemon API, `os.FindProcess` for PID liveness
- `runs.db` schema extended — `run_pid`, `daemon_run_id` columns; new `resources` table
