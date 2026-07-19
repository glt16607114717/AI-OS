package service

import (
	"ai-os-server/model"
	"log"
	"strings"
)

// ── ZCode 客户端专属提示词优化 ──
//
// 与 Trae 完全不同的噪音模型，物理隔离，互不影响：
//   - ZCode 无 <system-reminder>/<hooks_context> XML 标签（不需要 stripNoise/stripHooksContext）
//   - ZCode 每条 assistant 携带 reasoning_content（思维链），历史轮冗余（最大金矿）
//   - ZCode 工具集（Bash/Edit/EnterPlanMode 等）与 Trae（RunCommand/Task）完全不同
//
// 安全保障（与 Trae 共享的链路都不受影响）：
//   - system 消息透传：保护上帝指令/RAG/角色定位
//   - user 消息透传：图片识别(ProcessImages)、trace 剥离(extractAndStripTrace) 已在 OptimizeMessages 之前完成
//   - tool 消息只截断内容：保留 tool_call_id，不破坏 tool_calls 链路

// optimizeZCodeMessages ZCode 客户端的 messages 优化
// 返回优化后的 messages 和删除的 reasoning_content 计数
func optimizeZCodeMessages(messages []map[string]interface{}, cfg *model.GodRulesConfig) ([]map[string]interface{}, int) {
	if cfg == nil {
		return messages, 0
	}

	// ── 20 轮截断（ZCode 编辑器无限累加的防护）──
	// 与 Trae 链路共用同一阈值和逻辑（公共函数 truncateByRounds）
	// 阈值依据：20 轮之前的对话与当前任务几乎无关，重要节点应通过 task log 记录而非依赖上下文
	messages = truncateByRounds(messages, 20, "optimizeZCode")

	// 找第 10 条 user 消息的索引（与 Trae 共享同一阈值）
	// 此索引及之后的 tool 结果不截断，保护近期上下文
	keepFromIdx := 0
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			userCount++
			if userCount == 10 {
				keepFromIdx = i
				break
			}
		}
	}

	result := make([]map[string]interface{}, len(messages))
	strippedReasoning := 0

	for i, msg := range messages {
		role, _ := msg["role"].(string)

		switch role {
		case "assistant":
			// 历史轮删 reasoning_content（思维链），保留 content 和 tool_calls
			// 最新一条 assistant 透传（保留 reasoning_content 供当轮使用）
			if isLatestAssistant(messages, i) {
				result[i] = msg
			} else {
				// 浅拷贝后删除 reasoning_content，避免改原消息（保护其他链路的内存引用）
				newMsg := make(map[string]interface{}, len(msg))
				for k, v := range msg {
					newMsg[k] = v
				}
				if _, hasReason := newMsg["reasoning_content"]; hasReason {
					delete(newMsg, "reasoning_content")
					strippedReasoning++
				}
				result[i] = newMsg
			}

		case "tool":
			// 防御性清 hooks_context（ZCode 当前无此标签，no-op，保留以防未来变更）
			// 按轮次截断 tool 结果（与 Trae 共享阈值），保留 tool_call_id
			content := StringifyContent(msg["content"])
			if cfg.StripNoise {
				content = stripHooksContext(content)
			}
			if cfg.CompressToolResult && i < keepFromIdx && len(content) > 200 {
				content = content[:200] + "\n[...结果已截断，如需完整内容请重新执行该工具]"
			}
			result[i] = map[string]interface{}{
				"role":         "tool",
				"content":      content,
				"tool_call_id": msg["tool_call_id"],
			}

		default:
			// system / user / 其他：一律透传原消息
			// - system 保护上帝指令/RAG/角色定位，不做 StringifyContent（避免格式转换）
			// - user 保护图片识别成果和 trace 剥离成果，不做任何清理
			result[i] = msg
		}
	}

	if strippedReasoning > 0 {
		log.Printf("[optimizeZCode] stripped reasoning_content from %d/%d assistant messages",
			strippedReasoning, len(messages))
	}
	return result, strippedReasoning
}

// isLatestAssistant 判断 msg[idx] 是否为最后一条 assistant 消息
// 最后一条 assistant 的 reasoning_content 保留（供当轮使用），历史轮才删
func isLatestAssistant(messages []map[string]interface{}, idx int) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "assistant" {
			return i == idx
		}
	}
	return false
}

// optimizeZCodeTools ZCode 客户端的工具描述精简
// 针对描述最长的 4 个工具按段落精确裁剪（与 Trae 的 RunCommand/Task 精简互不影响）
//
// 裁剪对象（实测 conversation_7683.json）：
//   - EnterPlanMode(4030字符)：删 ## Examples 整段（5 个 GOOD + 3 个 BAD 示例）
//   - ExitPlanMode(1917字符)：删 ## Examples 整段（3 个示例）
//   - Agent(1946字符)：删 ## When to use 整段（AI 看工具名就知道何时委派）
//   - AskUserQuestion(1786字符)：删 Plan mode note + Preview feature 两段
//
// 保留：每个工具的核心功能描述（第一段）+ 使用条件列表（不含示例）
func optimizeZCodeTools(tools []interface{}, cfg *model.GodRulesConfig) []interface{} {
	result := make([]interface{}, len(tools))
	for i, raw := range tools {
		t, ok := raw.(map[string]interface{})
		if !ok {
			result[i] = raw
			continue
		}
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			result[i] = raw
			continue
		}
		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)

		switch name {
		case "EnterPlanMode":
			// 删除 "## Examples" 整段（5 GOOD + 3 BAD 示例，AI 看说明就懂）
			if idx := strings.Index(desc, "## Examples"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc

		case "ExitPlanMode":
			// 删除 "## Examples" 整段（3 个示例）
			if idx := strings.Index(desc, "## Examples"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc

		case "Agent":
			// 删除 "## When to use" 整段（AI 看工具名就知道何时委派）
			if idx := strings.Index(desc, "## When to use"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc

		case "AskUserQuestion":
			// 删除 "Plan mode note" 段（EnterPlanMode 工具已说明，此处冗余）
			if idx := strings.Index(desc, "Plan mode note:"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			// 删除 "Preview feature" 段（AI 看参数描述就懂，不需要教程）
			if idx := strings.Index(desc, "Preview feature:"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc
		}

		result[i] = t
	}
	return result
}
