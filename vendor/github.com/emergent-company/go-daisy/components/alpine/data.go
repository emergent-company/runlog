package alpine

import (
	"encoding/json"
	"strconv"
)

// ── Pre-built Alpine data states ──────────────────────────────────────────

// toggleState is the x-data state for show/hide toggles (dropdowns, modals, drawers).
type toggleState struct {
	Open bool `json:"open"`
}

// Toggle returns an x-data state for a simple open/closed toggle.
func Toggle(open bool) State {
	return toggleState{Open: open}
}

// tabsState is the x-data state for tab panels.
type tabsState struct {
	Tab string `json:"tab"`
}

// Tabs returns an x-data state for a tab panel with the given active tab.
func TabState(active string) State {
	return tabsState{Tab: active}
}

// dropdownState is the x-data state for dropdown menus with keyboard navigation.
type dropdownState struct {
	Open        bool `json:"open"`
	ActiveIndex int  `json:"activeIndex"`
}

// Dropdown returns an x-data state for a keyboard-navigable dropdown.
func DropdownState(open bool) State {
	return dropdownState{Open: open, ActiveIndex: -1}
}

// themeState is the x-data state for theme toggling.
type themeState struct {
	Dark bool `json:"dark"`
}

// Theme returns an x-data state for dark/light theme toggle.
func ThemeState(dark bool) State {
	return themeState{Dark: dark}
}

// counterState is the x-data state for simple increment/decrement counters.
type counterState struct {
	Count int `json:"count"`
}

// Counter returns an x-data state for a counter.
func CounterState(count int) State {
	return counterState{Count: count}
}

// accordionState is the x-data state for accordion panels.
// openItem is the key of the open item, or empty string for all closed.
type accordionState struct {
	Open string `json:"open"`
}

// Accordion returns an x-data state for an accordion.
func AccordionState(openItem string) State {
	return accordionState{Open: openItem}
}

// searchState is the x-data state for search/filter inputs.
type searchState struct {
	Search string   `json:"search"`
	Items  []string `json:"items"`
}

// Search returns an x-data state for a search input with item list.
func SearchState(items []string) State {
	return searchState{Search: "", Items: items}
}

// formState is the x-data state for form-level validation and submission.
type formState struct {
	Submitting bool   `json:"submitting"`
	Error      string `json:"error,omitempty"`
	Success    string `json:"success,omitempty"`
}

// Form returns an x-data state for form tracking.
func FormState() State {
	return formState{}
}

// modalState is the x-data state for modals with built-in scroll lock.
type modalState struct {
	Open bool `json:"open"`
}

// Modal returns an x-data state for a modal with appropriate lifecycle.
func ModalState(open bool) State {
	return modalState{Open: open}
}

// ToastItem is a single toast in the queue.
//
// Type is the DaisyUI alert modifier suffix ("success", "info", "warning",
// "error") — the queue renders it as "alert-" + Type. Duration is the
// auto-dismiss timeout in milliseconds; 0 (or negative) keeps the toast until
// dismissed.
type ToastItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
	Duration int    `json:"duration"`
}

// toastQueueState is the x-data state for a toast notification queue.
type toastQueueState struct {
	Toasts []ToastItem `json:"toasts"`
}

// ToastQueue returns an x-data state for a toast notification queue.
// Optional seed items are rendered on initial load (persistent — auto-dismiss
// timers only run for toasts added via add()).
//
// Seed items with an empty ID get a unique "seed-N" ID assigned, because
// Alpine's x-for :key requires unique keys — duplicate (empty) keys cause the
// template to render nothing.
//
// The toasts slice is always a non-nil slice so it marshals to JSON [] rather
// than null — Alpine can't push onto a null array.
func ToastQueueState(seed ...ToastItem) State {
	toasts := make([]ToastItem, 0, len(seed))
	for i, t := range seed {
		if t.ID == "" {
			t.ID = "seed-" + strconv.Itoa(i)
		}
		toasts = append(toasts, t)
	}
	return toastQueueState{Toasts: toasts}
}

