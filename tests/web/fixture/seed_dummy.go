package main

import (
	"fmt"
	"time"

	"github.com/emergent-company/runlog"
)

// seedDummy creates additional runs with varied event sequences for dev UI analysis.
func seedDummy(db *runlog.RunDB) error {
	now := time.Now()
	base := now.Add(-30 * time.Minute)

	if err := seedAnalyzer(db, base); err != nil {
		return err
	}
	if err := seedExperiment(db, base.Add(5*time.Minute)); err != nil {
		return err
	}
	if err := seedHybrid(db, base.Add(10*time.Minute)); err != nil {
		return err
	}
	if err := seedPerf(db, base.Add(15*time.Minute)); err != nil {
		return err
	}
	if err := seedMultiStep(db, base.Add(20*time.Minute)); err != nil {
		return err
	}
	return nil
}

// seedAnalyzer simulates an LLM-based analyzer/testing pipeline.
// Events: state_change, section, cli, log, token_usage, metric, gantt, token_summary, tag
func seedAnalyzer(db *runlog.RunDB, t0 time.Time) error {
	runID, err := db.InsertRun("TestAnalyzer", t0, "docker", "prod", nil)
	if err != nil {
		return err
	}

	db.InsertEvent(runID, 1, t0, 0, "state_change", "test started", nil)
	db.UpdateRunExperiment(runID, "exp-analyzer-v2")
	db.UpdateRunTags(runID, []string{"variant:llm-analyzer", "model:gemini-2.5-pro"})

	t := t0.Add(2 * time.Second)
	sid, _ := db.InsertGroupEvent(runID, 2, t, 2, "section", "analyze codebase")
	db.AppendGroupChildren(sid, []runlog.ChildEvent{
		{ElapsedS: 2, Kind: "cli", Message: "go list ./..."},
		{ElapsedS: 3, Kind: "log", Message: "found 24 packages"},
		{ElapsedS: 4, Kind: "cli", Message: "go vet ./..."},
		{ElapsedS: 5, Kind: "log", Message: "vet passed"},
	})

	// Tag after analysis
	t = t0.Add(8 * time.Second)
	db.InsertEvent(runID, 3, t, 8, "tag", "coverage:67%", map[string]any{"tags": []string{"coverage:67%", "files:142"}})
	db.UpdateRunTags(runID, []string{"variant:llm-analyzer", "model:gemini-2.5-pro", "coverage:67%", "files:142"})

	// Agent steps
	t = t0.Add(10 * time.Second)
	sid2, _ := db.InsertGroupEvent(runID, 4, t, 10, "section", "agent: researcher")
	db.AppendGroupChildren(sid2, []runlog.ChildEvent{
		{ElapsedS: 10, Kind: "log", Message: "researching code patterns..."},
		{ElapsedS: 12, Kind: "metric", Message: "researcher: 2.4s (in: 890, out: 2100)"},
		{ElapsedS: 13, Kind: "token_usage", Message: "LLM call #1: input=890 output=2100 cost=$0.005"},
	})

	t = t0.Add(15 * time.Second)
	db.InsertEvent(runID, 5, t, 15, "token_usage", "LLM call #2: input=1200 output=3400 cost=$0.008", map[string]any{
		"input_tokens":  1200,
		"output_tokens": 3400,
		"cost_usd":      0.008,
	})

	t = t0.Add(16 * time.Second)
	db.InsertEvent(runID, 6, t, 16, "metric", "planner: 3.1s (in: 450, out: 890)", map[string]any{
		"agent_name":    "planner",
		"duration_ms":   3100,
		"input_tokens":  450,
		"output_tokens": 890,
	})

	t = t0.Add(17 * time.Second)
	sid3, _ := db.InsertGroupEvent(runID, 7, t, 17, "section", "generate tests")
	db.AppendGroupChildren(sid3, []runlog.ChildEvent{
		{ElapsedS: 17, Kind: "cli", Message: "go test -v -run TestGenerated ./..."},
		{ElapsedS: 18, Kind: "log", Message: "=== RUN TestGenerated"},
		{ElapsedS: 19, Kind: "log", Message: "--- PASS: TestGenerated (0.5s)"},
	})

	t = t0.Add(22 * time.Second)
	db.InsertEvent(runID, 8, t, 22, "gantt", "orchestration timeline", map[string]any{
		"total_s": 22,
		"rows": []map[string]any{
			{"agent_name": "analyzer", "start_s": 0, "end_s": 5, "duration_ms": 5000},
			{"agent_name": "researcher", "start_s": 5, "end_s": 12, "duration_ms": 7000},
			{"agent_name": "planner", "start_s": 12, "end_s": 17, "duration_ms": 5000},
			{"agent_name": "generator", "start_s": 17, "end_s": 22, "duration_ms": 5000},
		},
	})

	t = t0.Add(23 * time.Second)
	db.InsertEvent(runID, 9, t, 23, "token_summary", "total: input=2540 output=6390 cost=$0.016", map[string]any{
		"total_runs":    2,
		"input_tokens":  2540,
		"output_tokens": 6390,
		"cost_usd":      0.016,
		"by_agent": map[string]any{
			"researcher": map[string]any{"input_tokens": 2090, "output_tokens": 5500, "cost_usd": 0.013},
			"planner":    map[string]any{"input_tokens": 450, "output_tokens": 890, "cost_usd": 0.003},
		},
	})

	db.InsertEvent(runID, 10, t, 24, "state_change", "test finished", nil)
	db.FinishRunWithCost(runID, t.Add(1*time.Second), runlog.OutcomePass, "", 2540, 6390, 0.016)
	return nil
}

