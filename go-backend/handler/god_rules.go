package handler

import (
	"ai-os-server/middleware"
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
	enabled, _ := body["enabled"].(bool)
	rules, _ := body["rules"].(string)
	promptOptimize, _ := body["prompt_optimize"].(bool)
	stripNoise, _ := body["strip_noise"].(bool)
	compressFile, _ := body["compress_file"].(bool)
	simplifyLang, _ := body["simplify_lang"].(bool)
	compressToolResult, _ := body["compress_tool_result"].(bool)
	compressTools, _ := body["compress_tools"].(bool)
	service.SaveGodRules(session.UserID, enabled, rules, promptOptimize, stripNoise, compressFile, simplifyLang, compressToolResult, compressTools)
	okResponse(w, "已保存")
}
