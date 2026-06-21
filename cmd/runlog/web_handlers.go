package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emergent-company/go-daisy/render"
	runlog "github.com/emergent-company/runlog"
	"github.com/labstack/echo/v4"
)

const pageLimit = 20

func (app *WebApp) handleDashboard(c echo.Context) error {
	allRuns, err := app.db.ListRuns(time.Now().Add(-30*24*time.Hour), 10)
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}

	passCount, failCount, skipCount, timeoutCount := 0, 0, 0, 0
	for _, r := range allRuns {
		if r.FinishedAt == nil {
			continue
		}
		if r.Skipped {
			skipCount++
		} else if r.Passed != nil && *r.Passed {
			passCount++
		} else if r.Reason != nil && *r.Reason == "timed out" {
			timeoutCount++
		} else {
			failCount++
		}
	}
	// Count timed-out runs as failures in the dashboard summary
	failCount += timeoutCount

	names, _ := app.db.DiscoverTests()
	allTestNames := make(map[string]bool)
	for _, n := range names {
		allTestNames[n] = true
	}
	// Include discovered test functions
	if discovered := DiscoverTestFunctions(app.workDir); discovered != nil {
		for _, funcs := range discovered {
			for _, f := range funcs {
				allTestNames[f] = true
			}
		}
	}
	catMap := make(map[string]int)
	for n := range allTestNames {
		cat := app.config.CategoryForTest(n)
		catMap[cat]++
	}
	var categories []catSummary
	for name, count := range catMap {
		categories = append(categories, catSummary{Name: name, TestCount: count})
	}

	data := dashboardData{
		TotalTests: len(allTestNames),
		TotalRuns:  len(allRuns),
		PassCount:  passCount,
		FailCount:  failCount,
		SkipCount:  skipCount,
		RecentRuns: allRuns,
		Categories: categories,
	}
	render.RenderAuto(c.Response().Writer, c.Request(),
		DashboardPage(data), DashboardContent(data))
	return nil
}

func (app *WebApp) handleAllRuns(c echo.Context) error {
	offsetStr := c.QueryParam("offset")
	offset := 0
	if offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	since := parseSinceParam(c.QueryParam("since"))
	allRuns, err := app.db.ListRuns(since, offset+runsPerPage+1)
	if err != nil {
		return fmt.Errorf("list runs: %w", err)
	}

	total := len(allRuns)
	page := allRuns
	if offset > 0 && offset < len(allRuns) {
		page = allRuns[offset:]
	}
	if len(page) > runsPerPage {
		page = page[:runsPerPage]
	}

	runRows := make([]runlog.RunRow, len(page))
	catMap := make(map[int64]string, len(page))
	for i, r := range page {
		runRows[i] = r
		catMap[r.ID] = app.config.CategoryForRun(&r)
	}

	categories := sortedKeys(app.config.Categories)
	f := runFilters{
		Category: c.QueryParam("category"),
		Status:   c.QueryParam("status"),
		Since:    c.QueryParam("since"),
		Search:   c.QueryParam("search"),
		Tags:     c.QueryParam("tags"),
		HasCost:  c.QueryParam("has_cost") == "1",
		Offset:   offset,
	}
	// If filter params are set, this is a table refresh — return only the table content.
	if render.IsPartial(c.Request()) && (offset > 0 || c.QueryParam("category") != "" || c.QueryParam("status") != "" || c.QueryParam("since") != "" || c.QueryParam("search") != "" || c.QueryParam("tags") != "" || c.QueryParam("has_cost") != "") {
		render.RenderPartial(c.Response().Writer, c.Request(), runsTableContent(runRows, catMap, total, f))
	} else {
		render.RenderAuto(c.Response().Writer, c.Request(),
			AllRunsPage(runRows, catMap, total, f, categories), AllRunsContent(runRows, catMap, total, f, categories))
	}
	return nil
}

