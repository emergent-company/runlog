package alpine

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

// toastItemState is a single toast in the queue.
type toastItemState struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
	Duration int    `json:"duration"`
}

// toastQueueState is the x-data state for a toast notification queue.
type toastQueueState struct {
	Toasts []toastItemState `json:"toasts"`
}

// ToastQueue returns an x-data state for a toast notification queue.
func ToastQueueState() State {
	return toastQueueState{}
}

// comboboxState is the x-data state for a searchable combobox with keyboard nav.
type comboboxState struct {
	Open        bool     `json:"open"`
	ActiveIndex int      `json:"activeIndex"`
	Search      string   `json:"search"`
	Selected    []string `json:"selected"`
}

// Combobox returns an x-data state for a combobox.
func ComboboxState(selected []string) State {
	return comboboxState{Open: false, ActiveIndex: -1, Selected: selected}
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
