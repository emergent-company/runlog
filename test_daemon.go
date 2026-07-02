package runlog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// StartTestDaemon starts a test HTTP server mimicking the daemon API
// backed by an isolated temp DB. Returns the daemon URL and DaemonClient.
func StartTestDaemon(t *testing.T) (string, *DaemonClient) { //nolint:deadcode
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir + "/test.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/runs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 65536))
			var req struct {
				PID        int    `json:"pid"`
				EnvProfile string `json:"env_profile"`
				Category   string `json:"category,omitempty"`
				StartedAt  string `json:"started_at,omitempty"`
			}
			json.Unmarshal(body, &req)
			started := req.StartedAt
			if started == "" {
				started = time.Now().UTC().Format(time.RFC3339)
			}
			res, err := db.db.Exec(
				`INSERT INTO test_runs(test_name, started_at, runner, category) VALUES (?, ?, 'test', ?)`,
				req.EnvProfile, started, req.Category,
			)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			id, _ := res.LastInsertId()
			writeJSON(w, 201, map[string]any{"id": fmt.Sprintf("test-%d", id), "test_run_id": id})
		case "GET":
			rows, _ := db.db.Query(`SELECT id, test_name, started_at, finished_at, passed, skipped, COALESCE(category,'') FROM test_runs ORDER BY started_at DESC`)
			defer rows.Close()
			type row struct {
				ID          int64   `json:"id"`
				TestName    string  `json:"test_name"`
				StartedAt   string  `json:"started_at"`
				FinishedAt  *string `json:"finished_at,omitempty"`
				Passed      *int    `json:"passed,omitempty"`
				Skipped     bool    `json:"skipped"`
				Category    string  `json:"category"`
				DaemonRunID string  `json:"daemon_run_id,omitempty"`
			}
			var out []row
			for rows.Next() {
				var s row
				var fin *string
				var p *int
				rows.Scan(&s.ID, &s.TestName, &s.StartedAt, &fin, &p, &s.Skipped, &s.Category)
				if p != nil {
					s.Passed = p
				}
				if fin != nil && *fin != "" {
					s.FinishedAt = fin
				}
				out = append(out, s)
			}
			writeJSON(w, 200, out)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})

	mux.HandleFunc("/runs/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/runs/")
		path = strings.TrimSuffix(path, "/")
		if strings.HasSuffix(path, "/events") && r.Method == "POST" {
			runID := strings.TrimSuffix(path, "/events")
			runID = strings.TrimSuffix(runID, "/")
			var testRunID int64
			fmt.Sscanf(runID, "test-%d", &testRunID)
			if testRunID == 0 {
				http.Error(w, "invalid run id", 400)
				return
			}
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			}
			json.Unmarshal(body, &req)
			var maxSeq int
			db.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM run_events WHERE run_id=?`, testRunID).Scan(&maxSeq)
			db.db.Exec(
				`INSERT INTO run_events(run_id, seq, kind, message, elapsed_s, occurred_at) VALUES (?, ?, ?, ?, 0.5, datetime('now'))`,
				testRunID, maxSeq+1, req.Kind, req.Message,
			)
			writeJSON(w, 201, map[string]any{"seq": maxSeq + 1})
			return
		}
		if strings.HasSuffix(path, "/done") && r.Method == "PUT" {
			runID := strings.TrimSuffix(path, "/done")
			runID = strings.TrimSuffix(runID, "/")
			var testRunID int64
			fmt.Sscanf(runID, "test-%d", &testRunID)
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Passed       *bool    `json:"passed,omitempty"`
				Skipped      *bool    `json:"skipped,omitempty"`
				Reason       string   `json:"reason,omitempty"`
				InputTokens  *int64   `json:"input_tokens,omitempty"`
				OutputTokens *int64   `json:"output_tokens,omitempty"`
				CostUSD      *float64 `json:"cost_usd,omitempty"`
			}
			json.Unmarshal(body, &req)
			now := time.Now().UTC().Format(time.RFC3339)
			q := "UPDATE test_runs SET finished_at=?"
			args := []any{now}
			if req.Passed != nil {
				v := 0
				if *req.Passed {
					v = 1
				}
				q += fmt.Sprintf(", passed=%d", v)
			}
			if req.Skipped != nil && *req.Skipped {
				q += ", skipped=1"
			}
			if req.Reason != "" {
				q += ", reason=?"
				args = append(args, req.Reason)
			}
			if req.InputTokens != nil {
				q += ", input_tokens=?"
				args = append(args, *req.InputTokens)
			}
			if req.OutputTokens != nil {
				q += ", output_tokens=?"
				args = append(args, *req.OutputTokens)
			}
			if req.CostUSD != nil {
				q += ", cost_usd=?"
				args = append(args, *req.CostUSD)
			}
			q += " WHERE id=?"
			args = append(args, testRunID)
			db.db.Exec(q, args...)
			writeJSON(w, 200, map[string]string{"status": "done"})
			return
		}
		http.Error(w, "not found", 404)
	})

	mux.HandleFunc("/test-runs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		var id int64
		fmt.Sscanf(r.URL.Path, "/test-runs/%d", &id)
		if id == 0 {
			http.Error(w, "invalid id", 400)
			return
		}
		var finishedAt *string
		var passed *int
		var skipped bool
		var testName, startedAt, category string
		err := db.db.QueryRow(
			`SELECT test_name, started_at, finished_at, passed, skipped, COALESCE(category,'') FROM test_runs WHERE id=?`,
			id,
		).Scan(&testName, &startedAt, &finishedAt, &passed, &skipped, &category)
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		writeJSON(w, 200, map[string]any{
			"id": id, "test_name": testName, "started_at": startedAt,
			"finished_at": finishedAt, "passed": passed, "skipped": skipped, "category": category,
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		db.Close()
		server.Close()
	})
	dc := NewDaemonClient(server.URL)
	return server.URL, dc
}

func writeJSON(w http.ResponseWriter, status int, v any) { //nolint:deadcode
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
