package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── 每日知识蒸馏（Map-Reduce 架构）──

// SessionData 一个 session 的对话数据
type SessionData struct {
	SessionID  string      `json:"session_id"`
	Msgs       []MsgGroup  `json:"msgs"`
	TotalChars int         `json:"total_chars"`
}

// MsgGroup 一个 msg_id 下的对话内容
type MsgGroup struct {
	MsgID   string `json:"msg_id"`
	Content string `json:"content"`
}

// DistillResult 蒸馏结果
type DistillResult struct {
	Knowledge      []DistillKnowledge  `json:"knowledge"`
	Suggestions    []DistillSuggestion `json:"suggestions"`
	DailySummary   string              `json:"daily_summary"`
	SessionSummary string              `json:"session_summary"` // 当前 session 摘要，≤500字，用于下一片前情提要
}

// DistillKnowledge 提炼的知识
type DistillKnowledge struct {
	Dimension string   `json:"dimension"`
	Title     string   `json:"title"`
	Context   string   `json:"context"`
	Content   string   `json:"content"`
	Priority  string   `json:"priority"`
	Tags      []string `json:"tags"`
	Project   string   `json:"project"`
}

// DistillSuggestion 优化建议
type DistillSuggestion struct {
	Category   string `json:"category"`
	Title      string `json:"title"`
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
	Priority   string `json:"priority"`
}

// ── Map 阶段汇总 ──

// mapResult 单个 session 的 Map 阶段产出
type mapResult struct {
	SessionID      string
	Knowledge      []DistillKnowledge
	Suggestions    []DistillSuggestion
	DailySummary   string
	SessionSummary string // 整个 session 的摘要
}

// ── 定时任务入口 ──

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

// ══════════════════════════════════════════════
// LLM 调用 + Prompt
// ══════════════════════════════════════════════

// buildDistillPrompt 构建 Map 阶段蒸馏 Prompt
func buildDistillPrompt(date, convContent string) string {
	return fmt.Sprintf(`你是一个技术知识提炼专家。请分析以下用户在 %s 的一段完整工作对话，提炼知识、生成日报片段和会话摘要。

【输出要求】
返回一个 JSON 对象：
{
  "knowledge": [                       // 提炼的知识条目
    {
      "dimension": "技术规范/架构决策/开发流程/Bug修复/工具技巧/环境配置",
      "title": "知识标题（≤30字）",
      "context": "这条知识产生的原因和背景",
      "content": "知识的具体内容（≤1500字，超过请拆分为多条）",
      "priority": "high|medium|low（high=阻断级Bug修复/核心架构决策/重大数据变更，medium=常规规范/工具技巧/流程说明，low=辅助性备注）",
      "tags": ["标签1", "标签2"],
      "project": "项目名称（ai-os/rmp/general 三选一，未知填 general）"
    }
  ],
  "suggestions": [                     // 工作流程优化建议
    {
      "category": "skill|bug|tech_vision|rule|workflow|env|prompt|other",
      "title": "建议标题",
      "problem": "当前存在的问题",
      "suggestion": "具体的改进建议",
      "priority": "high|medium|low"
    }
  ],
  "daily_summary": "今天这个对话 session 的工作摘要（100-500字）",
  "session_summary": "这个对话 session 的简洁摘要（≤500字），用于给下一个分片提供上下文。如果对话内容很少，可以不填"
}

【重要规则】
1. 每条 knowledge.content 必须 ≤ 1500 个中文字符，超过请拆分为多条
2. 只提炼有价值的技术知识，不要提炼闲聊内容
3. suggestions 的 title、problem、suggestion 三个字段都必须填写，不能留空或省略。title 用简短概括的一句话（≤30字），problem 描述当前存在的问题（不能只重复 title），suggestion 给出具体可执行的改进方案
4. suggestions.category 必须使用以下枚举值之一：skill(技能封装)、bug(Bug归因)、tech_vision(技术视野)、rule(规则加强)、workflow(流程工具)、env(环境配置)、prompt(提示词优化)、other(其他)。严禁使用其他值
5. suggestions.priority 必须使用以下枚举值之一：high(高)、medium(中)、low(低)
6. 每条 knowledge 的 content 字段必须自成一体、可独立被理解，不得使用"如前所述"等指代性表述
7. 当提供了 session 摘要时，仅从中提取知识概要，不逐句复述
8. 不要编造对话中不存在的内容
9. session_summary 用于连接多段对话，必须概括当前对话的核心内容
10. knowledge.dimension 必须使用以下六个中文枚举值之一（严禁改写、严禁大小写变化、严禁用英文）：
    - 技术规范：代码规范、命名约定、编码风格、最佳实践
    - 架构决策：技术选型、模块划分、设计模式、架构权衡
    - 开发流程：开发步骤、部署流程、发布流程、协作流程
    - Bug修复：具体 Bug 的根因分析与修复方案
    - 工具技巧：IDE、脚本、命令行、工具链的使用技巧
    - 环境配置：开发/测试/生产环境配置、依赖管理、端口分配
    无法归类的内容不要提炼为知识（直接跳过），严禁编造未在枚举里的维度
11. knowledge.project 必须使用以下三个枚举值之一（严禁大小写变化、严禁用中文、严禁用其他值）：
    - ai-os：AI-OS 项目相关（Go 后端、Web 前端、桌面端、代理网关、知识库、技能、LLM 路由）
    - rmp：RMP 系统相关（rmp-api、chartsapi、socket、nnd-robot、rmp-prd、nnd-flow-api）
    - general：通用知识、与具体项目无关、或无法明确判断归属
    严禁输出 "AI-OS"（大写）、"rmp-api"、"rmp-prd"、"nnd-robot"、"未知" 等非枚举值

【对话内容】
%s`, date, convContent)
}

