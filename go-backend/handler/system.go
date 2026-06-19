package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"net/http"
)

// ── 系统接口（健康检查/当前用户） ──

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	user := ""
	isAdmin := false
	if session != nil {
		user = session.Username
		isAdmin = session.IsAdmin
	}
	okResponse(w, map[string]interface{}{
		"status":   "online",
		"version":  "2.0.0-go",
		"user":     user,
		"is_admin": isAdmin,
	})
}

func SystemMe(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	apiKey, _ := service.GetUserAPIKey(session.UserID)
	okResponse(w, map[string]interface{}{
		"user_id":  session.UserID,
		"username": session.Username,
		"is_admin": session.IsAdmin,
		"token":    session.Expire.Format("2006-01-02T15:04:05Z07:00"),
		"api_key":  apiKey,
	})
}
