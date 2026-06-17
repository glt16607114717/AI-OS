package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── LLM 代理核心 ──

// ConversationContext 累积一个对话的完整上下文信息
type ConversationContext struct {
	ID               string
	UserID           int
	Username         string
	UserMessage      string   // 用户提问摘要
	Models           []string // 使用的模型列表
	FailoverCount    int      // 故障转移次数
	TotalPrompt      int
	TotalCompletion  int
	ToolCallCount    int
	Errors           []string
	AssistantContent string // 最终助手回复
	Source           string // proxy / workspace
	StartTime        time.Time
}

// SummarizeAndLog 写一条对话汇总日志
func (c *ConversationContext) SummarizeAndLog() {
	latency := int(time.Since(c.StartTime).Milliseconds())
	models := strings.Join(c.Models, ",")
	level := "info"
	if len(c.Errors) > 0 {
		level = "error"
	}
	detail := map[string]interface{}{
		"conversation_id":  c.ID,
		"user_id":          c.UserID,
		"username":         c.Username,
		"models":           c.Models,
		"failover_count":   c.FailoverCount,
		"prompt_tokens":    c.TotalPrompt,
		"completion_tokens": c.TotalCompletion,
		"tool_calls":       c.ToolCallCount,
		"errors":           c.Errors,
		"user_message":     c.UserMessage,
		"latency_ms":       latency,
	}
	detailJSON, _ := json.Marshal(detail)
	service.WriteLog("conversation",
		fmt.Sprintf("对话完成 | %s | 输入%d/输出%d/耗时%dms", models, c.TotalPrompt, c.TotalCompletion, latency),
		level, string(detailJSON))
}

// generateConversationID 生成对话唯一标识
func generateConversationID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ProxyChatCompletions LLM 代理转发
func ProxyChatCompletions(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID := 0
	username := "anonymous"
	if session != nil {
		userID = session.UserID
		username = session.Username
	}

	conversationID := generateConversationID()

	// 解析请求
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	// 注入上帝指令
	if messages, ok := req["messages"].([]interface{}); ok {
		msgMaps := make([]map[string]interface{}, len(messages))
		for i, m := range messages {
			msgMaps[i], _ = m.(map[string]interface{})
		}
		msgMaps = service.InjectGodRules(msgMaps)
		req["messages"] = msgMaps
	}

	// 提取用户消息摘要（用于日志和 embedding）
	userMsgSummary := ""
	if messages, ok := req["messages"].([]interface{}); ok {
		for _, m := range messages {
			if msg, ok := m.(map[string]interface{}); ok {
				if role, _ := msg["role"].(string); role == "user" {
					userMsgSummary = extractContent(msg["content"])
				}
			}
		}
	}
	if len(userMsgSummary) > 200 {
		userMsgSummary = userMsgSummary[:200]
	}

	convCtx := &ConversationContext{
		ID:          conversationID,
		UserID:      userID,
		Username:    username,
		UserMessage: userMsgSummary,
		Source:      "proxy",
		StartTime:   startTime,
	}

	// 获取路由
	route := service.GetRouteByStrategy()
	if route != nil {
		// Fix 1: 无条件用策略的 model_id 覆盖客户端传的 model
		req["model"] = route.ModelID
	} else {
		route = service.GetDefaultRoute()
		if route != nil {
			req["model"] = route.ModelID
		}
	}
	if route == nil {
		convCtx.Errors = append(convCtx.Errors, "无可用厂商（无策略、无可用选项）")
		convCtx.SummarizeAndLog()
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	modelID := route.ModelID

	// 请求转储（只保留最近20个文件）
	service.DumpRequest(req)

	// 额度降级
	downgraded := service.ShouldDowngradeModel(route.KeyID, modelID)
	if downgraded != modelID {
		req["model"] = downgraded
		modelID = downgraded
	}

	// 检查是否流式
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	// Fix 3: 构建故障转移路由列表
	attempts := buildFailoverAttempts(route)

	if isStream {
		handleStreamWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary, convCtx)
	} else {
		handleNormalWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary, convCtx)
	}
}

