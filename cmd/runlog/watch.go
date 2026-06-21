package main

import (
	"fmt"
	"strconv"
	"time"

	runlog "github.com/emergent-company/runlog"
)

var (
	ansiGreen   = "\x1b[32m"
	ansiRed     = "\x1b[31m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiReset   = "\x1b[0m"
)

func cmdWatch(db *runlog.RunDB, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: runlog watch <run-id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid run id: %s", args[0])
	}

	rawDB := db.RawDB()
	run := fetchRunByID(rawDB, id)
	if run == nil {
		return fmt.Errorf("run %d not found", id)
	}

	fmt.Printf("%s Watching run %d — %s %s\n", ansiBlue+ansiBold+"●"+ansiReset, id, run.TestName, ansiReset)
	if run.StartedAt != (time.Time{}) {
		fmt.Printf("  started %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if run.EnvName != nil && *run.EnvName != "" {
		fmt.Printf("  env     %s\n", *run.EnvName)
	}
	fmt.Println()

	seenSeq := 0

	if run.FinishedAt != nil {
		emitEvents(db, id, &seenSeq)
		fmt.Println()
		printResult(run)
		return exitCode(run)
	}

	for {
		emitEvents(db, id, &seenSeq)
		run = fetchRunByID(rawDB, id)
		if run.FinishedAt != nil {
			emitEvents(db, id, &seenSeq)
			fmt.Println()
			printResult(run)
			return exitCode(run)
		}
		time.Sleep(2 * time.Second)
	}
}

func emitEvents(db *runlog.RunDB, runID int64, seen *int) {
	events, err := db.ListEvents(runID)
	if err != nil || len(events) == 0 {
		return
	}
	for _, e := range events {
		if e.Seq <= *seen {
			continue
		}
		printEvent(e)
		*seen = e.Seq
	}
}

func printEvent(e runlog.EventRow) {
	icon := eventIcon(e.Kind)
	kind := colorKind(e.Kind)
	msg := truncateStr(e.Message, 80)
	fmt.Printf("  %s [%6.1fs] %s  %s\n", icon, e.ElapsedS, kind, msg)

	for _, c := range e.Children {
		cicon := eventIcon(c.Kind)
		ckind := colorKind(c.Kind)
		cmsg := truncateStr(c.Message, 76)
		fmt.Printf("    %s [%6.1fs] %s  %s\n", cicon, c.ElapsedS, ckind, cmsg)
		if c.Details != "" && c.Details != "{}" {
			for _, l := range prettyJSON(c.Details) {
				fmt.Printf("      %s%s%s\n", ansiDim, l, ansiReset)
			}
		}
	}

	if e.Details != nil && *e.Details != "" && *e.Details != "{}" && len(e.Children) == 0 {
		for _, l := range prettyJSON(*e.Details) {
			fmt.Printf("    %s%s%s\n", ansiDim, l, ansiReset)
		}
	}
}

func printResult(r *runlog.RunRow) {
	var icon, color, label string
	if r.Skipped {
		icon, color, label = "⊘", ansiYellow, "Skip"
	} else if r.Passed != nil && *r.Passed {
		icon, color, label = "✔", ansiGreen, "Pass"
	} else if r.Reason != nil && *r.Reason == "timed out" {
		icon, color, label = "✘", ansiRed, "Timeout"
	} else {
		icon, color, label = "✘", ansiRed, "Fail"
	}
	dur := "—"
	if r.FinishedAt != nil {
		d := r.FinishedAt.Sub(r.StartedAt)
		if d < time.Minute {
			dur = fmt.Sprintf("%.1fs", d.Seconds())
		} else {
			dur = fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
		}
	}
	fmt.Printf("  %s%s %s%s  (%s)\n", color, icon, label, ansiReset, dur)
}

func eventIcon(kind string) string {
	switch kind {
	case "state_change":
		return ansiBlue + "●" + ansiReset
	case "section":
		return ansiCyan + ansiBold + "▶" + ansiReset
	case "cli":
		return ansiYellow + "·" + ansiReset
	case "failure":
		return ansiRed + "✗" + ansiReset
	case "skip":
		return ansiYellow + "⊘" + ansiReset
	case "http_call":
		return ansiMagenta + "·" + ansiReset
	case "gantt", "gantt_row", "metric", "token_usage", "token_summary":
		return ansiGreen + "◆" + ansiReset
	case "skill":
		return ansiMagenta + "◆" + ansiReset
	default:
		return "·"
	}
}

func colorKind(kind string) string {
	var c string
	switch kind {
	case "state_change":
		c = ansiBlue
	case "section":
		c = ansiCyan + ansiBold
	case "cli":
		c = ansiYellow
	case "failure", "skip":
		c = ansiRed
	case "http_call":
		c = ansiMagenta
	case "gantt", "gantt_row", "metric", "token_usage", "token_summary":
		c = ansiGreen
	case "skill":
		c = ansiMagenta
	}
	return c + fmt.Sprintf("%-14s", kind) + ansiReset
}

func exitCode(r *runlog.RunRow) error {
	if r.Skipped || (r.Passed != nil && *r.Passed) {
		return nil
	}
	return fmt.Errorf("run %d: %s", r.ID, statusText(r))
}

func statusText(r *runlog.RunRow) string {
	if r.FinishedAt == nil {
		return "running"
	}
	if r.Skipped {
		return "skip"
	}
	if r.Passed != nil && *r.Passed {
		return "pass"
	}
	if r.Reason != nil && *r.Reason == "timed out" {
		return "timed out"
	}
	return "fail"
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}

// prettyJSON is defined in main.go (same package).