// buildReducePrompt 构建 Reduce 阶段精炼 Prompt
func buildReducePrompt(date, reduceContent string) string {
	return fmt.Sprintf(`你是一个技术知识精炼专家。以下是从用户 %s 的多个对话 session 中独立蒸馏出的知识、摘要和日报片段。

请进行二次精炼，完成以下任务：
1. 合并重复或相似的知识条目
2. 补全碎片化知识的因果关系（不同 session 讨论了同一件事的不同方面）
3. 跨 session 发现工作脉络
4. 生成一份完整的工作日报

【输出要求】
返回一个 JSON 对象：
{
  "knowledge": [...],        // 精炼后的知识条目（格式同前）
  "suggestions": [...],      // 精炼后的建议（格式同前）
  "daily_summary": "今天所有对话的工作日报（200-1000字）"
}

【重要规则】
1. suggestions 的 title、problem、suggestion 三个字段都必须填写，不能留空或省略
2. suggestions.category 必须使用以下枚举值之一：skill、bug、tech_vision、rule、workflow、env、prompt、other
3. suggestions.priority 必须使用以下枚举值之一：high、medium、low
4. 每条 knowledge.content 必须 ≤ 1500 个中文字符
5. 只输出 JSON，不要添加任何解释文字
6. knowledge.dimension 必须使用以下六个中文枚举值之一（严禁大小写变化、严禁用英文）：技术规范 / 架构决策 / 开发流程 / Bug修复 / 工具技巧 / 环境配置
7. knowledge.project 必须使用以下三个枚举值之一（严禁大小写变化、严禁用中文、严禁用其他值）：ai-os / rmp / general

【原始蒸馏结果】
%s`, date, reduceContent)
}

// callDistillLLM Map 阶段 LLM 调用
func callDistillLLM(userID int, date, convContent string) (*DistillResult, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	var apiKey, baseURL string
	err = conn.QueryRow(`SELECT k.api_key, v.base_url FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey, &baseURL)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("未找到智谱 API Key")
	}
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/chat/completions"

	prompt := buildDistillPrompt(date, convContent)

	body := map[string]interface{}{
		"model":          "glm-5.2",
		"messages":       []map[string]string{{"role": "user", "content": prompt}},
		"temperature":    0.3,
		"response_format": map[string]string{"type": "json_object"},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL,
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := SharedHTTPClientLong
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求智谱 API 失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		detail := string(respBody)
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return nil, fmt.Errorf("智谱 API 返回 %d: %s", resp.StatusCode, detail)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("AI 未返回有效内容")
	}

	content := result.Choices[0].Message.Content

	distillResult, err := parseDistillResult(content)
	if err != nil {
		log.Printf("[distill] JSON 解析失败: %v, content[:200]=%s", err, content[:min(200, len(content))])
		return nil, err
	}

	if len(distillResult.Knowledge) == 0 && len(distillResult.Suggestions) == 0 && distillResult.DailySummary == "" {
		log.Printf("[distill] LLM 返回空结果: content[:200]=%s", content[:min(200, len(content))])
	}

	return distillResult, nil
}

// callReduceLLM Reduce 阶段 LLM 调用
func callReduceLLM(userID int, date, reduceContent string) (*DistillResult, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	var apiKey, baseURL string
	err = conn.QueryRow(`SELECT k.api_key, v.base_url FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey, &baseURL)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("未找到智谱 API Key")
	}
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/chat/completions"

	prompt := buildReducePrompt(date, reduceContent)

	body := map[string]interface{}{
		"model":          "glm-5.2",
		"messages":       []map[string]string{{"role": "user", "content": prompt}},
		"temperature":    0.3,
		"response_format": map[string]string{"type": "json_object"},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL,
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := SharedHTTPClientLong
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求智谱 API 失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		detail := string(respBody)
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return nil, fmt.Errorf("智谱 API 返回 %d: %s", resp.StatusCode, detail)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("AI 未返回有效内容")
	}

	content := result.Choices[0].Message.Content

	distillResult, err := parseDistillResult(content)
	if err != nil {
		log.Printf("[distill] Reduce JSON 解析失败: %v, content[:200]=%s", err, content[:min(200, len(content))])
		return nil, err
	}

	return distillResult, nil
}

