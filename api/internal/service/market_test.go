package service

import (
	"errors"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

func TestBuildSearchParamsDefaults(t *testing.T) {
	q := domain.ListingSearch{}
	p, err := buildSearchParams(&q)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sort != "newest" || q.Page != 1 || q.PageSize != domain.DefaultPageSize {
		t.Fatalf("defaults not applied: params=%+v query=%+v", p, q)
	}
	if p.PageLimit != int32(domain.DefaultPageSize+1) || p.PageOffset != 0 {
		t.Fatalf("limit/offset = %d/%d", p.PageLimit, p.PageOffset)
	}
	if p.Search != nil || p.Exteriors == nil || len(p.Exteriors) != 0 {
		t.Fatalf("empty filters should be nil search and empty (non-nil) exteriors: %+v", p)
	}
}

func TestBuildSearchParamsMapsFilters(t *testing.T) {
	q := domain.ListingSearch{
		Text:     "  ak 100%_x  ",
		Wear:     []string{"fn", "FT"},
		Page:     3,
		PageSize: 500,
		Sort:     domain.SortPriceAsc,
	}
	p, err := buildSearchParams(&q)
	if err != nil {
		t.Fatal(err)
	}
	if *p.Search != `ak 100\%\_x` {
		t.Errorf("search = %q", *p.Search)
	}
	if len(p.Exteriors) != 2 || p.Exteriors[0] != "Factory New" || p.Exteriors[1] != "Field-Tested" {
		t.Errorf("exteriors = %v", p.Exteriors)
	}
	if q.PageSize != domain.MaxPageSize {
		t.Errorf("page size should be capped at %d, got %d", domain.MaxPageSize, q.PageSize)
	}
	if p.PageOffset != int32(2*domain.MaxPageSize) {
		t.Errorf("offset = %d", p.PageOffset)
	}
}

func TestBuildSearchParamsRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		q    domain.ListingSearch
	}{
		{"unknown sort", domain.ListingSearch{Sort: "cheapest"}},
		{"unknown wear", domain.ListingSearch{Wear: []string{"XX"}}},
		{"min price > max price", domain.ListingSearch{MinPrice: ptr(int64(500)), MaxPrice: ptr(int64(100))}},
		{"float above 1", domain.ListingSearch{MaxFloat: ptr(1.5)}},
		{"min float > max float", domain.ListingSearch{MinFloat: ptr(0.5), MaxFloat: ptr(0.1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildSearchParams(&tt.q)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