func (app *WebApp) handleTests(c echo.Context) error {
	categoryFilter := c.QueryParam("category")
	statusFilter := c.QueryParam("status")
	rawDB := app.db.RawDB()

	// Single window-function query: run count, latest start, and pass/skip status per test.
	// Uses idx_test_runs_name_started composite index for GROUP BY.
	rows, err := rawDB.Query(`
		SELECT t.test_name, t.run_count, t.last_started,
		       t.passed, t.skipped, t.reason, t.category
		FROM (
			SELECT test_name,
			       COUNT(*)       OVER (PARTITION BY test_name) AS run_count,
			       MAX(started_at) OVER (PARTITION BY test_name) AS last_started,
			       passed, skipped, reason, category,
			       ROW_NUMBER()   OVER (PARTITION BY test_name ORDER BY started_at DESC) AS rn
			FROM test_runs
		) t
		WHERE t.rn = 1
		ORDER BY t.test_name
	`)
	if err != nil {
		return fmt.Errorf("batch query: %w", err)
	}
	defer rows.Close()

	type aggRow struct {
		Name       string
		RunCount   int
		LastStarted string
		Category   *string
	}
	var agg []aggRow
	statusMap := make(map[string]string)
	for rows.Next() {
		var name string
		var runCount int
		var lastStarted string
		var passed, skipped sql.NullInt64
		var reason sql.NullString
		var category sql.NullString
		if err := rows.Scan(&name, &runCount, &lastStarted, &passed, &skipped, &reason, &category); err != nil {
			continue
		}
		var catPtr *string
		if category.Valid {
			catPtr = &category.String
		}
		agg = append(agg, aggRow{Name: name, RunCount: runCount, LastStarted: lastStarted, Category: catPtr})
		switch {
		case !passed.Valid:
			statusMap[name] = "running"
		case skipped.Valid && skipped.Int64 == 1:
			statusMap[name] = "skip"
		case passed.Int64 == 1:
			statusMap[name] = "pass"
		case reason.Valid && reason.String == "timed out":
			statusMap[name] = "timeout"
		default:
			statusMap[name] = "fail"
		}
	}
	_ = rows.Close()

	seen := make(map[string]bool)

	// Build entries from DB runs
	catMap := make(map[string][]testListEntry)
	for _, a := range agg {
		// Prefer DB-stored category, fall back to config match.
		cat := app.config.CategoryForTest(a.Name)
		if a.Category != nil && *a.Category != "" {
			cat = *a.Category
		}
		if categoryFilter != "" && cat != categoryFilter {
			continue
		}
		status := statusMap[a.Name]
		if status == "" {
			status = "none"
		}
		if statusFilter != "" && status != statusFilter {
			continue
		}
		lastRunAt := "-"
		if a.LastStarted != "" {
			if t, err := time.Parse(time.RFC3339Nano, a.LastStarted); err == nil {
				lastRunAt = t.Format("Jan 02 15:04")
			}
		}
		entry := testListEntry{
			Name:       a.Name,
			LastStatus: status,
			LastRunAt:  lastRunAt,
			RunCount:   a.RunCount,
		}
		catMap[cat] = append(catMap[cat], entry)
		seen[a.Name] = true
	}

	// Merge discovered test functions from filesystem
	if discovered := DiscoverTestFunctions(app.workDir); discovered != nil {
		for _, funcs := range discovered {
			for _, f := range funcs {
				if seen[f] {
					continue
				}
				if statusFilter != "" && statusFilter != "never_run" {
					continue
				}
				cat := app.config.CategoryForTest(f)
				if cat == "Uncategorized" {
					for dirCat, dirFuncs := range discovered {
						for _, df := range dirFuncs {
							if df == f {
								cat = dirCat
								break
							}
						}
						if cat != "Uncategorized" {
							break
						}
					}
				}
				if categoryFilter != "" && cat != categoryFilter {
					continue
				}
				entry := testListEntry{
					Name:       f,
					LastStatus: "never_run",
					NeverRun:   true,
				}
				catMap[cat] = append(catMap[cat], entry)
				seen[f] = true
			}
		}
	}

	var filteredCats []testListCategory
	for _, name := range sortedKeys(catMap) {
		filteredCats = append(filteredCats, testListCategory{Name: name, Tests: catMap[name]})
	}
	if filteredCats == nil {
		filteredCats = []testListCategory{}
	}

	// Build ALL categories for the dropdown from all sources
	allCats := make([]testListCategory, 0)
	seenCats := make(map[string]bool)
	// First: categories that have entries from DB or discovery
	for _, name := range sortedKeys(catMap) {
		allCats = append(allCats, testListCategory{Name: name, Tests: catMap[name]})
		seenCats[name] = true
	}
	// Then: config-only categories (empty but listed in dropdown)
	for _, name := range sortedKeys(app.config.Categories) {
		if !seenCats[name] {
			allCats = append(allCats, testListCategory{Name: name, Tests: catMap[name]})
		}
	}
	if len(allCats) == 0 {
		allCats = filteredCats
	}

	data := testListData{Categories: allCats, ActiveFilter: categoryFilter, StatusFilter: statusFilter}

	render.RenderAuto(c.Response().Writer, c.Request(),
		TestsPage(data), TestsContent(data))
	return nil
}