// extractJSONBlock 从内容中提取 JSON 块（去除 markdown 标记）
func extractJSONBlock(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}
	// 兜底：取第一个 { 到最后一个 } 之间的内容
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	return content
}

// cleanJSONForParsing 清洗 LLM 返回 JSON 的常见非法字符
// 解决 GLM 偶尔在 JSON 中输出 \w \s \d 等非标准转义、以及多余尾部字符
func cleanJSONForParsing(content string) string {
	// 1. 去除非打印字符（保留空格、换行、制表符）
	var b strings.Builder
	for _, r := range content {
		if r >= 32 && r < 127 || r == '\n' || r == '\r' || r == '\t' {
			b.WriteRune(r)
		} else if r >= 0x4E00 && r <= 0x9FFF { // 中文字符
			b.WriteRune(r)
		} else if r >= 0x3000 && r <= 0x303F { // 中文标点
			b.WriteRune(r)
		} else if r >= 0xFF00 && r <= 0xFFEF { // 全角字符
			b.WriteRune(r)
		}
	}
	content = b.String()

	// 2. 修复非标准 JSON 转义（\w \s \d 等正则转义）
	//    在 JSON 字符串值内部，这些需要双写反斜杠
	invalidEscapes := []string{`\w`, `\s`, `\d`, `\b`, `\a`, `\v`, `\x`}
	for _, esc := range invalidEscapes {
		content = strings.ReplaceAll(content, esc, `\\`+esc[1:])
	}

	return content
}

// parseDistillResult 尝试解析 LLM 返回的蒸馏结果，带多级降级
func parseDistillResult(content string) (*DistillResult, error) {
	// 第1级：直接解析
	var result DistillResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return &result, nil
	}

	// 第2级：extractJSONBlock 后解析
	clean := extractJSONBlock(content)
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		return &result, nil
	}

	// 第3级：字符清洗后解析
	clean = cleanJSONForParsing(content)
	if err := json.Unmarshal([]byte(clean), &result); err == nil {
		log.Printf("[distill] JSON 经清洗后解析成功")
		return &result, nil
	}

	return nil, fmt.Errorf("JSON 解析失败（3级降级均失败）")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ══════════════════════════════════════════════
// 日志 + 存储
// ══════════════════════════════════════════════

// distillLog 记录蒸馏日志到 sys_distill_log
func distillLog(userID int, username, date, triggerType, stage, status, message, detail string, durationMs int, sessionID string) {
	conn, err := GetDB()
	if err != nil || conn == nil {
		log.Printf("[distill-log] DB 连接失败，降级 stdout: [user=%d] [stage=%s] [status=%s] %s", userID, stage, status, message)
		return
	}
	_, err = conn.Exec(
		`INSERT INTO sys_distill_log (user_id, username, report_date, trigger_type, stage, status, message, detail, duration_ms, session_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, date, triggerType, stage, status, message, detail, durationMs, sessionID,
	)
	if err != nil {
		log.Printf("[distill-log] 写入失败: %v", err)
	}
}

// storeSuggestion 保存建议到 sys_ai_suggestion
func storeSuggestion(userID int, date string, s DistillSuggestion) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	// 拼装 content
	content := strings.TrimSpace(s.Problem)
	if strings.TrimSpace(s.Suggestion) != "" {
		if content != "" {
			content += "\n\n建议：" + strings.TrimSpace(s.Suggestion)
		} else {
			content = strings.TrimSpace(s.Suggestion)
		}
	}
	if content == "" {
		log.Printf("[distill] 跳过空建议: title=%s category=%s", s.Title, s.Category)
		return 0, nil
	}

	// title 兜底：LLM 没填则从 content 截取前 30 字
	title := strings.TrimSpace(s.Title)
	if title == "" {
		runes := []rune(content)
		if len(runes) > 30 {
			title = string(runes[:30])
		} else {
			title = string(runes)
		}
		log.Printf("[distill] title 为空，从 content 截取: %s", title)
	}

	result, err := conn.Exec(
		`INSERT INTO sys_ai_suggestion (user_id, report_date, category, project, title, content, priority)
		 VALUES (?, ?, ?, '未知', ?, ?, ?)`,
		userID, date, s.Category, title, content, s.Priority,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// storeWorkDiary 保存工作日报到 sys_work_diary
func storeWorkDiary(userID int, date, summary string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	// 表结构：user_id, username, report_date, title, content
	title := fmt.Sprintf("工作日报 %s", date)
	result, err := conn.Exec(
		`INSERT INTO sys_work_diary (user_id, username, report_date, title, content)
		 VALUES (?, '', ?, ?, ?)
		 ON DUPLICATE KEY UPDATE title = VALUES(title), content = VALUES(content)`,
		userID, date, title, summary,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
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

// EnsureDistillTable 确保蒸馏相关表存在
func EnsureDistillTable() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_ai_suggestion (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL,
		report_date DATE NOT NULL,
		category VARCHAR(50) DEFAULT '',
		title VARCHAR(200) DEFAULT '',
		problem TEXT,
		suggestion TEXT,
		priority VARCHAR(10) DEFAULT '中',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_user_date (user_id, report_date)
	)`)
}