// buildFailoverAttempts 构建故障转移路由列表（策略路由优先 + 轮询其他可用路由）
func buildFailoverAttempts(primary *service.RouteInfoType) []*service.RouteInfoType {
	attempts := []*service.RouteInfoType{primary}
	failoverRoutes := service.GetAllRoutesForFailover()
	for i := range failoverRoutes {
		if failoverRoutes[i].KeyID != primary.KeyID {
			attempts = append(attempts, &failoverRoutes[i])
		}
	}
	return attempts
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// handleNormalWithFailover 非流式响应（支持故障转移）
func handleNormalWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string, convCtx *ConversationContext) {
	var lastError string

	for idx, route := range attempts {
		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = append(convCtx.Models, route.ModelID)

		// 每个路由用自己的 model_id
		fwdReq := copyMap(req)
		fwdReq["model"] = route.ModelID

		bodyJSON, _ := json.Marshal(fwdReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		upstreamReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		upstreamReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(upstreamReq)
		if err != nil {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			lastError = err.Error()
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			latency := int(time.Since(startTime).Milliseconds())
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500]
			}
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			lastError = errMsg
			continue
		}

		// 成功
		var respData map[string]interface{}
		json.Unmarshal(body, &respData)

		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			promptTokens := intFloat(usage["prompt_tokens"])
			completionTokens := intFloat(usage["completion_tokens"])
			totalTokens := intFloat(usage["total_tokens"])
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID:   convCtx.ID,
				UserID:           userID, Username: username,
				VendorID:         route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		} else {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID:   convCtx.ID,
				UserID:           userID, Username: username,
				VendorID:       route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: true,
			})
		}

		// 记录助手回复
		if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok && content != "" {
						go service.AddChatMessage("assistant", content)
						convCtx.AssistantContent = content
					}
				}
			}
		}

		// 透传
		for k, v := range resp.Header {
			if k != "Content-Length" {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)

		log.Printf("[proxy] normal %s vendor=%d key=%s user=%s latency=%dms source=%s conv=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source, convCtx.ID)

		// 对话完成：合并存储 embedding + 写汇总日志
		if convCtx.AssistantContent != "" {
			go service.StoreEmbedding(userID, userMsgSummary+"\n\n"+convCtx.AssistantContent, convCtx.Source)
		}
		convCtx.SummarizeAndLog()
		return
	}

	// 所有厂商都失败
	convCtx.SummarizeAndLog()
	errResponse(w, fmt.Sprintf("所有厂商均失败: %s", lastError), 502)
}

