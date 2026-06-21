package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// VisionResult 图片识别结果
type VisionResult struct {
	Text       string // 图片的文字描述
	ImageCount int    // 识别了几张图
	Modified   bool   // 是否修改了 messages
}

var (
	// 图片 URL 正则（http/https 链接，以常见图片扩展名结尾，可带查询参数）
	imageURLRegex = regexp.MustCompile(`(?i)https?://[^\s"'<>)\\]+\.(?:png|jpg|jpeg|gif|webp|bmp)(?:\?[^\s"'<>)\\]*)?`)

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

// ProcessImages 检测并处理 messages 中的图片
// 规则：
//   - 只扫描最后一条 user message
//   - 支持 content 数组中的 image_url（本地上传）
//   - 支持 text 中的图片 URL（网络图片）
//   - 同一请求内同 URL 只识别一次
//   - 识别结果作为文字追加到 user message 末尾
func ProcessImages(messages []map[string]interface{}) ([]map[string]interface{}, *VisionResult) {
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

	// 处理两种 content 格式
	switch content := lastMsg["content"].(type) {
	case string:
		// 字符串格式：正则匹配图片 URL
		urls := imageURLRegex.FindAllString(content, -1)
		for _, url := range urls {
			if processed[url] {
				continue
			}
			processed[url] = true
			desc := recognizeImage(url, "")
			if desc != "" {
				imageCount++
				descriptions = append(descriptions, fmt.Sprintf("[图片 %d 内容：%s]", imageCount, desc))
			}
		}

	case []interface{}:
		// 数组格式（多模态）：处理 image_url 项 + 扫描 text 项里的 URL
		// 关键：无论识别成功还是失败，都要把 image_url 项从 content 里移除
		// 否则下游 chat 模型（如 deepseek）会因为不支持图片输入而报 400
		newContent := make([]interface{}, 0, len(content))
		for _, item := range content {
			m, ok := item.(map[string]interface{})
			if !ok {
				newContent = append(newContent, item)
				continue
			}
			t, _ := m["type"].(string)

			if t == "image_url" {
				// image_url 项：尝试识别，但不保留原项（避免下游报错）
				imgURL, _ := m["image_url"].(map[string]interface{})
				if imgURL == nil {
					continue // 无效的 image_url，直接丢弃
				}
				url, _ := imgURL["url"].(string)
				if url == "" || processed[url] {
					continue // 空 URL 或已处理，丢弃
				}
				processed[url] = true
				desc := recognizeImage(url, "")
				if desc != "" {
					imageCount++
					descriptions = append(descriptions, fmt.Sprintf("[图片 %d 内容：%s]", imageCount, desc))
				} else {
					// 识别失败：追加占位提示，不丢弃图片这个事实
					imageCount++
					descriptions = append(descriptions, fmt.Sprintf("[图片 %d：识别失败]", imageCount))
				}
				// 不把原 item 加回 newContent → 等效删除 image_url 项
				continue
			}

			if t == "text" {
				// text 项：扫描里面的图片 URL，但保留原 text（URL 作为文字是无害的）
				text, _ := m["text"].(string)
				urls := imageURLRegex.FindAllString(text, -1)
				for _, url := range urls {
					if processed[url] {
						continue
					}
					processed[url] = true
					desc := recognizeImage(url, "")
					if desc != "" {
						imageCount++
						descriptions = append(descriptions, fmt.Sprintf("[图片 %d 内容：%s]", imageCount, desc))
					}
				}
			}

			newContent = append(newContent, item)
		}
		// 替换为清理后的 content（image_url 已移除）
		lastMsg["content"] = newContent
	}

	if imageCount == 0 {
		return messages, &VisionResult{ImageCount: 0, Modified: false}
	}

	// 把图片描述追加到最后一条 user message
	appendText := "\n\n" + strings.Join(descriptions, "\n")
	switch content := lastMsg["content"].(type) {
	case string:
		lastMsg["content"] = content + appendText
	case []interface{}:
		// 数组格式：追加一个 text 项
		lastMsg["content"] = append(content, map[string]interface{}{
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

// recognizeImage 调用智谱 GLM-4V-Plus 识别图片
// imageURL 可以是 http(s):// 链接或 data:image/...;base64,... 格式
func recognizeImage(imageURL, prompt string) string {
	apiKey := getZhipuAPIKey()
	if apiKey == "" {
		log.Println("[vision] 未找到智谱 API Key，跳过图片识别")
		return ""
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
		log.Printf("[vision] 请求序列化失败: %v", err)
		return ""
	}

	req, err := http.NewRequest("POST", visionAPIURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[vision] 调用智谱视觉模型失败: %v", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	if resp.StatusCode != 200 {
		previewLen := 200
		if len(body) < previewLen {
			previewLen = len(body)
		}
		log.Printf("[vision] 智谱视觉模型返回 %d: %s", resp.StatusCode, string(body[:previewLen]))
		return ""
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}
	if len(result.Choices) == 0 {
		return ""
	}

	desc := strings.TrimSpace(result.Choices[0].Message.Content)
	return desc
}
