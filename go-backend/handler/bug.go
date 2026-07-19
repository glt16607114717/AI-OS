package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// GetMyBugs 用户查看自己的 Bug 列表
func GetMyBugs(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	list, err := service.GetMyBugs(session.UserID)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, list)
}

// GetAllBugs 查看所有 Bug（支持 status 过滤）
func GetAllBugs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, err := service.GetAllBugs(status)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, list)
}

// UpdateBugStatus 更新 Bug 状态
func UpdateBugStatus(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	status, _ := body["status"].(string)
	adminNote, _ := body["admin_note"].(string)
	if id == 0 {
		errResponse(w, "缺少 id", 400)
		return
	}
	if err := service.UpdateBugStatus(int(id), status, adminNote); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "状态已更新")
}

// WithdrawBug 撤销 Bug（改 status=withdrawn，不删除）
func WithdrawBug(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	if id == 0 {
		errResponse(w, "缺少 id", 400)
		return
	}
	if err := service.WithdrawBug(int(id), session.UserID); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "Bug 已撤销")
}