// seedExperiment simulates an experiment runner with multiple iterations.
// Events: state_change, section, cli, log, token_usage, tag, failure, group, gantt, metric
func seedExperiment(db *runlog.RunDB, t0 time.Time) error {
	// Create two runs of the same experiment test
	envVars := map[string]string{"EXPERIMENT": "llm-compare", "ITERATIONS": "3"}
	envVars2 := map[string]string{"EXPERIMENT": "llm-compare", "ITERATIONS": "2"}

	// Run 1: 3 iterations, partial failures
	runID, err := db.InsertRun("TestExperiment", t0, "docker", "staging", envVars)
	if err != nil {
		return err
	}
	db.InsertEvent(runID, 1, t0, 0, "state_change", "test started", nil)
	db.UpdateRunExperiment(runID, "exp-llm-compare")
	db.UpdateRunTags(runID, []string{"experiment:llm-compare", "model:gemini-2.0-flash"})

	t := t0.Add(1 * time.Second)
	db.InsertEvent(runID, 2, t, 1, "tag", "model: gemini-2.0-flash", map[string]any{"tags": []string{"experiment:llm-compare", "model:gemini-2.0-flash", "iter:3"}})
	db.UpdateRunTags(runID, []string{"experiment:llm-compare", "model:gemini-2.0-flash", "iter:3"})

	// Iteration 1 — pass
	t = t0.Add(3 * time.Second)
	sid, _ := db.InsertGroupEvent(runID, 3, t, 3, "section", "iteration 1/3")
	db.AppendGroupChildren(sid, []runlog.ChildEvent{
		{ElapsedS: 3, Kind: "cli", Message: "go test -v -run Iter1 ./..."},
		{ElapsedS: 4, Kind: "log", Message: "=== RUN Iter1"},
		{ElapsedS: 5, Kind: "log", Message: "--- PASS: Iter1 (0.3s)"},
		{ElapsedS: 5, Kind: "token_usage", Message: "LLM: input=320 output=1100 cost=$0.003"},
	})
	t = t0.Add(9 * time.Second)
	db.InsertEvent(runID, 4, t, 9, "metric", "iteration 1: 5.2s", map[string]any{
		"iteration":  1,
		"duration_s": 5.2,
		"passed":     true,
	})

	// Iteration 2 — fail
	t = t0.Add(10 * time.Second)
	sid2, _ := db.InsertGroupEvent(runID, 5, t, 10, "section", "iteration 2/3")
	db.AppendGroupChildren(sid2, []runlog.ChildEvent{
		{ElapsedS: 10, Kind: "cli", Message: "go test -v -run Iter2 ./..."},
		{ElapsedS: 11, Kind: "log", Message: "=== RUN Iter2"},
		{ElapsedS: 12, Kind: "log", Message: "    iter_test.go:42: expected 100, got 99"},
		{ElapsedS: 13, Kind: "failure", Message: "assertion failed at iter_test.go:42"},
	})
	t = t0.Add(15 * time.Second)
	db.InsertEvent(runID, 6, t, 15, "metric", "iteration 2: 5.8s (FAIL)", map[string]any{
		"iteration":   2,
		"duration_s":  5.8,
		"passed":      false,
		"fail_reason": "expected 100, got 99",
	})

	// Iteration 3 — pass (recovery)
	t = t0.Add(16 * time.Second)
	sid3, _ := db.InsertGroupEvent(runID, 7, t, 16, "section", "iteration 3/3")
	db.AppendGroupChildren(sid3, []runlog.ChildEvent{
		{ElapsedS: 16, Kind: "cli", Message: "go test -v -run Iter3 ./..."},
		{ElapsedS: 17, Kind: "log", Message: "=== RUN Iter3"},
		{ElapsedS: 18, Kind: "log", Message: "--- PASS: Iter3 (0.4s)"},
	})
	t = t0.Add(20 * time.Second)
	db.InsertEvent(runID, 8, t, 20, "metric", "iteration 3: 4.1s", map[string]any{
		"iteration":  3,
		"duration_s": 4.1,
		"passed":     true,
	})

	t = t0.Add(21 * time.Second)
	db.InsertEvent(runID, 9, t, 21, "gantt", "experiment iterations", map[string]any{
		"total_s": 21,
		"rows": []map[string]any{
			{"agent_name": "iteration-1", "start_s": 3, "end_s": 8, "duration_ms": 5000, "status": "pass"},
			{"agent_name": "iteration-2", "start_s": 10, "end_s": 15, "duration_ms": 5000, "status": "fail"},
			{"agent_name": "iteration-3", "start_s": 16, "end_s": 20, "duration_ms": 4000, "status": "pass"},
		},
	})

	t = t0.Add(22 * time.Second)
	db.InsertEvent(runID, 10, t, 22, "state_change", "test finished", nil)
	db.FinishRun(runID, t, runlog.OutcomeFail,
		"iteration 2 failed: expected 100, got 99")

	// Run 2: 2 iterations, all pass — to show run history
	runID2, err := db.InsertRun("TestExperiment", t0.Add(2*time.Minute), "docker", "staging", envVars2)
	if err != nil {
		return err
	}
	db.InsertEvent(runID2, 1, t0.Add(2*time.Minute), 0, "state_change", "test started", nil)
	db.UpdateRunExperiment(runID2, "exp-llm-compare")
	db.UpdateRunTags(runID2, []string{"experiment:llm-compare", "model:claude-3.5", "iter:2"})

	t = t0.Add(2*time.Minute + 3*time.Second)
	sid4, _ := db.InsertGroupEvent(runID2, 2, t, 3, "section", "iteration 1/2")
	db.AppendGroupChildren(sid4, []runlog.ChildEvent{
		{ElapsedS: 3, Kind: "cli", Message: "go test -v -run Iter1 ./..."},
		{ElapsedS: 4, Kind: "log", Message: "--- PASS: Iter1 (0.2s)"},
	})

	t = t0.Add(2*time.Minute + 8*time.Second)
	sid5, _ := db.InsertGroupEvent(runID2, 3, t, 8, "section", "iteration 2/2")
	db.AppendGroupChildren(sid5, []runlog.ChildEvent{
		{ElapsedS: 8, Kind: "cli", Message: "go test -v -run Iter2 ./..."},
		{ElapsedS: 9, Kind: "log", Message: "--- PASS: Iter2 (0.3s)"},
	})

	t = t0.Add(2*time.Minute + 10*time.Second)
	db.InsertEvent(runID2, 4, t, 10, "state_change", "test finished", nil)
	db.FinishRunWithCost(runID2, t, runlog.OutcomePass, "", 640, 1800, 0.004)

	return nil
}

