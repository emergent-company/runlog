## ADDED Requirements

### Requirement: Resource registration
The daemon SHALL accept `POST /runs/:id/resources` with fields: `resource_id` (project ID on the server), `resource_type` (default "project"). It SHALL store the resource in `daemon_resources` with `status=active` linked to the given run.

#### Scenario: Register project resource
- **WHEN** `POST /runs/:id/resources` is called with `{"resource_id": "proj-uuid", "resource_type": "project"}`
- **THEN** resource is stored with status=active and appears in GET /resources/orphaned only after its run dies

#### Scenario: Register to unknown run
- **WHEN** `POST /runs/:id/resources` is called with an unknown run ID
- **THEN** response is 404

### Requirement: Resource deregistration
The daemon SHALL accept `DELETE /runs/:id/resources/:resource_id` to mark a resource as deleted (the test's `t.Cleanup` successfully deleted it from the server). The resource SHALL be marked `status=deleted` with `deleted_at` timestamp. A deleted resource SHALL NOT be swept by the orphan sweeper.

#### Scenario: Deregister cleaned resource
- **WHEN** `DELETE /runs/:id/resources/:resource_id` is called after t.Cleanup deletes the project
- **THEN** resource status becomes deleted and it is excluded from future sweeps

### Requirement: Orphaned resource listing
`GET /resources/orphaned` SHALL return all resources with `status=active` whose owning run has `status=done` or `status=dead`. Each entry SHALL include: resource_id, resource_type, run_id, run_status, server_url, created_at.

#### Scenario: Orphan after run dies
- **WHEN** a run dies without cleaning its resource and GET /resources/orphaned is called
- **THEN** the resource appears in the response

#### Scenario: Active run resource not orphaned
- **WHEN** a resource belongs to an active run
- **THEN** GET /resources/orphaned does NOT include it

### Requirement: Manual cleanup trigger
`POST /cleanup` SHALL synchronously execute the orphan sweep: find all orphaned resources, attempt to delete each from the server, and return a summary of deleted and failed resources. This SHALL be safe to call at any time — it will never delete resources belonging to active runs.

#### Scenario: Cleanup deletes orphans
- **WHEN** `POST /cleanup` is called with orphaned resources present
- **THEN** each orphaned resource is deleted from the server and marked deleted in the DB; response includes count of deleted and failed

#### Scenario: Cleanup skips active resources
- **WHEN** `POST /cleanup` is called while a run is active
- **THEN** that run's resources are not touched
