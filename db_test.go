package runlog

import (
	"database/sql"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *RunDB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir + "/test.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunDB_InsertRun(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("basic insert", func(t *testing.T) {
		id, err := db.InsertRun("TestFoo", now, "host", "test-env", nil, "")
		if err != nil {
			t.Fatalf("InsertRun: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive id, got %d", id)
		}
	})

	t.Run("with env vars", func(t *testing.T) {
		env := map[string]string{"KEY": "VALUE", "API_KEY": "secret"}
		id, err := db.InsertRun("TestEnv", now, "runner", "env-name", env, "")
		if err != nil {
			t.Fatalf("InsertRun: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		var found bool
		for _, r := range runs {
			if r.ID == id {
				found = true
				if len(r.EnvVars) != 2 {
					t.Errorf("want 2 env vars, got %d", len(r.EnvVars))
				}
				if r.EnvVars["KEY"] != "VALUE" {
					t.Errorf("want KEY=VALUE, got %q", r.EnvVars["KEY"])
				}
				break
			}
		}
		if !found {
			t.Error("run not found in ListRuns")
		}
	})

	t.Run("empty runner and env name", func(t *testing.T) {
		id, err := db.InsertRun("TestEmpty", now, "", "", nil, "")
		if err != nil {
			t.Fatalf("InsertRun: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive id, got %d", id)
		}
	})
}

func TestRunDB_FinishRun(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	id, _ := db.InsertRun("TestFinish", now, "host", "env", nil, "")

	t.Run("finish as pass", func(t *testing.T) {
		finish := now.Add(5 * time.Second)
		if err := db.FinishRun(id, finish, OutcomePass, ""); err != nil {
			t.Fatalf("FinishRun: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		for _, r := range runs {
			if r.ID == id {
				if r.FinishedAt == nil {
					t.Error("FinishedAt should be set")
				}
				if r.Passed == nil || !*r.Passed {
					t.Error("Passed should be true")
				}
				return
			}
		}
		t.Error("run not found")
	})
}

func TestRunDB_FinishRunWithCost(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	passID, _ := db.InsertRun("TestCostPass", now, "host", "env", nil, "")
	failID, _ := db.InsertRun("TestCostFail", now, "host", "env", nil, "")
	skipID, _ := db.InsertRun("TestCostSkip", now, "host", "env", nil, "")
	timeoutID, _ := db.InsertRun("TestCostTimeout", now, "host", "env", nil, "")

	t.Run("pass with tokens and cost", func(t *testing.T) {
		finish := now.Add(10 * time.Second)
		err := db.FinishRunWithCost(passID, finish, OutcomePass, "", 1000, 500, 0.05)
		if err != nil {
			t.Fatalf("FinishRunWithCost: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		for _, r := range runs {
			if r.ID == passID {
				if r.InputTokens == nil || *r.InputTokens != 1000 {
					t.Errorf("want 1000 input tokens, got %v", r.InputTokens)
				}
				if r.OutputTokens == nil || *r.OutputTokens != 500 {
					t.Errorf("want 500 output tokens, got %v", r.OutputTokens)
				}
				if r.CostUSD == nil || *r.CostUSD != 0.05 {
					t.Errorf("want 0.05 cost, got %v", r.CostUSD)
				}
				if r.Passed == nil || !*r.Passed {
					t.Error("Passed should be true")
				}
				return
			}
		}
	})

	t.Run("fail outcome", func(t *testing.T) {
		err := db.FinishRunWithCost(failID, now.Add(2*time.Second), OutcomeFail, "test failed", 0, 0, 0)
		if err != nil {
			t.Fatalf("FinishRunWithCost: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		for _, r := range runs {
			if r.ID == failID {
				if r.Passed != nil && *r.Passed {
					t.Error("Passed should be false for fail outcome")
				}
				if r.Reason == nil || *r.Reason != "test failed" {
					t.Errorf("want reason='test failed', got %v", r.Reason)
				}
				return
			}
		}
	})

	t.Run("skip outcome", func(t *testing.T) {
		err := db.FinishRunWithCost(skipID, now.Add(1*time.Second), OutcomeSkip, "not applicable", 0, 0, 0)
		if err != nil {
			t.Fatalf("FinishRunWithCost: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		for _, r := range runs {
			if r.ID == skipID {
				if !r.Skipped {
					t.Error("Skipped should be true")
				}
				if r.Reason == nil || *r.Reason != "not applicable" {
					t.Errorf("want reason='not applicable', got %v", r.Reason)
				}
				return
			}
		}
	})

	t.Run("timeout outcome", func(t *testing.T) {
		err := db.FinishRunWithCost(timeoutID, now.Add(30*time.Second), OutcomeTimeout, "timed out", 0, 0, 0)
		if err != nil {
			t.Fatalf("FinishRunWithCost: %v", err)
		}
		runs, _ := db.ListRuns(time.Time{}, 0)
		for _, r := range runs {
			if r.ID == timeoutID {
				if r.Passed != nil && *r.Passed {
					t.Error("Passed should be false for timeout")
				}
				if r.Reason == nil || *r.Reason != "timed out" {
					t.Errorf("want reason='timed out', got %v", r.Reason)
				}
				return
			}
		}
	})
}

func TestRunDB_InsertEvent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	id, _ := db.InsertRun("TestEvents", now, "host", "env", nil, "")

	t.Run("insert basic event", func(t *testing.T) {
		err := db.InsertEvent(id, 1, now, 0.5, "log", "hello world", nil)
		if err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
		events, err := db.ListEvents(id)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("want 1 event, got %d", len(events))
		}
		if events[0].Kind != "log" {
			t.Errorf("want kind='log', got %q", events[0].Kind)
		}
		if events[0].Message != "hello world" {
			t.Errorf("want message='hello world', got %q", events[0].Message)
		}
		if events[0].Seq != 1 {
			t.Errorf("want seq=1, got %d", events[0].Seq)
		}
	})

	t.Run("sequential seq numbers", func(t *testing.T) {
		err := db.InsertEvent(id, 2, now, 0.3, "state_change", "started", nil)
		if err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
		err = db.InsertEvent(id, 3, now, 1.0, "cli", "go build", nil)
		if err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
		events, _ := db.ListEvents(id)
		if len(events) != 3 {
			t.Fatalf("want 3 events, got %d", len(events))
		}
	})

	t.Run("event with details JSON", func(t *testing.T) {
		details := map[string]any{"key": "value", "count": 42}
		err := db.InsertEvent(id, 4, now, 0.1, "http_call", "GET /health", details)
		if err != nil {
			t.Fatalf("InsertEvent with details: %v", err)
		}
		var detailsStr sql.NullString
		db.RawDB().QueryRow(`SELECT details FROM run_events WHERE run_id=? AND seq=4`, id).Scan(&detailsStr)
		if !detailsStr.Valid {
			t.Fatal("details should be set")
		}
		if detailsStr.String == "{}" {
			t.Error("details should not be empty JSON object")
		}
	})
}

func TestRunDB_InsertGroupEvent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	id, _ := db.InsertRun("TestGroup", now, "host", "env", nil, "")

	parentID, err := db.InsertGroupEvent(id, 1, now, 0.5, "section", "Setup")
	if err != nil {
		t.Fatalf("InsertGroupEvent: %v", err)
	}
	if parentID <= 0 {
		t.Errorf("expected positive parentID, got %d", parentID)
	}

	err = db.AppendGroupChildren(parentID, []ChildEvent{
		{Kind: "log", Message: "step 1", ElapsedS: 0.1},
		{Kind: "log", Message: "step 2", ElapsedS: 0.2},
	})
	if err != nil {
		t.Fatalf("AppendGroupChildren: %v", err)
	}

	events, _ := db.ListEvents(id)
	if len(events) == 0 {
		t.Fatal("expected at least 1 event")
	}
	found := false
	for _, e := range events {
		if e.Kind == "section" && e.Message == "Setup" {
			found = true
			if len(e.Children) == 0 {
				t.Error("expected children on section event")
			}
			break
		}
	}
	if !found {
		t.Error("section event not found")
	}
}

func TestRunDB_HasSkipEvent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	id, _ := db.InsertRun("TestSkipCheck", now, "host", "env", nil, "")

	t.Run("no skip event returns false", func(t *testing.T) {
		if db.HasSkipEvent(id) {
			t.Error("expected false for no skip event")
		}
	})

	t.Run("skip event returns true", func(t *testing.T) {
		db.InsertEvent(id, 1, now, 0, "skip", "not applicable", nil)
		if !db.HasSkipEvent(id) {
			t.Error("expected true after inserting skip event")
		}
	})
}

func TestRunDB_UpdateMetadata(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	id, _ := db.InsertRun("TestMeta", now, "host", "env", nil, "")

	t.Run("category", func(t *testing.T) {
		if err := db.UpdateRunCategory(id, "unit"); err != nil {
			t.Fatalf("UpdateRunCategory: %v", err)
		}
	})

	t.Run("experiment", func(t *testing.T) {
		if err := db.UpdateRunExperiment(id, "baseline-v2"); err != nil {
			t.Fatalf("UpdateRunExperiment: %v", err)
		}
	})

	t.Run("tags", func(t *testing.T) {
		if err := db.UpdateRunTags(id, []string{"fast", "critical"}); err != nil {
			t.Fatalf("UpdateRunTags: %v", err)
		}
	})

	t.Run("description", func(t *testing.T) {
		desc := RunDescription{Summary: "test run", Bullets: []string{"step 1", "step 2"}}
		if err := db.UpdateRunDescription(id, desc); err != nil {
			t.Fatalf("UpdateRunDescription: %v", err)
		}
	})

	t.Run("app version", func(t *testing.T) {
		if err := db.UpdateRunAppVersion(id, "1.0.0"); err != nil {
			t.Fatalf("UpdateRunAppVersion: %v", err)
		}
	})

	t.Run("test version", func(t *testing.T) {
		if err := db.UpdateRunTestVersion(id, "abc123"); err != nil {
			t.Fatalf("UpdateRunTestVersion: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		if err := db.UpdateRunTimeout(id, 300.0); err != nil {
			t.Fatalf("UpdateRunTimeout: %v", err)
		}
	})
}

func TestRunDB_ListRuns(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("empty DB returns empty slice", func(t *testing.T) {
		runs, err := db.ListRuns(time.Time{}, 0)
		if err != nil {
			t.Fatalf("ListRuns: %v", err)
		}
		if len(runs) != 0 {
			t.Errorf("want 0 runs, got %d", len(runs))
		}
	})

	t.Run("orders by started_at DESC", func(t *testing.T) {
		t1 := now.Add(-2 * time.Hour)
		t2 := now.Add(-1 * time.Hour)
		db.InsertRun("Old", t1, "host", "", nil, "")
		db.InsertRun("New", t2, "host", "", nil, "")

		runs, _ := db.ListRuns(time.Time{}, 0)
		if len(runs) < 2 {
			t.Fatal("expected at least 2 runs")
		}
		if runs[0].TestName != "New" {
			t.Errorf("want first='New', got %q", runs[0].TestName)
		}
	})

	t.Run("since filter", func(t *testing.T) {
		old := now.Add(-48 * time.Hour)
		db.InsertRun("48hAgo", old, "host", "", nil, "")

		since := now.Add(-24 * time.Hour)
		runs, _ := db.ListRuns(since, 0)
		for _, r := range runs {
			if r.TestName == "48hAgo" {
				t.Error("48hAgo should not appear in last-24h query")
			}
		}
	})

	t.Run("limit", func(t *testing.T) {
		runs, _ := db.ListRuns(time.Time{}, 1)
		if len(runs) > 1 {
			t.Errorf("want at most 1 run with limit=1, got %d", len(runs))
		}
	})
}

func TestRunDB_DiscoverTests(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("empty DB returns empty", func(t *testing.T) {
		names, err := db.DiscoverTests()
		if err != nil {
			t.Fatalf("DiscoverTests: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("want 0 tests, got %d", len(names))
		}
	})

	t.Run("returns distinct test names", func(t *testing.T) {
		db.InsertRun("TestA", now, "host", "", nil, "")
		db.InsertRun("TestA", now, "host", "", nil, "")
		db.InsertRun("TestB", now, "host", "", nil, "")

		names, _ := db.DiscoverTests()
		if len(names) != 2 {
			t.Errorf("want 2 distinct tests, got %d: %v", len(names), names)
		}
	})
}

func TestRunDB_StaleRuns(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("no stale runs", func(t *testing.T) {
		stale, err := db.ListStaleRuns()
		if err != nil {
			t.Fatalf("ListStaleRuns: %v", err)
		}
		if len(stale) != 0 {
			t.Errorf("want 0 stale, got %d", len(stale))
		}
	})

	t.Run("reap reaps only unfinished runs", func(t *testing.T) {
		fresh, _ := db.InsertRun("Fresh", now, "host", "", nil, "")
		old, _ := db.InsertRun("Old", now.Add(-2*time.Hour), "host", "", nil, "")
		db.FinishRun(fresh, now, OutcomePass, "")
		db.FinishRun(old, now.Add(-1*time.Hour), OutcomePass, "")

		db.InsertRun("Stale", now.Add(-2*time.Hour), "host", "", nil, "")
		db.InsertRun("Stale2", now.Add(-1*time.Hour), "host", "", nil, "")

		reaped, err := db.ReapStaleRuns("timeout")
		if err != nil {
			t.Fatalf("ReapStaleRuns: %v", err)
		}
		if reaped != 2 {
			t.Errorf("want 2 reaped runs, got %d", reaped)
		}
	})
}

func TestRunDB_ListFailingTests(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("empty DB returns empty", func(t *testing.T) {
		failing, err := db.ListFailingTests(time.Time{})
		if err != nil {
			t.Fatalf("ListFailingTests: %v", err)
		}
		if len(failing) != 0 {
			t.Errorf("want 0 failing, got %d", len(failing))
		}
	})

	t.Run("detects failing tests", func(t *testing.T) {
		failID, _ := db.InsertRun("TestFlaky", now, "host", "", nil, "")
		db.FinishRunWithCost(failID, now.Add(5*time.Second), OutcomeFail, "assertion error", 0, 0, 0)

		failing, _ := db.ListFailingTests(time.Time{})
		var found bool
		for _, f := range failing {
			if f.TestName == "TestFlaky" {
				found = true
				break
			}
		}
		if !found {
			t.Error("TestFlaky should be in failing tests list")
		}
	})

	t.Run("passing test not in failing list", func(t *testing.T) {
		passID, _ := db.InsertRun("TestStable", now, "host", "", nil, "")
		db.FinishRunWithCost(passID, now.Add(3*time.Second), OutcomePass, "", 0, 0, 0)

		failing, _ := db.ListFailingTests(time.Time{})
		for _, f := range failing {
			if f.TestName == "TestStable" {
				t.Error("TestStable should not be in failing tests list")
			}
		}
	})
}

func TestRunDB_ListExperiments(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("empty DB returns empty", func(t *testing.T) {
		exps, err := db.ListExperiments()
		if err != nil {
			t.Fatalf("ListExperiments: %v", err)
		}
		if len(exps) != 0 {
			t.Errorf("want 0 experiments, got %d", len(exps))
		}
	})

	t.Run("returns experiments with run counts", func(t *testing.T) {
		id, _ := db.InsertRun("TestExp1", now, "host", "", nil, "")
		db.UpdateRunExperiment(id, "baseline")
		db.FinishRun(id, now.Add(5*time.Second), OutcomePass, "")

		id2, _ := db.InsertRun("TestExp2", now, "host", "", nil, "")
		db.UpdateRunExperiment(id2, "canary")

		exps, _ := db.ListExperiments()
		if len(exps) == 0 {
			t.Fatal("expected at least 1 experiment")
		}
		var found bool
		for _, e := range exps {
			if e.Name == "baseline" {
				found = true
				if e.RunCount < 1 {
					t.Errorf("want RunCount >= 1, got %d", e.RunCount)
				}
				break
			}
		}
		if !found {
			t.Error("'baseline' experiment not found")
		}
	})
}

func TestRunDB_Path(t *testing.T) {
	db := openTestDB(t)
	path := db.Path()
	if path == "" {
		t.Error("Path() should not return empty string")
	}
}

func TestRunDB_RawDB(t *testing.T) {
	db := openTestDB(t)
	raw := db.RawDB()
	if raw == nil {
		t.Error("RawDB() should not return nil")
	}
}

func TestRunDB_Close(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRunDB_NonExistentRun(t *testing.T) {
	db := openTestDB(t)
	// FinishRun on non-existent ID is idempotent (no error, no-op).
	err := db.FinishRun(99999, time.Now(), OutcomePass, "")
	if err != nil {
		t.Errorf("expected no error for non-existent run, got %v", err)
	}
}
