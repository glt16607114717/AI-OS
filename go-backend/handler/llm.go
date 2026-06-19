package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ── LLM 处理核心 ──
//
// 架构：两个入口（ProxyChatCompletions / WorkspaceChat）都调用 handleChat，
// 由 handleChat 根据"是否有技能工具"分发到 agentLoop（多轮循环）或 proxyForward（纯透传）。

// ConversationContext 累积一个请求的完整上下文信息
type ConversationContext struct {
	UserID           int
	Username         string
	UserMessage      string   // 用户提问摘要
	Models           []string // 使用的模型列表
	KeyNames         []string // 使用的 API Key 名称列表
	FailoverCount    int      // 故障转移次数
	TotalPrompt      int
	TotalCompletion  int
	ToolCallCount    int
	Errors           []string
	AssistantContent string // 最终助手回复
	ChatHistoryID    int64  // sys_chat_history.id，用于下载日志
	Source           string // proxy / workspace
	IsAdmin          bool   // 是否管理员
	IsStream         bool   // 原始请求是否流式
	StartTime        time.Time
}

// SummarizeAndLog 写一条对话汇总日志
func (c *ConversationContext) SummarizeAndLog() {
	latency := int(time.Since(c.StartTime).Milliseconds())
	models := strings.Join(c.Models, ",")
	level := "info"
	if len(c.Errors) > 0 {
		level = "error"
	}
	detail := map[string]interface{}{
		"user_id":           c.UserID,
		"username":          c.Username,
		"models":            c.Models,
		"key_names":         c.KeyNames,
		"failover_count":    c.FailoverCount,
		"prompt_tokens":     c.TotalPrompt,
		"completion_tokens": c.TotalCompletion,
		"tool_calls":        c.ToolCallCount,
		"errors":            c.Errors,
		"user_message":      c.UserMessage,
		"latency_ms":        latency,
		"chat_history_id":   c.ChatHistoryID,
	}
	detailJSON, _ := json.Marshal(detail)
	service.WriteLog("conversation",
		fmt.Sprintf("对话完成 | %s | 输入%d/输出%d/耗时%dms", models, c.TotalPrompt, c.TotalCompletion, latency),
		level, string(detailJSON), c.UserID)
}

// ProxyChatCompletions LLM 代理转发入口（供 TRAE 等 OpenAI 兼容客户端调用）
func ProxyChatCompletions(w http.ResponseWriter, r *http.Request) {
	handleChat(w, r, "proxy")
}

// WorkspaceChat 工作台聊天入口
func WorkspaceChat(w http.ResponseWriter, r *http.Request) {
	handleChat(w, r, "workspace")
}

// handleChat 统一入口：预处理 + 分发
func handleChat(w http.ResponseWriter, r *http.Request, source string) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID := 0
	username := "anonymous"
	if session != nil {
		userID = session.UserID
		username = session.Username
	}

	// 解析请求
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	// 提取用户消息（兼容 string 和数组格式）
	userMsgSummary := ""
	userMsgRaw := ""
	if messages, ok := req["messages"].([]interface{}); ok {
		for _, m := range messages {
			if msg, ok := m.(map[string]interface{}); ok {
				if role, _ := msg["role"].(string); role == "user" {
					userMsgRaw = extractContent(msg["content"])
					userMsgSummary = userMsgRaw
				}
			}
		}
	}
	if len(userMsgSummary) > 200 {
		userMsgSummary = userMsgSummary[:200]
	}

	// 注入上帝指令 + RAG 知识库
	if messages, ok := req["messages"].([]interface{}); ok {
		msgMaps := make([]map[string]interface{}, len(messages))
		for i, m := range messages {
			msgMaps[i], _ = m.(map[string]interface{})
		}
		msgMaps = service.InjectGodRules(msgMaps)
		msgMaps = injectRAGContext(msgMaps, extractUserQuery(userMsgRaw), userID)
		req["messages"] = msgMaps
	}

	// 注入技能工具
	isAdmin := session != nil && session.IsAdmin
	tools, _ := service.GetBuiltinSkillToolDefinitions(userID, isAdmin)
	if len(tools) > 0 {
		req["tools"] = tools
	}

	// 流式判断
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	convCtx := &ConversationContext{
		UserID:      userID,
		Username:    username,
		UserMessage: userMsgSummary,
		Source:      source,
		IsAdmin:     isAdmin,
		IsStream:    isStream,
		StartTime:   startTime,
	}

	// 获取路由
	route := service.GetRouteByStrategy(userID)
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		convCtx.Errors = append(convCtx.Errors, "无可用厂商（无策略、无可用选项）")
		convCtx.SummarizeAndLog()
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}
	req["model"] = route.ModelID

	// 请求转储
	service.DumpRequest(req)

	// 额度降级（仅 proxy）
	if source == "proxy" {
		downgraded := service.ShouldDowngradeModel(route.KeyID, route.ModelID)
		if downgraded != route.ModelID {
			req["model"] = downgraded
		}
	}

	// 故障转移链
	attempts := buildFailoverAttempts(route, userID)

	// 分发：有技能工具 → Agent 循环；无工具 → 纯透传
	if len(tools) > 0 {
		log.Printf("[chat] %s user=%s tools=%d → agent", source, username, len(tools))
		agentLoop(w, req, attempts, convCtx)
	} else {
		log.Printf("[chat] %s user=%s stream=%v → proxy", source, username, isStream)
		proxyForward(w, req, attempts, convCtx)
	}
}

