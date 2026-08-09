package handlers

import (
	"net/http"
	"strconv"
)

func parsePagination(r *http.Request) (page, pageSize int32) {
	page = 1
	pageSize = 20
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = int32(p)
	}
	if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && ps > 0 && ps <= 100 {
		pageSize = int32(ps)
	}
	return
}
