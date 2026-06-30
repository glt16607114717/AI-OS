package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"net/http"
	"strconv"
)

// ── 日志查看 ──

func GetLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 200
	}
	category := r.URL.Query().Get("category")
	afterIDStr := r.URL.Query().Get("after_id")
	afterID, _ := strconv.Atoi(afterIDStr)

	session := middleware.GetSessionFromCtx(r)
	userID := 0
	isAdmin := false
	if session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}

	logs, maxID, err := service.GetLogs(limit, category, afterID, userID, isAdmin)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"logs": logs, "max_id": maxID})
}

func ClearLogs(w http.ResponseWriter, r *http.Request) {
	service.ClearLogs()
	okResponse(w, "日志已清空")
}

// GetChatSessions 按消息维度聚合的对话记录
func GetChatSessions(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 100
	}

	session := middleware.GetSessionFromCtx(r)
	userID := 0
	isAdmin := false
	if session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}

	sessions, err := service.GetChatSessions(limit, userID, isAdmin)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"sessions": sessions})
}
