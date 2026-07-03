package handler

import (
	"log"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tokenizer     *tiktoken.Tiktoken
	tokenizerOnce sync.Once
)

// getTokenizer 懒加载 tiktoken 编码器（cl100k_base，GPT-4/GLM/Claude 通用）
func getTokenizer() *tiktoken.Tiktoken {
	tokenizerOnce.Do(func() {
		tke, err := tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			log.Printf("[token] tiktoken init failed, fallback to char estimate: %v", err)
			return
		}
		tokenizer = tke
		log.Printf("[token] tiktoken cl100k_base loaded")
	})
	return tokenizer
}

// tokenRatio tiktoken cl100k_base 与 GLM tokenizer 的校准系数
// GLM tokenizer 对中文粒度更细，tiktoken 估算偏低约 19.5%
// 取 1.15 作为固定系数（略保守，避免高估）
const tokenRatio = 1.15

// estimateTokens 精确估算文本 token 数
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	if tke := getTokenizer(); tke != nil {
		n := len(tke.Encode(text, nil, nil))
		return int(float64(n) * tokenRatio)
	}
	return fallbackEstimate(text)
}

// fallbackEstimate 无 tiktoken 时的粗略估算（中英文区分）
func fallbackEstimate(text string) int {
	runes := []rune(text)
	cjk := 0
	other := 0
	for _, r := range runes {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3000 && r <= 0x30FF) {
			cjk++
		} else {
			other++
		}
	}
	count := int(float64(cjk)*1.2 + float64(other)/4.0)
	if count == 0 {
		count = 1
	}
	return count
}

// estimatePromptTokensV2 从请求 messages 中估算 prompt token 数
// 修复：用 toInterfaceSlice 兼容 []map[string]interface{} 类型（injectRAGContext 后类型变化）
func estimatePromptTokensV2(req map[string]interface{}) int {
	messages := toInterfaceSlice(req["messages"])
	if messages == nil {
		return 0
	}
	var sb strings.Builder
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if role, ok := msg["role"].(string); ok {
			sb.WriteString(role)
			sb.WriteString(":")
		}
		sb.WriteString(extractContent(msg["content"]))
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return 0
	}
	return estimateTokens(sb.String())
}

// estimateCompletionTokens 估算 completion（回复内容）token 数
func estimateCompletionTokens(content string) int {
	if content == "" {
		return 0
	}
	return estimateTokens(content)
}

// estimatePromptTokensV2FromMessages 从 messages 切片估算 prompt token 数（非流式路径用）
func estimatePromptTokensV2FromMessages(messages []interface{}) int {
	if messages == nil {
		return 0
	}
	var sb strings.Builder
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if role, ok := msg["role"].(string); ok {
			sb.WriteString(role)
			sb.WriteString(":")
		}
		sb.WriteString(extractContent(msg["content"]))
		sb.WriteString("\n")
	}
	if sb.Len() == 0 {
		return 0
	}
	return estimateTokens(sb.String())
}
