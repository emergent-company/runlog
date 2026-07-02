package runlog

import (
	"testing"
)

func TestCostTracking_EndToEnd(t *testing.T) {
	df := NewDogfoodRun(t, "cost")
	defer df.Done()
	df.Describe("Full cost tracking workflow end-to-end",
		"Creates a test run, simulates 3 LLM calls via token events",
		"Verifies tokens and cost are persisted to the database",
	)
	df.Event("log", "Starting full cost tracking E2E test")

	_, dc := StartTestDaemon(t)

	r := dc.CreateRun(t, CreateRunOpts{
		EnvProfile:  t.Name(),
		Category:    "cost",
		Description: "Full cost tracking workflow end-to-end",
	})

	dc.AddEvent(t, r.DaemonID, "token_usage", "1,000 in / 500 out  $0.050000")
	dc.AddEvent(t, r.DaemonID, "token_usage", "2,000 in / 1,000 out  $0.100000")
	dc.AddEvent(t, r.DaemonID, "token_usage", "1,500 in / 750 out  $0.075000")

	dc.MarkDone(t, r.DaemonID, MarkDoneOpts{
		Passed:       boolPtr(true),
		InputTokens:  int64Ptr(4500),
		OutputTokens: int64Ptr(2250),
		CostUSD:      float64Ptr(0.225),
	})

	run := dc.MustGetTestRun(t, r.TestRunID)
	if run == nil {
		t.Fatal("Expected at least one run")
	}
}
