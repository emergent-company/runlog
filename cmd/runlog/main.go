// cmd/runlog — CLI query tool for the runlog test database.
//
// # Subcommands
//
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
//	--since 5m                     time window (default: 5m)
//
// # Web UI
//
//	The web UI is served by the daemon (runlog daemon start) at <host>:<port>/ui/.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	rows, err := db.ListRuns(time.Now().Add(-since), 0)
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
	printRunSummary(rows)
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
		dur := ""
		if e.DurationMs != nil && *e.DurationMs > 0 {
			if *e.DurationMs < 1000 {
				dur = fmt.Sprintf(" %.0fms", *e.DurationMs)
			} else {
				dur = fmt.Sprintf(" %.1fs", *e.DurationMs/1000)
			}
		}
		fmt.Printf("%-5d  %7.2fs%s  %-14s  %-10s  %s\n",
			e.Seq, e.ElapsedS, dur, e.Kind, occurred, e.Message)
	}
	return nil
}

// cmdShow prints full detail of a run: metadata header then every event
// including pretty-printed details JSON.
func cmdShow(db *runlog.RunDB, runID int64) error {
	// Fetch the run row via ListRuns with a wide enough window.
	rows, err := db.ListRuns(time.Time{}, 0)
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
		dur := durationSuffix(e.DurationMs)
		fmt.Printf("[%5.2fs%s] %-14s  %-10s  %s\n", e.ElapsedS, dur, e.Kind, occurred, e.Message)
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
		rows, err := db.ListRuns(lastSeen, 0)
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

// cmdFailing prints a table of tests whose most recent completed run (within
// the since window) was a failure, sorted by failure streak length descending.
func cmdFailing(db *runlog.RunDB, since time.Duration) error {
	rows, err := db.ListFailingTests(time.Now().Add(-since))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Printf("no failing tests in the last %s\n", since)
		return nil
	}

	fmt.Printf("%-55s  %-8s  %-8s  %s\n", "TEST", "STREAK", "AGO", "REASON")
	fmt.Println(strings.Repeat("─", 110))
	for _, r := range rows {
		// Build streak string: ✗ repeated up to 5, then count.
		marks := r.FailStreak
		if marks > 5 {
			marks = 5
		}
		streakStr := strings.Repeat("✗", marks)
		if r.FailStreak > 5 {
			streakStr += fmt.Sprintf(" %d", r.FailStreak)
		} else {
			streakStr += fmt.Sprintf(" %d", r.FailStreak)
		}

		ago := formatAgePlain(r.LastRunAt)
		reason := "—"
		if r.Reason != nil && *r.Reason != "" {
			reason = *r.Reason
			if len(reason) > 50 {
				reason = reason[:49] + "…"
			}
		}
		fmt.Printf("%-55s  %-8s  %-8s  %s\n",
			truncate(r.TestName, 55), streakStr, ago, reason)
	}
	return nil
}

// cmdStats prints per-test aggregated statistics (pass rate, duration, run
// count) for all tests with at least one completed run within the since window.
func cmdStats(db *runlog.RunDB, since time.Duration) error {
	rows, err := db.TestStats(time.Now().Add(-since))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Printf("no test runs found in the last %s\n", since)
		return nil
	}

	fmt.Printf("%-50s  %5s  %5s  %7s  %7s  %7s  %s\n",
		"TEST", "RUNS", "PASS%", "AVG", "MIN", "MAX", "LAST")
	fmt.Println(strings.Repeat("─", 105))
	for _, r := range rows {
		passRate := "   —"
		if r.TotalRuns > 0 {
			pct := int(float64(r.PassCount) / float64(r.TotalRuns) * 100)
			passRate = fmt.Sprintf("%3d%%", pct)
		}

		fmtDur := func(s float64) string {
			if s == 0 {
				return "  —    "
			}
			return fmt.Sprintf("%6.1fs", s)
		}

		lastStr := "—"
		if r.LastPassed != nil {
			if *r.LastPassed {
				lastStr = "✓ " + formatAgePlain(r.LastRunAt)
			} else {
				lastStr = "✗ " + formatAgePlain(r.LastRunAt)
			}
		} else {
			lastStr = "~ " + formatAgePlain(r.LastRunAt) // skip
		}

		fmt.Printf("%-50s  %5d  %5s  %7s  %7s  %7s  %s\n",
			truncate(r.TestName, 50),
			r.TotalRuns,
			passRate,
			fmtDur(r.AvgDurationS),
			fmtDur(r.MinDurationS),
			fmtDur(r.MaxDurationS),
			lastStr,
		)
	}
	return nil
}

