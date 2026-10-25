package domain_test

import (
	"testing"

	"github.com/lnwbigbangsm-ops/backend-golang-coding-test/internal/core/domain"
)

func TestPage(t *testing.T) {
	p := domain.Page{Number: 3, Size: 20}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if p.Offset() != 40 {
		t.Errorf("Offset = %d, want 40", p.Offset())
	}
	for total, want := range map[int64]int64{0: 0, 1: 1, 20: 1, 21: 2, 100: 5} {
		if got := p.TotalPages(total); got != want {
			t.Errorf("TotalPages(%d) = %d, want %d", total, got, want)
		}
	}
	for _, bad := range []domain.Page{{Number: 0, Size: 1}, {Number: 1, Size: 0}, {Number: 1, Size: domain.MaxPageSize + 1}} {
		if bad.Validate() == nil {
			t.Errorf("Validate(%+v) = nil, want error", bad)
		}
	}
}
