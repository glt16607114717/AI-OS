package service

import (
	"ai-os-server/model"
	"path/filepath"
	"regexp"
	"strings"
)

// ── 提示词优化：发往 LLM 前的 messages 清洗 ──
//
// 作用：在 injectGodRulesAndRAG 内、转发给大模型之前调用。
// 只改发往 LLM 的 messages，不改原始留存（conversation_*.json 仍是完整 dump）。
//
// 三刀：
//   1) stripNoise   - 历史轮 user 消息里的 system-reminder 全删（只留 user_input）
//   2) compressFile - 最新轮的"文件打开信息"提取路径，压缩成 [当前文件: xxx]
//   3) simplifyLang - 删 system 消息和 user 消息里的语言要求块，最新轮末尾拼精简版

// 预编译正则
var (
	// 成对 system-reminder 块（含换行）
	reSysReminder = regexp.MustCompile(`(?s)\s*<system-reminder>.*?</system-reminder>\s*`)
	// 孤立标签残片
	reOrphanReminder = regexp.MustCompile(`</?system-reminder>`)
	// user_input 标签
	reUserInputPair = regexp.MustCompile(`(?s)<user_input>(.*?)</user_input>`)
	// 从 system-reminder 中提取 Path
	rePath = regexp.MustCompile(`Path:\s*(\S+)`)
	// system 消息里的 "# Response language" 段落（到下一个 # 标题前，保留下一个标题）
	reSysLangSection = regexp.MustCompile(`(?s)# Response language\n.*?(\n# )`)
)

// OptimizeMessages 对发往 LLM 的 messages 做噪音清理（按用户配置，默认全开）
// 入参 messages 已经过 InjectGodRules 处理（已注入上帝指令），此处只做减法/压缩。
func OptimizeMessages(messages []map[string]interface{}, cfg *model.GodRulesConfig) []map[string]interface{} {
	if cfg == nil {
		return messages
	}

	// 找最后一条 user 消息的索引
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role == "user" {
			lastUserIdx = i
			break
		}
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
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := StringifyContent(msg["content"])

		switch role {
		case "system":
			// 第3刀：删 system 里的语言要求段落
			if cfg.SimplifyLang {
				content = reSysLangSection.ReplaceAllString(content, "$1")
				content = strings.TrimSpace(content)
			}
			result[i] = map[string]interface{}{
				"role":    "system",
				"content": content,
			}

		case "user":
			if i == lastUserIdx {
				// 最新轮：必须保留原始 content 类型（数组保持数组）
				// 因为后续 ProcessImages 需要检查 content 是否为 []interface{} 来识别 image_url
				// 如果扁平化成 string，图片识别会被跳过
				result[i] = optimizeLatestUserMsg(msg, cfg)
			} else {
				// 历史轮：stripNoise 开则全删 reminder（只留 user_input），但保留图片描述
				if cfg.StripNoise {
					result[i] = optimizeHistoryUserMsg(msg)
				} else {
					result[i] = msg
				}
			}

		case "tool":
			// 第4刀：5 轮之前的 tool 结果，超 500 字符截断
			if cfg.CompressToolResult && i < keepFromIdx && len(content) > 500 {
				result[i] = map[string]interface{}{
					"role":          "tool",
					"content":       content[:500] + "\n[...结果已截断，如需完整内容请重新执行该工具]",
					"tool_call_id":  msg["tool_call_id"],
				}
			} else {
				result[i] = msg
			}

		default:
			result[i] = msg
		}
	}
	return result
}

// stripHistoryUser 清理历史轮 user 消息：删除所有 system-reminder，只保留 user_input 内容
func stripHistoryUser(content string) string {
	// 先尝试提取 user_input 标签内容
	if m := reUserInputPair.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// 没有配对标签，按"只有开标签"取其后内容
	if idx := strings.Index(content, "<user_input>"); idx >= 0 {
		rest := content[idx+len("<user_input>"):]
		rest = reOrphanReminder.ReplaceAllString(rest, "")
		return strings.TrimSpace(rest)
	}
	// 没有 user_input 标签，删所有 reminder 块
	cleaned := reSysReminder.ReplaceAllString(content, "")
	cleaned = reOrphanReminder.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

// optimizeHistoryUserMsg 优化历史轮 user 消息：保留图片描述，清理其他
func optimizeHistoryUserMsg(msg map[string]interface{}) map[string]interface{} {
	origContent := msg["content"]

	// 数组格式：遍历 text 项，保留图片描述，清理其他
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
				// 保留图片描述 text 项
				if strings.Contains(text, "视觉模型") || strings.Contains(text, "图片") && strings.Contains(text, "识别") {
					newArr = append(newArr, item)
				} else {
					cleaned := stripHistoryUser(text)
					if cleaned != "" {
						newArr = append(newArr, map[string]interface{}{
							"type": "text",
							"text": cleaned,
						})
					}
				}
			} else {
				// 非 text 项保留
				newArr = append(newArr, item)
			}
		}
		return map[string]interface{}{
			"role":    "user",
			"content": newArr,
		}
	}

	// string 格式：直接处理，但保留图片描述
	if s, ok := origContent.(string); ok {
		// 如果是字符串，尝试分割保留图片描述
		if strings.Contains(s, "视觉模型") || strings.Contains(s, "图片") && strings.Contains(s, "识别") {
			var kept string
			// 找到图片描述的起始位置
			if idx := strings.Index(s, "[系统已通过视觉模型识别"); idx >= 0 {
				cleaned := stripHistoryUser(s[:idx])
				kept = s[idx:]
				if cleaned != "" {
					return map[string]interface{}{
						"role":    "user",
						"content": cleaned + "\n\n" + kept,
					}
				} else {
					return map[string]interface{}{
						"role":    "user",
						"content": kept,
					}
				}
			}
		}
		// 没有图片描述，直接清理
		return map[string]interface{}{
			"role":    "user",
			"content": stripHistoryUser(s),
		}
	}

	// 其他类型：不动
	return msg
}

