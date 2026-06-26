package handler

import (
	"ai-os-server/circuit"
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

// sseIdleTimeout SSE 流中两个 chunk 之间的最大空闲时间
// 超过此时间无新 chunk 输出，视为上游卡死，切换到下一个模型
const sseIdleTimeout = 60 * time.Second

// estimatePromptTokens 从请求 messages 中估算 prompt token 数
func estimatePromptTokens(req map[string]interface{}) int {
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return 0
	}
	totalChars := 0
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		totalChars += len([]rune(extractContent(msg["content"])))
	}
	tokens := totalChars * 2 / 3
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// ── 代理通道：纯透传（无工具时走这里，真流式零延迟）──

// proxyForward 纯透明代理
// 流式：SSE 逐行 pipe，带 60s 空闲超时 + 自动故障转移
// 非流式：JSON 透传
func proxyForward(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) {
	userID := convCtx.UserID
	username := convCtx.Username
	startTime := convCtx.StartTime

	messages := toInterfaceSlice(req["messages"])
	clientTools := toInterfaceSlice(req["tools"])
	isStream := convCtx.IsStream

	// 非流式：走原有逻辑
	if !isStream {
		result, err := callLLMWithFailover(messages, clientTools, attempts, convCtx)
		if err != nil {
			convCtx.SummarizeAndLog()
			errResponse(w, fmt.Sprintf("所有厂商均失败: %s", err.Error()), 502)
			return
		}
		route := result.Route
		content := extractContentFromLLM(result.Data)
		if content != "" {
			convCtx.AssistantContent = content
			chatID := service.AddChatMessage(userID, "assistant", content)
			convCtx.ChatHistoryID = chatID
			latency := int(time.Since(startTime).Milliseconds())
			go service.SaveConversationLogWithUser(chatID, userID, username, req, content, route.ModelID, route.VendorID,
				convCtx.TotalPrompt, convCtx.TotalCompletion, 0, latency)
		}
		for k, v := range result.Header {
			if k != "Content-Length" {
				for _, vv := range v {
					w.Header().Add(k, vv)
				}
			}
		}
		w.WriteHeader(result.StatusCode)
		w.Write(result.Body)
		log.Printf("[proxy] normal %s user=%s latency=%dms",
			route.ModelID, username, time.Since(startTime).Milliseconds())
		convCtx.SummarizeAndLog()
		return
	}

	// ── 流式：SSE 读取 + 空闲超时 + 故障转移 ──

	writeSSEHeaders(w)
	flusher, _ := w.(http.Flusher)

	var totalContent strings.Builder
	var usage map[string]interface{}
	failedKeys := make(map[string]bool)
	var finalRoute *service.RouteInfoType
	switched := false // 是否发生过模型切换

	for idx, route := range attempts {
		if failedKeys[route.KeyID] {
			continue
		}
		// 熔断检查：独立进程已标记该 "模型+Key" 不可用
		if circuit.GetBreaker().IsOpen(route.ModelID, route.KeyID) {
			log.Printf("[proxy] circuit breaker open for %s/%s, skip", route.ModelID, route.KeyID)
			continue
		}

		source := "策略路由"
		if idx > 0 || switched {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = appendUnique(convCtx.Models, route.ModelID)
		convCtx.KeyNames = appendUnique(convCtx.KeyNames, route.KeyName)

		// 构造请求
		llmReq := map[string]interface{}{
			"model":    route.ModelID,
			"messages": messages,
			"stream":   true,
		}
		if len(clientTools) > 0 {
			llmReq["tools"] = clientTools
		}

		bodyJSON, _ := json.Marshal(llmReq)
		baseURL := strings.TrimRight(route.BaseURL, "/")
		httpReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", strings.NewReader(string(bodyJSON)))
		httpReq.Header.Set("Authorization", "Bearer "+route.APIKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := service.SharedHTTPClient.Do(httpReq)
		if err != nil {
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			failedKeys[route.KeyID] = true
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false, Error: errMsg,
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
			failedKeys[route.KeyID] = true
			continue
		}

		// 连接成功，开始读 SSE 流
		log.Printf("[proxy] stream vendor=%s model=%s source=%s", route.VendorName, route.ModelID, source)

		// 切换提示（非首个模型时插入）
		if switched {
			transitionMsg := fmt.Sprintf("\n\n---\n> ⚡ %s 响应中断，已自动切换至 **%s**（%s）继续回答\n\n",
				"上游模型", route.VendorName, route.ModelID)
			fmt.Fprintf(w, "data: %s\n\n", mustMarshalSSEChunk(transitionMsg))
			if flusher != nil {
				flusher.Flush()
			}
		}

		// SSE 读取循环（带空闲超时）
		reader := bufio.NewReaderSize(resp.Body, 64*1024)
		done := false
		streamFailed := false

		for !done && !streamFailed {
			// 空闲超时：用 goroutine + channel 实现
			type readResult struct {
				line []byte
				err  error
			}
			readCh := make(chan readResult, 1)
			go func() {
				line, err := reader.ReadBytes('\n')
				readCh <- readResult{line, err}
			}()

			var rr readResult
			select {
			case rr = <-readCh:
				// 正常收到数据
			case <-time.After(sseIdleTimeout):
				// 60 秒无输出 → 判定卡死
				log.Printf("[proxy] stream %s user=%s SSE_IDLE_TIMEOUT: 60s 无输出，切换模型", route.ModelID, username)
				service.RecordStat(&service.LLMStatType{
					UserID: userID, Username: username,
					VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
					LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
					Error: fmt.Sprintf("SSE 流 60s 无输出，判定卡死"),
				})
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s SSE 流 60s 无输出，判定卡死", route.VendorName))
				failedKeys[route.KeyID] = true
				switched = true
				streamFailed = true
				continue
			}

			if rr.err != nil {
				// 连接断开（EOF 或网络错误）
				if rr.err != io.EOF {
					service.RecordStat(&service.LLMStatType{
						UserID: userID, Username: username,
						VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
						LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
						Error: fmt.Sprintf("SSE 流异常中断: %s", rr.err.Error()),
					})
					convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("SSE 流异常中断: %s", rr.err.Error()))
					log.Printf("[proxy] stream %s user=%s SSE_ABORTED: %v", route.ModelID, username, rr.err)
				} else {
					service.RecordStat(&service.LLMStatType{
						UserID: userID, Username: username,
						VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
						LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
						Error: "SSE 流未收到 [DONE] 即关闭",
					})
					convCtx.Errors = append(convCtx.Errors, "SSE 流未收到 [DONE] 即关闭")
					log.Printf("[proxy] stream %s user=%s SSE_EOF_WITHOUT_DONE", route.ModelID, username)
				}
				// 连接断开 ≠ 卡死，不切换模型，直接结束
				break
			}

			lineStr := strings.TrimSpace(string(rr.line))
			if lineStr == "" {
				continue
			}
			if !strings.HasPrefix(lineStr, "data: ") {
				continue
			}
			data := lineStr[6:]
			if data == "[DONE]" {
				done = true
			}

			// 转发给客户端
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}

			// 解析 chunk（用于统计）
			var chunk map[string]interface{}
			if json.Unmarshal([]byte(data), &chunk) == nil {
				if u, ok := chunk["usage"].(map[string]interface{}); ok {
					usage = u
				}
				if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								totalContent.WriteString(content)
							}
						}
					}
				}
			}
		}

		resp.Body.Close()

		if done {
			// 正常结束
			finalRoute = route
			break
		}
		// streamFailed=true 时继续外层循环尝试下一个模型
		// 连接断开（非卡死）时也结束，不切换
		if !streamFailed {
			break
		}
	}

	// ── 所有模型都失败 ──
	if finalRoute == nil {
		errChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "所有可用模型均响应失败，请稍后重试",
				"type":    "all_models_failed",
			},
		}
		errJSON, _ := json.Marshal(errChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(errJSON))
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		convCtx.SummarizeAndLog()
		return
	}

	// ── 统计 ──
	content := totalContent.String()
	promptTokens, completionTokens, totalTokens := 0, 0, 0
	if usage != nil {
		promptTokens = intFloat(usage["prompt_tokens"])
		completionTokens = intFloat(usage["completion_tokens"])
		totalTokens = intFloat(usage["total_tokens"])
		if promptTokens == 0 {
			promptTokens = estimatePromptTokens(req)
		}
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
		convCtx.TotalPrompt += promptTokens
		convCtx.TotalCompletion += completionTokens
	} else {
		completionTokens = len([]rune(content)) * 2 / 3
		if completionTokens == 0 {
			completionTokens = 1
		}
		promptTokens = estimatePromptTokens(req)
		totalTokens = promptTokens + completionTokens
	}

	latency := int(time.Since(startTime).Milliseconds())
	service.RecordStat(&service.LLMStatType{
		UserID: userID, Username: username,
		VendorID: finalRoute.VendorID, KeyID: finalRoute.KeyID, ModelID: finalRoute.ModelID,
		PromptTokens: promptTokens, CompletionTokens: completionTokens,
		TotalTokens: totalTokens, LatencyMs: latency, Success: true,
	})

	if content != "" {
		convCtx.AssistantContent = content
		chatID := service.AddChatMessage(userID, "assistant", content)
		convCtx.ChatHistoryID = chatID
		go service.SaveConversationLogWithUser(chatID, userID, username, req, content, finalRoute.ModelID, finalRoute.VendorID, promptTokens, completionTokens, totalTokens, latency)
	}

	log.Printf("[proxy] stream %s user=%s latency=%dms tokens=%d/%d failover=%d",
		finalRoute.ModelID, username, latency, promptTokens, completionTokens, convCtx.FailoverCount)

	convCtx.SummarizeAndLog()
}

// mustMarshalSSEChunk 将文本内容序列化为 SSE chunk JSON
func mustMarshalSSEChunk(text string) string {
	chunk := map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"delta": map[string]interface{}{"content": text},
			"index": 0,
		}},
	}
	b, _ := json.Marshal(chunk)
	return string(b)
}
