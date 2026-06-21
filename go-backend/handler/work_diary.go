package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// ── 工作日报 ──

// GetWorkDiaries 获取日报列表
func GetWorkDiaries(w http.ResponseWriter, r *http.Request) {
	userID := 0
	isAdmin := false
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}
	date := r.URL.Query().Get("date")

	// 非管理员只能看自己的
	uid := userID
	if !isAdmin {
		uid = userID
	}

	list, err := service.GetWorkDiaries(uid, date, 100)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	okResponse(w, list)
}

// GetWorkDiary 获取单条日报
func GetWorkDiary(w http.ResponseWriter, r *http.Request) {
	userID := 0
	isAdmin := false
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		errResponse(w, "缺少日报 id", 400)
		return
	}

	diary, err := service.GetWorkDiary(id, userID, isAdmin)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	if diary == nil {
		errResponse(w, "日报不存在或无权限", 404)
		return
	}
	okResponse(w, diary)
}

// SaveWorkDiary 创建或更新日报
func SaveWorkDiary(w http.ResponseWriter, r *http.Request) {
	userID := 0
	username := ""
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		username = session.Username
	}
	if userID <= 0 {
		errResponse(w, "未登录", 401)
		return
	}

	var body struct {
		ID      int    `json:"id"`
		Date    string `json:"date"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		bodyRaw, _ := io.ReadAll(r.Body)
		json.Unmarshal(bodyRaw, &body)
	}
	if body.Date == "" {
		errResponse(w, "日期不能为空", 400)
		return
	}
	if body.Title == "" {
		body.Title = "工作日报"
	}
	if body.Content == "" {
		body.Content = "(无内容)"
	}

	id, err := service.SaveWorkDiary(body.ID, userID, username, body.Date, body.Title, body.Content)
	if err != nil {
		errResponse(w, err.Error(), 400)
		return
	}
	okResponse(w, map[string]interface{}{"id": id})
}

// DeleteWorkDiary 删除日报
func DeleteWorkDiary(w http.ResponseWriter, r *http.Request) {
	userID := 0
	isAdmin := false
	if session := middleware.GetSessionFromCtx(r); session != nil {
		userID = session.UserID
		isAdmin = session.IsAdmin
	}
	if userID <= 0 {
		errResponse(w, "未登录", 401)
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		errResponse(w, "缺少日报 id", 400)
		return
	}
	if err := service.DeleteWorkDiary(id, userID, isAdmin); err != nil {
		errResponse(w, err.Error(), 400)
		return
	}
	okResponse(w, "已删除")
}