func (app *WebApp) handleTestDetail(c echo.Context) error {
	testName := c.Param("name")
	offsetStr := c.QueryParam("offset")
	tagFilter := c.QueryParam("tag")
	offset := 0
	if offsetStr != "" {
		offset, _ = strconv.Atoi(offsetStr)
	}

	runs, err := queryRunsForTest(app.db.RawDB(), testName, pageLimit+1, offset, tagFilter)
	if err != nil {
		return fmt.Errorf("query runs: %w", err)
	}
	hasMore := len(runs) > pageLimit
	if hasMore {
		runs = runs[:pageLimit]
	}

	rawDB := app.db.RawDB()
	totalRuns := len(runs) + offset
	{
		q := `SELECT COUNT(*) FROM test_runs WHERE test_name = ?`
		args := []any{testName}
		if tagFilter != "" {
			q += ` AND tags LIKE '%"` + tagFilter + `"%'`
		}
		_ = rawDB.QueryRow(q, args...).Scan(&totalRuns)
	}

	var stats *testStats
	if offset == 0 {
		s := computeTestStats(rawDB, testName)
		stats = &s
	}

	hasCost := false
	for _, r := range runs {
		if r.CostUSD != nil && *r.CostUSD > 0 {
			hasCost = true
			break
		}
	}
	if offset > 0 {
		_ = rawDB.QueryRow(`SELECT COUNT(*) FROM test_runs WHERE test_name=? AND cost_usd IS NOT NULL AND cost_usd>0`, testName).Scan(&hasCost)
	}

	data := testDetailData{
		TestName:    testName,
		Runs:        runs,
		TotalRuns:   totalRuns,
		Offset:      offset,
		Limit:       pageLimit,
		Stats:       stats,
		TagFilter:   tagFilter,
		HasCostData: hasCost,
	}
	render.RenderAuto(c.Response().Writer, c.Request(),
		TestDetailPage(data), TestDetailContent(data))
	return nil
}

func (app *WebApp) handleRunDetail(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run id")
	}

	run := fetchRunByID(app.db.RawDB(), id)
	if run == nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	events, err := app.db.ListEvents(id)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	if events == nil {
		events = []runlog.EventRow{}
	}

	timeline := make([]runlog.EventRow, 0, len(events))
	meta := make([]runlog.EventRow, 0, len(events))
	for _, e := range events {
		if metaRunEventKinds[e.Kind] {
			meta = append(meta, e)
		} else {
			timeline = append(timeline, e)
		}
	}

	showDebug := c.QueryParam("debug") == "1"

	isActive := run.FinishedAt == nil
	sseURL := ""
	if isActive {
		sseURL = fmt.Sprintf("/ui/runs/%d/status", id)
	}

	data := runDetailData{
		Run:            *run,
		TimelineEvents: timeline,
		MetaEvents:     meta,
		ShowDebug:      showDebug,
		IsActive:       isActive,
		SSEURL:         sseURL,
	}
	render.RenderAuto(c.Response().Writer, c.Request(),
		RunDetailPage(data), RunDetailContent(data))
	return nil
}

