package service

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── 每日知识蒸馏 ──

// DistillResult 蒸馏结果
type DistillResult struct {
	Knowledge     []DistillKnowledge
	Suggestions   []DistillSuggestion
	DailySummary  string `json:"daily_summary"`
}

// DistillKnowledge 提炼的知识
type DistillKnowledge struct {
	Dimension string   `json:"dimension"`
	Title     string   `json:"title"`
	Context   string   `json:"context"`
	Content   string   `json:"content"`
	Priority  string   `json:"priority"`
	Tags      []string `json:"tags"`
}

// DistillSuggestion 优化建议
type DistillSuggestion struct {
	Category   string `json:"category"`
	Title      string `json:"title"`
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
	Priority   string `json:"priority"`
}

// RunDailyDistill 定时任务：蒸馏昨日对话（遍历所有活跃用户）
func RunDailyDistill() error {
	conn, err := GetDB()
	if err != nil {
		return fmt.Errorf("连接数据库失败: %v", err)
	}

	// 查询所有活跃用户
	rows, err := conn.Query("SELECT id, username FROM sys_user WHERE status = 1 ORDER BY id")
	if err != nil {
		return fmt.Errorf("查询用户列表失败: %v", err)
	}
	defer rows.Close()

	type userInfo struct {
		id       int
		username string
	}
	var users []userInfo
	for rows.Next() {
		var u userInfo
		if err := rows.Scan(&u.id, &u.username); err != nil {
			continue
		}
		users = append(users, u)
	}
	rows.Close()

	if len(users) == 0 {
		users = []userInfo{{id: 1, username: "default"}}
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	log.Printf("[cron] 共 %d 个用户需要蒸馏，日期=%s", len(users), yesterday)

	var lastErr error
	successCount := 0
	for _, u := range users {
		log.Printf("[cron] 开始蒸馏用户 %s (id=%d)", u.username, u.id)
		if err := RunDistillForDate(u.id, u.username, yesterday); err != nil {
			log.Printf("[cron] 用户 %s (id=%d) 蒸馏失败: %v", u.username, u.id, err)
			lastErr = err
			continue
		}
		successCount++
	}

	log.Printf("[cron] 蒸馏结束：成功 %d/%d", successCount, len(users))
	if successCount == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

// RunDistillForDate 指定日期蒸馏：查询对话 → 提炼知识 + 生成建议
func RunDistillForDate(userID int, username, date string) error {
	return RunDistillForDateWithTrigger(userID, username, date, "cron")
}

// RunDistillForDateWithTrigger 带触发类型的蒸馏
func RunDistillForDateWithTrigger(userID int, username, date, triggerType string) error {
	overallStart := time.Now()

	if userID <= 0 {
		var err error
		userID, err = getDefaultDistillUserID()
		if err != nil {
			distillLog(0, username, date, triggerType, "error", "fail", "获取默认用户失败: "+err.Error(), "", 0)
			return err
		}
	}
	if username == "" {
		username = getDistillUsername()
	}

	collectStart := time.Now()
	convContent, count, err := collectConversationForDate(userID, date)
	collectMs := int(time.Since(collectStart).Milliseconds())
	if err != nil {
		distillLog(userID, username, date, triggerType, "collect", "fail",
			"查询对话失败: "+err.Error(), "", collectMs)
		return fmt.Errorf("查询对话失败: %v", err)
	}

	if len(convContent) < 100 {
		distillLog(userID, username, date, triggerType, "collect", "info",
			fmt.Sprintf("无对话或对话过少，跳过。记录数=%d, 字符数=%d", count, len(convContent)), "", collectMs)
		return nil
	}

	distillLog(userID, username, date, triggerType, "collect", "success",
		fmt.Sprintf("记录数=%d, 字符数=%d", count, len(convContent)), "", collectMs)

	log.Printf("[distill] 开始蒸馏，日期=%s，user=%d，记录数=%d，字符数=%d", date, userID, count, len(convContent))

	conn, err := GetDB()
	if err != nil {
		distillLog(userID, username, date, triggerType, "error", "fail", "连接数据库失败: "+err.Error(), "", 0)
		return fmt.Errorf("连接数据库失败: %v", err)
	}

	// 分段调用 LLM 蒸馏
	log.Printf("[distill] 开始分段蒸馏，日期=%s，总字符数=%d", date, len(convContent))

	chunks := splitConversationIntoChunks(convContent, 25000)
	log.Printf("[distill] 分割为 %d 段", len(chunks))

	var allKnowledge []DistillKnowledge
	var allSuggestions []DistillSuggestion
	var allDailySummaries []string

	for i, chunk := range chunks {
		chunkStart := time.Now()
		result, err := callDistillLLM(userID, date, chunk)
		chunkMs := int(time.Since(chunkStart).Milliseconds())

		if err != nil {
			log.Printf("[distill] 第 %d/%d 段蒸馏失败: %v", i+1, len(chunks), err)
			distillLog(userID, username, date, triggerType, "llm_call", "fail",
				fmt.Sprintf("第 %d/%d 段失败: %v", i+1, len(chunks), err), "", chunkMs)
			continue
		}

		ds := result.DailySummary
		if len(ds) > 50 { ds = ds[:50] }
		log.Printf("[distill] 第 %d/%d 段完成：知识=%d，建议=%d，日报=%q", i+1, len(chunks), len(result.Knowledge), len(result.Suggestions), ds)
		distillLog(userID, username, date, triggerType, "llm_call", "success",
			fmt.Sprintf("第 %d/%d 段: 知识=%d, 建议=%d, 耗时=%dms", i+1, len(chunks), len(result.Knowledge), len(result.Suggestions), chunkMs),
			"", chunkMs)

		allKnowledge = append(allKnowledge, result.Knowledge...)
		allSuggestions = append(allSuggestions, result.Suggestions...)
		if result.DailySummary != "" {
			allDailySummaries = append(allDailySummaries, result.DailySummary)
		}
	}

	if len(allKnowledge) == 0 && len(allSuggestions) == 0 && len(allDailySummaries) == 0 {
		distillLog(userID, username, date, triggerType, "error", "fail", "所有段落蒸馏均失败", "", int(time.Since(overallStart).Milliseconds()))
		return fmt.Errorf("所有段落蒸馏均失败")
	}

	log.Printf("[distill] 分段蒸馏完成，汇总：知识=%d，建议=%d，日报段数=%d", len(allKnowledge), len(allSuggestions), len(allDailySummaries))

	// 存储知识（去重 + 向量化）
	knowledgeCount := 0
	knowledgeSkip := 0
	for _, k := range allKnowledge {
		if k.Content == "" || k.Dimension == "" {
			knowledgeSkip++
			continue
		}
		hash := contentHash(k.Content)
		var exists int
		conn.QueryRow("SELECT 1 FROM sys_embedding WHERE content_hash = ? AND user_id = ?", hash, userID).Scan(&exists)
		if exists == 1 {
			knowledgeSkip++
			continue
		}
		vector, err := GetEmbedding(k.Content)
		if err != nil {
			log.Printf("[distill] 向量化失败: %v", err)
			distillLog(userID, username, date, triggerType, "save_knowledge", "warn",
				"向量化失败: "+err.Error(), k.Title, 0)
			continue
		}
		vectorJSON, _ := json.Marshal(vector)
		source := fmt.Sprintf("distill:%s:%s:%s", date, k.Dimension, k.Title)
		conn.Exec(`INSERT INTO sys_embedding (user_id, content, content_hash, vector, source) VALUES (?, ?, ?, ?, ?)`,
			userID, k.Content, hash, string(vectorJSON), source)
		knowledgeCount++
	}
	distillLog(userID, username, date, triggerType, "save_knowledge", "success",
		fmt.Sprintf("保存=%d, 跳过=%d", knowledgeCount, knowledgeSkip), "", 0)

	// 存储建议
	suggestionCount := 0
	for _, s := range allSuggestions {
		if s.Title == "" || s.Suggestion == "" {
			continue
		}
		content := fmt.Sprintf("问题：%s\n\n建议：%s", s.Problem, s.Suggestion)
		conn.Exec(`INSERT INTO sys_ai_suggestion (user_id, report_date, category, title, content, priority) VALUES (?, ?, ?, ?, ?, ?)`,
			userID, date, s.Category, s.Title, content, s.Priority)
		suggestionCount++
	}
	distillLog(userID, username, date, triggerType, "save_suggestion", "success",
		fmt.Sprintf("保存=%d", suggestionCount), "", 0)

	// 存储工作日报
	if len(allDailySummaries) > 0 {
		summary := mergeDailyReports(allDailySummaries)
		title := fmt.Sprintf("%s 工作日报", date)
		log.Printf("[distill] 保存日报: user=%d, date=%s, content_len=%d", userID, date, len(summary))
		_, err = conn.Exec(`INSERT INTO sys_work_diary (user_id, username, report_date, title, content) VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE title=VALUES(title), content=VALUES(content), updated_at=NOW()`,
			userID, username, date, title, summary)
		if err != nil {
			log.Printf("[distill] 日报保存失败: %v", err)
			distillLog(userID, username, date, triggerType, "save_diary", "fail",
				"日报保存失败: "+err.Error(), "", 0)
		} else {
			log.Printf("[distill] 工作日报已保存，长度=%d", len(summary))
			distillLog(userID, username, date, triggerType, "save_diary", "success",
				fmt.Sprintf("日报长度=%d", len(summary)), "", 0)
		}
	} else {
		distillLog(userID, username, date, triggerType, "save_diary", "info", "无日报内容，跳过", "", 0)
	}

	totalMs := int(time.Since(overallStart).Milliseconds())
	log.Printf("[distill] 蒸馏完成：日期=%s，知识=%d，建议=%d，耗时=%dms", date, knowledgeCount, suggestionCount, totalMs)
	distillLog(userID, username, date, triggerType, "done", "success",
		fmt.Sprintf("知识=%d, 建议=%d, 耗时=%dms", knowledgeCount, suggestionCount, totalMs), "", totalMs)

	return nil
}

// mergeDailyReports 将多段日报合并为一份干净格式
// 逻辑：每个段落的 ### 子标题作为独立主题，收集所有段落中同主题的内容，去重后输出
// 不输出"明日计划"板块
func mergeDailyReports(segments []string) string {
	// 按主题分组收集内容
	// key=主题名(如"华庄客户全貌梳理"), value=去重后的条目集合
	topics := make(map[string]map[string]bool)

	// 一级分类名称规范化
	normalizeLevel1 := func(h string) string {
		h = strings.TrimSpace(h)
		switch {
		case strings.Contains(h, "今日完成") || strings.Contains(h, "今日工作") || strings.Contains(h, "工作内容") || strings.Contains(h, "今日产出"):
			return "今日完成"
		case strings.Contains(h, "遇到的问题") || strings.Contains(h, "问题与困难") || strings.Contains(h, "难点") || strings.Contains(h, "障碍"):
			return "遇到的问题"
		case strings.Contains(h, "明日计划") || strings.Contains(h, "明日工作") || strings.Contains(h, "后续计划") || strings.Contains(h, "工作计划"):
			return "明日计划" // 跳过，不输出
		}
		return ""
	}

	// 提取主题名（去除编号前缀如"1. ""2. ""- ""等）
	extractTopic := func(s string) string {
		s = strings.TrimSpace(s)
		// 去掉常见前缀
		s = strings.TrimPrefix(s, "1.")
		s = strings.TrimPrefix(s, "2.")
		s = strings.TrimPrefix(s, "3.")
		s = strings.TrimPrefix(s, "4.")
		s = strings.TrimPrefix(s, "5.")
		s = strings.TrimPrefix(s, "6.")
		s = strings.TrimPrefix(s, "7.")
		s = strings.TrimPrefix(s, "8.")
		s = strings.TrimPrefix(s, "9.")
		s = strings.TrimPrefix(s, ".")
		s = strings.TrimSpace(s)
		// 去掉多余空格
		for strings.Contains(s, "  ") {
			s = strings.ReplaceAll(s, "  ", " ")
		}
		return s
	}

	// 逐段解析
	for _, seg := range segments {
		lines := strings.Split(seg, "\n")
		currentL1 := ""  // 当前一级分类
		currentTopic := "" // 当前主题名

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "---" || trimmed == "***" {
				continue
			}

			// ## 一级标题
			if strings.HasPrefix(trimmed, "## ") {
				normalized := normalizeLevel1(strings.TrimPrefix(trimmed, "## "))
				if normalized == "__明日计划__" {
					currentL1 = "" // 跳过明日计划
					currentTopic = ""
					continue
				}
				currentL1 = normalized
				currentTopic = "" // 一级标题后重置主题
				continue
			}

			// ### 二级标题 → 新的主题
			if strings.HasPrefix(trimmed, "### ") {
				topicRaw := strings.TrimPrefix(trimmed, "### ")
				topicName := extractTopic(topicRaw)
				// 跳过明日计划相关标题
				if strings.Contains(strings.ToLower(topicName), "明日") || strings.Contains(strings.ToLower(topicName), "后续") {
					continue
				}
				if topicName != "" {
					currentTopic = topicName
					if topics[topicName] == nil {
						topics[topicName] = make(map[string]bool)
					}
				}
				continue
			}

			// 跳过明日计划下的列表项
			if currentL1 == "__明日计划__" || currentL1 == "" {
				continue
			}

			// 列表项
			item := strings.TrimLeft(trimmed, "-*·")
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			// 跳过明日计划相关的文本
			lower := strings.ToLower(item)
			if strings.Contains(lower, "明日") && (strings.Contains(lower, "计划") || strings.Contains(lower, "工作")) {
				continue
			}

			// 分配到当前主题；若无主题名，用一级分类名作主题
			// "明日计划" section 不输出
			if currentL1 == "明日计划" {
				currentTopic = "" // 跳过
				continue
			}
			if currentTopic == "" {
				if currentL1 == "" || currentL1 == "__明日计划__" {
					currentTopic = "其他工作"
				} else {
					currentTopic = currentL1
				}
				if topics[currentTopic] == nil {
					topics[currentTopic] = make(map[string]bool)
				}
			}

			// 去重后加入
			topics[currentTopic][item] = true
		}
	}

	// 重建 Markdown
	var out strings.Builder
	for topic, items := range topics {
		if len(items) == 0 {
			continue
		}
		out.WriteString("## ")
		out.WriteString(topic)
		out.WriteString("\n\n")
		for item := range items {
			out.WriteString("- ")
			out.WriteString(item)
			out.WriteString("\n")
		}
		out.WriteString("\n")
	}
	return strings.TrimSpace(out.String())
}

// splitConversationIntoChunks 按段落分割对话，每段不超过 maxLen 字符
func splitConversationIntoChunks(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	lines := strings.Split(content, "\n")
	var current strings.Builder

	for _, line := range lines {
		// 如果当前块加上这行超过限制，先保存当前块
		if current.Len()+len(line) > maxLen && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	// 处理最后一块
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	// 如果分段过多，每段取更多内容
	if len(chunks) > 10 {
		chunkLen := (len(content) / 8) + 1
		chunks = splitConversationIntoChunks(content, chunkLen)
	}

	return chunks
}

func getDefaultDistillUserID() (int, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, fmt.Errorf("连接数据库失败: %v", err)
	}
	var userID int
	conn.QueryRow("SELECT COALESCE(MIN(id), 1) FROM sys_user WHERE status = 1 LIMIT 1").Scan(&userID)
	if userID == 0 {
		userID = 1
	}
	return userID, nil
}

func getDistillUsername() string {
	conn, _ := GetDB()
	var username string
	conn.QueryRow("SELECT COALESCE(MAX(username), '') FROM sys_user WHERE status = 1 LIMIT 1").Scan(&username)
	return username
}

// collectConversationForDate 从 chat_history 收集指定用户指定日期的对话
func collectConversationForDate(userID int, date string) (string, int, error) {
	conn, err := GetDB()
	if err != nil {
		return "", 0, err
	}

	var convBuilder strings.Builder
	count := 0

	rows, err := conn.Query(`
		SELECT role, content, created_at FROM sys_chat_history
		WHERE user_id = ? AND DATE(created_at) = ?
		ORDER BY created_at ASC`, userID, date)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var role, content, createdAt string
		if err := rows.Scan(&role, &content, &createdAt); err != nil {
			continue
		}
		ts := createdAt
		if len(ts) > 16 {
			ts = ts[:16]
		}
		convBuilder.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, role, content))
		count++
	}

	// chat_history 不足时，从对话日志 JSON 补充（含完整 messages）
	if convBuilder.Len() < 100 {
		logCount, logContent := collectConversationFromLogs(userID, date)
		if logContent != "" {
			if convBuilder.Len() > 0 {
				convBuilder.WriteString("\n--- 对话日志补充 ---\n")
			}
			convBuilder.WriteString(logContent)
			count += logCount
		}
	}

	return convBuilder.String(), count, nil
}

