package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/emergent-company/go-daisy/components/layout"
	"github.com/emergent-company/go-daisy/devmode"
	"github.com/emergent-company/go-daisy/render"
	"github.com/emergent-company/go-daisy/staticfs"
	runlog "github.com/emergent-company/runlog"
	"github.com/labstack/echo/v4"
)

// ─────────────────────────────────────────────────────────────────────────────
// Sidebar
// ─────────────────────────────────────────────────────────────────────────────

var sidebarGroups = []layout.SidebarGroup{
	{
		Label: "Navigation",
		Items: []layout.SidebarItem{
			{Label: "Dashboard", Href: "/ui/", Icon: "lucide--layout-dashboard"},
			{Label: "Tests", Href: "/ui/tests", Icon: "lucide--flask-conical"},
			{Label: "All Runs", Href: "/ui/runs", Icon: "lucide--list"},
			{Label: "Linters", Href: "/ui/linters", Icon: "lucide--shield"},
		},
	},
	{
		Label: "Reference",
		Items: []layout.SidebarItem{
			{Label: "SDK", Href: "/ui/events", Icon: "lucide--file-text"},
			{Label: "Experiments", Href: "/ui/experiments", Icon: "lucide--layers"},
		},
	},
}

// ─────────────────────────────────────────────────────────────────────────────
// Template data types
// ─────────────────────────────────────────────────────────────────────────────

type dashboardData struct {
	TotalTests int
	TotalRuns  int
	PassCount  int
	FailCount  int
	SkipCount  int
	RecentRuns []runlog.RunRow
	Categories []catSummary
}

type catSummary struct {
	Name      string
	TestCount int
}

type testListData struct {
	Categories   []testListCategory
	ActiveFilter string
	StatusFilter string
}

type testListCategory struct {
	Name  string
	Tests []testListEntry
}

type testListEntry struct {
	Name       string
	LastStatus string
	LastRunAt  string
	RunCount   int
	NeverRun   bool
}

type trendPoint struct {
	DurationMS float64
	CostUSD    float64
	Index      int
}

type testStats struct {
	AvgDuration string
	MinDuration string
	MaxDuration string
	PassRate    string
	TotalRuns   int
	TrendData   []trendPoint
	TrendUp     bool
	TrendFlat   bool
	HasCostData bool
	AvgCost     string
}

type runFilters struct {
	Category string
	Status   string
	Since    string
	Search   string
	Tags     string
	HasCost  bool
	Offset   int
}

type testDetailData struct {
	TestName    string
	Runs        []runlog.RunRow
	TotalRuns   int
	Offset      int
	Limit       int
	Stats       *testStats
	TagFilter   string
	HasCostData bool
}

type runDetailData struct {
	Run            runlog.RunRow
	TimelineEvents []runlog.EventRow
	MetaEvents     []runlog.EventRow
	ShowDebug      bool
	IsActive       bool
	SSEURL         string
}

// ── Time formatting ──────────────────────────────────────────────────────────

type TimeFormat int

const (
	TimeFull  TimeFormat = iota // 10/06/2026 11:27:59
	TimeShort                   // 11:27:59
	TimeHuman                   // 2 min ago
	TimeISO                     // 2026-06-10T11:27:59Z
)

func formatTime(t time.Time, f TimeFormat) string {
	switch f {
	case TimeFull:
		return t.Format("15:04:05 02-01-2006")
	case TimeShort:
		return t.Format("15:04:05")
	case TimeISO:
		return t.Format(time.RFC3339)
	case TimeHuman:
		return relativeTime(t)
	default:
		return t.Format("02/01/2006 15:04:05")
	}
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		return t.Format("02/01/2006")
	}
}

// ansiToHTML converts ANSI escape codes to HTML span elements with color classes.
func ansiToHTML(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return html.EscapeString(s)
	}
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+2 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			code := s[i+2 : j]
			if code == "0" {
				buf.WriteString(`</span>`)
			} else if code == "1" {
				buf.WriteString(`<span class="font-bold">`)
			} else {
				colorClass := ""
				switch code {
				case "90":
					colorClass = "text-base-content/50"
				case "32":
					colorClass = "text-success"
				case "33":
					colorClass = "text-warning"
				case "31":
					colorClass = "text-error"
				case "34":
					colorClass = "text-info"
				case "35":
					colorClass = "text-secondary"
				case "36":
					colorClass = "text-accent"
				case "37":
					colorClass = "text-base-content"
				}
				if colorClass != "" {
					buf.WriteString(`<span class="` + colorClass + `">`)
				}
			}
			i = j + 1
		} else {
			buf.WriteByte(s[i])
			i++
		}
	}
	return buf.String()
}

