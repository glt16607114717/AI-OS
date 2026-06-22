package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// VisionResult 图片识别结果
type VisionResult struct {
	Text       string // 图片的文字描述
	ImageCount int    // 识别了几张图
	Modified   bool   // 是否修改了 messages
}

var (
	// 智谱视觉模型（GLM-4.6V 视觉推理专用套餐）
	visionModelID = "glm-4.6v"
	visionAPIURL  = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
)

// getZhipuAPIKey 获取智谱 API Key（复用系统已配置的）
func getZhipuAPIKey() string {
	conn, err := GetDB()
	if err != nil || conn == nil {
		return ""
	}

	var apiKey string
	err = conn.QueryRow(`SELECT k.api_key FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey)
	if err != nil || apiKey == "" {
		return ""
	}
	return apiKey
}

// StripImageContent 从所有 messages 中移除 image_url 项
// 用于防止图片内容透传到不支持多模态的第三方模型（火山方舟、智谱等）
// ProcessImages 只处理最后一条 user message，此函数兜底处理所有消息
func StripImageContent(messages []map[string]interface{}) {
	for i := range messages {
		content, ok := messages[i]["content"].([]interface{})
		if !ok {
			continue
		}
		newContent := make([]interface{}, 0, len(content))
		for _, item := range content {
			m, ok := item.(map[string]interface{})
			if !ok {
				newContent = append(newContent, item)
				continue
			}
			if t, _ := m["type"].(string); t == "image_url" {
				continue // 丢弃 image_url 项
			}
			newContent = append(newContent, item)
		}
		// 只有 content 为空数组时才降级为字符串，避免空数组导致请求异常
		if len(newContent) == 0 {
			messages[i]["content"] = ""
		} else {
			messages[i]["content"] = newContent
		}
	}
}

// ProcessImages 处理 messages 中上传的图片（image_url）
// 规则：
//   - 只扫描最后一条 user message
//   - 只处理 content 数组中的 image_url（正经上传的图片）
//   - 不扫描文本中的图片 URL（已改由前端接口处理）
//   - 识别结果作为文字追加到 user message 末尾
func ProcessImages(messages []map[string]interface{}, userID int, username string) ([]map[string]interface{}, *VisionResult) {
	if len(messages) == 0 {
		return messages, &VisionResult{}
	}

	// 找到最后一条 user message
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return messages, &VisionResult{}
	}

	lastMsg := messages[lastUserIdx]
	processed := map[string]bool{} // 已处理的 URL/base64（去重）
	var descriptions []string      // 图片描述列表
	imageCount := 0

	// 只处理数组格式的 content（多模态上传）
	content, ok := lastMsg["content"].([]interface{})
	if !ok {
		return messages, &VisionResult{}
	}

	// 遍历 content，处理 image_url 项，其他项原样保留
	newContent := make([]interface{}, 0, len(content))
	for _, item := range content {
		m, ok := item.(map[string]interface{})
		if !ok {
			newContent = append(newContent, item)
			continue
		}
		t, _ := m["type"].(string)

		if t == "image_url" {
			// image_url 项：尝试识别，但不保留原项（避免下游纯文本模型报错）
			imgURL, _ := m["image_url"].(map[string]interface{})
			if imgURL == nil {
				continue // 无效的 image_url，直接丢弃
			}
			url, _ := imgURL["url"].(string)
			if url == "" || processed[url] {
				continue // 空 URL 或已处理，丢弃
			}
			processed[url] = true
			desc, err := RecognizeImage(userID, username, url, "")
			imageSize := len(url)
			if err != nil {
				LogVisionRecognize(userID, username, url, imageSize, "", "", visionModelID, false, err.Error())
				imageCount++
				descriptions = append(descriptions, fmt.Sprintf("[图片 %d：识别失败]", imageCount))
			} else if desc != "" {
				LogVisionRecognize(userID, username, url, imageSize, "", desc, visionModelID, true, "")
				imageCount++
				descriptions = append(descriptions, fmt.Sprintf("[图片 %d 内容：%s]", imageCount, desc))
			} else {
				LogVisionRecognize(userID, username, url, imageSize, "", "", visionModelID, false, "返回空结果")
				imageCount++
				descriptions = append(descriptions, fmt.Sprintf("[图片 %d：识别失败]", imageCount))
			}
			// 不把原 item 加回 newContent → 等效删除 image_url 项
			continue
		}

		newContent = append(newContent, item)
	}
	// 替换为清理后的 content（image_url 已移除）
	lastMsg["content"] = newContent

	if imageCount == 0 {
		return messages, &VisionResult{ImageCount: 0, Modified: false}
	}

	// 把图片描述追加到最后一条 user message
	appendText := "\n\n" + strings.Join(descriptions, "\n")
	switch c := lastMsg["content"].(type) {
	case string:
		lastMsg["content"] = c + appendText
	case []interface{}:
		lastMsg["content"] = append(c, map[string]interface{}{
			"type": "text",
			"text": appendText,
		})
	}
	messages[lastUserIdx] = lastMsg

	log.Printf("[vision] 识别图片 %d 张", imageCount)
	return messages, &VisionResult{
		ImageCount: imageCount,
		Modified:   true,
	}
}

// RecognizeImage 调用智谱 GLM-4V-Plus 识别图片（导出供 handler 调用）
// imageURL 可以是 http(s):// 链接或 data:image/...;base64,... 格式
// RecognizeImage 调用智谱视觉模型识别图片，并记录日志
func RecognizeImage(userID int, username, imageURL, prompt string) (string, error) {
	apiKey := getZhipuAPIKey()
	if apiKey == "" {
		log.Println("[vision] 未找到智谱 API Key，跳过图片识别")
		return "", fmt.Errorf("未找到智谱 API Key")
	}

	if prompt == "" {
		prompt = "请详细描述这张图片的内容。如果是报错信息、代码截图、UI 界面、日志等，请准确提取其中的文字内容。"
	}

	reqBody := map[string]interface{}{
		"model": visionModelID,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{"url": imageURL}},
				},
			},
		},
		"temperature": 0.1,
		"max_tokens":  1000,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		errMsg := fmt.Sprintf("请求序列化失败: %v", err)
		log.Printf("[vision] %s", errMsg)
		return "", errors.New(errMsg)
	}

	req, err := http.NewRequest("POST", visionAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := SharedHTTPClientLong.Do(req)
	if err != nil {
		errMsg := fmt.Sprintf("调用智谱视觉模型失败: %v", err)
		log.Printf("[vision] %s", errMsg)
		return "", errors.New(errMsg)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		previewLen := 200
		if len(body) < previewLen {
			previewLen = len(body)
		}
		errMsg := fmt.Sprintf("智谱视觉模型返回 %d: %s", resp.StatusCode, string(body[:previewLen]))
		log.Printf("[vision] %s", errMsg)
		return "", errors.New(errMsg)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("返回无结果")
	}

	desc := strings.TrimSpace(result.Choices[0].Message.Content)
	return desc, nil
}

// LogVisionRecognize 写入视觉识别日志
func LogVisionRecognize(userID int, username, imageURL string, imageSize int, prompt, result, model string, success bool, errMsg string) {
	conn, err := GetDB()
	if err != nil {
		return
	}
	// 截断 imageURL（base64 可能很长）
	truncatedURL := imageURL
	if len(truncatedURL) > 200 {
		truncatedURL = truncatedURL[:200]
	}
	successInt := 0
	if success {
		successInt = 1
	}
	conn.Exec(
		"INSERT INTO sys_vision_log (user_id, username, image_url, image_size, prompt, result, model, success, error_msg) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		userID, username, truncatedURL, imageSize, prompt, result, model, successInt, errMsg,
	)
}
