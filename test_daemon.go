package runlog

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
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
			runID := fmt.Sprintf("test-daemon-%d", rand.Int63())
			_, err := db.db.Exec(
				`INSERT INTO daemon_runs(id, pid, env_profile, status, started_at) VALUES (?, ?, ?, 'active', ?)`,
				runID, req.PID, req.EnvProfile, started,
			)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			res, err := db.db.Exec(
				`INSERT INTO test_runs(test_name, started_at, runner, category, daemon_run_id) VALUES (?, ?, 'test', ?, ?)`,
				req.EnvProfile, started, req.Category, runID,
			)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			testRunID, _ := res.LastInsertId()
			writeJSON(w, 201, map[string]any{"id": runID, "test_run_id": testRunID})
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
		parts := strings.SplitN(path, "/", 3)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "missing id", 400)
			return
		}
		runID := parts[0]

		if strings.HasSuffix(path, "/functions") && r.Method == "POST" {
			var req struct {
				TestName   string `json:"test_name"`
				TestType   string `json:"test_type,omitempty"`
				Experiment string `json:"experiment,omitempty"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.TestName == "" {
				req.TestName = runID
			}
			started := time.Now().UTC().Format(time.RFC3339)
			res, err := db.db.Exec(
				`INSERT INTO test_runs(test_name, started_at, runner, test_type, experiment, daemon_run_id) VALUES (?, ?, 'test', ?, ?, ?)`,
				req.TestName, started, req.TestType, req.Experiment, runID,
			)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			id, _ := res.LastInsertId()
			writeJSON(w, 201, map[string]any{"test_run_id": id})
			return
		}

		if len(parts) == 1 {
			http.Error(w, "not found", 404)
			return
		}

		switch parts[1] {
		case "done":
			if r.Method == "PUT" {
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
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				passed := 1
				if req.Passed != nil && !*req.Passed {
					passed = 0
				}
				db.db.Exec(`UPDATE test_runs SET finished_at=?, passed=?, reason=? WHERE id=?`, now, passed, req.Reason, testRunID)
				if req.InputTokens != nil {
					db.db.Exec(`UPDATE test_runs SET input_tokens=? WHERE id=?`, *req.InputTokens, testRunID)
				}
				if req.OutputTokens != nil {
					db.db.Exec(`UPDATE test_runs SET output_tokens=? WHERE id=?`, *req.OutputTokens, testRunID)
				}
				if req.CostUSD != nil {
					db.db.Exec(`UPDATE test_runs SET cost_usd=? WHERE id=?`, *req.CostUSD, testRunID)
				}
				writeJSON(w, 200, map[string]string{"status": "done"})
				return
			}

		case "events":
			if r.Method == "POST" {
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req struct {
					Kind       string  `json:"kind"`
					Message    string  `json:"message"`
					DurationMs float64 `json:"duration_ms,omitempty"`
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
			if r.Method == "GET" {
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				rows, _ := db.db.Query(`SELECT id, seq, kind, message FROM run_events WHERE run_id=? ORDER BY seq`, testRunID)
				defer rows.Close()
				type ev struct {
					ID      int64  `json:"id"`
					Seq     int    `json:"seq"`
					Kind    string `json:"kind"`
					Message string `json:"message"`
				}
				var out []ev
				for rows.Next() {
					var e ev
					rows.Scan(&e.ID, &e.Seq, &e.Kind, &e.Message)
					out = append(out, e)
				}
				if out == nil {
					out = []ev{}
				}
				writeJSON(w, 200, out)
				return
			}

		case "category", "description", "experiment", "test_type":
			if r.Method == "PUT" {
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req struct{ Value string `json:"value"` }
				json.Unmarshal(body, &req)
				db.db.Exec(fmt.Sprintf(`UPDATE test_runs SET %s=? WHERE id=?`, parts[1]), req.Value, testRunID)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}

		case "tags":
			if r.Method == "PUT" {
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req struct{ Tags []string `json:"tags"` }
				json.Unmarshal(body, &req)
				b, _ := json.Marshal(req.Tags)
				db.db.Exec(`UPDATE test_runs SET tags=? WHERE id=?`, string(b), testRunID)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}

		case "version":
			if r.Method == "PUT" {
				var testRunID int64
				db.db.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
				if testRunID == 0 {
					http.Error(w, "run not found", 404)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req struct {
					AppVersion  string `json:"app_version,omitempty"`
					TestVersion string `json:"test_version,omitempty"`
				}
				json.Unmarshal(body, &req)
				db.db.Exec(`UPDATE test_runs SET app_version=?, test_version=? WHERE id=?`, req.AppVersion, req.TestVersion, testRunID)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}

		case "output":
			if r.Method == "PUT" {
				var testRunID int64
				fmt.Sscanf(runID, "test-%d", &testRunID)
				body, _ := io.ReadAll(r.Body)
				var req struct{ Output string `json:"output"` }
				json.Unmarshal(body, &req)
				db.db.Exec(`UPDATE test_runs SET raw_output=raw_output||? WHERE id=?`, req.Output, testRunID)
				writeJSON(w, 200, map[string]string{"status": "saved"})
				return
			}
		}

		http.Error(w, "not found", 404)
	})

	mux.HandleFunc("/test-runs/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/test-runs/")
		path = strings.TrimSuffix(path, "/")
		parts := strings.SplitN(path, "/", 3)
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "missing id", 400)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			http.Error(w, "invalid id", 400)
			return
		}

		if len(parts) == 1 {
			if r.Method != "GET" {
				http.Error(w, "method not allowed", 405)
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
			return
		}

		switch parts[1] {
		case "events":
			if r.Method == "POST" {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					Kind       string  `json:"kind"`
					Message    string  `json:"message"`
					DurationMs float64 `json:"duration_ms,omitempty"`
				}
				json.Unmarshal(body, &req)
				var maxSeq int
				db.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM run_events WHERE run_id=?`, id).Scan(&maxSeq)
				db.db.Exec(
					`INSERT INTO run_events(run_id, seq, kind, message, elapsed_s, occurred_at) VALUES (?, ?, ?, ?, 0.5, datetime('now'))`,
					id, maxSeq+1, req.Kind, req.Message,
				)
				writeJSON(w, 201, map[string]any{"seq": maxSeq + 1})
				return
			}
			if r.Method == "GET" {
				rows, _ := db.db.Query(`SELECT id, seq, kind, message FROM run_events WHERE run_id=? ORDER BY seq`, id)
				defer rows.Close()
				type ev struct {
					ID      int64  `json:"id"`
					Seq     int    `json:"seq"`
					Kind    string `json:"kind"`
					Message string `json:"message"`
				}
				var out []ev
				for rows.Next() {
					var e ev
					rows.Scan(&e.ID, &e.Seq, &e.Kind, &e.Message)
					out = append(out, e)
				}
				if out == nil {
					out = []ev{}
				}
				writeJSON(w, 200, out)
				return
			}

		case "done":
			if r.Method == "PUT" {
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
				args = append(args, id)
				db.db.Exec(q, args...)
				writeJSON(w, 200, map[string]string{"status": "done"})
				return
			}

		case "metadata":
			if len(parts) < 3 {
				http.Error(w, "missing field", 400)
				return
			}
			field := parts[2]
			if r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)
				var req struct{ Value string `json:"value"` }
				json.Unmarshal(body, &req)
				db.db.Exec(fmt.Sprintf(`UPDATE test_runs SET %s=? WHERE id=?`, field), req.Value, id)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}

		case "output":
			if r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)
				var req struct{ Output string `json:"output"` }
				json.Unmarshal(body, &req)
				db.db.Exec(`UPDATE test_runs SET raw_output=raw_output||? WHERE id=?`, req.Output, id)
				writeJSON(w, 200, map[string]string{"status": "saved"})
				return
			}

		case "tags":
			if r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)
				var req struct{ Tags []string `json:"tags"` }
				json.Unmarshal(body, &req)
				b, _ := json.Marshal(req.Tags)
				db.db.Exec(`UPDATE test_runs SET tags=? WHERE id=?`, string(b), id)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}

		case "versions":
			if r.Method == "PUT" {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					AppVersion  string `json:"app_version,omitempty"`
					TestVersion string `json:"test_version,omitempty"`
				}
				json.Unmarshal(body, &req)
				db.db.Exec(`UPDATE test_runs SET app_version=?, test_version=? WHERE id=?`, req.AppVersion, req.TestVersion, id)
				writeJSON(w, 200, map[string]string{"status": "updated"})
				return
			}
		}
		http.Error(w, "not found", 404)
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
