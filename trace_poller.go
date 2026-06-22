// Package e2eframework — trace_poller.go
//
// TracePoller polls the Memory server's Tempo proxy (/api/traces/search) in
// the background and writes new/updated agent.run spans to the RunDB as
// "trace_span" events.
//
// This provides real-time visibility in the TUI without any per-tick logging
// in test code.  The poller is started when a RunLog is created (when a
// projectID is known) and stopped when RunLog.Close() is called.
//
// Auth note: the poller re-uses the same auth logic as the test HTTP client —
// tokens with an "emt_" prefix are sent as Bearer; everything else as X-API-Key.
package runlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Tempo search response types ───────────────────────────────────────────────

type tempoSearchResp struct {
	Traces []tempoTraceResult `json:"traces"`
}

type tempoTraceResult struct {
	TraceID           string            `json:"traceID"`
	RootServiceName   string            `json:"rootServiceName"`
	RootTraceName     string            `json:"rootTraceName"`
	StartTimeUnixNano string            `json:"startTimeUnixNano"`
	DurationMs        float64           `json:"durationMs"`
	Attributes        map[string]string `json:"attributes"`
	SpanSets          []tempoSpanSetR   `json:"spanSets"`
	SpanSet           *tempoSpanSetR    `json:"spanSet"`
}

type tempoSpanSetR struct {
	Matched int          `json:"matched"`
	Spans   []tempoSpanR `json:"spans"`
}

type tempoSpanR struct {
	SpanID            string       `json:"spanID"`
	StartTimeUnixNano string       `json:"startTimeUnixNano"`
	DurationNanos     string       `json:"durationNanos"`
	Attributes        []tempoAttrR `json:"attributes"`
}

type tempoAttrR struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
		IntValue    string `json:"intValue"`
	} `json:"value"`
}

func (a tempoAttrR) Val() string { //nolint:deadcode
	if a.Value.StringValue != "" {
		return a.Value.StringValue
	}
	return a.Value.IntValue
}

// ── TracePoller ───────────────────────────────────────────────────────────────

// TracePoller polls the Memory server's Tempo proxy for agent.run traces
// associated with a specific project and writes new spans to the RunDB.
type TracePoller struct {
	serverURL string
	token     string
	projectID string
	runID     int64 // RunDB run_id to attach events to
	db        *RunDB

	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu        sync.Mutex
	seen      map[string]float64 // traceID → last durationMs we recorded
	startedAt time.Time
}

// NewTracePoller creates a TracePoller. Call Start() to begin polling.
func NewTracePoller(serverURL, token, projectID string, runDBID int64, db *RunDB) *TracePoller { //nolint:deadcode
	ctx, cancel := context.WithCancel(context.Background())
	return &TracePoller{
		serverURL: serverURL,
		token:     token,
		projectID: projectID,
		runID:     runDBID,
		db:        db,
		interval:  3 * time.Second,
		ctx:       ctx,
		cancel:    cancel,
		seen:      make(map[string]float64),
		startedAt: time.Now(),
	}
}

// Start launches the background polling goroutine.
func (p *TracePoller) Start() { //nolint:deadcode
	p.wg.Add(1)
	go p.loop()
}

// Stop cancels the polling goroutine and waits for it to exit.
func (p *TracePoller) Stop() { //nolint:deadcode
	p.cancel()
	p.wg.Wait()
}

