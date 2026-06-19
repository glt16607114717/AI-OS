package handler

import (
	"ai-os-server/service"
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── 代理通道：纯透传（无工具时走这里，真流式零延迟）──

// proxyForward 纯透明代理
// 流式：SSE 逐行 pipe；非流式：JSON 透传
// 不解析响应体，不拦截 tool_calls
func proxyForward(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) {
	userID := convCtx.UserID
	username := convCtx.Username
	startTime := convCtx.StartTime

	messages := toInterfaceSlice(req["messages"])
	isStream := convCtx.IsStream

	// 请求 LLM
	result, err := callLLMWithFailover(messages, nil, attempts, isStream, convCtx)
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
		scanner := bufio.NewScanner(streamResp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 支持 1MB 单行

		var totalContent strings.Builder
		var usage map[string]interface{}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := line[6:]
			if data == "[DONE]" {
				break
			}

			// 转发给客户端
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}

			// 解析 chunk（用于统计，但不影响转发）
			var chunk map[string]interface{}
			if json.Unmarshal([]byte(data), &chunk) == nil {
				if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						// 累计 content
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								totalContent.WriteString(content)
							}
						}
						// 检查 finish_reason
						if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
							if u, ok := chunk["usage"].(map[string]interface{}); ok {
								usage = u
							}
						}
					}
				}
				if u, ok := chunk["usage"].(map[string]interface{}); ok {
					usage = u
				}
			}
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
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
			// 估算
			completionTokens = len([]rune(content)) * 2 / 3
			if completionTokens == 0 {
				completionTokens = 1
			}
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
			chatID := service.AddChatMessage("assistant", content)
			convCtx.ChatHistoryID = chatID
			go service.SaveConversationLog(chatID, req, content, route.ModelID, route.VendorID, promptTokens, completionTokens, totalTokens, latency)
			go service.StoreEmbedding(userID, convCtx.UserMessage+"\n\n"+content, convCtx.Source)
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
		chatID := service.AddChatMessage("assistant", content)
		convCtx.ChatHistoryID = chatID
		latency := int(time.Since(startTime).Milliseconds())
		go service.SaveConversationLog(chatID, req, content, route.ModelID, route.VendorID,
			convCtx.TotalPrompt, convCtx.TotalCompletion, 0, latency)
		go service.StoreEmbedding(userID, convCtx.UserMessage+"\n\n"+content, convCtx.Source)
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