func collectConversationFromLogs(userID int, date string) (int, string) {
	files, err := filepath.Glob(filepath.Join(CONVERSATION_LOG_DIR, "*.json"))
	if err != nil || len(files) == 0 {
		return 0, ""
	}

	var builder strings.Builder
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var logData map[string]interface{}
		if json.Unmarshal(data, &logData) != nil {
			continue
		}

		// 按用户过滤（user_id=0 的旧文件不属于任何用户，跳过）
		logUserID, _ := logData["user_id"].(float64)
		if int(logUserID) != userID {
			continue
		}

		ts, _ := logData["timestamp"].(string)
		if ts == "" || !strings.HasPrefix(ts, date) {
			continue
		}

		req, _ := logData["request"].(map[string]interface{})
		messages, _ := req["messages"].([]interface{})
		if len(messages) == 0 {
			continue
		}

		builder.WriteString(fmt.Sprintf("\n=== 对话 %s ===\n", ts[:19]))
		for _, m := range messages {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			role, _ := msg["role"].(string)
			content := extractMessageContent(msg["content"])
			if role == "" || content == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts[:16], role, content))
			count++
		}

		if resp, ok := logData["response"].(map[string]interface{}); ok {
			if content, _ := resp["content"].(string); content != "" {
				builder.WriteString(fmt.Sprintf("[%s] assistant: %s\n", ts[:16], content))
				count++
			}
		}
	}
	return count, builder.String()
}

func extractMessageContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprintf("%v", content)
	}
}

// callDistillLLM 调用智谱 GLM-5.2 进行蒸馏
func callDistillLLM(userID int, date, convContent string) (*DistillResult, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 获取智谱 API Key
	var apiKey string
	err = conn.QueryRow(`SELECT k.api_key FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("未找到智谱 API Key")
	}

	prompt := buildDistillPrompt(date, convContent)

	body := map[string]interface{}{
		"model":          "glm-5.2",
		"messages":       []map[string]string{{"role": "user", "content": prompt}},
		"temperature":    0.3,
		"response_format": map[string]string{"type": "json_object"},
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST",
		"https://open.bigmodel.cn/api/coding/paas/v4/chat/completions",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := SharedHTTPClientLong.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 调用失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		errMsg := fmt.Sprintf("API 返回 %d: %s", resp.StatusCode, string(respBody)[:min(500, len(respBody))])
		log.Printf("[distill] %s", errMsg)
		distillLog(userID, "", date, "cron", "llm_call", "fail", errMsg, string(respBody)[:min(1000, len(respBody))], 0)
		return nil, fmt.Errorf("%s", errMsg)
	}

	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		distillLog(userID, "", date, "cron", "json_parse", "fail", "响应解析失败: "+err.Error(), "", 0)
		return nil, fmt.Errorf("响应解析失败: %v", err)
	}

	content := ""
	if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				content, _ = msg["content"].(string)
			}
		}
	}
	if content == "" {
		return nil, fmt.Errorf("AI 未返回有效内容")
	}

	// 解析 JSON（处理 LLM 返回的 markdown 代码块等情况）
	result := &DistillResult{}
	cleanContent := strings.TrimSpace(content)

	// 去除 markdown 代码块标记（含语言标识）
	if idx := strings.Index(cleanContent, "```"); idx >= 0 {
		// 找到 ``` 之后第一个换行，去掉整行
		rest := cleanContent[idx+3:]
		if nl := strings.Index(rest, "\n"); nl >= 0 {
			rest = rest[nl+1:]
		}
		// 去掉尾部 ```
		if last := strings.LastIndex(rest, "```"); last >= 0 {
			rest = rest[:last]
		}
		cleanContent = strings.TrimSpace(rest)
	}

	// 用花括号计数找到 JSON 边界（感知字符串，避免字符串内的花括号干扰）
	jsonBlock := extractJSONBlock(cleanContent)
	if jsonBlock == "" {
		jsonBlock = cleanContent
	}

	// 尝试直接解析
	if err := json.Unmarshal([]byte(jsonBlock), result); err != nil {
		log.Printf("[distill] JSON 解析失败: %v，原始内容前300字符: %s", err, content[:min(300, len(content))])
		distillLog(userID, "", date, "cron", "json_parse", "fail",
			"JSON 解析失败: "+err.Error(),
			content[:min(500, len(content))], 0)
	}

	// 检查是否有有效数据
	if len(result.Knowledge) == 0 && len(result.Suggestions) == 0 && result.DailySummary == "" {
		log.Printf("[distill] 解析结果为空，原始内容前300字符: %s", content[:min(300, len(content))])
		distillLog(userID, "", date, "cron", "json_parse", "warn",
			"解析结果为空",
			content[:min(500, len(content))], 0)
	}

	return result, nil
}