// handleStreamWithFailover 流式响应（支持故障转移）
func handleStreamWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string, convCtx *ConversationContext) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		errResponse(w, "不支持流式响应", 500)
		return
	}

	var lastError string

	for idx, route := range attempts {
		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = append(convCtx.Models, route.ModelID)

		// 每个路由用自己的 model_id
		fwdReq := copyMap(req)
		fwdReq["model"] = route.ModelID

		bodyJSON, _ := json.Marshal(fwdReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		upstreamReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		upstreamReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 300 * time.Second}
		resp, err := client.Do(upstreamReq)
		if err != nil {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 流式中断: %s", route.VendorName, err.Error()))
			lastError = err.Error()
			continue
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500]
			}
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			lastError = errMsg
			continue
		}

		// 连接成功，开始流式输出（不能再故障转移）
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		totalContent := ""
		success := false
		// 保存最后一个非 null 的 usage（火山方舟只有最后一个 chunk 带 usage）
		var lastUsage map[string]interface{}

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				fmt.Fprintf(w, "\n")
				flusher.Flush()
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				fmt.Fprintf(w, "%s\n", line)
				flusher.Flush()
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				success = true
				break
			}

			// 解析 SSE 数据，提取 usage 和 content（不在此处记录统计）
			var sseData map[string]interface{}
			if json.Unmarshal([]byte(data), &sseData) == nil {
				// usage 可能是 null（火山方舟中间 chunk），只保存非 null 的
				if usage, ok := sseData["usage"].(map[string]interface{}); ok {
					lastUsage = usage
				}
				// 提取内容：同时支持 content 和 reasoning_content（推理模型）
				if choices, ok := sseData["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok && content != "" {
								totalContent += content
							}
							if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
								totalContent += reasoning
							}
						}
					}
				}
			}

			fmt.Fprintf(w, "%s\n\n", line)
			flusher.Flush()
		}

		resp.Body.Close()

		// 流式结束后统一记录一条统计（无论 usage 是否存在）
		if success || totalContent != "" {
			latency := int(time.Since(startTime).Milliseconds())
			promptTokens, completionTokens, totalTokens := 0, 0, 0
			if lastUsage != nil {
				promptTokens = intFloat(lastUsage["prompt_tokens"])
				completionTokens = intFloat(lastUsage["completion_tokens"])
				totalTokens = intFloat(lastUsage["total_tokens"])
				convCtx.TotalPrompt += promptTokens
				convCtx.TotalCompletion += completionTokens
			}
			service.RecordStat(&service.LLMStatType{
				ConversationID:   convCtx.ID,
				UserID:           userID, Username: username,
				VendorID:         route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
		}

	// 记录聊天
		if totalContent != "" {
			go service.AddChatMessage("assistant", totalContent)
			convCtx.AssistantContent = totalContent
		}

		log.Printf("[proxy] stream %s vendor=%d key=%s user=%s latency=%dms source=%s conv=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source, convCtx.ID)

		// 对话完成：合并存储 embedding + 写汇总日志
		if convCtx.AssistantContent != "" {
			go service.StoreEmbedding(userID, userMsgSummary+"\n\n"+convCtx.AssistantContent, convCtx.Source)
		}
		convCtx.SummarizeAndLog()
		return
	}

	// 所有路由都失败
	convCtx.SummarizeAndLog()
	fmt.Fprintf(w, "data: {\"error\":{\"message\":\"所有厂商均失败: %s\",\"type\":\"all_vendors_failed\"}}\n\n", lastError)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// extractContent 兼容提取 content：string 或 [{"type":"text","text":"..."}] 数组格式
func extractContent(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if text, _ := m["text"].(string); text != "" {
						sb.WriteString(text)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}

func handleStreamResponse(w http.ResponseWriter, resp *http.Response, startTime time.Time, route *service.RouteInfoType, modelID string, userID int, username string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		errResponse(w, "不支持流式响应", 500)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	totalContent := ""
	success := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			fmt.Fprintf(w, "%s\n", line)
			flusher.Flush()
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}

		// 解析 SSE 数据，提取 usage
		var sseData map[string]interface{}
		if json.Unmarshal([]byte(data), &sseData) == nil {
			if usage, ok := sseData["usage"].(map[string]interface{}); ok {
				promptTokens := intFloat(usage["prompt_tokens"])
				completionTokens := intFloat(usage["completion_tokens"])
				totalTokens := intFloat(usage["total_tokens"])
				latency := int(time.Since(startTime).Milliseconds())
				service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: modelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
			success = true
		}
		// 提取内容
			if choices, ok := sseData["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							totalContent += content
						}
					}
				}
			}
		}

		fmt.Fprintf(w, "%s\n", line)
		flusher.Flush()
	}

	// 记录聊天
	if totalContent != "" {
		go service.AddChatMessage("assistant", totalContent)
	}

	// 兜底：如果 SSE 流没有返回 usage，流结束后补一条统计
	if !success {
		latency := int(time.Since(startTime).Milliseconds())
		service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: modelID,
			LatencyMs: latency, Success: true,
		})
	}

	log.Printf("[proxy] stream %s vendor=%d key=%s user=%s latency=%dms",
		modelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds())
}

func handleNormalResponse(w http.ResponseWriter, resp *http.Response, startTime time.Time, route *service.RouteInfoType, modelID string, userID int, username string) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errResponse(w, "读取上游响应失败", 502)
		return
	}

	// 如果上游返回错误
	if resp.StatusCode >= 400 {
		latency := int(time.Since(startTime).Milliseconds())
		service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: modelID,
			LatencyMs: latency, Success: false, Error: string(body),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	// 解析响应，提取 usage
	var respData map[string]interface{}
	if json.Unmarshal(body, &respData) == nil {
		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			promptTokens := intFloat(usage["prompt_tokens"])
			completionTokens := intFloat(usage["completion_tokens"])
			totalTokens := intFloat(usage["total_tokens"])
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: modelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
		}

		// 记录助手回复
		if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok && content != "" {
						go service.AddChatMessage("assistant", content)
					}
				}
			}
		}
	}

	// 透传响应
	for k, v := range resp.Header {
		if k != "Content-Length" {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(body)

	log.Printf("[proxy] normal %s vendor=%d key=%s user=%s latency=%dms status=%d",
		modelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), resp.StatusCode)
}