// GetDistillLogs 查询蒸馏日志
func GetDistillLogs(userID int, isAdmin bool, date string, limit int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var query string
	var args []interface{}

	if isAdmin {
		query = `SELECT id, user_id, username, report_date, trigger_type, stage, status, message, detail, duration_ms, session_id, created_at FROM sys_distill_log`
		if date != "" {
			query += " WHERE report_date = ?"
			args = append(args, date)
		}
		query += " ORDER BY id DESC LIMIT ?"
	} else {
		query = `SELECT id, user_id, username, report_date, trigger_type, stage, status, message, detail, duration_ms, session_id, created_at FROM sys_distill_log WHERE user_id = ?`
		args = append(args, userID)
		if date != "" {
			query += " AND report_date = ?"
			args = append(args, date)
		}
		query += " ORDER BY id DESC LIMIT ?"
	}
	args = append(args, limit)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, uid, dur int
		var username, reportDate, triggerType, stage, status, message, detail, sessionID string
		var createdAt time.Time
		if err := rows.Scan(&id, &uid, &username, &reportDate, &triggerType, &stage, &status, &message, &detail, &dur, &sessionID, &createdAt); err != nil {
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
			"session_id":   sessionID,
			"created_at":   createdAt.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// DistillKnowledgeByDate 查询指定日期的蒸馏知识
func DistillKnowledgeByDate(userID int, date string) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`
		SELECT id, title, content, context, category, priority, tags, project, session_id, created_at
		FROM sys_knowledge
		WHERE user_id = ? AND report_date = ? AND status = 'active'
		ORDER BY id DESC`, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var title, content, context, category, priority, tags, project, sessionID string
		var createdAt time.Time
		if err := rows.Scan(&id, &title, &content, &context, &category, &priority, &tags, &project, &sessionID, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":         id,
			"title":      title,
			"content":    content,
			"context":    context,
			"category":   category,
			"priority":   priority,
			"tags":       tags,
			"project":    project,
			"session_id": sessionID,
			"created_at": createdAt.Format("2006-01-02 15:04:05"),
		})
	}
	return result, nil
}

// ══════════════════════════════════════════════
// 辅助函数
// ══════════════════════════════════════════════

// collectSessionsForDate 收集指定用户指定日期的 session 对话（无 session_id 的跳过）
func collectSessionsForDate(userID int, date string) ([]SessionData, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`
		SELECT session_id, msg_id, role, content, created_at
		FROM sys_chat_history
		WHERE user_id = ? AND DATE(created_at) = ? AND session_id != ''
		ORDER BY session_id, created_at ASC`, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessionMap := make(map[string][]MsgGroup)
	msgContentMap := make(map[string]map[string]*strings.Builder) // session_id -> msg_id -> content builder
	sessionOrder := []string{}                                     // 保持 session 出现顺序
	msgOrderMap := make(map[string][]string)                       // session_id -> msg_id 顺序

	for rows.Next() {
		var sessionID, msgID, role, content, createdAt string
		if err := rows.Scan(&sessionID, &msgID, &role, &content, &createdAt); err != nil {
			continue
		}

		// 初始化 session
		if _, exists := sessionMap[sessionID]; !exists {
			sessionMap[sessionID] = []MsgGroup{}
			msgContentMap[sessionID] = make(map[string]*strings.Builder)
			msgOrderMap[sessionID] = []string{}
			sessionOrder = append(sessionOrder, sessionID)
		}

		// 初始化 msg
		if _, exists := msgContentMap[sessionID][msgID]; !exists {
			msgContentMap[sessionID][msgID] = &strings.Builder{}
			msgOrderMap[sessionID] = append(msgOrderMap[sessionID], msgID)
		}

		// 追加内容
		ts := createdAt
		if len(ts) > 16 {
			ts = ts[:16]
		}
		msgContentMap[sessionID][msgID].WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, role, content))
	}

	// 构建 SessionData 列表
	var sessions []SessionData
	for _, sid := range sessionOrder {
		var msgs []MsgGroup
		totalChars := 0
		for _, mid := range msgOrderMap[sid] {
			content := msgContentMap[sid][mid].String()
			msgs = append(msgs, MsgGroup{MsgID: mid, Content: content})
			totalChars += len(content)
		}
		sessions = append(sessions, SessionData{
			SessionID:  sid,
			Msgs:       msgs,
			TotalChars: totalChars,
		})
	}

	log.Printf("[distill] [stage=collect] user=%d date=%s sessions=%d totalChars=%d",
		userID, date, len(sessions), func() int {
			total := 0
			for _, s := range sessions {
				total += s.TotalChars
			}
			return total
		}())

	return sessions, nil
}

