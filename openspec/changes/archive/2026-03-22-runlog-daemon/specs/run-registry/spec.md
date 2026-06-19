## ADDED Requirements

### Requirement: Run registration
The daemon SHALL accept run registration via `POST /runs` with fields: `pid` (int), `env_profile` (string), `server_url` (string), `token` (string). It SHALL return a unique run ID (UUID). The run SHALL be stored in `daemon_runs` with `status=active` and the current timestamp.

#### Scenario: Register new run
- **WHEN** `POST /runs` is called with valid pid, env_profile, server_url, token
- **THEN** response is 201 with `{"id": "<uuid>"}` and run appears in GET /runs with status=active

### Requirement: Run completion marking
The daemon SHALL accept `PUT /runs/:id/done` to mark a run as cleanly finished. The run's `finished_at` SHALL be set and `status` set to `done`. The daemon SHALL trigger an immediate resource sweep for that run after marking it done.

#### Scenario: Mark run done
- **WHEN** `PUT /runs/:id/done` is called for an active run
- **THEN** run status becomes done, finished_at is set, and its resources are swept

### Requirement: PID-based run reaping
The daemon SHALL run a reaper goroutine that polls every 10 seconds. For each run with `status=active`, the reaper SHALL check if the registered PID is still alive using signal 0. If the PID is dead and no `PUT /runs/:id/done` was received, the run SHALL be marked `dead` and its resources swept.

#### Scenario: Reap crashed run
- **WHEN** a run is registered with a PID that subsequently dies without calling PUT /done
- **THEN** within 10 seconds the reaper marks the run dead and triggers resource sweep

#### Scenario: Active run not reaped
- **WHEN** a run is registered and its PID is still alive
- **THEN** reaper does not mark it dead

### Requirement: Run listing
`GET /runs` SHALL return all runs with `status=active` plus runs finished or died in the last 24 hours. Each entry SHALL include: id, pid, env_profile, status, started_at, finished_at, resource_count.

#### Scenario: List returns active runs
- **WHEN** GET /runs is called while a run is active
- **THEN** the active run appears in the response with status=active

### Requirement: Run persistence across daemon restarts
Run state SHALL be persisted to SQLite (`daemon_runs` table) on every mutation. On daemon startup, all runs with `status=active` SHALL be re-evaluated: if their PID is no longer alive, they SHALL be immediately marked dead and swept.

#### Scenario: Recover dead runs after restart
- **WHEN** daemon restarts and finds an active run whose PID is dead
- **THEN** that run is marked dead and its resources are swept during startup
