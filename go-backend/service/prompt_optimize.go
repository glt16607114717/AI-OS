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
// 三刀：
//   1) stripNoise        - 历史轮 user 消息里删除 path/terminal/lang 三类 system-reminder，
//                          保留 other 类（首轮 rules/LS/skill 等）和 user_input 真实内容
//   2) compressToolResult - 5 轮之前的 tool 结果超 200 字符截断
//   3) compressTools      - 精简 tools 数组中 RunCommand/Task 的冗长描述

// 预编译正则
var (
	// 成对 system-reminder 块（含换行）
	reSysReminder = regexp.MustCompile(`(?s)\s*<system-reminder>.*?</system-reminder>\s*`)
	// 孤立标签残片
	reOrphanReminder = regexp.MustCompile(`</?system-reminder>`)
)

// OptimizeMessages 对发往 LLM 的 messages 做噪音清理（按用户配置，默认全开）
// 入参 messages 已经过 InjectGodRules 处理（已注入上帝指令），此处只做减法/压缩。
func OptimizeMessages(messages []map[string]interface{}, cfg *model.GodRulesConfig) []map[string]interface{} {
	if cfg == nil {
		return messages
	}

	// 找第 5 条 user 消息的索引（保护边界：此索引及之后的 tool 结果不截断）
	keepFromIdx := 0
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			userCount++
			if userCount == 5 {
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
			// system 消息不动（语言要求保留、上帝指令/RAG 注入内容保留）
			result[i] = msg

		case "user":
			// 最新轮 user 消息完全不动；历史轮清理三类噪音
			if isLatestUser(messages, i) {
				result[i] = msg
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
			// 5 轮之前的 tool 结果，超 200 字符截断
			content := StringifyContent(msg["content"])
			if cfg.CompressToolResult && i < keepFromIdx && len(content) > 200 {
				result[i] = map[string]interface{}{
					"role":         "tool",
					"content":      content[:200] + "\n[...结果已截断，如需完整内容请重新执行该工具]",
					"tool_call_id": msg["tool_call_id"],
				}
			} else {
				result[i] = msg
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
// 删除 path/terminal/lang 三类 system-reminder，保留 other 类和 user_input
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
		return map[string]interface{}{
			"role":    "user",
			"content": stripNoiseReminders(s),
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

// ── 第3刀：精简工具定义 ──
//
// 砍 RunCommand 和 Task 的冗长描述：
// - RunCommand：删除 git commit/push 手把手教程（AI 本来就会，且上帝指令已禁止自行提交）
// - Task：删除 example 块（AI 看说明就懂，不需要弱智示例）
// 安全红线（Git Safety Protocol 核心）完整保留。

// OptimizeTools 精简 tools 数组中的工具描述
func OptimizeTools(tools []interface{}, cfg *model.GodRulesConfig) []interface{} {
	if !cfg.CompressTools {
		return tools
	}
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
