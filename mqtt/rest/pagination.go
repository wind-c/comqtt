// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2022 mochi-mqtt, mochi-co
// SPDX-FileContributor: mochi-co

package rest

import "strings"

const defaultPageSize = 20
const maxPageSize = 100

type PagedRequest struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Search   string `json:"search"`
}

type PagedResponse[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

func Paginate[T any](items []T, req PagedRequest) PagedResponse[T] {
	if req.PageSize <= 0 {
		req.PageSize = defaultPageSize
	}
	if req.PageSize > maxPageSize {
		req.PageSize = maxPageSize
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	filtered := items
	if req.Search != "" {
		filtered = nil
		for _, item := range items {
			if matchesSearch(item, req.Search) {
				filtered = append(filtered, item)
			}
		}
	}

	total := len(filtered)
	start := (req.Page - 1) * req.PageSize
	if start >= total {
		return PagedResponse[T]{
			Items:    []T{},
			Total:    total,
			Page:     req.Page,
			PageSize: req.PageSize,
		}
	}

	end := start + req.PageSize
	if end > total {
		end = total
	}

	return PagedResponse[T]{
		Items:    filtered[start:end],
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}
}

func matchesSearch(item any, search string) bool {
	s := strings.ToLower(search)
	v, ok := item.(string)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(v), s)
}
