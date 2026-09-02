package handler

import (
	// "ai-os-server/circuit" // [DISABLED 2025-07-22] 熔断机制暂时关闭
	"ai-os-server/service"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
)

// ── 公共：单次 LLM 调用（带故障转移）──

// LLMResult 单次 LLM 调用的结果
type LLMResult struct {
	Body       []byte                  // 原始响应体
	StatusCode int                     // HTTP 状态码
	Header     http.Header             // 响应头
	Data       map[string]interface{}  // 解析后的 JSON（仅非流式有效）
	Route      *service.RouteInfoType  // 使用的路由
}

// appendUnique 仅当切片中不存在 v 时才追加，保证 Models / KeyNames 唯一
// （agent 多轮循环中主路由不变，避免同一 key/model 被重复记录）
func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// callLLMWithFailover 用故障转移链请求 LLM（非流式）
// tools: 工具定义（可为 nil）
// 返回第一个成功的路由的响应；全部失败则返回 error
func callLLMWithFailover(messages []interface{}, tools []interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) (*LLMResult, error) {
	userID := convCtx.UserID
	username := convCtx.Username
	startTime := convCtx.StartTime
	failedKeys := make(map[string]bool)
	var lastError string

	for idx, route := range attempts {
		if failedKeys[routeIdentity(route)] {
			continue
		}
		// [DISABLED 2025-07-22] API key 大量熔断，暂时关闭熔断机制
		// if circuit.GetBreaker().IsOpen(route.ModelID, route.KeyID) {
		// 	log.Printf("[llm] circuit breaker open for %s/%s, skip", route.ModelID, route.KeyID)
		// 	continue
		// }

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = appendUnique(convCtx.Models, route.ModelID)
		convCtx.KeyNames = appendUnique(convCtx.KeyNames, route.KeyName)

		// 构造请求（始终非流式）：显式 stream=false，避免依赖上游默认值
		// （某些 OpenAI 兼容上游默认走流式，会返回 data:{...} 导致客户端 JSON 解析失败）
		llmReq := map[string]interface{}{
			"model":    route.ModelID,
			"messages": messages,
			"stream":   false,
		}
		// 注入 max_tokens（按厂商+模型精确配置）
		if route.MaxTokens > 0 {
			llmReq["max_tokens"] = route.MaxTokens
		}
		if len(tools) > 0 {
			llmReq["tools"] = tools
		}

		bodyJSON, _ := json.Marshal(llmReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		httpReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		httpReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")

		// 加总超时 context（60s）
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		httpReq = httpReq.WithContext(ctx)
		defer cancel()

		resp, err := service.SharedHTTPClient.Do(httpReq)
		if err != nil {
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username, Source: convCtx.Source,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
			LatencyMs: latency, Success: false, Error: err.Error(),
		})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			failedKeys[routeIdentity(route)] = true
			lastError = err.Error()
			continue
		}

	// 读完 body
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		errMsg := string(body)
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		service.RecordStat(&service.LLMStatType{
		UserID: userID, Username: username, Source: convCtx.Source,
		VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
		SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
		LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false, Error: errMsg,
	})
		convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
		failedKeys[routeIdentity(route)] = true
		lastError = errMsg
		continue
	}

		// 成功：解析 + 统计
		var respData map[string]interface{}
		json.Unmarshal(body, &respData)

		if usage, ok := respData["usage"].(map[string]interface{}); ok {
			promptTokens := intFloat(usage["prompt_tokens"])
			completionTokens := intFloat(usage["completion_tokens"])
			totalTokens := intFloat(usage["total_tokens"])
			// 厂商返回 0 时用 tiktoken 估算
			if promptTokens == 0 {
				promptTokens = estimatePromptTokensV2FromMessages(messages)
			}
			if completionTokens == 0 {
				completionTokens = estimateCompletionTokens(extractContentFromLLM(respData))
			}
			if totalTokens == 0 {
				totalTokens = promptTokens + completionTokens
			}
			service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username, Source: convCtx.Source,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			TotalTokens: totalTokens, LatencyMs: int(time.Since(startTime).Milliseconds()), Success: true,
		})
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		} else {
			// usage 缺失：用 tiktoken 估算
			promptTokens := estimatePromptTokensV2FromMessages(messages)
			completionTokens := estimateCompletionTokens(extractContentFromLLM(respData))
			totalTokens := promptTokens + completionTokens
			service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username, Source: convCtx.Source,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			TotalTokens: totalTokens, LatencyMs: int(time.Since(startTime).Milliseconds()), Success: true,
		})
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		}

		log.Printf("[llm] normal vendor=%s model=%s source=%s", route.VendorName, route.ModelID, source)
		return &LLMResult{
			Body:       body,
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Data:       respData,
			Route:      route,
		}, nil
	}

	return nil, fmt.Errorf("%s", lastError)
}

