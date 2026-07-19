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
	SessionID        string // Trae 会话 ID（本地 hook 注入的 [TRACE:session=xxx]）
	MsgId            string // 消息 ID（[TRACE:msg=xxx]，标识同一次用户输入触发的所有请求）
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
		"session_id":        c.SessionID,
		"msg_id":            c.MsgId,
	}
	detailJSON, _ := json.Marshal(detail)
	service.WriteLog("conversation",
		fmt.Sprintf("对话完成 | %s | 输入%d/输出%d/耗时%dms", models, c.TotalPrompt, c.TotalCompletion, latency),
		level, string(detailJSON), c.UserID, c.SessionID, c.MsgId)
}

// ProxyChatCompletions LLM 代理转发入口（供 TRAE 等 OpenAI 兼容客户端调用）
// 纯透传：不注入服务端技能，客户端工具原样转发，不解析响应体
func ProxyChatCompletions(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID, username := sessionUser(session)
	isAdmin := session != nil && session.IsAdmin

	// 业务用户不允许走代理链路（仅限工作台）
	if session != nil && session.UserType == "business" {
		errResponse(w, "业务用户无权使用代理通道", 403)
		return
	}

	// 逗你玩拦截：检查该用户是否被配置了恶搞
	if prankText, prankActive := CheckPrankActive(userID); prankActive {
		log.Printf("[prank] 用户 %s(%d) 命中逗你玩，开始 SSE 模拟", username, userID)
		HandlePrankSSE(w, r, prankText)
		return
	}

	// 解析请求
	req, ok := parseRequestBody(w, r)
	if !ok {
		return
	}

	// 转储原始请求（加工前），用于分析 Trae 注入的噪音
	service.DumpRawRequest(req)

	// 提取 trace 标记（本地 hook 注入的会话追踪信息，无标记时安全跳过）
	sessionID, msgId := extractAndStripTrace(req)

	// 提取用户消息
	userMsgRaw, userMsgSummary := extractLastUserMessage(req)
	if cleanedMsg := extractUserQuery(userMsgRaw); cleanedMsg != "" {
		service.AddChatMessage(userID, "user", cleanedMsg, sessionID, msgId)
	}

	// 注入角色定位（工作台专属，在上帝指令和 RAG 之前）
	injectWorkspaceRole(req, session)

	// 注入上帝指令 + RAG
	injectGodRulesAndRAG(req, userMsgRaw, userID, username)

	// 提示词精简（强制开启，不需要用户配置）
	// 工作台场景没有编辑器噪音，不需要精简
	optimizeCfg := &model.GodRulesConfig{
		StripNoise:         true,
		CompressToolResult: true,
		CompressTools:      true,
	}
	// 处理两种可能的类型：injectGodRulesAndRAG 后是 []map[string]interface{}
	var msgMaps []map[string]interface{}
	switch v := req["messages"].(type) {
	case []map[string]interface{}:
		msgMaps = v
	case []interface{}:
		msgMaps = make([]map[string]interface{}, len(v))
		for i, m := range v {
			msgMaps[i], _ = m.(map[string]interface{})
		}
	default:
		log.Printf("[DEBUG:Optimize] messages type mismatch: %T", req["messages"])
	}
	// 调试：打印代理收到的原始 messages 数量（排查 ZCode tail 窗口 vs 实际请求数）
	log.Printf("[DEBUG:RAW-RECV] 收到 %d 条消息 (ZCode rollout 说只有 64)", len(msgMaps))
	if len(msgMaps) > 0 {
		// 对话轮次管理（防止无限制累加）
		// 计算当前对话轮次（user 消息数）
		userRounds := 0
		for _, m := range msgMaps {
			if role, _ := m["role"].(string); role == "user" {
				userRounds++
			}
		}
		// 硬上限：超过 300 轮拒绝服务，防止截断本身也变成负担
		if userRounds > 300 {
			log.Printf("[chat] 对话已达 %d 轮，超过 300 轮硬上限，拒绝服务", userRounds)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false,
				"error": "当前对话已超过 300 轮上限，无法继续处理。请开一个新对话窗口。\n" +
					"建议：让当前 AI 汇总一下之前的上下文要点，复制到新对话中继续。",
			})
			return
		}
		// 270 轮提醒：在最新 user 消息前追加提示（留 30 轮空隙让用户操作上下文切换）
		if userRounds >= 270 && userRounds <= 300 {
			lastUserIdx := -1
			for i := len(msgMaps) - 1; i >= 0; i-- {
				if role, _ := msgMaps[i]["role"].(string); role == "user" {
					lastUserIdx = i
					break
				}
			}
			if lastUserIdx >= 0 {
				warning := "\n\n[系统提醒] 当前对话已达 " + fmt.Sprintf("%d", userRounds) +
					" 轮，即将达到 300 轮上限。建议尽快让 AI 汇总之前的上下文要点，" +
					"复制到新对话窗口继续。达到 300 轮后将无法继续对话。"
				content := service.StringifyContent(msgMaps[lastUserIdx]["content"])
				msgMaps[lastUserIdx]["content"] = content + warning
				log.Printf("[chat] 对话已达 %d 轮，追加提醒", userRounds)
			}
		}
		// DEBUG: dump 优化前的原始 messages（仅历史 user 消息，用于对比）
		for i, m := range msgMaps {
			if role, _ := m["role"].(string); role == "user" && i < len(msgMaps)-1 {
				c := service.StringifyContent(m["content"])
				if len(c) > 200 {
					preview := c
					if len(preview) > 1500 {
						preview = preview[:1500]
					}
					log.Printf("[DEBUG:Optimize-BEFORE] msg[%d] user len=%d content=%q", i, len(c), preview)
				}
			}
		}
		before := len(msgMaps)
		msgMaps = service.OptimizeMessages(msgMaps, optimizeCfg)
		// DEBUG: dump 优化后
		for i, m := range msgMaps {
			if role, _ := m["role"].(string); role == "user" && i < len(msgMaps)-1 {
				c := service.StringifyContent(m["content"])
				preview := c
				if len(preview) > 200 {
					preview = preview[:200]
				}
				log.Printf("[DEBUG:Optimize-AFTER]  msg[%d] user len=%d preview=%q", i, len(c), preview)
			}
		}
		log.Printf("[DEBUG:Optimize] messages %d -> %d, StripNoise=%v", before, len(msgMaps), optimizeCfg.StripNoise)
		req["messages"] = msgMaps
	}
	if tools, ok := req["tools"].([]interface{}); ok && len(tools) > 0 {
		req["tools"] = service.OptimizeTools(tools, optimizeCfg)
	}

	// 流式判断
	isStream := false
	if s, ok := req["stream"].(bool); ok && s {
		isStream = true
	}

	convCtx := &ConversationContext{
		UserID: userID, Username: username, UserMessage: userMsgSummary,
		Source: "proxy", IsAdmin: isAdmin, IsStream: isStream, StartTime: startTime,
		SessionID: sessionID, MsgId: msgId,
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
	proxyForward(w, r, req, attempts, convCtx)
}