// comboboxState is the x-data state for a searchable combobox with keyboard nav.
type comboboxState struct {
	Open        bool              `json:"open"`
	ActiveIndex int               `json:"activeIndex"`
	Search      string            `json:"search"`
	Selected    []string          `json:"selected"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Combobox returns an x-data state for a combobox. labels maps option values to
// their display labels so the trigger can show the label instead of the raw value.
func ComboboxState(selected []string, labels map[string]string) State {
	if selected == nil {
		selected = []string{}
	}
	if labels == nil {
		labels = map[string]string{}
	}
	return comboboxState{Open: false, ActiveIndex: -1, Selected: selected, Labels: labels}
}

// structuredInputState is the x-data state for repeatable key-value rows.
type structuredInputState struct {
	Rows []map[string]string `json:"rows"`
}

// StructuredInput returns an x-data state for repeatable structured input rows.
func StructuredInputState(rows []map[string]string) State {
	if rows == nil {
		rows = []map[string]string{}
	}
	return structuredInputState{Rows: rows}
}

// tagListState is the x-data state for editable tag collections.
type tagListState struct {
	Tags  []string `json:"tags"`
	Input string   `json:"input"`
}

// TagList returns an x-data state for an editable tag list.
func TagListState(tags []string) State {
	if tags == nil {
		tags = []string{}
	}
	return tagListState{Tags: tags}
}

// TagListData returns a self-contained Alpine x-data expression for an
// editable tag list with suggestion autocomplete: the reactive state (tags,
// input, open, activeIndex, suggestions) plus addTag/removeTag/selectSuggestion/
// handleKeydown methods inlined so they are accessible to @click/@keydown.
//
// The legacy "x-init = var root=this; root.addTag=..." pattern does NOT bind
// methods into the directive scope on Alpine 3.15+, so state and methods are
// emitted together as a single x-data object literal instead.
func TagListData(tags, suggestions []string) string {
	// json.Marshal(nil) emits `null`, which breaks the Alpine methods below
	// (they call .push/.indexOf/.length on the slices). Normalise to [] so the
	// x-data initialises empty arrays instead of null.
	if tags == nil {
		tags = []string{}
	}
	if suggestions == nil {
		suggestions = []string{}
	}
	tj, _ := json.Marshal(tags)
	sj, _ := json.Marshal(suggestions)
	return `{ tags: ` + string(tj) + `, input: '', open: false, activeIndex: -1, suggestions: ` + string(sj) + `, get filteredSuggestions() { var q = this.input.trim().toLowerCase(); var out = []; for (var i = 0; i < this.suggestions.length; i++) { var s = this.suggestions[i]; if (this.tags.indexOf(s) >= 0) continue; if (!q || s.toLowerCase().indexOf(q) >= 0) out.push(s); } return out; }, addTag() { var v = this.input.trim(); if (!v) return; var parts = v.split(','); for (var i = 0; i < parts.length; i++) { var t = parts[i].trim(); if (t && this.tags.indexOf(t) < 0) this.tags.push(t); } this.input = ''; this.activeIndex = -1; this.open = false; }, selectSuggestion(v) { if (!v || this.tags.indexOf(v) >= 0) return; this.tags.push(v); this.input = ''; this.activeIndex = -1; this.open = false; }, removeTag(idx) { this.tags.splice(idx, 1); }, tagColor(tag) { var hues = [0, 205, 150, 30, 270, 330, 90, 180]; var h = 0; for (var i = 0; i < tag.length; i++) { h = (h * 31 + tag.charCodeAt(i)) | 0; } var hue = hues[Math.abs(h) % hues.length]; return 'background-color: hsl(' + hue + ' 65% 24%); color: hsl(' + hue + ' 70% 88%); border-color: hsl(' + hue + ' 65% 34%);'; }, handleKeydown(e) { if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { var list = this.filteredSuggestions; if (!list.length) return; e.preventDefault(); this.open = true; var n = list.length; if (this.activeIndex < 0 || this.activeIndex >= n) this.activeIndex = e.key === 'ArrowDown' ? -1 : n; this.activeIndex = e.key === 'ArrowDown' ? (this.activeIndex + 1) % n : (this.activeIndex - 1 + n) % n; return; } if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); if (this.activeIndex >= 0 && this.activeIndex < this.filteredSuggestions.length) { this.selectSuggestion(this.filteredSuggestions[this.activeIndex]); } else { this.addTag(); } return; } if (e.key === 'Escape') { this.open = false; this.activeIndex = -1; return; } if (e.key === 'Backspace' && !this.input && this.tags.length) { this.removeTag(this.tags.length - 1); } } }`
}
