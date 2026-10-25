package domain

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Page is a 1-based page request.
type Page struct {
	Number int
	Size   int
}

// Validate checks the page bounds.
func (p Page) Validate() error {
	verr := &ValidationError{}
	if p.Number < 1 {
		verr.Add("page", "must be at least 1")
	}
	if p.Size < 1 || p.Size > MaxPageSize {
		verr.Add("page_size", "must be between 1 and 100")
	}
	return verr.OrNil()
}

// Offset is the number of items to skip.
func (p Page) Offset() int {
	return (p.Number - 1) * p.Size
}

// TotalPages returns how many pages of p.Size cover total items.
func (p Page) TotalPages(total int64) int64 {
	if total == 0 {
		return 0
	}
	return (total + int64(p.Size) - 1) / int64(p.Size)
}