// WorkspaceChat 工作台聊天入口
// 技能注入 + agent 循环：注入服务端技能，多轮 tool_calls 循环
func WorkspaceChat(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	session := middleware.GetSessionFromCtx(r)
	userID, username := sessionUser(session)
	isAdmin := session != nil && session.IsAdmin

	// 逗你玩拦截：检查该用户是否被配置了恶搞
	if prankText, prankActive := CheckPrankActive(userID); prankActive {
		log.Printf("[prank] 用户 %s(%d) 命中逗你玩，开始 SSE 模拟", username, userID)
		HandlePrankSSE(w, r, prankText)
		return
	}

	// 解析请求
	req, ok := parseRequestBody(w, r)
	if !ok {
		return
	}

	// 提取 trace 标记（本地 hook 注入的会话追踪信息，无标记时安全跳过）
	sessionID, msgId := extractAndStripTrace(req)

	// 提取用户消息
	userMsgRaw, userMsgSummary := extractLastUserMessage(req)
	if cleanedMsg := extractUserQuery(userMsgRaw); cleanedMsg != "" {
		service.AddChatMessage(userID, "user", cleanedMsg, sessionID, msgId)
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
		SessionID: sessionID, MsgId: msgId,
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
		proxyForward(w, r, req, attempts, convCtx)
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

// extractAndStripTrace 从 messages 数组中提取 [TRACE:session=xxx][TRACE:msg=xxx] 标记，
// 剥离后返回 sessionID/msgId，并将消息内容替换为干净版本。
// 没有标记时安全跳过（其他用户无 hook 也能正常工作）。
//
// 兼容两种客户端注入方式：
//   - Trae：trace 直接追加在最后一条 role=user 消息的 content 末尾（纯文本）
//   - ZCode：trace 包裹在 <hooks_context><additional_context> XML 标签中，注入到 role=tool 消息里
//
// 因此从后往前遍历所有消息（不限 role），找到第一个含 trace 的消息即返回。
// 数组格式（含 image_url）时检查所有 text 字段，不限 type。
func extractAndStripTrace(req map[string]interface{}) (sessionID, msgId string) {
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return "", ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]interface{})
		if !ok {
			continue
		}
		content := msg["content"]
		switch v := content.(type) {
		case string:
			sID, mID, cleaned := extractTraceMarkers(v)
			if sID != "" {
				msg["content"] = strings.TrimSpace(cleaned)
				return sID, mID
			}
		case []interface{}:
			// 数组格式（含 image_url）：在 text 项中查找并剥离 trace
			var sID, mID string
			var cleanedArr []interface{}
			for _, item := range v {
				m, ok := item.(map[string]interface{})
				if !ok {
					cleanedArr = append(cleanedArr, item)
					continue
				}
				if txt, ok := m["text"].(string); ok {
					csID, cmID, cleaned := extractTraceMarkers(txt)
					if csID != "" {
						sID = csID
						mID = cmID
						m["text"] = strings.TrimSpace(cleaned)
					}
				}
				cleanedArr = append(cleanedArr, item)
			}
			if sID != "" {
				msg["content"] = cleanedArr
				return sID, mID
			}
		}
	}
	return "", ""
}

