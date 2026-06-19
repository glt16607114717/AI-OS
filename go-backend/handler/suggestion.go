package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"net/http"
	"strconv"
)

// ── AI 建议 ──

func GetSuggestions(w http.ResponseWriter, r *http.Request) {
	userID := 0
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
	}
	status := r.URL.Query().Get("status")
	date := r.URL.Query().Get("date")
	suggestions, err := service.GetSuggestions(userID, status, date)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, suggestions)
}

func MarkSuggestionProcessed(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	if err := service.MarkSuggestionProcessed(id); err != nil {
		errResponse(w, err.Error(), 400)
		return
	}
	okResponse(w, "已标记为已处理")
}

func RunAIAnalysis(w http.ResponseWriter, r *http.Request) {
	if err := service.RunAIAnalysis(); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "分析完成")
}
