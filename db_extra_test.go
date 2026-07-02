package runlog

import (
	"testing"
	"time"
)

func TestRunDB_Suggestions(t *testing.T) {
	db := openTestDB(t)

	t.Run("empty DB returns empty", func(t *testing.T) {
		list, err := db.ListSuggestions("test")
		if err != nil {
			t.Fatalf("ListSuggestions: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("want 0, got %d", len(list))
		}
	})

	t.Run("insert and list", func(t *testing.T) {
		id, err := db.InsertSuggestion("exp1", "Fix timeout", "Increase timeout to 30s", "performance", "high", nil)
		if err != nil {
			t.Fatalf("InsertSuggestion: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive id, got %d", id)
		}

		list, _ := db.ListSuggestions("exp1")
		if len(list) != 1 {
			t.Fatalf("want 1, got %d", len(list))
		}
		if list[0].Title != "Fix timeout" {
			t.Errorf("want title='Fix timeout', got %q", list[0].Title)
		}
		if list[0].Experiment != "exp1" {
			t.Errorf("want experiment='exp1', got %q", list[0].Experiment)
		}
	})

	t.Run("scoped to experiment", func(t *testing.T) {
		db.InsertSuggestion("exp2", "Other", "body", "category", "low", nil)
		db.InsertSuggestion("exp1", "Second", "body", "category", "low", nil)

		list, _ := db.ListSuggestions("exp1")
		if len(list) != 2 {
			t.Errorf("want 2 for exp1, got %d", len(list))
		}
	})

	t.Run("delete", func(t *testing.T) {
		err := db.DeleteSuggestions("exp2")
		if err != nil {
			t.Fatalf("DeleteSuggestions: %v", err)
		}
		list, _ := db.ListSuggestions("exp2")
		if len(list) != 0 {
			t.Errorf("want 0 after delete, got %d", len(list))
		}
	})
}

func TestRunDB_AnalyzerTraces(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("insert and finish trace", func(t *testing.T) {
		traceID, err := db.InsertAnalyzerTrace("sug-1", nil)
		if err != nil {
			t.Fatalf("InsertAnalyzerTrace: %v", err)
		}
		if traceID <= 0 {
			t.Errorf("expected positive traceID, got %d", traceID)
		}

		err = db.FinishAnalyzerTrace(traceID)
		if err != nil {
			t.Fatalf("FinishAnalyzerTrace: %v", err)
		}
	})

	t.Run("trace events", func(t *testing.T) {
		runID, _ := db.InsertRun("TraceTest", now, "host", "", nil, "")
		traceID, _ := db.InsertAnalyzerTrace("sug-2", &runID)

		err := db.InsertAnalyzerTraceEvent(traceID, 1, AnalyzerEvent{
			Kind:    AEThought,
			Author:  "agent-1",
			Content: "starting analysis",
		})
		if err != nil {
			t.Fatalf("InsertAnalyzerTraceEvent: %v", err)
		}

		err = db.InsertAnalyzerTraceEvent(traceID, 2, AnalyzerEvent{
			Kind:    AEThought,
			Author:  "agent-1",
			Content: "analysis done",
		})
		if err != nil {
			t.Fatalf("InsertAnalyzerTraceEvent: %v", err)
		}

		traces, _ := db.ListTracesForRun(runID)
		if len(traces) == 0 {
			t.Fatal("expected at least 1 trace for run")
		}

		events, err := db.ListAnalyzerTraceEvents(traceID)
		if err != nil {
			t.Fatalf("ListAnalyzerTraceEvents: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("want 2 events, got %d", len(events))
		}
	})

	t.Run("delete traces", func(t *testing.T) {
		err := db.DeleteAnalyzerTraces("sug-1")
		if err != nil {
			t.Fatalf("DeleteAnalyzerTraces: %v", err)
		}
	})
}

func TestRunDB_Launchers(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("insert and list", func(t *testing.T) {
		id, err := db.InsertLauncher("TestLauncher", "staging", now, 12345)
		if err != nil {
			t.Fatalf("InsertLauncher: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive id, got %d", id)
		}

		list, _ := db.ListLaunchers("TestLauncher")
		if len(list) != 1 {
			t.Fatalf("want 1, got %d", len(list))
		}
		if list[0].LauncherPID != 12345 {
			t.Errorf("want LauncherPID=12345, got %d", list[0].LauncherPID)
		}
	})

	t.Run("finish launcher", func(t *testing.T) {
		id, _ := db.InsertLauncher("TestFinish", "prod", now, 54321)
		err := db.FinishLauncher(id, now.Add(5*time.Minute))
		if err != nil {
			t.Fatalf("FinishLauncher: %v", err)
		}
	})

	t.Run("empty list for unknown test", func(t *testing.T) {
		list, _ := db.ListLaunchers("NonExistent")
		if len(list) != 0 {
			t.Errorf("want 0 for unknown test, got %d", len(list))
		}
	})
}

func TestRunDB_Linters(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()

	t.Run("insert and list", func(t *testing.T) {
		id, err := db.InsertLinterRun("gofmt", "gofmt -l .", now)
		if err != nil {
			t.Fatalf("InsertLinterRun: %v", err)
		}
		if id <= 0 {
			t.Errorf("expected positive id, got %d", id)
		}

		runs, _ := db.ListLinterRuns()
		if len(runs) != 1 {
			t.Fatalf("want 1, got %d", len(runs))
		}
		if runs[0].LinterName != "gofmt" {
			t.Errorf("want linter='gofmt', got %q", runs[0].LinterName)
		}
	})

	t.Run("update result", func(t *testing.T) {
		id, _ := db.InsertLinterRun("govet", "go vet ./...", now)
		err := db.UpdateLinterRunResult(id, "passed", 0, "no issues", now.Add(10*time.Second))
		if err != nil {
			t.Fatalf("UpdateLinterRunResult: %v", err)
		}

		runs, _ := db.ListLinterRuns()
		for _, r := range runs {
			if r.ID == id {
				if r.Status != "passed" {
					t.Errorf("want status='passed', got %q", r.Status)
				}
				if r.ExitCode == nil || *r.ExitCode != 0 {
					t.Errorf("want exitCode=0, got %d", r.ExitCode)
				}
				return
			}
		}
		t.Error("linter run not found after update")
	})

	t.Run("run history", func(t *testing.T) {
		db.InsertLinterRun("gofmt", "gofmt -l .", now)
		db.InsertLinterRun("gofmt", "gofmt -l .", now)

		runs, total, err := db.ListLinterRunHistory("gofmt", 0, 10)
		if err != nil {
			t.Fatalf("ListLinterRunHistory: %v", err)
		}
		if len(runs) == 0 {
			t.Fatal("expected at least 1 run in history")
		}
		if total < 3 {
			t.Errorf("want total >= 3, got %d", total)
		}
	})

	t.Run("run history with pagination", func(t *testing.T) {
		runs, total, _ := db.ListLinterRunHistory("gofmt", 0, 1)
		if len(runs) > 1 {
			t.Errorf("want at most 1 with limit=1, got %d", len(runs))
		}
		if total < 3 {
			t.Errorf("want total >= 3, got %d", total)
		}
	})

	t.Run("empty history for unknown linter", func(t *testing.T) {
		runs, total, _ := db.ListLinterRunHistory("nonexistent", 0, 10)
		if len(runs) != 0 {
			t.Errorf("want 0 runs, got %d", len(runs))
		}
		if total != 0 {
			t.Errorf("want total=0, got %d", total)
		}
	})
}
