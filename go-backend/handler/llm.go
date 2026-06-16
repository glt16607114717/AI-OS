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
	if route != nil {
		// Fix 1: 无条件用策略的 model_id 覆盖客户端传的 model
		req["model"] = route.ModelID
		service.WriteLog("route", fmt.Sprintf("策略路由选中 %s（%s）→ %s", route.VendorName, keyPrefix8(route.KeyID), route.ModelID), "info", fmt.Sprintf("vendor=%d", route.VendorID))
	} else {
		route = service.GetDefaultRoute()
		if route != nil {
			req["model"] = route.ModelID
			service.WriteLog("route", fmt.Sprintf("默认路由到 %s → %s", route.VendorName, route.ModelID), "info", fmt.Sprintf("vendor=%d", route.VendorID))
		}
	}
	if route == nil {
		service.WriteLog("error", "无可用厂商（无策略、无可用选项）", "error", "")
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
		service.WriteLog("downgrade", fmt.Sprintf("%s → %s（Key %s 用量偏高）", modelID, downgraded, keyPrefix8(route.KeyID)), "warn", fmt.Sprintf("key_id=%s", route.KeyID))
		modelID = downgraded
	}

	// 提取用户消息摘要（用于日志）
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

	// 检查是否流式
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	service.WriteLog("request",
		fmt.Sprintf("%s → %s（%s）%s", modelID, route.VendorName, keyPrefix8(route.KeyID), ternary(isStream, "流式", "非流式")),
		"info", fmt.Sprintf("model=%s,stream=%v,user_msg=%s", modelID, isStream, userMsgSummary))

	// Fix 3: 构建故障转移路由列表
	attempts := buildFailoverAttempts(route)

	if isStream {
		handleStreamWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary)
	} else {
		handleNormalWithFailover(w, req, attempts, startTime, modelID, userID, username, userMsgSummary)
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
func handleNormalWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string) {
	failedVendors := make(map[int]bool)
	var lastError string

	for idx, route := range attempts {
		if failedVendors[route.VendorID] {
			continue
		}

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
		}

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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			service.WriteLog("failover", fmt.Sprintf("%s 请求失败（网络错误）", route.VendorName), "error", fmt.Sprintf("source=%s,error=%s", source, err.Error()))
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			service.WriteLog("failover", fmt.Sprintf("%s 请求失败（HTTP %d）", route.VendorName, resp.StatusCode), "error", fmt.Sprintf("source=%s,error=%s", source, errMsg))
			failedVendors[route.VendorID] = true
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
			service.WriteLog("request", fmt.Sprintf("响应成功: %s（%s）输入 %d / 输出 %d / 耗时 %dms", route.ModelID, source, promptTokens, completionTokens, latency), "info", "")
		} else {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: true,
			})
		}

		// 记录助手回复
		if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok && content != "" {
						go service.AddChatMessage("assistant", content)
						go service.StoreEmbedding(userID, content, "proxy")
					}
				}
			}
		}

		if source == "故障转移" {
			service.WriteLog("failover", fmt.Sprintf("故障转移成功: %s", route.VendorName), "info", fmt.Sprintf("model=%s", route.ModelID))
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

		log.Printf("[proxy] normal %s vendor=%d key=%s user=%s latency=%dms source=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source)
		return
	}

	// 所有厂商都失败
	service.WriteLog("failover", "所有厂商均失败", "error", fmt.Sprintf("last_error=%s", lastError))
	errResponse(w, fmt.Sprintf("所有厂商均失败: %s", lastError), 502)
}