// extractJSONBlock 用花括号计数提取最外层 JSON 对象（感知字符串）
func extractJSONBlock(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		} else if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// buildDistillPrompt 构建蒸馏 prompt
func buildDistillPrompt(date, convContent string) string {
	return fmt.Sprintf(`你是用户的专属 AI 效能教练。以下是用户今天（%s）的完整工作对话记录。

请基于对话内容，提炼两类产出：

## 一、知识蒸馏（knowledge）
从今天的对话中提炼有长期参考价值的知识，分四个维度：

- decisions（决策）：技术选型、架构决策、方案取舍。必须包含：面临什么问题、有哪些选项、为什么选这个、否定了什么
- pitfalls（踩坑）：踩坑记录、bug根因。必须包含：触发现象、根因、解法、预防措施
- business（业务）：业务规则、表结构、领域知识。必须包含：业务背景、适用场景
- habits（习惯）：用户的工作习惯和偏好。必须包含：从哪些对话观察到的

每条知识的 content 要自包含——脱离今天的对话上下文，单独看这条知识也能理解。
context 字段必须包含背景：当时面临什么问题、为什么做这个决策。不要只写结论。
如果某个维度今天没有有价值的发现，不要硬编，跳过该维度即可。

## 二、工作日报（daily_summary）
以结构化 Markdown 格式总结今天的工作成果：
- **今日完成**：列出今天完成的主要工作，用列表呈现
- **遇到的问题**：记录遇到的技术难点、业务问题及解决思路
- **明日计划**：基于今天的进展，预判明天的工作方向
只总结有价值的内容，避免流水账。如果对话内容很少，可以只输出简短的日报。

## 三、优化建议（suggestions）
只提两类建议：规则和技能。这两类可以直接落地执行。

### 规则建议（rule）
当 AI 反复犯同类错误时，指出：
- 犯了什么错（引用具体对话）
- 错误模式是什么（偶发还是反复）
- 建议约束什么（不要替用户写规则文本，只描述问题和建议方向）

### 技能建议（skill）
当发现重复操作或技能缺陷时，指出：
- 什么操作在重复（引用具体对话）
- 哪个技能有问题、问题是什么
- 建议封装什么 / 修改什么（描述方向，不给具体代码）

### Bug 归因（bug）
当对话中出现 bug 或错误时，指出：
- 什么错误、根因是什么
- 怎么修复的或建议怎么修复
- 怎么预防

### 技术视野（tech_vision）
当发现用户的实现方式较为原始、有更成熟的替代方案时：
- 指出当前做法（引用对话中的具体实现）
- 指出业界主流做法或更优方案
- 说明换方案能带来什么提升（性能/可维护性/开发效率）
- 注意：只在确实有显著更优方案时才提，不要为了凑数而建议"换框架"这种大动作

要求：
- 只输出你确信有价值的建议，宁缺毋滥
- 每条建议和问题必须引用今天的具体对话作为依据
- 不要替用户写最终方案，只描述问题和方向

## 输出格式（严格 JSON，不要输出其他内容）
{
  "knowledge": [
    {
      "dimension": "decisions|pitfalls|business|habits",
      "title": "简洁的标题",
      "context": "背景：当时的情况、面临的问题、为什么",
      "content": "提炼出的知识正文，要自包含",
      "priority": "high|medium|low",
      "tags": ["标签1", "标签2"]
    }
  ],
  "suggestions": [
    {
      "category": "rule|skill|bug|tech_vision",
      "title": "一句话概括",
      "problem": "观察到的具体问题，引用对话证据",
      "suggestion": "具体建议方向，点到为止，不给最终方案",
      "priority": "high|medium|low"
    }
  ],
  "daily_summary": "Markdown 格式的工作日报，包含今日完成、遇到的问题、明日计划"
}

## 今天（%s）的对话记录：
%s`, date, date, convContent)
}

