// Package e2eframework — cost_test.go
//
// Tests for token usage and cost tracking functionality.
package runlog

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestRunLog_RecordTokenUsage(t *testing.T) {
	// Create temp DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// Create a minimal RunLog for testing
	rl := &RunLog{
		t:         t,
		db:        db,
		StartedAt: time.Now(),
	}

	// Insert a test run
	runID, err := db.InsertRun("TestCost", rl.StartedAt, "host", "test-env", nil)
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	rl.runID = runID

	// Record some token usage
	rl.RecordTokenUsage(1000, 500, 0.05)
	rl.RecordTokenUsage(2000, 1000, 0.10)

	// Check accumulated values
	if rl.inputTokens != 3000 {
		t.Errorf("inputTokens = %d, want 3000", rl.inputTokens)
	}
	if rl.outputTokens != 1500 {
		t.Errorf("outputTokens = %d, want 1500", rl.outputTokens)
	}
	// Allow small floating point error
	if rl.costUSD < 0.14999 || rl.costUSD > 0.15001 {
		t.Errorf("costUSD = %f, want ~0.15", rl.costUSD)
	}

	// Finish the run and write to DB
	finishTime := time.Now()
	if err := db.FinishRunWithCost(runID, finishTime, OutcomePass, "", rl.inputTokens, rl.outputTokens, rl.costUSD); err != nil {
		t.Fatalf("FinishRunWithCost: %v", err)
	}

	// Query DB to verify values were persisted
	var inputTok, outputTok sql.NullInt64
	var cost sql.NullFloat64
	row := db.db.QueryRow(`SELECT input_tokens, output_tokens, cost_usd FROM test_runs WHERE id = ?`, runID)
	if err := row.Scan(&inputTok, &outputTok, &cost); err != nil {
		t.Fatalf("query test_runs: %v", err)
	}

	if !inputTok.Valid || inputTok.Int64 != 3000 {
		t.Errorf("DB input_tokens = %v, want 3000", inputTok)
	}
	if !outputTok.Valid || outputTok.Int64 != 1500 {
		t.Errorf("DB output_tokens = %v, want 1500", outputTok)
	}
	// Allow small floating point error
	if !cost.Valid || cost.Float64 < 0.14999 || cost.Float64 > 0.15001 {
		t.Errorf("DB cost_usd = %v, want ~0.15", cost)
	}
}

func TestRunLog_RecordTokenUsage_ZeroValues(t *testing.T) {
	// Create temp DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	rl := &RunLog{
		t:         t,
		db:        db,
		StartedAt: time.Now(),
	}

	runID, err := db.InsertRun("TestCostZero", rl.StartedAt, "host", "test-env", nil)
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	rl.runID = runID

	// Don't record any token usage
	finishTime := time.Now()
	if err := db.FinishRunWithCost(runID, finishTime, OutcomePass, "", 0, 0, 0); err != nil {
		t.Fatalf("FinishRunWithCost: %v", err)
	}

	// Verify columns are NULL when zero
	var inputTok, outputTok sql.NullInt64
	var cost sql.NullFloat64
	row := db.db.QueryRow(`SELECT input_tokens, output_tokens, cost_usd FROM test_runs WHERE id = ?`, runID)
	if err := row.Scan(&inputTok, &outputTok, &cost); err != nil {
		t.Fatalf("query test_runs: %v", err)
	}

	if inputTok.Valid {
		t.Errorf("DB input_tokens should be NULL, got %d", inputTok.Int64)
	}
	if outputTok.Valid {
		t.Errorf("DB output_tokens should be NULL, got %d", outputTok.Int64)
	}
	if cost.Valid {
		t.Errorf("DB cost_usd should be NULL, got %f", cost.Float64)
	}
}

func TestRunLog_RecordTokenUsage_WithFile(t *testing.T) {
	tmpDir := newTestDB(t)

	rl := NewRunLog(t)
	rl.Describe("Token usage recording with file logging",
		"Creates a RunLog via NewRunLog which creates a log file",
		"Records token usage and verifies it's persisted in the DB",
	)

	rl.Section("Recording")
	rl.Printf("Recording 5000 input / 2500 output tokens at $0.25")
	rl.RecordTokenUsage(5000, 2500, 0.25)

	// Close will write cost to DB
	rl.Close()

	// Verify the data was persisted
	db, err := OpenDB(filepath.Join(tmpDir, "runs.db"))
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	var inputTok, outputTok sql.NullInt64
	var cost sql.NullFloat64
	row := db.db.QueryRow(`SELECT input_tokens, output_tokens, cost_usd FROM test_runs WHERE test_name = ?`, t.Name())
	if err := row.Scan(&inputTok, &outputTok, &cost); err != nil {
		t.Fatalf("query test_runs: %v", err)
	}

	if !inputTok.Valid || inputTok.Int64 != 5000 {
		t.Errorf("DB input_tokens = %v, want 5000", inputTok)
	}
	if !outputTok.Valid || outputTok.Int64 != 2500 {
		t.Errorf("DB output_tokens = %v, want 2500", outputTok)
	}
	if !cost.Valid || cost.Float64 != 0.25 {
		t.Errorf("DB cost_usd = %v, want 0.25", cost)
	}
}
