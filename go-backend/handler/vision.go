package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strings"
)

// VisionRecognize 视觉识别专用接口（免登录，API Key 认证）
// 认证方式：?key=sk-xxx（通过 API Key 反查用户）
// 请求体：{"image": "data:image/png;base64,...", "prompt": "可选提示词"}
// 响应体：{"ok":true,"data":{"description":"识别结果","model":"glm-4.6v"}}
//
// 业务意图：img-ocr 技能调用此接口识别图片，不经过 chat 管线，
// 避免污染聊天历史和 LLM 统计。
func VisionRecognize(w http.ResponseWriter, r *http.Request) {
	// ── 认证：支持 ?key=sk-xxx 和 session 两种方式 ──
	var userID int
	var username string

	// 方式1: ?key= API Key 反查用户
	if apiKey := r.URL.Query().Get("key"); apiKey != "" && strings.HasPrefix(apiKey, "sk-") {
		uid, uname, _, err := middleware.APIKeyLookup(apiKey)
		if err == nil && uid > 0 {
			userID = uid
			username = uname
		}
	}

	// 方式2: session（Authorization header / cookie）
	if userID == 0 {
		session := middleware.GetSessionFromCtx(r)
		if session != nil {
			userID, username = sessionUser(session)
		}
	}

	if userID == 0 {
		errResponse(w, "未认证（需要有效的 ?key= 或登录态）", 401)
		return
	}

	// ── 解析请求 ──
	var req struct {
		Image  string `json:"image"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Image == "" {
		errResponse(w, "缺少 image 参数", 400)
		return
	}

	imageSize := len(req.Image)

	// ── 调用智谱视觉模型 ──
	desc, err := service.RecognizeImage(userID, username, req.Image, req.Prompt)

	if err != nil || desc == "" {
		errMsg := "识别失败"
		if err != nil {
			errMsg = err.Error()
		}
		service.LogVisionRecognize(userID, username, req.Image, imageSize, req.Prompt, "", "glm-4.6v", false, errMsg)
		errResponse(w, "图片识别失败: "+errMsg, 500)
		return
	}

	// ── 记录日志 ──
	service.LogVisionRecognize(userID, username, req.Image, imageSize, req.Prompt, desc, "glm-4.6v", true, "")

	// ── 返回 ──
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"description": desc,
			"model":       "glm-4.6v",
		},
	})
}