// GetDistillSuggestions 获取用户的蒸馏建议
func GetDistillSuggestions(userID int, date string) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT id, user_id, report_date, category, title, content, priority, status, created_at FROM sys_ai_suggestion"
	var conditions []string
	var args []interface{}
	conditions = append(conditions, "user_id = ?")
	args = append(args, userID)
	if date != "" {
		conditions = append(conditions, "report_date = ?")
		args = append(args, date)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id DESC"

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, uid int
		var reportDate, category, title, content, priority, status2, createdAt string
		if err := rows.Scan(&id, &uid, &reportDate, &category, &title, &content, &priority, &status2, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "user_id": uid, "report_date": reportDate, "category": category,
			"title": title, "content": content, "priority": priority,
			"status": status2, "created_at": createdAt,
		})
	}
	return result, nil
}

// EnsureDistillTable 确保建议表有 category 字段
func EnsureDistillTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	// 确保 sys_ai_suggestion 表存在（已在 suggestion.go 创建，此处只做字段补充）
	conn.Exec("CREATE TABLE IF NOT EXISTS sys_ai_suggestion (" +
		"id INT AUTO_INCREMENT PRIMARY KEY," +
		"user_id INT NOT NULL DEFAULT 0," +
		"report_date DATE NOT NULL," +
		"category VARCHAR(50) NOT NULL," +
		"project VARCHAR(200) DEFAULT ''," +
		"title VARCHAR(500) NOT NULL," +
		"content TEXT NOT NULL," +
		"priority VARCHAR(20) DEFAULT 'medium'," +
		"status VARCHAR(20) DEFAULT 'pending'," +
		"created_at DATETIME DEFAULT CURRENT_TIMESTAMP," +
		"processed_at DATETIME NULL," +
		"INDEX idx_user_id (user_id)," +
		"INDEX idx_date (report_date)," +
		"INDEX idx_status (status)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4")
}