// metaRunEventKinds are event kinds that carry run-level metadata (tags, tokens,
// state transitions) rather than execution timeline steps. Hidden from timeline
// by default; shown in debug mode or reflected in run metadata/stats.
var metaRunEventKinds = map[string]bool{
	"tag":           true,
	"state_change":  true,
	"token_usage":   true,
	"token_summary": true,
	"metric":        true,
	"app_version":   true,
	"test_version":  true,
}

type eventChildrenData struct {
	EventID  int64
	Children []runlog.ChildEvent
	Details  string
	Kind     string
}

type launchData struct {
	TestName string
	SSEURL   string
}

// ── Linter template data types ───────────────────────────────────────────────

type linterListEntry struct {
	Name       string
	Command    string
	LastStatus string // pass / fail / error / never_run
	LastRunAt  string
	RunCount   int
	NeverRun   bool
}

type linterListData struct {
	Linters []linterListEntry
}

type linterDetailData struct {
	Name      string
	Command   string
	Runs      []runlog.LinterRow
	TotalRuns int
	Offset    int
	Limit     int
}

type linterLauncherData struct {
	LinterName string
	SSEURL     string
	Command    string
}

type linterRunDetailData struct {
	LinterName string
	Run        runlog.LinterRow
}

// ─────────────────────────────────────────────────────────────────────────────
// LauncherManager — tracks running test processes, streams output via SSE
// ─────────────────────────────────────────────────────────────────────────────

type ActiveLaunch struct {
	TestName   string
	Cmd        *exec.Cmd
	Started    time.Time
	LauncherID int64
	RunID      int64

	mu        sync.Mutex
	lines     []string
	listeners []chan string
	done      chan struct{}
	exitCode  int
}

type LauncherManager struct {
	mu       sync.Mutex
	launches map[string]*ActiveLaunch
	db       *runlog.RunDB
	config   *runlog.Config
}

func NewLauncherManager(db *runlog.RunDB, config *runlog.Config) *LauncherManager {
	return &LauncherManager{
		launches: make(map[string]*ActiveLaunch),
		db:       db,
		config:   config,
	}
}

func (lm *LauncherManager) Launch(testName string, runID int64, extraEnv ...map[string]string) (*ActiveLaunch, error) {
	lm.mu.Lock()
	if _, ok := lm.launches[testName]; ok {
		lm.mu.Unlock()
		return nil, fmt.Errorf("test %q is already running", testName)
	}

	expanded := lm.config.BuildTestCommand(testName)
	now := time.Now()

	cmd := exec.Command("sh", "-c", expanded)
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), envMapToSlice(extraEnv[0])...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		lm.mu.Unlock()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		lm.mu.Unlock()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	al := &ActiveLaunch{
		TestName: testName,
		Cmd:      cmd,
		Started:  now,
		done:     make(chan struct{}),
	}
	lm.launches[testName] = al
	lm.mu.Unlock()

	if err := cmd.Start(); err != nil {
		lm.mu.Lock()
		delete(lm.launches, testName)
		lm.mu.Unlock()
		return nil, fmt.Errorf("start: %w", err)
	}

	pid := cmd.Process.Pid
	id, _ := lm.db.InsertLauncher(testName, "", now, pid)
	al.LauncherID = id
	al.RunID = runID

	seq := 0
	go func() {
		mr := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(mr)
		for scanner.Scan() {
			line := scanner.Text()
			al.broadcast(line)
			if skipLogLine(line) {
				continue
			}
			seq++
			kind := lineEventKind(line)
			if al.RunID != 0 && kind != "" {
				_ = lm.db.InsertEvent(al.RunID, seq, time.Now(), time.Since(al.Started).Seconds(), kind, line, nil)
			}
		}
		err := cmd.Wait()
		lm.db.FinishLauncher(id, time.Now())
		al.exitCode = 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				al.exitCode = exitErr.ExitCode()
			} else {
				al.exitCode = -1
			}
		}
		if al.RunID != 0 {
			outcome := runlog.OutcomePass
			if al.exitCode != 0 {
				outcome = runlog.OutcomeFail
			} else if lm.db.HasSkipEvent(al.RunID) {
				outcome = runlog.OutcomeSkip
			}
			lm.db.FinishRun(al.RunID, time.Now(), outcome, "")
		}
		close(al.done)
		lm.mu.Lock()
		delete(lm.launches, testName)
		lm.mu.Unlock()
	}()

	return al, nil
}

