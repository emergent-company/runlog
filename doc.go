// Package runlog provides terminal-native test observability for Go projects.
//
// It includes structured test logging (RunLog), a SQLite-backed run database,
// CLI/HTTP test helpers, a step-based test context API, and an optional
// LLM-powered run analyzer.
//
// The companion TUI binary (cmd/runlog) provides interactive run history,
// Gantt charts, test launching, and AI-powered analysis. The TypeScript SDK
// (sdk/typescript, @runlog/client) provides equivalent Jest reporter and
// Playwright fixture integrations for JS/TS test suites, all writing to the
// same event stream via the daemon's HTTP API.
//
// Quick start:
//
//	func TestExample(t *testing.T) {
//	    rl := runlog.NewRunLog(t)
//	    defer rl.Close()
//	    rl.Describe("Demonstrates basic runlog usage")
//	    rl.Section("Setup")
//	    rl.Printf("Setting up test...")
//	    rl.Section("Execution")
//	    rl.Printf("Running test logic...")
//	}
//
// # Event contract: kind, message, details
//
// Every event recorded by RunLog (and by any producer writing directly to
// the daemon's POST /runs/:id/events endpoint, including the TypeScript SDK)
// follows the same three-field shape:
//
//   - kind: a short snake_case string identifying the event type, e.g.
//     "cli", "http_call", "failure". The canonical list of kinds the web UI
//     renders specially — with per-kind badge colors and expand-panel
//     layouts — lives in cmd/runlog/events_reference.templ. Emitting an
//     unrecognized kind never breaks anything (the daemon logs a warning
//     and falls back to a generic JSON detail view), but reusing an
//     existing kind name for something it doesn't mean (e.g. tagging a
//     network request "cli") produces confusing, mislabeled UI rows —
//     prefer a new, accurately-named kind instead.
//
//   - message: a short, single-line, human-readable summary shown directly
//     in the events table (e.g. "GET /api/health → 200", "$ go build ./...",
//     "Error: connection refused"). Keep this under ~150 characters and free
//     of control characters (ANSI escape codes, newlines) — the table
//     truncates and does not interpret ANSI in this field. If the underlying
//     message is long or colorized (e.g. a captured CLI/test-framework
//     error), reduce it to a clean one-liner for message and preserve the
//     full original text in details instead.
//
//   - details: an optional JSON-serializable map with the rich, structured
//     payload for the expanded view — e.g. {method, url, status_code,
//     response_body} for http_call, {command, exit_code, output} for cli,
//     {matcher, locator, expected, actual} for an assertion. This is where
//     full multi-line/ANSI-colored text belongs; the UI renders known kinds'
//     details with dedicated layouts (see cmd/runlog/run_events.templ) and
//     falls back to a generic pretty-printed JSON view for custom kinds.
//
// This short-message/rich-details split is what keeps the events table
// scannable while still preserving full diagnostic detail on click. See
// RunLog.CLI, RunLog.CLIErr, and RunLog.HTTPCall for reference
// implementations of this contract; RunLog.Event is the generic escape
// hatch for custom kinds that should still follow it.
package runlog
