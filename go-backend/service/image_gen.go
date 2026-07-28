package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ── 图片生成（GLM-Image）──
//
// 调用智谱 GLM-Image 文生图模型，返回图片 URL。
// 作为服务端技能 skill_generate_image 被 agentLoop 调用：
//   handler/agent.go -> ExecuteBuiltinSkill("generate_image") -> ExecuteGenerateImage
//
// HTTP 调用模式严格照搬 service/vision.go:RecognizeImage：
//   - 复用 getZhipuAPIKey() 取 key
//   - 用 SharedHTTPClientLong（无首字节超时，靠 context 兜底）
//     SharedHTTPClient 的 ResponseHeaderTimeout=30s 会杀掉图片生成这种慢请求
//   - 超时 60 秒（图片生成本就慢，用户侧定的上限，超了让用户重试）

const (
	glmImageModel    = "glm-image"
	glmImageAPIURL   = "https://open.bigmodel.cn/api/paas/v4/images/generations"
	glmImageTimeout  = 60 * time.Second // 用户侧上限，可调整
	glmImageDefaultSize = "1024x1024"
)

// glmImageResponse 智谱生图 API 返回结构
// 实测返回: {"created":..., "data":[{"url":"https://..."}], "id":"...", "request_id":"..."}
type glmImageResponse struct {
	Created   int64 `json:"created"`
	Data      []struct {
		URL string `json:"url"`
	} `json:"data"`
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
}

// ExecuteGenerateImage 调用 GLM-Image 生成图片
// 返回 map 会作为 tool 消息 content 喂给大模型，大模型据此输出 markdown 图片
func ExecuteGenerateImage(userID int, username string, args map[string]interface{}) (map[string]interface{}, error) {
	// 1. 取 key
	apiKey := getZhipuAPIKey()
	if apiKey == "" {
		log.Println("[image_gen] 未找到智谱 API Key")
		LogGenerateImage(userID, username, "", "", "", "", 0, false, "未找到智谱 API Key")
		return nil, fmt.Errorf("未找到智谱 API Key")
	}

	// 2. 解析参数
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("缺少 prompt 参数")
	}
	size, _ := args["size"].(string)
	if size == "" {
		size = glmImageDefaultSize
	}

	// 3. 构造请求（OpenAI images 协议）
	reqBody := map[string]interface{}{
		"model":  glmImageModel,
		"prompt": prompt,
		"size":   size,
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %v", err)
	}

	req, err := http.NewRequest("POST", glmImageAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// 4. 超时控制（照搬 vision.go:359-361）
	ctx, cancel := context.WithTimeout(context.Background(), glmImageTimeout)
	defer cancel()
	req = req.WithContext(ctx)

	// 5. 发请求（必须用 Long，普通 client 30s 首字节超时会杀掉）
	startTime := time.Now()
	resp, err := SharedHTTPClientLong.Do(req)
	latency := time.Since(startTime).Milliseconds()
	if err != nil {
		errMsg := fmt.Sprintf("调用智谱 GLM-Image 失败: %v", err)
		log.Printf("[image_gen] %s (latency=%dms)", errMsg, latency)
		LogGenerateImage(userID, username, prompt, size, "", glmImageModel, int(latency), false, errMsg)
		return nil, errors.New(errMsg)
	}
	defer resp.Body.Close()

	// 6. 读响应
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	// 7. 检查 HTTP 状态
	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("GLM-Image 返回 HTTP %d: %s", resp.StatusCode, string(rawResp))
		log.Printf("[image_gen] %s (latency=%dms)", errMsg, latency)
		LogGenerateImage(userID, username, prompt, size, "", glmImageModel, int(latency), false, errMsg)
		return nil, errors.New(errMsg)
	}

	// 8. 解析 JSON
	var imgResp glmImageResponse
	if err := json.Unmarshal(rawResp, &imgResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v (raw: %s)", err, string(rawResp))
	}

	if len(imgResp.Data) == 0 || imgResp.Data[0].URL == "" {
		return nil, fmt.Errorf("GLM-Image 未返回图片 URL (raw: %s)", string(rawResp))
	}

	imageURL := imgResp.Data[0].URL
	log.Printf("[image_gen] ok user=%d latency=%dms size=%s url_len=%d",
		userID, latency, size, len(imageURL))

	// 9. 返回结构（三层保障让大模型稳定输出 markdown 图片）
	//   - url:       结构化字段，模型可解析
	//   - markdown:  现成的 markdown 图片语法，模型直接复用最稳
	//   - trace:     给前端进度展示用（agentLoop 会通过 event:progress 推送）
	LogGenerateImage(userID, username, prompt, size, imageURL, glmImageModel, int(latency), true, "")
	return map[string]interface{}{
		"url":      imageURL,
		"markdown": fmt.Sprintf("![生成的图片](%s)", imageURL),
		"size":     size,
		"trace":    fmt.Sprintf("🎨 已生成图片（耗时 %.1f 秒）", float64(latency)/1000.0),
	}, nil
}

// ── 统计日志（模仿 vision.go:LogVisionRecognize）──

// EnsureImageGenLogTable 确保图片生成日志表存在（启动时调用）
func EnsureImageGenLogTable() {
	conn, err := GetDB()
	if err != nil || conn == nil {
		log.Println("[image_gen] EnsureImageGenLogTable: 数据库连接失败")
		return
	}
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS sys_image_gen_log (
		id INT AUTO_INCREMENT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_id INT NOT NULL DEFAULT 0,
		username VARCHAR(100) NOT NULL DEFAULT '',
		prompt TEXT,
		size VARCHAR(20) NOT NULL DEFAULT '',
		image_url TEXT,
		model VARCHAR(50) NOT NULL DEFAULT '',
		latency_ms INT NOT NULL DEFAULT 0,
		success TINYINT NOT NULL DEFAULT 0,
		error_msg TEXT,
		INDEX idx_created (created_at),
		INDEX idx_user (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		log.Printf("[image_gen] EnsureImageGenLogTable 建表失败: %v", err)
	}
}

// LogGenerateImage 记录图片生成日志（成功和失败都记录，供统计仪表盘使用）
func LogGenerateImage(userID int, username, prompt, size, imageURL, model string, latencyMs int, success bool, errMsg string) {
	conn, err := GetDB()
	if err != nil || conn == nil {
		log.Printf("[image_gen] LogGenerateImage: 数据库连接失败")
		return
	}
	// 截断 imageURL（智谱 URL 可能带长签名参数）
	truncatedURL := imageURL
	if len(truncatedURL) > 500 {
		truncatedURL = imageURL[:500]
	}
	// 截断 prompt（避免超长提示词撑爆 TEXT 字段）
	truncatedPrompt := prompt
	if len(truncatedPrompt) > 500 {
		truncatedPrompt = prompt[:500]
	}
	successInt := 0
	if success {
		successInt = 1
	}
	_, err = conn.Exec(
		"INSERT INTO sys_image_gen_log (user_id, username, prompt, size, image_url, model, latency_ms, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		userID, username, truncatedPrompt, size, truncatedURL, model, latencyMs, successInt, errMsg,
	)
	if err != nil {
		log.Printf("[image_gen] LogGenerateImage 写入失败: %v", err)
	}
}
