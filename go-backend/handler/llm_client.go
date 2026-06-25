package handler

import (
	"ai-os-server/service"
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
		if failedKeys[route.KeyID] {
			continue
		}

		source := "策略路由"
		if idx > 0 {
			source = "故障转移"
			convCtx.FailoverCount++
		}
		convCtx.Models = appendUnique(convCtx.Models, route.ModelID)
		convCtx.KeyNames = appendUnique(convCtx.KeyNames, route.KeyName)

		// 构造请求（始终非流式）
		llmReq := map[string]interface{}{
			"model":    route.ModelID,
			"messages": messages,
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
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: latency, Success: false, Error: err.Error(),
			})
			convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s 网络错误: %s", route.VendorName, err.Error()))
			failedKeys[route.KeyID] = true
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
			UserID: userID, Username: username,
			VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
			LatencyMs: int(time.Since(startTime).Milliseconds()), Success: false, Error: errMsg,
		})
		convCtx.Errors = append(convCtx.Errors, fmt.Sprintf("%s HTTP %d: %s", route.VendorName, resp.StatusCode, errMsg))
		failedKeys[route.KeyID] = true
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
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				TotalTokens: totalTokens, LatencyMs: int(time.Since(startTime).Milliseconds()), Success: true,
			})
			convCtx.TotalPrompt += promptTokens
			convCtx.TotalCompletion += completionTokens
		} else {
			service.RecordStat(&service.LLMStatType{
				UserID: userID, Username: username,
				VendorID: route.VendorID, KeyID: route.KeyID, ModelID: route.ModelID,
				LatencyMs: int(time.Since(startTime).Milliseconds()), Success: true,
			})
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

// streamProgressAsSSE 把中间进度信息（如 SQL trace、工具调用提示）作为 SSE 帧推送
// 用于 agent 多轮工具调用时实时向客户端展示执行过程
// SSE 头必须先由调用方 writeSSEHeaders 写入
func streamProgressAsSSE(w http.ResponseWriter, content string) {
	chunk := map[string]interface{}{
		"object": "chat.completion.chunk",
		"choices": []interface{}{map[string]interface{}{
			"index": 0,
			"delta": map[string]interface{}{
				"content": content,
			},
		}},
	}
	chunkJSON, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", string(chunkJSON))
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
