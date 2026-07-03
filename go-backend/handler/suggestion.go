package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ── AI 建议 ──

// GetSuggestions 获取建议列表
func GetSuggestions(w http.ResponseWriter, r *http.Request) {
	userID := 0
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
	}
	status := r.URL.Query().Get("status")
	date := r.URL.Query().Get("date")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	suggestions, total, err := service.GetSuggestions(userID, status, date, page, pageSize)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	okResponse(w, map[string]interface{}{
		"data":     suggestions,
		"total":    total,
		"page":     page,
		"page_size": pageSize,
	})
}

// MarkSuggestionProcessed 标记建议状态（processed=已处理 / ignored=已忽略）
func MarkSuggestionProcessed(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")

	if idStr == "" || status == "" {
		var body struct {
			SuggestionID int    `json:"suggestion_id"`
			ID           int    `json:"id"`
			Status       string `json:"status"`
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil {
			if idStr == "" {
				if body.SuggestionID > 0 {
					idStr = strconv.Itoa(body.SuggestionID)
				} else if body.ID > 0 {
					idStr = strconv.Itoa(body.ID)
				}
			}
			if status == "" {
				status = body.Status
			}
		}
	}

	id, _ := strconv.Atoi(idStr)
	if id <= 0 {
		errResponse(w, "缺少建议 id", 400)
		return
	}
	if status == "" {
		status = "processed"
	}

	if err := service.UpdateSuggestionStatus(id, status); err != nil {
		errResponse(w, err.Error(), 400)
		return
	}

	if status == "ignored" {
		okResponse(w, "已忽略")
	} else {
		okResponse(w, "已标记为已处理")
	}
}

// GetDistillKnowledge 获取蒸馏知识
func GetDistillKnowledge(w http.ResponseWriter, r *http.Request) {
	userID := 0
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
	}
	date := r.URL.Query().Get("date")
	knowledge, err := service.DistillKnowledgeByDate(userID, date)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, knowledge)
}

// TriggerDistill 手动触发蒸馏（分析当天对话）
func TriggerDistill(w http.ResponseWriter, r *http.Request) {
	userID := 0
	username := ""
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		username = session.Username
	}
	today := time.Now().Format("2006-01-02")
	go func() {
		if err := service.RunDistillForDateWithTrigger(userID, username, today, "manual"); err != nil {
			log.Printf("[distill] 手动触发失败 user=%d date=%s: %v", userID, today, err)
		} else {
			log.Printf("[distill] 手动触发成功 user=%d date=%s", userID, today)
		}
	}()
	okResponse(w, map[string]interface{}{
		"message": "蒸馏任务已触发，将在后台执行",
		"date":    today,
	})
}

// GetDistillLogs 获取蒸馏日志
func GetDistillLogs(w http.ResponseWriter, r *http.Request) {
	userID := 0
	isAdmin := false
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}
	date := r.URL.Query().Get("date")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	logs, err := service.GetDistillLogs(userID, isAdmin, date, limit)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, logs)
}
