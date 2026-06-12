package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strconv"
)

// ── 统一响应 ──

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func okResponse(w http.ResponseWriter, data interface{}) {
	writeJSON(w, map[string]interface{}{"ok": true, "data": data})
}

func errResponse(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": msg})
}

// ── LLM 配置管理 ──

func GetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := service.GetCatalog()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, catalog)
}

func SaveVendorKeys(w http.ResponseWriter, r *http.Request) {
	vendorIDStr := r.URL.Query().Get("vendor_id")
	vendorID, err := strconv.Atoi(vendorIDStr)
	if err != nil {
		errResponse(w, "vendor_id 无效", 400)
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	keysRaw, _ := body["keys"].([]interface{})
	var keys []map[string]interface{}
	for _, k := range keysRaw {
		if m, ok := k.(map[string]interface{}); ok {
			keys = append(keys, m)
		}
	}
	if err := service.SaveVendorKeys(vendorID, keys); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "保存成功")
}

func ToggleVendor(w http.ResponseWriter, r *http.Request) {
	vendorIDStr := r.URL.Query().Get("vendor_id")
	vendorID, err := strconv.Atoi(vendorIDStr)
	if err != nil {
		errResponse(w, "vendor_id 无效", 400)
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	enabled, _ := body["enabled"].(bool)
	updated, err := service.ToggleVendor(vendorID, enabled)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"updated": updated})
}

// ── 路由策略 ──

func GetStrategies(w http.ResponseWriter, r *http.Request) {
	okResponse(w, service.GetStrategies())
}

func SaveStrategy(w http.ResponseWriter, r *http.Request) {
	var s map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	strategy := ParseStrategy(s)
	service.SaveStrategy(strategy)
	okResponse(w, strategy)
}

func DeleteStrategy(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errResponse(w, "缺少 id", 400)
		return
	}
	service.DeleteStrategy(id)
	okResponse(w, "已删除")
}

func SetActiveStrategy(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id, _ := body["id"].(string)
	if id == "" {
		errResponse(w, "缺少 id", 400)
		return
	}
	found := service.SetActiveStrategy(id)
	okResponse(w, map[string]interface{}{"found": found})
}

func GetAvailableOptions(w http.ResponseWriter, r *http.Request) {
	options, err := service.GetAvailableOptions()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, options)
}

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
	// 设置 cookie
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
	okResponse(w, users)
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
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	password, _ := body["password"].(string)
	if err := service.UpdateUserPassword(id, password); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "密码已更新")
}

func ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	status := 1
	if v, ok := body["status"].(float64); ok {
		status = int(v)
	}
	if err := service.ToggleUserStatus(id, status); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "状态已更新")
}

func ToggleUserAdmin(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	isAdmin, _ := body["is_admin"].(bool)
	if err := service.ToggleUserAdmin(id, isAdmin); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "权限已更新")
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.Atoi(idStr)
	if err := service.DeleteUser(id); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "用户已删除")
}

// ── 统计 ──

func GetStatsSummary(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days, _ := strconv.Atoi(daysStr)
	if days == 0 {
		days = 7
	}
	summary, err := service.GetStatsSummary(days)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, summary)
}

func GetRecentErrors(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 20
	}
	errors, err := service.GetRecentErrors(limit)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, errors)
}

func CleanupStats(w http.ResponseWriter, r *http.Request) {
	n := service.CleanupStats()
	okResponse(w, map[string]interface{}{"deleted": n})
}

// ── 日志 ──

func GetLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, _ := strconv.Atoi(limitStr)
	if limit == 0 {
		limit = 200
	}
	category := r.URL.Query().Get("category")
	afterIDStr := r.URL.Query().Get("after_id")
	afterID, _ := strconv.Atoi(afterIDStr)

	logs, maxID, err := service.GetLogs(limit, category, afterID)
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

// ── 上帝指令 ──

func GetGodRules(w http.ResponseWriter, r *http.Request) {
	okResponse(w, service.GetGodRules())
}

func SaveGodRules(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	enabled, _ := body["enabled"].(bool)
	rules, _ := body["rules"].(string)
	promptOptimize, _ := body["prompt_optimize"].(bool)
	service.SaveGodRules(enabled, rules, promptOptimize)
	okResponse(w, "已保存")
}

// ── 额度监控 ──

func GetQuotaStatus(w http.ResponseWriter, r *http.Request) {
	okResponse(w, service.GetQuotaStatus())
}

func SetQuotaEnabled(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	enabled, _ := body["enabled"].(bool)
	service.SetQuotaEnabled(enabled)
	okResponse(w, "已更新")
}

func ForceQuotaCheck(w http.ResponseWriter, r *http.Request) {
	service.ForceQuotaCheck()
	okResponse(w, "额度检查已触发")
}

// ── AI 建议 ──

func GetSuggestions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	date := r.URL.Query().Get("date")
	suggestions, err := service.GetSuggestions(status, date)
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

// ── 技能 ──

func GetSkillList(w http.ResponseWriter, r *http.Request) {
	skills, err := service.GetSkillList()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, skills)
}

// ── 健康检查 ──

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

// ── 当前用户信息（/api/system/me）──

func SystemMe(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	okResponse(w, map[string]interface{}{
		"user_id":   session.UserID,
		"username":  session.Username,
		"is_admin":  session.IsAdmin,
		"token":     session.Expire.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ── 辅助：ParseStrategy ──

// ParseStrategy 从 map 解析策略
func ParseStrategy(s map[string]interface{}) service.StrategyType {
	id, _ := s["id"].(string)
	name, _ := s["name"].(string)
	typeStr, _ := s["type"].(string)
	active, _ := s["active"].(bool)

	var options []service.StrategyOptionType
	if optsRaw, ok := s["options"].([]interface{}); ok {
		for _, o := range optsRaw {
			if om, ok := o.(map[string]interface{}); ok {
				opt := service.StrategyOptionType{
					VendorID:    intFloat(om["vendor_id"]),
					KeyID:       intFloatStr(om["key_id"]),
					ModelID:     strVal(om["model_id"]),
					VendorName:  strVal(om["vendor_name"]),
					BaseURL:     strVal(om["base_url"]),
					KeyName:     strVal(om["key_name"]),
					APIKey:      strVal(om["api_key"]),
					DisplayName: strVal(om["display_name"]),
				}
				options = append(options, opt)
			}
		}
	}

	return service.StrategyType{
		ID: id, Name: name, Type: typeStr,
		Active: active, Options: options,
	}
}

func intFloat(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(t)
		return n
	}
	return 0
}

func intFloatStr(v interface{}) string {
	switch t := v.(type) {
	case float64:
		return strconv.Itoa(int(t))
	case int:
		return strconv.Itoa(t)
	case string:
		return t
	}
	return ""
}

func strVal(v interface{}) string {
	s, _ := v.(string)
	return s
}