func (p *TracePoller) loop() { //nolint:deadcode
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// Poll immediately on start, then on each tick.
	p.poll()
	for {
		select {
		case <-p.ctx.Done():
			// One final poll to capture any spans that completed just before stop.
			p.poll()
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

// poll queries Tempo for agent.run traces in the project and persists any new
// or updated traces as "trace_span" events in the DB.
func (p *TracePoller) poll() { //nolint:deadcode
	if p.db == nil || p.runID == 0 || p.serverURL == "" {
		return
	}

	// Look back from when we started minus a small buffer.
	since := p.startedAt.Add(-30 * time.Second)
	params := url.Values{
		"limit": {"50"},
		"start": {strconv.FormatInt(since.Unix(), 10)},
		"end":   {strconv.FormatInt(time.Now().Unix(), 10)},
		"q":     {fmt.Sprintf(`{ .memory.project.id = "%s" && rootName = "agent.run" } | select(span.memory.agent.run_id, span.memory.project.id)`, p.projectID)},
	}

	body, err := p.fetch("/api/traces/search", params)
	if err != nil {
		// Tracing may not be enabled on this server; treat as non-fatal.
		return
	}

	var resp tempoSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	// Sort by start time so we process oldest first.
	sort.Slice(resp.Traces, func(i, j int) bool {
		ni, _ := strconv.ParseInt(resp.Traces[i].StartTimeUnixNano, 10, 64)
		nj, _ := strconv.ParseInt(resp.Traces[j].StartTimeUnixNano, 10, 64)
		return ni < nj
	})

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, tr := range resp.Traces {
		prev, alreadySeen := p.seen[tr.TraceID]
		// Emit an event when we first see the trace, or when its duration changes
		// (meaning the root span completed or updated).
		if !alreadySeen || (tr.DurationMs > 0 && tr.DurationMs != prev) {
			p.seen[tr.TraceID] = tr.DurationMs
			p.emitTrace(tr, !alreadySeen)
		}
	}
}

// emitTrace writes one trace_span event to the DB.
func (p *TracePoller) emitTrace(tr tempoTraceResult, isNew bool) { //nolint:deadcode
	startNs, _ := strconv.ParseInt(tr.StartTimeUnixNano, 10, 64)
	var startTime time.Time
	if startNs > 0 {
		startTime = time.Unix(0, startNs)
	}

	status := "in_flight"
	if tr.DurationMs > 0 {
		status = "complete"
	}

	runAgentRunID := tr.Attributes["memory.agent.run_id"]
	// Also check spanSets for the attribute.
	if runAgentRunID == "" {
		for _, ss := range tr.SpanSets {
			for _, sp := range ss.Spans {
				for _, a := range sp.Attributes {
					if a.Key == "memory.agent.run_id" {
						runAgentRunID = a.Val()
					}
				}
			}
		}
		if tr.SpanSet != nil {
			for _, sp := range tr.SpanSet.Spans {
				for _, a := range sp.Attributes {
					if a.Key == "memory.agent.run_id" {
						runAgentRunID = a.Val()
					}
				}
			}
		}
	}

	verb := "updated"
	if isNew {
		verb = "new"
	}

	durStr := "in flight"
	if tr.DurationMs > 0 {
		if tr.DurationMs < 1000 {
			durStr = fmt.Sprintf("%.0fms", tr.DurationMs)
		} else {
			durStr = fmt.Sprintf("%.2fs", tr.DurationMs/1000)
		}
	}

	spanName := tr.RootTraceName
	if spanName == "" {
		spanName = tr.RootServiceName
	}

	msg := fmt.Sprintf("[trace] %s %s %s (%s)", verb, tr.TraceID[:min16(len(tr.TraceID))], spanName, durStr)
	if runAgentRunID != "" {
		msg += " run=" + runAgentRunID
	}

	details := map[string]any{
		"trace_id":       tr.TraceID,
		"root_span_name": tr.RootTraceName,
		"root_service":   tr.RootServiceName,
		"status":         status,
		"duration_ms":    tr.DurationMs,
		"agent_run_id":   runAgentRunID,
		"project_id":     p.projectID,
		"is_new":         isNew,
	}
	if !startTime.IsZero() {
		details["start_time"] = startTime.UTC().Format(time.RFC3339)
	}

	elapsed := time.Since(p.startedAt).Seconds()
	seq := 0 // sequence is best-effort for trace events
	_ = p.db.InsertEvent(p.runID, seq, time.Now(), elapsed, "trace_span", msg, details)
}

// fetch performs an authenticated GET to the Memory server's Tempo proxy.
func (p *TracePoller) fetch(path string, params url.Values) ([]byte, error) { //nolint:deadcode
	u := strings.TrimRight(p.serverURL, "/") + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	SetAuthHeader(req, p.token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traces search returned %d", resp.StatusCode)
	}
	return body, nil
}

func min16(n int) int { //nolint:deadcode
	if n < 16 {
		return n
	}
	return 16
}