// StreamResult 流式调用的结果（callLLMStreamWithFailover 返回）
type StreamResult struct {
	Content   string                  // 累积的完整文本内容（已实时推给前端）
	ToolCalls []interface{}           // 累积的完整 tool_calls（按 index 组装）
	Route     *service.RouteInfoType  // 实际使用的路由
}

// callLLMStreamWithFailover 流式版 callLLMWithFailover
// 模仿 proxy.go 的全链路流式透传 + 故障转移通知体验：
//   - 对上游 stream=true，逐 chunk 透传给前端（真流式，非逐字假流式）
//   - 首字节前失败（连接/超时）：静默切换下一个 key
//   - 首字节后失败（SSE 中断/空闲超时）：推 event:progress 切换提示，标记 switched，切下一个 key
//   - 累积 content 和 tool_calls（按 index 累积分块 tool_calls）
//
// 用于 agentLoop，替代原来的非流式 callLLMWithFailover
func callLLMStreamWithFailover(w http.ResponseWriter, messages []interface{}, tools []interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext, toolChoice string) (*StreamResult, error) {
	userID := convCtx.UserID
	username := convCtx.Username
	startTime := convCtx.StartTime
	failedKeys := make(map[string]bool)
	switched := false // 是否发生过模型切换（用于决定是否推切换提示）

	var totalContent strings.Builder
	var lastError string

	for idx, route := range attempts {
		if failedKeys[routeIdentity(route)] {
			continue
		}
		// [DISABLED 2025-07-22] API key 大量熔断，暂时关闭熔断机制
		// if circuit.GetBreaker().IsOpen(route.ModelID, route.KeyID) {
		// 	log.Printf("[llm-stream] circuit breaker open for %s/%s, skip", route.ModelID, route.KeyID)
		// 	continue
		// }

		source := "策略路由"
		if idx > 0 || switched {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = appendUnique(convCtx.Models, route.ModelID)
		convCtx.KeyNames = appendUnique(convCtx.KeyNames, route.KeyName)

		// 构造流式请求
		llmReq := map[string]interface{}{
			"model":    route.ModelID,
			"messages": messages,
			"stream":   true,
		}
		if route.MaxTokens > 0 {
			llmReq["max_tokens"] = route.MaxTokens
		}
		if len(tools) > 0 {
			llmReq["tools"] = tools
			// tool_choice 透传（required=强制调工具 / auto=模型自由选）
			// 之前代理把这参数丢了，客户端发的 required 到上游变成默认 auto，
			// 模型可以只输出文本不调工具，导致 AgentRunner 端"纯文本重试循环"
			if toolChoice != "" {
				llmReq["tool_choice"] = toolChoice
			}
		}

		bodyJSON, _ := json.Marshal(llmReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		httpReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		httpReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := service.SharedHTTPClient.Do(httpReq)
		if err != nil {
			// 首字节前失败：静默切换（还没向前端推过任何内容）
			latency := int(time.Since(startTime).Milliseconds())
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username, Source: convCtx.Source,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			failedKeys[routeIdentity(route)] = true
			lastError = err.Error()
			log.Printf("[llm-stream] %s/%s 首字节前失败(静默切换): %s", route.VendorName, route.ModelID, err.Error())
			continue
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			errMsg := string(body)
			if len(errMsg) > 500 {
				errMsg = errMsg[:500]
			}
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username, Source: convCtx.Source,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
				LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			failedKeys[routeIdentity(route)] = true
			lastError = errMsg
			log.Printf("[llm-stream] %s/%s 首字节前 HTTP %d(静默切换)", route.VendorName, route.ModelID, resp.StatusCode)
			continue
		}

		// 连接成功，首字节即将到达
		log.Printf("[llm-stream] vendor=%s model=%s source=%s 开始流式读取", route.VendorName, route.ModelID, source)

		// 切换提示（发生过切换时，在本次流开始前推送通知，不污染 content）
		if switched {
			transitionMsg := fmt.Sprintf("⚡ 上游模型响应中断，已自动切换至 %s / %s 继续回答", route.VendorName, route.ModelID)
			streamProgressAsSSE(w, transitionMsg)
		}

		// 流式读取 + tool_calls 累积
		result, streamFailed := readAndForwardStream(w, resp.Body, route, convCtx, startTime, &totalContent)

		resp.Body.Close()

		if streamFailed {
			// 首字节后失败：记录 + 标记切换 + 继续尝试下一个 key
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username, Source: convCtx.Source,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
				LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
				Error: result.StreamError,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 流中断: %s", route.VendorName, result.StreamError))
			failedKeys[routeIdentity(route)] = true
			switched = true
			lastError = result.StreamError
			log.Printf("[llm-stream] %s/%s 流中断(切换): %s", route.VendorName, route.ModelID, result.StreamError)
			continue
		}

		// 流成功结束：统计 + 返回
		promptTokens := estimatePromptTokensV2FromMessages(messages)
		completionTokens := estimateCompletionTokens(totalContent.String())
		totalTokens := promptTokens + completionTokens
		service.RecordStat(&service.LLMStatType{
			UserID: userID, Username: username, Source: convCtx.Source,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			TotalTokens: totalTokens, LatencyMs: int(time.Since(startTime).Milliseconds()), Success: true,
		})
		convCtx.TotalPrompt += promptTokens
		convCtx.TotalCompletion += completionTokens

		log.Printf("[llm-stream] done vendor=%s model=%s content_len=%d tool_calls=%d",
			route.VendorName, route.ModelID, totalContent.Len(), len(result.ToolCalls))

		return &StreamResult{
			Content:   totalContent.String(),
			ToolCalls: result.ToolCalls,
			Route:     route,
		}, nil
	}

	return nil, fmt.Errorf("所有厂商均失败: %s", lastError)
}

// streamReadResult readAndForwardStream 的返回值
type streamReadResult struct {
	ToolCalls   []interface{} // 累积组装的完整 tool_calls
	StreamError string        // 非 空 时表示流读取失败（触发故障转移）
}

// readAndForwardStream 读取上游 SSE 流，逐 chunk 透传给前端，同时累积 content 和 tool_calls
// 模仿 proxy.go 的 SSE 读取逻辑（带空闲超时），但增加 tool_calls 分块累积
func readAndForwardStream(w http.ResponseWriter, body io.ReadCloser, route *service.RouteInfoType, convCtx *ConversationContext, startTime time.Time, totalContent *strings.Builder) (streamReadResult, bool) {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReaderSize(body, 64*1024)

	// tool_calls 累积器：按 index 分组
	type toolCallAccum struct {
		ID           string
		FunctionName string
		Arguments    strings.Builder
	}
	toolCallMap := make(map[int]*toolCallAccum)
	var orderedIndexes []int

	result := streamReadResult{}
	streamFailed := false

	for !streamFailed {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// 正常结束
				break
			}
			// 网络异常中断（非 EOF）
			result.StreamError = fmt.Sprintf("流读取异常: %v", err)
			streamFailed = true
			break
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" || strings.HasPrefix(lineStr, ":") {
			continue
		}
		if !strings.HasPrefix(lineStr, "data: ") {
			continue
		}
		data := lineStr[6:]
		if data == "[DONE]" {
			break
		}

		// 解析 chunk
		var chunk map[string]interface{}
		if e := json.Unmarshal([]byte(data), &chunk); e != nil {
			continue
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			// 可能是 usage 帧，跳过
			continue
		}
		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}
		delta, _ := choice["delta"].(map[string]interface{})
		if delta == nil {
			delta = map[string]interface{}{}
		}

		// 透传 content chunk 给前端（真流式）
		if content, ok := delta["content"].(string); ok && content != "" {
			// 调试：记录前 3 个 chunk 的内容（排查回车来源）
			if totalContent.Len() == 0 {
				log.Printf("[llm-stream] FIRST CHUNK %q (len=%d)", content, len(content))
			}
			totalContent.WriteString(content)
			// 透传给前端
			chunkOut := map[string]interface{}{
				"choices": []interface{}{map[string]interface{}{
					"delta": map[string]interface{}{"content": content},
					"index": 0,
				}},
			}
			chunkJSON, _ := json.Marshal(chunkOut)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkJSON))
			if flusher != nil {
				flusher.Flush()
			}
		}

		// 累积 tool_calls（按 index 分组）
		if tc, ok := delta["tool_calls"].([]interface{}); ok && len(tc) > 0 {
			for _, raw := range tc {
				tcItem, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				idx := 0
				if idxFloat, ok := tcItem["index"].(float64); ok {
					idx = int(idxFloat)
				}
				accum, exists := toolCallMap[idx]
				if !exists {
					accum = &toolCallAccum{}
					toolCallMap[idx] = accum
					orderedIndexes = append(orderedIndexes, idx)
				}
				if id, ok := tcItem["id"].(string); ok && id != "" {
					accum.ID = id
				}
				if fn, ok := tcItem["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						accum.FunctionName = name
					}
					if args, ok := fn["arguments"].(string); ok {
						accum.Arguments.WriteString(args)
					}
				}
			}
		}

		// 检查 finish_reason
		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			// 流结束（stop 或 tool_calls）
			break
		}
	}

	// 组装 tool_calls（按 index 顺序）
	if len(orderedIndexes) > 0 {
		var toolCalls []interface{}
		for _, idx := range orderedIndexes {
			accum := toolCallMap[idx]
			// arguments 必须是 JSON 字符串格式（OpenAI 兼容格式要求）
			// 不能解析成对象，否则下一轮发给上游时会报错：
			// "expected a string, but got {...} instead"
			argsStr := accum.Arguments.String()
			if argsStr == "" {
				argsStr = "{}"
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id": accum.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      accum.FunctionName,
					"arguments": argsStr, // 保持字符串格式
				},
			})
		}
		result.ToolCalls = toolCalls
	}

	return result, streamFailed
}

