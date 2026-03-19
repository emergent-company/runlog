// cmd/runlog — TUI browser and CLI query tool for the e2e test run log database.
//
// # Subcommands
//
//	runlog                         interactive TUI (default when no subcommand given)
//	runlog runs                    list recent runs as a table
//	runlog events <run-id>         list all events for a run
//	runlog show <run-id>           full dump: run metadata + every event with details
//	runlog tail                    stream new events as they arrive (like tail -f)
//	runlog analyze <run-id>        LLM analysis of a run with full conversation trace
//	runlog trace <run-id>          show stored analysis trace (no LLM call)
//	runlog test [<profile>] [<filter>]  load .env and exec go test (profile = MEMORY_TEST_ENV)
//
// # Global flags (apply to all subcommands)
//
//	--db path/to/runs.db           explicit DB path (default: .runlog/runs.db)
//	--since 5m                     time window for "runs" and TUI (default: 5m)
//
// # TUI keyboard navigation
//
//	↑ / k        move cursor up
//	↓ / j        move cursor down
//	Enter        drill in (run list → event list → event detail)
//	Esc / Backspace  go back one level
//	r            refresh current view from DB
//	q / Ctrl+C   quit
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	runlog "github.com/emergent-company/runlog"
)

// Build-time variables set by goreleaser via ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// ─────────────────────────────────────────────────────────────────────────────
// Styles
// ─────────────────────────────────────────────────────────────────────────────

var (
	styleTitleBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			PaddingLeft(1).PaddingRight(1)

	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("237"))

	styleNormal = lipgloss.NewStyle()

	stylePass = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	styleFail = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleSkip = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Bold(true)
	styleRun  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)

	styleKind = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	styleDetailKey = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	styleDetailVal = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	styleHelp   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	styleHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	styleTabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).PaddingLeft(1).PaddingRight(1)
	styleTabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Background(lipgloss.Color("234")).PaddingLeft(1).PaddingRight(1)

	styleSection     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	styleStateChange = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))
	styleMetric      = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	styleCLI         = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	styleCLIFail     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // red — cli with non-zero exit code
	styleAbort       = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true) // orange — zombie/incomplete

	// plain-text styles for non-interactive output
	plainPass  = "PASS"
	plainFail  = "FAIL"
	plainSkip  = "SKIP"
	plainRuns  = "RUNS"
	plainAbort = "DEAD" // finished_at set but passed never recorded
)

// ─────────────────────────────────────────────────────────────────────────────
// Non-interactive commands
// ─────────────────────────────────────────────────────────────────────────────

// cmdRuns prints a table of recent runs to stdout.
func cmdRuns(db *runlog.RunDB, since time.Duration) error {
	rows, err := db.ListRuns(time.Now().Add(-since))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Printf("no runs found in the last %s\n", since)
		return nil
	}

	// Check if any runs have cost data to decide whether to show cost column
	hasCost := false
	for _, r := range rows {
		if r.CostUSD != nil && *r.CostUSD > 0 {
			hasCost = true
			break
		}
	}

	if hasCost {
		fmt.Printf("%-6s  %-8s  %-6s  %-12s  %-8s  %-8s  %-7s  %-12s  %s\n",
			"ID", "STATUS", "RUNNER", "ENV", "AGE", "DURATION", "EVENTS", "COST", "TEST NAME")
		fmt.Println(strings.Repeat("─", 120))
	} else {
		fmt.Printf("%-6s  %-8s  %-6s  %-12s  %-8s  %-8s  %-7s  %s\n",
			"ID", "STATUS", "RUNNER", "ENV", "AGE", "DURATION", "EVENTS", "TEST NAME")
		fmt.Println(strings.Repeat("─", 102))
	}

	for _, r := range rows {
		status := passLabel(r)
		age := formatAgePlain(r.StartedAt)
		dur := formatDurationPlain(r.StartedAt, r.FinishedAt)
		runner := "—"
		if r.Runner != nil {
			runner = *r.Runner
		}
		env := "—"
		if r.EnvName != nil {
			env = *r.EnvName
		}

		if hasCost {
			costStr := "—"
			if r.CostUSD != nil && *r.CostUSD > 0 {
				costStr = fmt.Sprintf("$%.6f", *r.CostUSD)
			}
			fmt.Printf("%-6d  %-8s  %-6s  %-12s  %-8s  %-8s  %-7d  %-12s  %s\n",
				r.ID, status, runner, env, age, dur, r.EventCount, costStr, r.TestName)
		} else {
			fmt.Printf("%-6d  %-8s  %-6s  %-12s  %-8s  %-8s  %-7d  %s\n",
				r.ID, status, runner, env, age, dur, r.EventCount, r.TestName)
		}
	}
	return nil
}

// cmdEvents prints all events for a run as a table.
func cmdEvents(db *runlog.RunDB, runID int64) error {
	evs, err := db.ListEvents(runID)
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		fmt.Printf("no events found for run %d\n", runID)
		return nil
	}

	fmt.Printf("%-5s  %-8s  %-14s  %-10s  %s\n",
		"SEQ", "ELAPSED", "KIND", "OCCURRED", "MESSAGE")
	fmt.Println(strings.Repeat("─", 80))
	for _, e := range evs {
		occurred := e.OccurredAt.Format("15:04:05")
		fmt.Printf("%-5d  %7.2fs  %-14s  %-10s  %s\n",
			e.Seq, e.ElapsedS, e.Kind, occurred, e.Message)
	}
	return nil
}

