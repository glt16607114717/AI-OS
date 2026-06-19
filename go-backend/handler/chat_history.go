package handler

import (
	"ai-os-server/service"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ── 聊天历史 ──

func GetChatHistory(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 200
	}
	history, err := service.GetChatHistory(limit)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"messages": history, "max_id": service.GetChatMaxID()})
}

func ClearChatHistory(w http.ResponseWriter, r *http.Request) {
	service.ClearChatHistory()
	okResponse(w, "聊天记录已清空")
}

func DownloadConversationLog(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		errResponse(w, "无效的对话ID", 400)
		return
	}
	filePath := service.GetConversationLogPath(id)
	if filePath == "" {
		errResponse(w, "日志文件不存在或已过期（保留7天）", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=conversation_%d.json", id))
	http.ServeFile(w, r, filePath)
}
