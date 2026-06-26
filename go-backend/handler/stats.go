package handler

import (
	"ai-os-server/circuit"
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

// ── 错误日志 ──

func GetErrorLog(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	modelFilter := r.URL.Query().Get("model")
	userFilter := r.URL.Query().Get("user")

	list, total, err := service.GetErrorLog(page, pageSize, modelFilter, userFilter)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{
		"list": list, "total": total, "page": page, "page_size": pageSize,
	})
}

func GetErrorStats(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days, _ := strconv.Atoi(daysStr)
	if days == 0 {
		days = 7
	}
	stats, err := service.GetErrorStats(days)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, stats)
}

// ── 熔断状态 ──

func GetCircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	cb := circuit.GetBreaker()
	if cb == nil {
		okResponse(w, map[string]interface{}{"open_keys": []interface{}{}})
		return
	}
	okResponse(w, map[string]interface{}{
		"open_keys": cb.GetOpenKeys(),
	})
}