// handleStreamWithFailover 流式响应（支持故障转移）
func handleStreamWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string, userMsgSummary string) {
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
		}

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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			service.WriteLog("error", fmt.Sprintf("流式中断: %s", route.VendorName), "error", fmt.Sprintf("error=%s", err.Error()))
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			service.WriteLog("error", fmt.Sprintf("上游返回错误: HTTP %d（%s）", resp.StatusCode, route.VendorName), "error", fmt.Sprintf("error=%s", errMsg))
			failedVendors[route.VendorID] = true
			lastError = errMsg
			continue
		}

		// 连接成功，开始流式输出（不能再故障转移）
		if source == "故障转移" {
			service.WriteLog("failover", fmt.Sprintf("故障转移成功: %s（%s）", route.VendorName, source), "info", fmt.Sprintf("model=%s", route.ModelID))
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		totalContent := ""
		success := false

		for scanner.Scan() {
			line := scanner.Text()

			// Fix 2: 空行不跳过，输出保证 SSE 的 \n\n 格式
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

			// 解析 SSE 数据，提取 usage 和 content
			var sseData map[string]interface{}
			if json.Unmarshal([]byte(data), &sseData) == nil {
				if usage, ok := sseData["usage"].(map[string]interface{}); ok {
					promptTokens := intFloat(usage["prompt_tokens"])
					completionTokens := intFloat(usage["completion_tokens"])
					totalTokens := intFloat(usage["total_tokens"])
					latency := int(time.Since(startTime).Milliseconds())
					service.RecordStat(&service.LLMStatType{
						UserID: userID, Username: username,
						VendorID: route.VendorID, ModelID: route.ModelID,
						PromptTokens: promptTokens, CompletionTokens: completionTokens,
						TotalTokens: totalTokens, LatencyMs: latency, Success: true,
					})
					service.WriteLog("request", fmt.Sprintf("流式完成: %s（%d+%d tokens）", route.VendorName, promptTokens, completionTokens), "info", fmt.Sprintf("latency=%dms,model=%s", latency, route.ModelID))
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

			// Fix 2: 每行后跟 \n\n，保证 SSE 事件分隔
			fmt.Fprintf(w, "%s\n\n", line)
			flusher.Flush()
		}

		resp.Body.Close()

		// 无 usage 但有内容的也记录
		if !success && totalContent != "" {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: true,
			})
			service.WriteLog("request", fmt.Sprintf("流式完成（无 usage）: %s", route.VendorName), "info", fmt.Sprintf("latency=%dms", latency))
		}

		// 记录聊天
		if totalContent != "" {
			go service.AddChatMessage("assistant", totalContent)
			go service.StoreEmbedding(userID, totalContent, "proxy")
		}

		log.Printf("[proxy] stream %s vendor=%d key=%s user=%s latency=%dms source=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source)
		return
	}

	// 所有路由都失败
	service.WriteLog("failover", "所有厂商均失败（流式）", "error", fmt.Sprintf("last_error=%s", lastError))
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
		go service.StoreEmbedding(userID, totalContent, "proxy")
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
						go service.StoreEmbedding(userID, content, "proxy")
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

	// 请求转储（只保留最近20个文件）
	service.DumpRequest(req)

	service.WriteLog("workspace", fmt.Sprintf("工作台聊天开始: %s", route.ModelID), "info", fmt.Sprintf("user=%s", username))

	// 构建故障转移路由列表
	attempts := buildFailoverAttempts(route)

	// 检查是否流式
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	if isStream {
		handleWorkspaceStreamWithFailover(w, req, attempts, startTime, modelID, userID, username)
	} else {
		handleWorkspaceNormalWithFailover(w, req, attempts, startTime, modelID, userID, username)
	}
}

func keyPrefix8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// handleWorkspaceNormalWithFailover WorkspaceChat 非流式响应（支持故障转移和工具调用）
func handleWorkspaceNormalWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string) {
	failedVendors := make(map[int]bool)
	var lastError string

	for idx, route := range attempts {
		if failedVendors[route.VendorID] {
			continue
		}

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
		}

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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			service.WriteLog("failover", fmt.Sprintf("WorkspaceChat %s 请求失败（网络错误）", route.VendorName), "error", fmt.Sprintf("source=%s,error=%s", source, err.Error()))
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			service.WriteLog("failover", fmt.Sprintf("WorkspaceChat %s 请求失败（HTTP %d）", route.VendorName, resp.StatusCode), "error", fmt.Sprintf("source=%s,error=%s", source, errMsg))
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
						handleToolCallsWithFailover(w, req, attempts[idx:], toolCalls, username, startTime, userID)
						return
					}
					if content, ok := msg["content"].(string); ok && content != "" {
						go service.AddChatMessage("assistant", content)
						go service.StoreEmbedding(userID, content, "workspace")
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: latency, Success: true,
			})
			service.WriteLog("workspace", fmt.Sprintf("响应成功: %s（%s）输入 %d / 输出 %d / 耗时 %dms", route.ModelID, source, promptTokens, completionTokens, latency), "info", "")
		}

		if source == "故障转移" {
			service.WriteLog("failover", fmt.Sprintf("WorkspaceChat 故障转移成功: %s", route.VendorName), "info", fmt.Sprintf("model=%s", route.ModelID))
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

		log.Printf("[workspace] normal %s vendor=%d key=%s user=%s latency=%dms source=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source)
		return
	}

	service.WriteLog("failover", "WorkspaceChat 所有厂商均失败", "error", fmt.Sprintf("last_error=%s", lastError))
	errResponse(w, fmt.Sprintf("所有厂商均失败: %s", lastError), 502)
}