// DistillKnowledgeByDate 获取指定日期的用户蒸馏知识
func DistillKnowledgeByDate(userID int, date string) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := `SELECT id, dimension, title, context, content, priority, source, created_at FROM (
		SELECT id, 
			CASE 
				WHEN source LIKE 'distill:%%:decisions:%%' THEN 'decisions'
				WHEN source LIKE 'distill:%%:pitfalls:%%' THEN 'pitfalls'
				WHEN source LIKE 'distill:%%:business:%%' THEN 'business'
				WHEN source LIKE 'distill:%%:habits:%%' THEN 'habits'
				ELSE 'other'
			END as dimension,
			REPLACE(REPLACE(SUBSTRING_INDEX(source, ':', -1), '_', ' '), '-', ' ') as title,
			'' as context,
			content,
			'medium' as priority,
			source,
			created_at
		FROM sys_embedding 
		WHERE user_id = ? AND source LIKE 'distill:%%'
		%s
	) t ORDER BY created_at DESC LIMIT 50`

	var rows *sql.Rows
	var err2 error
	if date != "" {
		rows, err2 = conn.Query(fmt.Sprintf(query, "AND source LIKE ?"), userID, "distill:"+date+":%")
	} else {
		rows, err2 = conn.Query(fmt.Sprintf(query, ""), userID)
	}
	if err2 != nil {
		return nil, err2
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var dimension, title, context, content, priority, source, createdAt string
		if err := rows.Scan(&id, &dimension, &title, &context, &content, &priority, &source, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "dimension": dimension, "title": title,
			"context": context, "content": content,
			"priority": priority, "source": source, "created_at": createdAt,
		})
	}
	return result, nil
}

