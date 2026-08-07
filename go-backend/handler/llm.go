package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/model"
	"ai-os-server/service"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
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

	// 注入上帝指令 + RAG（修复 B4：从请求头 X-AIOS-Project 读取项目提示，动态收窄检索范围）
	injectGodRulesAndRAG(req, userMsgRaw, userID, username, r.Header.Get("X-AIOS-Project"))

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
		// messages 类型不匹配时静默跳过优化（罕见分支，无需日志）
	}
	if len(msgMaps) > 0 {
		// 对话轮次管理（防止无限制累加）
		// 计算当前对话轮次（user 消息数）
		userRounds := 0
		for _, m := range msgMaps {
			if role, _ := m["role"].(string); role == "user" {
				userRounds++
			}
		}
		// 硬上限：超过 600 轮拒绝服务，防止截断本身也变成负担
		if userRounds > 600 {
			log.Printf("[chat] 对话已达 %d 轮，超过 600 轮硬上限，拒绝服务", userRounds)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": false,
				"error": "当前对话已超过 600 轮上限，无法继续处理。请开一个新对话窗口。\n" +
					"建议：让当前 AI 汇总一下之前的上下文要点，复制到新对话中继续。",
			})
			return
		}
		// 570 轮提醒：在最新 user 消息前追加提示（留 30 轮空隙让用户操作上下文切换）
		if userRounds >= 570 && userRounds <= 600 {
			lastUserIdx := -1
			for i := len(msgMaps) - 1; i >= 0; i-- {
				if role, _ := msgMaps[i]["role"].(string); role == "user" {
					lastUserIdx = i
					break
				}
			}
			if lastUserIdx >= 0 {
				warning := "\n\n[系统提醒] 当前对话已达 " + fmt.Sprintf("%d", userRounds) +
					" 轮，即将达到 600 轮上限。建议尽快让 AI 汇总之前的上下文要点，" +
					"复制到新对话窗口继续。达到 600 轮后将无法继续对话。"
				content := service.StringifyContent(msgMaps[lastUserIdx]["content"])
				msgMaps[lastUserIdx]["content"] = content + warning
				log.Printf("[chat] 对话已达 %d 轮，追加提醒", userRounds)
			}
		}
		msgMaps = service.OptimizeMessages(msgMaps, optimizeCfg)
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

	// 注入角色定位（工作台专属，在上帝指令和 RAG 之前）
	injectWorkspaceRole(req, session)

	// 注入上帝指令 + RAG（修复 B4：从请求头 X-AIOS-Project 读取项目提示，动态收窄检索范围）
	injectGodRulesAndRAG(req, userMsgRaw, userID, username, r.Header.Get("X-AIOS-Project"))

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
// projectHint: 可选，从请求头 X-AIOS-Project 读取，用于收窄 RAG 检索范围
func injectGodRulesAndRAG(req map[string]interface{}, userMsgRaw string, userID int, username string, projectHint string) {
	messages, ok := req["messages"].([]interface{})
	if !ok {
		return
	}
	msgMaps := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		msgMaps[i], _ = m.(map[string]interface{})
	}
	msgMaps = service.InjectGodRules(msgMaps, userID)
	msgMaps = injectRAGContext(msgMaps, extractUserQuery(userMsgRaw), userID, username, projectHint)

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

	// 5. 英文指令前缀截断：IDE 注入的系统提示词（intent. / When a skill / You are ...）
	//    是英文段落，真实用户问题是中文段落，从第一个中文段落截取到末尾
	cleaned = stripEnglishInstructionPrefix(cleaned)

	// 6. 网页内容过滤：IDE fetch 工具抓取的网页全文当 query 检索，严重污染向量
	//    典型特征：以 "Web page content:" 开头，或包含大量 HTML 标签/超链接
	if strings.HasPrefix(cleaned, "Web page content:") || strings.HasPrefix(cleaned, "Web Page Content:") {
		return "" // 网页全文不作为检索 query
	}
	// HTML 标签密度检测：如果 <a href / <div / <span 等标签占比过高，判定为 HTML 噪音
	if isHTMLHeavyContent(cleaned) {
		return ""
	}

	// 6b. <result> 标签过滤：AI 工具执行结果被当 query 检索
	if strings.HasPrefix(cleaned, "<result>") || strings.HasPrefix(cleaned, "<result ") {
		return ""
	}

	// 6c. 代码片段过滤：以代码语法开头的纯代码内容，无业务语义
	codePrefixes := []string{"//", "/*", "func ", "function ", "const ", "import ", "package ", "type "}
	for _, p := range codePrefixes {
		if strings.HasPrefix(cleaned, p) {
			return ""
		}
	}

	// 6d. JSON 报文过滤：API 返回的 JSON 数据被当 query 检索
	if strings.HasPrefix(cleaned, "{") || strings.HasPrefix(cleaned, "[{") {
		// 轻量检测：如果是 JSON 格式（含引号包裹的 key），判定为报文噪音
		if strings.Contains(cleaned, "\":") || strings.Contains(cleaned, "\",") {
			return ""
		}
	}

	// 6e. 操作日志过滤：带时间戳格式的运行日志（14:14:08音频上传成功...）
	logRe := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}`)
	if logRe.MatchString(cleaned) {
		return ""
	}

	// 6f. 纯文件路径过滤：反引号包裹的文件路径，或纯 Windows/Unix 路径
	if strings.HasPrefix(cleaned, "`") || strings.HasPrefix(cleaned, "d:\\") || strings.HasPrefix(cleaned, "D:\\") ||
		strings.HasPrefix(cleaned, "c:\\") || strings.HasPrefix(cleaned, "C:\\") ||
		strings.HasPrefix(cleaned, "/home/") || strings.HasPrefix(cleaned, "/opt/") {
		return ""
	}

	// 7. 纯闲聊过滤：过短或无业务语义的对话（"好的"、"没看到"等）
	//    只有 CJK 字符 < 8 且无代码/技术关键词的短消息不送检索
	runes := []rune(cleaned)
	cjk := countCJKChars(cleaned)
	if cjk < 8 && !hasTechnicalKeywords(cleaned) {
		return ""
	}
	_ = runes // 保持变量使用

	return cleaned
}

