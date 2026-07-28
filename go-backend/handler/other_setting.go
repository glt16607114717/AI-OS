package handler

import (
	"ai-os-server/model"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// GetOtherSettingHandler 获取其他设置（管理员）
func GetOtherSettingHandler(w http.ResponseWriter, r *http.Request) {
	cfg := service.GetOtherSetting()
	okResponse(w, cfg)
}

// SaveOtherSettingHandler 保存其他设置（管理员）
func SaveOtherSettingHandler(w http.ResponseWriter, r *http.Request) {
	var req model.OtherSetting
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "参数错误", http.StatusBadRequest)
		return
	}
	if req.MessageRounds < 1 {
		errResponse(w, "消息对话轮数最小为 1", http.StatusBadRequest)
		return
	}

	service.SaveOtherSetting(&req)
	okResponse(w, nil)
}
