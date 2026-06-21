# Pattern: Table + Filters + Pagination

## Overview

Runlog uses HTMX for interactive table views with server-side filtering,
paginated "load more", and push-state URL updates. Every filter action is a
GET request that returns an HTML partial — no client-side rendering.

## Anatomy

```
┌──────────────────────────────────────────────┐
│  Title                                       │
├──────────────────────────────────────────────┤
│  [Category ▼]  [Status ▼]  [Search...]       │
│     ^-- <form hx-get hx-target hx-push-url   │
├──────────────────────────────────────────────┤
│  TABLE                                       │
│  ┌─────┬────────┬──────────┬────────┐        │
│  │ Seq │ Name   │ Status   │ Last   │        │
│  ├─────┼────────┼──────────┼────────┤        │
│  │ 1   │ Foo    │ Pass     │ 12:00  │        │
│  │ 2   │ Bar    │ Fail     │ 12:01  │        │
│  └─────┴────────┴──────────┴────────┘        │
│  [Load More] ← only if more rows exist        │
└──────────────────────────────────────────────┘
```

## Implementation

### 1. Templates (`.templ`)

**Two variants of each view:**

| Variant | When | What it renders |
|---------|------|----------------|
| `XxxPage` | Full page load | `<form>` filters + title + table |
| `XxxContent` | HTMX partial | Called by both full page and HTMX |

**Filter form** — wraps all filter inputs in a `<form>` with `hx-*`:

```templ
<form
    class="flex flex-wrap gap-3 mb-4 items-end"
    hx-get="/ui/runs"
    hx-target="#runs-table"
    hx-trigger="change from:select, keyup changed delay:500ms from:input"
    hx-push-url="true"
>
    <select name="category">...</select>
    <select name="status">...</select>
    <input name="search" placeholder="Search...">
</form>
```

Rules:
- Use `<form>` not `<div>` — HTMX auto-serializes form controls as query params
- `hx-target` targets the table container, not the form itself
- `hx-push-url="true"` updates browser URL with query params
- `hx-trigger` specifies which events fire the request:
  - `change from:select` — dropdown changes
  - `keyup changed delay:500ms from:input` — search with debounce

**Table partial** — only the table + load-more button, no filters/title:

```templ
templ runsTableContent(rows []Row, total int, f Filters) {
    if len(rows) > 0 {
        @renderTable(rows)
        if f.Offset + pageSize < total {
            @loadMoreButton(buildURL("/ui/runs", f.Offset + pageSize, f), "#runs-table")
        }
    } else {
        @emptyState("No results match filters.")
    }
}
```

### 2. Handler (Go)

**Single handler serves both full page and HTMX partial:**

```go
func (app *WebApp) handleList(c echo.Context) error {
    // 1. Parse filter params from query
    category := c.QueryParam("category")
    status := c.QueryParam("status")
    search := c.QueryParam("search")
    offset, _ := strconv.Atoi(c.QueryParam("offset"))

    // 2. Query DB with filters + pagination
    rows, total := queryDB(category, status, search, offset, pageSize)

    // 3. Build filter state for template (preserves selected values)
    f := Filters{
        Category: category,
        Status:   status,
        Search:   search,
        Offset:   offset,
    }

    // 4. Render — RenderAuto decides full page vs HTMX partial
    render.RenderAuto(w, r, ListPage(rows, total, f), ListContent(rows, total, f))
    return nil
}
```

Key points:
- `RenderAuto` checks HTMX headers: full page → `ListPage`, HTMX → `ListContent`
- Filter params flow: query string → handler → template → form keeps selected values
- URL is the single source of truth for filter state

### 3. Pagination: "Load More"

**Template** — rendered after the table when more rows exist:

```templ
templ loadMoreButton(url string, target string) {
    <button
        class="btn btn-ghost btn-sm w-full mt-4"
        hx-get={ url }
        hx-target={ target }
        hx-swap="beforeend"
    >Load More</button>
}
```

- `hx-swap="beforeend"` appends new rows to existing table
- URL includes all current filter params + incremented offset
- Button disappears when `offset + pageSize >= total`

**Handler** — offset param already handled by the same query:

```go
offset, _ := strconv.Atoi(c.QueryParam("offset"))
rows := queryDB(offset, pageSize)
```

### 4. URL Query Params

```
/ui/runs?category=auth&status=pass&search=login&offset=50
```

Benefits:
- Shareable/bookmarkable URLs
- Browser back/forward works
- Single source of truth: no client state
- `hx-push-url="true"` keeps URL in sync on every filter change

### 5. Filter Types

| Type | Implementation | HTMX trigger |
|------|---------------|--------------|
| Dropdown (`<select>`) | `name="category"` | `change from:select` |
| Text search (`<input>`) | `name="search"` | `keyup changed delay:500ms from:input` |
| Checkbox | `name="has_cost" value="1"` | `change from:[type=checkbox]` |

## Existing Implementations

| File | View | Filters |
|------|------|---------|
| `tests.templ` | Tests | Category (select), Status (select) |
| `all_runs.templ` | All Runs | Category, Status, Since, Search, Tags, Has cost |

## Anti-patterns to avoid

| Anti-pattern | Why | Fix |
|-------------|-----|-----|
| Individual `hx-get` on each `<select>` | Params not serialized together, URL only has one param | Use `<form>` wrapper with `hx-trigger` |
| `<div>` instead of `<form>` | HTMX doesn't auto-serialize child inputs | Use `<form hx-get ...>` |
| `hx-target` on the form itself | Response replaces the form, losing state | Target the table container, not the form |
| Client-side state (React/Vue) | Duplicates server state, breaks URL sharing | Server-rendered partials + HTMX |
| Fetch-on-scroll without load-more | Hard to test, no URL representation | Load More button + offset param |
