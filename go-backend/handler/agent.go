package handler

import (
	"ai-os-server/service"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── Agent 通道：多轮 tool_calls 循环 ──

// 死循环检测：连续 N 轮 tool_calls 内容完全相同，判定为死循环
const stuckRoundsLimit = 3

// agentLoop Agent 循环（无轮数上限，仅靠死循环检测兜底）
// LLM 始终非流式请求，支持多轮 tool_calls（查表名→查结构→查数据→总结）
func agentLoop(w http.ResponseWriter, req map[string]interface{}, attempts []*service.RouteInfoType, convCtx *ConversationContext) {
	userID := convCtx.UserID
	username := convCtx.Username
	isAdmin := convCtx.IsAdmin
	startTime := convCtx.StartTime

	messages := toInterfaceSlice(req["messages"])
	tools := toInterfaceSlice(req["tools"])

	// 收集每轮执行的 SQL（最终展示给用户，保证查询可追溯）
	var sqlTrace []string

	// 流式请求：循环开始前就写 SSE 响应头，这样中间过程可以边执行边推送
	if convCtx.IsStream {
		writeSSEHeaders(w)
		convCtx.SSEHeaderWritten = true
	}

	// 死循环检测：记录上一轮 tool_calls 的签名
	var lastToolCallSig string
	stuckCount := 0

	log.Printf("[agent] start user=%s tools=%d", username, len(tools))

	for round := 0; ; round++ {
		// 1. 请求 LLM（非流式，带故障转移）
		result, err := callLLMWithFailover(messages, tools, attempts, false, convCtx)
		if err != nil {
			convCtx.SummarizeAndLog()
			errResponse(w, fmt.Sprintf("所有厂商均失败: %s", err.Error()), 502)
			return
		}

		// 2. 检查 tool_calls
		toolCalls := extractToolCalls(result.Data)

		if len(toolCalls) == 0 {
			// 3a. 最终答案
			content := extractContentFromLLM(result.Data)
			if content == "" {
				content = "（模型未返回内容）"
			}
			log.Printf("[agent] done user=%s rounds=%d latency=%dms content_len=%d",
				username, round+1, time.Since(startTime).Milliseconds(), len(content))
			finishAgent(w, req, content, result.Route, convCtx, sqlTrace)
			return
		}

		// 3b. 有 tool_calls
		convCtx.ToolCallCount++

		// 死循环检测：计算本轮 tool_calls 签名（函数名+参数 JSON）
		var sigParts []string
		for _, tci := range toolCalls {
			args := getToolCallArgs(tci)
			argsJSON, _ := json.Marshal(args)
			sigParts = append(sigParts, getToolCallName(tci)+":"+string(argsJSON))
		}
		currentSig := strings.Join(sigParts, "|")

		if currentSig == lastToolCallSig {
			stuckCount++
			if stuckCount >= stuckRoundsLimit {
				log.Printf("[agent] stuck loop detected user=%s round=%d (same tool_calls %d times)", username, round+1, stuckCount)
				// 强制总结（不带 tools）
				forceResult, forceErr := callLLMWithFailover(messages, nil, attempts, false, convCtx)
				if forceErr != nil {
					finishAgent(w, req, "检测到查询陷入循环，且总结失败。请尝试换一种问法。", nil, convCtx, sqlTrace)
					return
				}
				content := extractContentFromLLM(forceResult.Data)
				if content == "" {
					content = "（模型未返回内容）"
				}
				log.Printf("[agent] stuck summary done user=%s content_len=%d", username, len(content))
				finishAgent(w, req, content, forceResult.Route, convCtx, sqlTrace)
				return
			}
		} else {
			stuckCount = 0
			lastToolCallSig = currentSig
		}

		log.Printf("[agent] round=%d tool_calls=%d", round+1, len(toolCalls))

		// 检查是否有非 skill_ 工具（TRAE 自带工具）
		hasNonSkill := false
		for _, tci := range toolCalls {
			if name := getToolCallName(tci); !strings.HasPrefix(name, "skill_") {
				hasNonSkill = true
				break
			}
		}

		if hasNonSkill {
		// 透传给 TRAE 执行
		log.Printf("[agent] proxy tool_calls to client (non-skill)")
		proxyToolCalls(w, toolCalls, convCtx)
		convCtx.SummarizeAndLog()
		return
	}

		// 执行所有 skill_ 工具
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    "",
			"tool_calls": toolCalls,
		})

		for _, tci := range toolCalls {
			name := getToolCallName(tci)
			skillCode := strings.TrimPrefix(name, "skill_")
			args := getToolCallArgs(tci)
			tcID := getToolCallID(tci)

			// 流式请求时，先把"即将调用什么技能"告知客户端
			if convCtx.SSEHeaderWritten {
				streamProgressAsSSE(w, fmt.Sprintf("\n[调用技能 %s...]\n", skillCode))
			}

			result, execErr := service.ExecuteBuiltinSkill(skillCode, userID, isAdmin, args)

			// 通用收集 trace（任何技能只要返回 trace 字段就展示）
			if execErr == nil && result != nil {
				if trace, ok := result["trace"].(string); ok && trace != "" {
					sqlTrace = append(sqlTrace, trace)
					// 流式请求时立即推送 trace，让用户实时看到执行过程
					if convCtx.SSEHeaderWritten {
						streamProgressAsSSE(w, trace+"\n")
					}
				}
			}

			var contentStr string
			if execErr != nil {
				log.Printf("[agent] skill=%s error: %s", skillCode, execErr.Error())
				errJSON, _ := json.Marshal(map[string]interface{}{"error": execErr.Error()})
				contentStr = string(errJSON)
			} else {
				resultJSON, _ := json.Marshal(result)
				contentStr = string(resultJSON)
				log.Printf("[agent] skill=%s ok result_len=%d", skillCode, len(contentStr))
			}

			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"content":      contentStr,
				"tool_call_id": tcID,
			})
		}
		// 继续循环，让 LLM 看到工具结果后决定下一步
	}
}

