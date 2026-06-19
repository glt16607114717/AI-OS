package handler

import (
	"ai-os-server/service"
	"net/http"
	"strconv"
)

// ── 用量统计 ──

func GetStatsSummary(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days, _ := strconv.Atoi(daysStr)
	if days == 0 {
		days = 7
	}
	summary, err := service.GetStatsSummary(days)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, summary)
}

func GetRecentErrors(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 20
	}
	errors, err := service.GetRecentErrors(limit)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, errors)
}

func CleanupStats(w http.ResponseWriter, r *http.Request) {
	n := service.CleanupStats()
	okResponse(w, map[string]interface{}{"deleted": n})
}
