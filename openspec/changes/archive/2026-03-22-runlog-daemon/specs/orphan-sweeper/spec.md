## ADDED Requirements

### Requirement: Periodic orphan sweep
The daemon SHALL run an orphan sweeper goroutine that ticks every 60 seconds. On each tick it SHALL: (1) find all resources with `status=active` whose owning run has `status=done` or `status=dead`, (2) for each such resource call `DELETE /api/projects/:resource_id` on the resource's `server_url` with its `token`, (3) mark the resource `status=deleted` with `deleted_at` on success, (4) log and skip (retry next tick) on failure.

#### Scenario: Sweep deletes orphaned project
- **WHEN** a run is marked dead and has an active resource
- **THEN** within 60 seconds the sweeper calls DELETE on the server for that project and marks it deleted

#### Scenario: Sweep skips active run resources
- **WHEN** the sweeper ticks and a resource belongs to an active run
- **THEN** the sweeper does NOT call DELETE for that resource

#### Scenario: Sweep retries on server error
- **WHEN** DELETE /api/projects/:id returns a 5xx error
- **THEN** the resource remains status=active and is retried on the next tick

#### Scenario: Sweep handles 404 gracefully
- **WHEN** DELETE /api/projects/:id returns 404 (already deleted)
- **THEN** the resource is marked deleted (idempotent — not an error)

### Requirement: Immediate sweep on run completion
When a run transitions to `done` or `dead` (either via `PUT /runs/:id/done` or the reaper), the daemon SHALL trigger an immediate non-blocking sweep for that run's resources rather than waiting for the next 60-second tick.

#### Scenario: Immediate sweep after clean finish
- **WHEN** `PUT /runs/:id/done` is called and the run has active resources (cleanup hooks failed)
- **THEN** sweep is triggered immediately, not waiting for the next tick

### Requirement: Sweep does not block the API
The orphan sweeper SHALL run in a separate goroutine. Sweep operations (HTTP DELETE calls to the server) SHALL NOT block the daemon's HTTP API from serving requests.

#### Scenario: API responsive during sweep
- **WHEN** the sweeper is executing DELETE calls to the server
- **THEN** GET /runs responds within 100ms
