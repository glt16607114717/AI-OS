package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"ai-os-server/middleware"
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

// ── 全局图片识别缓存（Redis，24 小时有效）──
// Trae 编辑器每次请求都带完整对话历史（含 base64 图片），导致同一张图被重复识别数十次
// 用图片内容 hash 做 key，24 小时内只识别一次

const imageCacheTTL = 24 * time.Hour

// imageCacheKey 根据图片 URL 生成缓存 key
// base64 data URL：取前 1024 字符做 md5（足够区分不同图片）
// http URL：直接用 URL 本身做 key
func imageCacheKey(url string) string {
	if strings.HasPrefix(url, "data:image/") {
		prefix := url
		if len(prefix) > 1024 {
			prefix = prefix[:1024]
		}
		h := md5.Sum([]byte(prefix))
		return "img:" + hex.EncodeToString(h[:])
	}
	return "url:" + url
}

// getRedisClient 获取 Redis 客户端，连接失败返回 nil
func getRedisClient() *redis.Client {
	return middleware.GetRedis()
}

// getCachedImage 从 Redis 查询缓存，未命中返回 ("", false)
func getCachedImage(key string) (string, bool) {
	rdb := getRedisClient()
	if rdb == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	desc, err := rdb.Get(ctx, "vision_cache:"+key).Result()
	if err != nil {
		return "", false
	}
	return desc, true
}

// setCachedImage 写入 Redis 缓存
func setCachedImage(key, desc string) {
	rdb := getRedisClient()
	if rdb == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rdb.Set(ctx, "vision_cache:"+key, desc, imageCacheTTL)
}

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
//   - 扫描所有 user message（不只最后一条，因为历史消息里的图片也需要识别）
//   - 只处理 content 数组中的 image_url（正经上传的图片）
//   - 识别结果作为文字追加到对应 user message 末尾
//   - 同一 URL 在一次请求内只识别一次（缓存）
func ProcessImages(messages []map[string]interface{}, userID int, username string) ([]map[string]interface{}, *VisionResult) {
	if len(messages) == 0 {
		return messages, &VisionResult{}
	}

	urlCache := map[string]string{} // URL → 描述缓存（同一次请求内去重）
	totalImages := 0
	modified := false

	// 遍历所有消息，处理每条 user message 中的 image_url
	for i := range messages {
		role, _ := messages[i]["role"].(string)
		if role != "user" {
			continue
		}

		content, ok := messages[i]["content"].([]interface{})
		if !ok {
			continue
		}

		// 检查这条消息是否有 image_url
		hasImage := false
		for _, item := range content {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "image_url" {
					hasImage = true
					break
				}
			}
		}
		if !hasImage {
			continue
		}

		// 处理这条消息中的 image_url（并发识别）
		var descriptions []string
		imageCount := 0
		newContent := make([]interface{}, 0, len(content))

		// 第一遍：扫描所有 image_url，检查缓存，收集需要调 API 的
		type imgTask struct {
			url      string
			idx      int // 在描述列表中的序号
			cached   string
			needCall bool
		}
		var tasks []imgTask
		descSlots := []string{} // 预分配描述槽位

		for _, item := range content {
			m, ok := item.(map[string]interface{})
			if !ok {
				newContent = append(newContent, item)
				continue
			}
			t, _ := m["type"].(string)

			if t == "image_url" {
				imgURL, _ := m["image_url"].(map[string]interface{})
				if imgURL == nil {
					continue
				}
				url, _ := imgURL["url"].(string)
				if url == "" {
					continue
				}

				idx := len(descSlots)
				descSlots = append(descSlots, "") // 预占位

				// 检查请求内缓存
				if cachedDesc, exists := urlCache[url]; exists {
					imageCount++
					task := imgTask{url: url, idx: idx, cached: cachedDesc, needCall: false}
					tasks = append(tasks, task)
					continue
				}

				// 检查 Redis 全局缓存
				redisKey := imageCacheKey(url)
				if cachedDesc, found := getCachedImage(redisKey); found {
					urlCache[url] = cachedDesc
					imageCount++
					tasks = append(tasks, imgTask{url: url, idx: idx, cached: cachedDesc, needCall: false})
					continue
				}

				urlCache[url] = "" // 标记已处理
				imageCount++
				tasks = append(tasks, imgTask{url: url, idx: idx, needCall: true})
				continue
			}

			newContent = append(newContent, item)
		}

		if imageCount == 0 {
			continue
		}

		// 第二遍：并发调用 API 识别需要识别的图片
		var wg sync.WaitGroup
		for i := range tasks {
			if !tasks[i].needCall {
				continue
			}
			wg.Add(1)
			go func(task *imgTask) {
				defer wg.Done()
				desc, err := RecognizeImage(userID, username, task.url, "")
				imageSize := len(task.url)
				if err != nil {
					LogVisionRecognize(userID, username, task.url, imageSize, "", "", visionModelID, false, err.Error())
					task.cached = ""
				} else if desc != "" {
					task.cached = desc
					setCachedImage(imageCacheKey(task.url), desc)
					LogVisionRecognize(userID, username, task.url, imageSize, "", desc, visionModelID, true, "")
				} else {
					LogVisionRecognize(userID, username, task.url, imageSize, "", "", visionModelID, false, "返回空结果")
					task.cached = ""
				}
			}(&tasks[i])
		}
		wg.Wait()

		// 第三遍：按顺序填充描述
		for _, task := range tasks {
			globalIdx := totalImages + task.idx + 1
			if task.cached != "" {
				descSlots[task.idx] = fmt.Sprintf("[图片 %d 内容：%s]", globalIdx, task.cached)
			} else {
				descSlots[task.idx] = fmt.Sprintf("[图片 %d：识别失败]", globalIdx)
			}
		}
		descriptions = descSlots

		// 把图片描述追加到这条 user message
		appendText := "\n\n[系统已通过视觉模型识别了以下图片内容，这就是你看到的图片，请基于此内容回答，不要说自己无法查看图片：]\n" + strings.Join(descriptions, "\n")
		newContent = append(newContent, map[string]interface{}{
			"type": "text",
			"text": appendText,
		})
		messages[i]["content"] = newContent
		totalImages += imageCount
		modified = true
	}

	if !modified {
		// 调试：记录最后一条 user 消息的情况
		lastUserContent := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if role, _ := messages[i]["role"].(string); role == "user" {
				lastUserContent = fmt.Sprintf("%v", messages[i]["content"])
				if len(lastUserContent) > 200 {
					lastUserContent = lastUserContent[:200]
				}
				break
			}
		}
		log.Printf("[vision] 未发现 image_url，最后一条 user 内容前200字符: %s", lastUserContent)
		return messages, &VisionResult{ImageCount: 0, Modified: false}
	}

	log.Printf("[vision] 共识别图片 %d 张", totalImages)
	return messages, &VisionResult{
		ImageCount: totalImages,
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
		"max_tokens":  4096,
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

	// 视觉模型是非流式请求，图片解码+推理可能超过 30 秒首字节超时，
	// 用 SharedHTTPClientLong（无首字节超时），靠 context 的 120 秒兜底
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

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
