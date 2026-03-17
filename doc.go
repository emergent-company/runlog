// Package runlog provides terminal-native test observability for Go projects.
//
// It includes structured test logging (RunLog), a SQLite-backed run database,
// CLI/HTTP test helpers, a step-based test context API, and an optional
// LLM-powered run analyzer.
//
// The companion TUI binary (cmd/runlog) provides interactive run history,
// Gantt charts, test launching, and AI-powered analysis.
//
// Quick start:
//
//	func TestExample(t *testing.T) {
//	    rl := runlog.NewRunLog(t, "example")
//	    rl.Describe("Demonstrates basic runlog usage")
//	    rl.Section("Setup")
//	    rl.Printf("Setting up test...")
//	    rl.Section("Execution")
//	    rl.Printf("Running test logic...")
//	}
package runlog