func envMapToSlice(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, k+"="+v)
	}
	return s
}

func (lm *LauncherManager) Get(testName string) *ActiveLaunch {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.launches[testName]
}

func (al *ActiveLaunch) broadcast(line string) {
	al.mu.Lock()
	al.lines = append(al.lines, line)
	for _, ch := range al.listeners {
		select {
		case ch <- line:
		default:
		}
	}
	al.mu.Unlock()
}

func (al *ActiveLaunch) Subscribe() chan string {
	ch := make(chan string, 256)
	al.mu.Lock()
	for _, line := range al.lines {
		select {
		case ch <- line:
		default:
		}
	}
	al.listeners = append(al.listeners, ch)
	al.mu.Unlock()
	return ch
}

func (al *ActiveLaunch) Unsubscribe(ch chan string) {
	al.mu.Lock()
	for i, l := range al.listeners {
		if l == ch {
			al.listeners = append(al.listeners[:i], al.listeners[i+1:]...)
			break
		}
	}
	al.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// LinterManager — tracks running linter processes, streams output via SSE
// ─────────────────────────────────────────────────────────────────────────────

type ActiveLinter struct {
	Name    string
	Cmd     *exec.Cmd
	Started time.Time
	RunID   int64

	mu        sync.Mutex
	lines     []string
	listeners []chan string
	done      chan struct{}
	exitCode  int
}

type LinterManager struct {
	mu      sync.Mutex
	runs    map[string]*ActiveLinter
	db      *runlog.RunDB
	sse     *SSEBroker
	workDir string
}

func NewLinterManager(db *runlog.RunDB, sse *SSEBroker, workDir string) *LinterManager {
	return &LinterManager{
		runs:    make(map[string]*ActiveLinter),
		db:      db,
		sse:     sse,
		workDir: workDir,
	}
}

func (lm *LinterManager) Lint(name, command string) (*ActiveLinter, error) {
	lm.mu.Lock()
	if _, ok := lm.runs[name]; ok {
		lm.mu.Unlock()
		return nil, fmt.Errorf("linter %q is already running", name)
	}

	now := time.Now()
	cmd := exec.Command("sh", "-c", command)
	if lm.workDir != "" {
		cmd.Dir = lm.workDir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		lm.mu.Unlock()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		lm.mu.Unlock()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	al := &ActiveLinter{
		Name:    name,
		Cmd:     cmd,
		Started: now,
		done:    make(chan struct{}),
	}
	lm.runs[name] = al
	lm.mu.Unlock()

	// Create DB row
	runID, err := lm.db.InsertLinterRun(name, command, now)
	if err != nil {
		lm.mu.Lock()
		delete(lm.runs, name)
		lm.mu.Unlock()
		return nil, fmt.Errorf("insert linter run: %w", err)
	}
	al.RunID = runID

	if err := cmd.Start(); err != nil {
		lm.mu.Lock()
		delete(lm.runs, name)
		lm.mu.Unlock()
		return nil, fmt.Errorf("start: %w", err)
	}

	var outputBuf strings.Builder
	go func() {
		mr := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(mr)
		for scanner.Scan() {
			line := scanner.Text()
			al.broadcast(line)
			outputBuf.WriteString(line + "\n")
		}
		err := cmd.Wait()
		al.exitCode = 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				al.exitCode = exitErr.ExitCode()
			} else {
				al.exitCode = -1
			}
		}
		status := "passed"
		if al.exitCode != 0 {
			status = "failed"
		}
		_ = lm.db.UpdateLinterRunResult(al.RunID, status, al.exitCode, outputBuf.String(), time.Now())
		close(al.done)
		if lm.sse != nil {
			data, _ := json.Marshal(map[string]interface{}{
				"name":      name,
				"status":    status,
				"exit_code": al.exitCode,
			})
			lm.sse.Publish("linters", SSEEvent{Event: "linter-done", Data: string(data)})
		}
		lm.mu.Lock()
		delete(lm.runs, name)
		lm.mu.Unlock()
	}()

	return al, nil
}

