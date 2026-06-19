// cmd/runlog/daemon.go — runlog daemon HTTP server implementation.
//
// The daemon is a long-lived background process that:
//   - Tracks active test runs by PID (registered via POST /runs before syscall.Exec)
//   - Maintains a resource registry (run → project IDs on the remote server)
//   - Runs a background orphan sweeper (60s tick) that deletes server resources
//     when their owning run's process is no longer alive
//   - Runs a run reaper (10s tick) that marks dead any run whose PID has gone away
//
// The daemon is started via "runlog daemon" (re-exec with --daemon flag + Setsid).
// All state is persisted to the existing runs.db SQLite database via new tables
// daemon_runs and daemon_resources (migration 13).
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	runlog "github.com/emergent-company/runlog"
)

// ─────────────────────────────────────────────────────────────────────────────
// UUID helper (stdlib only — no external deps)
// ─────────────────────────────────────────────────────────────────────────────

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// DaemonServer
// ─────────────────────────────────────────────────────────────────────────────

// DaemonServer is the long-lived daemon HTTP process.
type DaemonServer struct {
	db        *runlog.RunDB
	port      int
	startedAt time.Time
	mux       *http.ServeMux
	sweepCh   chan struct{} // non-blocking trigger for immediate sweep
	mu        sync.Mutex
	timeout   time.Duration // max duration before a test run is auto-timed-out
}

// newDaemonServer creates a DaemonServer backed by the given RunDB and port.
func newDaemonServer(db *runlog.RunDB, port int, timeout time.Duration) *DaemonServer {
	d := &DaemonServer{
		db:        db,
		port:      port,
		startedAt: time.Now(),
		mux:       http.NewServeMux(),
		sweepCh:   make(chan struct{}, 1),
		timeout:   timeout,
	}
	d.registerRoutes()
	return d
}

func (d *DaemonServer) registerRoutes() {
	d.mux.HandleFunc("/health", d.handleHealth)
	d.mux.HandleFunc("/runs", d.handleRuns)
	d.mux.HandleFunc("/runs/", d.handleRunsPath)
	d.mux.HandleFunc("/resources/orphaned", d.handleOrphaned)
	d.mux.HandleFunc("/cleanup", d.handleCleanup)
	d.mux.HandleFunc("/reap", d.handleReap)
	d.mux.HandleFunc("/status", d.handleStatus)
	d.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
}

// ServeHTTP implements http.Handler.
func (d *DaemonServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mux.ServeHTTP(w, r)
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: GET /health
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: GET /status
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	activeRuns, totalResources := d.statusCounts()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":            "running",
		"pid":               os.Getpid(),
		"port":              d.port,
		"uptime_s":          int(time.Since(d.startedAt).Seconds()),
		"active_runs":       activeRuns,
		"tracked_resources": totalResources,
	})
}

