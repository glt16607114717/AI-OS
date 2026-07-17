package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// GetMyRequirements 用户查看自己的需求列表
func GetMyRequirements(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	list, err := service.GetMyRequirements(session.UserID)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, list)
}

// GetAllRequirementsAdmin 管理员查看所有需求
func GetAllRequirementsAdmin(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, err := service.GetAllRequirements(status)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, list)
}

// UpdateRequirementStatus 管理员更新需求状态
func UpdateRequirementStatus(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	status, _ := body["status"].(string)
	adminNote, _ := body["admin_note"].(string)
	if id == 0 {
		errResponse(w, "缺少 id", 400)
		return
	}
	if err := service.UpdateRequirementStatus(int(id), status, adminNote); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "状态已更新")
}