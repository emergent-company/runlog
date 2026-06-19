# Runlog Web UI — Specification

**Date:** 2026-06-10
**Status:** Draft

## Why

TUI requires terminal. Web UI makes runlog accessible from browser — browse test history, launch tests, inspect failures without terminal. Share run links across team.

## Stack

| Layer | Choice |
|-------|--------|
| HTTP framework | Echo v4 (mounted under `/ui/` on existing daemon ServeMux) |
| Templating | Templ (`.templ` files → type-safe Go) |
| UI components | `github.com/emergent-company/go-daisy` (DaisyUI + Tailwind CSS) |
| Interactivity | HTMX v4 (no custom JS) |
| Live output | Server-Sent Events (SSE) via HTMX native `hx-trigger="sse:..."` |
| Static assets | go-daisy `staticfs` or local `embed.FS` |

## Architecture

```
daemon port 7430
├── GET /health              (existing ServeMux)
├── POST /runs               (existing ServeMux)
├── GET /runs/{id}           (existing ServeMux)
├── ...                      (existing API routes)
└── /ui/*                    (Echo v4, mounted via http.StripPrefix)
    ├── GET  /               → dashboard
    ├── GET  /tests          → test list
    ├── GET  /tests/{name}   → run history
    ├── GET  /runs/{id}      → event timeline
    ├── GET  /runs/{id}/events/{eid}  → event children partial
    ├── POST /launch/{name}  → spawn test, return SSE log viewer
    └── GET  /launch/{name}/events   → SSE stream
```

Echo instance created in daemon init, reuses same `*RunDB`. Mounted as:
```go
mux.Handle("/ui/", http.StripPrefix("/ui", webApp))
```

## Pages

### Dashboard (`GET /ui/`)
- Stat cards: total tests, total runs, pass/fail/skip counts
- Recent runs table (last 10): test name, status badge, duration, timestamp
- Categories summary

### Tests (`GET /ui/tests`)
- Category filter `<select>` with HTMX `hx-trigger="change" hx-target="#test-list"`
- Test list grouped by category: test name, latest status badge (`ui.Badge`), last-run time
- Click test → drill into run history via `hx-get="/ui/tests/{name}" hx-target="#content"`

### Test Detail (`GET /ui/tests/{name}`)
- Run history `table.Table`: status badge, duration, started-at, token usage
- "Run Test" button: `hx-post="/ui/launch/{name}" hx-target="#launch-area"`
- Pagination: `hx-get="?offset=20" hx-target="#run-table" hx-swap="outerHTML"`

### Run Detail (`GET /ui/runs/{id}`)
- Metadata header: test name, status, timestamps, tokens/cost
- Event timeline: chronological list with kind badges, elapsed time
- Click event → `hx-get="/ui/runs/{id}/events/{eid}" hx-target="#event-{eid}"` → expand children

### Live Launch (`POST /ui/launch/{name}` + SSE)
1. POST spawns `go test -v -run {name} ./...` in background goroutine
2. Returns `<pre hx-trigger="sse:/ui/launch/{name}/events" hx-swap="beforeend">`
3. SSE sends `data: <line>\n\n` for each stdout/stderr line
4. Completion: `event: done\ndata: {"exit_code": N}\n\n`

## File Structure

```
cmd/runlog/
├── daemon.go              # MODIFIED: init WebApp, mount /ui/
├── web.go                 # NEW: WebApp struct, route setup, LauncherManager
├── web_handlers.go        # NEW: handler functions
└── templates/             # NEW: Templ files
    ├── layout.templ       # layout.Sidebar shell
    ├── dashboard.templ    # stat cards + recent runs
    ├── tests.templ        # category filter + test list
    ├── test_detail.templ  # run history table
    ├── run_detail.templ   # event timeline
    ├── run_events.templ   # children partial
    └── launch.templ       # SSE log viewer
```

## go-daisy Components Used

| Component | Usage |
|-----------|-------|
| `layout.Sidebar` | Page shell with nav |
| `ui.Badge` | Pass (green), fail (red), skip (yellow) |
| `ui.Card` | Dashboard stat cards |
| `ui.Button` | Launch test, nav back |
| `table.Table` | Tests list, runs table, events |
| `nav.TabMenu` | Dashboard/Tests nav |
| `components/logs` | Live output log stream |
| `render.RenderAuto` | All handlers (auto page vs partial) |
| `render.AppendToast` | Launch feedback toasts |

## HTMX Interaction Map

```
Sidebar:
  Dashboard → hx-get="/ui/"           hx-target="#content"
  Tests     → hx-get="/ui/tests"      hx-target="#content"

Tests:
  Category <select>  → hx-get="/ui/tests?category=X"  hx-target="#test-list"  trigger:change
  Test row click     → hx-get="/ui/tests/{name}"      hx-target="#content"

Test Detail:
  Run row click      → hx-get="/ui/runs/{id}"         hx-target="#content"
  Run Test button    → hx-post="/ui/launch/{name}"    hx-target="#launch-area"
  Load more          → hx-get="?offset=20"            hx-target="#run-table"  swap:outerHTML

Run Detail:
  Event row click    → hx-get="/ui/runs/{id}/events/{eid}"  hx-target="#event-{eid}"

Launch:
  SSE connected      → hx-trigger="sse:/ui/launch/{name}/events"  hx-target="#log-output"  swap:beforeend
```

## Scenarios

### Dashboard loads with stats
- **WHEN** `GET /ui/`
- **THEN** stat cards, recent runs table, sidebar nav rendered

### Tests page with category filter
- **WHEN** `GET /ui/tests`
- **THEN** tests grouped by category with status badges
- **AND** category filter triggers HTMX partial swap

### Test detail with run history
- **WHEN** `GET /ui/tests/TestFoo`
- **THEN** run history table with pass/fail/skip badges, durations

### Run detail with expandable events
- **WHEN** `GET /ui/runs/42`
- **THEN** event timeline with kind badges
- **AND** click expands children via HTMX partial

### Live test launch with SSE
- **WHEN** POST `/ui/launch/TestFoo`
- **THEN** button replaced with `<pre>` log viewer
- **AND** SSE stream delivers stdout/stderr in real time
- **AND** completion event updates status

### Existing API preserved
- **WHEN** `GET /health`
- **THEN** 200 `{"status":"ok"}`
- **AND** all existing daemon routes unchanged

## Dependencies

Add to `go.mod`:
- `github.com/emergent-company/go-daisy`
- `github.com/labstack/echo/v4`
- `github.com/a-h/templ`

## Implementation Order

1. Dependencies (`go.mod` + `go mod tidy`)
2. Templates (all `.templ` files)
3. `web.go` — Echo setup, routes, LauncherManager
4. `web_handlers.go` — all handler functions
5. Modify `daemon.go` — mount `/ui/`
6. `templ generate` + `go build` — verify
7. Manual testing
