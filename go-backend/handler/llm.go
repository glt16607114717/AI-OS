package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/model"
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
	SSEHeaderWritten bool   // SSE 响应头是否已写入（防止重复 WriteHeader）
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
// 纯透传：不注入服务端技能，客户端工具原样转发，不解析响应体
func ProxyChatCompletions(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID, username := sessionUser(session)
	isAdmin := session != nil && session.IsAdmin

	// 解析请求
	req, ok := parseRequestBody(w, r)
	if !ok {
		return
	}

	// 提取用户消息
	userMsgRaw, userMsgSummary := extractLastUserMessage(req)
	if cleanedMsg := extractUserQuery(userMsgRaw); cleanedMsg != "" {
		service.AddChatMessage(userID, "user", cleanedMsg)
	}

	// 注入上帝指令 + RAG
	injectGodRulesAndRAG(req, userMsgRaw, userID, username)

	// 流式判断
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	convCtx := &ConversationContext{
		UserID: userID, Username: username, UserMessage: userMsgSummary,
		Source: "proxy", IsAdmin: isAdmin, IsStream: isStream, StartTime: startTime,
	}

	// 路由（代理专属：含额度降级）
	route := service.GetRouteByStrategy(userID)
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		convCtx.Errors = append(convCtx.Errors, "无可用厂商")
		convCtx.SummarizeAndLog()
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}
	req["model"] = route.ModelID

	// 额度降级（仅代理）
	if downgraded := service.ShouldDowngradeModel(route.KeyID, route.ModelID); downgraded != route.ModelID {
		req["model"] = downgraded
	}

	service.DumpRequest(req)
	attempts := buildFailoverAttempts(route, userID)

	log.Printf("[proxy] user=%s stream=%v model=%s", username, isStream, req["model"])
	proxyForward(w, req, attempts, convCtx)
}

// WorkspaceChat 工作台聊天入口
// 技能注入 + agent 循环：注入服务端技能，多轮 tool_calls 循环
func WorkspaceChat(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID, username := sessionUser(session)
	isAdmin := session != nil && session.IsAdmin

	// 解析请求
	req, ok := parseRequestBody(w, r)
	if !ok {
		return
	}

	// 提取用户消息
	userMsgRaw, userMsgSummary := extractLastUserMessage(req)
	if cleanedMsg := extractUserQuery(userMsgRaw); cleanedMsg != "" {
		service.AddChatMessage(userID, "user", cleanedMsg)
	}

	// 注入上帝指令 + RAG
	injectGodRulesAndRAG(req, userMsgRaw, userID, username)

	// 注入服务端技能（工作台专属）
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
		UserID: userID, Username: username, UserMessage: userMsgSummary,
		Source: "workspace", IsAdmin: isAdmin, IsStream: isStream, StartTime: startTime,
	}

	// 路由（工作台专属：含额度降级）
	route := service.GetRouteByStrategy(userID)
	if route == nil {
		route = service.GetDefaultRoute()
	}
	if route == nil {
		convCtx.Errors = append(convCtx.Errors, "无可用厂商")
		convCtx.SummarizeAndLog()
		errResponse(w, "请先配置 API Key 和模型", 400)
		return
	}
	req["model"] = route.ModelID

	// 额度降级
	if downgraded := service.ShouldDowngradeModel(route.KeyID, route.ModelID); downgraded != route.ModelID {
		req["model"] = downgraded
	}

	service.DumpRequest(req)
	attempts := buildFailoverAttempts(route, userID)

	if len(tools) > 0 {
		log.Printf("[workspace] user=%s tools=%d model=%s → agent", username, len(tools), req["model"])
		agentLoop(w, req, attempts, convCtx)
	} else {
		log.Printf("[workspace] user=%s no_tools model=%s → proxy", username, req["model"])
		proxyForward(w, req, attempts, convCtx)
	}
}

// ── 公共辅助函数（纯函数，无副作用，两个入口共用）──

// sessionUser 从 session 提取 userID 和 username
func sessionUser(session *model.Session) (int, string) {
	if session == nil {
		return 0, "anonymous"
	}
	return session.UserID, session.Username
}

