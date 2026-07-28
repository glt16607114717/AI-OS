package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strconv"
)

// ── 知识库巡检 API ──
//
// 路由（main.go 注册）：
//   GET  /api/audit/reports                              - 报告列表（分页+日期范围）
//   GET  /api/audit/reports/{id}                         - 报告详情
//   POST /api/audit/run                                  - 手动触发巡检（管理员）
//   GET  /api/audit/pending-archives                     - 待归档列表
//   POST /api/audit/pending-archives/{id}/approve        - 审核通过（管理员）
//   POST /api/audit/pending-archives/{id}/reject         - 审核驳回（管理员）

// GetAuditReports 获取巡检报告列表
// GET /api/audit/reports?page=1&page_size=20&start_date=2026-07-01&end_date=2026-07-21
func GetAuditReports(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	reports, total, err := service.GetAuditReports(page, pageSize, startDate, endDate)
	if err != nil {
		errResponse(w, "获取报告列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	okResponse(w, map[string]interface{}{
		"list":  reports,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// GetAuditReportDetail 获取报告详情
// GET /api/audit/reports/{id}
func GetAuditReportDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/audit/reports/"):]
	// 去掉可能的尾部斜杠
	for len(idStr) > 0 && idStr[len(idStr)-1] == '/' {
		idStr = idStr[:len(idStr)-1]
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		errResponse(w, "无效的报告 ID", http.StatusBadRequest)
		return
	}

	detail, err := service.GetAuditReportDetail(id)
	if err != nil {
		errResponse(w, "获取报告详情失败: "+err.Error(), http.StatusNotFound)
		return
	}

	okResponse(w, detail)
}

// RunAuditManually 手动触发巡检（管理员）
// POST /api/audit/run  body: {"date": "2026-07-20"}  不传 date 默认评审昨天
func RunAuditManually(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}

	// 解析请求体（可选 date 字段）
	var req struct {
		Date string `json:"date"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	date := req.Date

	// 异步执行（评审可能耗时 30 秒+）
	go func() {
		if date == "" {
			service.RunDailyAudit() // 默认评审昨天
		} else {
			service.RunAuditForDate(date)
		}
	}()

	okResponse(w, map[string]interface{}{
		"message": "巡检任务已触发（异步执行，约 1-2 分钟后查看报告）",
		"date":    date,
		"trigger": session.Username,
	})
}

// GetPendingArchives 获取待归档列表
// GET /api/audit/pending-archives?status=pending&page=1&page_size=20
func GetPendingArchives(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	list, total, err := service.GetPendingArchives(status, page, pageSize)
	if err != nil {
		errResponse(w, "获取待归档列表失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	okResponse(w, map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ApprovePendingArchive 审核通过（执行归档）
// POST /api/audit/pending-archives/{id}/approve
func ApprovePendingArchive(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/api/audit/pending-archives/", "/approve")
	if id <= 0 {
		errResponse(w, "无效的待归档 ID", http.StatusBadRequest)
		return
	}

	if err := service.ApprovePendingArchive(id, session.Username); err != nil {
		errResponse(w, "审核通过失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	okResponse(w, map[string]interface{}{
		"message": "已审核通过并执行归档",
		"id":      id,
		"reviewer": session.Username,
	})
}

// RejectPendingArchive 审核驳回
// POST /api/audit/pending-archives/{id}/reject
func RejectPendingArchive(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}

	id := extractIDFromPath(r.URL.Path, "/api/audit/pending-archives/", "/reject")
	if id <= 0 {
		errResponse(w, "无效的待归档 ID", http.StatusBadRequest)
		return
	}

	if err := service.RejectPendingArchive(id, session.Username); err != nil {
		errResponse(w, "审核驳回失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	okResponse(w, map[string]interface{}{
		"message": "已驳回",
		"id":      id,
		"reviewer": session.Username,
	})
}

// extractIDFromPath 从 URL 路径中提取 ID
// 示例：extractIDFromPath("/api/audit/pending-archives/123/approve", "/api/audit/pending-archives/", "/approve") -> 123
func extractIDFromPath(path, prefix, suffix string) int {
	if len(path) <= len(prefix) {
		return 0
	}
	s := path[len(prefix):]
	if len(suffix) > 0 && len(s) > len(suffix) {
		s = s[:len(s)-len(suffix)]
	}
	// 去掉尾部斜杠
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return id
}
