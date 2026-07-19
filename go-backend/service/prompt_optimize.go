package service

import (
	"ai-os-server/model"
	"log"
	"regexp"
	"strings"
)

// ── 提示词优化：发往 LLM 前的 messages 清洗 ──
//
// 作用：在 injectGodRulesAndRAG 内、转发给大模型之前调用。
// 只改发往 LLM 的 messages，不改原始留存（conversation_*.json 仍是完整 dump）。
//
// 四刀：
//   1) stripNoise        - 历史轮 user 消息里删除 path/terminal/lang 三类 system-reminder，
//                          保留 other 类（首轮 rules/LS/skill 等）和 user_input 真实内容
//   2) stripHooksContext  - 所有消息（含最新轮）删除 hooks_context 块（Trae IDE hook 事件壳子）
//   3) compressToolResult - 10 轮之前的 tool 结果超 200 字符截断
//   4) compressTools      - 精简 tools 数组中 RunCommand/Task 的冗长描述

// 预编译正则
var (
	// 成对 system-reminder 块（含换行）
	reSysReminder = regexp.MustCompile(`(?s)\s*<system-reminder>.*?</system-reminder>\s*`)
	// 孤立标签残片
	reOrphanReminder = regexp.MustCompile(`</?system-reminder>`)
	// hooks_context 块（Trae IDE hook 事件壳子，纯噪音，所有消息通用）
	reHooksContext = regexp.MustCompile("(?s)\x3chooks_context\x3e.*?\x3c/hooks_context\x3e")
)

// OptimizeMessages 对发往 LLM 的 messages 做噪音清理（按用户配置，默认全开）
// 分流入口：根据客户端类型走不同的优化逻辑
//   - ZCode 客户端 → optimizeZCodeMessages（删历史思维链 + tool 结果截断）
//   - 其他（Trae）→ optimizeTraeMessages（system-reminder/hooks_context 清理）
// 入参 messages 已经过 InjectGodRules 处理（已注入上帝指令），此处只做减法/压缩。
func OptimizeMessages(messages []map[string]interface{}, cfg *model.GodRulesConfig) []map[string]interface{} {
	if cfg == nil {
		return messages
	}

	// 客户端识别：ZCode 客户端硬编码注入 "You are ZCode" 作为 system 消息（Trae 无此特征）
	if isZCodeClient(messages) {
		optimized, stripped := optimizeZCodeMessages(messages, cfg)
		log.Printf("[OptimizeMessages] client=zcode, messages=%d->%d, stripped_reasoning=%d",
			len(messages), len(optimized), stripped)
		return optimized
	}

	optimized := optimizeTraeMessages(messages, cfg)
	log.Printf("[OptimizeMessages] client=trae, messages=%d->%d", len(messages), len(optimized))
	return optimized
}

// isZCodeClient 通过 system 消息特征识别 ZCode 客户端
// ZCode 客户端硬编码注入 "You are ZCode, an interactive coding agent" 作为 system 消息
func isZCodeClient(messages []map[string]interface{}) bool {
	for _, msg := range messages {
		if role, _ := msg["role"].(string); role == "system" {
			if strings.Contains(StringifyContent(msg["content"]), "You are ZCode") {
				return true
			}
		}
	}
	return false
}

