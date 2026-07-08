package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// linterURLSegment encodes a linter name for safe use in a URL path segment.
// Linter names contain '/' (e.g. 'core/gofmt') which the Go HTTP router treats
// as path separators even when percent-encoded. Replacing '/' with the
// reversible placeholder '--' (never present in real linter configuration
// names) resolves this without changing route patterns.
func linterURLSegment(name string) string { return strings.Replace(name, "/", "--", -1) }

// ── Dashboard ─────────────────────────────────────────────────────────────────

func HomeURL() string { return "/ui/" }

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestsURL() string                   { return "/ui/tests" }
func TestsCategoryURL(cat string) string { return "/ui/tests?category=" + url.QueryEscape(cat) }
func TestDetailURL(name string) string   { return "/ui/tests/" + url.PathEscape(name) }
func TestDetailPaginatedURL(name string, offset int, tag string) string {
	return fmt.Sprintf("/ui/tests/%s?offset=%d&tag=%s", url.PathEscape(name), offset, url.QueryEscape(tag))
}

// ── Runs ──────────────────────────────────────────────────────────────────────

func AllRunsURL() string { return "/ui/runs" }
func AllRunsPaginatedURL(f runFilters) string {
	return fmt.Sprintf("/ui/runs?offset=%d&category=%s&status=%s&since=%s&search=%s&tags=%s&has_cost=%s&test_type=%s",
		f.Offset,
		url.QueryEscape(f.Category),
		url.QueryEscape(f.Status),
		url.QueryEscape(f.Since),
		url.QueryEscape(f.Search),
		url.QueryEscape(f.Tags),
		boolStr(f.HasCost),
		url.QueryEscape(f.TestType),
	)
}
func RunDetailURL(id int64) string { return fmt.Sprintf("/ui/runs/%d", id) }
func RunEventURL(runID, eventID int64) string {
	return fmt.Sprintf("/ui/runs/%d/events/%d", runID, eventID)
}
func RunEventsTableURL(id int64) string { return fmt.Sprintf("/ui/runs/%d/events-table", id) }
func RunStatusSSEURL(id int64) string   { return fmt.Sprintf("/ui/runs/%d/status", id) }

// ── SSE ───────────────────────────────────────────────────────────────────────

func SSEStreamURL(topic string) string {
	return fmt.Sprintf("/ui/stream?topic=%s", url.QueryEscape(topic))
}

// ── Launch ────────────────────────────────────────────────────────────────────

func LaunchURL(name string) string       { return "/ui/launch/" + url.PathEscape(name) }
func LaunchEventsURL(name string) string { return "/ui/launch/" + url.PathEscape(name) + "/events" }

// ── Linters ───────────────────────────────────────────────────────────────────

func LintersURL() string                 { return "/ui/linters" }
func LintersCacheBustURL() string        { return fmt.Sprintf("/ui/linters?_=%d", time.Now().UnixMilli()) }
func LinterEventsSSEURL() string         { return "/ui/linters/events" }
func LinterDetailURL(name string) string { return "/ui/linters/" + linterURLSegment(name) }
func LinterDetailPaginatedURL(name string, offset int) string {
	return fmt.Sprintf("/ui/linters/%s?offset=%d", linterURLSegment(name), offset)
}
func RunLinterURL(name string) string { return "/ui/linters/" + linterURLSegment(name) + "/run" }
func RunAllLintersURL() string        { return "/ui/linters/run-all" }
func LinterRunDetailURL(name string, id int64) string {
	return fmt.Sprintf("/ui/linters/%s/runs/%d", linterURLSegment(name), id)
}
func LinterEventsSSEURLFor(name string) string {
	return "/ui/linters/" + linterURLSegment(name) + "/events"
}

// ── Environments ──────────────────────────────────────────────────────────────

func EnvironmentsURL() string                 { return "/ui/environments" }
func EnvironmentDetailURL(name string) string { return "/ui/environments/" + url.PathEscape(name) }

// ── Experiments ───────────────────────────────────────────────────────────────

func ExperimentsURL() string                 { return "/ui/experiments" }
func ExperimentDetailURL(name string) string { return "/ui/experiments/" + url.PathEscape(name) }

// ── Reference ─────────────────────────────────────────────────────────────────

func SDKReferenceURL() string { return "/ui/events" }

// ── Search ────────────────────────────────────────────────────────────────────

func SearchURL() string { return "/ui/search" }

// ── Footer ────────────────────────────────────────────────────────────────────

func FooterStatusURL() string { return "/ui/footer-status" }

// ── Helpers ───────────────────────────────────────────────────────────────────

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