func (lm *LinterManager) Running() []string {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	var names []string
	for name := range lm.runs {
		names = append(names, name)
	}
	return names
}

func (lm *LinterManager) Get(name string) *ActiveLinter {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.runs[name]
}

func (al *ActiveLinter) broadcast(line string) {
	al.mu.Lock()
	al.lines = append(al.lines, line)
	for _, ch := range al.listeners {
		select {
		case ch <- line:
		default:
		}
	}
	al.mu.Unlock()
}

func (al *ActiveLinter) Subscribe() chan string {
	ch := make(chan string, 256)
	al.mu.Lock()
	for _, line := range al.lines {
		select {
		case ch <- line:
		default:
		}
	}
	al.listeners = append(al.listeners, ch)
	al.mu.Unlock()
	return ch
}

func (al *ActiveLinter) Unsubscribe(ch chan string) {
	al.mu.Lock()
	for i, l := range al.listeners {
		if l == ch {
			al.listeners = append(al.listeners[:i], al.listeners[i+1:]...)
			break
		}
	}
	al.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// statusFromRun returns the status string for a RunRow.
func statusFromRun(r runlog.RunRow) string {
	if r.FinishedAt == nil {
		return "running"
	}
	if r.Skipped {
		return "skip"
	}
	if r.Passed != nil && *r.Passed {
		return "pass"
	}
	if r.Reason != nil && *r.Reason == "timed out" {
		return "timeout"
	}
	return "fail"
}

// ─────────────────────────────────────────────────────────────────────────────
// WebApp
// ─────────────────────────────────────────────────────────────────────────────

type WebApp struct {
	echo      *echo.Echo
	db        *runlog.RunDB
	config    *runlog.Config
	lm        *LauncherManager
	linterMgr *LinterManager
	sse       *SSEBroker
	startedAt time.Time
	workDir   string
}

func newWebApp(db *runlog.RunDB, config *runlog.Config, workDir string) *WebApp {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Custom HTTP error handler: return HTML instead of JSON for all errors.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		var msg string
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			msg = fmt.Sprintf("%v", he.Message)
		} else {
			msg = err.Error()
		}
		if msg == "" {
			msg = http.StatusText(code)
		}
		// For HTMX requests return a partial; for full requests return a page.
		if render.IsPartial(c.Request()) {
			html := fmt.Sprintf(`<div class="p-6 text-center"><h2 class="text-xl font-bold text-error mb-2">Error %d</h2><p class="text-base-content/70">%s</p></div>`, code, html.EscapeString(msg))
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			c.Response().WriteHeader(code)
			c.Response().Write([]byte(html))
		} else {
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			c.Response().WriteHeader(code)
			c.Response().Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><body><div style="padding:2rem;text-align:center"><h1>Error %d</h1><p>%s</p></div></body></html>`, code, html.EscapeString(msg))))
		}
	}

	sseBroker := newSSEBroker(db)
	app := &WebApp{
		echo:      e,
		db:        db,
		config:    config,
		lm:        NewLauncherManager(db, config),
		linterMgr: NewLinterManager(db, sseBroker, workDir),
		sse:       sseBroker,
		startedAt: time.Now(),
		workDir:   workDir,
	}

	app.sse.SetLinterManager(app.linterMgr)
	go app.sse.Run(context.Background())
	go app.runTimeoutWorker(context.Background())

	// Static assets from go-daisy
	e.StaticFS("/static", staticfs.FS())

	// Enable go-daisy dev mode on every request so data-component attributes
	// are emitted — feedback-overlay uses them to show component names in tooltips.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.SetRequest(c.Request().WithContext(
				devmode.WithDevMode(c.Request().Context()),
			))
			return next(c)
		}
	})

	// Routes
	e.GET("/", app.handleDashboard)
	e.GET("/events", app.handleEventsReference)
	e.GET("/footer-status", app.handleFooterStatus)
	e.GET("/experiments", app.handleExperiments)
	e.GET("/experiments/:name", app.handleExperimentDetail)
	e.GET("/tests", app.handleTests)
	e.GET("/tests/:name", app.handleTestDetail)
	e.GET("/runs", app.handleAllRuns)
	e.GET("/runs/:id", app.handleRunDetail)
	e.GET("/runs/:id/events/:eventID", app.handleEventChildren)
	e.GET("/runs/:id/events-table", app.handleRunEventsTable)
	e.GET("/runs/:id/status", app.handleRunStatusSSE)
	e.GET("/stream", app.handleSSEStream)
	e.POST("/launch/:name", app.handleLaunchTest)
	e.GET("/launch/:name", app.handleLaunchTest)
	e.GET("/launch/:name/events", app.handleLaunchEvents)

	// Linter routes
	e.GET("/linters", app.handleLinters)
	e.GET("/linters/events", app.handleLinterEventsStream)
	e.GET("/linters/:name", app.handleLinterDetail)
	e.GET("/linters/:name/runs/:runID", app.handleLinterRunDetail)
	e.POST("/linters/:name/run", app.handleRunLinter)
	e.POST("/linters/run-all", app.handleRunAllLinters)
	e.GET("/linters/:name/events", app.handleLinterEvents)

	return app
}

// linterDefs returns the list of configured linters, falling back to lefthook
// discovery if no linters are configured in the config file.
func (app *WebApp) linterDefs() []runlog.LinterDef {
	if len(app.config.Linters) > 0 {
		return app.config.Linters
	}
	precommit, _ := runlog.DiscoverLintersFromLefthook(app.workDir, "pre-commit")
	lint, _ := runlog.DiscoverLintersFromLefthook(app.workDir, "lint")
	return runlog.MergeLinters(precommit, lint)
}

func (app *WebApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.echo.ServeHTTP(w, r)
}

// runTimeoutWorker periodically checks for runs that have exceeded their
// per-test timeout (timeout_seconds) and marks them as timed out.
func (app *WebApp) runTimeoutWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rawDB := app.db.RawDB()
			rows, err := rawDB.Query(`
				SELECT id, test_name
				FROM test_runs
				WHERE finished_at IS NULL
				  AND (
				    (timeout_seconds IS NOT NULL AND datetime(started_at, '+' || timeout_seconds || ' seconds') < datetime('now'))
				    OR
				    (timeout_seconds IS NULL AND datetime(started_at, '+300 seconds') < datetime('now'))
				  )
			`)
			if err != nil {
				continue
			}
			var stale []struct {
				id       int64
				testName string
			}
			for rows.Next() {
				var s struct {
					id       int64
					testName string
				}
				if err := rows.Scan(&s.id, &s.testName); err == nil {
					stale = append(stale, s)
				}
			}
			rows.Close()

			for _, s := range stale {
				if err := app.db.FinishRun(s.id, time.Now(), runlog.OutcomeTimeout, "timed out"); err != nil {
					continue
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// reqCtx returns a context with the request context from Echo.
func reqCtx(c echo.Context) context.Context {  //nolint:deadcode
	return c.Request().Context()
}

// skipLogLine returns true for lines that should NOT be persisted as run_events.
// These are lines from go test -v that are already handled by the test's own
// dbEvent() path (rl.Printf → dbEvent inserts into run_events directly).
func skipLogLine(line string) bool {
	// Skip go test framework output noise
	switch {
	case strings.HasPrefix(line, "=== RUN "):
		return true
	case strings.HasPrefix(line, "--- PASS"):
		return true
	case strings.HasPrefix(line, "--- FAIL"):
		return true
	case strings.HasPrefix(line, "--- SKIP"):
		return true
	case strings.HasPrefix(line, "PASS"):
		return true
	case strings.HasPrefix(line, "FAIL"):
		return true
	case strings.HasPrefix(line, "ok  "):
		return true
	case strings.HasPrefix(line, "?   "):
		return true
	case strings.HasPrefix(line, "--- SKIP"):
		return true
	case strings.HasPrefix(line, "testing: warning"):
		return true
	}
	return false
}

// lineEventKind detects the event kind from a go test stdout line.
// Returns "" for lines that should be skipped entirely (handled by skipLogLine).
func lineEventKind(line string) string {
	return "log"
}