// optimizeTraeMessages Trae 客户端的 messages 噪音清理（原 OptimizeMessages 逻辑）
// 四刀：stripNoise + stripHooksContext + compressToolResult(10轮) + compressTools
func optimizeTraeMessages(messages []map[string]interface{}, cfg *model.GodRulesConfig) []map[string]interface{} {
	if cfg == nil {
		return messages
	}

	// ── 20 轮截断（Trae 编辑器无限累加的防护）──
	// 与 ZCode 链路共用同一阈值和逻辑
	// 阈值依据：20 轮之前的对话与当前任务几乎无关，重要节点应通过 task log 记录而非依赖上下文
	messages = truncateByRounds(messages, 20, "optimizeTrae")

	// 找第 10 条 user 消息的索引（保护边界：此索引及之后的 tool 结果不截断）
	// 5 轮太激进，放宽到 10 轮，保留更多历史上下文
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
	stripCount := 0
	for i, msg := range messages {
		role, _ := msg["role"].(string)

		switch role {
		case "system":
			// system 消息保留语言要求/上帝指令/RAG，但清除 hooks_context 噪音（Trae IDE hook 事件壳子）
			content := StringifyContent(msg["content"])
			result[i] = map[string]interface{}{
				"role":    "system",
				"content": stripHooksContext(content),
			}

		case "user":
			if isLatestUser(messages, i) {
				// 最新轮：只清 hooks_context，保留 system-reminder（文件/终端上下文有用）
				result[i] = stripHooksFromUserMsg(msg)
			} else {
				if cfg.StripNoise {
					result[i] = optimizeHistoryUserMsg(msg)
					newContent := StringifyContent(result[i]["content"])
					origContent := StringifyContent(msg["content"])
					if len(newContent) < len(origContent) {
						stripCount++
					}
				} else {
					result[i] = msg
				}
			}

		case "tool":
			// 先清 hooks_context，再按轮次截断（system-reminder/toolcall_status 等保留）
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
			result[i] = msg
		}
	}
	if stripCount > 0 {
		log.Printf("[OptimizeMessages] stripped %d/%d history user messages", stripCount, len(messages))
	}
	return result
}

// truncateByRounds 按 user 消息轮次截断历史（公共函数，Trae 和 ZCode 共用）
// 保留所有 system 消息 + 最近 maxRounds 条 user 消息及其后续所有消息
// 在 user 消息边界截断，避免 tool_calls 配对断裂
func truncateByRounds(messages []map[string]interface{}, maxRounds int, tag string) []map[string]interface{} {
	// 从后往前数 user 消息，找到第 maxRounds 条 user 的位置
	keepFrom := len(messages) // 默认保留全部（不超过阈值时不截断）
	uc := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			uc++
			if uc == maxRounds {
				keepFrom = i
				break
			}
		}
	}

	// 不超过阈值，不截断
	if keepFrom >= len(messages) {
		return messages
	}

	// 分离 system 消息和保留区消息
	systemMsgs := []map[string]interface{}{}
	recentMsgs := []map[string]interface{}{}
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			// system 消息始终保留（上帝指令/角色定位/RAG 上下文不能丢）
			systemMsgs = append(systemMsgs, msg)
		} else if i >= keepFrom {
			recentMsgs = append(recentMsgs, msg)
		}
	}

	result := append(systemMsgs, recentMsgs...)
	log.Printf("[%s] %d轮截断: %d -> %d 条消息 (丢弃 %d 条更早历史)",
		tag, maxRounds, len(messages), len(result), len(messages)-len(result))
	return result
}

// isLatestUser 判断 msg[idx] 是否为最后一条 user 消息
func isLatestUser(messages []map[string]interface{}, idx int) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			return i == idx
		}
	}
	return false
}

// optimizeHistoryUserMsg 清理历史轮 user 消息：
// 删除 path/terminal/lang 三类 system-reminder + hooks_context，保留 other 类和 user_input
func optimizeHistoryUserMsg(msg map[string]interface{}) map[string]interface{} {
	origContent := msg["content"]

	// 数组格式：遍历 text 项，逐项清理
	if arr, ok := origContent.([]interface{}); ok {
		newArr := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]interface{})
			if !ok {
				newArr = append(newArr, item)
				continue
			}
			t, _ := m["type"].(string)
			if t == "text" {
				text, _ := m["text"].(string)
				cleaned := stripNoiseReminders(text)
				cleaned = stripHooksContext(cleaned)
				if cleaned != "" {
					newArr = append(newArr, map[string]interface{}{
						"type": "text",
						"text": cleaned,
					})
				}
			} else {
				// image_url 等非 text 项保留
				newArr = append(newArr, item)
			}
		}
		return map[string]interface{}{
			"role":    "user",
			"content": newArr,
		}
	}

	// string 格式：直接清理
	if s, ok := origContent.(string); ok {
		cleaned := stripNoiseReminders(s)
		cleaned = stripHooksContext(cleaned)
		return map[string]interface{}{
			"role":    "user",
			"content": cleaned,
		}
	}

	// 其他类型：不动
	return msg
}