// cmdTestsList prints a plain-text table of all known tests with their last
// run status and age, discovered from the database and optional config.
func cmdTestsList(db *runlog.RunDB, since time.Duration, category, testType string) error {
	rows, err := db.ListRuns(time.Now().Add(-since), 0)
	if err != nil {
		return err
	}

	// Discover tests from DB + config.
	dbDir := filepath.Dir(db.Path())
	_, _ = runlog.LoadConfig(dbDir)
	names, err := db.DiscoverTests()
	if err != nil {
		return err
	}

	// Real per-test category/test_type from the catalog, replacing the old
	// hardcoded "Uncategorized" bucket.
	catalog, err := db.ListTestCatalog()
	if err != nil {
		return err
	}
	catByName, typeByName := categoryTestTypeMaps(catalog)

	var entries []testEntry
	for _, name := range names {
		cat := catByName[name]
		if cat == "" {
			cat = "Uncategorized"
		}
		tt := typeByName[name]
		if tt == "" {
			tt = "other"
		}
		if category != "" && cat != category {
			continue
		}
		if testType != "" && tt != testType {
			continue
		}
		entries = append(entries, testEntry{Name: name, Category: cat, TestType: tt})
	}

	fmt.Printf("%-20s  %-*s  %-12s  %6s  %4s\n", "category", 55, "test name", "type", "last", "st")
	fmt.Println(strings.Repeat("─", 105))
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
		fmt.Printf("%-20s  %-55s  %-12s  %6s  %4s\n",
			truncate(catLabel, 20), truncate(te.Name, 55), truncate(te.TestType, 12), lastAge, lastStatus)
	}
	return nil
}