// ProxyModels 模型列表代理
func ProxyModels(w http.ResponseWriter, r *http.Request) {
	route := service.GetRouteByStrategy()
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	baseURL := strings.TrimRight(route.BaseURL, "/")
	upstreamURL := baseURL + "/models"

	req, _ := http.NewRequest("GET", upstreamURL, nil)
	req.Header.Set("Authorization", "Bearer "+route.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		errResponse(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// WorkspaceChat 工作台聊天（带 Function Calling + 流式 + 故障转移）
func WorkspaceChat(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID := 0
	username := "anonymous"
	if session != nil {
		userID = session.UserID
		username = session.Username
	}

	conversationID := generateConversationID()

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	// 注入上帝指令
	if messages, ok := req["messages"].([]interface{}); ok {
		msgMaps := make([]map[string]interface{}, len(messages))
		for i, m := range messages {
			msgMaps[i], _ = m.(map[string]interface{})
		}
		msgMaps = service.InjectGodRules(msgMaps)
		req["messages"] = msgMaps
	}

	// 注入技能工具
	tools, _ := service.GetSkillToolDefinitions()
	if len(tools) > 0 {
		req["tools"] = tools
	}

	// 提取用户消息摘要
	userMsgSummary := ""
	if messages, ok := req["messages"].([]interface{}); ok {
		for _, m := range messages {
			if msg, ok := m.(map[string]interface{}); ok {
				if role, _ := msg["role"].(string); role == "user" {
					if content, _ := msg["content"].(string); content != "" {
						userMsgSummary = content
					}
				}
			}
		}
	}
	if len(userMsgSummary) > 200 {
		userMsgSummary = userMsgSummary[:200]
	}

	convCtx := &ConversationContext{
		ID:          conversationID,
		UserID:      userID,
		Username:    username,
		UserMessage: userMsgSummary,
		Source:      "workspace",
		StartTime:   startTime,
	}

	// 获取路由
	route := service.GetRouteByStrategy()
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		convCtx.Errors = append(convCtx.Errors, "无可用厂商（无策略、无可用选项）")
		convCtx.SummarizeAndLog()
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	modelID := route.ModelID
	req["model"] = modelID

	// 请求转储（只保留最近20个文件）
	service.DumpRequest(req)

	// 构建故障转移路由列表
	attempts := buildFailoverAttempts(route)

	// 检查是否流式
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	if isStream {
		handleWorkspaceStreamWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary, convCtx)
	} else {
		handleWorkspaceNormalWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary, convCtx)
	}
}

func keyPrefix8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// handleWorkspaceNormalWithFailover WorkspaceChat 非流式响应（支持故障转移和工具调用）
func handleWorkspaceNormalWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string, convCtx *ConversationContext) {
	failedVendors := make(map[int]bool)
	var lastError string

	for idx, route := range attempts {
		if failedVendors[route.VendorID] {
			continue
		}

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = append(convCtx.Models, route.ModelID)

		fwdReq := copyMap(req)
		fwdReq["model"] = route.ModelID

		bodyJSON, _ := json.Marshal(fwdReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		upstreamReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		upstreamReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(upstreamReq)
		if err != nil {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			failedVendors[route.VendorID] = true
			lastError = err.Error()
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			latency := int(time.Since(startTime).Milliseconds())
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500]
			}
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			failedVendors[route.VendorID] = true
			lastError = errMsg
			continue
		}

		var respData map[string]interface{}
		json.Unmarshal(body, &respData)

		// 处理工具调用
		if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
						// 执行技能调用（带故障转移）
						convCtx.ToolCallCount++
						handleToolCallsWithFailover(w, req, attempts[idx:], toolCalls, username, startTime, userID, userMsgSummary, convCtx)
						return
					}
					if content, ok := msg["content"].(string); ok && content != "" {
						go service.AddChatMessage("assistant", content)
						convCtx.AssistantContent = content
					}
				}
			}
		}

		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			promptTokens := intFloat(usage["prompt_tokens"])
			completionTokens := intFloat(usage["completion_tokens"])
			totalTokens := intFloat(usage["total_tokens"])
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID:   convCtx.ID,
				UserID:           userID, Username: username,
				VendorID:         route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		}

		for k, v := range resp.Header {
			if k != "Content-Length" {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(body)

		log.Printf("[workspace] normal %s vendor=%d key=%s user=%s latency=%dms source=%s conv=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source, convCtx.ID)

		// 对话完成：合并存储 embedding + 写汇总日志
		if convCtx.AssistantContent != "" {
			go service.StoreEmbedding(userID, userMsgSummary+"\n\n"+convCtx.AssistantContent, convCtx.Source)
		}
		convCtx.SummarizeAndLog()
		return
	}

	// 所有厂商都失败
	convCtx.SummarizeAndLog()
	errResponse(w, fmt.Sprintf("所有厂商均失败: %s", lastError), 502)
}

// handleWorkspaceStreamWithFailover WorkspaceChat 流式响应（支持故障转移）
func handleWorkspaceStreamWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string, convCtx *ConversationContext) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		errResponse(w, "不支持流式响应", 500)
		return
	}

	failedVendors := make(map[int]bool)
	var lastError string

	for idx, route := range attempts {
		if failedVendors[route.VendorID] {
			continue
		}

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = append(convCtx.Models, route.ModelID)

		fwdReq := copyMap(req)
		fwdReq["model"] = route.ModelID

		bodyJSON, _ := json.Marshal(fwdReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		upstreamReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		upstreamReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 300 * time.Second}
		resp, err := client.Do(upstreamReq)
		if err != nil {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 流式中断: %s", route.VendorName, err.Error()))
			failedVendors[route.VendorID] = true
			lastError = err.Error()
			continue
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500]
			}
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				ConversationID: convCtx.ID,
				UserID:         userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			failedVendors[route.VendorID] = true
			lastError = errMsg
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		totalContent := ""
		success := false

		for scanner.Scan() {
			line := scanner.Text()

			if line == "" {
				fmt.Fprintf(w, "\n")
				flusher.Flush()
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				fmt.Fprintf(w, "%s\n", line)
				flusher.Flush()
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				success = true
				break
			}

			var sseData map[string]interface{}
			if json.Unmarshal([]byte(data), &sseData) == nil {
				if usage, ok := sseData["usage"].(map[string]interface{}); ok {
					promptTokens := intFloat(usage["prompt_tokens"])
					completionTokens := intFloat(usage["completion_tokens"])
					totalTokens := intFloat(usage["total_tokens"])
					latency := int(time.Since(startTime).Milliseconds())
					service.RecordStat(&service.LLMStatType{
					ConversationID:   convCtx.ID,
					UserID:           userID, Username: username,
					VendorID:         route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
					PromptTokens: promptTokens, CompletionTokens: completionTokens,
					TotalTokens: totalTokens, LatencyMs: latency, Success: true,
				})
				convCtx.TotalPrompt += promptTokens
				convCtx.TotalCompletion += completionTokens
				success = true
			}
			if choices, ok := sseData["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							totalContent += content
						}
					}
				}
			}
		}

		fmt.Fprintf(w, "%s\n\n", line)
		flusher.Flush()
	}

	resp.Body.Close()

	if !success && totalContent != "" {
		latency := int(time.Since(startTime).Milliseconds())
		service.RecordStat(&service.LLMStatType{
			ConversationID: convCtx.ID,
			UserID:         userID, Username: username,
			VendorID:       route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			LatencyMs: latency, Success: true,
		})
	}

		if totalContent != "" {
			go service.AddChatMessage("assistant", totalContent)
			convCtx.AssistantContent = totalContent
		}

		log.Printf("[workspace] stream %s vendor=%d key=%s user=%s latency=%dms source=%s conv=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source, convCtx.ID)

		// 对话完成：合并存储 embedding + 写汇总日志
		if convCtx.AssistantContent != "" {
			go service.StoreEmbedding(userID, userMsgSummary+"\n\n"+convCtx.AssistantContent, convCtx.Source)
		}
		convCtx.SummarizeAndLog()
		return
	}

	// 所有路由都失败
	convCtx.SummarizeAndLog()
	fmt.Fprintf(w, "data: {\"error\":{\"message\":\"所有厂商均失败: %s\",\"type\":\"all_vendors_failed\"}}\n\n", lastError)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleToolCallsWithFailover 工具调用（支持故障转移）
