package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/model"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// ── 上帝指令 ──

func GetGodRules(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	okResponse(w, service.GetGodRules(session.UserID))
}

func SaveGodRules(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	cfg := &model.GodRulesConfig{
		UserID:             session.UserID,
		Enabled:            getBool(body, "enabled"),
		Rules:              getStr(body, "rules"),
		PromptOptimize:     getBool(body, "prompt_optimize"),
		StripNoise:         getBool(body, "strip_noise"),
		CompressFile:       getBool(body, "compress_file"),
		SimplifyLang:       getBool(body, "simplify_lang"),
		CompressToolResult: getBool(body, "compress_tool_result"),
		CompressTools:      getBool(body, "compress_tools"),
	}
	service.SaveGodRulesFull(cfg)
	okResponse(w, "已保存")
}

func getBool(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func getStr(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
