package handler

import (
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// ── 用户管理 ──

func Login(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	username, _ := body["username"].(string)
	password, _ := body["password"].(string)
	if username == "" || password == "" {
		errResponse(w, "用户名和密码不能为空", 400)
		return
	}
	result, err := service.LoginUser(username, password)
	if err != nil {
		errResponse(w, err.Error(), 401)
		return
	}
	token, _ := result["token"].(string)
	http.SetCookie(w, &http.Cookie{
		Name: "token", Value: token, Path: "/", MaxAge: 30 * 24 * 3600,
		HttpOnly: false, SameSite: http.SameSiteLaxMode,
	})
	okResponse(w, result)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "token", Value: "", Path: "/", MaxAge: -1})
	okResponse(w, "已退出")
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := service.ListUsers()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	if users == nil {
		users = []map[string]interface{}{}
	}
	okResponse(w, map[string]interface{}{"users": users})
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	username, _ := body["username"].(string)
	password, _ := body["password"].(string)
	isAdmin, _ := body["is_admin"].(bool)
	if username == "" || password == "" {
		errResponse(w, "用户名和密码不能为空", 400)
		return
	}
	result, err := service.CreateUser(username, password, isAdmin)
	if err != nil {
		errResponse(w, err.Error(), 400)
		return
	}
	okResponse(w, result)
}

func UpdateUserPassword(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	password, _ := body["password"].(string)
	if err := service.UpdateUserPassword(id, password); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "密码已更新")
}

func ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	status := int(intFloat(body["status"]))
	if status != 1 && status != 2 {
		errResponse(w, "status 参数无效（1=启用, 2=停用）", 400)
		return
	}
	if err := service.ToggleUserStatus(id, status); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "状态已更新")
}

func ToggleUserAdmin(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	isAdmin, _ := body["is_admin"].(bool)
	if err := service.ToggleUserAdmin(id, isAdmin); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "权限已更新")
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id := intFloat(body["id"])
	if err := service.DeleteUser(id); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "用户已删除")
}
