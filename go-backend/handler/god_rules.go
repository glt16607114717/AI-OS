package handler

import (
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

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
