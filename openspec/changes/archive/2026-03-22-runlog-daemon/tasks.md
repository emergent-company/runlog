## 1. Config & Paths

- [x] 1.1 Add `DaemonPort int` field to `Config` struct in `config.go`
- [x] 1.2 Add `daemon_port:` parsing to `parseConfigFile` in `config.go`
- [x] 1.3 Add `DaemonPidFile() string` helper to `paths.go` returning `<RunlogDir()>/daemon.pid`

## 2. Machine-Scoped Project Naming

- [x] 2.1 Add `machineID() string` function in `project.go` (SHA-256 of hostname, first 8 hex chars, fallback "00000000")
- [x] 2.2 Update `UniqueProjectName(prefix)` in `project.go` to produce `<prefix>-<mid8>-<timestamp_ms>`

## 3. DB Schema Extension

- [x] 3.1 Add `daemon_runs` and `daemon_resources` table definitions to `db.go` (CREATE TABLE IF NOT EXISTS)
- [x] 3.2 Call the new table migrations in the existing DB init / migrate path so they are created automatically

## 4. Daemon HTTP Server

- [x] 4.1 Create `cmd/runlog/daemon.go` with the `DaemonServer` struct (HTTP mux, DB handle, sweeper channel, port)
- [x] 4.2 Implement `GET /health` → `{"status":"ok"}`
- [x] 4.3 Implement `POST /runs` — insert into `daemon_runs`, return `{"id":"<uuid>"}`; accept `pid`, `env_profile`, `server_url`, `token`
- [x] 4.4 Implement `GET /runs` — return active runs + runs finished/dead in last 24h with resource_count
- [x] 4.5 Implement `GET /runs/:id` — return single run details + resource list
- [x] 4.6 Implement `PUT /runs/:id/done` — set status=done, finished_at, trigger immediate sweep
- [x] 4.7 Implement `POST /runs/:id/resources` — insert into `daemon_resources` linked to run; 404 if run unknown
- [x] 4.8 Implement `DELETE /runs/:id/resources/:resource_id` — set status=deleted, deleted_at
- [x] 4.9 Implement `GET /resources/orphaned` — resources with status=active whose run is done/dead
- [x] 4.10 Implement `POST /cleanup` — synchronous orphan sweep; return `{"deleted":N,"failed":N}`

## 5. Orphan Sweeper & Run Reaper

- [x] 5.1 Implement `sweepOrphans(db)` function — query orphaned resources, DELETE each from server, mark deleted; 404 treated as success; 5xx skipped (retry next tick)
- [x] 5.2 Implement periodic sweeper goroutine (60s tick) in `DaemonServer`
- [x] 5.3 Implement immediate non-blocking sweep trigger (channel send to sweeper) called after run transitions to done/dead
- [x] 5.4 Implement run reaper goroutine (10s tick) — poll active runs, check `processAlive(pid)`, mark dead + trigger sweep when PID gone

## 6. Daemon Lifecycle (start / stop / status)

- [x] 6.1 Add `runlog daemon` subcommand to `cmd/runlog/main.go` with sub-subcommands: `(default=start)`, `stop`, `status`
- [x] 6.2 Implement daemon start: re-exec self with `--daemon` flag + `Setsid: true`; wait up to 2s for `/health` to respond; print port; exit 0
- [x] 6.3 Implement `--daemon` internal mode: write PID file, open DB, run auto-migration, start HTTP server + sweeper + reaper goroutines; handle SIGTERM for clean shutdown
- [x] 6.4 Implement `runlog daemon stop`: read PID file, SIGTERM, wait up to 5s, remove PID file
- [x] 6.5 Implement `runlog daemon status`: check PID file + `/health`; print status, pid, port, uptime, active_runs, tracked_resources
- [x] 6.6 On daemon startup, recover stale active runs: mark dead and sweep any run whose PID is no longer alive

## 7. Test Runner Integration

- [x] 7.1 In `cmd/runlog/test.go`, before `syscall.Exec`: attempt `POST /runs` to daemon (if reachable); on success set `RUNLOG_RUN_ID` and `RUNLOG_DAEMON_URL` in the exec env; fail-open on any error
- [x] 7.2 Add `daemonURL() string` helper that returns `RUNLOG_DAEMON_URL` env var if set, else `http://localhost:<DaemonPort>`

## 8. Framework Integration (project.go)

- [x] 8.1 In `CreateProject`, after parsing the project ID: if `RUNLOG_RUN_ID` and `RUNLOG_DAEMON_URL` are set, call `POST <daemon_url>/runs/<run_id>/resources` with `resource_id` and `resource_type=project`; best-effort with 500ms timeout
- [x] 8.2 In `DeleteProjectOnCleanup`, after successful server deletion: if `RUNLOG_RUN_ID` and `RUNLOG_DAEMON_URL` are set, call `DELETE <daemon_url>/runs/<run_id>/resources/<project_id>`; best-effort, failure does not fail cleanup

## 9. runlog cleanup Command

- [x] 9.1 Add `runlog cleanup` subcommand to `cmd/runlog/main.go` that calls `POST /cleanup` on the daemon and prints the result; if daemon not running, print "daemon not running"
