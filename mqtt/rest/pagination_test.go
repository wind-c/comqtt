// mqtt/rest/pagination_test.go
package rest

import (
	"net/url"
	"testing"
)

func TestParsePageDefaults(t *testing.T) {
	p := ParsePage(url.Values{})
	if p.Page != 1 || p.Size != 25 {
		t.Fatalf("defaults: %+v", p)
	}
}

func TestParsePageClamps(t *testing.T) {
	p := ParsePage(url.Values{"page": {"0"}, "size": {"5000"}})
	if p.Page != 1 {
		t.Fatalf("page should be clamped to 1, got %d", p.Page)
	}
	if p.Size != MaxPageSize {
		t.Fatalf("size should be clamped to %d, got %d", MaxPageSize, p.Size)
	}
}

func TestApplyPaginationSlices(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	got := ApplyPagination(items, PageParams{Page: 2, Size: 3})
	want := []int{3, 4, 5}
	if !equalInts(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
