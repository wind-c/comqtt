// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaginateAllItems(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10})

	require.Equal(t, 5, result.Total)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 10, result.PageSize)
	require.Len(t, result.Items, 5)
	require.Equal(t, "a", result.Items[0])
	require.Equal(t, "e", result.Items[4])
}

func TestPaginateFirstPage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 3})

	require.Equal(t, 11, result.Total)
	require.Equal(t, 1, result.Page)
	require.Len(t, result.Items, 3)
	require.Equal(t, "a", result.Items[0])
	require.Equal(t, "c", result.Items[2])
}

func TestPaginateMiddlePage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	result := Paginate(items, PagedRequest{Page: 2, PageSize: 3})

	require.Equal(t, 8, result.Total)
	require.Equal(t, 2, result.Page)
	require.Len(t, result.Items, 3)
	require.Equal(t, "d", result.Items[0])
	require.Equal(t, "f", result.Items[2])
}

func TestPaginateLastPartialPage(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	result := Paginate(items, PagedRequest{Page: 2, PageSize: 3})

	require.Equal(t, 5, result.Total)
	require.Equal(t, 2, result.Page)
	require.Len(t, result.Items, 2)
	require.Equal(t, "d", result.Items[0])
	require.Equal(t, "e", result.Items[1])
}

func TestPaginatePageOutOfRange(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := Paginate(items, PagedRequest{Page: 5, PageSize: 3})

	require.Equal(t, 3, result.Total)
	require.Equal(t, 5, result.Page)
	require.Len(t, result.Items, 0)
}

func TestPaginateEmptyItems(t *testing.T) {
	var items []string
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10})

	require.Equal(t, 0, result.Total)
	require.Equal(t, 1, result.Page)
	require.Len(t, result.Items, 0)
}

func TestPaginateDefaultPageSize(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := Paginate(items, PagedRequest{Page: 1})

	require.Equal(t, 3, result.Total)
	require.Equal(t, 1, result.Page)
	require.Equal(t, 20, result.PageSize)
	require.Len(t, result.Items, 3)
}

func TestPaginateNoSearchFilter(t *testing.T) {
	items := []string{"alpha", "beta", "gamma", "delta"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10})

	require.Equal(t, 4, result.Total)
	require.Len(t, result.Items, 4)
}

func TestPaginateSearchFilterCaseInsensitive(t *testing.T) {
	items := []string{"Alpha", "BETA", "gamma", "delta"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10, Search: "alpha"})

	require.Equal(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "Alpha", result.Items[0])
}

func TestPaginateSearchWithPagination(t *testing.T) {
	items := []string{"alpha", "beta", "gamma", "alpha2", "alpha3", "delta", "epsilon"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 2, Search: "alpha"})

	require.Equal(t, 3, result.Total)
	require.Len(t, result.Items, 2)
	require.Equal(t, "alpha", result.Items[0])
	require.Equal(t, "alpha2", result.Items[1])
}

func TestPaginateSearchNoMatch(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10, Search: "xyz"})

	require.Equal(t, 0, result.Total)
	require.Len(t, result.Items, 0)
}

func TestPaginateSearchEmptySearchString(t *testing.T) {
	items := []string{"alpha", "beta"}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 10, Search: ""})

	require.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
}

func TestPaginateMaxPageSizeEnforced(t *testing.T) {
	items := make([]string, 200)
	for i := range items {
		items[i] = "item-" + string(rune('a'+i%26))
	}
	result := Paginate(items, PagedRequest{Page: 1, PageSize: 500})

	require.Equal(t, maxPageSize, result.PageSize)
	require.Len(t, result.Items, maxPageSize)
	require.Equal(t, 200, result.Total)
}
