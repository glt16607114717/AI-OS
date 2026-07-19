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
const sseIdleTimeout = 40 * time.Second

// ── 代理通道：纯透传（无工具时走这里，真流式零延迟）──

// proxyForward 纯透明代理
// 流式：SSE 逐行 pipe，带 60s 空闲超时 + 自动故障转移
// 非流式：JSON 透传
func proxyForward(w http.ResponseWriter, r *http.Request, req map[string]interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) {
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
			chatID := service.AddChatMessage(userID, "assistant", content, convCtx.SessionID, convCtx.MsgId)
			convCtx.ChatHistoryID = chatID
			latency := int(time.Since(startTime).Milliseconds())
			go service.SaveConversationLogWithUser(chatID, userID, username, req, content, route.ModelID, route.VendorID,
				convCtx.TotalPrompt, convCtx.TotalCompletion, 0, latency)
		}
		// 非流式响应：主动声明 application/json，不再裸透传上游 Header
		// （避免上游可能的 text/event-stream 污染客户端解析）
		w.Header().Set("Content-Type", "application/json")
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
		// 注入 max_tokens（按厂商+模型精确配置，实测上限值）
		// 不透传客户端的 max_tokens：客户端值未必贴合模型上限，且不同模型上限差异大
		if route.MaxTokens > 0 {
			llmReq["max_tokens"] = route.MaxTokens
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
				SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
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
				SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
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
		hasToolCalls := false
		reasoningChars := 0
		chunkCount := 0

		type readResult struct {
			line []byte
			err  error
		}
		lineCh := make(chan readResult, 1)
		go func() {
			defer close(lineCh)
			for {
				line, err := reader.ReadBytes('\n')
				lineCh <- readResult{line, err}
				if err != nil {
					return
				}
			}
		}()

		for !done && !streamFailed {
			var rr readResult
			select {
			case rr = <-lineCh:
			case <-time.After(sseIdleTimeout):
				log.Printf("[proxy] stream %s user=%s SSE_IDLE_TIMEOUT: %v 无输出，切换模型", route.ModelID, username, sseIdleTimeout)
				service.RecordStat(&service.LLMStatType{
					UserID: userID, Username: username,
					VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
					SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
					LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
					Error: fmt.Sprintf("SSE 流 %v 无输出，判定卡死", sseIdleTimeout),
				})
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s SSE 流 %v 无输出，判定卡死", route.VendorName, sseIdleTimeout))
				failedKeys[route.KeyID] = true
				switched = true
				streamFailed = true
				continue
			}

			if rr.err != nil {
				// EOF 时可能还携带最后一行数据（Go ReadBytes 行为：data+EOF 同时返回）
				if rr.err == io.EOF && len(rr.line) > 0 {
					// 处理 EOF 携带的最后一行
					lastLine := strings.TrimSpace(string(rr.line))
					if strings.HasPrefix(lastLine, "data: ") {
						lastData := lastLine[6:]
						if lastData == "[DONE]" {
							// 最后一行就是 [DONE]，正常结束
							if totalContent.Len() > 0 || hasToolCalls {
								done = true
							} else {
								log.Printf("[proxy] stream %s user=%s ZERO_DUMP: chunks=%d content=%d toolCalls=%v reasoning=%d", route.ModelID, username, chunkCount, totalContent.Len(), hasToolCalls, reasoningChars)
								log.Printf("[proxy] stream %s user=%s ZERO_CONTENT_DONE: EOF携带[DONE]但无内容，切换模型", route.ModelID, username)
								failedKeys[route.KeyID] = true
								switched = true
								streamFailed = true
							}
						} else {
							// 最后一行是数据 chunk，转发并累计
							fmt.Fprintf(w, "data: %s\n\n", lastData)
							if flusher != nil { flusher.Flush() }
							var chunk map[string]interface{}
							if json.Unmarshal([]byte(lastData), &chunk) == nil {
								if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
									if choice, ok := choices[0].(map[string]interface{}); ok {
										if delta, ok := choice["delta"].(map[string]interface{}); ok {
											if content, ok := delta["content"].(string); ok {
												totalContent.WriteString(content)
											}
											if tc, ok := delta["tool_calls"].([]interface{}); ok && len(tc) > 0 {
												hasToolCalls = true
											}
										}
									}
								}
							}
						}
						// EOF 处理完最后一行后，判断是否已有足够内容
						if done {
							break // 跳出 SSE 读取循环，正常结束
						}
						if totalContent.Len() > 0 || hasToolCalls {
							// 有内容但未收到 [DONE]：上游可能在末尾截断了 [DONE]
							// 内容已完整转发给客户端，当正常结束处理
							log.Printf("[proxy] stream %s user=%s SSE_EOF_WITH_CONTENT (%d 字符，无[DONE]但内容已转发，当正常结束)",
								route.ModelID, username, totalContent.Len())
							done = true
							break
						}
					}
				}

				if rr.err == io.EOF {
					// 纯 EOF，无内容（上面已处理有内容的情况）
					log.Printf("[proxy] stream %s user=%s SSE_EOF_WITHOUT_DONE (已有 %d 字符内容，切换模型重试)",
						route.ModelID, username, totalContent.Len())
					service.RecordStat(&service.LLMStatType{
						UserID: userID, Username: username,
						VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
						SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
						LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
						Error: fmt.Sprintf("SSE 流未收到 [DONE] 即关闭（已有 %d 字符内容）", totalContent.Len()),
					})
					convCtx.Errors = append(convCtx.Errors, "SSE 流未收到 [DONE] 即关闭")
					failedKeys[route.KeyID] = true
					switched = true
					streamFailed = true
					continue
				}

				// 非 EOF 的网络异常中断：切换模型重试
				service.RecordStat(&service.LLMStatType{
					UserID: userID, Username: username,
					VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
					SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
					LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
					Error: fmt.Sprintf("SSE 流异常中断: %s", rr.err.Error()),
				})
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("SSE 流异常中断: %s", rr.err.Error()))
				log.Printf("[proxy] stream %s user=%s SSE_ABORTED: %v", route.ModelID, username, rr.err)
				failedKeys[route.KeyID] = true
				switched = true
				streamFailed = true
				continue
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
				if totalContent.Len() == 0 && !hasToolCalls {
					// 0 内容输出 → 当失败，切换模型
					log.Printf("[proxy] stream %s user=%s ZERO_CONTENT_DONE: 收到[DONE]但无内容，切换模型", route.ModelID, username)
					service.RecordStat(&service.LLMStatType{
						UserID: userID, Username: username,
						VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
						SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
						LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false,
						Error: "收到[DONE]但无内容输出",
					})
					convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 收到[DONE]但无内容输出", route.VendorName))
					failedKeys[route.KeyID] = true
					switched = true
					streamFailed = true
					continue
				}
				done = true
			}

			// 转发给客户端（[DONE] 不转发，统一在最后发送）
			chunkCount++
			if data != "[DONE]" {
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
			}

			// 解析 chunk（用于统计 + 调试）
			var chunk map[string]interface{}
			if json.Unmarshal([]byte(data), &chunk) == nil {
				if u, ok := chunk["usage"].(map[string]interface{}); ok {
					usage = u
				}
				if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							// content 检查（正文）
							if content, ok := delta["content"].(string); ok {
								totalContent.WriteString(content)
							}
							// tool_calls 检查（工具调用，与 content 互斥，必须平级）
							if tc, ok := delta["tool_calls"].([]interface{}); ok && len(tc) > 0 {
								hasToolCalls = true
							}
							// reasoning_content 检查（思考过程，调试用）
							if rc, ok := delta["reasoning_content"].(string); ok && len(rc) > 0 {
								reasoningChars += len(rc)
							}
						}
					}
				}
			}
		}

		resp.Body.Close()

		if done && (totalContent.Len() > 0 || hasToolCalls) {
			// 正常结束：收到 [DONE] 且有内容
			finalRoute = route
			break
		}
		// 其他所有情况 → 继续外层循环尝试下一个模型
		if !streamFailed && !done {
			// SSE 循环异常退出（既没 DONE 也没报错），记录并切换
			log.Printf("[proxy] stream %s user=%s SSE_LOOP_EXIT: 异常退出，切换模型", route.ModelID, username)
			failedKeys[route.KeyID] = true
			switched = true
		}
	}

	// 成功时统一发送 [DONE]（流式读取时暂缓，避免过早结束客户端流）
	if finalRoute != nil {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
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
		// 厂商返回 0 时用 tiktoken 估算
		if promptTokens == 0 {
			promptTokens = estimatePromptTokensV2(req)
		}
		if completionTokens == 0 {
			completionTokens = estimateCompletionTokens(content)
		}
		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}
		convCtx.TotalPrompt += promptTokens
		convCtx.TotalCompletion += completionTokens
	} else {
		// usage 完全缺失：prompt 和 completion 都用 tiktoken 估算
		promptTokens = estimatePromptTokensV2(req)
		completionTokens = estimateCompletionTokens(content)
		totalTokens = promptTokens + completionTokens
	}

	latency := int(time.Since(startTime).Milliseconds())

	// 先保存 assistant 消息，获取 ChatHistoryID，再 RecordStat
	if content != "" {
		convCtx.AssistantContent = content
		chatID := service.AddChatMessage(userID, "assistant", content, convCtx.SessionID, convCtx.MsgId)
		convCtx.ChatHistoryID = chatID
		go service.SaveConversationLogWithUser(chatID, userID, username, req, content, finalRoute.ModelID, finalRoute.VendorID, promptTokens, completionTokens, totalTokens, latency)
	}

	service.RecordStat(&service.LLMStatType{
		UserID: userID, Username: username,
		VendorID: finalRoute.VendorID, KeyID: finalRoute.KeyID, ModelID: finalRoute.ModelID,
		SessionID: convCtx.SessionID, MsgID: convCtx.MsgId, ChatHistoryID: convCtx.ChatHistoryID,
		PromptTokens: promptTokens, CompletionTokens: completionTokens,
		TotalTokens: totalTokens, LatencyMs: latency, Success: true,
	})

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
