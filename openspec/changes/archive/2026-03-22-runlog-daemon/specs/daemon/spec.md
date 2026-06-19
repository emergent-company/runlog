## ADDED Requirements

### Requirement: Daemon starts as background process
`runlog daemon` SHALL spawn a background process (`runlogd`) that survives the invoking terminal session. The daemon SHALL write its PID to `.runlog/daemon.pid` in the project directory and start an HTTP server on the configured port (default 7430). The invoking process SHALL exit with status 0 once the daemon is confirmed listening.

#### Scenario: Start daemon when not running
- **WHEN** user runs `runlog daemon` and no daemon is running
- **THEN** a background process is spawned, PID file is written, HTTP server starts, and the command exits 0 with a message indicating the port

#### Scenario: Start daemon when already running
- **WHEN** user runs `runlog daemon` and a daemon is already running (PID file exists and process is alive)
- **THEN** command exits 0 with a message indicating the daemon is already running and its PID

### Requirement: Daemon stops cleanly
`runlog daemon stop` SHALL send SIGTERM to the daemon process, wait for it to exit (up to 5 seconds), and remove the PID file. All in-flight sweeper operations SHALL complete before shutdown.

#### Scenario: Stop running daemon
- **WHEN** user runs `runlog daemon stop` and daemon is running
- **THEN** daemon receives SIGTERM, exits cleanly, PID file is removed, command exits 0

#### Scenario: Stop when daemon not running
- **WHEN** user runs `runlog daemon stop` and no daemon is running
- **THEN** command exits 0 with message "daemon not running"

### Requirement: Daemon reports status
`runlog daemon status` SHALL report whether the daemon is running, its PID, port, uptime, count of active runs, and count of tracked resources.

#### Scenario: Status when running
- **WHEN** user runs `runlog daemon status` and daemon is running
- **THEN** output includes: status=running, pid, port, uptime, active_runs count, tracked_resources count

#### Scenario: Status when not running
- **WHEN** user runs `runlog daemon status` and daemon is not running
- **THEN** output includes: status=stopped

### Requirement: Daemon port is configurable
The daemon HTTP port SHALL be configurable via `daemon_port` in `.runlog/config.yaml`. The default SHALL be 7430. `RUNLOG_DAEMON_URL` environment variable SHALL override the URL entirely (useful for CI or non-standard setups).

#### Scenario: Custom port from config
- **WHEN** `.runlog/config.yaml` contains `daemon_port: 8888`
- **THEN** daemon listens on port 8888 and `runlog daemon status` reports port 8888

### Requirement: Daemon HTTP API
The daemon SHALL expose a JSON HTTP API on localhost only. All endpoints SHALL return JSON. The API SHALL include:
- `GET  /health` — liveness check
- `POST /runs` — register a new run
- `GET  /runs` — list active and recent runs
- `GET  /runs/:id` — get run details
- `PUT  /runs/:id/done` — mark run finished
- `POST /runs/:id/resources` — register a resource to a run
- `DELETE /runs/:id/resources/:resource_id` — deregister a resource
- `GET  /resources/orphaned` — list resources with no active run
- `POST /cleanup` — trigger immediate orphan sweep

#### Scenario: Health check
- **WHEN** GET /health is called
- **THEN** response is 200 with `{"status":"ok"}`

#### Scenario: Register run
- **WHEN** POST /runs is called with `{"pid": 1234, "env_profile": "mcj-emergent", "server_url": "http://...", "token": "..."}`
- **THEN** response is 201 with `{"id": "<uuid>"}` and run is stored with status=active
