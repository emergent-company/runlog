package main

import (
	"bufio"
	"context"
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
		},
	},
	{
		Label: "Reference",
		Items: []layout.SidebarItem{
			{Label: "Events", Href: "/ui/events", Icon: "lucide--file-text"},
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
	AvgDuration  string
	MinDuration  string
	MaxDuration  string
	PassRate     string
	TotalRuns    int
	TrendData    []trendPoint
	TrendUp      bool
	TrendFlat    bool
	HasCostData  bool
	AvgCost      string
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
	TimeFull   TimeFormat = iota // 10/06/2026 11:27:59
	TimeShort                     // 11:27:59
	TimeHuman                     // 2 min ago
	TimeISO                       // 2026-06-10T11:27:59Z
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
			// Skip t.Log() lines (file.go:line: prefix) — these are already
			// persisted by the test's rl.Printf() → dbEvent() path. Keeping
			// them would create duplicate events for every Printf call.
			if skipLogLine(line) {
				continue
			}
			seq++
			if al.RunID != 0 {
				_ = lm.db.InsertEvent(al.RunID, seq, time.Now(), time.Since(al.Started).Seconds(), "log", line, nil)
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

	app := &WebApp{
		echo:      e,
		db:        db,
		config:    config,
		lm:        NewLauncherManager(db, config),
		sse:       newSSEBroker(db),
		startedAt: time.Now(),
		workDir:   workDir,
	}

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

	return app
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
			var stale []struct{ id int64; testName string }
			for rows.Next() {
				var s struct{ id int64; testName string }
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
func reqCtx(c echo.Context) context.Context {
	return c.Request().Context()
}

// skipLogLine returns true for lines that should NOT be persisted as run_events.
// These are lines from go test -v that are already handled by the test's own
// dbEvent() path (rl.Printf → dbEvent inserts into run_events directly).
func skipLogLine(line string) bool {
	// Skip t.Log() output lines: "    file.go:line: message"
	// These are prefixed with 4+ spaces followed by a .go file reference.
	// They are duplicates of rl.Printf → dbEvent entries.
	if len(line) > 4 {
		trimmed := line
		i := 0
		for i < len(trimmed) && trimmed[i] == ' ' {
			i++
		}
		if i >= 4 && i < len(trimmed)-5 {
			rest := trimmed[i:]
			// Match "file.go:number: " pattern
			if dotIdx := strings.Index(rest, ".go:"); dotIdx > 0 && dotIdx < 40 {
				afterDot := rest[dotIdx+4:]
				colonIdx := strings.Index(afterDot, ":")
				if colonIdx > 0 && colonIdx < 10 {
					// Has .go:digits: prefix — it's a t.Log() line, skip it
					return true
				}
			}
		}
	}
	return false
}