// isHTMLHeavyContent 检测内容是否以 HTML 标签为主（标签数占比高）
func isHTMLHeavyContent(text string) bool {
	htmlTags := regexp.MustCompile(`<(?:a|div|span|img|p|li|ul|ol|table|tr|td|th|br|hr|h[1-6])\b[^>]*>`)
	matches := htmlTags.FindAllString(text, -1)
	if len(matches) > 10 {
		return true // 超过10个HTML标签，判定为网页噪音
	}
	// 超链接密度：javascript:void 或 https:// 链接超过 5 个
	linkCount := strings.Count(text, "http") + strings.Count(text, "javascript:")
	return linkCount > 5
}

// hasTechnicalKeywords 检测是否包含技术/业务关键词（用于区分闲聊和业务查询）
func hasTechnicalKeywords(text string) bool {
	keywords := []string{
		"bug", "error", "fix", "deploy", "部署", "测试", "生产", "代码",
		"接口", "api", "sql", "数据库", "配置", "权限", "功能", "需求",
		"principle", "架构", "逻辑", "字段", "页面", "模块", "路由",
		"model", "prompt", "rag", "knowledge", "优化", "修复", "实现",
		"refactor", "review", "merge", "branch", "commit",
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// stripEnglishInstructionPrefix 截断 IDE 注入的英文指令前缀，提取真实中文用户意图
// 仅当文本较长（>200字符）时触发截断，短消息不动，避免误伤纯英文提问
func stripEnglishInstructionPrefix(text string) string {
	runes := []rune(text)
	if len(runes) < 200 {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		rl := len([]rune(line))
		if rl < 5 {
			continue
		}
		cjk := countCJKChars(line)
		if cjk > 0 && float64(cjk)/float64(rl) > 0.3 {
			result := strings.TrimSpace(strings.Join(lines[i:], "\n"))
			if len([]rune(result)) >= 5 {
				return result
			}
		}
	}
	return text
}

// countCJKChars 统计中日韩字符数量（含全角标点）
func countCJKChars(s string) int {
	count := 0
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK 统一汉字
			(r >= 0x3400 && r <= 0x4DBF) || // CJK 扩展A
			(r >= 0x3000 && r <= 0x303F) || // CJK 标点
			(r >= 0xFF00 && r <= 0xFFEF) { // 全角字符
			count++
		}
	}
	return count
}

// buildRAGProjects 构建检索范围
// 设计决策（2026-07-24）：取消 project 过滤，全库检索
// 原因：入库端无法准确判断 project（AI 只看到文本片段，缺乏上下文），
// 强行分类导致 93% 标为 general，project 过滤形同虚设。
// 改为全库检索 + 向量相似度排序，让 score 自己说话。
// projectHint 参数保留兼容但不再使用。
func buildRAGProjects(projectHint string) []string {
	return nil // 全库检索，不做 project 过滤
}

// cleanQueryNoise 清洗 query 噪音：剥离系统 prompt 残留、丢弃网页/日志垃圾
// 返回清洗后的 query；返回空字符串表示整条是噪音，应跳过检索
func cleanQueryNoise(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}

	// ① 网页爬取内容："Web page content:\n---\n..." — 整条丢弃
	if strings.HasPrefix(q, "Web page content") {
		return ""
	}

	// ② 运行日志：以 "HH:MM:SS" 时间戳开头 — 整条丢弃
	//    典型：15:59:33音频上传成功... 15:59:49开始处理...
	timeLogRe := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}`)
	if timeLogRe.MatchString(q) {
		return ""
	}

	// ③ 系统 prompt 前缀粘连：剥离前缀，保留后面的真实问题
	//    典型："intent. When a skill...IMMEDIATELY...\n\n[真实问题]"
	//    策略：找到第一个 \n\n，取后半段
	promptPrefixPatterns := []string{
		"intent. When a skill",
		"You are an interactive",
		"IMPORTANT: Assist with",
	}
	for _, prefix := range promptPrefixPatterns {
		if strings.HasPrefix(q, prefix) {
			// 找第一个双换行，取之后的内容
			idx := strings.Index(q, "\n\n")
			if idx >= 0 && idx < len(q)-2 {
				rest := strings.TrimSpace(q[idx+2:])
				if rest != "" {
					q = rest
				}
			}
			break // 只剥离一次
		}
	}

	return q
}

// injectRAGContext 检索知识库（Qdrant），将相关内容注入 system prompt 最前面
// projectHint: 保留兼容，当前不做 project 过滤（全库检索 + score 排序）
// 阈值：0.6（2026-07-24 从 0.5 提升，拦截低质量召回）
// 巡检采集：userID % 10 == 0 的用户采样记录检索日志（供每日巡检评审）
func injectRAGContext(messages []map[string]interface{}, userMsg string, userID int, username string, projectHint string) []map[string]interface{} {
	if userMsg == "" || userID <= 0 {
		return messages
	}

	// ── 噪音清洗：去除系统 prompt 残留、网页垃圾、运行日志 ──
	userMsg = cleanQueryNoise(userMsg)
	if userMsg == "" { // 清洗完空了，不查库了
		return messages
	}

	// 归一化 projectHint
	projects := buildRAGProjects(projectHint)

	// 多向量检索：将 query 拆分为多个语义片段，分路检索后用 RRF 融合
	// 优势：超长 prompt 不会被噪声淹没，短 prompt 有更多检索入口
	results, err := service.SearchKnowledgeMulti(userMsg, 10, projects, nil)
	if err != nil || len(results) == 0 {
		return messages
	}

	// 巡检采样标记：10% 随机采样
	shouldAudit := rand.Intn(10) == 0

	// 构建知识上下文
	var ctx strings.Builder
	ctx.WriteString("[HIGHEST PRIORITY - 知识库参考]\n")
	ctx.WriteString("以下内容来自企业知识库，在回答时必须优先参考：\n\n")
	count := 0
	for _, r := range results {
		// 阈值过滤：用 VectorScore（最高向量相似度）判断，不用 RRF 融合分数
		// RRF 分数是相对排序分，不能直接作为相似度阈值使用
		vecScore := r.VectorScore
		if r.Score > vecScore { // 兼容单向量检索（Score=VectorScore）
			vecScore = r.Score
		}
		if vecScore < 0.6 {
			continue
		}
		count++
		displayScore := vecScore
		if r.VectorScore > 0 && r.Score != r.VectorScore {
			// 多向量融合：显示 "RRF排序 / 最高向量分"
			displayScore = r.VectorScore
		}
		ctx.WriteString(fmt.Sprintf("--- 参考 %d（相似度 %.0f%%）---\n%s\n\n", count, displayScore*100, r.Content))

		// 巡检采集：记录最高向量相似度（vector_score），供日报检索质量 avg_score 使用
		if shouldAudit {
			service.LogRetrieveAudit(userID, username, userMsg, projects, r)
		}
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
