package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── LLM 代理核心 ──

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

	// 获取路由
	route := service.GetRouteByStrategy()
	if route == nil {
		// 尝试默认路由
		route = service.GetDefaultRoute()
	}
	if route == nil {
		service.WriteLog("proxy", "无可用厂商处理模型", "error", fmt.Sprintf("user=%s", username))
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	modelID, _ := req["model"].(string)
	// 额度降级
	downgraded := service.ShouldDowngradeModel(route.KeyID, modelID)
	if downgraded != modelID {
		req["model"] = downgraded
		service.WriteLog("proxy", fmt.Sprintf("额度降级: %s -> %s", modelID, downgraded), "warn", "")
		modelID = downgraded
	}

	// 设置请求模型
	if req["model"] == nil {
		req["model"] = route.ModelID
		modelID = route.ModelID
	}

	// 构建上游 URL
	baseURL := strings.TrimRight(route.BaseURL, "/")
	upstreamURL := baseURL + "/chat/completions"

	// 发送请求
	bodyJSON, _ := json.Marshal(req)
	upstreamReq, _ := http.NewRequest("POST", upstreamURL, strings.NewReader(string(bodyJSON)))
	upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
	upstreamReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		latency := int(time.Since(startTime).Milliseconds())
		service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username,
			VendorID: route.VendorID, ModelID: modelID,
			LatencyMs: latency, Success: false, Error: err.Error(),
		})
		service.WriteLog("proxy", fmt.Sprintf("上游请求失败: %s", err.Error()), "error", "")
		errResponse(w, fmt.Sprintf("上游请求失败: %s", err.Error()), 502)
		return
	}
	defer resp.Body.Close()

	// 检查是否流式
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	if isStream {
		handleStreamResponse(w, resp, startTime, route, modelID, userID, username)
	} else {
		handleNormalResponse(w, resp, startTime, route, modelID, userID, username)
	}
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
					VendorID: route.VendorID, ModelID: modelID,
					PromptTokens: promptTokens, CompletionTokens: completionTokens,
					TotalTokens: totalTokens, LatencyMs: latency, Success: true,
				})
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
			VendorID: route.VendorID, ModelID: modelID,
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
				VendorID: route.VendorID, ModelID: modelID,
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

// WorkspaceChat 工作台聊天（带 Function Calling）
func WorkspaceChat(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	username := "anonymous"
	if session != nil {
		username = session.Username
	}

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

	// 获取路由
	route := service.GetRouteByStrategy()
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	modelID := route.ModelID
	req["model"] = modelID

	baseURL := strings.TrimRight(route.BaseURL, "/")
	upstreamURL := baseURL + "/chat/completions"

	bodyJSON, _ := json.Marshal(req)
	upstreamReq, _ := http.NewRequest("POST", upstreamURL, strings.NewReader(string(bodyJSON)))
	upstreamReq.Header.Set("Authorization", "Bearer "+route.APIKey)
	upstreamReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		errResponse(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// 解析响应，处理 function calling
	var respData map[string]interface{}
	json.Unmarshal(body, &respData)

	if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
					// 执行技能调用
					handleToolCalls(w, req, route, toolCalls, username, startTime)
					return
				}
				// 记录助手回复
				if content, ok := msg["content"].(string); ok && content != "" {
					go service.AddChatMessage("assistant", content)
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

	log.Printf("[workspace] %s user=%s latency=%dms", modelID, username, time.Since(startTime).Milliseconds())
}

func handleToolCalls(w http.ResponseWriter, originalReq map[string]interface{}, route *service.RouteInfoType, toolCalls []interface{}, username string, startTime time.Time) {
	// 执行第一个 tool call
	tc, _ := toolCalls[0].(map[string]interface{})
	fn, _ := tc["function"].(map[string]interface{})
	fnName, _ := fn["name"].(string)

	if strings.HasPrefix(fnName, "skill_") {
		skillID := strings.TrimPrefix(fnName, "skill_")
		result, err := service.ExecuteSkill(skillID)
		if err != nil {
			errResponse(w, err.Error(), 500)
			return
		}

		// 把技能结果发回给 LLM 做总结
		messages, _ := originalReq["messages"].([]interface{})
		// 添加 assistant 的 tool_call 消息
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    "",
			"tool_calls": toolCalls,
		})
		// 添加 tool 结果
		resultJSON, _ := json.Marshal(result)
		tcID, _ := tc["id"].(string)
		messages = append(messages, map[string]interface{}{
			"role":         "tool",
			"content":      string(resultJSON),
			"tool_call_id": tcID,
		})

		// 第二次请求 LLM
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
			errResponse(w, err.Error(), 502)
			return
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)

		for k, v := range resp2.Header {
			if k != "Content-Length" {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
		}
		w.WriteHeader(resp2.StatusCode)
		w.Write(body2)

		// 记录
		var respData2 map[string]interface{}
		if json.Unmarshal(body2, &respData2) == nil {
			if choices, ok := respData2["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if msg, ok := choice["message"].(map[string]interface{}); ok {
						if content, ok := msg["content"].(string); ok && content != "" {
							go service.AddChatMessage("assistant", content)
						}
					}
				}
			}
		}
		log.Printf("[workspace] skill=%s user=%s latency=%dms", skillID, username, time.Since(startTime).Milliseconds())
	} else {
		errResponse(w, fmt.Sprintf("未知工具: %s", fnName), 400)
	}
}

func keyPrefix8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