// splitSessionIntoChunks 按 msg_id 边界切分 session，每片 ≤ 70000 字符
func splitSessionIntoChunks(sess SessionData) []string {
	const maxChunkSize = 70000

	if sess.TotalChars <= maxChunkSize {
		// 不需要分片，拼接所有 msg
		var builder strings.Builder
		for _, msg := range sess.Msgs {
			builder.WriteString(msg.Content)
		}
		return []string{builder.String()}
	}

	var chunks []string
	var current strings.Builder

	for _, msg := range sess.Msgs {
		// 如果当前块加上这条 msg 超限，且当前块非空，先保存
		if current.Len()+len(msg.Content) > maxChunkSize && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(msg.Content)
	}

	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}

// callDistillLLMWithRetry 带 3 次重试的 Map 阶段 LLM 调用
func callDistillLLMWithRetry(userID int, date, convContent, sessionID string) (*DistillResult, int, error) {
	maxRetries := 3
	retryDelays := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 第 %d 次重试（等待 %v）",
				userID, sessionID, attempt, retryDelays[attempt-1])
			time.Sleep(retryDelays[attempt-1])
		}

		result, err := callDistillLLM(userID, date, convContent)
		if err == nil {
			return result, attempt, nil
		}
		lastErr = err
		log.Printf("[distill] [user=%d] [session=%s] [stage=map] 第 %d 次尝试失败: %v",
			userID, sessionID, attempt+1, err)
	}

	return nil, maxRetries - 1, lastErr
}

// callReduceLLMWithRetry 带 3 次重试的 Reduce 阶段 LLM 调用
func callReduceLLMWithRetry(userID int, date, reduceContent string) (*DistillResult, int, error) {
	maxRetries := 3
	retryDelays := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[distill] [user=%d] [stage=reduce] 第 %d 次重试（等待 %v）",
				userID, attempt, retryDelays[attempt-1])
			time.Sleep(retryDelays[attempt-1])
		}

		result, err := callReduceLLM(userID, date, reduceContent)
		if err == nil {
			return result, attempt, nil
		}
		lastErr = err
		log.Printf("[distill] [user=%d] [stage=reduce] 第 %d 次尝试失败: %v",
			userID, attempt+1, err)
	}

	return nil, maxRetries - 1, lastErr
}