func handleToolCallsWithFailover(w http.ResponseWriter, originalReq map[string]interface{}, attempts []*service.RouteInfoType, toolCalls []interface{}, username string, startTime time.Time, userID int, userMsgSummary string, convCtx *ConversationContext) {
	tc, _ := toolCalls[0].(map[string]interface{})
	fn, _ := tc["function"].(map[string]interface{})
	fnName, _ := fn["name"].(string)

	if strings.HasPrefix(fnName, "skill_") {
		skillID := strings.TrimPrefix(fnName, "skill_")
		result, err := service.ExecuteSkill(skillID)
		if err != nil {
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("技能执行失败 %s: %s", skillID, err.Error()))
			convCtx.SummarizeAndLog()
			errResponse(w, err.Error(), 500)
			return
		}

		messages, _ := originalReq["messages"].([]interface{})
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    "",
			"tool_calls": toolCalls,
		})
		resultJSON, _ := json.Marshal(result)
		tcID, _ := tc["id"].(string)
		messages = append(messages, map[string]interface{}{
			"role":         "tool",
			"content":      string(resultJSON),
			"tool_call_id": tcID,
		})

		failedVendors := make(map[int]bool)
		var lastError string

		for idx, route := range attempts {
			if failedVendors[route.VendorID] {
				continue
			}

			source := "策略路由"
			if idx > 0 {
				source = "故障转移"
				convCtx.FailoverCount++
			}
			convCtx.Models = append(convCtx.Models, route.ModelID)

			secondReq := map[string]interface{}{
				"model":    route.ModelID,
				"messages": messages,
				"stream":   false,
			}
			secondJSON, _ := json.Marshal(secondReq)
			baseURL := strings.TrimRight(route.BaseURL, "/")
			upstreamReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(secondJSON)))
			upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
			upstreamReq.Header.Set("Content-Type", "application/json")

			client := &http.Client{Timeout: 120 * time.Second}
			resp2, err := client.Do(upstreamReq)
			if err != nil {
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 工具总结网络错误: %s", route.VendorName, err.Error()))
				failedVendors[route.VendorID] = true
				lastError = err.Error()
				continue
			}

			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()

			if resp2.StatusCode != 200 {
				errMsg := string(body2)
				if len(errMsg) > 500 {
					errMsg = errMsg[:500]
				}
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 工具总结 HTTP %d: %s", route.VendorName, resp2.StatusCode, errMsg))
				failedVendors[route.VendorID] = true
				lastError = errMsg
				continue
			}

			for k, v := range resp2.Header {
				if k != "Content-Length" {
					for _, vv := range v {
						w.Header().Add(k, vv)
					}
				}
			}
			w.WriteHeader(resp2.StatusCode)
			w.Write(body2)

			var respData2 map[string]interface{}
			if json.Unmarshal(body2, &respData2) == nil {
				if usage, ok := respData2["usage"].(map[string]interface{}); ok {
					promptTokens := intFloat(usage["prompt_tokens"])
					completionTokens := intFloat(usage["completion_tokens"])
					totalTokens := intFloat(usage["total_tokens"])
					latency := int(time.Since(startTime).Milliseconds())
					service.RecordStat(&service.LLMStatType{
					ConversationID:   convCtx.ID,
					UserID:           userID, Username: username,
					VendorID:         route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
					PromptTokens: promptTokens, CompletionTokens: completionTokens,
					TotalTokens: totalTokens, LatencyMs: latency, Success: true,
				})
				convCtx.TotalPrompt += promptTokens
				convCtx.TotalCompletion += completionTokens
				}
				if choices, ok := respData2["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if msg, ok := choice["message"].(map[string]interface{}); ok {
							if content, ok := msg["content"].(string); ok && content != "" {
								go service.AddChatMessage("assistant", content)
								convCtx.AssistantContent = content
							}
						}
					}
				}
			}
			log.Printf("[workspace] skill=%s user=%s latency=%dms source=%s conv=%s", skillID, username, time.Since(startTime).Milliseconds(), source, convCtx.ID)

			// 对话完成：合并存储 embedding + 写汇总日志
			if convCtx.AssistantContent != "" {
				go service.StoreEmbedding(userID, userMsgSummary+"\n\n"+convCtx.AssistantContent, convCtx.Source)
			}
			convCtx.SummarizeAndLog()
			return
		}

		// 工具调用后总结全部失败
		convCtx.SummarizeAndLog()
		errResponse(w, fmt.Sprintf("工具调用后总结失败: %s", lastError), 502)
	} else {
		convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("未知工具: %s", fnName))
		convCtx.SummarizeAndLog()
		errResponse(w, fmt.Sprintf("未知工具: %s", fnName), 400)
	}
}
