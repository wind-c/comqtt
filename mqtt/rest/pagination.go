// mqtt/rest/pagination.go
package rest

import (
	"net/url"
	"strconv"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 500
)

type PageParams struct {
	Page int
	Size int
}

func ParsePage(q url.Values) PageParams {
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(q.Get("size"))
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return PageParams{Page: page, Size: size}
}

func ApplyPagination[T any](items []T, p PageParams) []T {
	start := (p.Page - 1) * p.Size
	end := start + p.Size
	if start >= len(items) {
		return nil
	}
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

type Page[T any] struct {
	Page  int `json:"page"`
	Size  int `json:"size"`
	Total int `json:"total"`
	Items []T `json:"items"`
}
