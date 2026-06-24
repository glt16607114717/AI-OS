package handler

import (
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

// estimatePromptTokens 从请求 messages 中估算 prompt token 数
// 估算公式：len(rune) * 2 / 3（中文约 1.5 字/token，与 completionTokens 估算一致）
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
// 流式：SSE 逐行 pipe；非流式：JSON 透传
// 不解析响应体，不拦截 tool_calls
func proxyForward(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) {
	userID := convCtx.UserID
	username := convCtx.Username
	startTime := convCtx.StartTime

	messages := toInterfaceSlice(req["messages"])
	clientTools := toInterfaceSlice(req["tools"]) // 客户端自带的工具（LS/Read/Grep 等），透传给大模型
	isStream := convCtx.IsStream

	// 请求 LLM（透传客户端工具，不注入服务端技能）
	result, err := callLLMWithFailover(messages, clientTools, attempts, isStream, convCtx)
	if err != nil {
		convCtx.SummarizeAndLog()
		errResponse(w, fmt.Sprintf("所有厂商均失败: %s", err.Error()), 502)
		return
	}

	route := result.Route

	// 流式：逐行读取上游 SSE 并转发给客户端
	if isStream {
		streamResp := getStreamResp(result)
		if streamResp == nil {
			convCtx.SummarizeAndLog()
			errResponse(w, "流式响应异常", 502)
			return
		}
		defer streamResp.Body.Close()

		writeSSEHeaders(w)

		flusher, _ := w.(http.Flusher)
		reader := bufio.NewReaderSize(streamResp.Body, 64*1024)

		var totalContent strings.Builder
		var usage map[string]interface{}

		done := false
		var readErr error
		for {
			line, err := reader.ReadBytes('\n')
			readErr = err
			lineStr := strings.TrimSpace(string(line))
			if lineStr != "" {
				if !strings.HasPrefix(lineStr, "data: ") {
					goto nextLine
				}
				data := lineStr[6:]
				if data == "[DONE]" {
					done = true
				}

				// 转发给客户端（[DONE] 也要转发）
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}

				// 解析 chunk（用于统计，但不影响转发）
				var chunk map[string]interface{}
				if json.Unmarshal([]byte(data), &chunk) == nil {
					// 提取 usage（任何 chunk 都可能含 usage）
					if u, ok := chunk["usage"].(map[string]interface{}); ok {
						usage = u
					}
					if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]interface{}); ok {
							// 累计 content
							if delta, ok := choice["delta"].(map[string]interface{}); ok {
								if content, ok := delta["content"].(string); ok {
									totalContent.WriteString(content)
								}
							}
						}
					}
				}

				// [DONE] 块已处理完，退出循环（确保不漏掉 [DONE] 之后的 chunk）
				if done {
					break
				}
			}
		nextLine:
			if err != nil {
				break
			}
		}

		// SSE 流中断检测：区分正常结束和异常中断
		if !done {
			if readErr != nil && readErr != io.EOF {
				// 流被异常中断（网络错误、上游超时等）
				convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("SSE 流异常中断: %s", readErr.Error()))
				log.Printf("[proxy] stream %s user=%s SSE_ABORTED: %v", route.ModelID, username, readErr)
			} else if readErr == io.EOF {
				// 上游关闭连接但未发送 [DONE]，内容可能不完整
				convCtx.Errors = append(convCtx.Errors, "SSE 流未收到 [DONE] 即关闭")
				log.Printf("[proxy] stream %s user=%s SSE_EOF_WITHOUT_DONE", route.ModelID, username)
			}
		}

		// 确保客户端收到 [DONE]
		if !done {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}

		// 统计
		content := totalContent.String()
		promptTokens, completionTokens, totalTokens := 0, 0, 0
		if usage != nil {
			promptTokens = intFloat(usage["prompt_tokens"])
			completionTokens = intFloat(usage["completion_tokens"])
			totalTokens = intFloat(usage["total_tokens"])
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		} else {
			// 估算（上游未返回 usage 时）
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
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			PromptTokens: promptTokens, CompletionTokens: completionTokens,
			TotalTokens: totalTokens, LatencyMs: latency, Success: true,
		})

		if content != "" {
			convCtx.AssistantContent = content
			chatID := service.AddChatMessage(userID, "assistant", content)
			convCtx.ChatHistoryID = chatID
			go service.SaveConversationLogWithUser(chatID, userID, username, req, content, route.ModelID, route.VendorID, promptTokens, completionTokens, totalTokens, latency)
		}

		log.Printf("[proxy] stream %s user=%s latency=%dms tokens=%d/%d",
			route.ModelID, username, latency, promptTokens, completionTokens)

		convCtx.SummarizeAndLog()
		return
	}

	// 非流式：透传 body
	content := extractContentFromLLM(result.Data)
	if content != "" {
		convCtx.AssistantContent = content
		chatID := service.AddChatMessage(userID, "assistant", content)
		convCtx.ChatHistoryID = chatID
		latency := int(time.Since(startTime).Milliseconds())
		go service.SaveConversationLogWithUser(chatID, userID, username, req, content, route.ModelID, route.VendorID,
			convCtx.TotalPrompt, convCtx.TotalCompletion, 0, latency)
	}

	// 透传响应头和 body
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
}