// buildReduceInput 构建 Reduce 阶段的输入文本
func buildReduceInput(results []mapResult, date string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("日期：%s\n\n", date))
	builder.WriteString(fmt.Sprintf("共有 %d 个对话 session 的蒸馏结果需要精炼：\n\n", len(results)))

	for i, mr := range results {
		builder.WriteString(fmt.Sprintf("═══ Session %d (session_id=%s) ═══\n", i+1, mr.SessionID))
		builder.WriteString(fmt.Sprintf("【摘要】%s\n", mr.SessionSummary))
		builder.WriteString(fmt.Sprintf("【日报片段】%s\n", mr.DailySummary))

		builder.WriteString("【知识条目】\n")
		for j, k := range mr.Knowledge {
			builder.WriteString(fmt.Sprintf("  %d. [%s] %s\n     %s\n", j+1, k.Dimension, k.Title, k.Content))
		}

		builder.WriteString("【建议】\n")
		for j, s := range mr.Suggestions {
			builder.WriteString(fmt.Sprintf("  %d. [%s] %s\n     %s\n", j+1, s.Category, s.Title, s.Suggestion))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// splitReduceInput 硬切 Reduce 输入（按字符数）
func splitReduceInput(content string, maxLen int) []string {
	if len(content) <= maxLen {
		return []string{content}
	}

	var chunks []string
	for i := 0; i < len(content); i += maxLen {
		end := i + maxLen
		if end > len(content) {
			end = len(content)
		}
		chunks = append(chunks, content[i:end])
	}
	return chunks
}

// RunDistillForDate 指定日期蒸馏
func RunDistillForDate(userID int, username, date string) error {
	return RunDistillForDateWithTrigger(userID, username, date, "cron")
}

// RunDistillForDateWithTrigger Map-Reduce 蒸馏流程
func RunDistillForDateWithTrigger(userID int, username, date, triggerType string) error {
	overallStart := time.Now()

	if userID <= 0 {
		var err error
		userID, err = getDefaultDistillUserID()
		if err != nil {
			distillLog(userID, username, date, triggerType, "error", "fail", "获取默认用户失败: "+err.Error(), "", 0, "")
			return err
		}
	}
	if username == "" {
		username = getDistillUsername()
	}

	// ══════════════════════════════════════════════
	// 阶段 1：收集 sessions
	// ══════════════════════════════════════════════
	collectStart := time.Now()
	sessions, err := collectSessionsForDate(userID, date)
	collectMs := int(time.Since(collectStart).Milliseconds())

	if err != nil {
		distillLog(userID, username, date, triggerType, "collect", "fail",
			"查询 session 对话失败: "+err.Error(), "", collectMs, "")
		return fmt.Errorf("查询 session 对话失败: %v", err)
	}

	// 构建 session 详情日志
	var sessionDetails []string
	for _, s := range sessions {
		sessionDetails = append(sessionDetails, fmt.Sprintf("session=%s msgs=%d chars=%d", s.SessionID, len(s.Msgs), s.TotalChars))
	}
	log.Printf("[distill] [user=%d] [stage=collect] 收集到 %d 个 session: %s", userID, len(sessions), strings.Join(sessionDetails, "; "))

	if len(sessions) == 0 {
		distillLog(userID, username, date, triggerType, "collect", "info",
			fmt.Sprintf("无有效 session 对话，跳过。sessions=%d", len(sessions)), "", collectMs, "")
		return nil
	}

	distillLog(userID, username, date, triggerType, "collect", "success",
		fmt.Sprintf("sessions=%d", len(sessions)), "", collectMs, "")

	// ══════════════════════════════════════════════
	// 阶段 2：Map — 每个 session 独立蒸馏
	// ══════════════════════════════════════════════
	mapStart := time.Now()
	var mapResults []mapResult
	totalChunks := 0
	successChunks := 0
	failChunks := 0

	for si, sess := range sessions {
		log.Printf("[distill] [user=%d] [session=%s] [stage=map] 开始 Map 蒸馏 (%d/%d)，msgs=%d，chars=%d",
			userID, sess.SessionID, si+1, len(sessions), len(sess.Msgs), sess.TotalChars)

		// 过滤：< 500 字符跳过
		if sess.TotalChars < 500 {
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 跳过：字符数不足 500（实际=%d）",
				userID, sess.SessionID, sess.TotalChars)
			distillLog(userID, username, date, triggerType, "map", "info",
				fmt.Sprintf("session=%s 跳过：字符数不足500（%d）", sess.SessionID, sess.TotalChars), "", 0, sess.SessionID)
			continue
		}

		// 分片
		chunks := splitSessionIntoChunks(sess)
		needSplit := len(chunks) > 1
		if needSplit {
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 需要分片：%d 片",
				userID, sess.SessionID, len(chunks))
		} else {
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 无需分片，直接蒸馏", userID, sess.SessionID)
		}
		totalChunks += len(chunks)

		var prevSummary string // 上一片的 session_summary，用作下一片的前情提要

		var sessionKnowledge []DistillKnowledge
		var sessionSuggestions []DistillSuggestion
		var sessionSummaries []string
		var sessionDailySummaries []string
		sessionRetries := 0

		for ci, chunk := range chunks {
			chunkChars := len(chunk)
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 第 %d/%d 片，chars=%d",
				userID, sess.SessionID, ci+1, len(chunks), chunkChars)

			// 第 2+ 片注入前情提要
			if ci > 0 && prevSummary != "" {
				injectPrefix := fmt.Sprintf("【前情提要】以下是对前面内容的摘要，请结合此上下文理解当前对话：\n%s\n\n--- 以下是第 %d 段对话 ---\n\n", prevSummary, ci+1)
				chunk = injectPrefix + chunk
			}

			chunkStart := time.Now()
			result, retries, err := callDistillLLMWithRetry(userID, date, chunk, sess.SessionID)
			chunkMs := int(time.Since(chunkStart).Milliseconds())
			sessionRetries += retries

			if err != nil {
				log.Printf("[distill] [user=%d] [session=%s] [stage=map] 第 %d/%d 片蒸馏失败（重试%d次后放弃）: %v",
					userID, sess.SessionID, ci+1, len(chunks), retries, err)
				distillLog(userID, username, date, triggerType, "map", "fail",
					fmt.Sprintf("session=%s 第%d/%d片失败（重试%d次）: %v", sess.SessionID, ci+1, len(chunks), retries, err),
					"", chunkMs, sess.SessionID)
				failChunks++
				continue
			}

			successChunks++
			ds := result.DailySummary
			if len(ds) > 50 {
				ds = ds[:50]
			}
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 第 %d/%d 片完成：知识=%d，建议=%d，日报=%q，摘要=%d字，耗时=%dms，重试=%d",
				userID, sess.SessionID, ci+1, len(chunks), len(result.Knowledge), len(result.Suggestions), ds,
				len(result.SessionSummary), chunkMs, retries)
			distillLog(userID, username, date, triggerType, "map", "success",
				fmt.Sprintf("session=%s 第%d/%d片: 知识=%d 建议=%d 摘要=%d字 耗时=%dms 重试=%d",
					sess.SessionID, ci+1, len(chunks), len(result.Knowledge), len(result.Suggestions),
					len(result.SessionSummary), chunkMs, retries),
				"", chunkMs, sess.SessionID)

			// 保存当前片的摘要作为下一片的前情提要
			if result.SessionSummary != "" {
				prevSummary = result.SessionSummary
				sessionSummaries = append(sessionSummaries, result.SessionSummary)
			}

			sessionKnowledge = append(sessionKnowledge, result.Knowledge...)
			sessionSuggestions = append(sessionSuggestions, result.Suggestions...)
			if result.DailySummary != "" {
				sessionDailySummaries = append(sessionDailySummaries, result.DailySummary)
			}
		}

		if len(sessionKnowledge) == 0 && len(sessionSuggestions) == 0 && len(sessionDailySummaries) == 0 {
			log.Printf("[distill] [user=%d] [session=%s] [stage=map] 该 session 所有片蒸馏均无产出", userID, sess.SessionID)
			continue
		}

		// 整个 session 的摘要：取最后一片的（最完整），或合并所有摘要
		finalSummary := ""
		if len(sessionSummaries) > 0 {
			finalSummary = sessionSummaries[len(sessionSummaries)-1]
		}

		mapResults = append(mapResults, mapResult{
			SessionID:      sess.SessionID,
			Knowledge:      sessionKnowledge,
			Suggestions:    sessionSuggestions,
			DailySummary:   strings.Join(sessionDailySummaries, "\n"),
			SessionSummary: finalSummary,
		})

		log.Printf("[distill] [user=%d] [session=%s] [stage=map] Map 完成：知识=%d，建议=%d，摘要=%d字，总重试=%d",
			userID, sess.SessionID, len(sessionKnowledge), len(sessionSuggestions), len(finalSummary), sessionRetries)
	}

	mapMs := int(time.Since(mapStart).Milliseconds())
	log.Printf("[distill] [user=%d] [stage=map] Map 阶段完成：sessions=%d，chunks=%d，成功=%d，失败=%d，耗时=%dms",
		userID, len(sessions), totalChunks, successChunks, failChunks, mapMs)
	distillLog(userID, username, date, triggerType, "map", "success",
		fmt.Sprintf("sessions=%d chunks=%d success=%d fail=%d 耗时=%dms", len(sessions), totalChunks, successChunks, failChunks, mapMs),
		"", mapMs, "")

	if len(mapResults) == 0 {
		distillLog(userID, username, date, triggerType, "error", "fail",
			"所有 session 蒸馏均无产出", "", int(time.Since(overallStart).Milliseconds()), "")
		return fmt.Errorf("所有 session 蒸馏均无产出")
	}

	// ══════════════════════════════════════════════
	// 阶段 3：Reduce — 合并精炼
	// ══════════════════════════════════════════════
	reduceStart := time.Now()

	var allKnowledge []DistillKnowledge
	var allSuggestions []DistillSuggestion
	var allDailySummaries []string

	for _, mr := range mapResults {
		allKnowledge = append(allKnowledge, mr.Knowledge...)
		allSuggestions = append(allSuggestions, mr.Suggestions...)
		if mr.DailySummary != "" {
			allDailySummaries = append(allDailySummaries, mr.DailySummary)
		}
	}

	// 构建 Reduce 输入
	reduceInput := buildReduceInput(mapResults, date)
	reduceInputLen := len(reduceInput)
	log.Printf("[distill] [user=%d] [stage=reduce] Reduce 输入大小=%d 字符，原始知识=%d，原始建议=%d",
		userID, reduceInputLen, len(allKnowledge), len(allSuggestions))

	// 如果 Reduce 输入 > 80000，硬切
	reduceChunks := splitReduceInput(reduceInput, 80000)
	log.Printf("[distill] [user=%d] [stage=reduce] Reduce 分 %d 批", userID, len(reduceChunks))

	var reducedKnowledge []DistillKnowledge
	var reducedSuggestions []DistillSuggestion
	var finalDailySummary string

	for ci, rc := range reduceChunks {
		chunkStart := time.Now()
		result, retries, err := callReduceLLMWithRetry(userID, date, rc)
		chunkMs := int(time.Since(chunkStart).Milliseconds())

		if err != nil {
			log.Printf("[distill] [user=%d] [stage=reduce] 第 %d/%d 批 Reduce 失败（重试%d次后放弃）: %v",
				userID, ci+1, len(reduceChunks), retries, err)
			distillLog(userID, username, date, triggerType, "reduce", "fail",
				fmt.Sprintf("第%d/%d批失败（重试%d次）: %v", ci+1, len(reduceChunks), retries, err),
				"", chunkMs, "")
			// Reduce 失败不丢弃 Map 结果，回退到原始数据
			continue
		}

		log.Printf("[distill] [user=%d] [stage=reduce] 第 %d/%d 批完成：知识=%d，建议=%d，日报=%d字，耗时=%dms，重试=%d",
			userID, ci+1, len(reduceChunks), len(result.Knowledge), len(result.Suggestions),
			len(result.DailySummary), chunkMs, retries)
		distillLog(userID, username, date, triggerType, "reduce", "success",
			fmt.Sprintf("第%d/%d批: 知识=%d 建议=%d 耗时=%dms 重试=%d",
				ci+1, len(reduceChunks), len(result.Knowledge), len(result.Suggestions), chunkMs, retries),
			"", chunkMs, "")

		reducedKnowledge = append(reducedKnowledge, result.Knowledge...)
		reducedSuggestions = append(reducedSuggestions, result.Suggestions...)
		if result.DailySummary != "" {
			finalDailySummary = result.DailySummary
		}
	}

	reduceMs := int(time.Since(reduceStart).Milliseconds())
	log.Printf("[distill] [user=%d] [stage=reduce] Reduce 完成：精炼知识=%d，建议=%d，日报=%d字，耗时=%dms",
		userID, len(reducedKnowledge), len(reducedSuggestions), len(finalDailySummary), reduceMs)
	distillLog(userID, username, date, triggerType, "reduce", "success",
		fmt.Sprintf("精炼知识=%d 建议=%d 日报=%d字 耗时=%dms", len(reducedKnowledge), len(reducedSuggestions), len(finalDailySummary), reduceMs),
		"", reduceMs, "")

	// Reduce 失败时回退到 Map 原始数据
	if len(reducedKnowledge) == 0 && len(reducedSuggestions) == 0 {
		log.Printf("[distill] [user=%d] [stage=reduce] Reduce 无产出，回退到 Map 原始数据", userID)
		reducedKnowledge = allKnowledge
		reducedSuggestions = allSuggestions
	}
	if finalDailySummary == "" && len(allDailySummaries) > 0 {
		finalDailySummary = strings.Join(allDailySummaries, "\n")
	}

	// ══════════════════════════════════════════════
	// 阶段 4：入库
	// ══════════════════════════════════════════════
	storeStart := time.Now()

	// 知识入库：优先用 Reduce 精炼结果（reducedKnowledge），Reduce 失败时回退到 Map 原始产出（allKnowledge）
	// 修复 B1：之前错误地遍历 mapResults（Map 原始数据），导致 Reduce 精炼白做
	knowledgeToStore := reducedKnowledge
	useReduce := true
	if len(knowledgeToStore) == 0 {
		knowledgeToStore = allKnowledge
		useReduce = false
	}
	log.Printf("[distill] [user=%d] [stage=store] 入库数据源=%s，待入库条数=%d",
		userID, func() string { if useReduce { return "reduce" }; return "map_fallback" }(), len(knowledgeToStore))

	knowledgeStored := 0
	knowledgeSkipped := 0
	for _, k := range knowledgeToStore {
		// Reduce 合并后的知识是跨 session 的，session_id 留空串；Map 兜底场景按 source 区分
		saved, err := StoreDistillKnowledge(userID, date, k, "")
		if err != nil {
			log.Printf("[distill] [user=%d] [stage=store] 知识入库失败: %v, title=%s", userID, err, k.Title)
			distillLog(userID, username, date, triggerType, "save_knowledge", "fail",
				fmt.Sprintf("知识入库失败: %v, title=%s", err, k.Title), "", 0, "")
			continue
		}
		if saved {
			knowledgeStored++
		} else {
			knowledgeSkipped++
		}
	}
	log.Printf("[distill] [user=%d] [stage=store] 知识入库完成：新增=%d，跳过(重复)=%d", userID, knowledgeStored, knowledgeSkipped)
	distillLog(userID, username, date, triggerType, "save_knowledge", "success",
		fmt.Sprintf("新增=%d 跳过=%d", knowledgeStored, knowledgeSkipped), "", int(time.Since(storeStart).Milliseconds()), "")

	// 建议入库
	suggestionStored := 0
	for _, s := range reducedSuggestions {
		if _, err := storeSuggestion(userID, date, s); err != nil {
			log.Printf("[distill] [user=%d] [stage=store] 建议入库失败: %v, title=%s", userID, err, s.Title)
			continue
		}
		suggestionStored++
	}
	log.Printf("[distill] [user=%d] [stage=store] 建议入库完成：新增=%d", userID, suggestionStored)

	// 工作日报入库
	diaryStored := 0
	if finalDailySummary != "" {
		if _, err := storeWorkDiary(userID, date, finalDailySummary); err != nil {
			log.Printf("[distill] [user=%d] [stage=store] 日报保存失败: %v", userID, err)
			distillLog(userID, username, date, triggerType, "save_diary", "fail",
				"日报保存失败: "+err.Error(), "", 0, "")
		} else {
			diaryStored = 1
			log.Printf("[distill] [user=%d] [stage=store] 日报保存成功，%d字", userID, len(finalDailySummary))
		}
	}
	distillLog(userID, username, date, triggerType, "save_diary", "success",
		fmt.Sprintf("日报=%d字", len(finalDailySummary)), "", 0, "")

	totalMs := int(time.Since(overallStart).Milliseconds())
	log.Printf("[distill] [user=%d] [stage=done] 蒸馏完成：sessions=%d map知识=%d reduce精炼=%d 入库=%d 建议=%d 日报=%d 总耗时=%dms",
		userID, len(sessions), len(allKnowledge), len(reducedKnowledge), knowledgeStored, suggestionStored, diaryStored, totalMs)
	distillLog(userID, username, date, triggerType, "done", "success",
		fmt.Sprintf("sessions=%d map知识=%d reduce精炼=%d 入库=%d 建议=%d 日报=%d 总耗时=%dms",
			len(sessions), len(allKnowledge), len(reducedKnowledge), knowledgeStored, suggestionStored, diaryStored, totalMs),
		"", totalMs, "")

	return nil
}