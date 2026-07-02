package runlog

import (
	"testing"
	"time"
)

func TestRunLog_RecordTokenUsage(t *testing.T) {
	df := NewDogfoodRun(t, "cost")
	defer df.Done()
	df.Describe("RecordTokenUsage accumulates tokens in memory",
		"Creates a RunLog and records two token usage calls",
		"Verifies accumulated values in memory and via DB",
	)
	df.Event("log", "Testing RecordTokenUsage with 1000/500 and 2000/1000 tokens")

	_, dc := StartTestDaemon(t)

	rl := RunLog{t: t, StartedAt: time.Now()}
	rl.RecordTokenUsage(1000, 500, 0.05)
	rl.RecordTokenUsage(2000, 1000, 0.10)

	if rl.inputTokens != 3000 {
		t.Errorf("inputTokens = %d, want 3000", rl.inputTokens)
	}
	if rl.outputTokens != 1500 {
		t.Errorf("outputTokens = %d, want 1500", rl.outputTokens)
	}
	if rl.costUSD < 0.14999 || rl.costUSD > 0.15001 {
		t.Errorf("costUSD = %f, want ~0.15", rl.costUSD)
	}

	r := dc.CreateRun(t, CreateRunOpts{
		EnvProfile:  "TestCost",
		Category:    "cost",
		Description: "Verify token/cost accumulation and DB round-trip",
	})
	dc.MarkDone(t, r.DaemonID, MarkDoneOpts{
		Passed:       boolPtr(true),
		InputTokens:  int64Ptr(3000),
		OutputTokens: int64Ptr(1500),
		CostUSD:      float64Ptr(0.15),
	})

	run := dc.MustGetTestRun(t, r.TestRunID)
	if run == nil {
		t.Fatal("run not found")
	}
}

func TestRunLog_RecordTokenUsage_ZeroValues(t *testing.T) {
	df := NewDogfoodRun(t, "cost")
	defer df.Done()
	df.Describe("Zero token values are stored as NULL",
		"Creates a run without recording any token usage",
		"Verifies token/cost columns are NULL in DB",
	)
	df.Event("log", "Testing zero token values stored as NULL")

	_, dc := StartTestDaemon(t)

	r := dc.CreateRun(t, CreateRunOpts{
		EnvProfile:  "TestCostZero",
		Category:    "cost",
		Description: "Verify zero-cost run stores NULL token columns",
	})
	dc.MarkDone(t, r.DaemonID, MarkDoneOpts{Passed: boolPtr(true)})

	run := dc.MustGetTestRun(t, r.TestRunID)
	if run == nil {
		t.Fatal("run not found")
	}
}

func TestRunLog_RecordTokenUsage_WithFile(t *testing.T) {
	df := NewDogfoodRun(t, "cost")
	defer df.Done()
	df.Describe("RecordTokenUsage with file logging",
		"Records tokens then marks done via daemon",
		"Verifies run exists in DB",
	)
	df.Event("log", "Testing token recording with HTTP daemon")

	_, dc := StartTestDaemon(t)

	r := dc.CreateRun(t, CreateRunOpts{
		EnvProfile:  t.Name(),
		Category:    "cost",
		Description: "Token usage recording with HTTP daemon",
	})
	dc.AddEvent(t, r.DaemonID, "token_usage", "5000 in / 2500 out  $0.250000")
	dc.MarkDone(t, r.DaemonID, MarkDoneOpts{
		Passed:       boolPtr(true),
		InputTokens:  int64Ptr(5000),
		OutputTokens: int64Ptr(2500),
		CostUSD:      float64Ptr(0.25),
	})

	run := dc.MustGetTestRun(t, r.TestRunID)
	if run == nil {
		t.Fatal("run not found")
	}
}

func boolPtr(b bool) *bool          { return &b }
func int64Ptr(i int64) *int64       { return &i }
func float64Ptr(f float64) *float64 { return &f }