func (app *WebApp) handleRunEventsTable(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run id")
	}

	run := fetchRunByID(app.db.RawDB(), id)
	if run == nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	events, err := app.db.ListEvents(id)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}
	if events == nil {
		events = []runlog.EventRow{}
	}

	timeline := make([]runlog.EventRow, 0, len(events))
	meta := make([]runlog.EventRow, 0, len(events))
	for _, e := range events {
		if metaRunEventKinds[e.Kind] {
			meta = append(meta, e)
		} else {
			timeline = append(timeline, e)
		}
	}

	showDebug := c.QueryParam("debug") == "1"

	data := runDetailData{
		Run:            *run,
		TimelineEvents: timeline,
		MetaEvents:     meta,
		ShowDebug:      showDebug,
	}
	render.RenderPartial(c.Response().Writer, c.Request(), eventsSection(data))
	return nil
}

func (app *WebApp) handleEventChildren(c echo.Context) error {
	idStr := c.Param("id")
	eventIDStr := c.Param("eventID")

	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run id")
	}
	eventID, err := strconv.ParseInt(eventIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid event id")
	}

	events, err := app.db.ListEvents(runID)
	if err != nil {
		return fmt.Errorf("list events: %w", err)
	}

	for _, e := range events {
		if e.ID == eventID {
			data := eventChildrenData{EventID: eventID, Children: e.Children, Kind: e.Kind}
			if e.Details != nil {
				data.Details = *e.Details
			}
			render.RenderPartial(c.Response().Writer, c.Request(),
				EventChildrenPartial(data))
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "event not found")
}

func (app *WebApp) handleExperiments(c echo.Context) error {
	experiments, err := app.db.ListExperiments()
	if err != nil {
		return fmt.Errorf("list experiments: %w", err)
	}
	if experiments == nil {
		experiments = []runlog.ExperimentSummary{}
	}
	render.RenderAuto(c.Response().Writer, c.Request(),
		ExperimentsPage(experiments), ExperimentsContent(experiments))
	return nil
}

func (app *WebApp) handleExperimentDetail(c echo.Context) error {
	expName := c.Param("name")
	allExperiments, err := app.db.ListExperiments()
	if err != nil {
		return fmt.Errorf("list experiments: %w", err)
	}
	for _, exp := range allExperiments {
		if exp.Name == expName {
			render.RenderAuto(c.Response().Writer, c.Request(),
				ExperimentDetailPage(exp), ExperimentDetailContent(exp))
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "experiment not found")
}

func (app *WebApp) handleLaunchFromList(c echo.Context) error {
	return app.handleLaunchTest(c)
}

func (app *WebApp) handleEventsReference(c echo.Context) error {
	render.RenderAuto(c.Response().Writer, c.Request(),
		EventsReferencePage(), EventsReferenceContent())
	return nil
}

func (app *WebApp) handleSSEStream(c echo.Context) error {
	topic := c.QueryParam("topic")
	if topic == "" {
		topic = "footer"
	}

	w := c.Response().Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ch := app.sse.Subscribe(topic)
	defer app.sse.Unsubscribe(topic, ch)

	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return nil
	}
	flusher.Flush()

	ctx := c.Request().Context()
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			_, err := io.WriteString(w, fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Event, evt.Data))
			if err != nil {
				return nil
			}
			flusher.Flush()

		case <-ctx.Done():
			return nil
		}
	}
}

func (app *WebApp) handleFooterStatus(c echo.Context) error {
	rawDB := app.db.RawDB()
	var totalRuns, totalTests int
	_ = rawDB.QueryRow(`SELECT COUNT(*) FROM test_runs`).Scan(&totalRuns)
	_ = rawDB.QueryRow(`SELECT COUNT(DISTINCT test_name) FROM test_runs`).Scan(&totalTests)

	statusDot := "status-success"
	statusText := "Running"
	uptime := time.Since(app.startedAt)
	uptimeStr := fmt.Sprintf("%.0fm", uptime.Minutes())
	if uptime.Hours() >= 1 {
		uptimeStr = fmt.Sprintf("%.0fh", uptime.Hours())
	}

	html := fmt.Sprintf(
		`<div class="status %s status-xs"></div><span class="text-base-content/50">%s — %d runs, %d tests</span><span class="text-base-content/30 ml-2">up %s</span>`,
		statusDot, statusText, totalRuns, totalTests, uptimeStr,
	)
	return c.HTML(http.StatusOK, html)
}