// seedHybrid mixes many event kinds: skills, credentials, groups, nested sections, trace_span.
// Events: state_change, section, group, cli, log, failure, skip, tag, trace_span, skill, credentials
func seedHybrid(db *runlog.RunDB, t0 time.Time) error {
	runID, err := db.InsertRun("TestHybrid", t0, "host", "localhost", nil)
	if err != nil {
		return err
	}

	db.InsertEvent(runID, 1, t0, 0, "state_change", "test started", nil)
	db.UpdateRunTags(runID, []string{"variant:hybrid", "module:auth+data"})
	db.UpdateRunDescription(runID, runlog.RunDescription{
		Summary: "Hybrid test with auth, data processing, and validation phases",
		Bullets: []string{"OAuth2 token exchange", "Data pipeline ETL", "Cross-module validation"},
	})

	// Phase 1: Auth — starts with trace_span
	t := t0.Add(1 * time.Second)
	sid, _ := db.InsertGroupEvent(runID, 2, t, 1, "section", "authentication")
	db.AppendGroupChildren(sid, []runlog.ChildEvent{
		{ElapsedS: 1, Kind: "cli", Message: "curl -X POST https://auth.example.com/token"},
		{ElapsedS: 2, Kind: "log", Message: "token exchanged successfully"},
		{ElapsedS: 2, Kind: "skill", Message: "oauth2.token_exchange (120ms)"},
		{ElapsedS: 3, Kind: "credentials", Message: "using vault://staging/oauth2-client"},
	})

	db.InsertEvent(runID, 3, t.Add(3*time.Second), 4, "trace_span", "auth.token_exchange (45ms)", map[string]any{
		"trace_id":    "trace-auth-1",
		"span_id":     "span-auth-1",
		"parent_id":   nil,
		"duration_ms": 45,
		"service":     "auth",
		"operation":   "token_exchange",
	})

	// Phase 2: Data processing — group events for sub-steps
	t = t0.Add(6 * time.Second)
	sid2, _ := db.InsertGroupEvent(runID, 4, t, 6, "section", "data processing")
	db.AppendGroupChildren(sid2, []runlog.ChildEvent{
		{ElapsedS: 6, Kind: "cli", Message: "./etl_pipeline --input /data/raw --output /data/processed"},
		{ElapsedS: 7, Kind: "log", Message: "ETL started (batch_id=42)"},
		{ElapsedS: 9, Kind: "log", Message: "ETL complete: 1420 rows processed"},
		{ElapsedS: 9, Kind: "tag", Message: "rows:1420,errors:3"},
	})

	t = t0.Add(12 * time.Second)
	sid3, _ := db.InsertGroupEvent(runID, 5, t, 12, "group", "data quality checks")
	db.AppendGroupChildren(sid3, []runlog.ChildEvent{
		{ElapsedS: 12, Kind: "cli", Message: "pq --db /data/processed -q 'SELECT count(*) FROM errors'"},
		{ElapsedS: 13, Kind: "log", Message: "3 validation errors found"},
		{ElapsedS: 14, Kind: "failure", Message: "row #412: null in non-nullable column email"},
	})
	db.InsertEvent(runID, 6, t.Add(3*time.Second), 15, "failure", "data quality check failed: 3 errors in processed/412", map[string]any{
		"check":      "non-null-constraints",
		"errors":     3,
		"sample_row": 412,
		"column":     "email",
	})

	// Phase 3: Validation — partial skip
	t = t0.Add(18 * time.Second)
	sid4, _ := db.InsertGroupEvent(runID, 7, t, 18, "section", "validation")
	db.AppendGroupChildren(sid4, []runlog.ChildEvent{
		{ElapsedS: 18, Kind: "cli", Message: "go test -v -run TestValidation ./..."},
		{ElapsedS: 19, Kind: "log", Message: "=== RUN TestValidation/Sanity"},
		{ElapsedS: 20, Kind: "log", Message: "--- PASS: TestValidation/Sanity (0.1s)"},
		{ElapsedS: 21, Kind: "log", Message: "=== RUN TestValidation/DeepCheck"},
		{ElapsedS: 22, Kind: "skip", Message: "TestValidation/DeepCheck requires GPUs"},
	})

	t = t0.Add(23 * time.Second)
	db.InsertEvent(runID, 8, t, 23, "state_change", "test finished", nil)
	db.FinishRun(runID, t, runlog.OutcomeFail, "data quality check failed (3 errors)")
	return nil
}