// toInterfaceSlice 将 []interface{} 或 []map[string]interface{} 统一转为 []interface{}
// 解决 req["messages"] 经 injectRAGContext 后类型变为 []map[string]interface{} 的断言问题
func toInterfaceSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	// reflect 兜底处理 []map[string]interface{} 等其他切片类型
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}
	return nil
}

// ── 公共：SSE 输出 ──

// writeSSEHeaders 设置 SSE 响应头
func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
}

// streamContentAsSSE 把完整文本内容以伪流式（逐字）输出为 SSE
func streamContentAsSSE(w http.ResponseWriter, content string) {
	for _, r := range content {
		chunk := map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{
				"delta": map[string]interface{}{"content": string(r)},
				"index": 0,
			}},
		}
		chunkJSON, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkJSON))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// streamProgressAsSSE 把中间进度信息（如 SQL trace、工具调用提示）作为独立 SSE 事件推送
// 用 event:progress 标记，与 data: 帧区分，前端单独处理不写入正文 content
// 避免进度信息的换行符污染最终回答（之前用 delta.content 推送导致每条消息前后多回车）
// SSE 头必须先由调用方 writeSSEHeaders 写入
func streamProgressAsSSE(w http.ResponseWriter, content string) {
	// 进度信息用 event:progress 帧推送，前端识别后单独展示，不混入 msg.content
	chunk := map[string]interface{}{
		"type":    "progress",
		"content": content,
	}
	chunkJSON, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "event: progress\ndata: %s\n\n", string(chunkJSON))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// outputContent 按客户端期望格式输出最终答案