// parseRequestBody 解析请求体
func parseRequestBody(w http.ResponseWriter, r *http.Request) (map[string]interface{}, bool) {
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return nil, false
	}
	return req, true
}

// extractLastUserMessage 提取最后一条 user 消息的原始内容和摘要
func extractLastUserMessage(req map[string]interface{}) (string, string) {
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return "", ""
	}
	userMsgRaw := ""
	for _, m := range messages {
		if msg, ok := m.(map[string]interface{}); ok {
			if role, _ := msg["role"].(string); role == "user" {
				userMsgRaw = extractContent(msg["content"])
			}
		}
	}
	summary := extractUserQuery(userMsgRaw)
	if len(summary) > 200 {
		summary = summary[:200]
	}
	return userMsgRaw, summary
}

// injectGodRulesAndRAG 注入上帝指令和 RAG 知识库上下文
func injectGodRulesAndRAG(req map[string]interface{}, userMsgRaw string, userID int, username string) {
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return
	}
	msgMaps := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		msgMaps[i], _ = m.(map[string]interface{})
	}
	msgMaps = service.InjectGodRules(msgMaps)
	msgMaps = injectRAGContext(msgMaps, extractUserQuery(userMsgRaw), userID)

	// 图片识别预处理：检测最后一条 user message 的图片（上传/URL），识别后追加文字描述
	msgMaps, visionResult := service.ProcessImages(msgMaps, userID, username)
	// 兜底：剥离所有消息中残留的 image_url 项，防止透传到纯文本模型
	service.StripImageContent(msgMaps)
	req["messages"] = msgMaps

	if visionResult != nil && visionResult.Modified {
		log.Printf("[chat] 图片识别 %d 张，已追加文字描述", visionResult.ImageCount)
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

// stringifyContent 兼容 string 和 []interface{} 格式的 content，统一转成 string（委托给 service 包）
func stringifyContent(content interface{}) string {
	return service.StringifyContent(content)
}

func keyPrefix8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// extractUserQuery 过滤 IDE 注入的噪音块，提取干净的用户提问
// 用于 sys_llm_log.detail.user_message 和 sys_chat_history 展示，不影响原始 req 留存
func extractUserQuery(raw string) string {
	if raw == "" {
		return ""
	}

	// 1. 优先提取 <user_input> 标签内的真实用户消息
	//    先尝试成对闭合标签；匹配不到再尝试「只有开标签」取其后内容（防 IDE 切分导致闭标签丢失）
	cleaned := raw
	pairRe := regexp.MustCompile(`(?s)<user_input>(.*?)</user_input>`)
	if match := pairRe.FindStringSubmatch(raw); len(match) > 1 {
		cleaned = match[1]
	} else if idx := strings.Index(raw, "<user_input>"); idx >= 0 {
		cleaned = raw[idx+len("<user_input>"):]
	}

	// 2. 去除成对的 IDE 注入 XML 噪音块
	pairNoise := []string{
		`(?s)<system-reminder>.*?</system-reminder>`,
		`(?s)<environment>.*?</environment>`,
		`(?s)<trae_rules_context>.*?</trae_rules_context>`,
		`(?s)<available_skills>.*?</available_skills>`,
		`(?s)<rules>.*?</rules>`,
	}
	for _, p := range pairNoise {
		cleaned = regexp.MustCompile(p).ReplaceAllString(cleaned, "")
	}

	// 3. 清除所有孤立的 XML 标签残片（开标签 + 闭标签都干掉，防跨 text 段拼接导致标签残缺）
	orphanTags := []string{
		`</?system-reminder>`,
		`</?user_input>`,
		`</?environment>`,
		`</?trae_rules_context>`,
		`</?available_skills>`,
		`</?rules>`,
		`</?available_terminal>`,
		`</?tool_calls?>`,
		`</?toolcall_status>`,
		`</?toolcall_result>`,
	}
	for _, p := range orphanTags {
		cleaned = regexp.MustCompile(p).ReplaceAllString(cleaned, "")
	}

	// 4. 清理多余空行和首尾空白
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
		if r.Score < 0.35 {
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
			original := stringifyContent(msg["content"])
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

	resp, err := service.SharedHTTPClient.Do(req)
	if err != nil {
		errResponse(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}