// finishAgent 统一处理 Agent 结束：拼接 trace → 保存 → 输出
// route 为 nil 时使用空 modelID（兜底场景）
func finishAgent(w http.ResponseWriter, req map[string]interface{}, content string, route *service.RouteInfoType, convCtx *ConversationContext, sqlTrace []string) {
	// 流式请求：trace 已在 agentLoop 中实时推送，不再拼接
	// 非流式请求：保持原行为，把 trace 拼到答案前面
	if !convCtx.IsStream {
		if len(sqlTrace) > 0 {
			content = strings.Join(sqlTrace, "\n\n") + "\n\n" + content
		}
	}

	modelID := ""
	vendorID := 0
	if route != nil {
		modelID = route.ModelID
		vendorID = route.VendorID
	}

	// 保存
	convCtx.AssistantContent = content
	chatID := service.AddChatMessage(convCtx.UserID, "assistant", content)
	convCtx.ChatHistoryID = chatID
	latency := int(time.Since(convCtx.StartTime).Milliseconds())
	go service.SaveConversationLog(chatID, req, content, modelID, vendorID,
		convCtx.TotalPrompt, convCtx.TotalCompletion, 0, latency)

	// 输出
	outputContent(w, content, modelID, convCtx)
	convCtx.SummarizeAndLog()
}

// proxyToolCalls 把非 skill_ 工具的 tool_calls 透传给客户端（TRAE 自己执行）
func proxyToolCalls(w http.ResponseWriter, toolCalls []interface{}, convCtx *ConversationContext) {
	tcID := getToolCallID(toolCalls[0])
	isStream := convCtx.IsStream

	if isStream {
		// 防止重复写 SSE 头（agent 循环中可能已写过）
		if !convCtx.SSEHeaderWritten {
			writeSSEHeaders(w)
			convCtx.SSEHeaderWritten = true
		}

		// tool_calls 帧
		chunk := map[string]interface{}{
			"id":      tcID,
			"object":  "chat.completion.chunk",
			"choices": []interface{}{map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"role":       "assistant",
					"tool_calls": toolCalls,
				},
				"finish_reason": nil,
			}},
		}
		chunkJSON, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkJSON))

		// finish 帧
		fmt.Fprintf(w, "data: {\"id\":\"%s\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n", tcID)
		fmt.Fprintf(w, "data: [DONE]\n\n")

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		response := map[string]interface{}{
			"id":      "chatcmpl-agent",
			"object":  "chat.completion",
			"choices": []interface{}{map[string]interface{}{
				"index": 0,
				"message": map[string]interface{}{
					"role":       "assistant",
					"content":    "",
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			}},
		}
		respJSON, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}
}
