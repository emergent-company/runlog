## Context

`runlog` is a CLI tool that writes test run data to a local SQLite database (`runs.db`). The `runlog test` subcommand loads env files and then does `syscall.Exec` — replacing itself with `go test`. After exec there is no runlog process alive; the daemon pattern requires a separate persistent process.

The codebase already has:
- `processAlive(pid int) bool` and `sigterm(pid int)` in `launch_unix.go` / `launch_windows.go`
- `RunlogDir()` that walks up to find `.runlog/` — the natural home for the PID file and daemon socket
- `Config` struct in `config.go` — straightforward to extend with `DaemonPort`
- `db.go` / `runlog.go` — SQLite layer that can be extended with new tables

The daemon must be a **separate long-lived process** started by `runlog daemon` and communicated with over HTTP on localhost. All other `runlog` subcommands (including `runlog test`) talk to it as a client.

## Goals / Non-Goals

**Goals:**
- Daemonize via `runlog daemon` (start, stop, status)
- Track active test runs by PID — detect clean finish and unclean death
- Registry: map run ID → owned project IDs on the remote server
- Orphan sweeper: background goroutine, 60s tick, deletes server resources whose owning run's PID is dead
- Machine-scoped project naming: short machine ID prefix on all `UniqueProjectName` output
- Framework integration: `CreateProject` registers, `DeleteProjectOnCleanup` deregisters — fail-open when daemon not running
- `runlog test` injects `RUNLOG_RUN_ID` + `RUNLOG_DAEMON_URL` into env before exec
- `runlog cleanup` — manual safe-cleanup command

**Non-Goals:**
- Multi-machine coordination (daemon is local only)
- Centralized server-side daemon
- Windows daemonization (PID-based detection works; full service integration out of scope)
- Resource types other than projects (agents, tokens) — future work

## Decisions

### 1. Daemonization: double-fork vs. re-exec flag

**Chosen: re-exec with `--daemon` flag + `Setsid`**

`runlog daemon` spawns itself as `runlog daemon --daemon` with `Setsid: true`, then exits. The child writes a PID file to `.runlog/daemon.pid` and starts the HTTP server. This is the standard Go daemon pattern — no double-fork needed, cross-platform compatible.

Alternative: `syscall.ForkExec` double-fork. More complex, no real benefit over re-exec.

### 2. HTTP API vs. Unix socket

**Chosen: HTTP on localhost with configurable port (default 7430)**

- Consistent with the `RUNLOG_DAEMON_URL` env var that gets injected into test processes
- Works across all platforms
- Port configurable via `daemon_port` in `.runlog/config.yaml`
- `RUNLOG_DAEMON_URL` defaults to `http://localhost:7430`

Alternative: Unix domain socket (`.runlog/daemon.sock`). Slightly faster, no port conflicts, but awkward to pass as a URL to child processes and doesn't work on Windows.

### 3. PID tracking: daemon watches go test PID

After `runlog test` registers a run (`POST /runs`), it does `syscall.Exec`. The daemon receives the PID of the **runlog process** before exec. After exec, the OS reuses that PID for `go test` — so the PID the daemon was given **becomes** the `go test` process. The daemon polls `processAlive(pid)` to detect when the test run ends.

This means `runlog test` must:
1. `POST /runs {pid: os.Getpid(), env: profile}` — register before exec
2. Get back a `run_id`
3. Set `RUNLOG_RUN_ID=<run_id>` and `RUNLOG_DAEMON_URL=<url>` in env
4. `syscall.Exec(go test ...)` — PID is now go test's PID, daemon continues watching it

When go test exits cleanly, the test framework calls `PUT /runs/:id/done` in `TestMain` teardown. If it crashes, the reaper detects PID death and marks it done.

### 4. Orphan sweeper: tick + PID check

The sweeper runs every 60 seconds inside the daemon:
1. Query all resources with `status=active`
2. For each resource, look up its owning run's PID
3. If PID is dead AND run is not marked done → mark run as dead, sweep its resources
4. For each resource to sweep: call `DELETE /api/projects/:id` on the configured server
5. Mark resource deleted

Server URL and token come from the same env vars the test suite uses (`MEMORY_TEST_SERVER`, `MEMORY_TEST_TOKEN`), read from the run's registered environment.

### 5. Machine ID: hostname hash

`UniqueProjectName(prefix)` gains a machine ID segment:

```
e2e-<mid>-<prefix>-<timestamp>
```

Where `<mid>` is the first 8 characters of the hex-encoded SHA256 of `os.Hostname()`. Deterministic, short, collision-resistant across dev machines and CI runners.

### 6. Fail-open framework integration

`CreateProject` attempts `POST /daemon/resources` but catches all errors and proceeds. If the daemon is not running, project creation works exactly as before — no test breaks. The daemon is an enhancement, not a hard dependency.

### 7. DB schema extension

Two new tables in `runs.db` (auto-migrated on daemon start):

```sql
CREATE TABLE daemon_runs (
  id          TEXT PRIMARY KEY,   -- UUID
  pid         INTEGER NOT NULL,
  env_profile TEXT,
  status      TEXT NOT NULL DEFAULT 'active',  -- active | done | dead
  started_at  DATETIME NOT NULL DEFAULT (datetime('now')),
  finished_at DATETIME
);

CREATE TABLE daemon_resources (
  id          TEXT PRIMARY KEY,   -- UUID
  run_id      TEXT NOT NULL REFERENCES daemon_runs(id),
  resource_id TEXT NOT NULL,      -- project ID on the server
  server_url  TEXT NOT NULL,
  token       TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'active',  -- active | deleted
  created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
  deleted_at  DATETIME
);
```

These are separate from the existing `runs` table (which is append-only test event log) to avoid any schema coupling.

## Risks / Trade-offs

- **PID reuse race** → After `syscall.Exec`, the OS may theoretically reuse the PID before the daemon polls. In practice this is impossible in the same exec chain — the child process is the same PID. Risk: near zero.

- **Daemon not running in CI** → CI uses Docker, not the host daemon. The framework's fail-open means tests still work. CI cleanup is handled by the ephemeral environment. Mitigation: document that the daemon is for local dev; CI is not a target.

- **Multiple concurrent local runs** → Two terminals both running `runlog test` simultaneously. Both register separate run IDs. Both have distinct project namespaces (same machine ID prefix, different timestamps). The sweeper only cleans runs whose PID is dead — active runs are never touched. Safe.

- **Daemon crash loses registry** → If the daemon itself crashes, in-flight run data in memory is lost. Mitigation: run state is written to SQLite on every mutation (not just in memory), so a restart recovers it.

- **Token stored in SQLite** → The server token is stored in `daemon_resources` to enable autonomous cleanup. `runs.db` is a local file in the project directory. Mitigation: no worse than storing it in `.env` files, which is already the practice. Document this.

## Migration Plan

1. Daemon is opt-in — existing `runlog test` usage continues to work unchanged if daemon is not running
2. Machine ID prefix on project names is a format change but does not break anything — projects are ephemeral and uniquely named per run
3. New DB tables are created automatically on first `runlog daemon` start
4. No config changes required to use — defaults work out of the box

## Open Questions

- Should `runlog daemon status` show which tests are currently running (with their filters/profiles)? Useful for visibility but adds UI surface.
- Should the sweeper delete agents attached to a project before deleting the project, or rely on server-side cascade? (Current server does cascade — assume it continues to.)
- Should `runlog cleanup` also handle resources from runs that finished cleanly but whose cleanup hook failed? (Yes — that's the main use case. The sweeper covers this too on the next tick.)