// optimizeLatestUserMsg 优化最新轮 user 消息，保留原始 content 类型
// content 是数组时：遍历 text 项做优化，保留 image_url 等非文本项
// content 是字符串时：直接做字符串优化
func optimizeLatestUserMsg(msg map[string]interface{}, cfg *model.GodRulesConfig) map[string]interface{} {
	origContent := msg["content"]

	// 数组格式：逐项优化，保留结构
	if arr, ok := origContent.([]interface{}); ok {
		newArr := make([]interface{}, 0, len(arr))
		var fileHint string
		for _, item := range arr {
			m, ok := item.(map[string]interface{})
			if !ok {
				newArr = append(newArr, item)
				continue
			}
			t, _ := m["type"].(string)
			if t == "text" {
				text, _ := m["text"].(string)
				text, fh := optimizeLatestUserText(text, cfg)
				if fh != "" {
					fileHint = fh
				}
				m["text"] = text
				newArr = append(newArr, m)
			} else {
				// image_url 等非 text 项，原样保留
				newArr = append(newArr, item)
			}
		}
		// 追加文件提示（作为新的 text 项）
		if fileHint != "" {
			newArr = append(newArr, map[string]interface{}{
				"type": "text",
				"text": "\n\n[当前文件: " + fileHint + "]",
			})
		}
		return map[string]interface{}{
			"role":    "user",
			"content": newArr,
		}
	}

	// 字符串格式：直接优化
	if s, ok := origContent.(string); ok {
		optimized, _ := optimizeLatestUserText(s, cfg)
		return map[string]interface{}{
			"role":    "user",
			"content": optimized,
		}
	}

	// 其他类型：不动
	return msg
}

// optimizeLatestUserText 对文本内容做优化（压缩文件信息、删语言块），返回优化后文本 + fileHint
func optimizeLatestUserText(content string, cfg *model.GodRulesConfig) (string, string) {
	var fileHint string

	// 第2刀：压缩文件信息（从 reminder 提取 Path，拼成一行）
	if cfg.CompressFile {
		reminders := reSysReminder.FindAllString(content, -1)
		for _, r := range reminders {
			if pm := rePath.FindStringSubmatch(r); len(pm) > 1 {
				fileHint = filepath.Base(pm[1])
			}
		}
	}

	// 删除"文件打开信息"的 reminder
	content = removeFileReminders(content)

	// 第3刀：删语言要求块
	if cfg.SimplifyLang {
		content = removeLangReminders(content)
	}
	content = reOrphanReminder.ReplaceAllString(content, "")
	content = strings.TrimSpace(content)

	return content, fileHint
}

// removeFileReminders 删除含 "Path:" 的 system-reminder 块
func removeFileReminders(content string) string {
	return reSysReminder.ReplaceAllStringFunc(content, func(match string) string {
		if strings.Contains(match, "Path:") {
			return ""
		}
		return match
	})
}

// ── 第5刀：精简工具定义 ──
//
// 砍 RunCommand 和 Task 的冗长描述：
// - RunCommand：删除 git commit/push 手把手教程（AI 本来就会，且上帝指令已禁止自行提交）
// - Task：删除 example 块（AI 看说明就懂，不需要弱智示例）
// 安全红线（Git Safety Protocol 核心）完整保留。

// OptimizeTools 精简 tools 数组中的工具描述
func OptimizeTools(tools []interface{}, cfg *model.GodRulesConfig) []interface{} {
	if !cfg.PromptOptimize || !cfg.CompressTools {
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

// removeLangReminders 删除含 "Response Language" 或 "语言" 的 system-reminder 块
func removeLangReminders(content string) string {
	return reSysReminder.ReplaceAllStringFunc(content, func(match string) string {
		if strings.Contains(match, "Response Language") || strings.Contains(match, "语言要求") {
			return ""
		}
		return match
	})
}