func (app *WebApp) handleLaunchTest(c echo.Context) error {
	testName := c.Param("name")

	// Pre-create test_runs row so we have an ID to redirect to.
	runID, err := app.db.InsertRun(testName, time.Now(), "web-ui", "", nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create run: "+err.Error())
	}

	env := map[string]string{
		"RUNLOG_RUN_ID": fmt.Sprintf("%d", runID),
		"TEST_RUNS_DB":  app.db.Path(),
	}
	if _, err := app.lm.Launch(testName, runID, env); err != nil {
		// Launch failed — clean up the pre-created run row.
		_, _ = app.db.RawDB().Exec(`DELETE FROM test_runs WHERE id = ?`, runID)
		w := c.Response().Writer
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`<div class="alert alert-error mt-4 mb-6"><span>%s</span></div>`, err.Error())
		_, _ = io.WriteString(w, html)
		return nil
	}

	// Redirect to the run detail page.
	target := fmt.Sprintf("/ui/runs/%d", runID)
	if render.IsHTMX(c.Request()) {
		w := c.Response().Writer
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)
		return nil
	}
	http.Redirect(c.Response().Writer, c.Request(), target, http.StatusSeeOther)
	return nil
}

func (app *WebApp) handleLaunchEvents(c echo.Context) error {
	testName := c.Param("name")
	al := app.lm.Get(testName)
	if al == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no active launch for "+testName)
	}

	w := c.Response().Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ch := al.Subscribe()
	defer al.Unsubscribe(ch)

	ctx := c.Request().Context()
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return nil
			}
			_, _ = io.WriteString(w, "data: ")
			_, _ = io.WriteString(w, line)
			_, _ = io.WriteString(w, "\n\n")
			flusher.Flush()

		case <-al.done:
			_, _ = io.WriteString(w, fmt.Sprintf("event: done\ndata: {\"exit_code\":%d}\n\n", al.exitCode))
			flusher.Flush()
			return nil

		case <-ctx.Done():
			return nil
		}
	}
}