// handleWorkspaceStreamWithFailover WorkspaceChat 流式响应（支持故障转移）
func handleWorkspaceStreamWithFailover(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, startTime time.Time, modelID string, userID int, username string) {
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
		}

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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			service.WriteLog("error", fmt.Sprintf("WorkspaceChat 流式中断: %s", route.VendorName), "error", fmt.Sprintf("error=%s", err.Error()))
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: errMsg,
			})
			service.WriteLog("error", fmt.Sprintf("WorkspaceChat 上游返回错误: HTTP %d（%s）", resp.StatusCode, route.VendorName), "error", fmt.Sprintf("error=%s", errMsg))
			failedVendors[route.VendorID] = true
			lastError = errMsg
			continue
		}

		if source == "故障转移" {
			service.WriteLog("failover", fmt.Sprintf("WorkspaceChat 故障转移成功: %s", route.VendorName), "info", fmt.Sprintf("model=%s", route.ModelID))
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
						UserID: userID, Username: username,
						VendorID: route.VendorID, ModelID: route.ModelID,
						PromptTokens: promptTokens, CompletionTokens: completionTokens,
						TotalTokens: totalTokens, LatencyMs: latency, Success: true,
					})
					service.WriteLog("workspace", fmt.Sprintf("流式完成: %s（%d+%d tokens）", route.VendorName, promptTokens, completionTokens), "info", fmt.Sprintf("latency=%dms,model=%s", latency, route.ModelID))
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, ModelID: route.ModelID,
				LatencyMs: latency, Success: true,
			})
			service.WriteLog("workspace", fmt.Sprintf("流式完成（无 usage）: %s", route.VendorName), "info", fmt.Sprintf("latency=%dms", latency))
		}

		if totalContent != "" {
			go service.AddChatMessage("assistant", totalContent)
			go service.StoreEmbedding(userID, totalContent, "workspace")
		}

		log.Printf("[workspace] stream %s vendor=%d key=%s user=%s latency=%dms source=%s",
			route.ModelID, route.VendorID, keyPrefix8(route.KeyID), username, time.Since(startTime).Milliseconds(), source)
		return
	}

	service.WriteLog("failover", "WorkspaceChat 所有厂商均失败（流式）", "error", fmt.Sprintf("last_error=%s", lastError))
	fmt.Fprintf(w, "data: {\"error\":{\"message\":\"所有厂商均失败: %s\",\"type\":\"all_vendors_failed\"}}\n\n", lastError)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// handleToolCallsWithFailover 工具调用（支持故障转移）
func handleToolCallsWithFailover(w http.ResponseWriter, originalReq map[string]interface{}, attempts []*service.RouteInfoType, toolCalls []interface{}, username string, startTime time.Time, userID int) {
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
			}

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
				failedVendors[route.VendorID] = true
				lastError = err.Error()
				continue
			}

			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()

			if resp2.StatusCode != 200 {
				failedVendors[route.VendorID] = true
				lastError = string(body2)
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
				if choices, ok := respData2["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if msg, ok := choice["message"].(map[string]interface{}); ok {
							if content, ok := msg["content"].(string); ok && content != "" {
								go service.AddChatMessage("assistant", content)
								go service.StoreEmbedding(userID, content, "workspace")
							}
						}
					}
				}
			}
			log.Printf("[workspace] skill=%s user=%s latency=%dms source=%s", skillID, username, time.Since(startTime).Milliseconds(), source)
			return
		}

		errResponse(w, fmt.Sprintf("工具调用后总结失败: %s", lastError), 502)
	} else {
		errResponse(w, fmt.Sprintf("未知工具: %s", fnName), 400)
	}
}
