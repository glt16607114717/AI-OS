package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strconv"
)

// ── 语音指令 CRUD ──

// VoiceStatus 语音助手状态接口
func VoiceGetStatus(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	commands, _ := service.GetVoiceCommands(session.UserID)
	enabled := service.GetVoiceEnabled(session.UserID)

	okResponse(w, map[string]interface{}{
		"ok":        true,
		"enabled":   enabled,
		"listening": false,   // 本地 Python 才有
		"model_ready": false,  // 本地 Python 才有
		"commands":  commands,
	})
}

// VoiceAddCommand 添加语音指令
func VoiceAddCommand(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	phrase, _ := body["phrase"].(string)

	cmd, err := service.AddVoiceCommand(session.UserID, phrase)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, cmd)
}

// VoiceUpdateCommand 更新语音指令
func VoiceUpdateCommand(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	cmdIDStr := r.URL.Query().Get("id")
	if cmdIDStr == "" {
		// 也支持 body 中的 id
		if id, ok := body["id"]; ok {
			cmdIDStr = strconv.Itoa(intFloat(id))
		}
	}
	cmdID, _ := strconv.Atoi(cmdIDStr)
	if cmdID == 0 {
		errResponse(w, "缺少指令 id", 400)
		return
	}

	if err := service.UpdateVoiceCommand(session.UserID, cmdID, body); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "更新成功")
}

// VoiceDeleteCommand 删除语音指令
func VoiceDeleteCommand(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	cmdIDStr := r.URL.Query().Get("id")
	if cmdIDStr == "" {
		if id, ok := body["id"]; ok {
			cmdIDStr = strconv.Itoa(intFloat(id))
		}
	}
	cmdID, _ := strconv.Atoi(cmdIDStr)
	if cmdID == 0 {
		errResponse(w, "缺少指令 id", 400)
		return
	}

	if err := service.DeleteVoiceCommand(session.UserID, cmdID); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "删除成功")
}

// VoiceSetEnabled 设置语音启用状态
func VoiceSetEnabled(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	enabled, _ := body["enabled"].(bool)

	if err := service.SetVoiceEnabled(session.UserID, enabled); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "设置成功")
}
