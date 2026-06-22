package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/a-h/templ"
)

// DefaultPageSize is the default number of rows per page for table views.
const DefaultPageSize = 50

// FilterType indicates the type of filter control to render.
type FilterType int

const (
	FilterSelect   FilterType = iota
	FilterText
	FilterCheckbox
	FilterStatus
)

// FilterOption is one option in a select filter.
type FilterOption struct {
	Value string
	Label string
}

// FilterConfig describes one filter input in the filter bar.
type FilterConfig struct {
	Type        FilterType
	Name        string
	Label       string
	Placeholder string
	Value       string
	Options     []FilterOption
}

// ColumnAlign controls text alignment within a column.
type ColumnAlign string

const (
	ColLeft   ColumnAlign = "text-left"
	ColRight  ColumnAlign = "text-right"
	ColCenter ColumnAlign = "text-center"
)

// ColumnConfig describes one column in the table.
// Render is a function that takes a row (as any) and returns a templ.Component.
type ColumnConfig struct {
	Label  string
	Width  string
	Align  ColumnAlign
	Render func(any) templ.Component
}

// RowClickFunc returns an hx-get URL for a row, or empty string if not clickable.
type RowClickFunc func(row any) string

// TableConfig is the complete configuration for a TableView component.
type TableConfig struct {
	Title     string
	HXGet     string
	HXTarget  string
	TargetID  string
	EmptyMsg  string
	Filters   []FilterConfig
	Columns   []ColumnConfig
	Rows      []any
	Total     int
	Offset    int
	PageSize  int
	RowClick  RowClickFunc
}

// filterBarHXTrigger builds the hx-trigger attribute for the filter form.
func filterBarHXTrigger(filters []FilterConfig) string {  //nolint:deadcode
	var parts []string
	hasSelect := false
	hasText := false
	for _, f := range filters {
		switch f.Type {
		case FilterSelect, FilterStatus:
			hasSelect = true
		case FilterText:
			hasText = true
		}
	}
	if hasSelect {
		parts = append(parts, "change from:select")
	}
	if hasText {
		parts = append(parts, "keyup changed delay:500ms from:input[name]")
	}
	return strings.Join(parts, ", ")
}

// loadMoreURL builds the URL for the Load More button including all filter params.
func loadMoreURL(base string, offset int, filters []FilterConfig) string {  //nolint:deadcode
	vals := url.Values{}
	vals.Set("offset", fmt.Sprintf("%d", offset))
	for _, f := range filters {
		if f.Value != "" {
			vals.Set(f.Name, f.Value)
		}
	}
	return base + "?" + vals.Encode()
}

// statusOptions returns the standard set of filter options for test run status.
func statusOptions() []FilterOption {  //nolint:deadcode
	return []FilterOption{
		{Value: "pass", Label: "Pass"},
		{Value: "fail", Label: "Fail"},
		{Value: "skip", Label: "Skip"},
		{Value: "running", Label: "Running"},
		{Value: "never_run", Label: "Never Run"},
		{Value: "timeout", Label: "Timeout"},
	}
}