// stripNoiseReminders 删除 path/terminal/lang 三类 system-reminder，保留 other 类
func stripNoiseReminders(content string) string {
	content = reSysReminder.ReplaceAllStringFunc(content, func(match string) string {
		// 删除 path 类（文件打开信息）
		if strings.Contains(match, "Path:") {
			return ""
		}
		// 删除 terminal 类（终端状态）
		if strings.Contains(match, "terminals") || strings.Contains(match, "Terminals") {
			return ""
		}
		// 删除 lang 类（语言设置）
		if strings.Contains(match, "Response Language") || strings.Contains(match, "语言要求") {
			return ""
		}
		// 保留 other 类（rules/LS/skill 等核心能力）
		return match
	})
	content = reOrphanReminder.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

// stripHooksFromUserMsg 清理 user 消息中的 hooks_context（最新轮用）
func stripHooksFromUserMsg(msg map[string]interface{}) map[string]interface{} {
	origContent := msg["content"]

	if arr, ok := origContent.([]interface{}); ok {
		newArr := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]interface{})
			if !ok {
				newArr = append(newArr, item)
				continue
			}
			t, _ := m["type"].(string)
			if t == "text" {
				text, _ := m["text"].(string)
				cleaned := stripHooksContext(text)
				if cleaned != "" {
					newArr = append(newArr, map[string]interface{}{
						"type": "text",
						"text": cleaned,
					})
				}
			} else {
				newArr = append(newArr, item)
			}
		}
		return map[string]interface{}{"role": "user", "content": newArr}
	}

	if s, ok := origContent.(string); ok {
		return map[string]interface{}{"role": "user", "content": stripHooksContext(s)}
	}

	return msg
}

// stripHooksContext 删除 hooks_context 块（所有消息通用）
func stripHooksContext(content string) string {
	before := len(content)
	result := strings.TrimSpace(reHooksContext.ReplaceAllString(content, ""))
	if before != len(result) {
		log.Printf("[DEBUG:stripHooksContext] %d -> %d (stripped %d)", before, len(result), before-len(result))
	}
	return result
}

// ── 第3刀：精简工具定义 ──
//
// 砍 RunCommand 和 Task 的冗长描述：
// - RunCommand：删除 git commit/push 手把手教程（AI 本来就会，且上帝指令已禁止自行提交）
// - Task：删除 example 块（AI 看说明就懂，不需要弱智示例）
// 安全红线（Git Safety Protocol 核心）完整保留。

// OptimizeTools 精简 tools 数组中的工具描述（分流入口）
//   - ZCode 客户端 → optimizeZCodeTools（精简 EnterPlanMode/ExitPlanMode/Agent/AskUserQuestion）
//   - 其他（Trae）→ optimizeTraeTools（精简 RunCommand/Task）
func OptimizeTools(tools []interface{}, cfg *model.GodRulesConfig) []interface{} {
	if cfg == nil || !cfg.CompressTools {
		return tools
	}

	// 客户端识别需要从 messages 判断，这里只有 tools，无法直接判断
	// 通过工具特征判断：ZCode 工具集含 EnterPlanMode/Bash/Edit，Trae 工具集含 RunCommand/Task
	if isZCodeToolset(tools) {
		return optimizeZCodeTools(tools, cfg)
	}
	return optimizeTraeTools(tools, cfg)
}

// isZCodeToolset 通过工具集特征判断是否为 ZCode 客户端
// ZCode 工具集含 EnterPlanMode/ExitPlanMode/Bash/Edit，Trae 含 RunCommand/Task
func isZCodeToolset(tools []interface{}) bool {
	zcodeMarkers := map[string]bool{
		"EnterPlanMode": true, "ExitPlanMode": true, "AskUserQuestion": true, "SendMessage": true,
	}
	for _, raw := range tools {
		t, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := fn["name"].(string); zcodeMarkers[name] {
			return true
		}
	}
	return false
}

// optimizeTraeTools Trae 客户端的工具精简（原 OptimizeTools 逻辑）
func optimizeTraeTools(tools []interface{}, cfg *model.GodRulesConfig) []interface{} {
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
		case "RunCommand":
			// 删除 "### Committing changes with git" 及之后所有内容（含 push 教程）
			if idx := strings.Index(desc, "### Committing changes with git"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc

		case "Task":
			// 删除 "Example usage:" 及之后所有内容（含 example 块）
			if idx := strings.Index(desc, "Example usage:"); idx >= 0 {
				desc = strings.TrimSpace(desc[:idx])
			}
			fn["description"] = desc
		}

		result[i] = t
	}
	return result
}