// seedPerf simulates a performance run with many metric events and gantt data.
// Events: state_change, section, cli, log, metric, gantt, tag
func seedPerf(db *runlog.RunDB, t0 time.Time) error {
	runID, err := db.InsertRun("TestPerf", t0, "docker", "benchmark", map[string]string{"SCALE": "1000"})
	if err != nil {
		return err
	}

	db.InsertEvent(runID, 1, t0, 0, "state_change", "test started", nil)
	db.UpdateRunExperiment(runID, "exp-perf-baseline")
	db.UpdateRunTags(runID, []string{"variant:perf", "scale:1000"})

	// Warmup phase
	t := t0.Add(1 * time.Second)
	sid, _ := db.InsertGroupEvent(runID, 2, t, 1, "section", "warmup")
	db.AppendGroupChildren(sid, []runlog.ChildEvent{
		{ElapsedS: 1, Kind: "cli", Message: "./bench --warmup --requests 100"},
		{ElapsedS: 2, Kind: "log", Message: "warmup complete: 100 req in 1.2s"},
		{ElapsedS: 2, Kind: "metric", Message: "warmup: p50=12ms p95=34ms p99=89ms"},
	})

	// Benchmark runs — 3 iterations with detailed metrics
	benchmarks := []struct {
		name   string
		scale  int
		p50    int
		p95    int
		p99    int
		durS   float64
		tokens int
	}{
		{"small", 100, 8, 22, 65, 2.1, 340},
		{"medium", 500, 15, 48, 142, 3.4, 890},
		{"large", 1000, 28, 95, 280, 5.2, 2100},
	}

	for i, b := range benchmarks {
		offset := 4 + i*7
		offF := float64(offset)
		t = t0.Add(time.Duration(offset) * time.Second)
		sid2, _ := db.InsertGroupEvent(runID, 3+i, t, offF, "section", "benchmark: "+b.name)
		db.AppendGroupChildren(sid2, []runlog.ChildEvent{
			{ElapsedS: offF, Kind: "cli", Message: fmt.Sprintf("./bench --scale %d --requests 1000", b.scale)},
			{ElapsedS: offF + 0.5, Kind: "log", Message: fmt.Sprintf("benchmark %s started (scale=%d)", b.name, b.scale)},
			{ElapsedS: offF + b.durS, Kind: "log", Message: fmt.Sprintf("benchmark %s complete: %d req in %.1fs", b.name, b.scale, b.durS)},
		})
		db.InsertEvent(runID, 4+i, t.Add(time.Duration(b.durS)*time.Second), offF+b.durS,
			"metric", fmt.Sprintf("%s: p50=%dms p95=%dms p99=%dms", b.name, b.p50, b.p95, b.p99), map[string]any{
				"benchmark":  b.name,
				"scale":      b.scale,
				"p50_ms":     b.p50,
				"p95_ms":     b.p95,
				"p99_ms":     b.p99,
				"duration_s": b.durS,
			})
		db.InsertEvent(runID, 5+i, t.Add(time.Duration(b.durS+1)*time.Second), offF+b.durS+1,
			"token_usage", fmt.Sprintf("%s: input=%d output=%d cost=$%.4f", b.name, b.tokens, b.tokens*3, float64(b.tokens)*0.000002), map[string]any{
				"benchmark":     b.name,
				"input_tokens":  b.tokens,
				"output_tokens": b.tokens * 3,
				"cost_usd":      float64(b.tokens) * 0.000002,
			})
	}

	// Final gantt
	totalS := 4 + 3*7 + 2
	t = t0.Add(time.Duration(totalS) * time.Second)
	db.InsertEvent(runID, 8, t, float64(totalS), "gantt", "benchmark orchestration", map[string]any{
		"total_s": totalS,
		"rows": []map[string]any{
			{"agent_name": "warmup", "start_s": 1, "end_s": 3, "duration_ms": 2000, "status": "pass"},
			{"agent_name": "bench-small", "start_s": 4, "end_s": 6, "duration_ms": 2100, "status": "pass"},
			{"agent_name": "bench-medium", "start_s": 11, "end_s": 14, "duration_ms": 3400, "status": "pass"},
			{"agent_name": "bench-large", "start_s": 18, "end_s": 23, "duration_ms": 5200, "status": "pass"},
		},
	})

	t = t0.Add(time.Duration(totalS+1) * time.Second)
	db.InsertEvent(runID, 9, t, float64(totalS+1), "state_change", "test finished", nil)
	db.FinishRunWithCost(runID, t, runlog.OutcomePass, "", 3330, 9990, 0.015)
	return nil
}