func (app *WebApp) handleRunStatusSSE(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid run id")
	}

	run := fetchRunByID(app.db.RawDB(), id)
	if run == nil {
		return echo.NewHTTPError(http.StatusNotFound, "run not found")
	}

	w := c.Response().Writer
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	var doneCh <-chan struct{}
	var al *ActiveLaunch
	al = app.lm.Get(run.TestName)
	if al != nil && al.RunID == id {
		doneCh = al.done
	}

	ctx := c.Request().Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			html, err := app.buildEventsTableHTML(ctx, id)
			if err == nil {
				data, _ := json.Marshal(map[string]string{"html": html})
				_, _ = io.WriteString(w, fmt.Sprintf("event: events-table\ndata: %s\n\n", data))
				flusher.Flush()
			}
			if doneCh == nil {
				r := fetchRunByID(app.db.RawDB(), id)
				if r == nil || r.FinishedAt != nil {
					_, _ = io.WriteString(w, "event: done\ndata: {}\n\n")
					flusher.Flush()
					return nil
				}
			}
		case <-doneCh:
			html, err := app.buildEventsTableHTML(ctx, id)
			if err == nil {
				data, _ := json.Marshal(map[string]string{"html": html})
				_, _ = io.WriteString(w, fmt.Sprintf("event: events-table\ndata: %s\n\n", data))
				flusher.Flush()
			}
			_, _ = io.WriteString(w, fmt.Sprintf("event: done\ndata: {\"exit_code\":%d}\n\n", al.exitCode))
			flusher.Flush()
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func (app *WebApp) buildEventsTableHTML(ctx context.Context, runID int64) (string, error) {
	run := fetchRunByID(app.db.RawDB(), runID)
	if run == nil {
		return "", fmt.Errorf("run not found")
	}
	events, err := app.db.ListEvents(runID)
	if err != nil {
		return "", err
	}
	if events == nil {
		events = []runlog.EventRow{}
	}
	timeline := make([]runlog.EventRow, 0, len(events))
	meta := make([]runlog.EventRow, 0, len(events))
	for _, e := range events {
		if metaRunEventKinds[e.Kind] {
			meta = append(meta, e)
		} else {
			timeline = append(timeline, e)
		}
	}
	data := runDetailData{
		Run:            *run,
		TimelineEvents: timeline,
		MetaEvents:     meta,
		IsActive:       run.FinishedAt == nil,
	}
	var buf strings.Builder
	if err := eventsSection(data).Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func queryRunsForTest(rawDB *sql.DB, testName string, limit, offset int, tagFilter string) ([]runlog.RunRow, error) {
	q := `
		SELECT id, test_name, started_at, finished_at, passed, skipped,
		       description, tags, experiment, runner, reason, env_name,
		       input_tokens, output_tokens, cost_usd, env_vars,
		       app_version, test_version, category
		FROM test_runs
		WHERE test_name = ?`
	args := []any{testName}
	if tagFilter != "" {
		q += ` AND tags LIKE '%"` + tagFilter + `"%'`
	}
	q += ` ORDER BY started_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := rawDB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunRows(rows)
}

func fetchRunByID(rawDB *sql.DB, id int64) *runlog.RunRow {
	rows, err := rawDB.Query(`
		SELECT id, test_name, started_at, finished_at, passed, skipped,
		       description, tags, experiment, runner, reason, env_name,
		       input_tokens, output_tokens, cost_usd, env_vars,
		       app_version, test_version, category
		FROM test_runs WHERE id = ?`, id)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result, err := scanRunRows(rows)
	if err != nil || len(result) == 0 {
		return nil
	}
	return &result[0]
}

func scanRunRows(rows *sql.Rows) ([]runlog.RunRow, error) {
	var result []runlog.RunRow
	for rows.Next() {
		var r runlog.RunRow
		var startedStr string
		var finishedStr, descJSON, tagsJSON *string
		var experiment, runner, reason, envName *string
		var inputTokens, outputTokens *int64
		var costUSD *float64
		var envVarsJSON *string
		var appVersion, testVersion *string
		var passedInt *int
		var skipped bool

		if err := rows.Scan(
			&r.ID, &r.TestName, &startedStr, &finishedStr,
			&passedInt, &skipped,
			&descJSON, &tagsJSON, &experiment,
			&runner, &reason, &envName,
			&inputTokens, &outputTokens, &costUSD, &envVarsJSON,
			&appVersion, &testVersion, &r.Category,
		); err != nil {
			return nil, fmt.Errorf("scan run row: %w", err)
		}

		r.StartedAt, _ = time.Parse(time.RFC3339Nano, startedStr)
		if finishedStr != nil {
			t, _ := time.Parse(time.RFC3339Nano, *finishedStr)
			r.FinishedAt = &t
		}
		r.Skipped = skipped
		if passedInt != nil && !skipped {
			p := *passedInt == 1
			r.Passed = &p
		}
		if descJSON != nil && *descJSON != "" {
			var d runlog.RunDescription
			if json.Unmarshal([]byte(*descJSON), &d) == nil {
				r.Description = &d
			}
		}
		if tagsJSON != nil && *tagsJSON != "" {
			var tags []string
			if json.Unmarshal([]byte(*tagsJSON), &tags) == nil {
				r.Tags = tags
			}
		}
		r.Experiment = experiment
		r.AppVersion = appVersion
		r.TestVersion = testVersion
		r.Runner = runner
		r.Reason = reason
		r.EnvName = envName
		r.InputTokens = inputTokens
		r.OutputTokens = outputTokens
		r.CostUSD = costUSD
		if envVarsJSON != nil && *envVarsJSON != "" {
			var envVars map[string]string
			if json.Unmarshal([]byte(*envVarsJSON), &envVars) == nil {
				r.EnvVars = envVars
			}
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func computeTestStats(rawDB *sql.DB, testName string) testStats {
	var s testStats

	// Aggregate: avg, min, max duration, pass rate
	var avgS, minS, maxS sql.NullFloat64
	var total, passed int
	_ = rawDB.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG((julianday(finished_at)-julianday(started_at))*86400),0),
		       COALESCE(MIN((julianday(finished_at)-julianday(started_at))*86400),0),
		       COALESCE(MAX((julianday(finished_at)-julianday(started_at))*86400),0),
		       COALESCE(SUM(CASE WHEN passed=1 THEN 1 ELSE 0 END),0)
		FROM test_runs WHERE test_name=? AND finished_at IS NOT NULL`, testName).Scan(&total, &avgS, &minS, &maxS, &passed)
	s.TotalRuns = total

	if avgS.Valid {
		s.AvgDuration = fmtDuration(avgS.Float64)
		s.MinDuration = fmtDuration(minS.Float64)
		s.MaxDuration = fmtDuration(maxS.Float64)
	}
	if total > 0 {
		s.PassRate = fmt.Sprintf("%.0f%%", float64(passed)/float64(total)*100)
	}

	// Last 20 runs for trend
	trendRows, err := rawDB.Query(`
		SELECT (julianday(finished_at)-julianday(started_at))*86400, cost_usd
		FROM test_runs WHERE test_name=? AND finished_at IS NOT NULL
		ORDER BY started_at DESC LIMIT 20`, testName)
	if err == nil {
		defer trendRows.Close()
		for trendRows.Next() {
			var dur, cost sql.NullFloat64
			if err := trendRows.Scan(&dur, &cost); err != nil {
				continue
			}
			pt := trendPoint{Index: len(s.TrendData)}
			if dur.Valid {
				pt.DurationMS = dur.Float64 * 1000
			}
			if cost.Valid && cost.Float64 > 0 {
				pt.CostUSD = cost.Float64
				s.HasCostData = true
			}
			s.TrendData = append(s.TrendData, pt)
		}
	}
	// Reverse to chronological order
	for i, j := 0, len(s.TrendData)-1; i < j; i, j = i+1, j-1 {
		s.TrendData[i], s.TrendData[j] = s.TrendData[j], s.TrendData[i]
	}
	for i := range s.TrendData {
		s.TrendData[i].Index = i
	}

	// Trend direction: compare avg of first half vs second half
	if n := len(s.TrendData); n >= 4 {
		half := n / 2
		var firstHalf, secondHalf float64
		for i := 0; i < half; i++ {
			firstHalf += s.TrendData[i].DurationMS
		}
		for i := half; i < n; i++ {
			secondHalf += s.TrendData[i].DurationMS
		}
		firstAvg := firstHalf / float64(half)
		secondAvg := secondHalf / float64(n-half)
		diff := secondAvg - firstAvg
		if diff > firstAvg*0.1 {
			s.TrendUp = true
		} else if diff < -firstAvg*0.1 {
			s.TrendUp = false
		} else {
			s.TrendFlat = true
		}
	} else {
		s.TrendFlat = true
	}

	// Avg cost
	if s.HasCostData {
		var avgCost sql.NullFloat64
		_ = rawDB.QueryRow(`SELECT AVG(cost_usd) FROM test_runs WHERE test_name=? AND cost_usd IS NOT NULL AND cost_usd>0`, testName).Scan(&avgCost)
		if avgCost.Valid {
			s.AvgCost = fmt.Sprintf("$%.4f", avgCost.Float64)
		}
	}

	return s
}

func fmtDuration(s float64) string {
	if s < 0.001 {
		return "< 1ms"
	}
	if s < 1.0 {
		return fmt.Sprintf("%.0fms", s*1000)
	}
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	return fmt.Sprintf("%.0fm %0.0fs", s/60, float64(int(s)%60))
}

func parseSinceParam(since string) time.Time {
	switch since {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}
