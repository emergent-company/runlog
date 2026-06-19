package main

import (
	"time"

	"github.com/emergent-company/runlog"
)

func seed(db *runlog.RunDB) error {
	now := time.Now()
	startBase := now.Add(-2 * time.Hour)

	// --- TestPass: one passing run with sections, CLI, log, state_change events ---
	passRun1, err := db.InsertRun("TestPass", startBase, "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(passRun1, 1, startBase, 0, "state_change", "test started", nil)
	sectionID, _ := db.InsertGroupEvent(passRun1, 2, startBase.Add(2*time.Second), 2, "section", "setup phase")
	db.AppendGroupChildren(sectionID, []runlog.ChildEvent{
		{ElapsedS: 2, Kind: "cli", Message: "go build ./..."},
		{ElapsedS: 3, Kind: "log", Message: "build succeeded (3 packages)"},
	})
	db.InsertEvent(passRun1, 3, startBase.Add(5*time.Second), 5, "cli", "go test -v -run TestPass ./...", nil)
	db.InsertEvent(passRun1, 4, startBase.Add(6*time.Second), 6, "log", "=== RUN   TestPass", nil)
	db.InsertEvent(passRun1, 5, startBase.Add(7*time.Second), 7, "log", "--- PASS: TestPass (0.01s)", nil)
	db.InsertEvent(passRun1, 6, startBase.Add(8*time.Second), 8, "state_change", "test finished", nil)
	finish := startBase.Add(8 * time.Second)
	db.FinishRun(passRun1, finish, runlog.OutcomePass, "")

	// --- TestFail: one failing run ---
	failStart := startBase.Add(30 * time.Minute)
	failRun1, err := db.InsertRun("TestFail", failStart, "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(failRun1, 1, failStart, 0, "state_change", "test started", nil)
	db.InsertEvent(failRun1, 2, failStart.Add(1*time.Second), 1, "cli", "go test -v -run TestFail ./...", nil)
	db.InsertEvent(failRun1, 3, failStart.Add(2*time.Second), 2, "log", "=== RUN   TestFail", nil)
	db.InsertEvent(failRun1, 4, failStart.Add(3*time.Second), 3, "log", "    fail_test.go:20: expected 42, got 0", nil)
	db.InsertEvent(failRun1, 5, failStart.Add(5*time.Second), 5, "failure", "assertion failed at fail_test.go:20", map[string]any{
		"expected": 42,
		"actual":   0,
		"file":     "fail_test.go",
		"line":     20,
	})
	db.InsertEvent(failRun1, 6, failStart.Add(6*time.Second), 6, "log", "--- FAIL: TestFail (0.02s)", nil)
	db.FinishRun(failRun1, failStart.Add(6*time.Second), runlog.OutcomeFail, "expected 42, got 0")

	// second run of TestFail — passes
	failRun2, err := db.InsertRun("TestFail", failStart.Add(1*time.Hour), "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(failRun2, 1, failStart.Add(1*time.Hour), 0, "state_change", "test started", nil)
	db.InsertEvent(failRun2, 2, failStart.Add(1*time.Hour).Add(1*time.Second), 1, "cli", "go test -v -run TestFail ./...", nil)
	db.InsertEvent(failRun2, 3, failStart.Add(1*time.Hour).Add(2*time.Second), 2, "log", "=== RUN   TestFail", nil)
	db.InsertEvent(failRun2, 4, failStart.Add(1*time.Hour).Add(3*time.Second), 3, "log", "--- PASS: TestFail (0.01s)", nil)
	db.InsertEvent(failRun2, 5, failStart.Add(1*time.Hour).Add(4*time.Second), 4, "state_change", "test finished", nil)
	db.FinishRun(failRun2, failStart.Add(1*time.Hour).Add(4*time.Second), runlog.OutcomePass, "")

	// --- TestSkip: one skipped run ---
	skipStart := startBase.Add(1 * time.Hour)
	skipRun1, err := db.InsertRun("TestSkip", skipStart, "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(skipRun1, 1, skipStart, 0, "state_change", "test started", nil)
	db.InsertEvent(skipRun1, 2, skipStart.Add(1*time.Second), 1, "cli", "go test -v -run TestSkip ./...", nil)
	db.InsertEvent(skipRun1, 3, skipStart.Add(2*time.Second), 2, "log", "=== RUN   TestSkip", nil)
	db.InsertEvent(skipRun1, 4, skipStart.Add(3*time.Second), 3, "skip", "TestSkip requires Redis", nil)
	db.InsertEvent(skipRun1, 5, skipStart.Add(4*time.Second), 4, "log", "--- SKIP: TestSkip (0.01s)", nil)
	db.FinishRun(skipRun1, skipStart.Add(4*time.Second), runlog.OutcomeSkip, "TestSkip requires Redis")

	// --- TestMetrics: one pass run with token usage, cost, env_vars ---
	metricsStart := startBase.Add(90 * time.Minute)
	envVars := map[string]string{"GOOGLE_AI_API_KEY": "sk-xxxx", "MODEL": "gemini-2.0-flash"}
	metricsRun1, err := db.InsertRun("TestMetrics", metricsStart, "docker", "staging", envVars)
	if err != nil {
		return err
	}
	db.InsertEvent(metricsRun1, 1, metricsStart, 0, "state_change", "test started", nil)
	agentSection, _ := db.InsertGroupEvent(metricsRun1, 2, metricsStart.Add(2*time.Second), 2, "section", "agent: researcher")
	db.AppendGroupChildren(agentSection, []runlog.ChildEvent{
		{ElapsedS: 2, Kind: "metric", Message: "researcher completed in 2.5s (in: 450, out: 1200, cost: $0.003)"},
		{ElapsedS: 4, Kind: "token_usage", Message: "LLM call #1: input=450 output=1200 cost=$0.003"},
	})
	db.InsertEvent(metricsRun1, 3, metricsStart.Add(5*time.Second), 5, "token_usage", "LLM call #2: input=800 output=2400 cost=$0.006", map[string]any{
		"input_tokens":  800,
		"output_tokens": 2400,
		"cost_usd":      0.006,
	})
	db.InsertEvent(metricsRun1, 4, metricsStart.Add(6*time.Second), 6, "token_summary", "total: input=1250 output=3600 cost=$0.009", map[string]any{
		"total_runs":    1,
		"input_tokens":  1250,
		"output_tokens": 3600,
		"cost_usd":      0.009,
		"by_agent": map[string]any{
			"researcher": map[string]any{
				"input_tokens":  450,
				"output_tokens": 1200,
				"cost_usd":      0.003,
			},
		},
	})
	db.InsertEvent(metricsRun1, 5, metricsStart.Add(7*time.Second), 7, "gantt", "agent timeline", map[string]any{
		"total_s": 7,
		"rows": []map[string]any{
			{"agent_name": "researcher", "start_s": 0, "end_s": 2.5, "duration_ms": 2500},
			{"agent_name": "planner", "start_s": 2.5, "end_s": 5.0, "duration_ms": 2500},
		},
	})
	db.FinishRunWithCost(metricsRun1, metricsStart.Add(7*time.Second), runlog.OutcomePass, "",
		1250, 3600, 0.009)

	// --- TestWithTags: one pass run with tags and experiment ---
	tagsStart := startBase.Add(105 * time.Minute)
	tagsRun1, err := db.InsertRun("TestWithTags", tagsStart, "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(tagsRun1, 1, tagsStart, 0, "state_change", "test started", nil)
	sectionID2, _ := db.InsertGroupEvent(tagsRun1, 2, tagsStart.Add(1*time.Second), 1, "section", "validation")
	db.AppendGroupChildren(sectionID2, []runlog.ChildEvent{
		{ElapsedS: 1, Kind: "cli", Message: "go vet ./..."},
		{ElapsedS: 2, Kind: "log", Message: "vet passed"},
	})
	db.InsertEvent(tagsRun1, 3, tagsStart.Add(3*time.Second), 3, "tag", "variant: baseline", map[string]any{
		"tags": []string{"variant:baseline", "run:1"},
	})
	db.InsertEvent(tagsRun1, 4, tagsStart.Add(4*time.Second), 4, "log", "--- PASS: TestWithTags (0.02s)", nil)
	db.FinishRun(tagsRun1, tagsStart.Add(4*time.Second), runlog.OutcomePass, "")
	db.UpdateRunTags(tagsRun1, []string{"variant:baseline", "run:1"})
	db.UpdateRunExperiment(tagsRun1, "exp-tag-demo")
	db.UpdateRunDescription(tagsRun1, runlog.RunDescription{
		Summary: "Validated baseline variant with vet and lint checks",
		Bullets: []string{"go vet passed", "go build passed", "3 test cases verified"},
	})
	db.UpdateRunTags(tagsRun1, []string{"variant:baseline", "run:1"})

	// Add a second run for TestPass to have multi-run history
	passRun2, err := db.InsertRun("TestPass", startBase.Add(115*time.Minute), "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(passRun2, 1, startBase.Add(115*time.Minute), 0, "state_change", "test started", nil)
	db.InsertEvent(passRun2, 2, startBase.Add(115*time.Minute).Add(1*time.Second), 1, "cli", "go test -v -run TestPass ./...", nil)
	db.InsertEvent(passRun2, 3, startBase.Add(115*time.Minute).Add(2*time.Second), 2, "log", "=== RUN   TestPass\n--- PASS: TestPass (0.01s)", nil)
	db.FinishRun(passRun2, startBase.Add(115*time.Minute).Add(3*time.Second), runlog.OutcomePass, "")

	// Add a skipped run for TestPass
	passRun3, err := db.InsertRun("TestPass", startBase.Add(125*time.Minute), "docker", "staging", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(passRun3, 1, startBase.Add(125*time.Minute), 0, "state_change", "test started", nil)
	db.InsertEvent(passRun3, 2, startBase.Add(125*time.Minute).Add(1*time.Second), 1, "cli", "go test -v -run TestPass ./...", nil)
	db.InsertEvent(passRun3, 3, startBase.Add(125*time.Minute).Add(2*time.Second), 2, "skip", "Redis not available in staging", nil)
	db.FinishRun(passRun3, startBase.Add(125*time.Minute).Add(3*time.Second), runlog.OutcomeSkip, "Redis not available in staging")

	// one more failing run for TestFail — with trace_span event
	failRun3, err := db.InsertRun("TestFail", startBase.Add(135*time.Minute), "docker", "staging",
		map[string]string{"REDIS_URL": "redis://localhost:6379"})
	if err != nil {
		return err
	}
	db.InsertEvent(failRun3, 1, startBase.Add(135*time.Minute), 0, "state_change", "test started", nil)
	db.InsertEvent(failRun3, 2, startBase.Add(135*time.Minute).Add(1*time.Second), 1, "trace_span", "redis connect (3ms)", map[string]any{
		"trace_id":    "abc123",
		"span_id":     "span-1",
		"duration_ms": 3,
		"service":     "redis",
	})
	db.InsertEvent(failRun3, 3, startBase.Add(135*time.Minute).Add(4*time.Second), 4, "failure", "connection pool exhausted", map[string]any{
		"error":        "pool exhausted",
		"active_conns": 32,
		"max_conns":    32,
	})
	db.FinishRun(failRun3, startBase.Add(135*time.Minute).Add(5*time.Second), runlog.OutcomeFail,
		"connection pool exhausted (max=32)")

	// A run that was never finished (stale) to test the timeout mechanism.
	// Start time set in the past so the timeout worker catches it.
	staleStart := startBase.Add(90 * time.Minute)
	staleRun1, err := db.InsertRun("TestPass", staleStart, "host", "localhost", nil)
	if err != nil {
		return err
	}
	db.InsertEvent(staleRun1, 1, staleStart, 0, "state_change", "test started", nil)
	db.InsertEvent(staleRun1, 2, staleStart.Add(1*time.Second), 1, "log", "stale — never finished", nil)

	return nil
}