func outputContent(w http.ResponseWriter, content string, modelID string, convCtx *ConversationContext) {
	if convCtx.IsStream {
		// 防止重复写 SSE 头（agent 循环中可能已写过）
		if !convCtx.SSEHeaderWritten {
			writeSSEHeaders(w)
			convCtx.SSEHeaderWritten = true
		}
		streamContentAsSSE(w, content)
	} else {
		response := map[string]interface{}{
			"id":     "chatcmpl-agent",
			"object": "chat.completion",
			"model":  modelID,
			"choices": []interface{}{map[string]interface{}{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     convCtx.TotalPrompt,
				"completion_tokens": convCtx.TotalCompletion,
				"total_tokens":      convCtx.TotalPrompt + convCtx.TotalCompletion,
			},
		}
		respJSON, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}
}

// ── 公共：响应提取辅助 ──

// extractToolCalls 从 LLM 响应中提取 tool_calls
func extractToolCalls(data map[string]interface{}) []interface{} {
	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil
	}
	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return nil
	}
	if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
		return toolCalls
	}
	return nil
}

// extractContentFromLLM 从 LLM 响应中提取 content
func extractContentFromLLM(data map[string]interface{}) string {
	choices, ok := data["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return ""
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return ""
	}
	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return ""
	}
	content, _ := msg["content"].(string)
	return content
}

// getToolCallName 从 tool_call 中提取函数名
func getToolCallName(tci interface{}) string {
	tc, ok := tci.(map[string]interface{})
	if !ok {
		return ""
	}
	fn, ok := tc["function"].(map[string]interface{})
	if !ok {
		return ""
	}
	name, _ := fn["name"].(string)
	return name
}

// getToolCallArgs 从 tool_call 中提取参数
func getToolCallArgs(tci interface{}) map[string]interface{} {
	tc, ok := tci.(map[string]interface{})
	if !ok {
		return nil
	}
	fn, ok := tc["function"].(map[string]interface{})
	if !ok {
		return nil
	}
	args := make(map[string]interface{})
	if argStr, ok := fn["arguments"].(string); ok {
		json.Unmarshal([]byte(argStr), &args)
	}
	return args
}

// getToolCallID 从 tool_call 中提取 id
func getToolCallID(tci interface{}) string {
	tc, ok := tci.(map[string]interface{})
	if !ok {
		return ""
	}
	id, _ := tc["id"].(string)
	return id
}