// cmdShow prints full detail of a run: metadata header then every event
// including pretty-printed details JSON.
func cmdShow(db *runlog.RunDB, runID int64) error {
	// Fetch the run row via ListRuns with a wide enough window.
	rows, err := db.ListRuns(time.Time{})
	if err != nil {
		return err
	}
	var run *runlog.RunRow
	for i := range rows {
		if rows[i].ID == runID {
			run = &rows[i]
			break
		}
	}
	if run == nil {
		return fmt.Errorf("run %d not found", runID)
	}

	passed := "—"
	if run.Skipped {
		passed = plainSkip
	} else if run.Passed != nil {
		if *run.Passed {
			passed = plainPass
		} else {
			passed = plainFail
		}
	}
	fmt.Printf("run:     %d\n", run.ID)
	fmt.Printf("test:    %s\n", run.TestName)
	fmt.Printf("status:  %s\n", passed)
	if run.Reason != nil {
		fmt.Printf("reason:  %s\n", *run.Reason)
	}
	fmt.Printf("started: %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
	if run.FinishedAt != nil {
		fmt.Printf("finished:%s\n", run.FinishedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("duration:%s\n", formatDurationPlain(run.StartedAt, run.FinishedAt))
	}
	fmt.Printf("events:  %d\n", run.EventCount)
	if run.Runner != nil {
		fmt.Printf("runner:  %s\n", *run.Runner)
	}
	if run.EnvName != nil {
		fmt.Printf("env:     %s\n", *run.EnvName)
	}
	if run.InputTokens != nil || run.OutputTokens != nil || run.CostUSD != nil {
		if run.InputTokens != nil {
			fmt.Printf("input tokens:  %s\n", runlog.FormatInt(*run.InputTokens))
		}
		if run.OutputTokens != nil {
			fmt.Printf("output tokens: %s\n", runlog.FormatInt(*run.OutputTokens))
		}
		if run.CostUSD != nil {
			fmt.Printf("cost:    $%.6f\n", *run.CostUSD)
		}
	}
	if len(run.EnvVars) > 0 {
		fmt.Println("\nEnvironment Variables:")
		// Sort keys for consistent display.
		keys := make([]string, 0, len(run.EnvVars))
		for k := range run.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := run.EnvVars[k]
			// Mask API keys for security (show first 8 chars and "...").
			if strings.Contains(strings.ToLower(k), "key") || strings.Contains(strings.ToLower(k), "token") {
				if len(v) > 12 {
					v = v[:8] + "..." + v[len(v)-4:]
				}
			}
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
	fmt.Println(strings.Repeat("─", 80))

	evs, err := db.ListEvents(runID)
	if err != nil {
		return err
	}
	for _, e := range evs {
		occurred := e.OccurredAt.Format("15:04:05")
		fmt.Printf("[%5.2fs] %-14s  %-10s  %s\n", e.ElapsedS, e.Kind, occurred, e.Message)
		if e.Details != nil && *e.Details != "" && *e.Details != "{}" {
			lines := prettyJSON(*e.Details)
			for _, l := range lines {
				fmt.Printf("         %s\n", l)
			}
		}
		for _, c := range e.Children {
			fmt.Printf("  [%5.2fs] %-12s  %s\n", c.ElapsedS, c.Kind, c.Message)
			if c.Details != "" && c.Details != "{}" {
				lines := prettyJSON(c.Details)
				for _, l := range lines {
					fmt.Printf("           %s\n", l)
				}
			}
		}
	}
	return nil
}

// cmdTail streams new events to stdout as they arrive, similar to tail -f.
func cmdTail(db *runlog.RunDB, since time.Duration) error {
	lastSeen := time.Now().Add(-since)
	fmt.Printf("tailing events since %s (Ctrl+C to stop)…\n", lastSeen.Format("15:04:05"))
	for {
		rows, err := db.ListRuns(lastSeen)
		if err != nil {
			return err
		}
		for _, r := range rows {
			evs, err := db.ListEvents(r.ID)
			if err != nil {
				continue
			}
			for _, e := range evs {
				if e.OccurredAt.After(lastSeen) {
					fmt.Printf("[%s] run=%-5d %-14s  %s\n",
						e.OccurredAt.Format("15:04:05"), r.ID, e.Kind, e.Message)
					if e.OccurredAt.After(lastSeen) {
						lastSeen = e.OccurredAt
					}
				}
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// cmdExperiments prints a plain-text table of all experiments, using the same
// data as the Experiments tab in the TUI.
func cmdExperiments(db *runlog.RunDB) error {
	exps, err := db.ListExperiments()
	if err != nil {
		return err
	}
	if len(exps) == 0 {
		fmt.Println("no experiments found (run tests with EXPERIMENT=name)")
		return nil
	}
	fmt.Printf("%-40s  %5s  %6s  %8s  %6s\n", "experiment", "runs", "pass%", "cost", "last")
	fmt.Println(strings.Repeat("─", 75))
	for _, exp := range exps {
		age := formatAgePlain(exp.LastRunAt)
		passRate := "   —  "
		if exp.RunCount > 0 {
			pct := int(float64(exp.PassCount) / float64(exp.RunCount) * 100)
			passRate = fmt.Sprintf("%5d%%", pct)
		}
		fmt.Printf("%-40s  %5d  %s  %8s  %6s\n",
			truncate(exp.Name, 40), exp.RunCount, passRate,
			formatCostShort(exp.TotalCostUSD), age)
	}
	return nil
}

// cmdTestsList prints a plain-text table of all known tests with their last
// run status and age, discovered from the database and optional config.
func cmdTestsList(db *runlog.RunDB, since time.Duration) error {
	rows, err := db.ListRuns(time.Now().Add(-since))
	if err != nil {
		return err
	}

	// Discover tests from DB + config.
	dbDir := filepath.Dir(db.Path())
	cfg, _ := runlog.LoadConfig(dbDir)
	names, err := db.DiscoverTests()
	if err != nil {
		return err
	}

	// Build entries: categorized first, then uncategorized.
	seen := make(map[string]bool)
	var entries []testEntry
	if cfg != nil && len(cfg.Categories) > 0 {
		var cats []string
		for cat := range cfg.Categories {
			cats = append(cats, cat)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			for _, name := range cfg.Categories[cat] {
				entries = append(entries, testEntry{Name: name, Category: cat})
				seen[name] = true
			}
		}
	}
	for _, name := range names {
		if !seen[name] {
			entries = append(entries, testEntry{Name: name, Category: "Uncategorized"})
		}
	}

	fmt.Printf("%-20s  %-*s  %6s  %4s\n", "category", 55, "test name", "last", "st")
	fmt.Println(strings.Repeat("─", 92))
	prevCat := ""
	for _, te := range entries {
		catLabel := ""
		if te.Category != prevCat {
			catLabel = te.Category
			prevCat = te.Category
		}
		lastAge := "      "
		lastStatus := " — "
		for _, r := range rows {
			if r.TestName == te.Name {
				lastAge = formatAgePlain(r.StartedAt)
				lastStatus = passLabel(r)
				break
			}
		}
		fmt.Printf("%-20s  %-55s  %6s  %4s\n",
			truncate(catLabel, 20), truncate(te.Name, 55), lastAge, lastStatus)
	}
	return nil
}

// cmdTestRuns prints all recent runs for a specific test name.
func cmdTestRuns(db *runlog.RunDB, testName string, since time.Duration) error {
	rows, err := db.ListRuns(time.Now().Add(-since))
	if err != nil {
		return err
	}
	var matching []runlog.RunRow
	for _, r := range rows {
		if r.TestName == testName {
			matching = append(matching, r)
		}
	}
	if len(matching) == 0 {
		fmt.Printf("no runs found for %q in the last %s\n", testName, since)
		return nil
	}
	fmt.Printf("runs for: %s\n", testName)
	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("%-6s  %-8s  %-8s  %-8s  %-7s\n", "ID", "STATUS", "AGE", "DURATION", "EVENTS")
	fmt.Println(strings.Repeat("─", 80))
	for _, r := range matching {
		fmt.Printf("%-6d  %-8s  %-8s  %-8s  %-7d\n",
			r.ID, passLabel(r), formatAgePlain(r.StartedAt),
			formatDurationPlain(r.StartedAt, r.FinishedAt), r.EventCount)
	}
	return nil
}

// cmdClear deletes the runs database and all per-run log files/directories
// under the same logs/ directory, then reports what was removed.
// It does not require the DB to be open; it works directly on the filesystem.
// For safety, sibling-file cleanup is only performed when the parent directory
// is named "logs", "test-logs", or ".runlog".
func cmdClear(dbPath string) error {
	logsDir := filepath.Dir(dbPath)

	// Remove runs.db itself.
	dbRemoved := false
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("remove %s: %w", dbPath, err)
		}
		dbRemoved = true
	}

	if dbRemoved {
		fmt.Printf("removed: %s\n", dbPath)
	} else {
		fmt.Printf("skipped: %s (not found)\n", dbPath)
	}

	// Only clean siblings when parent dir has a known safe name.
	// This prevents accidentally wiping /tmp or other broad directories when
	// --db points to a non-standard path.
	base := filepath.Base(logsDir)
	if base != "logs" && base != "test-logs" && base != ".runlog" {
		fmt.Printf("skipped sibling cleanup: parent dir %q is not named 'logs', 'test-logs', or '.runlog'\n", logsDir)
		return nil
	}

	// Remove everything directly inside the logs/ directory — subdirectories
	// (per-run log dirs) and any remaining files (e.g. .log files) EXCEPT config files.
	entries, err := os.ReadDir(logsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read dir %s: %w", logsDir, err)
	}

	var removedDirs, removedFiles int
	for _, e := range entries {
		// skip config files
		if e.Name() == "config.yaml" {
			continue
		}
		p := filepath.Join(logsDir, e.Name())
		if e.IsDir() {
			if err := os.RemoveAll(p); err != nil {
				fmt.Fprintf(os.Stderr, "runlog clear: warning: remove dir %s: %v\n", p, err)
				continue
			}
			removedDirs++
		} else {
			if err := os.Remove(p); err != nil {
				fmt.Fprintf(os.Stderr, "runlog clear: warning: remove file %s: %v\n", p, err)
				continue
			}
			removedFiles++
		}
	}

	if removedDirs > 0 || removedFiles > 0 {
		fmt.Printf("removed: %d subdirectories, %d files from %s\n", removedDirs, removedFiles, logsDir)
	} else {
		fmt.Printf("nothing else to remove in %s\n", logsDir)
	}
	return nil
}

// cmdReap marks stale (orphaned) runs as FAIL.
// If runID > 0, only that specific run is reaped.
// If runID == 0, all runs with finished_at IS NULL are reaped.
// When dryRun is true, it prints what would be reaped without changing anything.
func cmdReap(db *runlog.RunDB, runID int64, dryRun bool) error {
	// Collect candidate runs.
	allRuns, err := db.ListRuns(time.Time{})
	if err != nil {
		return err
	}

	var stale []runlog.RunRow
	for _, r := range allRuns {
		if r.FinishedAt != nil {
			continue // already finished
		}
		if r.Skipped {
			continue
		}
		if r.Passed != nil {
			continue
		}
		if runID > 0 && r.ID != runID {
			continue
		}
		stale = append(stale, r)
	}

	if len(stale) == 0 {
		if runID > 0 {
			// Check if the run exists but is already finished.
			for _, r := range allRuns {
				if r.ID == runID {
					fmt.Printf("run %d is already finished (status: %s)\n", runID, passLabel(r))
					return nil
				}
			}
			return fmt.Errorf("run %d not found", runID)
		}
		fmt.Println("no stale runs found")
		return nil
	}

	// Print what we found.
	for _, r := range stale {
		age := formatAgePlain(r.StartedAt)
		fmt.Printf("  %d  %-50s  started %s ago  (%d events)\n",
			r.ID, r.TestName, age, r.EventCount)
	}

	if dryRun {
		fmt.Printf("\ndry-run: would reap %d stale run(s)\n", len(stale))
		return nil
	}

	// Mark each stale run as FAIL.
	reason := "reaped: process died without closing run"
	if runID > 0 {
		// Single run.
		if err := db.FinishRun(runID, stale[0].StartedAt, runlog.OutcomeFail, reason); err != nil {
			return fmt.Errorf("finish run %d: %w", runID, err)
		}
	} else {
		// Batch.
		n, err := db.ReapStaleRuns(reason)
		if err != nil {
			return err
		}
		if int(n) != len(stale) {
			fmt.Fprintf(os.Stderr, "warning: expected to reap %d runs but updated %d\n", len(stale), n)
		}
	}

	fmt.Printf("\nreaped %d stale run(s) → FAIL\n", len(stale))
	return nil
}

// cmdInspect prints the full inspector view for a run: metadata (same as the
// run drawer), followed by every event with the same detail content that the
// TUI inspector panel shows, rendered via buildDetailLines.
func cmdInspect(db *runlog.RunDB, runID int64) error {
	// Resolve the run row.
	rows, err := db.ListRuns(time.Time{})
	if err != nil {
		return err
	}
	var run *runlog.RunRow
	for i := range rows {
		if rows[i].ID == runID {
			run = &rows[i]
			break
		}
	}
	if run == nil {
		return fmt.Errorf("run %d not found", runID)
	}

	// ── Run metadata (mirrors run drawer) ───────────────────────────────────
	passed := "—"
	if run.Skipped {
		passed = plainSkip
	} else if run.Passed != nil {
		if *run.Passed {
			passed = plainPass
		} else {
			passed = plainFail
		}
	}
	fmt.Printf("id:       %d\n", run.ID)
	fmt.Printf("status:   %s\n", passed)
	if run.Reason != nil {
		fmt.Printf("reason:   %s\n", *run.Reason)
	}
	fmt.Printf("started:  %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
	if run.FinishedAt != nil {
		fmt.Printf("finished: %s\n", run.FinishedAt.Format("15:04:05"))
		fmt.Printf("duration: %s\n", formatDurationPlain(run.StartedAt, run.FinishedAt))
	}
	fmt.Printf("events:   %d\n", run.EventCount)
	fmt.Printf("test:     %s\n", run.TestName)
	if run.Description != nil {
		fmt.Printf("description:\n")
		for _, chunk := range wrapText(run.Description.Summary, 80) {
			fmt.Printf("  %s\n", chunk)
		}
		for _, b := range run.Description.Bullets {
			fmt.Printf("  • %s\n", b)
		}
	}
	if run.TokenSummary != nil && (run.TokenSummary.InputTokens > 0 || run.TokenSummary.OutputTokens > 0) {
		fmt.Printf("tokens:   %s in / %s out\n",
			formatTokenCount(run.TokenSummary.InputTokens),
			formatTokenCount(run.TokenSummary.OutputTokens))
		if run.TokenSummary.CostUSD > 0 {
			fmt.Printf("cost:     $%.6f\n", run.TokenSummary.CostUSD)
		}
	}
	if len(run.Tags) > 0 {
		fmt.Printf("tags:     %s\n", strings.Join(run.Tags, ", "))
	}
	if run.Experiment != nil && *run.Experiment != "" {
		fmt.Printf("experiment: %s\n", *run.Experiment)
	}

	// ── Events with inspector detail ─────────────────────────────────────────
	evs, err := db.ListEvents(runID)
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		fmt.Println("\n(no events)")
		return nil
	}

	const inspectorWidth = 80

	for _, ev := range evs {
		fmt.Println()
		// Event header line: elapsed, kind, message (same as event list row)
		fmt.Printf("  [%6.1fs]  %-14s  %s\n", ev.ElapsedS, ev.Kind, ev.Message)

		// Inspector detail for this event (same rendering as TUI inspector panel)
		detailLines := buildDetailLines(ev, nil, inspectorWidth)
		for _, l := range detailLines {
			fmt.Println(stripANSI(l))
		}

		// Children (expanded, same as when section is open in TUI)
		for ci := range ev.Children {
			child := ev.Children[ci]
			fmt.Printf("    · [%6.1fs]  %-12s  %s\n", child.ElapsedS, child.Kind, child.Message)
			childLines := buildDetailLines(ev, &child, inspectorWidth)
			for _, l := range childLines {
				fmt.Println("    " + stripANSI(l))
			}
		}
	}
	return nil
}

// cmdAnalyze runs the LLM analyzer for a single run and prints the full
// conversation trace (system prompt, user message, thoughts, tool calls,
// tool results, text output, token usage) followed by the suggestions.
// With --json it outputs the suggestions as a JSON array instead.
func cmdAnalyze(db *runlog.RunDB, runID int64, jsonOut bool) error {
	analyzer, err := runlog.NewAnalyzer(db)
	if err != nil {
		return fmt.Errorf("create analyzer: %w", err)
	}

	// Collect trace events.
	var events []runlog.AnalyzerEvent
	analyzer.OnEvent = func(ev runlog.AnalyzerEvent) {
		events = append(events, ev)
		// Stream a one-liner to stderr so the user sees progress.
		line := fmtTraceOneLiner(ev)
		if line != "" {
			fmt.Fprintln(os.Stderr, line)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Analysing run %d…\n\n", runID)
	suggestions, err := analyzer.RunByRunID(ctx, runID)
	if err != nil {
		return fmt.Errorf("run analyzer: %w", err)
	}

	// ── Full trace dump to stdout ───────────────────────────────────────────
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  ANALYSIS TRACE — run #%d  (%d events, %d suggestions)\n", runID, len(events), len(suggestions))
	fmt.Println(strings.Repeat("=", 80))

	for i, ev := range events {
		fmt.Println()
		fmt.Printf("── event %d: %s", i+1, ev.Kind)
		if ev.Author != "" {
			fmt.Printf("  [%s]", ev.Author)
		}
		fmt.Println()
		fmt.Println(strings.Repeat("─", 60))

		switch ev.Kind {
		case runlog.AESystemPrompt:
			fmt.Println(ev.Content)

		case runlog.AEUserMessage:
			fmt.Println(ev.Content)

		case runlog.AEThought:
			fmt.Println(ev.Content)

		case runlog.AEText:
			fmt.Println(ev.Content)

		case runlog.AEToolCall:
			fmt.Printf("tool: %s\n", ev.ToolName)
			if ev.ToolArgs != nil {
				argsJSON, _ := json.MarshalIndent(ev.ToolArgs, "", "  ")
				fmt.Printf("args:\n%s\n", string(argsJSON))
			}

		case runlog.AEToolResult:
			fmt.Printf("tool: %s\n", ev.ToolName)
			if ev.ToolResponse != nil {
				respJSON, _ := json.MarshalIndent(ev.ToolResponse, "", "  ")
				fmt.Printf("response:\n%s\n", string(respJSON))
			}

		case runlog.AETokenUsage:
			fmt.Printf("prompt:  %d\n", ev.PromptTokens)
			fmt.Printf("output:  %d\n", ev.OutputTokens)
			fmt.Printf("thought: %d\n", ev.ThoughtTokens)
			fmt.Printf("total:   %d\n", ev.TotalTokens)

		case runlog.AEError:
			if ev.ErrorCode != "" {
				fmt.Printf("code: %s\n", ev.ErrorCode)
			}
			fmt.Printf("message: %s\n", ev.ErrorMessage)

		case runlog.AETurnComplete:
			fmt.Println("(turn complete)")
		}
	}

	// ── Suggestions ─────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  SUGGESTIONS (%d)\n", len(suggestions))
	fmt.Println(strings.Repeat("=", 80))

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(suggestions)
	}

	if len(suggestions) == 0 {
		fmt.Println("\n  (no suggestions generated)")
		return nil
	}

	for i, s := range suggestions {
		fmt.Println()
		fmt.Printf("--- %d. [%s] %s ---\n", i+1, strings.ToUpper(s.Priority), s.Title)
		fmt.Printf("category: %s\n", s.Category)
		if len(s.RunIDs) > 0 {
			ids := make([]string, len(s.RunIDs))
			for j, id := range s.RunIDs {
				ids[j] = fmt.Sprintf("%d", id)
			}
			fmt.Printf("run IDs:  %s\n", strings.Join(ids, ", "))
		}
		fmt.Printf("\n%s\n", s.Body)
	}

	return nil
}

// cmdTrace prints the stored analysis trace for a run from the database.
// If no trace exists, it prints a message and exits cleanly.
func cmdTrace(db *runlog.RunDB, runID int64) error {
	traceID, err := db.GetLatestTraceForRun(runID)
	if err != nil {
		return fmt.Errorf("lookup trace: %w", err)
	}
	if traceID == 0 {
		fmt.Fprintf(os.Stderr, "No analysis trace found for run %d.\n", runID)
		fmt.Fprintf(os.Stderr, "Run 'runlog analyze %d' first to generate one.\n", runID)
		return nil
	}

	events, err := db.ListAnalyzerTraceEvents(traceID)
	if err != nil {
		return fmt.Errorf("load trace events: %w", err)
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  ANALYSIS TRACE — run #%d  (%d events)\n", runID, len(events))
	fmt.Println(strings.Repeat("=", 80))

	for i, ev := range events {
		fmt.Println()
		fmt.Printf("── event %d: %s", i+1, ev.Kind)
		if ev.Author != "" {
			fmt.Printf("  [%s]", ev.Author)
		}
		fmt.Println()
		fmt.Println(strings.Repeat("─", 60))

		switch ev.Kind {
		case runlog.AESystemPrompt, runlog.AEUserMessage, runlog.AEThought, runlog.AEText:
			fmt.Println(ev.Content)

		case runlog.AEToolCall:
			fmt.Printf("tool: %s\n", ev.ToolName)
			if ev.ToolArgs != nil {
				argsJSON, _ := json.MarshalIndent(ev.ToolArgs, "", "  ")
				fmt.Printf("args:\n%s\n", string(argsJSON))
			}

		case runlog.AEToolResult:
			fmt.Printf("tool: %s\n", ev.ToolName)
			if ev.ToolResponse != nil {
				respJSON, _ := json.MarshalIndent(ev.ToolResponse, "", "  ")
				fmt.Printf("response:\n%s\n", string(respJSON))
			}

		case runlog.AETokenUsage:
			fmt.Printf("prompt:  %d\n", ev.PromptTokens)
			fmt.Printf("output:  %d\n", ev.OutputTokens)
			fmt.Printf("thought: %d\n", ev.ThoughtTokens)
			fmt.Printf("total:   %d\n", ev.TotalTokens)

		case runlog.AEError:
			if ev.ErrorCode != "" {
				fmt.Printf("code: %s\n", ev.ErrorCode)
			}
			fmt.Printf("message: %s\n", ev.ErrorMessage)

		case runlog.AETurnComplete:
			fmt.Println("(turn complete)")
		}
	}

	// Also show suggestions for this run.
	suggKey := fmt.Sprintf("run:%d", runID)
	suggestions, err := db.ListSuggestions(suggKey)
	if err != nil {
		return fmt.Errorf("load suggestions: %w", err)
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  SUGGESTIONS (%d)\n", len(suggestions))
	fmt.Println(strings.Repeat("=", 80))

	if len(suggestions) == 0 {
		fmt.Println("\n  (no suggestions)")
		return nil
	}

	for i, s := range suggestions {
		fmt.Println()
		fmt.Printf("--- %d. [%s] %s ---\n", i+1, strings.ToUpper(s.Priority), s.Title)
		fmt.Printf("category: %s\n", s.Category)
		if len(s.RunIDs) > 0 {
			ids := make([]string, len(s.RunIDs))
			for j, id := range s.RunIDs {
				ids[j] = fmt.Sprintf("%d", id)
			}
			fmt.Printf("run IDs:  %s\n", strings.Join(ids, ", "))
		}
		fmt.Printf("\n%s\n", s.Body)
	}

	return nil
}

// fmtTraceOneLiner returns a compact one-line summary for a trace event,
// suitable for streaming progress to stderr.  Returns "" for events that
// should be suppressed (turn_complete, etc.).
func fmtTraceOneLiner(ev runlog.AnalyzerEvent) string {
	prefix := ""
	if ev.Author != "" {
		prefix = "[" + ev.Author + "] "
	}
	switch ev.Kind {
	case runlog.AESystemPrompt:
		lines := strings.Count(ev.Content, "\n") + 1
		return prefix + fmt.Sprintf("SYSTEM PROMPT (%d lines)", lines)
	case runlog.AEUserMessage:
		lines := strings.Count(ev.Content, "\n") + 1
		return prefix + fmt.Sprintf("USER MESSAGE (%d lines)", lines)
	case runlog.AEThought:
		c := strings.NewReplacer("\n", " ", "\r", "").Replace(ev.Content)
		if len(c) > 120 {
			c = c[:117] + "..."
		}
		return prefix + "thought: " + c
	case runlog.AEText:
		c := strings.NewReplacer("\n", " ", "\r", "").Replace(ev.Content)
		if len(c) > 200 {
			c = c[:197] + "..."
		}
		return prefix + c
	case runlog.AEToolCall:
		return prefix + ">> " + ev.Content
	case runlog.AEToolResult:
		c := ev.Content
		if len(c) > 200 {
			c = c[:197] + "..."
		}
		return prefix + "<< " + c
	case runlog.AETokenUsage:
		return prefix + ev.Content
	case runlog.AEError:
		return prefix + "ERROR: " + ev.Content
	default:
		return ""
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — View state constants
// ─────────────────────────────────────────────────────────────────────────────

type viewState int

const (
	viewRuns         viewState = iota // main runs table (tab 0)
	viewEvents                        // event list for selected run
	viewExperiments                   // experiments tab (tab 1)
	viewExpRuns                       // drill-in: runs for a selected experiment
	viewExpAnalysis                   // LLM analysis for a selected experiment
	viewTests                         // tests tab (tab 2)
	viewTestRuns                      // drill-in: runs for a selected test
	viewTestAnalysis                  // LLM analysis for a selected test
	viewRunAnalysis                   // LLM analysis for a single run
)

// tabNames is the ordered list of tab labels shown in the tab bar.
var tabNames = []string{"Runs", "Experiments", "Tests"}

// spinnerFrames are used to animate the in-flight status indicator.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Known tests registry
// ─────────────────────────────────────────────────────────────────────────────

type testEntry struct {
	Name     string
	Category string
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Message types
// ─────────────────────────────────────────────────────────────────────────────

type tickMsg struct{}
type errMsg struct{ err error }
type runsLoadedMsg struct{ rows []runlog.RunRow }
type eventsLoadedMsg struct {
	rows      []runlog.EventRow
	resetView bool
}
type experimentsLoadedMsg struct{ exps []runlog.ExperimentSummary }
type analysisLoadedMsg struct{ suggestions []runlog.SuggestionRow }
type analyzeErrMsg struct{ err error }
type testAnalysisLoadedMsg struct{ suggestions []runlog.SuggestionRow }
type testAnalyzeErrMsg struct{ err error }
type runAnalysisLoadedMsg struct{ suggestions []runlog.SuggestionRow }
type runAnalyzeErrMsg struct{ err error }
type runAnalysisTraceMsg struct{ event runlog.AnalyzerEvent }
type drawerSuggestionsLoadedMsg struct {
	exp         string
	suggestions []runlog.SuggestionRow
}
type testLaunchedMsg struct {
	launcherID int64
	pid        int
}
type testLaunchErrMsg struct{ err error }
type testKilledMsg struct{}
type testKillErrMsg struct{ err error }
type testsLoadedMsg struct{ entries []testEntry }
type activeLauncherMsg struct {
	launcherID int64
	pid        int
}

// displayItem is an entry in the flat display list used by viewEvents.
// It references an event by index into m.events and optionally a child index.
type displayItem struct {
	eventIdx int
	childIdx int // -1 for top-level event rows
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Model
// ─────────────────────────────────────────────────────────────────────────────

type model struct {
	db    *runlog.RunDB
	since time.Duration

	// Terminal dimensions
	width  int
	height int

	// Active view
	state     viewState
	activeTab int // 0=Runs, 1=Experiments, 2=Tests

	// Runs tab
	runs        []runlog.RunRow
	runCursor   int
	runOffset   int
	filterTest  string // if set, show only runs for this test name
	lastRefresh time.Time

	// Events view (drill-in from Runs)
	selectedRun  runlog.RunRow
	events       []runlog.EventRow
	displayList  []displayItem // flat list including expanded children
	expanded     map[int64]bool
	listCursor   int
	listOffset   int
	drawerLines  []string
	drawerOffset int

	// Experiments tab
	experiments []runlog.ExperimentSummary
	expCursor   int

	// Experiment runs (drill-in)
	selectedExp  runlog.ExperimentSummary
	expRuns      []runlog.RunRow
	expRunCursor int
	expRunOffset int

	// Experiment analysis
	suggestions  []runlog.SuggestionRow
	suggCursor   int
	suggOffset   int
	analyzingExp bool
	analyzeErr   string
	analyzer     *runlog.Analyzer

	// Test analysis (single test, Tests tab drill-in)
	testSuggestions []runlog.SuggestionRow
	testSuggCursor  int
	testSuggOffset  int
	analyzingTest   bool
	testAnalyzeErr  string

	// Single-run analysis (Events view drill-in)
	runSuggestions       []runlog.SuggestionRow
	runSuggCursor        int
	runSuggOffset        int
	analyzingRun         bool
	runAnalyzeErr        string
	runAnalysisTrace     []runlog.AnalyzerEvent    // conversation trace events from OnEvent
	runTraceOffset       int                       // scroll offset in trace view (left panel)
	runTraceCursor       int                       // selected trace event index
	runTraceDrawerOffset int                       // scroll offset in trace detail drawer (right panel)
	runTraceCh           chan runlog.AnalyzerEvent // channel for streaming trace events from analyzer
	showRunTrace         bool                      // when true, show trace instead of suggestions

	// Drawer suggestions (shown inline in experiments list)
	drawerSuggestionsExp string
	drawerSuggestions    []runlog.SuggestionRow

	// Tests tab
	testCursor  int
	testOffset  int
	testEntries []testEntry // populated from DB + config at startup

	// Config (loaded from .runlog.yaml if present)
	config *runlog.Config

	// Test runs (drill-in)
	selectedTest  testEntry
	testRunCursor int
	testRunOffset int

	// Test launcher
	testEnv           string // env name for ./test invocations
	testLaunching     bool
	testLaunchSuccess bool
	testLaunchErr     string
	activePID         int
	activeLauncherID  int64

	// Search
	searchActive bool   // true when the user is typing a search query
	searchQuery  string // committed search filter (applied to table rows)
	searchInput  string // in-progress input buffer while searchActive is true

	// Spinner
	spinnerFrame int
}

// newModel creates the initial TUI model.
func newModel(db *runlog.RunDB, since time.Duration, cfg *runlog.Config) model {
	env := os.Getenv("MEMORY_TEST_ENV")
	if env == "" {
		env = "mcj-emergent"
	}
	if cfg == nil {
		cfg = &runlog.Config{}
	}
	return model{
		db:       db,
		since:    since,
		width:    80,
		height:   24,
		expanded: make(map[int64]bool),
		testEnv:  env,
		config:   cfg,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Init / Update / View
// ─────────────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.loadRuns(),
		m.loadExperiments(),
		m.loadTests(),
		m.loadActiveLauncher(),
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg{} }),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(
			m.loadRuns(),
			m.loadTests(),
			m.loadActiveLauncher(),
			tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg{} }),
		)

	case runsLoadedMsg:
		m.runs = msg.rows
		m.lastRefresh = time.Now()
		// Keep cursor in bounds.
		visible := m.filteredRuns()
		if m.runCursor >= len(visible) && len(visible) > 0 {
			m.runCursor = len(visible) - 1
		}
		m.ensureRunVisible()
		return m, nil

	case eventsLoadedMsg:
		m.events = msg.rows
		if msg.resetView {
			m.listCursor = 0
			m.listOffset = 0
			m.drawerOffset = 0
		}
		// Rebuild flat display list.
		m.displayList = m.buildDisplayList()
		if m.listCursor < len(m.displayList) {
			item := m.displayList[m.listCursor]
			ev := m.events[item.eventIdx]
			var child *runlog.ChildEvent
			if item.childIdx >= 0 {
				child = &ev.Children[item.childIdx]
			}
			m.drawerLines = buildDetailLines(ev, child, m.drawerWidth())
		}
		return m, nil

	case experimentsLoadedMsg:
		m.experiments = msg.exps
		if m.expCursor >= len(m.experiments) && len(m.experiments) > 0 {
			m.expCursor = len(m.experiments) - 1
		}
		// Load drawer suggestions for currently selected experiment.
		if len(m.experiments) > 0 && m.expCursor < len(m.experiments) {
			return m, m.loadDrawerSuggestions(m.experiments[m.expCursor].Name)
		}
		return m, nil

	case testsLoadedMsg:
		m.testEntries = msg.entries
		if m.testCursor >= len(m.testEntries) && len(m.testEntries) > 0 {
			m.testCursor = len(m.testEntries) - 1
		}
		return m, nil

	case drawerSuggestionsLoadedMsg:
		m.drawerSuggestionsExp = msg.exp
		m.drawerSuggestions = msg.suggestions
		return m, nil

	case analysisLoadedMsg:
		m.suggestions = msg.suggestions
		m.analyzingExp = false
		m.suggCursor = 0
		m.suggOffset = 0
		return m, nil

	case analyzeErrMsg:
		m.analyzeErr = msg.err.Error()
		m.analyzingExp = false
		return m, nil

	case testAnalysisLoadedMsg:
		m.testSuggestions = msg.suggestions
		m.analyzingTest = false
		m.testSuggCursor = 0
		m.testSuggOffset = 0
		return m, nil

	case testAnalyzeErrMsg:
		m.testAnalyzeErr = msg.err.Error()
		m.analyzingTest = false
		return m, nil

	case runAnalysisLoadedMsg:
		m.runSuggestions = msg.suggestions
		m.analyzingRun = false
		m.runSuggCursor = 0
		m.runSuggOffset = 0
		return m, nil

	case runAnalyzeErrMsg:
		m.runAnalyzeErr = msg.err.Error()
		m.analyzingRun = false
		return m, nil

	case runAnalysisTraceMsg:
		m.runAnalysisTrace = append(m.runAnalysisTrace, msg.event)
		// Auto-scroll to bottom and keep cursor at latest while analyzing.
		if m.analyzingRun {
			m.runTraceCursor = len(m.runAnalysisTrace) - 1
			visHeight := m.visibleRunSuggRows() - 1 // minus header line
			if len(m.runAnalysisTrace) > visHeight {
				m.runTraceOffset = len(m.runAnalysisTrace) - visHeight
			}
		}
		return m, m.listenRunTrace()

	case testLaunchedMsg:
		m.testLaunching = false
		m.testLaunchSuccess = true
		m.activePID = msg.pid
		m.activeLauncherID = msg.launcherID
		return m, nil

	case testLaunchErrMsg:
		m.testLaunching = false
		m.testLaunchErr = msg.err.Error()
		return m, nil

	case testKilledMsg:
		m.activePID = 0
		m.activeLauncherID = 0
		return m, nil

	case testKillErrMsg:
		m.testLaunchErr = msg.err.Error()
		return m, nil

	case activeLauncherMsg:
		m.activePID = msg.pid
		m.activeLauncherID = msg.launcherID
		return m, nil

	case errMsg:
		// silently ignore DB errors (DB may not exist yet)
		return m, nil

	case tea.KeyMsg:
		// ── Search input mode ─────────────────────────────────────────────
		if m.searchActive {
			switch msg.Type {
			case tea.KeyEscape:
				// Cancel search input and clear any committed query.
				m.searchActive = false
				m.searchInput = ""
				m.searchQuery = ""
				m.resetCursorsForSearch()
				return m, nil
			case tea.KeyEnter:
				// Commit search — apply filter and reset cursors.
				m.searchActive = false
				m.searchQuery = m.searchInput
				m.searchInput = ""
				m.resetCursorsForSearch()
				return m, nil
			case tea.KeyBackspace:
				if len(m.searchInput) > 0 {
					m.searchInput = m.searchInput[:len(m.searchInput)-1]
				}
				return m, nil
			default:
				// Append printable characters.
				if msg.Type == tea.KeyRunes {
					m.searchInput += string(msg.Runes)
				} else if msg.Type == tea.KeySpace {
					m.searchInput += " "
				}
				return m, nil
			}
		}

		switch m.state {

		// ── Runs view ──────────────────────────────────────────────────────
		case viewRuns:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.runCursor = 0
					m.runOffset = 0
					return m, nil
				}
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery // pre-fill with existing query
				return m, nil
			case "tab":
				m.searchQuery = ""
				m.activeTab = (m.activeTab + 1) % len(tabNames)
				switch m.activeTab {
				case 0:
					m.state = viewRuns
				case 1:
					m.state = viewExperiments
				case 2:
					m.state = viewTests
				}
				return m, nil
			case "up", "k":
				visible := m.filteredRuns()
				if m.runCursor > 0 {
					m.runCursor--
					m.ensureRunVisible()
				}
				_ = visible
				return m, nil
			case "down", "j":
				visible := m.filteredRuns()
				if m.runCursor < len(visible)-1 {
					m.runCursor++
					m.ensureRunVisible()
				}
				return m, nil
			case "enter":
				visible := m.filteredRuns()
				if len(visible) > 0 && m.runCursor < len(visible) {
					m.selectedRun = visible[m.runCursor]
					m.state = viewEvents
					return m, m.loadEventsReset(m.selectedRun.ID)
				}
			case "f":
				// Toggle filter: if already filtering, clear; otherwise filter to cursor run.
				visible := m.filteredRuns()
				if m.filterTest != "" {
					m.filterTest = ""
					m.runCursor = 0
					m.runOffset = 0
				} else if len(visible) > 0 && m.runCursor < len(visible) {
					m.filterTest = visible[m.runCursor].TestName
					m.runCursor = 0
					m.runOffset = 0
				}
				return m, nil
			case "r":
				m.lastRefresh = time.Time{}
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			case "t":
				visible := m.filteredRuns()
				if len(visible) > 0 && m.runCursor < len(visible) {
					m.testLaunchErr = ""
					m.testLaunchSuccess = false
					m.testLaunching = true
					return m, m.launchTest(visible[m.runCursor].TestName)
				}
			case "x":
				if m.activePID != 0 {
					return m, m.killTest(m.activeLauncherID, m.activePID)
				}
			case "a", "A":
				visible := m.filteredRuns()
				if len(visible) > 0 && m.runCursor < len(visible) {
					m.selectedRun = visible[m.runCursor]
					newM, cmd := m.triggerRunAnalysis(false)
					return newM, cmd
				}
				return m, nil
			}

		// ── Events view ────────────────────────────────────────────────────
		case viewEvents:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.listCursor = 0
					m.listOffset = 0
					return m, nil
				}
				m.state = viewRuns
				return m, nil
			case "up", "k":
				filtered := m.searchFilteredDisplayList()
				if m.listCursor > 0 {
					m.listCursor--
					if m.listCursor < m.listOffset {
						m.listOffset = m.listCursor
					}
					m.drawerOffset = 0
					if m.listCursor < len(filtered) {
						item := filtered[m.listCursor]
						ev := m.events[item.eventIdx]
						var child *runlog.ChildEvent
						if item.childIdx >= 0 {
							child = &ev.Children[item.childIdx]
						}
						m.drawerLines = buildDetailLines(ev, child, m.drawerWidth())
					}
				}
				return m, nil
			case "down", "j":
				filtered := m.searchFilteredDisplayList()
				if m.listCursor < len(filtered)-1 {
					m.listCursor++
					vis := m.visibleEventRows() - 1
					if m.listCursor >= m.listOffset+vis {
						m.listOffset = m.listCursor - vis + 1
					}
					m.drawerOffset = 0
					if m.listCursor < len(filtered) {
						item := filtered[m.listCursor]
						ev := m.events[item.eventIdx]
						var child *runlog.ChildEvent
						if item.childIdx >= 0 {
							child = &ev.Children[item.childIdx]
						}
						m.drawerLines = buildDetailLines(ev, child, m.drawerWidth())
					}
				}
				return m, nil
			case "enter", " ":
				// Toggle expand/collapse for the selected event.
				filtered := m.searchFilteredDisplayList()
				if m.listCursor < len(filtered) {
					item := filtered[m.listCursor]
					if item.childIdx < 0 {
						ev := m.events[item.eventIdx]
						if len(ev.Children) > 0 {
							m.expanded[ev.ID] = !m.expanded[ev.ID]
							m.displayList = m.buildDisplayList()
						}
					}
				}
				return m, nil
			case "[":
				if m.drawerOffset > 0 {
					m.drawerOffset--
				}
				return m, nil
			case "]":
				if m.drawerOffset < len(m.drawerLines)-1 {
					m.drawerOffset++
				}
				return m, nil
			case "r":
				return m, m.loadEvents(m.selectedRun.ID)
			case "a", "A":
				newM, cmd := m.triggerRunAnalysis(false)
				return newM, cmd
			}

		// ── Experiments view ───────────────────────────────────────────────
		case viewExperiments:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.expCursor = 0
					return m, nil
				}
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "tab":
				m.searchQuery = ""
				m.activeTab = (m.activeTab + 1) % len(tabNames)
				switch m.activeTab {
				case 0:
					m.state = viewRuns
				case 1:
					m.state = viewExperiments
				case 2:
					m.state = viewTests
				}
				return m, nil
			case "up", "k":
				filteredExps := m.searchFilteredExperiments()
				if m.expCursor > 0 {
					m.expCursor--
					if m.expCursor < len(filteredExps) {
						return m, m.loadDrawerSuggestions(filteredExps[m.expCursor].Name)
					}
				}
				return m, nil
			case "down", "j":
				filteredExps := m.searchFilteredExperiments()
				if m.expCursor < len(filteredExps)-1 {
					m.expCursor++
					return m, m.loadDrawerSuggestions(filteredExps[m.expCursor].Name)
				}
				return m, nil
			case "enter":
				filteredExps := m.searchFilteredExperiments()
				if m.expCursor < len(filteredExps) {
					m.selectedExp = filteredExps[m.expCursor]
					m.expRuns = nil
					m.expRunCursor = 0
					m.expRunOffset = 0
					m.searchQuery = "" // clear search when drilling in
					m.state = viewExpRuns
					// Filter m.runs to just those for this experiment.
					var filtered []runlog.RunRow
					for _, r := range m.runs {
						if r.TestName == m.selectedExp.Name {
							filtered = append(filtered, r)
						}
					}
					m.expRuns = filtered
				}
				return m, nil
			case "a", "A":
				if m.expCursor < len(m.experiments) {
					m.selectedExp = m.experiments[m.expCursor]
					var newM model
					newM, cmd := m.triggerAnalysis(false)
					return newM, cmd
				}
				return m, nil
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		// ── Experiment runs view ───────────────────────────────────────────
		case viewExpRuns:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.expRunCursor = 0
					m.expRunOffset = 0
					return m, nil
				}
				m.state = viewExperiments
				return m, nil
			case "up", "k":
				filtered := m.searchFilteredExpRuns()
				if m.expRunCursor > 0 {
					m.expRunCursor--
					if m.expRunCursor < m.expRunOffset {
						m.expRunOffset = m.expRunCursor
					}
				}
				_ = filtered
				return m, nil
			case "down", "j":
				filtered := m.searchFilteredExpRuns()
				if m.expRunCursor < len(filtered)-1 {
					m.expRunCursor++
					vis := m.visibleExpRunRows() - 1
					if m.expRunCursor >= m.expRunOffset+vis {
						m.expRunOffset = m.expRunCursor - vis + 1
					}
				}
				return m, nil
			case "enter":
				filtered := m.searchFilteredExpRuns()
				if m.expRunCursor < len(filtered) {
					m.selectedRun = filtered[m.expRunCursor]
					m.state = viewEvents
					return m, m.loadEventsReset(m.selectedRun.ID)
				}
			case "a", "A":
				newM, cmd := m.triggerAnalysis(false)
				return newM, cmd
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		// ── Experiment analysis view ───────────────────────────────────────
		case viewExpAnalysis:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.suggCursor = 0
					m.suggOffset = 0
					return m, nil
				}
				m.state = viewExpRuns
				return m, nil
			case "up", "k":
				filtered := m.searchFilteredSuggestions()
				if m.suggCursor > 0 {
					m.suggCursor--
					if m.suggCursor < m.suggOffset {
						m.suggOffset = m.suggCursor
					}
				}
				_ = filtered
				return m, nil
			case "down", "j":
				filtered := m.searchFilteredSuggestions()
				if m.suggCursor < len(filtered)-1 {
					m.suggCursor++
					vis := m.visibleSuggRows()
					if m.suggCursor >= m.suggOffset+vis {
						m.suggOffset = m.suggCursor - vis + 1
					}
				}
				return m, nil
			case "a", "A":
				newM, cmd := m.triggerAnalysis(true)
				return newM, cmd
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		// ── Tests view ─────────────────────────────────────────────────────
		case viewTests:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "esc":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.testCursor = 0
					m.testOffset = 0
					return m, nil
				}
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "tab":
				m.searchQuery = ""
				m.activeTab = (m.activeTab + 1) % len(tabNames)
				switch m.activeTab {
				case 0:
					m.state = viewRuns
				case 1:
					m.state = viewExperiments
				case 2:
					m.state = viewTests
				}
				return m, nil
			case "up", "k":
				filteredTests := m.searchFilteredTests()
				if m.testCursor > 0 {
					m.testCursor--
					vis := m.visibleTestRows()
					if m.testCursor < m.testOffset {
						m.testOffset = m.testCursor
					}
					_ = vis
				}
				_ = filteredTests
				return m, nil
			case "down", "j":
				filteredTests := m.searchFilteredTests()
				if m.testCursor < len(filteredTests)-1 {
					m.testCursor++
					vis := m.visibleTestRows()
					if m.testCursor >= m.testOffset+vis {
						m.testOffset = m.testCursor - vis + 1
					}
				}
				return m, nil
			case "enter":
				filteredTests := m.searchFilteredTests()
				if m.testCursor < len(filteredTests) {
					m.selectedTest = filteredTests[m.testCursor]
					m.testRunCursor = 0
					m.testRunOffset = 0
					m.activePID = 0
					m.activeLauncherID = 0
					m.testLaunchErr = ""
					m.testLaunchSuccess = false
					m.searchQuery = "" // clear search when drilling in
					m.state = viewTestRuns
					return m, m.loadActiveLauncher()
				}
				return m, nil
			case "t":
				filteredTests := m.searchFilteredTests()
				if m.testCursor < len(filteredTests) {
					m.selectedTest = filteredTests[m.testCursor]
					m.testLaunchErr = ""
					m.testLaunchSuccess = false
					m.testLaunching = true
					m.activePID = 0
					m.activeLauncherID = 0
					return m, m.launchTest(m.selectedTest.Name)
				}
			case "x":
				if m.activePID != 0 {
					return m, m.killTest(m.activeLauncherID, m.activePID)
				}
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		// ── Test runs view ─────────────────────────────────────────────────
		case viewTestRuns:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.testRunCursor = 0
					m.testRunOffset = 0
					return m, nil
				}
				m.state = viewTests
				return m, nil
			case "up", "k":
				runsForTest := m.testFilteredRuns()
				if m.testRunCursor > 0 {
					m.testRunCursor--
					if m.testRunCursor < m.testRunOffset {
						m.testRunOffset = m.testRunCursor
					}
				}
				_ = runsForTest
				return m, nil
			case "down", "j":
				runsForTest := m.testFilteredRuns()
				if m.testRunCursor < len(runsForTest)-1 {
					m.testRunCursor++
					vis := m.visibleTestRunRows()
					if m.testRunCursor >= m.testRunOffset+vis {
						m.testRunOffset = m.testRunCursor - vis + 1
					}
				}
				return m, nil
			case "enter":
				runsForTest := m.testFilteredRuns()
				if m.testRunCursor < len(runsForTest) {
					m.selectedRun = runsForTest[m.testRunCursor]
					m.state = viewEvents
					return m, m.loadEventsReset(m.selectedRun.ID)
				}
			case "t":
				m.testLaunchErr = ""
				m.testLaunchSuccess = false
				m.testLaunching = true
				return m, m.launchTest(m.selectedTest.Name)
			case "a", "A":
				newM, cmd := m.triggerTestAnalysis(false)
				return newM, cmd
			case "x":
				if m.activePID != 0 {
					return m, m.killTest(m.activeLauncherID, m.activePID)
				}
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		case viewTestAnalysis:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.testSuggCursor = 0
					m.testSuggOffset = 0
					return m, nil
				}
				m.state = viewTestRuns
				return m, nil
			case "up", "k":
				filtered := m.searchFilteredTestSuggestions()
				if m.testSuggCursor > 0 {
					m.testSuggCursor--
					if m.testSuggCursor < m.testSuggOffset {
						m.testSuggOffset = m.testSuggCursor
					}
				}
				_ = filtered
				return m, nil
			case "down", "j":
				filtered := m.searchFilteredTestSuggestions()
				if m.testSuggCursor < len(filtered)-1 {
					m.testSuggCursor++
					vis := m.visibleTestSuggRows()
					if m.testSuggCursor >= m.testSuggOffset+vis {
						m.testSuggOffset = m.testSuggCursor - vis + 1
					}
				}
				return m, nil
			case "a", "A":
				newM, cmd := m.triggerTestAnalysis(true)
				return newM, cmd
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}

		// ── Run Analysis view ─────────────────────────────────────────────
		case viewRunAnalysis:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "/":
				m.searchActive = true
				m.searchInput = m.searchQuery
				return m, nil
			case "esc", "backspace":
				if m.searchQuery != "" {
					m.searchQuery = ""
					m.runSuggCursor = 0
					m.runSuggOffset = 0
					m.runTraceCursor = 0
					m.runTraceOffset = 0
					return m, nil
				}
				if m.showRunTrace && !m.analyzingRun {
					// If viewing trace after completion, go back to suggestions.
					m.showRunTrace = false
					return m, nil
				}
				m.state = viewEvents
				return m, nil
			case "up", "k":
				if m.analyzingRun || m.showRunTrace {
					// Move trace cursor up.
					if m.runTraceCursor > 0 {
						m.runTraceCursor--
						m.runTraceDrawerOffset = 0 // reset drawer scroll on cursor move
						// Scroll if cursor goes above visible area.
						if m.runTraceCursor < m.runTraceOffset {
							m.runTraceOffset = m.runTraceCursor
						}
					}
				} else if m.runSuggCursor > 0 {
					m.runSuggCursor--
					if m.runSuggCursor < m.runSuggOffset {
						m.runSuggOffset = m.runSuggCursor
					}
				}
				return m, nil
			case "down", "j":
				if m.analyzingRun || m.showRunTrace {
					// Move trace cursor down.
					if m.runTraceCursor < len(m.runAnalysisTrace)-1 {
						m.runTraceCursor++
						m.runTraceDrawerOffset = 0 // reset drawer scroll on cursor move
						// Scroll if cursor goes below visible area.
						vis := m.visibleRunSuggRows() - 1 // minus header line
						if m.runTraceCursor >= m.runTraceOffset+vis {
							m.runTraceOffset = m.runTraceCursor - vis + 1
						}
					}
				} else {
					filtered := m.searchFilteredRunSuggestions()
					if m.runSuggCursor < len(filtered)-1 {
						m.runSuggCursor++
						vis := m.visibleRunSuggRows()
						if m.runSuggCursor >= m.runSuggOffset+vis {
							m.runSuggOffset = m.runSuggCursor - vis + 1
						}
					}
				}
				return m, nil
			case "left", "h":
				// Scroll drawer up in trace view.
				if (m.analyzingRun || m.showRunTrace) && m.runTraceDrawerOffset > 0 {
					m.runTraceDrawerOffset--
				}
				return m, nil
			case "right", "l":
				// Scroll drawer down in trace view.
				if m.analyzingRun || m.showRunTrace {
					m.runTraceDrawerOffset++
				}
				return m, nil
			case "t":
				// Toggle between suggestions and trace history.
				if !m.analyzingRun && len(m.runAnalysisTrace) > 0 {
					m.showRunTrace = !m.showRunTrace
					if m.showRunTrace {
						// Scroll to end of trace by default.
						vis := m.visibleRunSuggRows() - 1 // minus header line
						maxOff := len(m.runAnalysisTrace) - vis
						if maxOff < 0 {
							maxOff = 0
						}
						m.runTraceOffset = maxOff
					}
				}
				return m, nil
			case "a", "A":
				newM, cmd := m.triggerRunAnalysis(true)
				return newM, cmd
			case "r":
				return m, tea.Batch(m.loadRuns(), m.loadExperiments())
			}
		}

		// Spinner tick — advance frame on every key press too (cosmetic).
		m.spinnerFrame++
	}
	return m, nil
}

