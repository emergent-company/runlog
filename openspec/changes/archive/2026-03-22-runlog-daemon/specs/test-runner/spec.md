## ADDED Requirements

### Requirement: Run ID injected before exec
`runlog test` SHALL, before calling `syscall.Exec`, attempt to register the current run with the local daemon (if running). On success it SHALL set `RUNLOG_RUN_ID=<uuid>` and `RUNLOG_DAEMON_URL=<url>` in the process environment so the exec'd `go test` process inherits them. If the daemon is not running, `runlog test` SHALL proceed with exec unchanged (fail-open).

#### Scenario: Daemon running — run ID injected
- **WHEN** `runlog test mcj-emergent` is run and daemon is listening on default port
- **THEN** `RUNLOG_RUN_ID` and `RUNLOG_DAEMON_URL` are set in the environment of the exec'd `go test` process

#### Scenario: Daemon not running — exec proceeds normally
- **WHEN** `runlog test mcj-emergent` is run and daemon is not running
- **THEN** `go test` is exec'd without `RUNLOG_RUN_ID` or `RUNLOG_DAEMON_URL`, tests run normally

### Requirement: Framework registers resources best-effort
`framework.CreateProject` SHALL, when `RUNLOG_RUN_ID` and `RUNLOG_DAEMON_URL` are set in the environment, call `POST <daemon_url>/runs/<run_id>/resources` with the created project's ID. This call SHALL be made asynchronously or with a short timeout (≤500ms) and SHALL NOT fail or slow down the test if the daemon is unreachable.

#### Scenario: Resource registered on create
- **WHEN** `CreateProject` is called and RUNLOG_RUN_ID + RUNLOG_DAEMON_URL are set
- **THEN** the project is registered with the daemon within 500ms

#### Scenario: Daemon unreachable — project still created
- **WHEN** `CreateProject` is called and RUNLOG_DAEMON_URL points to a non-listening address
- **THEN** project is created on the server normally; registration error is logged but does not fail the test

### Requirement: Framework deregisters resources on cleanup
`framework.DeleteProjectOnCleanup` SHALL, after successfully deleting the project from the server, call `DELETE <daemon_url>/runs/<run_id>/resources/<project_id>` to deregister it. This call SHALL be best-effort — failure does not fail the test.

#### Scenario: Resource deregistered after successful cleanup
- **WHEN** `t.Cleanup` runs, project is deleted from server, and daemon is reachable
- **THEN** resource is deregistered from daemon (status=deleted in DB)

#### Scenario: Cleanup proceeds if deregister fails
- **WHEN** project is deleted but daemon deregister call fails
- **THEN** test cleanup is considered successful; daemon will sweep the resource on next tick