// cmdTestRuns prints all recent runs for a specific test name.
func cmdTestRuns(db *runlog.RunDB, testName string, since time.Duration) error {
	rows, err := db.ListRuns(time.Now().Add(-since), 0)
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
	hasCost := false
	for _, r := range matching {
		if r.CostUSD != nil && *r.CostUSD > 0 {
			hasCost = true
			break
		}
	}
	fmt.Printf("runs for: %s\n", testName)
	fmt.Println(strings.Repeat("─", 80))
	if hasCost {
		fmt.Printf("%-6s  %-8s  %-8s  %-8s  %-7s  %s\n", "ID", "STATUS", "AGE", "DURATION", "EVENTS", "COST")
	} else {
		fmt.Printf("%-6s  %-8s  %-8s  %-8s  %-7s\n", "ID", "STATUS", "AGE", "DURATION", "EVENTS")
	}
	fmt.Println(strings.Repeat("─", 80))
	for _, r := range matching {
		if hasCost {
			costStr := "—"
			if r.CostUSD != nil && *r.CostUSD > 0 {
				costStr = fmt.Sprintf("$%.6f", *r.CostUSD)
			}
			fmt.Printf("%-6d  %-8s  %-8s  %-8s  %-7d  %s\n",
				r.ID, passLabel(r), formatAgePlain(r.StartedAt),
				formatDurationPlain(r.StartedAt, r.FinishedAt), r.EventCount, costStr)
		} else {
			fmt.Printf("%-6d  %-8s  %-8s  %-8s  %-7d\n",
				r.ID, passLabel(r), formatAgePlain(r.StartedAt),
				formatDurationPlain(r.StartedAt, r.FinishedAt), r.EventCount)
		}
	}
	printRunSummary(matching)
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
	allRuns, err := db.ListRuns(time.Time{}, 0)
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
	rows, err := db.ListRuns(time.Time{}, 0)
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
	category := "Uncategorized"
	if run.Category != nil && *run.Category != "" {
		category = *run.Category
	}
	testType := run.TestType
	if testType == "" {
		testType = "other"
	}
	fmt.Printf("category: %s\n", category)
	fmt.Printf("type:     %s\n", testType)
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
// TUI — Known tests registry
// ─────────────────────────────────────────────────────────────────────────────

type testEntry struct {
	Name     string
	Category string
	TestType string
}

// categoryTestTypeMaps builds test-name → category and test-name → test_type
// lookup maps from a test catalog snapshot, defaulting blank values to
// "Uncategorized" / "other" respectively. Shared by cmdTestsList (plain CLI
// output) and loadTests (TUI) so both surfaces show real per-test
// classification instead of a hardcoded literal.
func categoryTestTypeMaps(catalog []runlog.TestCatalogRow) (cats, types map[string]string) {
	cats = make(map[string]string, len(catalog))
	types = make(map[string]string, len(catalog))
	for _, c := range catalog {
		cat := c.Category
		if cat == "" {
			cat = "Uncategorized"
		}
		tt := c.TestType
		if tt == "" {
			tt = "other"
		}
		cats[c.TestName] = cat
		types[c.TestName] = tt
	}
	return cats, types
}

// ─────────────────────────────────────────────────────────────────────────────
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

// printRunSummary appends an aggregate footer after a run table.
func printRunSummary(rows []runlog.RunRow) {
	if len(rows) == 0 {
		return
	}
	var passed, failed, skipped int
	var totalDur time.Duration
	durCount := 0
	var totalCost float64
	var totalIn, totalOut int64
	hasCost := false
	for _, r := range rows {
		if r.Skipped {
			skipped++
		} else if r.Passed != nil {
			if *r.Passed {
				passed++
			} else {
				failed++
			}
		}
		if r.FinishedAt != nil {
			totalDur += r.FinishedAt.Sub(r.StartedAt)
			durCount++
		}
		if r.CostUSD != nil && *r.CostUSD > 0 {
			totalCost += *r.CostUSD
			hasCost = true
		}
		if r.InputTokens != nil {
			totalIn += *r.InputTokens
		}
		if r.OutputTokens != nil {
			totalOut += *r.OutputTokens
		}
	}

	fmt.Println(strings.Repeat("─", 60))
	// counts line
	countLine := fmt.Sprintf("  runs: %d  passed: %d  failed: %d", len(rows), passed, failed)
	if skipped > 0 {
		countLine += fmt.Sprintf("  skipped: %d", skipped)
	}
	fmt.Println(countLine)

	// timing line
	if durCount > 0 {
		avg := totalDur / time.Duration(durCount)
		fmtDur := func(d time.Duration) string {
			if d < time.Minute {
				return fmt.Sprintf("%.1fs", d.Seconds())
			}
			return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
		}
		fmt.Printf("  total: %s  avg: %s\n", fmtDur(totalDur), fmtDur(avg))
	}

	// cost line
	if hasCost {
		costLine := fmt.Sprintf("  cost: $%.6f", totalCost)
		if totalIn > 0 || totalOut > 0 {
			costLine += fmt.Sprintf("  tokens: %s in / %s out",
				formatTokenCount(totalIn), formatTokenCount(totalOut))
		}
		fmt.Println(costLine)
	}
}

func formatDurationTUI(start time.Time, end *time.Time, frame int) string {
	if end == nil {
		spin := "-"
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
		lines = append(lines, "  "+key+":"+" "+val)
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
			lines = append(lines, "  "+"details:")
			for _, l := range prettyJSON(child.Details) {
				lines = append(lines, "    "+truncate(l, width-6))
			}
		}
		return lines
	}

	// Top-level event detail.
	add("kind", ev.Kind)
	add("seq", fmt.Sprintf("%d", ev.Seq))
	add("elapsed", fmt.Sprintf("%.3fs", ev.ElapsedS))
	if ev.DurationMs != nil && *ev.DurationMs > 0 {
		if *ev.DurationMs < 1000 {
			add("duration", fmt.Sprintf("%.0fms", *ev.DurationMs))
		} else {
			add("duration", fmt.Sprintf("%.3fs", *ev.DurationMs/1000))
		}
	}
	add("occurred_at", formatTime(ev.OccurredAt, TimeISO))
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
		lines = append(lines, "  "+fmt.Sprintf("%d children", len(ev.Children)))
	} else if ev.Details != nil && *ev.Details != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+"details:")
		for _, l := range prettyJSON(*ev.Details) {
			lines = append(lines, "    "+truncate(l, width-6))
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
			lines = append(lines, "  "+truncate(l, width-4))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+"input:")
	lines = append(lines, "    "+truncate("$ "+d.Invocation, width-6))
	if d.ExitCode != 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+fmt.Sprintf("exit code: %d", d.ExitCode))
	}
	if d.ErrorMsg != "" {
		lines = append(lines, "  "+"error: "+truncate(d.ErrorMsg, width-10))
	}
	if d.Output != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+"output:")
		for _, l := range strings.Split(strings.TrimRight(d.Output, "\n"), "\n") {
			for _, chunk := range wrapText(l, width-6) {
				lines = append(lines, "    "+chunk)
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
			lines = append(lines, "  "+truncate(l, width-4))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+"name:"+" "+d.Name)
	if d.NameMatchesDir != nil {
		v := "yes"
		if !*d.NameMatchesDir {
			v = "no — mismatch with directory name"
		}
		lines = append(lines, "  "+"name matches dir:"+" "+v)
	}
	if d.Desc != "" {
		lines = append(lines, "")
		lines = append(lines, "  "+"description:")
		for _, chunk := range wrapText(d.Desc, width-6) {
			lines = append(lines, "    "+chunk)
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
			lines = append(lines, "  "+truncate(l, width-4))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+"path:"+" "+truncate(d.Path, width-12))
	if d.Error != "" {
		lines = append(lines, "  "+"error:"+" "+truncate(d.Error, width-12))
		return lines
	}
	tokenVal := "present"
	if !d.HasToken {
		tokenVal = "MISSING"
	}
	lines = append(lines, "  "+"token:"+" "+tokenVal)
	if d.ServerURL != "" {
		lines = append(lines, "  "+"server_url:"+" "+truncate(d.ServerURL, width-18))
	}
	if len(d.Keys) > 0 {
		lines = append(lines, "  "+"keys:"+" "+strings.Join(d.Keys, ", "))
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
			lines = append(lines, "  "+truncate(l, width-4))
		}
		return lines
	}
	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+"agent:"+" "+d.AgentName)
	lines = append(lines, "  "+"run_id:"+" "+d.RunID)
	lines = append(lines, "  "+"input tokens:"+" "+runlog.FormatInt(d.InputTokens))
	lines = append(lines, "  "+"output tokens:"+" "+runlog.FormatInt(d.OutputTokens))
	lines = append(lines, "  "+"cost:"+" "+fmt.Sprintf("$%.6f", d.CostUSD))
	durSec := float64(d.DurationMs) / 1000.0
	lines = append(lines, "  "+"duration:"+" "+fmt.Sprintf("%.2fs", durSec))
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
			strings.Repeat("█", endCol-startCol) +
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
		lines = append(lines, "  (no gantt rows)")
	}
	return lines
}