// extractTraceMarkers 只扫 content 末尾，纯字符串查找提取 trace 标记。
// 不用正则，不受消息体大小影响。
func extractTraceMarkers(content string) (sessionID, msgId, cleaned string) {
	cleaned = content
	const sessionPrefix = "[TRACE:session="
	const msgPrefix = "[TRACE:msg="

	sidx := strings.LastIndex(content, sessionPrefix)
	if sidx < 0 {
		return "", "", content
	}
	midx := strings.LastIndex(content, msgPrefix)
	if midx < 0 || midx < sidx {
		return "", "", content
	}

	// session_id: sidx+len(prefix) 到下一个 ]
	sStart := sidx + len(sessionPrefix)
	sEnd := strings.Index(content[sStart:], "]")
	if sEnd < 0 {
		return "", "", content
	}
	sessionID = content[sStart : sStart+sEnd]

	// msg_id: midx+len(prefix) 到下一个 ]
	mStart := midx + len(msgPrefix)
	mEnd := strings.Index(content[mStart:], "]")
	if mEnd < 0 {
		return "", "", content
	}
	msgId = content[mStart : mStart+mEnd]

	// 剥离标记及其前面的空行
	stripStart := sidx
	for stripStart > 0 && (content[stripStart-1] == '\n' || content[stripStart-1] == '\r') {
		stripStart--
	}
	cleaned = content[:stripStart]
	return sessionID, msgId, cleaned
}

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

