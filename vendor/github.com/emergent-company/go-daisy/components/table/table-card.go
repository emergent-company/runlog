package table

import "github.com/a-h/templ"

// TableCardFooterProps configures the rich footer with per-page, item count, and circle pagination.
type TableCardFooterProps struct {
	CurrentPage int
	TotalPages  int
	TotalItems  int64
	PageSize    int
	BaseURL     string
	TargetID    string
	Attrs       templ.Attributes
}

// StartItem returns the 1-based index of the first item on the current page.
func (p TableCardFooterProps) StartItem() int64 {
	if p.TotalItems == 0 {
		return 0
	}
	return int64((p.CurrentPage-1)*p.PageSize) + 1
}

// EndItem returns the 1-based index of the last item on the current page.
func (p TableCardFooterProps) EndItem() int64 {
	end := int64(p.CurrentPage * p.PageSize)
	if end > p.TotalItems {
		return p.TotalItems
	}
	return end
}