// ─────────────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
// Shared rendering helpers
// ─────────────────────────────────────────────────────────────────────────────

func statusLabel(r runlog.RunRow) string {
	if r.Skipped {
		return "SKIP"
	}
	if r.Passed == nil {
		if r.FinishedAt != nil {
			return "DEAD" // zombie: finished but outcome not recorded
		}
		return "RUNS" // spinner — still in-flight
	}
	if *r.Passed {
		return "PASS"
	}
	return "FAIL"
}

func kindStyled(kind string) string { //nolint:deadcode
	return kindStyledWithDetails(kind, "")
}

// kindStyledWithDetails returns the styled kind string, using red for "cli"
// events that have a non-zero exit_code in their details JSON.
func kindStyledWithDetails(kind, detailsJSON string) string {
	padded := fmt.Sprintf("%-12s", kind)
	if kind == "cli" && detailsJSON != "" {
		if cliExitCode(detailsJSON) != 0 {
			return padded
		}
		return padded
	}
	switch kind {
	case "section":
		return padded
	case "state_change":
		return padded
	case "metric", "token_summary", "gantt_row":
		return padded
	case "cli":
		return padded
	case "group":
		return padded
	case "trace_span":
		return padded
	case "failure":
		return padded
	default:
		return padded
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

func formatAge(t time.Time) string { //nolint:deadcode
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

func formatDuration(start time.Time, end *time.Time) string { //nolint:deadcode
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

// durationSuffix returns a compact inline string for an event's duration_ms.
// Returns "" when duration is nil or zero.
func durationSuffix(d *float64) string {
	if d == nil || *d <= 0 {
		return ""
	}
	if *d < 1000 {
		return fmt.Sprintf(" %0.fms", *d)
	}
	return fmt.Sprintf(" %0.1fs", *d/1000)
}

func fmtDurationMs(d *float64) string {
	return durationSuffix(d)
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
  runlog failing [flags]                list currently failing tests (streak-sorted)
  runlog stats [flags]                  per-test pass rate, avg duration, run count
  runlog experiments [flags]            list all experiments (non-interactive)
  runlog tests [flags]                  list all known tests with last status
  runlog tests [flags] <test-name>      list recent runs for a specific test
   runlog inspect [flags] <run-id>       full inspector dump of a run (all events + details)
   runlog watch [flags] <run-id>         live-tail a running test's events
  runlog analyze [flags] <run-id>       LLM analysis of a run with full conversation trace
  runlog trace [flags] <run-id>         show stored analysis trace for a run (no LLM call)
  runlog skills install [flags]         install embedded skills into tool directories
  runlog skills list                    list all embedded skills
  runlog test [<profile>] [<filter>]    load .env and run go test (profile = MEMORY_TEST_ENV)
  runlog env list                       list configured environments
  runlog env show <name>                show environment details and status
  runlog env validate <name>            validate all checks for an environment
  runlog lint [<linter-name> ...]       run linters from lefthook.yml or config
  runlog clear [--db <path>]            delete runs.db and all per-run log files
  runlog reap [--dry-run] [<run-id>]    mark stale/orphaned runs as FAIL
  runlog version                        print version and exit

FLAGS
  --db <path>      path to runs.db  (default: auto-resolved to .runlog/runs.db)
  --since <dur>    time window for "runs", "tests", and TUI, e.g. 5m, 1h, 24h  (default: 24h)
  --json           (analyze only) output suggestions as JSON instead of text
  --category <name>   filter by category, exact match (used by "tests")
  --test-type <name>  filter by test type, exact match (used by "tests")

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
  runlog failing                        # currently failing tests, last 24h
  runlog failing --since 7d             # failing tests over the last 7 days
  runlog stats                          # per-test pass rate + duration (last 24h)
  runlog stats --since 7d               # stats over the last 7 days
  runlog experiments                    # table of all experiments
  runlog tests                          # table of all tests with last run status
  runlog tests --since 7d              # tests with runs from last 7 days
  runlog tests --category integration   # tests in the "integration" category
  runlog tests --test-type unit         # tests classified as test_type "unit"
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
	// Support "Nd" shorthand for N days (e.g. "7d" = 168h).
	if strings.HasSuffix(val, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(val, "d"))
		if err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
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
	// ── Internal daemon mode ───────────────────────────────────────────────
	// When the daemon spawns itself via re-exec, it passes --daemon as the
	// first argument.  Handle this before any other flag or subcommand logic.
	if len(os.Args) > 1 && os.Args[1] == "--daemon" {
		if err := runDaemonInternal(os.Args[2:]); err != nil {
			log.Fatalf("runlog daemon: %v", err)
		}
		return
	}

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
	var analyzeJSON *bool                    // set by "analyze" subcommand
	var reapDryRun *bool                     // set by "reap" subcommand
	var testsCategory, testsTestType *string // set by "tests" subcommand

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

	case "failing", "stats":
		fs, dbOut, sinceOut := subFS(subcommand, *globalDB, *globalSince)
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)

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
		testsCategory = fs.String("category", "", "filter by category (exact match)")
		testsTestType = fs.String("test-type", "", "filter by test type (exact match)")
		if err := fs.Parse(subArgs); err != nil {
			if err == flag.ErrHelp {
				os.Exit(0)
			}
			os.Exit(2)
		}
		dbPath = resolveDBPath(*dbOut)
		since = parseSince(*sinceOut, subcommand)
		subArgs = fs.Args() // optional positional arg: test name for run listing

	case "inspect", "watch":
		// --since is not meaningful for inspect/watch but accept it silently.
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

	case "env":
		// env shows/validates environment config; does not need DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")

	case "lint":
		// lint runs lefthook-discovered linters; does not need DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")

	case "test":
		// test loads .env and execs go test; does not need DB.
		dbPath = resolveDBPath(*globalDB)
		since = parseSince(*globalSince, "")

	case "daemon", "cleanup":
		// daemon and cleanup communicate with the local daemon; do not need DB directly.
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

	if subcommand == "env" {
		if err := cmdEnv(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog env: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if subcommand == "lint" {
		if err := cmdLint(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog lint: %v\n", err)
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

	if subcommand == "daemon" {
		if err := cmdDaemon(subArgs, dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "runlog daemon: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if subcommand == "cleanup" {
		if err := cmdCleanup(subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog cleanup: %v\n", err)
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

	case "failing":
		if err := cmdFailing(db, since); err != nil {
			fmt.Fprintf(os.Stderr, "runlog failing: %v\n", err)
			os.Exit(1)
		}

	case "stats":
		if err := cmdStats(db, since); err != nil {
			fmt.Fprintf(os.Stderr, "runlog stats: %v\n", err)
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
			if err := cmdTestsList(db, since, *testsCategory, *testsTestType); err != nil {
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

	case "watch":
		if len(subArgs) < 1 {
			fmt.Fprintf(os.Stderr, "runlog watch: missing <run-id>\n")
			fmt.Fprintf(os.Stderr, "usage: runlog watch <run-id>\n")
			os.Exit(2)
		}
		_, err := strconv.ParseInt(subArgs[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "runlog watch: invalid run-id %q: %v\n", subArgs[0], err)
			os.Exit(2)
		}
		if err := cmdWatch(db, subArgs); err != nil {
			fmt.Fprintf(os.Stderr, "runlog watch: %v\n", err)
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
		fmt.Println("runlog: no subcommand specified. Use --help for usage.")
		usage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "runlog: unknown subcommand %q\n\n", subcommand)
		usage()
		os.Exit(2)
	}
}
