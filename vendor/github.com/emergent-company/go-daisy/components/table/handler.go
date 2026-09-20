package table

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SortParams holds extracted sort state from query params.
type SortParams struct {
	By  string
	Dir SortDir
}

// ParseSort extracts sort_by and sort_dir from query params.
// Defaults to defaultBy and defaultDir when params are absent.
func ParseSort(q url.Values, defaultBy string, defaultDir SortDir) SortParams {
	by := strings.TrimSpace(q.Get("sort_by"))
	if by == "" {
		by = defaultBy
	}
	dir := SortAsc
	rawDir := strings.ToLower(strings.TrimSpace(q.Get("sort_dir")))
	if rawDir == "" {
		dir = defaultDir
	} else if rawDir == "desc" {
		dir = SortDesc
	}
	return SortParams{By: by, Dir: dir}
}

// ParseSortFromRequest is a convenience wrapper for ParseSort(r.URL.Query(), ...).
func ParseSortFromRequest(r *http.Request, defaultBy string, defaultDir SortDir) SortParams {
	return ParseSort(r.URL.Query(), defaultBy, defaultDir)
}

// NextSortDir cycles: none → asc → desc → none. Returns "asc" when current is empty.
func NextSortDir(current SortDir, column string, sortBy string) SortDir {
	if column != sortBy {
		return SortAsc
	}
	switch current {
	case SortAsc:
		return SortDesc
	case SortDesc:
		return SortNone
	default:
		return SortAsc
	}
}

// SortIcon returns the sort indicator icon for a column.
func SortIcon(col string, sp SortParams) string {
	if col != sp.By {
		return ""
	}
	if sp.Dir == SortAsc {
		return "↑"
	}
	return "↓"
}

// SortLink builds a query string for sorting a specific column.
func SortLink(q url.Values, column string, sp SortParams) string {
	dir := NextSortDir(sp.Dir, column, sp.By)
	params := cloneValues(q)
	if dir == SortNone {
		params.Del("sort_by")
		params.Del("sort_dir")
	} else {
		params.Set("sort_by", column)
		params.Set("sort_dir", string(dir))
	}
	return params.Encode()
}

// PaginationParams holds extracted pagination state from query params.
type PaginationParams struct {
	Page    int
	PerPage int
	Offset  int
}

// ParsePagination extracts page and per_page from query params.
// Defaults to page 1, perPage 10. Sanitizes to ≥ 1.
func ParsePagination(q url.Values, defaultPerPage int) PaginationParams {
	if defaultPerPage <= 0 {
		defaultPerPage = 10
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 {
		perPage = defaultPerPage
	}
	return PaginationParams{
		Page:    page,
		PerPage: perPage,
		Offset:  (page - 1) * perPage,
	}
}

// ParsePaginationFromRequest is a convenience wrapper.
func ParsePaginationFromRequest(r *http.Request, defaultPerPage int) PaginationParams {
	return ParsePagination(r.URL.Query(), defaultPerPage)
}

// PageLink builds a query string for a specific page number.
func PageLink(q url.Values, page int) string {
	params := cloneValues(q)
	if page <= 1 {
		params.Del("page")
	} else {
		params.Set("page", strconv.Itoa(page))
	}
	return params.Encode()
}

// FilterParams holds extracted filter values from query params.
type FilterParams map[string]string

// ParseFilters extracts known filter keys from query params.
// Only keys present in the keys slice are included.
func ParseFilters(q url.Values, keys ...string) FilterParams {
	f := make(FilterParams)
	for _, key := range keys {
		v := strings.TrimSpace(q.Get(key))
		f[key] = v
	}
	return f
}

// ParseFiltersFromRequest is a convenience wrapper.
func ParseFiltersFromRequest(r *http.Request, keys ...string) FilterParams {
	return ParseFilters(r.URL.Query(), keys...)
}

// cloneValues copies url.Values without mutating the original.
func cloneValues(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for k, vv := range src {
		vv2 := make([]string, len(vv))
		copy(vv2, vv)
		dst[k] = vv2
	}
	return dst
}

// PaginationMeta holds computed pagination metadata.
type PaginationMeta struct {
	TotalPages int
	HasNext    bool
	HasPrev    bool
	Offset     int
	Page       int
	PerPage    int
	Total      int
}

// CalcPaginationMeta computes pagination metadata from a total count and
// pagination params. Useful for setting response headers or rendering
// pagination components.
func CalcPaginationMeta(total int, pg PaginationParams) PaginationMeta {
	totalPages := total / pg.PerPage
	if total%pg.PerPage != 0 {
		totalPages++
	}
	if totalPages < 1 {
		totalPages = 1
	}
	return PaginationMeta{
		TotalPages: totalPages,
		HasNext:    pg.Page < totalPages,
		HasPrev:    pg.Page > 1,
		Offset:     pg.Offset,
		Page:       pg.Page,
		PerPage:    pg.PerPage,
		Total:      total,
	}
}