func (m model) View() string {
	var out string
	switch m.state {
	case viewRuns:
		out = m.viewRuns()
	case viewEvents:
		out = m.viewEvents()
	case viewExperiments:
		out = m.viewExperiments()
	case viewExpRuns:
		out = m.viewExpRuns()
	case viewExpAnalysis:
		out = m.viewExpAnalysis()
	case viewTests:
		out = m.viewTests()
	case viewTestRuns:
		out = m.viewTestRuns()
	case viewTestAnalysis:
		out = m.viewTestAnalysis()
	case viewRunAnalysis:
		out = m.viewRunAnalysis()
	default:
		out = m.viewRuns()
	}
	return m.applySearchBar(out)
}

// buildDisplayList rebuilds the flat list of event/child rows used by viewEvents.
// Top-level events are always shown; children are shown only when the parent is expanded.
func (m model) buildDisplayList() []displayItem {
	var list []displayItem
	for i, ev := range m.events {
		list = append(list, displayItem{eventIdx: i, childIdx: -1})
		if m.expanded[ev.ID] {
			for j := range ev.Children {
				list = append(list, displayItem{eventIdx: i, childIdx: j})
			}
		}
	}
	return list
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Runs view
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewRuns() string {
	title := m.tabBar()
	bodyHeight := m.height - 3 // tab bar + header + help

	// ── Layout ─────────────────────────────────────────────────────────────
	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	// Column header
	const durWidth = 11
	const fixedCols = 2 + 4 + 2 + 2 + 6 + 2 + durWidth + 2 + 8 + 2 + 5
	nameWidth := lw - fixedCols
	if nameWidth < 10 {
		nameWidth = 10
	}
	header := styleHeader.Render(fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5s",
		"st", nameWidth, "test name", "age", durWidth, "duration", "cost", "evt"))
	bodyHeight-- // header takes one line

	visible := m.visibleRunRows() - 1 // -1 for header

	// ── Left panel: run list ────────────────────────────────────────────────
	visibleRuns := m.filteredRuns()
	var leftRows []string
	if len(visibleRuns) == 0 {
		leftRows = append(leftRows, styleNormal.Render("  (no runs in this window)"))
	}
	end := m.runOffset + visible
	if end > len(visibleRuns) {
		end = len(visibleRuns)
	}
	for i := m.runOffset; i < end; i++ {
		r := visibleRuns[i]
		age := formatAgePlain(r.StartedAt)
		dur := formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame)
		for utf8.RuneCountInString(dur) < durWidth {
			dur += " "
		}
		name := truncate(r.TestName, nameWidth)
		statusStyled := statusLabel(r)
		statusPlain := passLabel(r)
		var renderedLine string
		if i == m.runCursor {
			badge := "  " + statusPlain
			rest := fmt.Sprintf("  %-*s  %6s  %-*s  %8s  %5d",
				nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleSelected.Render(badge + rest)
		} else {
			coloredLine := fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5d",
				statusStyled, nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleNormal.Render(coloredLine)
		}
		leftRows = append(leftRows, renderedLine)
	}
	leftBody := padBody(leftRows, visible)

	// ── Right drawer: run summary ────────────────────────────────────────────
	var drawerRows []string
	if len(visibleRuns) == 0 || m.runCursor >= len(visibleRuns) {
		drawerRows = append(drawerRows, styleKind.Render("  (no run selected)"))
	} else {
		r := visibleRuns[m.runCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("id", fmt.Sprintf("%d", r.ID))
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("status:")+" "+statusLabel(r))
		if r.Reason != nil {
			add("reason", *r.Reason)
		}
		add("started", r.StartedAt.Format("2006-01-02 15:04:05"))
		if r.FinishedAt != nil {
			add("finished", r.FinishedAt.Format("15:04:05"))
			add("duration", formatDurationPlain(r.StartedAt, r.FinishedAt))
		} else {
			add("duration", formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame))
		}
		add("events", fmt.Sprintf("%d", r.EventCount))
		if r.Runner != nil {
			add("runner", *r.Runner)
		}
		if r.EnvName != nil {
			add("env", *r.EnvName)
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("test:"))
		for _, chunk := range wrapText(r.TestName, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		if r.Description != nil {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("description:"))
			for _, chunk := range wrapText(r.Description.Summary, dw-4) {
				drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
			}
		}
		if r.TokenSummary != nil && (r.TokenSummary.InputTokens > 0 || r.TokenSummary.OutputTokens > 0) {
			drawerRows = append(drawerRows, "")
			tokLine := fmt.Sprintf("%s in / %s out",
				formatTokenCount(r.TokenSummary.InputTokens),
				formatTokenCount(r.TokenSummary.OutputTokens))
			add("tokens", tokLine)
			if r.TokenSummary.CostUSD > 0 {
				add("cost", fmt.Sprintf("$%.6f", r.TokenSummary.CostUSD))
			} else {
				add("cost", "—")
			}
		}
		// ── Analysis history ────────────────────────────────────────────────
		traces, _ := m.db.ListTracesForRun(r.ID)
		if len(traces) > 0 {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(fmt.Sprintf("analysis: (%d)", len(traces))))
			stylePrioHigh := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
			stylePrioMedium := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
			stylePrioLow := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
			// Show cached suggestions from the latest analysis.
			suggKey := fmt.Sprintf("run:%d", r.ID)
			if suggs, err := m.db.ListSuggestions(suggKey); err == nil && len(suggs) > 0 {
				for _, s := range suggs {
					var badge lipgloss.Style
					var label string
					switch strings.ToLower(s.Priority) {
					case "high":
						badge, label = stylePrioHigh, "[H]"
					case "low":
						badge, label = stylePrioLow, "[L]"
					default:
						badge, label = stylePrioMedium, "[M]"
					}
					titleW := dw - 8
					if titleW < 5 {
						titleW = 5
					}
					drawerRows = append(drawerRows, "  "+badge.Render(label)+" "+styleDetailVal.Render(truncate(s.Title, titleW)))
				}
			}
			// List trace timestamps.
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("trace history:"))
			for i, tr := range traces {
				age := formatAgePlain(tr.StartedAt)
				durStr := "..."
				if tr.FinishedAt != nil {
					durStr = formatDurationPlain(tr.StartedAt, tr.FinishedAt)
				}
				marker := " "
				if i == 0 {
					marker = "*"
				}
				drawerRows = append(drawerRows, "  "+styleKind.Render(fmt.Sprintf("  %s #%d  %s ago  %s", marker, tr.ID, age, durStr)))
			}
			drawerRows = append(drawerRows, "  "+styleKind.Render("  press 'a' for details"))
		} else {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleKind.Render("no analysis yet — press 'a'"))
		}

		// Launch status (shown after pressing 't').
		drawerRows = append(drawerRows, "")
		if m.testLaunching {
			spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
			drawerRows = append(drawerRows, "  "+styleRun.Render("launching… "+spin))
		} else if m.testLaunchSuccess && m.activePID != 0 {
			drawerRows = append(drawerRows, "  "+stylePass.Render(fmt.Sprintf("launched!  pid %d", m.activePID)))
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
		} else if m.activePID != 0 {
			drawerRows = append(drawerRows, "  "+styleRun.Render(fmt.Sprintf("running  pid %d", m.activePID)))
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
		} else if m.testLaunchErr != "" {
			for _, chunk := range wrapText("err: "+m.testLaunchErr, dw-4) {
				drawerRows = append(drawerRows, "  "+styleFail.Render(chunk))
			}
		} else {
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 't' to run"))
		}
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine left + separator + right, line by line ──────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < bodyHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}
	// header line: left header + separator + blank drawer header
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	for i := 0; i < bodyHeight; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ────────────────────────────────────────────────────────────
	helpLeft := "  ↑↓/jk navigate · Enter open · a analyze · t run · x kill · f filter · / search · r refresh · Tab switch tab · q quit"
	if m.filterTest != "" {
		helpLeft += "  [filter: " + truncate(m.filterTest, 30) + "]"
	}
	helpRight := ""
	if !m.lastRefresh.IsZero() {
		helpRight = "updated " + m.lastRefresh.Format("15:04:05") + "  "
	}
	gap := m.width - utf8.RuneCountInString(stripANSI(helpLeft)) - utf8.RuneCountInString(helpRight)
	if gap < 1 {
		gap = 1
	}
	help := styleHelp.Render(helpLeft + strings.Repeat(" ", gap) + helpRight)
	return strings.Join([]string{title, headerLine, body, help}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Formatting helpers (plain-text, used by both CLI and TUI)
// ─────────────────────────────────────────────────────────────────────────────

func passLabel(r runlog.RunRow) string {
	if r.Skipped {
		return plainSkip
	}
	if r.Passed == nil {
		if r.FinishedAt != nil {
			return plainAbort
		}
		return plainRuns
	}
	if *r.Passed {
		return plainPass
	}
	return plainFail
}

func formatAgePlain(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

func formatDurationPlain(start time.Time, end *time.Time) string {
	if end == nil {
		d := time.Since(start)
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	d := end.Sub(start)
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func formatDurationTUI(start time.Time, end *time.Time, frame int) string {
	if end == nil {
		spin := spinnerFrames[frame%len(spinnerFrames)]
		d := time.Since(start)
		return fmt.Sprintf("%.1fs %s", d.Seconds(), spin)
	}
	return formatDurationPlain(start, end)
}

func formatTokenCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// formatCostShort formats a USD cost for compact column display.
// Returns "—" when zero, "$X.XXXX" otherwise (capped at 8 chars wide).
func formatCostShort(usd float64) string {
	if usd <= 0 {
		return "—"
	}
	if usd < 0.01 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// runCost returns the total cost for a run, or 0 if no token summary exists.
func runCost(r runlog.RunRow) float64 {
	if r.TokenSummary == nil {
		return 0
	}
	return r.TokenSummary.CostUSD
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Event list view
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewEvents() string {
	// ── Title bar ──────────────────────────────────────────────────────────
	status := passLabel(m.selectedRun)
	dur := formatDurationTUI(m.selectedRun.StartedAt, m.selectedRun.FinishedAt, m.spinnerFrame)
	evCount := fmt.Sprintf("  %d events", len(m.events))
	title := m.titleBar(fmt.Sprintf(" %s  %s  %s%s",
		status, truncate(m.selectedRun.TestName, m.width-50), dur, evCount))

	// ── Layout ─────────────────────────────────────────────────────────────
	dw := m.drawerWidth()
	// left panel width: total - separator(1) - drawer
	lw := m.width - 1 - dw
	if lw < 20 {
		lw = 20
	}
	visible := m.visibleEventRows()

	// Column header for the left panel — dimmed, aligned with data rows.
	// Row format: "  elapsed(8)  kind(12)  message"
	evHeader := styleHeader.Render(fmt.Sprintf("  %-8s  %-12s  %s", "elapsed", "kind", "message"))
	evHeaderLine := padToWidth(evHeader, lw) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("│") +
		styleHeader.Render(" inspector")
	visible-- // header takes one line

	// ── Left panel: event list ──────────────────────────────────────────────
	filteredDisplay := m.searchFilteredDisplayList()
	var leftRows []string
	if len(filteredDisplay) == 0 {
		leftRows = append(leftRows, styleNormal.Render("  (no events)"))
	}
	end := m.listOffset + visible
	if end > len(filteredDisplay) {
		end = len(filteredDisplay)
	}
	for di := m.listOffset; di < end; di++ {
		item := filteredDisplay[di]
		ev := m.events[item.eventIdx]

		var line string
		if item.childIdx >= 0 {
			// ── Child row (depth 1) ──
			child := ev.Children[item.childIdx]
			elapsed := fmt.Sprintf("%6.1fs", child.ElapsedS)
			msg := truncate(child.Message, lw-28)
			if di == m.listCursor {
				plain := fmt.Sprintf("  · %s  %-10s  %s", elapsed, child.Kind, msg)
				line = styleSelected.Render(plain)
			} else {
				kindStr := kindStyledWithDetails(child.Kind, child.Details)
				line = styleNormal.Render(fmt.Sprintf("  · %s  %s  %s", elapsed, kindStr, msg))
			}
		} else {
			// ── Top-level row (depth 0) ──
			elapsed := fmt.Sprintf("%6.1fs", ev.ElapsedS)
			msg := truncate(ev.Message, lw-32)
			// collapse/expand indicator
			indicator := ""
			if len(ev.Children) > 0 {
				if m.expanded[ev.ID] {
					indicator = styleKind.Render(" ▼")
				} else {
					indicator = styleKind.Render(fmt.Sprintf(" ▶ %d", len(ev.Children)))
				}
			}
			evDetails := ""
			if ev.Details != nil {
				evDetails = *ev.Details
			}
			if di == m.listCursor {
				expandPlain := ""
				if len(ev.Children) > 0 {
					if m.expanded[ev.ID] {
						expandPlain = " ▼"
					} else {
						expandPlain = fmt.Sprintf(" ▶ %d", len(ev.Children))
					}
				}
				plain := fmt.Sprintf("  %s  %-12s  %s%s", elapsed, ev.Kind, msg, expandPlain)
				line = styleSelected.Render(plain)
			} else {
				kindStr := kindStyledWithDetails(ev.Kind, evDetails)
				line = styleNormal.Render(fmt.Sprintf("  %s  %s  %s%s", elapsed, kindStr, msg, indicator))
			}
		}
		leftRows = append(leftRows, line)
	}
	leftBody := padBody(leftRows, visible)

	// ── Right drawer: inspector ─────────────────────────────────────────────
	drawerVisible := m.visibleDrawerLines()
	drawerStart := m.drawerOffset
	drawerEnd := drawerStart + drawerVisible
	if drawerEnd > len(m.drawerLines) {
		drawerEnd = len(m.drawerLines)
	}
	var drawerRows []string
	if len(m.drawerLines) == 0 {
		drawerRows = append(drawerRows, styleKind.Render("  (select an item)"))
	} else {
		drawerRows = append(drawerRows, m.drawerLines[drawerStart:drawerEnd]...)
	}
	drawerBody := padBody(drawerRows, drawerVisible)

	// ── Combine left + separator + right, line by line ──────────────────────
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	// ensure both slices are the same length
	for len(leftLines) < visible {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < visible {
		rightLines = append(rightLines, "")
	}

	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	var combined []string
	for i := 0; i < visible; i++ {
		lLine := leftLines[i]
		rLine := rightLines[i]
		lPadded := padToWidth(lLine, lw)
		rPadded := padToWidth(rLine, dw)
		sep := styleDrawerSep.Render("│")
		combined = append(combined, lPadded+sep+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ────────────────────────────────────────────────────────────
	scrollInfo := ""
	if len(filteredDisplay) > visible {
		pct := int(math.Round(float64(m.listCursor+1) / float64(len(filteredDisplay)) * 100))
		scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.listCursor+1, len(filteredDisplay), pct)
	}
	help := styleHelp.Render("  ↑↓/jk navigate · Enter/Space expand · [/] scroll inspector · a analyze · / search · Esc back · q quit" + scrollInfo)
	return strings.Join([]string{title, evHeaderLine, body, help}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Experiments list view
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewExperiments() string {
	title := m.tabBar()
	bodyHeight := m.height - 3 // tab bar + header + help

	// ── Layout ─────────────────────────────────────────────────────────────
	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	// Column header
	const passWidth = 6
	const countWidth = 5
	const ageWidth = 6
	const costWidth = 8
	// fixed overhead: "  "(2) + count(5) + "  "(2) + pass(6) + "  "(2) + age(6) + "  "(2) + cost(8) + "  "(2) = 35
	const fixedCols = 2 + countWidth + 2 + passWidth + 2 + ageWidth + 2 + costWidth + 2
	nameWidth := lw - fixedCols
	if nameWidth < 10 {
		nameWidth = 10
	}
	header := styleHeader.Render(fmt.Sprintf("  %-*s  %5s  %6s  %8s  %6s",
		nameWidth, "experiment", "runs", "pass%", "cost", "last"))
	bodyHeight-- // header takes one line

	// ── Left panel: experiment list ─────────────────────────────────────────
	filteredExps := m.searchFilteredExperiments()
	var leftRows []string
	if len(filteredExps) == 0 {
		if m.searchQuery != "" {
			leftRows = append(leftRows, styleNormal.Render("  (no experiments match search)"))
		} else {
			leftRows = append(leftRows, styleNormal.Render("  (no experiments found — run tests with EXPERIMENT=name)"))
		}
	}
	for i, exp := range filteredExps {
		age := formatAgePlain(exp.LastRunAt)
		passRate := "  —   "
		effectiveCount := exp.RunCount - exp.SkipCount
		if effectiveCount > 0 {
			pct := int(float64(exp.PassCount) / float64(effectiveCount) * 100)
			passRate = fmt.Sprintf("%5d%%", pct)
		}
		name := truncate(exp.Name, nameWidth)
		if i == m.expCursor {
			plain := fmt.Sprintf("  %-*s  %5d  %s  %8s  %6s",
				nameWidth, name, exp.RunCount, passRate, formatCostShort(exp.TotalCostUSD), age)
			leftRows = append(leftRows, styleSelected.Render(plain))
		} else {
			// colour pass rate
			var passStyled string
			if effectiveCount > 0 {
				pct := int(float64(exp.PassCount) / float64(effectiveCount) * 100)
				switch {
				case pct == 100:
					passStyled = stylePass.Render(passRate)
				case pct == 0:
					passStyled = styleFail.Render(passRate)
				default:
					passStyled = styleRun.Render(passRate)
				}
			} else {
				passStyled = styleKind.Render(passRate)
			}
			line := fmt.Sprintf("  %-*s  %5d  %s  %8s  %6s",
				nameWidth, name, exp.RunCount, passStyled, formatCostShort(exp.TotalCostUSD), age)
			leftRows = append(leftRows, styleNormal.Render(line))
		}
		if len(leftRows) >= bodyHeight {
			break
		}
	}
	leftBody := padBody(leftRows, bodyHeight)

	// ── Right drawer: experiment summary ────────────────────────────────────
	var drawerRows []string
	if len(filteredExps) == 0 || m.expCursor >= len(filteredExps) {
		drawerRows = append(drawerRows, styleKind.Render("  (no experiment selected)"))
	} else {
		exp := filteredExps[m.expCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("name", exp.Name)
		add("runs", fmt.Sprintf("%d", exp.RunCount))
		passed := fmt.Sprintf("%d", exp.PassCount)
		failed := fmt.Sprintf("%d", exp.FailCount)
		skipped := fmt.Sprintf("%d", exp.SkipCount)
		inFlight := exp.RunCount - exp.PassCount - exp.FailCount - exp.SkipCount
		if inFlight > 0 {
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("passed:")+" "+stylePass.Render(passed))
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("failed:")+" "+styleFail.Render(failed))
			if exp.SkipCount > 0 {
				drawerRows = append(drawerRows, "  "+styleDetailKey.Render("skipped:")+" "+styleSkip.Render(skipped))
			}
			add("running", fmt.Sprintf("%d", inFlight))
		} else {
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("passed:")+" "+stylePass.Render(passed))
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("failed:")+" "+styleFail.Render(failed))
			if exp.SkipCount > 0 {
				drawerRows = append(drawerRows, "  "+styleDetailKey.Render("skipped:")+" "+styleSkip.Render(skipped))
			}
		}
		add("last run", exp.LastRunAt.Format("2006-01-02 15:04"))
		if len(exp.Tags) > 0 {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("tags:"))
			for _, t := range exp.Tags {
				drawerRows = append(drawerRows, "    "+styleDetailVal.Render(truncate(t, dw-6)))
			}
		}
		// ── Cached analysis suggestions ─────────────────────────────────────
		if m.drawerSuggestionsExp == exp.Name && len(m.drawerSuggestions) > 0 {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("analysis:"))
			stylePrioHigh := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
			stylePrioMedium := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
			stylePrioLow := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
			for _, s := range m.drawerSuggestions {
				var badge lipgloss.Style
				var label string
				switch strings.ToLower(s.Priority) {
				case "high":
					badge, label = stylePrioHigh, "[H]"
				case "low":
					badge, label = stylePrioLow, "[L]"
				default:
					badge, label = stylePrioMedium, "[M]"
				}
				titleW := dw - 8
				if titleW < 5 {
					titleW = 5
				}
				drawerRows = append(drawerRows, "  "+badge.Render(label)+" "+styleDetailVal.Render(truncate(s.Title, titleW)))
			}
			drawerRows = append(drawerRows, "  "+styleKind.Render("  press 'a' for details"))
		} else if m.drawerSuggestionsExp == exp.Name {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleKind.Render("no analysis yet — press 'a'"))
		}
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < bodyHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	for i := 0; i < bodyHeight; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	helpLeft := "  ↑↓/jk navigate · Enter open · a analyze · / search · r refresh · Tab switch tab · q quit"
	helpRight := ""
	if !m.lastRefresh.IsZero() {
		helpRight = "updated " + m.lastRefresh.Format("15:04:05") + "  "
	}
	gap := m.width - utf8.RuneCountInString(stripANSI(helpLeft)) - utf8.RuneCountInString(helpRight)
	if gap < 1 {
		gap = 1
	}
	help := styleHelp.Render(helpLeft + strings.Repeat(" ", gap) + helpRight)
	return strings.Join([]string{title, headerLine, body, help}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Experiment runs view (drill-in from experiments list)
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewExpRuns() string {
	title := m.titleBar(" Experiment: " + m.selectedExp.Name)
	bodyHeight := m.height - 3 // title + header + help

	// ── Layout ─────────────────────────────────────────────────────────────
	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	const durWidth = 11
	const fixedCols = 2 + 4 + 2 + 2 + 6 + 2 + durWidth + 2 + 8 + 2 + 5
	nameWidth := lw - fixedCols
	if nameWidth < 10 {
		nameWidth = 10
	}

	header := styleHeader.Render(fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5s",
		"st", nameWidth, "test name", "age", durWidth, "duration", "cost", "evt"))
	bodyHeight-- // header takes one line

	visible := m.visibleExpRunRows() - 1 // -1 for header

	// ── Left panel: run list ────────────────────────────────────────────────
	filteredExpRuns := m.searchFilteredExpRuns()
	var leftRows []string
	if len(filteredExpRuns) == 0 {
		leftRows = append(leftRows, styleNormal.Render("  (no runs)"))
	}
	end := m.expRunOffset + visible
	if end > len(filteredExpRuns) {
		end = len(filteredExpRuns)
	}
	for i := m.expRunOffset; i < end; i++ {
		r := filteredExpRuns[i]
		age := formatAgePlain(r.StartedAt)
		dur := formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame)
		for utf8.RuneCountInString(dur) < durWidth {
			dur += " "
		}
		name := truncate(r.TestName, nameWidth)
		statusStyled := statusLabel(r)
		statusPlain := passLabel(r)
		var renderedLine string
		if i == m.expRunCursor {
			badge := "  " + statusPlain
			rest := fmt.Sprintf("  %-*s  %6s  %-*s  %8s  %5d",
				nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleSelected.Render(badge + rest)
		} else {
			coloredLine := fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5d",
				statusStyled, nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleNormal.Render(coloredLine)
		}
		leftRows = append(leftRows, renderedLine)
	}
	leftBody := padBody(leftRows, visible)

	// ── Right drawer: run summary ───────────────────────────────────────────
	var drawerRows []string
	if len(filteredExpRuns) == 0 || m.expRunCursor >= len(filteredExpRuns) {
		drawerRows = append(drawerRows, styleKind.Render("  (no run selected)"))
	} else {
		r := filteredExpRuns[m.expRunCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("id", fmt.Sprintf("%d", r.ID))
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("status:")+" "+statusLabel(r))
		if r.Reason != nil {
			add("reason", *r.Reason)
		}
		add("started", r.StartedAt.Format("2006-01-02 15:04:05"))
		if r.FinishedAt != nil {
			add("finished", r.FinishedAt.Format("15:04:05"))
			add("duration", formatDurationPlain(r.StartedAt, r.FinishedAt))
		} else {
			add("duration", formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame))
		}
		add("events", fmt.Sprintf("%d", r.EventCount))
		if r.Runner != nil {
			add("runner", *r.Runner)
		}
		if r.EnvName != nil {
			add("env", *r.EnvName)
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("test:"))
		for _, chunk := range wrapText(r.TestName, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		if r.Description != nil {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("description:"))
			for _, chunk := range wrapText(r.Description.Summary, dw-4) {
				drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
			}
		}
		if len(r.Tags) > 0 {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("tags:"))
			for _, t := range r.Tags {
				drawerRows = append(drawerRows, "    "+styleDetailVal.Render(truncate(t, dw-6)))
			}
		}
		if r.TokenSummary != nil && (r.TokenSummary.InputTokens > 0 || r.TokenSummary.OutputTokens > 0) {
			drawerRows = append(drawerRows, "")
			tokLine := fmt.Sprintf("%s in / %s out",
				formatTokenCount(r.TokenSummary.InputTokens),
				formatTokenCount(r.TokenSummary.OutputTokens))
			add("tokens", tokLine)
			if r.TokenSummary.CostUSD > 0 {
				add("cost", fmt.Sprintf("$%.6f", r.TokenSummary.CostUSD))
			} else {
				add("cost", "—")
			}
		}
	}
	drawerBody := padBody(drawerRows, visible)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < visible {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < visible {
		rightLines = append(rightLines, "")
	}
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	for i := 0; i < visible; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	scrollInfo := ""
	if len(filteredExpRuns) > visible {
		pct := int(math.Round(float64(m.expRunCursor+1) / float64(len(filteredExpRuns)) * 100))
		scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.expRunCursor+1, len(filteredExpRuns), pct)
	}
	help := styleHelp.Render("  ↑↓/jk navigate · Enter open events · a analyze · / search · Esc back · r refresh · q quit" + scrollInfo)
	return strings.Join([]string{title, headerLine, body, help}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Experiment analysis view (LLM-generated suggestions)
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewExpAnalysis() string {
	title := m.titleBar(" Analysis: " + m.selectedExp.Name)
	bodyHeight := m.visibleSuggRows()

	// ── Layout ─────────────────────────────────────────────────────────────
	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	// Priority badge styles
	stylePrioHigh := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	stylePrioMedium := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	stylePrioLow := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)

	prioStyle := func(p string) lipgloss.Style {
		switch strings.ToLower(p) {
		case "high":
			return stylePrioHigh
		case "low":
			return stylePrioLow
		default:
			return stylePrioMedium
		}
	}
	prioLabel := func(p string) string {
		switch strings.ToLower(p) {
		case "high":
			return "[H]"
		case "low":
			return "[L]"
		default:
			return "[M]"
		}
	}

	// ── Left panel: suggestion list ─────────────────────────────────────────
	filteredSuggs := m.searchFilteredSuggestions()
	var leftRows []string

	if m.analyzingExp {
		spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		leftRows = append(leftRows, styleRun.Render("  Analyzing… "+spin))
	} else if len(filteredSuggs) == 0 {
		if m.analyzeErr != "" {
			leftRows = append(leftRows, styleFail.Render("  Error: "+truncate(m.analyzeErr, lw-10)))
			leftRows = append(leftRows, styleKind.Render("  Press 'A' to retry analysis."))
		} else {
			leftRows = append(leftRows, styleKind.Render("  (no suggestions — press 'a' to analyze)"))
		}
	} else {
		end := m.suggOffset + bodyHeight
		if end > len(filteredSuggs) {
			end = len(filteredSuggs)
		}
		for i := m.suggOffset; i < end; i++ {
			s := filteredSuggs[i]
			badge := prioLabel(s.Priority)
			// Available width for title: lw - 2 (indent) - 4 (badge) - 1 (space)
			titleWidth := lw - 7
			if titleWidth < 5 {
				titleWidth = 5
			}
			t := truncate(s.Title, titleWidth)
			if i == m.suggCursor {
				plain := fmt.Sprintf("  %s %-*s", badge, titleWidth, t)
				leftRows = append(leftRows, styleSelected.Render(plain))
			} else {
				badgeStyled := prioStyle(s.Priority).Render(badge)
				leftRows = append(leftRows, styleNormal.Render("  ")+badgeStyled+styleNormal.Render(" "+t))
			}
		}
	}
	leftBody := padBody(leftRows, bodyHeight)

	// ── Right drawer: suggestion detail ────────────────────────────────────
	var drawerRows []string
	if !m.analyzingExp && len(filteredSuggs) > 0 && m.suggCursor < len(filteredSuggs) {
		s := filteredSuggs[m.suggCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("priority", strings.ToUpper(s.Priority))
		add("category", s.Category)
		add("generated", s.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC"))
		if len(s.RunIDs) > 0 {
			ids := make([]string, len(s.RunIDs))
			for i, id := range s.RunIDs {
				ids[i] = fmt.Sprintf("%d", id)
			}
			add("run IDs", strings.Join(ids, ", "))
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("title:"))
		for _, chunk := range wrapText(s.Title, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("body:"))
		for _, chunk := range wrapText(s.Body, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
	} else if m.analyzingExp {
		drawerRows = append(drawerRows, styleKind.Render("  Running LLM analysis…"))
	} else {
		drawerRows = append(drawerRows, styleKind.Render("  (no suggestion selected)"))
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < bodyHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}

	var combined []string
	for i := 0; i < bodyHeight; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	scrollInfo := ""
	if len(filteredSuggs) > 0 && len(filteredSuggs) > bodyHeight {
		pct := int(math.Round(float64(m.suggCursor+1) / float64(len(filteredSuggs)) * 100))
		scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.suggCursor+1, len(filteredSuggs), pct)
	}
	helpLeft := "  ↑↓/jk navigate · a re-analyze · / search · Esc back · q quit" + scrollInfo
	if m.analyzeErr != "" {
		helpLeft = styleHelp.Render(helpLeft) + " " + styleFail.Render("err: "+truncate(m.analyzeErr, 40))
		return strings.Join([]string{title, body, helpLeft}, "\n")
	}
	help := styleHelp.Render(helpLeft)
	return strings.Join([]string{title, body, help}, "\n")
}

// buildDetailLines renders lines for the detail view.
//
//   - child != nil  → show that specific child's detail (kind, elapsed, message, details JSON).
//   - child == nil && ev.Kind == "gantt" → render the Gantt chart scaled to width.
//   - otherwise → metadata + pretty-printed details JSON.
func buildDetailLines(ev runlog.EventRow, child *runlog.ChildEvent, width int) []string {
	var lines []string
	// maxVal: drawer width minus "  key: " prefix (2 + up to 12 + 2 = ~16 chars conservative)
	maxVal := width - 18
	if maxVal < 10 {
		maxVal = 10
	}
	add := func(key, val string) {
		val = truncate(val, maxVal)
		lines = append(lines, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
	}

	if child != nil {
		// Showing one child from a section.
		add("kind", child.Kind)
		add("elapsed", fmt.Sprintf("%.3fs", child.ElapsedS))
		if child.Message != "" {
			add("message", child.Message)
		}
		if child.Details != "" {
			if child.Kind == "gantt" {
				var gd runlog.GanttData
				if json.Unmarshal([]byte(child.Details), &gd) == nil {
					lines = append(lines, "")
					lines = append(lines, renderGantt(gd, width)...)
					return lines
				}
			}
			if child.Kind == "cli" {
				lines = append(lines, renderCLIDetail(child.Details, width)...)
				return lines
			}
			if child.Kind == "skill" {
				lines = append(lines, renderSkillDetail(child.Details, width)...)
				return lines
			}
			lines = append(lines, "")
			lines = append(lines, "  "+styleDetailKey.Render("details:"))
			for _, l := range prettyJSON(child.Details) {
				lines = append(lines, "    "+styleDetailVal.Render(truncate(l, width-6)))
			}
		}
		return lines
	}

	// Top-level event detail.
	add("kind", ev.Kind)
	add("seq", fmt.Sprintf("%d", ev.Seq))
	add("elapsed", fmt.Sprintf("%.3fs", ev.ElapsedS))
	add("occurred_at", ev.OccurredAt.Format(time.RFC3339))
	if ev.Message != "" {
		add("message", ev.Message)
	}

	if ev.Kind == "gantt" && ev.Details != nil && *ev.Details != "" {
		var gd runlog.GanttData
		if json.Unmarshal([]byte(*ev.Details), &gd) == nil {
			lines = append(lines, "")
			lines = append(lines, renderGantt(gd, width)...)
			return lines
		}
	}

	if ev.Kind == "cli" && ev.Details != nil && *ev.Details != "" {
		lines = append(lines, renderCLIDetail(*ev.Details, width)...)
		return lines
	}

	if ev.Kind == "skill" && ev.Details != nil && *ev.Details != "" {
		lines = append(lines, renderSkillDetail(*ev.Details, width)...)
		return lines
	}

	if ev.Kind == "credentials" && ev.Details != nil && *ev.Details != "" {
		lines = append(lines, renderCredentialsDetail(*ev.Details, width)...)
		return lines
	}

	if ev.Kind == "metric" && ev.Details != nil && *ev.Details != "" {
		lines = append(lines, renderMetricDetail(*ev.Details, width)...)
		return lines
	}

	// If this is a group/section event, list children.
	if len(ev.Children) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+styleDetailKey.Render(fmt.Sprintf("%d children", len(ev.Children))))
	} else if ev.Details != nil && *ev.Details != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleDetailKey.Render("details:"))
		for _, l := range prettyJSON(*ev.Details) {
			lines = append(lines, "    "+styleDetailVal.Render(truncate(l, width-6)))
		}
	}
	return lines
}

// renderCLIDetail renders a "cli" event details JSON blob ({"invocation":…,"output":…})
// as styled lines with the command and its output.
func renderCLIDetail(detailsJSON string, width int) []string {
	var d struct {
		Invocation string `json:"invocation"`
		Output     string `json:"output"`
		ErrorMsg   string `json:"error_msg"`
		ExitCode   int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		// Fallback: just pretty-print the raw JSON.
		var lines []string
		for _, l := range prettyJSON(detailsJSON) {
			lines = append(lines, "  "+styleDetailVal.Render(truncate(l, width-4)))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+styleDetailKey.Render("input:"))
	lines = append(lines, "    "+styleDetailVal.Render(truncate("$ "+d.Invocation, width-6)))
	if d.ExitCode != 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+styleCLIFail.Render(fmt.Sprintf("exit code: %d", d.ExitCode)))
	}
	if d.ErrorMsg != "" {
		lines = append(lines, "  "+styleCLIFail.Render("error: "+truncate(d.ErrorMsg, width-10)))
	}
	if d.Output != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleDetailKey.Render("output:"))
		for _, l := range strings.Split(strings.TrimRight(d.Output, "\n"), "\n") {
			for _, chunk := range wrapText(l, width-6) {
				lines = append(lines, "    "+styleDetailVal.Render(chunk))
			}
		}
	}
	return lines
}

// renderSkillDetail renders a "skill" event details JSON blob ({"name":…,"desc":…,"name_matches_dir":…})
// as labelled fields with the description word-wrapped.
func renderSkillDetail(detailsJSON string, width int) []string {
	var d struct {
		Name           string `json:"name"`
		Desc           string `json:"desc"`
		NameMatchesDir *bool  `json:"name_matches_dir"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		var lines []string
		for _, l := range prettyJSON(detailsJSON) {
			lines = append(lines, "  "+styleDetailVal.Render(truncate(l, width-4)))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+styleDetailKey.Render("name:")+" "+styleDetailVal.Render(d.Name))
	if d.NameMatchesDir != nil {
		v := "yes"
		if !*d.NameMatchesDir {
			v = "no — mismatch with directory name"
		}
		lines = append(lines, "  "+styleDetailKey.Render("name matches dir:")+" "+styleDetailVal.Render(v))
	}
	if d.Desc != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+styleDetailKey.Render("description:"))
		for _, chunk := range wrapText(d.Desc, width-6) {
			lines = append(lines, "    "+styleDetailVal.Render(chunk))
		}
	}
	return lines
}

// renderCredentialsDetail renders a "credentials" event details JSON blob as
// labelled fields, showing path, token presence, and server URL.
func renderCredentialsDetail(detailsJSON string, width int) []string {
	var d struct {
		Path         string   `json:"path"`
		HasToken     bool     `json:"has_token"`
		HasServerURL bool     `json:"has_server_url"`
		ServerURL    string   `json:"server_url"`
		Keys         []string `json:"keys"`
		Error        string   `json:"error"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		var lines []string
		for _, l := range prettyJSON(detailsJSON) {
			lines = append(lines, "  "+styleDetailVal.Render(truncate(l, width-4)))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+styleDetailKey.Render("path:")+" "+styleDetailVal.Render(truncate(d.Path, width-12)))
	if d.Error != "" {
		lines = append(lines, "  "+styleDetailKey.Render("error:")+" "+styleDetailVal.Render(truncate(d.Error, width-12)))
		return lines
	}
	tokenVal := "present"
	if !d.HasToken {
		tokenVal = "MISSING"
	}
	lines = append(lines, "  "+styleDetailKey.Render("token:")+" "+styleDetailVal.Render(tokenVal))
	if d.ServerURL != "" {
		lines = append(lines, "  "+styleDetailKey.Render("server_url:")+" "+styleDetailVal.Render(truncate(d.ServerURL, width-18)))
	}
	if len(d.Keys) > 0 {
		lines = append(lines, "  "+styleDetailKey.Render("keys:")+" "+styleDetailVal.Render(strings.Join(d.Keys, ", ")))
	}
	return lines
}

// renderMetricDetail renders a "metric" event details JSON blob as a human-readable summary.
func renderMetricDetail(detailsJSON string, width int) []string {
	var d struct {
		AgentName    string  `json:"agent_name"`
		RunID        string  `json:"run_id"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CostUSD      float64 `json:"cost_usd"`
		DurationMs   int     `json:"duration_ms"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		var lines []string
		for _, l := range prettyJSON(detailsJSON) {
			lines = append(lines, "  "+styleDetailVal.Render(truncate(l, width-4)))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+styleDetailKey.Render("agent:")+" "+styleDetailVal.Render(d.AgentName))
	lines = append(lines, "  "+styleDetailKey.Render("run_id:")+" "+styleDetailVal.Render(d.RunID))
	lines = append(lines, "  "+styleDetailKey.Render("input tokens:")+" "+styleDetailVal.Render(runlog.FormatInt(d.InputTokens)))
	lines = append(lines, "  "+styleDetailKey.Render("output tokens:")+" "+styleDetailVal.Render(runlog.FormatInt(d.OutputTokens)))
	lines = append(lines, "  "+styleDetailKey.Render("cost:")+" "+styleDetailVal.Render(fmt.Sprintf("$%.6f", d.CostUSD)))
	durSec := float64(d.DurationMs) / 1000.0
	lines = append(lines, "  "+styleDetailKey.Render("duration:")+" "+styleDetailVal.Render(fmt.Sprintf("%.2fs", durSec)))
	return lines
}

// renderGantt renders a GanttData as a list of styled lines scaled to terminal width.
func renderGantt(gd runlog.GanttData, width int) []string {
	const reservedCols = 46 // agent name (20) + timing (14) + tokens/cost (12)
	barWidth := width - reservedCols
	if barWidth < 10 {
		barWidth = 10
	}

	var lines []string
	totalS := gd.TotalS
	if totalS <= 0 {
		// Fall back: compute from rows.
		for _, r := range gd.Rows {
			if r.EndS > totalS {
				totalS = r.EndS
			}
		}
	}
	if totalS <= 0 {
		totalS = 1
	}

	for _, r := range gd.Rows {
		startFrac := r.StartS / totalS
		endFrac := r.EndS / totalS
		startCol := int(math.Round(startFrac * float64(barWidth)))
		endCol := int(math.Round(endFrac * float64(barWidth)))
		if endCol <= startCol {
			endCol = startCol + 1
		}
		if endCol > barWidth {
			endCol = barWidth
		}

		bar := strings.Repeat(" ", startCol) +
			styleMetric.Render(strings.Repeat("█", endCol-startCol)) +
			strings.Repeat(" ", barWidth-endCol)

		name := truncate(r.AgentName, 18)
		timing := fmt.Sprintf("%.1fs-%.1fs", r.StartS, r.EndS)
		tokens := ""
		if r.InputTokens > 0 || r.OutputTokens > 0 {
			tokens = fmt.Sprintf(" %d/%d tok", r.InputTokens, r.OutputTokens)
		}
		cost := ""
		if r.EstimatedCostUSD > 0 {
			cost = fmt.Sprintf(" $%.4f", r.EstimatedCostUSD)
		}

		line := fmt.Sprintf("  %-18s [%s] %-10s%s%s",
			name, bar, timing, tokens, cost)
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		lines = append(lines, styleKind.Render("  (no gantt rows)"))
	}
	return lines
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Tests tab views
// ─────────────────────────────────────────────────────────────────────────────

// viewTests renders the Tests tab: a list of all known tests, grouped by category,
// with a right-side drawer showing the last run's description for the selected test.
func (m model) viewTests() string {
	title := m.tabBar()
	bodyHeight := m.height - 3 // tab bar + header + help

	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	// Column header
	header := styleHeader.Render(fmt.Sprintf("  %-12s  %-*s  %6s  %4s",
		"category", lw-30, "test name", "last", "st"))
	bodyHeight-- // header takes one line

	// ── Left panel: test list ────────────────────────────────────────────────
	filteredTests := m.searchFilteredTests()
	var leftRows []string
	visible := bodyHeight
	n := len(filteredTests)
	end := m.testOffset + visible
	if end > n {
		end = n
	}
	prevCat := ""
	for i := m.testOffset; i < end; i++ {
		te := filteredTests[i]
		// Category separator (shown inline as a dim prefix change)
		catLabel := ""
		if te.Category != prevCat {
			catLabel = te.Category
			prevCat = te.Category
		}
		// Find last run for this test
		lastStatus := ""
		lastAge := ""
		for _, r := range m.runs {
			if r.TestName == te.Name {
				lastStatus = statusLabel(r)
				lastAge = formatAgePlain(r.StartedAt)
				break
			}
		}
		nameW := lw - 30
		if nameW < 10 {
			nameW = 10
		}
		name := truncate(te.Name, nameW)
		catW := 12
		cat := fmt.Sprintf("%-*s", catW, truncate(catLabel, catW))
		if i == m.testCursor {
			plain := fmt.Sprintf("  %-*s  %-*s  %6s  %4s",
				catW, truncate(catLabel, catW), nameW, te.Name, lastAge, passLabel2(te.Name, m.runs))
			leftRows = append(leftRows, styleSelected.Render(plain))
		} else {
			_ = cat
			_ = name
			statusPart := lastStatus
			if statusPart == "" {
				statusPart = styleKind.Render("  —— ")
			}
			line := fmt.Sprintf("  %-*s  %-*s  %6s  %s",
				catW, styleKind.Render(cat), nameW, te.Name, lastAge, statusPart)
			leftRows = append(leftRows, styleNormal.Render(line))
		}
	}
	if n == 0 && m.searchQuery != "" {
		leftRows = append(leftRows, styleNormal.Render("  (no tests match search)"))
	}
	leftBody := padBody(leftRows, bodyHeight)

	// ── Right drawer: last run description for selected test ─────────────────
	var drawerRows []string
	if n == 0 || m.testCursor >= n {
		drawerRows = append(drawerRows, styleKind.Render("  (no test selected)"))
	} else {
		te := filteredTests[m.testCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("test:"))
		for _, chunk := range wrapText(te.Name, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		add("category", te.Category)
		drawerRows = append(drawerRows, "")

		// Find most recent run for this test
		var lastRun *runlog.RunRow
		for i := range m.runs {
			if m.runs[i].TestName == te.Name {
				lastRun = &m.runs[i]
				break
			}
		}
		if lastRun != nil {
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("last run:")+" "+statusLabel(*lastRun))
			add("started", lastRun.StartedAt.Format("2006-01-02 15:04:05"))
			if lastRun.FinishedAt != nil {
				add("duration", formatDurationPlain(lastRun.StartedAt, lastRun.FinishedAt))
			}
			if lastRun.Description != nil {
				drawerRows = append(drawerRows, "")
				drawerRows = append(drawerRows, "  "+styleDetailKey.Render("description:"))
				for _, chunk := range wrapText(lastRun.Description.Summary, dw-4) {
					drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
				}
				if len(lastRun.Description.Bullets) > 0 {
					drawerRows = append(drawerRows, "")
					for _, b := range lastRun.Description.Bullets {
						for idx, chunk := range wrapText(b, dw-6) {
							prefix := "  • "
							if idx > 0 {
								prefix = "    "
							}
							drawerRows = append(drawerRows, "  "+styleDetailVal.Render(prefix+chunk))
						}
					}
				}
			}
		} else {
			drawerRows = append(drawerRows, "  "+styleKind.Render("no recent runs found"))
		}

		// Show env and launch hint
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("env: ")+styleDetailVal.Render(m.testEnv))
		if m.testLaunching {
			spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
			drawerRows = append(drawerRows, "  "+styleRun.Render("launching… "+spin))
		} else if m.testLaunchSuccess && m.activePID != 0 {
			drawerRows = append(drawerRows, "  "+stylePass.Render(fmt.Sprintf("launched!  pid %d", m.activePID)))
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
		} else if m.activePID != 0 {
			drawerRows = append(drawerRows, "  "+styleRun.Render(fmt.Sprintf("running  pid %d", m.activePID)))
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
		} else if m.testLaunchErr != "" {
			for _, chunk := range wrapText("err: "+m.testLaunchErr, dw-4) {
				drawerRows = append(drawerRows, "  "+styleFail.Render(chunk))
			}
		} else {
			drawerRows = append(drawerRows, "  "+styleKind.Render("press 't' to run"))
		}
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < bodyHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	for i := 0; i < bodyHeight; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	helpLeft := "  ↑↓/jk navigate · Enter show runs · / search · t run · x kill · r refresh · Tab switch tab · q quit"
	helpRight := ""
	if !m.lastRefresh.IsZero() {
		helpRight = "updated " + m.lastRefresh.Format("15:04:05") + "  "
	}
	gap := m.width - utf8.RuneCountInString(stripANSI(helpLeft)) - utf8.RuneCountInString(helpRight)
	if gap < 1 {
		gap = 1
	}
	help := styleHelp.Render(helpLeft + strings.Repeat(" ", gap) + helpRight)
	return strings.Join([]string{title, headerLine, body, help}, "\n")
}

// passLabel2 returns a plain-text pass label for the most recent run of testName.
func passLabel2(testName string, runs []runlog.RunRow) string {
	for _, r := range runs {
		if r.TestName == testName {
			return passLabel(r)
		}
	}
	return " — "
}

// viewTestRuns renders the drill-in view for a selected test: a list of its
// recent runs with the same layout as viewExpRuns.
func (m model) viewTestRuns() string {
	title := m.titleBar(" Test: " + m.selectedTest.Name)
	bodyHeight := m.height - 3

	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	const durWidth = 11
	const fixedCols = 2 + 4 + 2 + 2 + 6 + 2 + durWidth + 2 + 8 + 2 + 5
	nameWidth := lw - fixedCols
	if nameWidth < 10 {
		nameWidth = 10
	}
	header := styleHeader.Render(fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5s",
		"st", nameWidth, "test name", "age", durWidth, "duration", "cost", "evt"))
	bodyHeight--
	visible := bodyHeight

	runsForTest := m.testFilteredRuns()
	n := len(runsForTest)

	// ── Left panel: run list ─────────────────────────────────────────────────
	var leftRows []string
	if n == 0 {
		leftRows = append(leftRows, styleNormal.Render("  (no runs found for this test)"))
	}
	end := m.testRunOffset + visible
	if end > n {
		end = n
	}
	for i := m.testRunOffset; i < end; i++ {
		r := runsForTest[i]
		age := formatAgePlain(r.StartedAt)
		dur := formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame)
		for utf8.RuneCountInString(dur) < durWidth {
			dur += " "
		}
		name := truncate(r.TestName, nameWidth)
		statusStyled := statusLabel(r)
		statusPlain := passLabel(r)
		var renderedLine string
		if i == m.testRunCursor {
			badge := "  " + statusPlain
			rest := fmt.Sprintf("  %-*s  %6s  %-*s  %8s  %5d",
				nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleSelected.Render(badge + rest)
		} else {
			coloredLine := fmt.Sprintf("  %-4s  %-*s  %6s  %-*s  %8s  %5d",
				statusStyled, nameWidth, name, age, durWidth, dur, formatCostShort(runCost(r)), r.EventCount)
			renderedLine = styleNormal.Render(coloredLine)
		}
		leftRows = append(leftRows, renderedLine)
	}
	leftBody := padBody(leftRows, visible)

	// ── Right drawer: run summary ────────────────────────────────────────────
	var drawerRows []string
	if n == 0 || m.testRunCursor >= n {
		drawerRows = append(drawerRows, styleKind.Render("  (no run selected)"))
	} else {
		r := runsForTest[m.testRunCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("id", fmt.Sprintf("%d", r.ID))
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("status:")+" "+statusLabel(r))
		if r.Reason != nil {
			add("reason", *r.Reason)
		}
		add("started", r.StartedAt.Format("2006-01-02 15:04:05"))
		if r.FinishedAt != nil {
			add("finished", r.FinishedAt.Format("15:04:05"))
			add("duration", formatDurationPlain(r.StartedAt, r.FinishedAt))
		} else {
			add("duration", formatDurationTUI(r.StartedAt, r.FinishedAt, m.spinnerFrame))
		}
		add("events", fmt.Sprintf("%d", r.EventCount))
		if r.Runner != nil {
			add("runner", *r.Runner)
		}
		if r.EnvName != nil {
			add("env", *r.EnvName)
		}
		if r.Description != nil {
			drawerRows = append(drawerRows, "")
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render("description:"))
			for _, chunk := range wrapText(r.Description.Summary, dw-4) {
				drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
			}
		}
		if r.TokenSummary != nil && (r.TokenSummary.InputTokens > 0 || r.TokenSummary.OutputTokens > 0) {
			drawerRows = append(drawerRows, "")
			tokLine := fmt.Sprintf("%s in / %s out",
				formatTokenCount(r.TokenSummary.InputTokens),
				formatTokenCount(r.TokenSummary.OutputTokens))
			add("tokens", tokLine)
			if r.TokenSummary.CostUSD > 0 {
				add("cost", fmt.Sprintf("$%.6f", r.TokenSummary.CostUSD))
			} else {
				add("cost", "—")
			}
		}
	}
	// Launch hint
	drawerRows = append(drawerRows, "")
	if m.testLaunching {
		spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		drawerRows = append(drawerRows, "  "+styleRun.Render("launching… "+spin))
	} else if m.testLaunchSuccess && m.activePID != 0 {
		drawerRows = append(drawerRows, "  "+stylePass.Render(fmt.Sprintf("launched!  pid %d", m.activePID)))
		drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
	} else if m.activePID != 0 {
		drawerRows = append(drawerRows, "  "+styleRun.Render(fmt.Sprintf("running  pid %d", m.activePID)))
		drawerRows = append(drawerRows, "  "+styleKind.Render("press 'x' to kill"))
	} else if m.testLaunchErr != "" {
		for _, chunk := range wrapText("err: "+m.testLaunchErr, dw-4) {
			drawerRows = append(drawerRows, "  "+styleFail.Render(chunk))
		}
	} else {
		drawerRows = append(drawerRows, "  "+styleKind.Render("press 't' to run again"))
	}
	drawerBody := padBody(drawerRows, visible)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < visible {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < visible {
		rightLines = append(rightLines, "")
	}
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	for i := 0; i < visible; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	scrollInfo := ""
	if n > visible {
		pct := int(math.Round(float64(m.testRunCursor+1) / float64(n) * 100))
		scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.testRunCursor+1, n, pct)
	}
	help := styleHelp.Render("  ↑↓/jk navigate · Enter open events · a analyze · t run · x kill · / search · Esc back · r refresh · q quit" + scrollInfo)
	return strings.Join([]string{title, headerLine, body, help}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Commands (DB queries)
// ─────────────────────────────────────────────────────────────────────────────

// loadActiveLauncher queries the DB for the most-recently launched (unfinished)
// process for the currently selected test.  If the PID is still alive it sends
// activeLauncherMsg so the TUI can display the kill option even after a runlog
// restart.  Filtering by test name ensures the running status is only shown for
// the test that was actually launched, not every test in the list.
func (m model) loadActiveLauncher() tea.Cmd {
	db := m.db
	testName := m.selectedTest.Name
	return func() tea.Msg {
		rows, err := db.ListLaunchers(testName)
		if err != nil || len(rows) == 0 {
			return nil
		}
		for _, r := range rows {
			if r.FinishedAt != nil {
				continue // already finished
			}
			// Check if the PID is still alive.
			if !processAlive(r.LauncherPID) {
				// Process is dead; mark it finished in the DB.
				_ = db.FinishLauncher(r.ID, time.Now())
				continue
			}
			return activeLauncherMsg{launcherID: r.ID, pid: r.LauncherPID}
		}
		return nil
	}
}

func (m model) loadRuns() tea.Cmd {
	return func() tea.Msg {
		since := time.Now().Add(-m.since)
		rows, err := m.db.ListRuns(since)
		if err != nil {
			return errMsg{err}
		}
		return runsLoadedMsg{rows}
	}
}

func (m model) loadEvents(runID int64) tea.Cmd {
	return m.loadEventsOpt(runID, false)
}

func (m model) loadEventsReset(runID int64) tea.Cmd {
	return m.loadEventsOpt(runID, true)
}

func (m model) loadEventsOpt(runID int64, reset bool) tea.Cmd {
	return func() tea.Msg {
		rows, err := m.db.ListEvents(runID)
		if err != nil {
			return errMsg{err}
		}
		return eventsLoadedMsg{rows: rows, resetView: reset}
	}
}

func (m model) loadExperiments() tea.Cmd {
	return func() tea.Msg {
		exps, err := m.db.ListExperiments()
		if err != nil {
			return errMsg{err}
		}
		return experimentsLoadedMsg{exps: exps}
	}
}

// loadTests discovers test names from the database, then merges with config
// categories to produce an ordered []testEntry for the Tests tab.
func (m model) loadTests() tea.Cmd {
	db := m.db
	cfg := m.config
	return func() tea.Msg {
		names, err := db.DiscoverTests()
		if err != nil {
			return errMsg{err}
		}

		// Build entries with categories from config.
		// Categorized tests appear first (in config order), uncategorized last.
		seen := make(map[string]bool)
		var entries []testEntry

		// First: add tests in config category order (preserves display order).
		if cfg != nil && len(cfg.Categories) > 0 {
			// Collect category keys in a stable order by iterating the config.
			// Since Go maps are unordered, we sort categories alphabetically.
			var cats []string
			for cat := range cfg.Categories {
				cats = append(cats, cat)
			}
			sort.Strings(cats)
			for _, cat := range cats {
				for _, name := range cfg.Categories[cat] {
					entries = append(entries, testEntry{Name: name, Category: cat})
					seen[name] = true
				}
			}
		}

		// Second: add DB-discovered tests not in any config category.
		for _, name := range names {
			if !seen[name] {
				entries = append(entries, testEntry{Name: name, Category: "Uncategorized"})
			}
		}

		return testsLoadedMsg{entries: entries}
	}
}

// loadDrawerSuggestions fetches cached suggestions for exp from the DB and
// returns them as a drawerSuggestionsLoadedMsg (fast SQLite read, no LLM).
func (m model) loadDrawerSuggestions(exp string) tea.Cmd {
	return func() tea.Msg {
		rows, err := m.db.ListSuggestions(exp)
		if err != nil {
			rows = nil
		}
		return drawerSuggestionsLoadedMsg{exp: exp, suggestions: rows}
	}
}

// loadSuggestions reads cached suggestions from DB (fast, no LLM).
func (m model) loadSuggestions(experiment string) tea.Cmd {
	return func() tea.Msg {
		rows, err := m.db.ListSuggestions(experiment)
		if err != nil {
			return analyzeErrMsg{err}
		}
		return analysisLoadedMsg{suggestions: rows}
	}
}

// runAnalysis runs the LLM analyzer in a goroutine (slow, calls Gemini API).
func (m model) runAnalysis(experiment string) tea.Cmd {
	return func() tea.Msg {
		a := m.analyzer
		if a == nil {
			return analyzeErrMsg{fmt.Errorf("analyzer not initialised (GOOGLE_AI_API_KEY missing?)")}
		}
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		suggestions, err := a.Run(ctx, experiment)
		if err != nil {
			return analyzeErrMsg{err}
		}
		return analysisLoadedMsg{suggestions: suggestions}
	}
}

// triggerAnalysis decides whether to use cached suggestions (force=false) or
// run a fresh LLM analysis (force=true). It initialises m.analyzer on first
// call and sets m.analyzingExp / m.state accordingly.
// Returns the updated model (all mutations applied) alongside an optional Cmd.
func (m model) triggerAnalysis(force bool) (model, tea.Cmd) {
	exp := m.selectedExp.Name
	if exp == "" {
		return m, nil
	}

	if !force {
		// Check cache first — no API key needed for this path.
		rows, err := m.db.ListSuggestions(exp)
		if err == nil && len(rows) > 0 {
			m.suggestions = rows
			m.suggCursor = 0
			m.suggOffset = 0
			m.analyzeErr = ""
			m.state = viewExpAnalysis
			return m, nil
		}
	}

	// No cache (or force): need the LLM — lazily initialise the analyzer.
	if m.analyzer == nil {
		a, err := runlog.NewAnalyzer(m.db)
		if err != nil {
			m.analyzeErr = err.Error()
			m.state = viewExpAnalysis
			return m, nil
		}
		m.analyzer = a
	}

	// Start async LLM run.
	m.analyzingExp = true
	m.analyzeErr = ""
	m.state = viewExpAnalysis
	return m, m.runAnalysis(exp)
}

// triggerTestAnalysis decides whether to use cached suggestions (force=false) or
// run a fresh LLM analysis (force=true) for the currently selected test.
// It initialises m.analyzer on first call and sets m.analyzingTest / m.state.
func (m model) triggerTestAnalysis(force bool) (model, tea.Cmd) {
	testName := m.selectedTest.Name
	if testName == "" {
		return m, nil
	}
	suggKey := "test:" + testName

	if !force {
		// Check cache first — no API key needed for this path.
		rows, err := m.db.ListSuggestions(suggKey)
		if err == nil && len(rows) > 0 {
			m.testSuggestions = rows
			m.testSuggCursor = 0
			m.testSuggOffset = 0
			m.testAnalyzeErr = ""
			m.state = viewTestAnalysis
			return m, nil
		}
	}

	// No cache (or force): need the LLM — lazily initialise the analyzer.
	if m.analyzer == nil {
		a, err := runlog.NewAnalyzer(m.db)
		if err != nil {
			m.testAnalyzeErr = err.Error()
			m.state = viewTestAnalysis
			return m, nil
		}
		m.analyzer = a
	}

	m.analyzingTest = true
	m.testAnalyzeErr = ""
	m.state = viewTestAnalysis
	return m, m.runTestAnalysis(testName)
}

// runTestAnalysis runs the LLM analyzer for a single test in a goroutine.
func (m model) runTestAnalysis(testName string) tea.Cmd {
	return func() tea.Msg {
		a := m.analyzer
		if a == nil {
			return testAnalyzeErrMsg{fmt.Errorf("analyzer not initialised (GOOGLE_AI_API_KEY missing?)")}
		}
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		suggestions, err := a.RunByTestName(ctx, testName)
		if err != nil {
			return testAnalyzeErrMsg{err}
		}
		return testAnalysisLoadedMsg{suggestions: suggestions}
	}
}

// loadTestSuggestions reads cached suggestions for a single test from the DB.
func (m model) loadTestSuggestions(testName string) tea.Cmd {
	suggKey := "test:" + testName
	return func() tea.Msg {
		rows, err := m.db.ListSuggestions(suggKey)
		if err != nil {
			return testAnalyzeErrMsg{err}
		}
		return testAnalysisLoadedMsg{suggestions: rows}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Test analysis view (LLM-generated suggestions for a single test)
// ─────────────────────────────────────────────────────────────────────────────

func (m model) viewTestAnalysis() string {
	title := m.titleBar(" Analysis: " + m.selectedTest.Name)
	bodyHeight := m.height - 3

	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	const prioWidth = 8
	const catWidth = 16
	const fixedCols = 2 + prioWidth + 2 + catWidth + 2
	titleWidth := lw - fixedCols
	if titleWidth < 10 {
		titleWidth = 10
	}
	header := styleHeader.Render(fmt.Sprintf("  %-*s  %-*s  %s", prioWidth, "priority", catWidth, "category", "title"))
	bodyHeight--
	visible := bodyHeight

	// ── Left panel: suggestion list ──────────────────────────────────────────
	filteredTestSuggs := m.searchFilteredTestSuggestions()
	var leftRows []string
	if m.analyzingTest {
		spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		leftRows = append(leftRows, styleRun.Render("  analysing… "+spin))
	} else if m.testAnalyzeErr != "" {
		leftRows = append(leftRows, styleFail.Render("  Error: "+truncate(m.testAnalyzeErr, lw-10)))
	} else if len(filteredTestSuggs) == 0 {
		leftRows = append(leftRows, styleKind.Render("  (no suggestions — press 'a' to analyze)"))
	} else {
		end := m.testSuggOffset + visible
		if end > len(filteredTestSuggs) {
			end = len(filteredTestSuggs)
		}
		for i := m.testSuggOffset; i < end; i++ {
			s := filteredTestSuggs[i]
			prio := strings.ToUpper(s.Priority)
			cat := truncate(s.Category, catWidth)
			suggTitle := truncate(s.Title, titleWidth)
			if i == m.testSuggCursor {
				plain := fmt.Sprintf("  %-*s  %-*s  %s", prioWidth, prio, catWidth, cat, suggTitle)
				leftRows = append(leftRows, styleSelected.Render(plain))
			} else {
				var prioStyled string
				switch strings.ToLower(s.Priority) {
				case "high":
					prioStyled = styleFail.Render(fmt.Sprintf("%-*s", prioWidth, prio))
				case "medium":
					prioStyled = styleRun.Render(fmt.Sprintf("%-*s", prioWidth, prio))
				default:
					prioStyled = styleKind.Render(fmt.Sprintf("%-*s", prioWidth, prio))
				}
				line := fmt.Sprintf("  %s  %-*s  %s", prioStyled, catWidth, cat, suggTitle)
				leftRows = append(leftRows, styleNormal.Render(line))
			}
		}
	}
	leftBody := padBody(leftRows, visible)

	// ── Right drawer: suggestion detail ─────────────────────────────────────
	var drawerRows []string
	if m.testSuggCursor < len(filteredTestSuggs) && !m.analyzingTest {
		s := filteredTestSuggs[m.testSuggCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("priority", strings.ToUpper(s.Priority))
		add("category", s.Category)
		add("generated", s.GeneratedAt.Format("2006-01-02 15:04:05"))
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("title:"))
		for _, chunk := range wrapText(s.Title, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("body:"))
		for _, chunk := range wrapText(s.Body, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
	} else if !m.analyzingTest {
		drawerRows = append(drawerRows, styleKind.Render("  (no suggestion selected)"))
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < visible {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}
	headerLine := padToWidth(header, lw) + styleDrawerSep.Render("│") + strings.Repeat(" ", dw)

	var combined []string
	maxRows := visible
	if bodyHeight > visible {
		maxRows = bodyHeight
	}
	for i := 0; i < maxRows; i++ {
		lLine := ""
		if i < len(leftLines) {
			lLine = leftLines[i]
		}
		rLine := ""
		if i < len(rightLines) {
			rLine = rightLines[i]
		}
		lPadded := padToWidth(lLine, lw)
		rPadded := padToWidth(rLine, dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	scrollInfo := ""
	if len(filteredTestSuggs) > visible {
		pct := 0
		if len(filteredTestSuggs) > 0 {
			pct = int(math.Round(float64(m.testSuggCursor+1) / float64(len(filteredTestSuggs)) * 100))
		}
		scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.testSuggCursor+1, len(filteredTestSuggs), pct)
	}
	helpLeft := "  ↑↓/jk navigate · a re-analyze · / search · Esc back · q quit" + scrollInfo
	if m.testAnalyzeErr != "" {
		helpLeft = styleHelp.Render(helpLeft) + " " + styleFail.Render("err: "+truncate(m.testAnalyzeErr, 40))
	}
	return strings.Join([]string{title, headerLine, body, styleHelp.Render(helpLeft)}, "\n")
}

// visibleTestSuggRows returns the number of suggestion rows visible in viewTestAnalysis.
func (m model) visibleTestSuggRows() int {
	n := m.height - 3 - 1 // title(1) + header(1) + help(1) overhead, -1 for header
	if n < 1 {
		return 1
	}
	return n
}

// ─── Single-run analysis ─────────────────────────────────────────────────────

// triggerRunAnalysis decides whether to use cached suggestions (force=false) or
// run a fresh LLM analysis (force=true) for the currently selected run.
func (m model) triggerRunAnalysis(force bool) (model, tea.Cmd) {
	runID := m.selectedRun.ID
	if runID == 0 {
		return m, nil
	}
	suggKey := fmt.Sprintf("run:%d", runID)

	if !force {
		rows, err := m.db.ListSuggestions(suggKey)
		if err == nil && len(rows) > 0 {
			m.runSuggestions = rows
			m.runSuggCursor = 0
			m.runSuggOffset = 0
			m.runAnalyzeErr = ""

			// Load stored trace so the 't' toggle works for past analyses.
			m.runAnalysisTrace = nil
			m.runTraceOffset = 0
			m.runTraceCursor = 0
			m.runTraceDrawerOffset = 0
			m.showRunTrace = false
			if traceID, terr := m.db.GetLatestTraceForRun(runID); terr == nil && traceID > 0 {
				if events, lerr := m.db.ListAnalyzerTraceEvents(traceID); lerr == nil {
					m.runAnalysisTrace = events
				}
			}

			m.state = viewRunAnalysis
			return m, nil
		}
	}

	// No cache (or force): need the LLM — lazily initialise the analyzer.
	if m.analyzer == nil {
		a, err := runlog.NewAnalyzer(m.db)
		if err != nil {
			m.runAnalyzeErr = err.Error()
			m.state = viewRunAnalysis
			return m, nil
		}
		m.analyzer = a
	}

	m.analyzingRun = true
	m.runAnalyzeErr = ""
	m.runAnalysisTrace = nil
	m.runTraceOffset = 0
	m.runTraceCursor = 0
	m.runTraceDrawerOffset = 0
	m.showRunTrace = false
	m.runTraceCh = make(chan runlog.AnalyzerEvent, 64)
	m.state = viewRunAnalysis
	return m, tea.Batch(m.runRunAnalysis(runID), m.listenRunTrace())
}

// runRunAnalysis runs the LLM analyzer for a single run in a goroutine.
// Trace events are streamed through m.runTraceCh; the channel is closed on
// completion so listenRunTrace can detect the end.
func (m model) runRunAnalysis(runID int64) tea.Cmd {
	return func() tea.Msg {
		a := m.analyzer
		if a == nil {
			return runAnalyzeErrMsg{fmt.Errorf("analyzer not initialised (GOOGLE_AI_API_KEY missing?)")}
		}

		ch := m.runTraceCh
		a.OnEvent = func(ev runlog.AnalyzerEvent) {
			// Non-blocking send — drop if buffer full (unlikely with 64 slots).
			select {
			case ch <- ev:
			default:
			}
		}
		defer func() {
			a.OnEvent = nil
			close(ch)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		suggestions, err := a.RunByRunID(ctx, runID)
		if err != nil {
			return runAnalyzeErrMsg{err}
		}
		return runAnalysisLoadedMsg{suggestions: suggestions}
	}
}

// listenRunTrace reads one trace line from the channel and returns it as a
// message.  When the channel is closed (analysis done), it returns nil so
// bubbletea stops re-subscribing.
func (m model) listenRunTrace() tea.Cmd {
	ch := m.runTraceCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil // channel closed — analysis finished
		}
		return runAnalysisTraceMsg{event: ev}
	}
}

// loadRunSuggestions reads cached suggestions for a single run from the DB.
func (m model) loadRunSuggestions(runID int64) tea.Cmd {
	suggKey := fmt.Sprintf("run:%d", runID)
	return func() tea.Msg {
		rows, err := m.db.ListSuggestions(suggKey)
		if err != nil {
			return runAnalyzeErrMsg{err}
		}
		return runAnalysisLoadedMsg{suggestions: rows}
	}
}

// formatAnalyzerEvent converts an AnalyzerEvent to a single display line for
// the TUI trace view.  Returns "" for events that should be skipped.
func formatAnalyzerEvent(ev runlog.AnalyzerEvent) string {
	prefix := ""
	if ev.Author != "" {
		prefix = "[" + ev.Author + "] "
	}

	switch ev.Kind {
	case runlog.AEThought:
		content := ev.Content
		if len(content) > 120 {
			content = content[:117] + "..."
		}
		// Replace newlines for single-line display.
		content = strings.NewReplacer("\n", " ", "\r", "").Replace(content)
		return prefix + "thought: " + content
	case runlog.AEText:
		content := ev.Content
		if len(content) > 200 {
			content = content[:197] + "..."
		}
		content = strings.NewReplacer("\n", " ", "\r", "").Replace(content)
		return prefix + content
	case runlog.AEToolCall:
		return prefix + ">> " + ev.Content
	case runlog.AEToolResult:
		return prefix + "<< " + ev.Content
	case runlog.AETokenUsage:
		return prefix + ev.Content
	case runlog.AEError:
		return prefix + "ERROR: " + ev.Content
	case runlog.AESystemPrompt:
		lines := strings.Count(ev.Content, "\n") + 1
		return prefix + fmt.Sprintf("SYSTEM PROMPT (%d lines)", lines)
	case runlog.AEUserMessage:
		lines := strings.Count(ev.Content, "\n") + 1
		return prefix + fmt.Sprintf("USER MESSAGE (%d lines)", lines)
	case runlog.AETurnComplete:
		return "" // skip turn markers in TUI — too noisy
	default:
		return ""
	}
}

// appendTraceEventDetail appends detailed info about an AnalyzerEvent to the
// drawer rows.  Used in the right panel when a trace event is selected.
func appendTraceEventDetail(rows []string, ev runlog.AnalyzerEvent, width int) []string {
	add := func(key, val string) {
		rows = append(rows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(truncate(val, width-len(key)-4)))
	}

	add("kind", string(ev.Kind))
	if ev.Author != "" {
		add("author", ev.Author)
	}

	switch ev.Kind {
	case runlog.AEToolCall:
		add("tool", ev.ToolName)
		if ev.ToolArgs != nil {
			argsJSON, _ := json.MarshalIndent(ev.ToolArgs, "", "  ")
			rows = append(rows, "")
			rows = append(rows, "  "+styleDetailKey.Render("args:"))
			for _, chunk := range wrapText(string(argsJSON), width-4) {
				rows = append(rows, "    "+styleDetailVal.Render(chunk))
			}
		}

	case runlog.AEToolResult:
		add("tool", ev.ToolName)
		if ev.ToolResponse != nil {
			respJSON, _ := json.MarshalIndent(ev.ToolResponse, "", "  ")
			rows = append(rows, "")
			rows = append(rows, "  "+styleDetailKey.Render("response:"))
			for _, chunk := range wrapText(string(respJSON), width-4) {
				rows = append(rows, "    "+styleDetailVal.Render(chunk))
			}
		}

	case runlog.AETokenUsage:
		add("prompt", fmt.Sprintf("%d", ev.PromptTokens))
		add("output", fmt.Sprintf("%d", ev.OutputTokens))
		add("thought", fmt.Sprintf("%d", ev.ThoughtTokens))
		add("total", fmt.Sprintf("%d", ev.TotalTokens))

	case runlog.AEError:
		if ev.ErrorCode != "" {
			add("code", ev.ErrorCode)
		}
		if ev.ErrorMessage != "" {
			rows = append(rows, "")
			rows = append(rows, "  "+styleDetailKey.Render("message:"))
			for _, chunk := range wrapText(ev.ErrorMessage, width-4) {
				rows = append(rows, "    "+styleFail.Render(chunk))
			}
		}

	case runlog.AEThought, runlog.AEText, runlog.AESystemPrompt, runlog.AEUserMessage:
		rows = append(rows, "")
		rows = append(rows, "  "+styleDetailKey.Render("content:"))
		for _, chunk := range wrapText(ev.Content, width-4) {
			rows = append(rows, "    "+styleDetailVal.Render(chunk))
		}
	}

	return rows
}

// visibleRunSuggRows returns the number of suggestion rows visible in viewRunAnalysis.
func (m model) visibleRunSuggRows() int {
	// title(1) + help(1) = 2 overhead lines
	n := m.height - 2
	if n < 1 {
		return 1
	}
	return n
}

// viewRunAnalysis renders the LLM analysis view for a single run.
func (m model) viewRunAnalysis() string {
	title := m.titleBar(fmt.Sprintf(" Analysis: run #%d  %s", m.selectedRun.ID, truncate(m.selectedRun.TestName, m.width-40)))
	bodyHeight := m.visibleRunSuggRows()

	dw := m.drawerWidth()
	lw := m.width - 1 - dw
	if lw < 30 {
		lw = 30
	}

	// Priority badge styles (same as viewExpAnalysis).
	stylePrioHigh := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	stylePrioMedium := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	stylePrioLow := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)

	prioStyle := func(p string) lipgloss.Style {
		switch strings.ToLower(p) {
		case "high":
			return stylePrioHigh
		case "low":
			return stylePrioLow
		default:
			return stylePrioMedium
		}
	}
	prioLabel := func(p string) string {
		switch strings.ToLower(p) {
		case "high":
			return "[H]"
		case "low":
			return "[L]"
		default:
			return "[M]"
		}
	}

	// ── Left panel: suggestion list ─────────────────────────────────────────
	var leftRows []string

	if m.analyzingRun || m.showRunTrace {
		// Show trace: live during analysis, or historical when toggled with 't'.
		headerLine := ""
		traceAvail := bodyHeight
		if m.analyzingRun {
			spin := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
			headerLine = styleRun.Render("  Analyzing " + spin)
			traceAvail-- // reserve 1 for spinner
		} else {
			headerLine = styleKind.Render(fmt.Sprintf("  Trace (%d lines) — press t for suggestions", len(m.runAnalysisTrace)))
			traceAvail--
		}
		leftRows = append(leftRows, headerLine)
		if traceAvail > 0 && len(m.runAnalysisTrace) > 0 {
			start := m.runTraceOffset
			end := start + traceAvail
			if end > len(m.runAnalysisTrace) {
				end = len(m.runAnalysisTrace)
			}
			for i := start; i < end; i++ {
				ev := m.runAnalysisTrace[i]
				line := truncate(formatAnalyzerEvent(ev), lw-4)
				if !m.analyzingRun && i == m.runTraceCursor {
					leftRows = append(leftRows, styleSelected.Render("  "+line))
				} else {
					// Color-code by event kind.
					switch ev.Kind {
					case runlog.AEThought:
						leftRows = append(leftRows, styleKind.Render("  "+line))
					case runlog.AEToolCall:
						leftRows = append(leftRows, stylePass.Render("  "+line))
					case runlog.AEToolResult:
						leftRows = append(leftRows, styleNormal.Render("  "+line))
					case runlog.AEError:
						leftRows = append(leftRows, styleFail.Render("  "+line))
					case runlog.AESystemPrompt, runlog.AEUserMessage:
						leftRows = append(leftRows, styleRun.Render("  "+line))
					default:
						leftRows = append(leftRows, styleKind.Render("  "+line))
					}
				}
			}
		}
	} else {
		filteredRunSuggs := m.searchFilteredRunSuggestions()
		if len(filteredRunSuggs) == 0 {
			if m.runAnalyzeErr != "" {
				leftRows = append(leftRows, styleFail.Render("  Error: "+truncate(m.runAnalyzeErr, lw-10)))
				leftRows = append(leftRows, styleKind.Render("  Press 'A' to retry analysis."))
			} else {
				leftRows = append(leftRows, styleKind.Render("  (no suggestions — press 'a' to analyze)"))
			}
		} else {
			end := m.runSuggOffset + bodyHeight
			if end > len(filteredRunSuggs) {
				end = len(filteredRunSuggs)
			}
			for i := m.runSuggOffset; i < end; i++ {
				s := filteredRunSuggs[i]
				badge := prioLabel(s.Priority)
				titleWidth := lw - 7
				if titleWidth < 5 {
					titleWidth = 5
				}
				t := truncate(s.Title, titleWidth)
				if i == m.runSuggCursor {
					plain := fmt.Sprintf("  %s %-*s", badge, titleWidth, t)
					leftRows = append(leftRows, styleSelected.Render(plain))
				} else {
					badgeStyled := prioStyle(s.Priority).Render(badge)
					leftRows = append(leftRows, styleNormal.Render("  ")+badgeStyled+styleNormal.Render(" "+t))
				}
			}
		}
	}
	leftBody := padBody(leftRows, bodyHeight)

	// ── Right drawer: suggestion detail ────────────────────────────────────
	filteredRunSuggs2 := m.searchFilteredRunSuggestions()
	var drawerRows []string
	if !m.analyzingRun && !m.showRunTrace && len(filteredRunSuggs2) > 0 && m.runSuggCursor < len(filteredRunSuggs2) {
		s := filteredRunSuggs2[m.runSuggCursor]
		add := func(key, val string) {
			val = truncate(val, dw-len(key)-4)
			drawerRows = append(drawerRows, "  "+styleDetailKey.Render(key+":")+" "+styleDetailVal.Render(val))
		}
		add("priority", strings.ToUpper(s.Priority))
		add("category", s.Category)
		add("generated", s.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC"))
		if len(s.RunIDs) > 0 {
			ids := make([]string, len(s.RunIDs))
			for i, id := range s.RunIDs {
				ids[i] = fmt.Sprintf("%d", id)
			}
			add("run IDs", strings.Join(ids, ", "))
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("title:"))
		for _, chunk := range wrapText(s.Title, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
		drawerRows = append(drawerRows, "")
		drawerRows = append(drawerRows, "  "+styleDetailKey.Render("body:"))
		for _, chunk := range wrapText(s.Body, dw-4) {
			drawerRows = append(drawerRows, "    "+styleDetailVal.Render(chunk))
		}
	} else if m.analyzingRun {
		// While analyzing, show the selected trace event detail if we have one.
		if m.runTraceCursor >= 0 && m.runTraceCursor < len(m.runAnalysisTrace) {
			drawerRows = appendTraceEventDetail(drawerRows, m.runAnalysisTrace[m.runTraceCursor], dw)
		} else {
			drawerRows = append(drawerRows, styleKind.Render("  Running LLM analysis…"))
		}
	} else if m.showRunTrace {
		if m.runTraceCursor >= 0 && m.runTraceCursor < len(m.runAnalysisTrace) {
			drawerRows = appendTraceEventDetail(drawerRows, m.runAnalysisTrace[m.runTraceCursor], dw)
		} else {
			drawerRows = append(drawerRows, styleKind.Render("  (no trace event selected)"))
		}
	} else {
		drawerRows = append(drawerRows, styleKind.Render("  (no suggestion selected)"))
	}

	// Apply drawer scroll offset for trace views.
	if (m.analyzingRun || m.showRunTrace) && m.runTraceDrawerOffset > 0 {
		if m.runTraceDrawerOffset >= len(drawerRows) {
			drawerRows = nil
		} else {
			drawerRows = drawerRows[m.runTraceDrawerOffset:]
		}
	}
	drawerBody := padBody(drawerRows, bodyHeight)

	// ── Combine ─────────────────────────────────────────────────────────────
	styleDrawerSep := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	leftLines := strings.Split(leftBody, "\n")
	rightLines := strings.Split(drawerBody, "\n")
	for len(leftLines) < bodyHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < bodyHeight {
		rightLines = append(rightLines, "")
	}

	var combined []string
	for i := 0; i < bodyHeight; i++ {
		lPadded := padToWidth(leftLines[i], lw)
		rPadded := padToWidth(rightLines[i], dw)
		combined = append(combined, lPadded+styleDrawerSep.Render("│")+rPadded)
	}
	body := strings.Join(combined, "\n")

	// ── Help bar ─────────────────────────────────────────────────────────────
	scrollInfo := ""
	var helpLeft string
	if m.analyzingRun || m.showRunTrace {
		// Trace mode help.
		tracePos := ""
		if len(m.runAnalysisTrace) > 0 {
			tracePos = fmt.Sprintf("  %d/%d", m.runTraceCursor+1, len(m.runAnalysisTrace))
		}
		helpLeft = "  ↑↓/jk select · ←→/hl scroll detail · a re-analyze · t suggestions · Esc back · q quit" + tracePos
	} else {
		if len(filteredRunSuggs2) > 0 && len(filteredRunSuggs2) > bodyHeight {
			pct := int(math.Round(float64(m.runSuggCursor+1) / float64(len(filteredRunSuggs2)) * 100))
			scrollInfo = fmt.Sprintf("  %d/%d (%d%%)", m.runSuggCursor+1, len(filteredRunSuggs2), pct)
		}
		helpLeft = "  ↑↓/jk navigate · a re-analyze · / search · t trace · Esc back · q quit" + scrollInfo
	}
	if m.runAnalyzeErr != "" {
		helpLeft = styleHelp.Render(helpLeft) + " " + styleFail.Render("err: "+truncate(m.runAnalyzeErr, 40))
		return strings.Join([]string{title, body, helpLeft}, "\n")
	}
	help := styleHelp.Render(helpLeft)
	return strings.Join([]string{title, body, help}, "\n")
}

// launchTest starts a test as a detached subprocess using the configured
// testCommand (from .runlog.yaml) or the default "go test -v -run {name} ./...".
// The process is started with Setsid so it survives if the TUI is closed.
// On success it inserts a test_launchers row and sends testLaunchedMsg{pid};
// on error it sends testLaunchErrMsg.
func (m model) launchTest(testName string) tea.Cmd {
	env := m.testEnv
	db := m.db
	cfg := m.config
	return func() tea.Msg {
		// Build the command from the config template.
		// Supported placeholders: {name} → test function name, {env} → test env.
		tmpl := cfg.TestCommandOrDefault()
		expanded := strings.ReplaceAll(tmpl, "{name}", testName)
		expanded = strings.ReplaceAll(expanded, "{env}", env)

		// Split into command + args. Use shell execution via "sh -c" so that
		// pipes, redirects, and complex commands work as expected.
		now := time.Now()
		cmd := exec.Command("sh", "-c", expanded)
		// Run in a new session so the child survives if the TUI is closed.
		setSysProcAttr(cmd)
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil
		if err := cmd.Start(); err != nil {
			return testLaunchErrMsg{err}
		}
		pid := cmd.Process.Pid
		// Detach: let the child run on its own.
		go func() { _ = cmd.Wait() }()

		// Persist the launcher PID to the DB so it survives runlog restarts.
		launcherID, err := db.InsertLauncher(testName, env, now, pid)
		if err != nil {
			// Non-fatal: we still launched successfully, just without DB record.
			launcherID = 0
		}
		return testLaunchedMsg{launcherID: launcherID, pid: pid}
	}
}

// killTest sends SIGTERM to the given PID and marks the launcher row finished.
func (m model) killTest(launcherID int64, pid int) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if err := sigterm(pid); err != nil {
			return testKillErrMsg{fmt.Errorf("kill %d: %w", pid, err)}
		}
		if launcherID != 0 {
			_ = db.FinishLauncher(launcherID, time.Now())
		}
		return testKilledMsg{}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Search helpers
// ─────────────────────────────────────────────────────────────────────────────

// matchesSearch returns true if the given text contains the search query
// (case-insensitive substring match). Returns true for empty queries.
func (m model) matchesSearch(text string) bool {
	if m.searchQuery == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(m.searchQuery))
}

// resetCursorsForSearch resets cursors to 0 for the current view after
// a search query is committed or cleared. This ensures the cursor doesn't
// point beyond the (now filtered) data.
func (m *model) resetCursorsForSearch() {
	switch m.state {
	case viewRuns:
		m.runCursor = 0
		m.runOffset = 0
	case viewEvents:
		m.listCursor = 0
		m.listOffset = 0
	case viewExperiments:
		m.expCursor = 0
	case viewExpRuns:
		m.expRunCursor = 0
		m.expRunOffset = 0
	case viewExpAnalysis:
		m.suggCursor = 0
		m.suggOffset = 0
	case viewTests:
		m.testCursor = 0
		m.testOffset = 0
	case viewTestRuns:
		m.testRunCursor = 0
		m.testRunOffset = 0
	case viewTestAnalysis:
		m.testSuggCursor = 0
		m.testSuggOffset = 0
	case viewRunAnalysis:
		m.runSuggCursor = 0
		m.runSuggOffset = 0
		m.runTraceCursor = 0
		m.runTraceOffset = 0
	}
}

// searchableViews returns true for views that support the / search shortcut.
func searchableView(s viewState) bool {
	switch s {
	case viewRuns, viewEvents, viewExperiments, viewExpRuns, viewExpAnalysis,
		viewTests, viewTestRuns, viewTestAnalysis, viewRunAnalysis:
		return true
	}
	return false
}

// searchFilteredExperiments returns the experiments list filtered by the search query.
func (m model) searchFilteredExperiments() []runlog.ExperimentSummary {
	if m.searchQuery == "" {
		return m.experiments
	}
	var out []runlog.ExperimentSummary
	for _, exp := range m.experiments {
		if m.matchesSearch(exp.Name) {
			out = append(out, exp)
		}
	}
	return out
}

// searchFilteredExpRuns returns the experiment runs filtered by the search query on TestName.
func (m model) searchFilteredExpRuns() []runlog.RunRow {
	if m.searchQuery == "" {
		return m.expRuns
	}
	var out []runlog.RunRow
	for _, r := range m.expRuns {
		if m.matchesSearch(r.TestName) {
			out = append(out, r)
		}
	}
	return out
}

// searchFilteredSuggestions returns the suggestions list filtered by the search query on Title.
func (m model) searchFilteredSuggestions() []runlog.SuggestionRow {
	if m.searchQuery == "" {
		return m.suggestions
	}
	var out []runlog.SuggestionRow
	for _, s := range m.suggestions {
		if m.matchesSearch(s.Title) {
			out = append(out, s)
		}
	}
	return out
}

// searchFilteredTests returns the test entries list filtered by the search query on Name.
func (m model) searchFilteredTests() []testEntry {
	if m.searchQuery == "" {
		return m.testEntries
	}
	var out []testEntry
	for _, te := range m.testEntries {
		if m.matchesSearch(te.Name) {
			out = append(out, te)
		}
	}
	return out
}

// searchFilteredTestSuggestions returns test suggestions filtered by the search query on Title.
func (m model) searchFilteredTestSuggestions() []runlog.SuggestionRow {
	if m.searchQuery == "" {
		return m.testSuggestions
	}
	var out []runlog.SuggestionRow
	for _, s := range m.testSuggestions {
		if m.matchesSearch(s.Title) {
			out = append(out, s)
		}
	}
	return out
}

// searchFilteredRunSuggestions returns run-level suggestions filtered by the search query on Title.
func (m model) searchFilteredRunSuggestions() []runlog.SuggestionRow {
	if m.searchQuery == "" {
		return m.runSuggestions
	}
	var out []runlog.SuggestionRow
	for _, s := range m.runSuggestions {
		if m.matchesSearch(s.Title) {
			out = append(out, s)
		}
	}
	return out
}

// searchFilteredDisplayList returns the events display list filtered by the search query on Kind/Message.
func (m model) searchFilteredDisplayList() []displayItem {
	if m.searchQuery == "" {
		return m.displayList
	}
	var out []displayItem
	for _, item := range m.displayList {
		ev := m.events[item.eventIdx]
		if item.childIdx >= 0 {
			child := ev.Children[item.childIdx]
			if m.matchesSearch(child.Kind) || m.matchesSearch(child.Message) {
				out = append(out, item)
			}
		} else {
			if m.matchesSearch(ev.Kind) || m.matchesSearch(ev.Message) {
				out = append(out, item)
			}
		}
	}
	return out
}

// applySearchBar overlays the search input or active-filter indicator onto the
// last line (help bar) of the rendered view output.
func (m model) applySearchBar(out string) string {
	if !m.searchActive && m.searchQuery == "" {
		return out
	}
	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		return out
	}

	if m.searchActive {
		// Replace help bar with search input prompt.
		prompt := "  / " + m.searchInput + "█"
		padded := prompt + strings.Repeat(" ", max(0, m.width-utf8.RuneCountInString(prompt)))
		lines[len(lines)-1] = styleHelp.Render(padded)
	} else if m.searchQuery != "" {
		// Append search indicator to the existing help bar.
		indicator := styleHelp.Render("  [search: " + m.searchQuery + "]")
		lines[len(lines)-1] = lines[len(lines)-1] + indicator
	}
	return strings.Join(lines, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// TUI — Layout helpers
// ─────────────────────────────────────────────────────────────────────────────

// filteredRuns returns the subset of m.runs matching the active filterTest
// and search query, or all runs when neither is set.
func (m model) filteredRuns() []runlog.RunRow {
	if m.filterTest == "" && m.searchQuery == "" {
		return m.runs
	}
	var out []runlog.RunRow
	for _, r := range m.runs {
		if m.filterTest != "" && r.TestName != m.filterTest {
			continue
		}
		if !m.matchesSearch(r.TestName) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// visibleRunRows returns the number of run list rows that fit in the current
// terminal — mirrors the bodyHeight calculation in viewRuns().
func (m model) visibleRunRows() int {
	n := m.height - 3 - 1 // titleBar(1) + helpBar(1) + header(1) = 3 overhead lines
	if n < 1 {
		return 1
	}
	return n
}

// ensureRunVisible adjusts m.runOffset so that m.runCursor is within the
// visible window. Call this after every cursor movement.
func (m *model) ensureRunVisible() {
	pageH := m.visibleRunRows()
	if m.runCursor < m.runOffset {
		m.runOffset = m.runCursor
	} else if m.runCursor >= m.runOffset+pageH {
		m.runOffset = m.runCursor - pageH + 1
	}
}

func (m model) visibleEventRows() int {
	n := m.height - 2
	if n < 1 {
		return 1
	}
	return n
}

func (m model) visibleDetailLines() int {
	n := m.height - 2
	if n < 1 {
		return 1
	}
	return n
}

// drawerWidth returns the width of the right-side inspector panel.
// It is approximately 1/2 of the terminal width, with a minimum of 28 columns.
func (m model) drawerWidth() int {
	w := m.width / 2
	if w < 28 {
		w = 28
	}
	return w
}

// visibleDrawerLines returns the number of content lines that fit in the drawer.
func (m model) visibleDrawerLines() int {
	n := m.height - 2
	if n < 1 {
		return 1
	}
	return n
}

// visibleExpRunRows returns the number of run rows visible in the viewExpRuns panel.
func (m model) visibleExpRunRows() int {
	// title(1) + header(1) + help(1) = 3 overhead lines
	n := m.height - 3
	if n < 1 {
		return 1
	}
	return n
}

// visibleSuggRows returns the number of suggestion rows visible in viewExpAnalysis.
func (m model) visibleSuggRows() int {
	// title(1) + help(1) = 2 overhead lines
	n := m.height - 2
	if n < 1 {
		return 1
	}
	return n
}

// visibleTestRows returns the number of test rows visible in viewTests.
func (m model) visibleTestRows() int {
	// tabBar(1) + header(1) + help(1) = 3 overhead
	n := m.height - 3 - 1 // -1 for header
	if n < 1 {
		return 1
	}
	return n
}

// visibleTestRunRows returns the number of run rows visible in viewTestRuns.
func (m model) visibleTestRunRows() int {
	// title(1) + header(1) + help(1) = 3 overhead
	n := m.height - 3 - 1 // -1 for header
	if n < 1 {
		return 1
	}
	return n
}

// testFilteredRuns returns all runs whose TestName matches the selectedTest.
func (m model) testFilteredRuns() []runlog.RunRow {
	if m.selectedTest.Name == "" {
		return nil
	}
	var out []runlog.RunRow
	for _, r := range m.runs {
		if r.TestName == m.selectedTest.Name {
			out = append(out, r)
		}
	}
	return out
}

// titleBar renders a full-width title bar with the given plain-text content.
// It avoids .Width(m.width) on the lipgloss style — which over-counts by the
// padding width and produces a clipped line with a black trailing patch.
// Instead, the content string is padded with spaces so that
// content + PaddingLeft(1) + PaddingRight(1) == m.width exactly.
func (m model) titleBar(content string) string {
	// styleTitleBar has PaddingLeft(1) and PaddingRight(1) = 2 chars total.
	const padCols = 2
	innerWidth := m.width - padCols
	if innerWidth < 1 {
		innerWidth = 1
	}
	runeLen := utf8.RuneCountInString(content)
	if runeLen < innerWidth {
		content += strings.Repeat(" ", innerWidth-runeLen)
	}
	return styleTitleBar.Render(content)
}

// tabBar renders the two-tab bar used by viewRuns and viewExperiments.
// The active tab is highlighted; inactive tab is dimmed.
func (m model) tabBar() string {
	var parts []string
	for i, name := range tabNames {
		if i == m.activeTab {
			parts = append(parts, styleTabActive.Render(name))
		} else {
			parts = append(parts, styleTabInactive.Render(name))
		}
	}
	bar := strings.Join(parts, "")
	// Pad the rest of the line with the background used by the title bar.
	used := 0
	for _, p := range parts {
		used += utf8.RuneCountInString(stripANSI(p))
	}
	remainder := m.width - used
	if remainder > 0 {
		bar += lipgloss.NewStyle().Background(lipgloss.Color("234")).Render(strings.Repeat(" ", remainder))
	}
	return bar
}

// padBody joins rows into a string and appends blank lines to reach exactly
// `height` lines. This keeps the total view output at a fixed height so
// bubbletea always redraws the title bar at the top of the screen.
func padBody(rows []string, height int) string {
	body := strings.Join(rows, "\n")
	if len(rows) < height {
		body += strings.Repeat("\n", height-len(rows))
	}
	return body
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared rendering helpers
// ─────────────────────────────────────────────────────────────────────────────

func statusLabel(r runlog.RunRow) string {
	if r.Skipped {
		return styleSkip.Render("SKIP")
	}
	if r.Passed == nil {
		if r.FinishedAt != nil {
			return styleAbort.Render("DEAD") // zombie: finished but outcome not recorded
		}
		return styleRun.Render("RUNS") // spinner — still in-flight
	}
	if *r.Passed {
		return stylePass.Render("PASS")
	}
	return styleFail.Render("FAIL")
}

func kindStyled(kind string) string {
	return kindStyledWithDetails(kind, "")
}

// kindStyledWithDetails returns the styled kind string, using red for "cli"
// events that have a non-zero exit_code in their details JSON.
func kindStyledWithDetails(kind, detailsJSON string) string {
	padded := fmt.Sprintf("%-12s", kind)
	if kind == "cli" && detailsJSON != "" {
		if cliExitCode(detailsJSON) != 0 {
			return styleCLIFail.Render(padded)
		}
		return styleCLI.Render(padded)
	}
	switch kind {
	case "section":
		return styleSection.Render(padded)
	case "state_change":
		return styleStateChange.Render(padded)
	case "metric", "token_summary", "gantt_row":
		return styleMetric.Render(padded)
	case "cli":
		return styleCLI.Render(padded)
	case "group":
		return styleSection.Render(padded)
	case "trace_span":
		return styleMetric.Render(padded)
	case "failure":
		return styleFail.Render(padded)
	default:
		return styleKind.Render(padded)
	}
}

// cliExitCode parses the exit_code field from a cli event's details JSON.
// Returns 0 if absent or zero (success).
func cliExitCode(detailsJSON string) int {
	var d struct {
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &d); err != nil {
		return 0
	}
	return d.ExitCode
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func formatDuration(start time.Time, end *time.Time) string {
	return formatDurationPlain(start, end)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

func prettyJSON(raw string) []string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return strings.Split(raw, "\n")
	}
	return strings.Split(buf.String(), "\n")
}

// wrapText splits s into lines of at most w runes each, breaking on spaces.
func wrapText(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var lines []string
	for utf8.RuneCountInString(s) > w {
		cut := w
		// walk back to a space boundary
		runes := []rune(s)
		for cut > 0 && runes[cut-1] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = w // no space found — hard cut
		}
		lines = append(lines, strings.TrimRight(string(runes[:cut]), " "))
		s = strings.TrimLeft(string(runes[cut:]), " ")
	}
	if s != "" {
		lines = append(lines, s)
	}
	return lines
}

// stripANSI removes ANSI CSI escape sequences from s.
func stripANSI(s string) string {
	var out []rune
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			// skip until a byte in 0x40–0x7E (the final byte of a CSI sequence)
			i += 2
			for i < len(runes) {
				c := runes[i]
				i++
				if c >= 0x40 && c <= 0x7E {
					break
				}
			}
			continue
		}
		out = append(out, runes[i])
		i++
	}
	return string(out)
}

// padToWidth appends spaces to s so that its visible (ANSI-stripped) rune width
// equals w. If it is already >= w, it is returned as-is (no truncation).
func padToWidth(s string, w int) string {
	visible := utf8.RuneCountInString(stripANSI(s))
	if visible < w {
		return s + strings.Repeat(" ", w-visible)
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// Math helpers
// ─────────────────────────────────────────────────────────────────────────────

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// DB path resolution
// ─────────────────────────────────────────────────────────────────────────────

func resolveDBPath(explicit string) string {
	// 1. Explicit --db flag.
	if explicit != "" {
		return explicit
	}

	// 2. $RUNLOG_DB environment variable.
	if d := os.Getenv("RUNLOG_DB"); d != "" {
		return d
	}

	// 3. Legacy $TEST_LOG_DIR (for backward compatibility with e2e repo).
	if d := os.Getenv("TEST_LOG_DIR"); d != "" {
		return filepath.Join(d, "runs.db")
	}

	// 4. config file "db" field.
	if cfg, err := runlog.LoadConfig(""); err == nil && cfg.DBPath != "" {
		return cfg.DBPath
	}

	// 5. Default write location in .runlog under project root.
	runlogDir := runlog.RunlogDir()
	if runlogDir != "" {
		return filepath.Join(runlogDir, "runs.db")
	}

	return ".runlog/runs.db"
}

// ─────────────────────────────────────────────────────────────────────────────
// Usage
// ─────────────────────────────────────────────────────────────────────────────

func usage() {
	fmt.Fprint(os.Stderr, `runlog — Go test run log browser and TUI

USAGE
  runlog [flags]                        open interactive TUI (auto-refreshes every 2s)
  runlog runs [flags]                   list recent runs
  runlog events [flags] <run-id>        list events for a run
  runlog show [flags] <run-id>          full detail dump of a run
  runlog tail [flags]                   stream new events as they arrive
  runlog experiments [flags]            list all experiments (non-interactive)
  runlog tests [flags]                  list all known tests with last status
  runlog tests [flags] <test-name>      list recent runs for a specific test
  runlog inspect [flags] <run-id>       full inspector dump of a run (all events + details)
  runlog analyze [flags] <run-id>       LLM analysis of a run with full conversation trace
  runlog trace [flags] <run-id>         show stored analysis trace for a run (no LLM call)
  runlog skills install [flags]         install embedded skills into tool directories
  runlog skills list                    list all embedded skills
  runlog test [<profile>] [<filter>]    load .env and run go test (profile = MEMORY_TEST_ENV)
  runlog clear [--db <path>]            delete runs.db and all per-run log files
  runlog reap [--dry-run] [<run-id>]    mark stale/orphaned runs as FAIL
  runlog version                        print version and exit

FLAGS
  --db <path>      path to runs.db  (default: auto-resolved to .runlog/runs.db)
  --since <dur>    time window for "runs", "tests", and TUI, e.g. 5m, 1h, 24h  (default: 24h)
  --json           (analyze only) output suggestions as JSON instead of text

EXAMPLES
  runlog                                # interactive TUI, last 24 hours (auto-refreshes)
  runlog --since 1h                     # TUI, last hour
  runlog runs                           # plain table of recent runs (last 24h)
  runlog runs --since 2h                # runs from last 2 hours
  runlog events 42                      # events for run ID 42
  runlog show 42                        # full detail dump for run 42
  runlog show 42 | grep state_change
  runlog tail                           # live stream of new events
  runlog tail --since 1h                # include runs from last hour
  runlog experiments                    # table of all experiments
  runlog tests                          # table of all tests with last run status
  runlog tests --since 7d              # tests with runs from last 7 days
  runlog tests TestCLIInstalled_Version # runs for that specific test
  runlog inspect 42                     # all events + inspector details for run 42
  runlog analyze 42                     # LLM analysis of run 42 with full trace
  runlog analyze --json 42              # same but output suggestions as JSON
  runlog trace 42                       # show stored trace from last analysis of run 42
  runlog clear                          # delete runs.db + all log files/dirs in .runlog/
  runlog clear --db /path/to/runs.db    # clear a specific database location
  runlog reap                           # mark all stale (orphaned) runs as FAIL
  runlog reap 527                       # mark only run 527 as FAIL
  runlog reap --dry-run                 # show stale runs without changing anything
  runlog test                           # all tests using .env defaults
  runlog test mcj-emergent              # overlay .env.mcj-emergent on .env
  runlog test localhost TestCLI_Version # named env + single test filter
  runlog test -- -count=1 -timeout 5m  # pass raw go test flags
`)
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

// parseSince parses a --since flag value and exits on error.
func parseSince(val, context string) time.Duration {
	d, err := time.ParseDuration(val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runlog %s: invalid --since value %q: %v\n", context, val, err)
		os.Exit(1)
	}
	return d
}

// subFS returns a new FlagSet wired with the common --db and --since flags,
// inheriting the provided defaults. The caller must call fs.Parse(args) and
// then read back *dbOut / *sinceOut.
func subFS(name, dbDefault, sinceDefault string) (fs *flag.FlagSet, dbOut, sinceOut *string) {
	fs = flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = usage
	dbOut = fs.String("db", dbDefault, "path to runs.db")
	sinceOut = fs.String("since", sinceDefault, "time window (e.g. 5m, 1h, 24h)")
	return
}

func main() {
	// Load .env (and optional .env.<MEMORY_TEST_ENV> overlay) from the
	// current working directory so GOOGLE_AI_API_KEY and other env vars are
	// available without requiring shell exports.
	wd, _ := os.Getwd()
	if wd != "" {
		runlog.LoadDotEnvFrom(wd)
	}

	// Phase 1: peel off global flags that appear BEFORE the subcommand.
	// flag.ContinueOnError + fs.Parse stops at the first non-flag arg, so
	// after this call fs.Args()[0] is the subcommand (if any) and the rest
	// are the subcommand's own args (which may contain more flags).
	globalFS := flag.NewFlagSet("runlog", flag.ContinueOnError)
	globalFS.Usage = usage
	globalDB := globalFS.String("db", "", "path to runs.db")
	globalSince := globalFS.String("since", "24h", "time window (e.g. 5m, 1h, 24h)")

	if err := globalFS.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		os.Exit(2)
	}

	// Phase 2: identify the subcommand and parse its own flags, which lets
	// flags appear either before or after the subcommand word.
	remaining := globalFS.Args() // everything after the last global flag
	subcommand := ""
	subArgs := remaining
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		subcommand = remaining[0]
		subArgs = remaining[1:]
	}

	// Per-subcommand flag parsing. Each subcommand gets its own FlagSet that
	// inherits the current global values as defaults, so global flags set
	// before the subcommand word are still honoured.
	var dbPath string
	var since time.Duration
	var analyzeJSON *bool // set by "analyze" subcommand
	var reapDryRun *bool  // set by "reap" subcommand

	switch subcommand {
	case "runs", "tail", "":
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)

	case "events", "show":
		// --since is not meaningful for events/show but accept it silently.
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // positional args (run-id)

	case "experiments":
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)

	case "tests":
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // optional positional arg: test name for run listing

	case "inspect":
		// --since is not meaningful for inspect but accept it silently.
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // positional args (run-id)

	case "analyze":
		// analyze <run-id> [--json] — LLM analysis with full trace.
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		analyzeJSON = fs.Bool("json", false, "output suggestions as JSON")
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // positional args (run-id)

	case "trace":
		// trace <run-id> — show stored analysis trace.
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // positional args (run-id)

	case "clear":
		// clear does not need --since; only --db is relevant.
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)

	case "reap":
		// reap [--dry-run] [<run-id>]
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		reapDryRun = fs.Bool("dry-run", false, "show what would be reaped without changing anything")
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // positional args (optional run-id)

	case "skills":
		// skills subcommand manages agent skill installation; does not need DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")

	case "test":
		// test loads .env and execs go test; does not need DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")

	default:
		// Unknown subcommand or help — handle below without DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")
	}

	// "clear" and "skills" do not need the DB — handle before opening it.
	if subcommand == "clear" {
		if err := cmdClear(dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "runlog clear: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if subcommand == "skills" {
		if err := cmdSkills(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog skills: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if subcommand == "test" {
		if err := cmdTest(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog test: %v\n", err)
			os.Exit(1)
		}
		return
	}

	db, err := runlog.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runlog: cannot open database at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	switch subcommand {

	case "runs":
		if err := cmdRuns(db, since); err != nil {
			fmt.Fprintf(os.Stderr, "runlog runs: %v\n", err)
			os.Exit(1)
		}

	case "events":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog events: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog events <run-id>\n")
			os.Exit(2)
		}
		runID, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog events: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdEvents(db, runID); err != nil {
			fmt.Fprintf(os.Stderr, "runlog events: %v\n", err)
			os.Exit(1)
		}

	case "show":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog show: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog show <run-id>\n")
			os.Exit(2)
		}
		runID, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog show: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdShow(db, runID); err != nil {
			fmt.Fprintf(os.Stderr, "runlog show: %v\n", err)
			os.Exit(1)
		}

	case "tail":
		if err := cmdTail(db, since); err != nil {
			fmt.Fprintf(os.Stderr, "runlog tail: %v\n", err)
			os.Exit(1)
		}

	case "experiments":
		if err := cmdExperiments(db); err != nil {
			fmt.Fprintf(os.Stderr, "runlog experiments: %v\n", err)
			os.Exit(1)
		}

	case "tests":
		if len(subArgs) > 0 {
			// tests <name> — list runs for that test
			if err := cmdTestRuns(db, subArgs[0], since); err != nil {
				fmt.Fprintf(os.Stderr, "runlog tests: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := cmdTestsList(db, since); err != nil {
				fmt.Fprintf(os.Stderr, "runlog tests: %v\n", err)
				os.Exit(1)
			}
		}

	case "inspect":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog inspect: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog inspect <run-id>\n")
			os.Exit(2)
		}
		runID, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog inspect: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdInspect(db, runID); err != nil {
			fmt.Fprintf(os.Stderr, "runlog inspect: %v\n", err)
			os.Exit(1)
		}

	case "analyze":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog analyze: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog analyze [--json] <run-id>\n")
			os.Exit(2)
		}
		runID, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog analyze: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdAnalyze(db, runID, *analyzeJSON); err != nil {
			fmt.Fprintf(os.Stderr, "runlog analyze: %v\n", err)
			os.Exit(1)
		}

	case "trace":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog trace: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog trace <run-id>\n")
			os.Exit(2)
		}
		runID, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog trace: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdTrace(db, runID); err != nil {
			fmt.Fprintf(os.Stderr, "runlog trace: %v\n", err)
			os.Exit(1)
		}

	case "reap":
		var runID int64
		if len(subArgs) > 0 {
			var err error
			runID, err = strconv.ParseInt(subArgs[0], 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "runlog reap: invalid run-id %q: %v\n", subArgs[0], err)
				os.Exit(2)
			}
		}
		if err := cmdReap(db, runID, *reapDryRun); err != nil {
			fmt.Fprintf(os.Stderr, "runlog reap: %v\n", err)
			os.Exit(1)
		}

	case "help", "--help", "-h":
		usage()

	case "version", "--version":
		fmt.Printf("runlog %s (commit %s, built %s)\n", version, commit, date)

	case "":
		// No subcommand — launch TUI.
		dbDir := filepath.Dir(db.Path())
		cfg, cfgErr := runlog.LoadConfig(dbDir)
		if cfgErr != nil {
			fmt.Fprintf(os.Stderr, "runlog: warning: config error: %v\n", cfgErr)
			cfg = &runlog.Config{}
		}
		p := tea.NewProgram(
			newModel(db, since, cfg),
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "runlog: TUI error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "runlog: unknown subcommand %q\n\n", subcommand)
		usage()
		os.Exit(2)
	}
}