// seedMultiStep simulates a long-running test with many sequential steps.
// Events: state_change, section, cli, log
func seedMultiStep(db *runlog.RunDB, t0 time.Time) error {
	runID, err := db.InsertRun("TestMultiStep", t0, "host", "localhost", nil)
	if err != nil {
		return err
	}

	db.InsertEvent(runID, 1, t0, 0, "state_change", "test started", nil)

	// 8 sequential steps
	stepNames := []string{
		"parse config",
		"connect database",
		"load fixtures",
		"validate schema",
		"seed test data",
		"run pre-checks",
		"execute scenarios",
		"verify results",
		"cleanup resources",
	}

	for i, name := range stepNames {
		off := 2 + i*2
		t := t0.Add(time.Duration(off) * time.Second)
		sid, _ := db.InsertGroupEvent(runID, off, t, float64(off), "section", name)
		db.AppendGroupChildren(sid, []runlog.ChildEvent{
			{ElapsedS: float64(off), Kind: "cli", Message: fmt.Sprintf("step_%d --name %s", i+1, name)},
			{ElapsedS: float64(off) + 0.5, Kind: "log", Message: fmt.Sprintf("step %d/%d: %s completed", i+1, len(stepNames), name)},
		})
	}

	total := 2 + len(stepNames)*2
	t := t0.Add(time.Duration(total) * time.Second)
	db.InsertEvent(runID, total, t, float64(total), "state_change", "test finished", nil)
	db.FinishRun(runID, t, runlog.OutcomePass, "")
	db.UpdateRunDescription(runID, runlog.RunDescription{
		Summary: fmt.Sprintf("Multi-step test with %d sequential phases", len(stepNames)),
		Bullets: []string{"All 9 steps completed successfully", "No failures or warnings"},
	})
	return nil
}