// ── 蒸馏日志（持久化到 sys_distill_log）──

// distillLog 写一条蒸馏日志到数据库
// stage: collect / llm_call / json_parse / save_knowledge / save_suggestion / save_diary / done / error
// status: success / fail / warn / info
// detail: 详细信息（错误内容、原始响应等），可空
func distillLog(userID int, username, date, triggerType, stage, status, message, detail string, durationMs int) {
	conn, err := GetDB()
	if err != nil || conn == nil {
		log.Printf("[distill-log] DB 连接失败，降级 stdout: user=%d date=%s stage=%s status=%s msg=%s",
			userID, date, stage, status, message)
		return
	}
	_, err = conn.Exec(
		`INSERT INTO sys_distill_log (user_id, username, report_date, trigger_type, stage, status, message, detail, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, date, triggerType, stage, status, message, detail, durationMs,
	)
	if err != nil {
		log.Printf("[distill-log] 写入失败: %v", err)
	}
}

// GetDistillLogs 查询蒸馏日志（管理员可查所有用户，普通用户只查自己）
func GetDistillLogs(userID int, isAdmin bool, date string, limit int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	query := `SELECT id, user_id, username, report_date, trigger_type, stage, status, message, detail, duration_ms, created_at
		FROM sys_distill_log`
	var conditions []string
	var args []interface{}

	if !isAdmin {
		conditions = append(conditions, "user_id = ?")
		args = append(args, userID)
	}
	if date != "" {
		conditions = append(conditions, "report_date = ?")
		args = append(args, date)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"
	if limit > 0 && limit <= 500 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	} else {
		query += " LIMIT 100"
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, uid, dur int
		var username, reportDate, triggerType, stage, status, message, detail, createdAt string
		if err := rows.Scan(&id, &uid, &username, &reportDate, &triggerType, &stage, &status, &message, &detail, &dur, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":           id,
			"user_id":      uid,
			"username":     username,
			"report_date":  reportDate,
			"trigger_type": triggerType,
			"stage":        stage,
			"status":       status,
			"message":      message,
			"detail":       detail,
			"duration_ms":  dur,
			"created_at":   createdAt,
		})
	}
	return result, nil
}
