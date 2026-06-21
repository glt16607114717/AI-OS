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
	suggestions, err := service.GetSuggestions(userID, status, date)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, suggestions)
}

// MarkSuggestionProcessed 标记建议已处理
func MarkSuggestionProcessed(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		var body struct {
			SuggestionID int `json:"suggestion_id"`
			ID           int `json:"id"`
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil {
			if body.SuggestionID > 0 {
				idStr = strconv.Itoa(body.SuggestionID)
			} else if body.ID > 0 {
				idStr = strconv.Itoa(body.ID)
			}
		}
	}
	id, _ := strconv.Atoi(idStr)
	if id <= 0 {
		errResponse(w, "缺少建议 id", 400)
		return
	}
	if err := service.MarkSuggestionProcessed(id); err != nil {
		errResponse(w, err.Error(), 400)
		return
	}
	okResponse(w, "已标记为已处理")
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
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
	}
	today := time.Now().Format("2006-01-02")
	go func() {
		if err := service.RunDistillForDate(userID, today); err != nil {
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