func (d *DaemonServer) statusCounts() (activeRuns, totalResources int) {
	rawDB := d.db.RawDB()
	row := rawDB.QueryRow(`SELECT COUNT(*) FROM daemon_runs WHERE status='active'`)
	_ = row.Scan(&activeRuns)
	row = rawDB.QueryRow(`SELECT COUNT(*) FROM daemon_resources WHERE status='active'`)
	_ = row.Scan(&totalResources)
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: /runs  (POST = register, GET = list)
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		d.handleRegisterRun(w, r)
	case http.MethodGet:
		d.handleListRuns(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type registerRunRequest struct {
	PID        int    `json:"pid"`
	EnvProfile string `json:"env_profile"`
	ServerURL  string `json:"server_url"`
	Token      string `json:"token"`
}

func (d *DaemonServer) handleRegisterRun(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req registerRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.PID <= 0 {
		http.Error(w, "pid required", http.StatusBadRequest)
		return
	}

	id := newUUID()
	rawDB := d.db.RawDB()
	_, err = rawDB.Exec(
		`INSERT INTO daemon_runs (id, pid, env_profile, server_url, token, status, started_at)
		 VALUES (?, ?, ?, ?, ?, 'active', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		id, req.PID, req.EnvProfile, req.ServerURL, req.Token,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("db insert: %v", err), http.StatusInternalServerError)
		return
	}
	// Also insert into test_runs so dogfood runs appear in the test dashboard.
	testName := req.EnvProfile
	if testName == "" {
		testName = "unnamed"
	}
	_, _ = rawDB.Exec(
		`INSERT INTO test_runs (test_name, started_at, runner, env_name, daemon_run_id)
		 VALUES (?, strftime('%Y-%m-%dT%H:%M:%SZ','now'), 'dogfood', ?, ?)`,
		testName, req.ServerURL, id,
	)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

type runListEntry struct {
	ID            string  `json:"id"`
	PID           int     `json:"pid"`
	EnvProfile    string  `json:"env_profile"`
	Status        string  `json:"status"`
	StartedAt     string  `json:"started_at"`
	FinishedAt    *string `json:"finished_at,omitempty"`
	ResourceCount int     `json:"resource_count"`
}

func (d *DaemonServer) handleListRuns(w http.ResponseWriter, r *http.Request) {
	rawDB := d.db.RawDB()
	rows, err := rawDB.Query(`
		SELECT r.id, r.pid, r.env_profile, r.status, r.started_at, r.finished_at,
		       COUNT(res.id) AS resource_count
		FROM daemon_runs r
		LEFT JOIN daemon_resources res ON res.run_id = r.id AND res.status = 'active'
		WHERE r.status = 'active'
		   OR (r.status IN ('done','dead') AND r.finished_at >= strftime('%Y-%m-%dT%H:%M:%SZ', datetime('now', '-24 hours')))
		GROUP BY r.id
		ORDER BY r.started_at DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("db query: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []runListEntry
	for rows.Next() {
		var e runListEntry
		var finishedAt sql.NullString
		if err := rows.Scan(&e.ID, &e.PID, &e.EnvProfile, &e.Status, &e.StartedAt, &finishedAt, &e.ResourceCount); err != nil {
			continue
		}
		if finishedAt.Valid {
			e.FinishedAt = &finishedAt.String
		}
		result = append(result, e)
	}
	if result == nil {
		result = []runListEntry{}
	}
	writeJSON(w, http.StatusOK, result)
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: /runs/<id>  and  /runs/<id>/done  and  /runs/<id>/resources
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleRunsPath(w http.ResponseWriter, r *http.Request) {
	// Strip leading /runs/
	path := strings.TrimPrefix(r.URL.Path, "/runs/")
	path = strings.TrimSuffix(path, "/")

	parts := strings.SplitN(path, "/", 3)
	// parts[0] = run ID
	// parts[1] = "done" | "resources" (optional)
	// parts[2] = resource_id (optional, for DELETE)

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing run id", http.StatusBadRequest)
		return
	}

	runID := parts[0]

	if len(parts) == 1 {
		// GET /runs/:id
		if r.Method == http.MethodGet {
			d.handleGetRun(w, r, runID)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch parts[1] {
	case "done":
		if r.Method == http.MethodPut {
			d.handleMarkRunDone(w, r, runID)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

	case "events":
		if r.Method == http.MethodPost && len(parts) == 2 {
			d.handleInsertEvent(w, r, runID)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

	case "resources":
		if len(parts) == 2 {
			// POST /runs/:id/resources
			if r.Method == http.MethodPost {
				d.handleRegisterResource(w, r, runID)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// DELETE /runs/:id/resources/:resource_id
		resourceID := parts[2]
		if r.Method == http.MethodDelete {
			d.handleDeregisterResource(w, r, runID, resourceID)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

type runDetail struct {
	runListEntry
	Resources []resourceEntry `json:"resources"`
}

func (d *DaemonServer) handleGetRun(w http.ResponseWriter, r *http.Request, runID string) {
	rawDB := d.db.RawDB()
	var entry runListEntry
	var finishedAt sql.NullString
	err := rawDB.QueryRow(`SELECT id, pid, env_profile, status, started_at, finished_at FROM daemon_runs WHERE id = ?`, runID).
		Scan(&entry.ID, &entry.PID, &entry.EnvProfile, &entry.Status, &entry.StartedAt, &finishedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("db: %v", err), http.StatusInternalServerError)
		return
	}
	if finishedAt.Valid {
		entry.FinishedAt = &finishedAt.String
	}

	rows, err := rawDB.Query(`SELECT id, resource_id, resource_type, status, created_at FROM daemon_resources WHERE run_id = ?`, runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("db resources: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var resources []resourceEntry
	for rows.Next() {
		var res resourceEntry
		if err := rows.Scan(&res.ID, &res.ResourceID, &res.ResourceType, &res.Status, &res.CreatedAt); err != nil {
			continue
		}
		resources = append(resources, res)
	}
	if resources == nil {
		resources = []resourceEntry{}
	}
	entry.ResourceCount = len(resources)

	writeJSON(w, http.StatusOK, runDetail{runListEntry: entry, Resources: resources})
}

type markDoneRequest struct {
	Passed *bool `json:"passed,omitempty"` // nil = assume pass
}

func (d *DaemonServer) handleMarkRunDone(w http.ResponseWriter, r *http.Request, runID string) {
	rawDB := d.db.RawDB()
	res, err := rawDB.Exec(`
		UPDATE daemon_runs
		SET status='done', finished_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id=? AND status='active'`, runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("db: %v", err), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Already done/dead or not found — still 200 (idempotent)
	}

	// Check for optional pass/fail status in request body.
	passed := true // default: run completed = pass
	if r.Body != nil {
		var req markDoneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Passed != nil {
			passed = *req.Passed
		}
	}

	// Also update the linked test_run so the dashboard reflects completion.
	_, _ = rawDB.Exec(`
		UPDATE test_runs
		SET finished_at=strftime('%Y-%m-%dT%H:%M:%SZ','now'), passed=?
		WHERE daemon_run_id=?`, passed, runID)
	// Trigger immediate non-blocking sweep
	select {
	case d.sweepCh <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "done"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: POST /runs/:id/events
// ─────────────────────────────────────────────────────────────────────────────

type insertEventRequest struct {
	Kind    string         `json:"kind"`
	Message string         `json:"message"`
	Elapsed float64        `json:"elapsed_s"`
	Details map[string]any `json:"details,omitempty"`
}

func (d *DaemonServer) handleInsertEvent(w http.ResponseWriter, r *http.Request, runID string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req insertEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Kind == "" {
		http.Error(w, "kind required", http.StatusBadRequest)
		return
	}

	rawDB := d.db.RawDB()
	// Find the test_runs ID linked by daemon_run_id
	var testRunID int64
	err = rawDB.QueryRow(`SELECT id FROM test_runs WHERE daemon_run_id=?`, runID).Scan(&testRunID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	// Get next sequence number
	var maxSeq int
	_ = rawDB.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM run_events WHERE run_id=?`, testRunID).Scan(&maxSeq)
	seq := maxSeq + 1

	var detailsJSON *string
	if len(req.Details) > 0 {
		b, err := json.Marshal(req.Details)
		if err == nil {
			s := string(b)
			detailsJSON = &s
		}
	}
	_, err = rawDB.Exec(
		`INSERT INTO run_events (run_id, seq, occurred_at, elapsed_s, kind, message, details)
		 VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%SZ','now'), ?, ?, ?, ?)`,
		testRunID, seq, req.Elapsed, req.Kind, req.Message, detailsJSON,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("db insert: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"seq": seq})
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: POST /runs/:id/resources  and  DELETE /runs/:id/resources/:resource_id
// ─────────────────────────────────────────────────────────────────────────────

type resourceEntry struct {
	ID           string  `json:"id"`
	ResourceID   string  `json:"resource_id"`
	ResourceType string  `json:"resource_type"`
	RunID        string  `json:"run_id,omitempty"`
	RunStatus    string  `json:"run_status,omitempty"`
	ServerURL    string  `json:"server_url,omitempty"`
	Token        string  `json:"-"` // never returned in JSON
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	DeletedAt    *string `json:"deleted_at,omitempty"`
}

type registerResourceRequest struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
}

func (d *DaemonServer) handleRegisterResource(w http.ResponseWriter, r *http.Request, runID string) {
	// Verify run exists
	rawDB := d.db.RawDB()
	var serverURL, token string
	err := rawDB.QueryRow(`SELECT server_url, token FROM daemon_runs WHERE id=?`, runID).
		Scan(&serverURL, &token)
	if err == sql.ErrNoRows {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("db: %v", err), http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req registerResourceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ResourceID == "" {
		http.Error(w, "resource_id required", http.StatusBadRequest)
		return
	}
	resourceType := req.ResourceType
	if resourceType == "" {
		resourceType = "project"
	}

	id := newUUID()
	_, err = rawDB.Exec(`
		INSERT INTO daemon_resources (id, run_id, resource_id, resource_type, server_url, token, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
		id, runID, req.ResourceID, resourceType, serverURL, token,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("db insert: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (d *DaemonServer) handleDeregisterResource(w http.ResponseWriter, r *http.Request, runID, resourceID string) {
	rawDB := d.db.RawDB()
	_, err := rawDB.Exec(`
		UPDATE daemon_resources
		SET status='deleted', deleted_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE run_id=? AND resource_id=? AND status='active'`,
		runID, resourceID,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("db: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: GET /resources/orphaned
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleOrphaned(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := queryOrphaned(d.db.RawDB())
	if err != nil {
		http.Error(w, fmt.Sprintf("db: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

type orphanEntry struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	RunID        string `json:"run_id"`
	RunStatus    string `json:"run_status"`
	ServerURL    string `json:"server_url"`
	CreatedAt    string `json:"created_at"`
}

func queryOrphaned(rawDB *sql.DB) ([]orphanEntry, error) {
	rows, err := rawDB.Query(`
		SELECT res.resource_id, res.resource_type, res.run_id, r.status, res.server_url, res.created_at
		FROM daemon_resources res
		JOIN daemon_runs r ON r.id = res.run_id
		WHERE res.status = 'active' AND r.status IN ('done','dead')
		ORDER BY res.created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []orphanEntry
	for rows.Next() {
		var e orphanEntry
		if err := rows.Scan(&e.ResourceID, &e.ResourceType, &e.RunID, &e.RunStatus, &e.ServerURL, &e.CreatedAt); err != nil {
			continue
		}
		result = append(result, e)
	}
	if result == nil {
		result = []orphanEntry{}
	}
	return result, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: POST /reap — synchronous timeout reap
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleReap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.reapTimedOutRuns()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ─────────────────────────────────────────────────────────────────────────────
// Route: POST /cleanup — synchronous orphan sweep
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	deleted, failed := sweepOrphans(d.db.RawDB())
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted, "failed": failed})
}

// ─────────────────────────────────────────────────────────────────────────────
// Orphan sweeper
// ─────────────────────────────────────────────────────────────────────────────

// sweepOrphans finds all orphaned resources (active resources whose run is done/dead),
// attempts to DELETE each from the remote server, and marks them deleted on success.
// 404 from the server is treated as success (idempotent). 5xx and other errors
// leave the resource as active (will be retried on next tick).
// Returns counts of deleted and failed.
func sweepOrphans(rawDB *sql.DB) (deleted, failed int) {
	type sweepItem struct {
		id         string
		resourceID string
		serverURL  string
		token      string
	}

	rows, err := rawDB.Query(`
		SELECT res.id, res.resource_id, res.server_url, res.token
		FROM daemon_resources res
		JOIN daemon_runs r ON r.id = res.run_id
		WHERE res.status = 'active' AND r.status IN ('done','dead')
	`)
	if err != nil {
		log.Printf("daemon: sweep query: %v", err)
		return
	}
	defer rows.Close()

	var items []sweepItem
	for rows.Next() {
		var item sweepItem
		if err := rows.Scan(&item.id, &item.resourceID, &item.serverURL, &item.token); err != nil {
			continue
		}
		items = append(items, item)
	}
	_ = rows.Close()

	client := &http.Client{Timeout: 15 * time.Second}
	for _, item := range items {
		url := strings.TrimRight(item.serverURL, "/") + "/api/projects/" + item.resourceID
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			log.Printf("daemon: sweep build request for %s: %v", item.resourceID, err)
			failed++
			continue
		}
		req.Header.Set("X-API-Key", item.token)

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("daemon: sweep DELETE %s: %v", item.resourceID, err)
			failed++
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
			// Success or already gone — mark deleted
			_, dbErr := rawDB.Exec(`
				UPDATE daemon_resources
				SET status='deleted', deleted_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
				WHERE id=?`, item.id)
			if dbErr != nil {
				log.Printf("daemon: sweep mark deleted %s: %v", item.id, dbErr)
				failed++
			} else {
				deleted++
				log.Printf("daemon: swept resource %s (project %s)", item.id, item.resourceID)
			}
		} else {
			log.Printf("daemon: sweep DELETE %s: server returned %d", item.resourceID, resp.StatusCode)
			failed++
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// Sweeper goroutine — periodic + triggered
// ─────────────────────────────────────────────────────────────────────────────

// runSweeper runs the orphan sweeper loop: ticks every 60 seconds and also
// responds to immediate triggers sent on d.sweepCh.
func (d *DaemonServer) runSweeper() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			deleted, failed := sweepOrphans(d.db.RawDB())
			if deleted > 0 || failed > 0 {
				log.Printf("daemon: sweep tick: deleted=%d failed=%d", deleted, failed)
			}
		case <-d.sweepCh:
			deleted, failed := sweepOrphans(d.db.RawDB())
			if deleted > 0 || failed > 0 {
				log.Printf("daemon: sweep immediate: deleted=%d failed=%d", deleted, failed)
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Run reaper goroutine
// ─────────────────────────────────────────────────────────────────────────────

// runReaper polls every 10 seconds; for each active run whose PID is dead,
// marks it as dead and triggers an immediate sweep.
func (d *DaemonServer) runReaper() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		d.reapDeadRuns()
	}
}

func (d *DaemonServer) reapDeadRuns() {
	rawDB := d.db.RawDB()
	rows, err := rawDB.Query(`SELECT id, pid FROM daemon_runs WHERE status='active'`)
	if err != nil {
		log.Printf("daemon: reaper query: %v", err)
		return
	}
	defer rows.Close()

	type activeRun struct {
		id  string
		pid int
	}
	var runs []activeRun
	for rows.Next() {
		var run activeRun
		if err := rows.Scan(&run.id, &run.pid); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	_ = rows.Close()

	for _, run := range runs {
		if !processAlive(run.pid) {
			_, err := rawDB.Exec(`
				UPDATE daemon_runs
				SET status='dead', finished_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
				WHERE id=? AND status='active'`, run.id)
			if err != nil {
				log.Printf("daemon: reaper mark dead %s: %v", run.id, err)
				continue
			}
			log.Printf("daemon: reaped dead run %s (pid %d)", run.id, run.pid)
			// Trigger immediate sweep
			select {
			case d.sweepCh <- struct{}{}:
			default:
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test timeout watcher — marks test_runs as failed if no events within
// the configured timeout period.
// ─────────────────────────────────────────────────────────────────────────────

// reapTimedOutRuns finds test_runs with finished_at IS NULL that started
// longer ago than d.timeout, kills the launcher process if alive, and marks
// the run as FAIL with reason "timed out".
func (d *DaemonServer) reapTimedOutRuns() {
	cutoff := time.Now().Add(-d.timeout).UTC().Format(time.RFC3339)

	rawDB := d.db.RawDB()
	rows, err := rawDB.Query(`
		SELECT t.id, t.test_name
		FROM test_runs t
		WHERE t.finished_at IS NULL AND t.started_at < ?
	`, cutoff)
	if err != nil {
		log.Printf("daemon: timeout query: %v", err)
		return
	}
	defer rows.Close()

	type staleRun struct {
		id       int64
		testName string
	}
	var stale []staleRun
	for rows.Next() {
		var s staleRun
		if err := rows.Scan(&s.id, &s.testName); err != nil {
			continue
		}
		stale = append(stale, s)
	}
	_ = rows.Close()

	for _, s := range stale {
		// Kill the launcher process if still alive
		var pid int
		err := rawDB.QueryRow(`
			SELECT launcher_pid FROM test_launchers
			WHERE test_name = ? AND finished_at IS NULL
			ORDER BY launched_at DESC LIMIT 1
		`, s.testName).Scan(&pid)
		if err == nil && pid > 0 {
			proc, err := os.FindProcess(pid)
			if err == nil {
				_ = proc.Kill()
			}
			_, _ = rawDB.Exec(
				`UPDATE test_launchers SET finished_at = ? WHERE test_name = ? AND finished_at IS NULL`,
				time.Now().UTC().Format(time.RFC3339Nano), s.testName,
			)
		}

		_ = d.db.FinishRun(s.id, time.Now(), runlog.OutcomeTimeout, "timed out")
		log.Printf("daemon: timed out run %d (%s)", s.id, s.testName)
	}
}

// runTimeoutWatcher polls every 30 seconds and reaps timed-out runs.
func (d *DaemonServer) runTimeoutWatcher() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		d.reapTimedOutRuns()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Startup recovery: reap stale active runs from before a daemon restart
// ─────────────────────────────────────────────────────────────────────────────

func (d *DaemonServer) recoverStaleRuns() {
	rawDB := d.db.RawDB()
	rows, err := rawDB.Query(`SELECT id, pid FROM daemon_runs WHERE status='active'`)
	if err != nil {
		log.Printf("daemon: recovery query: %v", err)
		return
	}
	defer rows.Close()

	type activeRun struct {
		id  string
		pid int
	}
	var stale []activeRun
	for rows.Next() {
		var run activeRun
		if err := rows.Scan(&run.id, &run.pid); err != nil {
			continue
		}
		if !processAlive(run.pid) {
			stale = append(stale, run)
		}
	}
	_ = rows.Close()

	for _, run := range stale {
		_, err := rawDB.Exec(`
			UPDATE daemon_runs
			SET status='dead', finished_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
			WHERE id=? AND status='active'`, run.id)
		if err != nil {
			log.Printf("daemon: recovery mark dead %s: %v", run.id, err)
			continue
		}
		log.Printf("daemon: recovery: reaped stale run %s (pid %d)", run.id, run.pid)
	}

	if len(stale) > 0 {
		// Sweep orphans created by the stale runs
		deleted, failed := sweepOrphans(rawDB)
		log.Printf("daemon: recovery sweep: deleted=%d failed=%d", deleted, failed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Start — listens for connections, returns the listener and a channel
// ─────────────────────────────────────────────────────────────────────────────

// Start starts the daemon HTTP server on d.port, runs the sweeper and reaper
// goroutines, and blocks until the returned stop function is called or the
// listener is closed.
func (d *DaemonServer) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", d.port))
	if err != nil {
		return fmt.Errorf("daemon: listen on port %d: %w", d.port, err)
	}

	// Startup recovery before accepting connections
	d.recoverStaleRuns()

	go d.runSweeper()
	go d.runReaper()
	go d.runTimeoutWatcher()

	log.Printf("daemon: listening on port %d (pid %d) timeout=%s", d.port, os.Getpid(), d.timeout)
	srv := &http.Server{Handler: d}
	return srv.Serve(ln)
}