// buildFailoverAttempts 构建故障转移路由列表（策略路由优先 + 轮询其他可用路由）
func buildFailoverAttempts(primary *service.RouteInfoType, userID int) []*service.RouteInfoType {
	attempts := []*service.RouteInfoType{primary}
	failoverRoutes := service.GetAllRoutesForFailover(userID)
	for i := range failoverRoutes {
		if failoverRoutes[i].KeyID != primary.KeyID {
			attempts = append(attempts, &failoverRoutes[i])
		}
	}
	return attempts
}

// ── 辅助函数 ──

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// extractContent 兼容提取 content：string 或 [{"type":"text","text":"..."}] 数组格式
func extractContent(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if text, _ := m["text"].(string); text != "" {
						sb.WriteString(text)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}

func keyPrefix8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// extractUserQuery 过滤 IDE 注入的噪音块，提取干净的用户提问（用于 RAG 检索）
func extractUserQuery(raw string) string {
	if raw == "" {
		return ""
	}

	// 去除各类 IDE 注入的 XML 标签块
	patterns := []string{
		`(?s)<system-reminder>.*?</system-reminder>`,
		`(?s)<environment>.*?</environment>`,
		`(?s)<trae_rules_context>.*?</trae_rules_context>`,
		`(?s)<available_skills>.*?</available_skills>`,
		`(?s)<rules>.*?</rules>`,
	}
	cleaned := raw
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	// 清理多余空行和首尾空白
	cleaned = strings.TrimSpace(cleaned)
	for strings.Contains(cleaned, "\n\n\n") {
		cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	}

	return cleaned
}

// injectRAGContext 检索知识库，将相关内容注入 system prompt 最前面
func injectRAGContext(messages []map[string]interface{}, userMsg string, userID int) []map[string]interface{} {
	if userMsg == "" || userID <= 0 {
		return messages
	}

	// 语义搜索知识库
	results, err := service.SearchSimilar(userMsg, 5, userID)
	if err != nil || len(results) == 0 {
		return messages
	}

	// 构建知识上下文
	var ctx strings.Builder
	ctx.WriteString("[HIGHEST PRIORITY - 知识库参考]\n")
	ctx.WriteString("以下内容来自企业知识库，在回答时必须优先参考：\n\n")
	count := 0
	for _, r := range results {
		if r.Score < 0.5 {
			continue
		}
		count++
		ctx.WriteString(fmt.Sprintf("--- 参考 %d（相似度 %.0f%%）---\n%s\n\n", count, r.Score*100, r.Content))
	}

	if count == 0 {
		return messages
	}

	ragCtx := strings.TrimSpace(ctx.String())

	// 注入到 system 消息最前面
	result := make([]map[string]interface{}, len(messages))
	injected := false
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" && !injected {
			original, _ := msg["content"].(string)
			result[i] = map[string]interface{}{
				"role":    "system",
				"content": ragCtx + "\n\n---\n\n" + original,
			}
			injected = true
		} else {
			result[i] = msg
		}
	}
	if !injected {
		result = append([]map[string]interface{}{
			{"role": "system", "content": ragCtx},
		}, result...)
	}
	return result
}

// ProxyModels 代理 /v1/models 接口
func ProxyModels(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	userID := 0
	if session != nil {
		userID = session.UserID
	}
	route := service.GetRouteByStrategy(userID)
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}

	baseURL := strings.TrimRight(route.BaseURL, "/")
	upstreamURL := baseURL + "/models"

	req, _ := http.NewRequest("GET", upstreamURL, nil)
	req.Header.Set("Authorization", "Bearer "+route.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		errResponse(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
