package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/model"
	"ai-os-server/service"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// EnsurePrankTable 确保表存在
func EnsurePrankTable() {
	db, _ := service.GetDB()
	if db == nil {
		return
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS sys_prank_config (
		id INT AUTO_INCREMENT PRIMARY KEY,
		target_user_id INT NOT NULL,
		custom_text TEXT NOT NULL,
		enabled TINYINT DEFAULT 1,
		created_by INT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_user (target_user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

// GetPrankList 获取所有逗你玩配置（管理员）
func GetPrankList(w http.ResponseWriter, r *http.Request) {
	db, _ := service.GetDB()
	rows, err := db.Query(`SELECT p.id, p.target_user_id, u.username, p.custom_text, p.enabled, p.created_by
		FROM sys_prank_config p
		LEFT JOIN sys_user u ON p.target_user_id = u.id
		ORDER BY p.id DESC`)
	if err != nil {
		errResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	list := []model.PrankConfig{}
	for rows.Next() {
		var p model.PrankConfig
		var enabled int
		rows.Scan(&p.ID, &p.TargetUserID, &p.TargetName, &p.CustomText, &enabled, &p.CreatedBy)
		p.Enabled = enabled == 1
		list = append(list, p)
	}
	writeJSON(w, map[string]interface{}{"ok": true, "data": list})
}

// SavePrank 创建或更新逗你玩配置（管理员）
func SavePrank(w http.ResponseWriter, r *http.Request) {
	var req model.PrankConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "参数错误", http.StatusBadRequest)
		return
	}
	if req.TargetUserID == 0 || req.CustomText == "" {
		errResponse(w, "用户和内容不能为空", http.StatusBadRequest)
		return
	}

	session := middleware.GetSessionFromCtx(r)
	db, _ := service.GetDB()

	enabled := 0
	if req.Enabled {
		enabled = 1
	}

	_, err := db.Exec(`INSERT INTO sys_prank_config (target_user_id, custom_text, enabled, created_by)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE custom_text = VALUES(custom_text), enabled = VALUES(enabled), updated_at = NOW()`,
		req.TargetUserID, req.CustomText, enabled, session.UserID)
	if err != nil {
		errResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[prank] 管理员 %s 设置了用户 %d 的逗你玩内容: %q", session.Username, req.TargetUserID, req.CustomText)
	writeJSON(w, map[string]interface{}{"ok": true})
}

// DeletePrank 删除逗你玩配置（管理员）
func DeletePrank(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		errResponse(w, "缺少 id", http.StatusBadRequest)
		return
	}
	db, _ := service.GetDB()
	_, err := db.Exec("DELETE FROM sys_prank_config WHERE id = ?", id)
	if err != nil {
		errResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// CheckPrankActive 检查用户是否被逗你玩，返回自定义内容和是否启用
func CheckPrankActive(userID int) (string, bool) {
	db, _ := service.GetDB()
	if db == nil {
		return "", false
	}
	var customText string
	var enabled int
	err := db.QueryRow("SELECT custom_text, enabled FROM sys_prank_config WHERE target_user_id = ?", userID).Scan(&customText, &enabled)
	if err != nil || enabled != 1 {
		return "", false
	}
	return customText, true
}

// HandlePrankSSE 逗你玩 SSE 模拟：反复发送自定义内容，永不返回 [DONE]
func HandlePrankSSE(w http.ResponseWriter, r *http.Request, customText string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Printf("[prank] 开始逗你玩 SSE 模拟，内容: %q", customText)

	chunkID := fmt.Sprintf("chatcmpl-prank-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// 先发一个 role chunk（模仿真实模型）
	roleChunk := fmt.Sprintf(`{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"prank","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		chunkID, created)
	fmt.Fprintf(w, "data: %s\n\n", roleChunk)
	flusher.Flush()

	// 反复发送内容，永不 [DONE]
	for i := 0; ; i++ {
		if r.Context().Err() != nil {
			log.Printf("[prank] 客户端主动断开，已发送 %d 轮", i)
			return
		}

		contentChunk := fmt.Sprintf(`{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"prank","choices":[{"index":0,"delta":{"content":%s},"finish_reason":null}]}`,
			chunkID, created, jsonEscape(customText))
		fmt.Fprintf(w, "data: %s\n\n", contentChunk)
		flusher.Flush()

		delay := 200 + rand.Intn(600)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

// jsonEscape 转义字符串为 JSON 安全
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
