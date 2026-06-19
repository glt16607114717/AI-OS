package handler

import (
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

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