// injectWorkspaceRole 在 messages 最前方注入工作台角色定位 system prompt
// 类似 IDE 拼接提示词的方式，告诉大模型当前的角色定位和服务边界
//
// 核心设计原则（重要）：
//   工作台只服务不懂技术的业务人员。无论用户账面身份是什么（developer/business/admin），
//   只要他进了工作台，就一律按业务人员对待——不假设他懂技术，不开放技术工具调用。
//   真正的开发者用的是 VS Code / Trae 这类编辑器，不走工作台。
//   因此工作台的角色定位里不存在任何"开发者模式"分支，也不区分 UserType。
func injectWorkspaceRole(req map[string]interface{}, session *model.Session) {
	messages, ok := req["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return
	}

	// 角色定位 prompt（对所有进入工作台的用户统一生效，不区分身份）
	rolePrompt := `你是 RMP 系统的智能助手，服务于公司内部完全不懂技术的业务人员。
请假设你的用户：不懂编程、不懂数据库、不懂接口、不懂任何技术名词。他只会用电脑办公，看不懂技术方案。

【你的核心职责】
1. 业务知识问答：基于知识库中的操作手册，帮助用户理解系统功能、解决操作问题。
2. 需求收集：当用户表达了改进期望或新功能想法时，通过自然对话了解需求背景，然后调用 submit_requirement 技能提交。
3. Bug 收集：当用户反馈遇到系统问题、异常、报错时，通过自然对话收集 bug 信息（什么操作触发的、预期结果、实际结果），然后调用 submit_bug 技能提交。

【需求收集原则】
- 需求收集必须全程使用自然语言、大白话，像同事聊天一样。
- 你要和用户聊业务场景：在什么情况下需要？现在怎么做的（用大白话说流程，不要说字段名）？多久做一次？
- 当用户说清楚「想要什么 + 什么场景 + 现在的痛点」，就可以提交了。模糊也没关系，不要为了凑细节硬问。
- 回应要短。用户说"增加 Word 导出"，你回"好的，现在只能导 PDF，想加个 Word 格式对吧？主要是为了能编辑修改？"就行，不要整理成"一、背景 二、目标 三、功能说明"。
- 当用户补充完信息、确认需求后，可以输出一个汇总让他过目。汇总可以结构化（比如"1. xxx 2. xxx 3. xxx"这种形式），但内容必须是他能看懂的大白话。用户确认后立即调用 submit_requirement 技能提交。
- 业务人员能看懂业务流程图、业务表格、算账公式（如"金额 = 单价 × 数量"）。需要解释业务流程或算账规则时，可以用这些方式，让用户更容易理解。
- 业务人员可以做业务决策（如"匹配不上时停下来人工处理，还是用默认价？""一个工位上多产品时，按产品拆分还是合并？"）。这是他业务范围内的事，该问就问。
- 需求提交后，用户如果再对同一个需求提优化建议，你应该更新这条需求，而不是新建一条。调用 submit_requirement 技能时带上 requirement_id 参数即可更新。
- 如何判断是新增还是修改：
  - 上下文中有明确的需求 ID（比如刚提交过，从工具返回里能看到 id）：向用户确认"是对之前那个需求做修改吗？"，确认后带 requirement_id 调 action=submit
  - 上下文中没有需求 ID（比如新对话、或之前的需求被截断）：立即调 action=list 查用户最近的需求列表，然后把列表展示给用户让他选是改哪条还是新建
- 调用 submit_requirement 时一定要带上 action 参数：
  - 查列表：{ "action": "list" }（只传 action，不传其他参数）
  - 新增：{ "action": "submit", "title": "需求标题", "scenario": "场景", "pain_point": "痛点" }
  - 修改：{ "action": "submit", "requirement_id": 123, "title": "新标题", ... }

【绝对禁止的 6 项行为】（违反任何一项都是严重错误）
1. 禁止输出任何技术方案、技术规格、实现思路。你不是产品经理，不要出"更详细的方案"。特别禁止出现"我帮你出技术方案""确认完我出方案"这类承诺。
2. 禁止输出数据库字段名（如 product_id、user_id、status 等）。用"产品编号"、"用户"这种大白话代替。
3. 禁止让用户做技术决策（如"取最新一份还是按时间降序？""是快照还是实时？""用真实表格还是图片插入？"）。这些是开发决定的，用户说了算反而害了他。
4. 禁止承诺开发工作（如"我来实现""我来开发"）。你只负责把需求记下来提交。
5. 禁止把对话变成"需求规格说明书"（PRD）。以下形式都属于违规：
   - 列出"一、背景 / 二、目标 / 三、功能说明 / 四、需要确认"这种正式文档章节
   - 用大表格罗列功能点（入口位置/导出格式/生成方式/权限控制...）
   - 整理"需要确认"清单超过 3 条
   正确做法：可以输出结构化汇总（"1. xxx 2. xxx"这种），但格式要轻松，像聊天记录的总结，不要像产品文档。
6. 禁止用技术术语解释业务（如"通过外键关联""走消息队列""异步生成"）。业务人员听不懂这些。

【提交需求】
- 提交需求的唯一方式是调用 submit_requirement 技能。
- 不要自己用 MySQL 查数据来"完善"需求。
- 提交后告诉用户"已经记下了，会转给产品经理评估"，然后结束这个话题。

【语言风格】
- 用通俗的中文交流，像和同事聊天，不要写文档腔。
- 回答简洁，不要长篇大论。`

	// 构造 system 消息，插入到 messages 最前方
	roleMsg := map[string]interface{}{
		"role":    "system",
		"content": rolePrompt,
	}

	req["messages"] = append([]interface{}{roleMsg}, messages...)
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
	msgMaps = service.InjectGodRules(msgMaps, userID)
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

// injectRAGContext 检索知识库（Qdrant），将相关内容注入 system prompt 最前面
func injectRAGContext(messages []map[string]interface{}, userMsg string, userID int) []map[string]interface{} {
	if userMsg == "" || userID <= 0 {
		return messages
	}

	// Qdrant 向量检索（忽略用户隔离，全员共享，按项目过滤）
	results, err := service.SearchKnowledge(userMsg, 5, []string{"ai-os", "rmp", "general"}, nil)
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
