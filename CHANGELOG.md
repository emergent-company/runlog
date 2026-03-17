# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-17

Initial release. Extracted from `github.com/emergent-company/emergent.memory.e2e/framework`.

### Added

- **RunLog** structured test logger with sections, groups, key-value pairs, and Gantt chart timing
- **RunDB** SQLite-backed run database for persisting test run history and events
- **Analyzer** LLM-powered test failure analysis with conversation traces and suggestions
- **TestContext** step-based API with `Step()`, `CLIResult`, `HTTPResult` for structured test workflows
- **TUI binary** (`cmd/runlog`) with interactive run browser, test launcher, and AI analysis
- Dynamic test discovery from database (replaces static test registry)
- Configurable test launcher via `.runlog.yaml` `testCommand` field
- `.runlog.yaml` configuration file for categories, test commands, and DB path
- Simplified DB path resolution: `$RUNLOG_DB` / `.runlog.yaml` / `./runs.db` / `./logs/runs.db`
- CLI subcommands: `runs`, `events`, `show`, `tail`, `tests`, `experiments`, `inspect`, `analyze`, `trace`, `clear`, `version`
- Cross-platform binaries via goreleaser (linux/darwin amd64+arm64, windows/amd64)
- `install.sh` script for quick binary installation
- CI workflow (vet, build, test on push/PR)
- Release workflow (goreleaser on tag push)

### Changed

- Package name from `e2eframework` to `runlog`
- Module path from `github.com/emergent-company/emergent.memory.e2e/framework` to `github.com/emergent-company/runlog`
- DB path resolution simplified — removed Docker-specific and source-file-relative paths
- Test launcher uses configurable command template instead of searching for `./test` script
