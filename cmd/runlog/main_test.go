package main

import (
	"fmt"
	"os"
	"testing"

	runlog "github.com/emergent-company/runlog"
)

func TestMain(m *testing.M) {
	shellURL := os.Getenv("RUNLOG_DAEMON_URL")

	runlog.LoadDotEnvFrom(".")
	runlog.LoadEnvFile(".env.local", true)

	fileURL := os.Getenv("RUNLOG_DAEMON_URL")

	if shellURL != "" && shellURL != fileURL {
		os.Setenv("RUNLOG_DAEMON_URL", shellURL)
		fmt.Fprintf(os.Stderr, "⚠ RUNLOG_DAEMON_URL overridden by env var: %s (file had: %s)\n", shellURL, fileURL)
	} else if shellURL == "" && fileURL == "" {
		if os.Getenv("RUNLOG_SKIP_DAEMON") == "1" {
			fmt.Fprintln(os.Stderr, "⚠ RUNLOG_DAEMON_URL not set — skipping daemon, tests run unrecorded")
		} else {
			fmt.Fprintln(os.Stderr, "✖ RUNLOG_DAEMON_URL not set. Set in .env.local or use RUNLOG_SKIP_DAEMON=1 to skip.")
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
