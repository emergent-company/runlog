## ADDED Requirements

### Requirement: Machine ID prefix on project names
`UniqueProjectName(prefix string)` SHALL prepend a short machine ID to the generated name. The machine ID SHALL be the first 8 hex characters of the SHA-256 hash of the system hostname. The resulting format SHALL be: `<prefix>-<mid8>-<timestamp_ms>`.

#### Scenario: Name includes machine ID
- **WHEN** `UniqueProjectName("e2e-ask")` is called on a machine with hostname "mydevbox"
- **THEN** the returned name matches the pattern `e2e-ask-<8hexchars>-<13digits>`

#### Scenario: Same machine produces consistent prefix
- **WHEN** `UniqueProjectName` is called twice on the same machine
- **THEN** both names share the same 8-character machine ID segment

#### Scenario: Different machines produce different prefixes
- **WHEN** `UniqueProjectName` is called on two machines with different hostnames
- **THEN** the machine ID segments differ (with overwhelming probability)

### Requirement: Machine ID is stable and short
The machine ID SHALL be derived deterministically from the hostname (no random component). It SHALL be exactly 8 lowercase hex characters. If `os.Hostname()` returns an error, the machine ID SHALL fall back to `"00000000"`.

#### Scenario: Fallback on hostname error
- **WHEN** hostname cannot be retrieved
- **THEN** machine ID is "00000000" and name generation still succeeds
