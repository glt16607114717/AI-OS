package service

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// ── 知识库每日巡检（audit）──
//
// 职责：
//   1. 采集检索日志（injectRAGContext 里采样）和入库日志（StoreDistill/StoreUpload 后全量）
//   2. 每日 8 点 cron 触发 RunDailyAudit：聚合指标 + 调智谱 LLM 评审 + 写报告
//   3. 评审产出的 problem_cases 写入 sys_knowledge_pending_archive，等人工审核闭环
//
// 关键约束（用户明确要求）：
//   1. API 地址必须用 coding plan（https://open.bigmodel.cn/api/coding/paas/v4），
//      绝不用 API 地址（/api/paas/v4），因为账号没钱
//   2. 智谱 5.2 百万上下文够装，但单次请求 > 50 万 token 时自动分批

// ══════════════════════════════════════════════════════════════
// 表结构定义（EnsureAuditTables 建表，main.go init 调用）
// ══════════════════════════════════════════════════════════════

// EnsureAuditTables 建立巡检相关的三张表
func EnsureAuditTables() {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[audit] EnsureAuditTables: 获取数据库连接失败: %v", err)
		return
	}

	// 1. 巡检日志表
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_knowledge_audit_log (
		id INT AUTO_INCREMENT PRIMARY KEY,
		log_type VARCHAR(20) NOT NULL COMMENT 'retrieve=检索日志, store=入库日志',
		user_id INT NOT NULL DEFAULT 0,
		username VARCHAR(100) DEFAULT '',
		query TEXT COMMENT '用户原始查询（检索日志）',
		projects_filter VARCHAR(200) DEFAULT '' COMMENT '检索范围逗号分隔',
		knowledge_id INT NOT NULL DEFAULT 0,
		title VARCHAR(500) DEFAULT '',
		category VARCHAR(50) DEFAULT '',
		project VARCHAR(50) DEFAULT '',
		priority VARCHAR(20) DEFAULT '',
		score DECIMAL(5,4) DEFAULT 0 COMMENT '相似度（检索日志）',
		content_preview VARCHAR(600) DEFAULT '' COMMENT '内容前300字',
		source VARCHAR(200) DEFAULT '' COMMENT 'distill: 或 upload:（入库日志）',
		audit_date DATE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_log_type (log_type),
		INDEX idx_audit_date (audit_date),
		INDEX idx_knowledge_id (knowledge_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='知识库巡检日志'`)

	// 2. 巡检报告表
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_knowledge_audit_report (
		id INT AUTO_INCREMENT PRIMARY KEY,
		report_date DATE NOT NULL UNIQUE,
		metrics_retrieve JSON COMMENT '检索指标',
		metrics_store JSON COMMENT '入库指标',
		overall_score INT DEFAULT 0,
		score_level VARCHAR(20) DEFAULT '',
		report_content MEDIUMTEXT COMMENT 'Markdown报告',
		problem_cases JSON COMMENT '问题案例',
		status VARCHAR(20) DEFAULT 'completed',
		error_msg TEXT,
		llm_model VARCHAR(50) DEFAULT '',
		llm_tokens_used INT DEFAULT 0,
		llm_api_url VARCHAR(200) DEFAULT '' COMMENT '实际调用URL（验证coding plan）',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_report_date (report_date)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='知识库每日巡检报告'`)

	// 3. 待归档表（闭环）
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_knowledge_pending_archive (
		id INT AUTO_INCREMENT PRIMARY KEY,
		knowledge_id INT NOT NULL,
		audit_report_id INT NOT NULL,
		reason VARCHAR(500) DEFAULT '',
		suggested_action VARCHAR(50) DEFAULT '',
		status VARCHAR(20) DEFAULT 'pending' COMMENT 'pending/approved/rejected/done',
		reviewed_by VARCHAR(100) DEFAULT '',
		reviewed_at DATETIME NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uk_knowledge_report (knowledge_id, audit_report_id),
		INDEX idx_status (status),
		INDEX idx_audit_report_id (audit_report_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='待归档知识'`)

	log.Println("[audit] 巡检表已就绪（sys_knowledge_audit_log/report/pending_archive）")
}

// ══════════════════════════════════════════════════════════════
// 日志采集（handler/llm.go 和 service/knowledge.go 调用）
// ══════════════════════════════════════════════════════════════

// LogRetrieveAudit 采集一条检索日志（在 injectRAGContext 召回循环里调用）
// 采样策略由调用方决定（推荐 rand.Intn(10) == 0，即 10% 随机采样）
func LogRetrieveAudit(userID int, username, query string, projects []string, item KnowledgeItem) {
	if userID <= 0 {
		return
	}
	conn, err := GetDB()
	if err != nil {
		return
	}

	preview := item.Content
	if len(preview) > 300 {
		runes := []rune(preview)
		if len(runes) > 300 {
			preview = string(runes[:300])
		}
	}

	auditDate := time.Now().Format("2006-01-02")

	// vector_score：最高向量相似度（用于日报检索质量 avg_score 计算）
	// 多向量检索时 Score 是 RRF 融合分数，不能直接作为 avg_score 使用
	var vecScore sql.NullFloat64
	if item.VectorScore > 0 {
		vecScore = sql.NullFloat64{Float64: item.VectorScore, Valid: true}
	}

	_, err = conn.Exec(`INSERT INTO sys_knowledge_audit_log
		(log_type, user_id, username, query, projects_filter, knowledge_id, title, category, project, priority, score, vector_score, content_preview, audit_date)
		VALUES ('retrieve', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, query, strings.Join(projects, ","), item.ID, item.Title,
		item.Category, item.Project, item.Priority, item.Score, vecScore, preview, auditDate)
	if err != nil {
		log.Printf("[audit] LogRetrieveAudit 写入失败: %v", err)
	}
}

// LogStoreAudit 采集一条入库日志（StoreDistill/StoreUpload 成功后调用）
func LogStoreAudit(userID int, knowledgeID int64, project, category, title, priority, content, source string) {
	if knowledgeID <= 0 {
		return
	}
	conn, err := GetDB()
	if err != nil {
		return
	}

	preview := content
	if len(preview) > 0 {
		runes := []rune(preview)
		if len(runes) > 300 {
			preview = string(runes[:300])
		}
	}

	username := ""
	auditDate := time.Now().Format("2006-01-02")

	_, err = conn.Exec(`INSERT INTO sys_knowledge_audit_log
		(log_type, user_id, username, projects_filter, knowledge_id, title, category, project, priority, content_preview, source, audit_date)
		VALUES ('store', ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, knowledgeID, title, category, project, priority, preview, source, auditDate)
	if err != nil {
		log.Printf("[audit] LogStoreAudit 写入失败: %v", err)
	}
}

// ══════════════════════════════════════════════════════════════
// 巡检主流程（cron 每天 8 点调用）
// ══════════════════════════════════════════════════════════════

// RunDailyAudit 每日巡检主流程
// 步骤：采集昨日日志 -> 聚合指标 -> 抽样样本 -> 调智谱评审 -> 写报告 -> 写 pending_archive
func RunDailyAudit() error {
	// 评审"昨天"的数据（凌晨 3 点蒸馏，8 点评审昨天产出）
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	return RunAuditForDate(yesterday)
}

// RunAuditForDate 评审指定日期的数据（供手动触发复用）
func RunAuditForDate(date string) error {
	log.Printf("[audit] 开始巡检 date=%s", date)

	conn, err := GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %v", err)
	}

	// 占位报告（status=running），防止重复跑
	_, _ = conn.Exec(`INSERT INTO sys_knowledge_audit_report (report_date, status, error_msg) VALUES (?, 'running', '')
		ON DUPLICATE KEY UPDATE status='running', error_msg=''`, date)

	// 1. 采集日志
	retrieveLogs, err := fetchAuditLogs(conn, "retrieve", date)
	if err != nil {
		return finalizeAuditFail(conn, date, fmt.Errorf("采集检索日志失败: %v", err))
	}
	storeLogs, err := fetchAuditLogs(conn, "store", date)
	if err != nil {
		return finalizeAuditFail(conn, date, fmt.Errorf("采集入库日志失败: %v", err))
	}
	log.Printf("[audit] date=%s 检索日志=%d 条，入库日志=%d 条", date, len(retrieveLogs), len(storeLogs))

	// 2. 聚合指标
	metricsRetrieve := buildRetrieveMetrics(retrieveLogs)
	metricsStore := buildStoreMetrics(storeLogs)

	// 3. 抽样样本（检索分层抽样 30 条，入库全量）
	retrieveSamples := sampleRetrieveLogs(retrieveLogs, 30)
	storeSamples := storeLogs // 入库日志全量发（每天 ≤200 条）
	if len(storeSamples) > 200 {
		storeSamples = storeSamples[:200] // 防御性截断
	}

	// 4. 调智谱评审（含分批保护）
	auditResult, apiURL, tokensUsed, err := callAuditLLM(date, metricsRetrieve, metricsStore, retrieveSamples, storeSamples)
	if err != nil {
		return finalizeAuditFail(conn, date, fmt.Errorf("智谱评审失败: %v", err))
	}
	log.Printf("[audit] 评审完成 score=%d level=%s api=%s tokens=%d cases=%d",
		auditResult.OverallScore, auditResult.ScoreLevel, apiURL, tokensUsed, len(auditResult.ProblemCases))

	// 5. 序列化指标
	metricsRetrieveJSON, _ := json.Marshal(metricsRetrieve)
	metricsStoreJSON, _ := json.Marshal(metricsStore)
	problemCasesJSON, _ := json.Marshal(auditResult.ProblemCases)

	// 6. 写报告
	_, err = conn.Exec(`UPDATE sys_knowledge_audit_report SET
		metrics_retrieve=?, metrics_store=?, overall_score=?, score_level=?,
		report_content=?, problem_cases=?, status='completed', error_msg='',
		llm_model=?, llm_tokens_used=?, llm_api_url=? WHERE report_date=?`,
		string(metricsRetrieveJSON), string(metricsStoreJSON),
		auditResult.OverallScore, auditResult.ScoreLevel,
		auditResult.ReportMarkdown, string(problemCasesJSON),
		"glm-5.2", tokensUsed, apiURL, date)
	if err != nil {
		return finalizeAuditFail(conn, date, fmt.Errorf("写入报告失败: %v", err))
	}

	// 7. 取报告 id
	var reportID int
	if err := conn.QueryRow("SELECT id FROM sys_knowledge_audit_report WHERE report_date=?", date).Scan(&reportID); err != nil {
		return fmt.Errorf("查询报告 id 失败: %v", err)
	}

	// 8. 自动归档：信任巡检 LLM 的判断，建议 archive 的直接归档
	// 不再加规则二次过滤——query 污染已在源头修（HTML过滤、闲聊过滤），LLM 不会再误判
	// 归档是软删除，随时可恢复
	autoArchived := 0
	for _, pc := range auditResult.ProblemCases {
		if pc.SuggestedAction != "archive" || pc.KnowledgeID <= 0 {
			continue
		}

		var qdrantID string
		conn.QueryRow("SELECT IFNULL(qdrant_id,'') FROM sys_knowledge WHERE id=?", pc.KnowledgeID).Scan(&qdrantID)

		// 软删除 + 删 Qdrant 向量
		conn.Exec("UPDATE sys_knowledge SET status='archived' WHERE id=?", pc.KnowledgeID)
		if qdrantID != "" {
			if err := QdrantDelete(qdrantID); err != nil {
				log.Printf("[audit] 自动归档 Qdrant 删除失败（不阻断）knowledge_id=%d: %v", pc.KnowledgeID, err)
			}
		}

		// 记录归档历史（供页面查看，不再有 pending 状态）
		conn.Exec(`INSERT INTO sys_knowledge_pending_archive
			(knowledge_id, audit_report_id, reason, suggested_action, status, reviewed_by, reviewed_at)
			VALUES (?, ?, ?, 'archive', 'done', 'system-auto', NOW())`,
			pc.KnowledgeID, reportID, pc.Reason)
		autoArchived++
	}
	log.Printf("[audit] 巡检完成 date=%s score=%d 自动归档=%d 条", date, auditResult.OverallScore, autoArchived)
	return nil
}

// finalizeAuditFail 巡检失败时把报告状态置 failed，记录错误
func finalizeAuditFail(conn *sql.DB, date string, err error) error {
	log.Printf("[audit] 巡检失败 date=%s err=%v", date, err)
	_, _ = conn.Exec(`UPDATE sys_knowledge_audit_report SET status='failed', error_msg=? WHERE report_date=?`, err.Error(), date)
	return err
}

// ══════════════════════════════════════════════════════════════
// 日志查询与指标聚合
// ══════════════════════════════════════════════════════════════

// auditLogRow 巡检日志记录
type auditLogRow struct {
	ID             int     `json:"id"`
	LogType        string  `json:"log_type"`
	UserID         int     `json:"user_id"`
	Username       string  `json:"username"`
	Query          string  `json:"query"`
	ProjectsFilter string  `json:"projects_filter"`
	KnowledgeID    int     `json:"knowledge_id"`
	Title          string  `json:"title"`
	Category       string  `json:"category"`
	Project        string  `json:"project"`
	Priority       string  `json:"priority"`
	Score          float64 `json:"score"`           // RRF 融合分数（多向量）或向量分数（单向量）
	VectorScore    float64 `json:"vector_score"`    // 最高向量相似度（用于 avg_score 计算，优先于此字段）
	ContentPreview string  `json:"content_preview"`
	Source         string  `json:"source"`
}

// fetchAuditLogs 查指定日期指定类型的日志
func fetchAuditLogs(conn *sql.DB, logType, date string) ([]auditLogRow, error) {
	rows, err := conn.Query(`SELECT id, log_type, user_id, username, IFNULL(query,''), IFNULL(projects_filter,''),
		knowledge_id, title, category, project, priority, score, IFNULL(vector_score,0), IFNULL(content_preview,''), IFNULL(source,'')
		FROM sys_knowledge_audit_log WHERE log_type=? AND audit_date=? ORDER BY id`, logType, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []auditLogRow
	for rows.Next() {
		var r auditLogRow
		var score sql.NullFloat64
		var vecScore sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.LogType, &r.UserID, &r.Username, &r.Query, &r.ProjectsFilter,
			&r.KnowledgeID, &r.Title, &r.Category, &r.Project, &r.Priority, &score, &vecScore, &r.ContentPreview, &r.Source); err != nil {
			continue
		}
		if score.Valid {
			r.Score = score.Float64
		}
		if vecScore.Valid {
			r.VectorScore = vecScore.Float64
		}
		result = append(result, r)
	}
	return result, nil
}

// buildRetrieveMetrics 聚合检索指标
func buildRetrieveMetrics(logs []auditLogRow) map[string]interface{} {
	m := map[string]interface{}{
		"sample_count": len(logs),
	}
	if len(logs) == 0 {
		return m
	}

	// 平均 score：优先用 VectorScore（最高向量相似度），因为 RRF 分数是相对排序分没有绝对意义
	// 单向量检索时 Score 和 VectorScore 相同，不改变原有行为
	totalScore := 0.0
	validScoreCount := 0
	for _, l := range logs {
		s := l.VectorScore
		if s == 0 {
			s = l.Score // 回退：没有 vector_score 时用 score（兼容旧数据）
		}
		if s > 0 {
			totalScore += s
			validScoreCount++
		}
	}
	if validScoreCount > 0 {
		m["avg_score"] = totalScore / float64(validScoreCount)
	}

	// score 分布：也用 VectorScore 判断
	high, mid, low := 0, 0, 0
	for _, l := range logs {
		s := l.VectorScore
		if s == 0 {
			s = l.Score
		}
		if s >= 0.8 {
			high++
		} else if s >= 0.6 {
			mid++
		} else {
			low++
		}
	}
	m["score_distribution"] = map[string]int{"high": high, "mid": mid, "low": low}

	// 按 project 分布
	projectCount := map[string]int{}
	for _, l := range logs {
		p := l.Project
		if p == "" {
			p = "unknown"
		}
		projectCount[p]++
	}
	m["project_distribution"] = projectCount

	return m
}

// buildStoreMetrics 聚合入库指标
func buildStoreMetrics(logs []auditLogRow) map[string]interface{} {
	m := map[string]interface{}{
		"total": len(logs),
	}
	if len(logs) == 0 {
		return m
	}

	// 按 category 分布
	categoryCount := map[string]int{}
	projectCount := map[string]int{}
	priorityCount := map[string]int{}
	for _, l := range logs {
		c := l.Category
		if c == "" {
			c = "unknown"
		}
		categoryCount[c]++

		p := l.Project
		if p == "" {
			p = "unknown"
		}
		projectCount[p]++

		pr := l.Priority
		if pr == "" {
			pr = "unknown"
		}
		priorityCount[pr]++
	}
	m["category_distribution"] = categoryCount
	m["project_distribution"] = projectCount
	m["priority_distribution"] = priorityCount

	// other 占比（越低越好）
	otherCount := categoryCount["other"]
	if len(logs) > 0 {
		m["other_ratio"] = float64(otherCount) / float64(len(logs))
	}

	return m
}

// sampleRetrieveLogs 检索日志分层抽样（高/中/低 score 各 1/3）
func sampleRetrieveLogs(logs []auditLogRow, max int) []auditLogRow {
	if len(logs) <= max {
		return logs
	}

	// 分层
	var high, mid, low []auditLogRow
	for _, l := range logs {
		if l.Score >= 0.8 {
			high = append(high, l)
		} else if l.Score >= 0.6 {
			mid = append(mid, l)
		} else {
			low = append(low, l)
		}
	}

	// 每层取 max/3
	perLayer := max / 3
	if perLayer < 1 {
		perLayer = 1
	}

	sampled := []auditLogRow{}
	sampled = append(sampled, takeRandom(high, perLayer)...)
	sampled = append(sampled, takeRandom(mid, perLayer)...)
	sampled = append(sampled, takeRandom(low, perLayer)...)

	// 不够 max 从剩余补
	if len(sampled) < max {
		used := map[int]bool{}
		for _, l := range sampled {
			used[l.ID] = true
		}
		for _, l := range logs {
			if len(sampled) >= max {
				break
			}
			if !used[l.ID] {
				sampled = append(sampled, l)
			}
		}
	}
	return sampled
}

// takeRandom 从切片中随机取 n 条
func takeRandom(items []auditLogRow, n int) []auditLogRow {
	if len(items) <= n {
		return items
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	perm := r.Perm(len(items))
	result := []auditLogRow{}
	for i := 0; i < n && i < len(perm); i++ {
		result = append(result, items[perm[i]])
	}
	return result
}

// ══════════════════════════════════════════════════════════════
// 智谱 LLM 评审（关键约束：走 coding plan 地址）
// ══════════════════════════════════════════════════════════════

// ProblemCase 评审产出的问题案例
type ProblemCase struct {
	LogID           int    `json:"log_id"`
	KnowledgeID     int    `json:"knowledge_id"`
	Type            string `json:"type"`             // retrieve|store
	Reason          string `json:"reason"`           // 为什么是问题
	SuggestedAction string `json:"suggested_action"` // archive|fix_category|fix_project
}

// AuditResult 评审结果
type AuditResult struct {
	OverallScore   int            `json:"overall_score"`
	ScoreLevel     string         `json:"score_level"` // green|yellow|red
	RetrieveAnalysis map[string]interface{} `json:"retrieve_analysis"`
	StoreAnalysis    map[string]interface{} `json:"store_analysis"`
	ProblemCases   []ProblemCase  `json:"problem_cases"`
	ReportMarkdown string         `json:"report_markdown"`
}

// callAuditLLM 调用智谱 GLM-5.2 评审知识库质量
// 关键约束 1：地址必须走 coding plan（/api/coding/paas/v4），绝不用 API 地址
// 关键约束 2：单次请求 token > 50 万时分批（按 log_type 分），最后合并 problem_cases 去重
//
// 返回：(评审结果, 实际调用URL, 消耗token, 错误)
func callAuditLLM(date string, metricsRetrieve, metricsStore map[string]interface{},
	retrieveSamples, storeSamples []auditLogRow) (*AuditResult, string, int, error) {

	conn, err := GetDB()
	if err != nil {
		return nil, "", 0, fmt.Errorf("获取数据库连接失败: %v", err)
	}

	// ── 关键约束 1：从 sys_vendor 读智谱 API Key 和 base_url ──
	var apiKey, baseURL string
	err = conn.QueryRow(`SELECT k.api_key, v.base_url FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey, &baseURL)
	if err != nil || apiKey == "" {
		return nil, "", 0, fmt.Errorf("未找到智谱 API Key: %v", err)
	}
	// ⚠️ 兜底地址必须带 /coding/（coding plan），绝不能用 /api/paas/v4
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/chat/completions"

	// 安全检查：如果 base_url 不含 /coding/，日志告警但仍继续（不阻断流程）
	if !strings.Contains(baseURL, "/coding/") {
		log.Printf("[audit] ⚠️ 警告：base_url 不含 /coding/，可能误用了 API 地址（账号没钱）：base_url=%s", baseURL)
	}
	log.Printf("[audit] 调用智谱 API: %s（coding plan）", apiURL)

	// ── 构建 prompt ──
	prompt := buildAuditPrompt(date, metricsRetrieve, metricsStore, retrieveSamples, storeSamples)

	// ── 关键约束 2：token 估算 + 分批保护 ──
	estTokens := estimateTokens(prompt)
	log.Printf("[audit] 评审 prompt 估算 token=%d（阈值 50 万）", estTokens)

	if estTokens > 500000 {
		// 分批评审：先评审检索样本，再分批评审入库样本，最后合并
		log.Printf("[audit] token 超 50 万，启动分批评审")
		return callAuditLLMBatched(apiURL, apiKey, date, metricsRetrieve, metricsStore, retrieveSamples, storeSamples)
	}

	// 单次评审
	return callAuditLLMSingle(apiURL, apiKey, prompt)
}

// callAuditLLMSingle 单次调用智谱评审
func callAuditLLMSingle(apiURL, apiKey, prompt string) (*AuditResult, string, int, error) {
	body := map[string]interface{}{
		"model":            "glm-5.2",
		"messages":         []map[string]string{{"role": "user", "content": prompt}},
		"temperature":      0.3,
		"response_format":  map[string]string{"type": "json_object"},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, apiURL, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := SharedHTTPClientLong
	resp, err := client.Do(req)
	if err != nil {
		return nil, apiURL, 0, fmt.Errorf("请求智谱 API 失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apiURL, 0, err
	}

	if resp.StatusCode != 200 {
		detail := string(respBody)
		if len(detail) > 500 {
			detail = detail[:500]
		}
		return nil, apiURL, 0, fmt.Errorf("智谱 API 返回 %d: %s", resp.StatusCode, detail)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, apiURL, 0, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return nil, apiURL, 0, fmt.Errorf("AI 未返回有效内容")
	}

	content := result.Choices[0].Message.Content
	tokensUsed := result.Usage.TotalTokens

	var auditResult AuditResult
	if err := json.Unmarshal([]byte(content), &auditResult); err != nil {
		log.Printf("[audit] JSON 解析失败: %v, content[:300]=%s", err, content[:min(300, len(content))])
		return nil, apiURL, tokensUsed, fmt.Errorf("解析评审 JSON 失败: %v", err)
	}

	return &auditResult, apiURL, tokensUsed, nil
}

// callAuditLLMBatched 分批评审（token 超 50 万时触发）
// 策略：检索样本一批 + 入库样本分批（每批 ≤ 100 条），最后合并 problem_cases
func callAuditLLMBatched(apiURL, apiKey, date string, metricsRetrieve, metricsStore map[string]interface{},
	retrieveSamples, storeSamples []auditLogRow) (*AuditResult, string, int, error) {

	allProblemCases := []ProblemCase{}
	totalTokens := 0
	var lastAPIURL string
	var overallScore int
	var scoreLevel string
	var reportParts []string

	// 批 1：检索样本 + 检索指标
	if len(retrieveSamples) > 0 {
		prompt := buildAuditPromptPartial(date, "retrieve", metricsRetrieve, retrieveSamples)
		result, api, tokens, err := callAuditLLMSingle(apiURL, apiKey, prompt)
		if err == nil && result != nil {
			allProblemCases = append(allProblemCases, result.ProblemCases...)
			totalTokens += tokens
			lastAPIURL = api
			overallScore = result.OverallScore
			scoreLevel = result.ScoreLevel
			if result.ReportMarkdown != "" {
				reportParts = append(reportParts, "## 检索质量评审\n\n"+result.ReportMarkdown)
			}
		}
	}

	// 批 2-N：入库样本分批（每批 100 条）
	batchSize := 100
	for i := 0; i < len(storeSamples); i += batchSize {
		end := i + batchSize
		if end > len(storeSamples) {
			end = len(storeSamples)
		}
		batch := storeSamples[i:end]
		batchMetrics := buildStoreMetrics(batch)
		prompt := buildAuditPromptPartial(date, "store", batchMetrics, batch)
		result, api, tokens, err := callAuditLLMSingle(apiURL, apiKey, prompt)
		if err == nil && result != nil {
			allProblemCases = append(allProblemCases, result.ProblemCases...)
			totalTokens += tokens
			lastAPIURL = api
			if result.OverallScore > overallScore {
				overallScore = result.OverallScore
				scoreLevel = result.ScoreLevel
			}
			if result.ReportMarkdown != "" {
				reportParts = append(reportParts, fmt.Sprintf("## 入库质量评审（批 %d-%d）\n\n%s", i, end, result.ReportMarkdown))
			}
		}
	}

	// 合并去重 problem_cases（按 knowledge_id）
	seen := map[int]bool{}
	uniqueCases := []ProblemCase{}
	for _, pc := range allProblemCases {
		if pc.KnowledgeID > 0 && !seen[pc.KnowledgeID] {
			seen[pc.KnowledgeID] = true
			uniqueCases = append(uniqueCases, pc)
		}
	}

	return &AuditResult{
		OverallScore:   overallScore,
		ScoreLevel:     scoreLevel,
		ProblemCases:   uniqueCases,
		ReportMarkdown: strings.Join(reportParts, "\n\n---\n\n"),
	}, lastAPIURL, totalTokens, nil
}

// estimateTokens 估算字符串 token 数（中文约 1.5 token/字，英文约 0.25 token/字符）
// 简化估算：字符数 × 1.2（混合中英文的保守估计）
func estimateTokens(s string) int {
	runes := []rune(s)
	// 中文字符占比高时按 1.5，英文高时按 0.25，这里取保守 1.2
	return int(float64(len(runes)) * 1.2)
}

// ══════════════════════════════════════════════════════════════
// 评审 Prompt 构建
// ══════════════════════════════════════════════════════════════

// buildAuditPrompt 构建完整评审 Prompt（单次模式）
func buildAuditPrompt(date string, metricsRetrieve, metricsStore map[string]interface{},
	retrieveSamples, storeSamples []auditLogRow) string {

	metricsRetrieveJSON, _ := json.Marshal(metricsRetrieve)
	metricsStoreJSON, _ := json.Marshal(metricsStore)
	retrieveSamplesJSON, _ := json.Marshal(simplifySamples(retrieveSamples))
	storeSamplesJSON, _ := json.Marshal(simplifySamples(storeSamples))

	return fmt.Sprintf(`你是企业知识库质量评审专家。请评审 AI-OS 知识库在 %s 的运行情况。

【检索指标】
%s

【入库指标】
%s

【检索抽样案例】（分层抽样，含 query/召回知识/score）
%s

【入库抽样案例】（全量，含分类/项目/内容预览）
%s

【评审任务】
请输出一个 JSON 对象：
{
  "overall_score": 0-100,
  "score_level": "green|yellow|red",
  "retrieve_analysis": {
    "accuracy_rate": 0-100,
    "avg_score": 0-1,
    "issues": ["问题1", "问题2"],
    "suggestions": ["建议1", "建议2"]
  },
  "store_analysis": {
    "classification_accuracy": 0-100,
    "work_log_ratio": 0-100,
    "issues": ["问题1"],
    "suggestions": ["建议1"]
  },
  "problem_cases": [
    {
      "log_id": 123,
      "knowledge_id": 456,
      "type": "retrieve|store",
      "reason": "为什么这条是问题",
      "suggested_action": "archive|fix_category|fix_project"
    }
  ],
  "report_markdown": "完整的 Markdown 报告正文（含指标表格、问题分析、改进建议）"
}

【评分标准】
- green (80-100)：健康，无需干预
- yellow (60-79)：有改善空间，关注 problem_cases
- red (<60)：严重问题，需立即处理

【判断"工作流水"的特征】（suggested_action=archive）
- title/content 含"今天/昨日/本次提交/N 次提交/N 个文件/变更清单/提交记录"
- content 高度时效性（3 天后失效）
- 不是通用技术知识

【判断"分类错误"的特征】（suggested_action=fix_category 或 fix_project）
- category=other 但内容明显属于某个枚举（技术规范/架构决策/开发流程/Bug修复/工具技巧/环境配置）
- project 不在 [ai-os, rmp, general] 内

【判断"检索不相关"的特征】（suggested_action=archive 或 fix_category）
- query 与召回的 title/content 主题不相关
- score < 0.6 但被注入了对话（阈值已经 0.5）

【重要规则】
1. problem_cases 的 log_id 和 knowledge_id 必须来自上面的抽样案例，不要编造
2. problem_cases 只列"需要处理"的问题，相关度高的不要列
3. report_markdown 要含：检索质量分析、入库质量分析、问题案例汇总、改进建议
4. 只输出 JSON，不要添加任何解释文字`, date, string(metricsRetrieveJSON), string(metricsStoreJSON),
		string(retrieveSamplesJSON), string(storeSamplesJSON))
}

// buildAuditPromptPartial 构建分批评审 Prompt（只评审检索 或 只评审入库）
func buildAuditPromptPartial(date, batchType string, metrics map[string]interface{}, samples []auditLogRow) string {
	metricsJSON, _ := json.Marshal(metrics)
	samplesJSON, _ := json.Marshal(simplifySamples(samples))

	typeLabel := "检索质量"
	if batchType == "store" {
		typeLabel = "入库质量"
	}

	return fmt.Sprintf(`你是企业知识库质量评审专家。请评审 AI-OS 知识库在 %s 的%s。

【指标】
%s

【抽样案例】
%s

请输出一个 JSON 对象：
{
  "overall_score": 0-100,
  "score_level": "green|yellow|red",
  "problem_cases": [
    {"log_id": 123, "knowledge_id": 456, "type": "%s", "reason": "...", "suggested_action": "archive|fix_category|fix_project"}
  ],
  "report_markdown": "Markdown 报告正文"
}

【评分标准】green(80-100)/yellow(60-79)/red(<60)
【工作流水特征】含"今天/昨日/N次提交/N个文件/变更清单"，3天后失效
【分类错误特征】category=other 但明显属于某枚举；project 不在 [ai-os,rmp,general]

只输出 JSON。`, date, typeLabel, string(metricsJSON), string(samplesJSON), batchType)
}

// simplifySamples 简化样本（去掉冗余字段，省 token）
func simplifySamples(samples []auditLogRow) []map[string]interface{} {
	result := []map[string]interface{}{}
	for _, s := range samples {
		m := map[string]interface{}{
			"log_id":       s.ID,
			"knowledge_id": s.KnowledgeID,
			"title":        s.Title,
			"category":     s.Category,
			"project":      s.Project,
			"priority":     s.Priority,
		}
		if s.LogType == "retrieve" {
			m["query"] = s.Query
			m["score"] = s.Score
		}
		if s.ContentPreview != "" {
			m["content_preview"] = s.ContentPreview
		}
		if s.Source != "" {
			m["source"] = s.Source
		}
		result = append(result, m)
	}
	return result
}

// ══════════════════════════════════════════════════════════════
// 报告查询 + 闭环操作（handler 调用）
// ══════════════════════════════════════════════════════════════

// GetAuditReports 查询报告列表（分页 + 日期范围筛选）
func GetAuditReports(page, pageSize int, startDate, endDate string) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}

	conditions := []string{}
	args := []interface{}{}
	if startDate != "" {
		conditions = append(conditions, "report_date >= ?")
		args = append(args, startDate)
	}
	if endDate != "" {
		conditions = append(conditions, "report_date <= ?")
		args = append(args, endDate)
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := conn.QueryRow("SELECT COUNT(*) FROM sys_knowledge_audit_report"+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT id, report_date, overall_score, score_level, status,
		IFNULL(error_msg,''), llm_model, llm_tokens_used, IFNULL(llm_api_url,''), created_at
		FROM sys_knowledge_audit_report` + whereClause +
		" ORDER BY report_date DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id int
		var reportDate string
		var score int
		var level, status, errMsg, model, apiURL, createdAt string
		var tokens int
		if err := rows.Scan(&id, &reportDate, &score, &level, &status, &errMsg, &model, &tokens, &apiURL, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "report_date": reportDate, "overall_score": score, "score_level": level,
			"status": status, "error_msg": errMsg, "llm_model": model,
			"llm_tokens_used": tokens, "llm_api_url": apiURL, "created_at": createdAt,
		})
	}
	return result, total, nil
}

// GetAuditReportDetail 查询单份报告详情
func GetAuditReportDetail(id int) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	var reportDate string
	var score int
	var level, status, content, errMsg, model, apiURL, createdAt string
	var tokens int
	var metricsRetrieveJSON, metricsStoreJSON, problemCasesJSON []byte

	err = conn.QueryRow(`SELECT report_date, overall_score, score_level,
		IFNULL(metrics_retrieve,'{}'), IFNULL(metrics_store,'{}'),
		IFNULL(report_content,''), IFNULL(problem_cases,'[]'),
		status, IFNULL(error_msg,''), llm_model, llm_tokens_used, IFNULL(llm_api_url,''), created_at
		FROM sys_knowledge_audit_report WHERE id=?`, id).Scan(
		&reportDate, &score, &level, &metricsRetrieveJSON, &metricsStoreJSON,
		&content, &problemCasesJSON, &status, &errMsg, &model, &tokens, &apiURL, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("报告不存在: %v", err)
	}

	var metricsRetrieve, metricsStore interface{}
	json.Unmarshal(metricsRetrieveJSON, &metricsRetrieve)
	json.Unmarshal(metricsStoreJSON, &metricsStore)
	var problemCases interface{}
	json.Unmarshal(problemCasesJSON, &problemCases)

	return map[string]interface{}{
		"id": id, "report_date": reportDate, "overall_score": score, "score_level": level,
		"metrics_retrieve": metricsRetrieve, "metrics_store": metricsStore,
		"report_content": content, "problem_cases": problemCases,
		"status": status, "error_msg": errMsg, "llm_model": model,
		"llm_tokens_used": tokens, "llm_api_url": apiURL, "created_at": createdAt,
	}, nil
}

// GetPendingArchives 查询待归档列表
func GetPendingArchives(status string, page, pageSize int) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}

	conditions := []string{}
	args := []interface{}{}
	if status != "" {
		conditions = append(conditions, "pa.status = ?")
		args = append(args, status)
	}
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	if err := conn.QueryRow("SELECT COUNT(*) FROM sys_knowledge_pending_archive pa"+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT pa.id, pa.knowledge_id, pa.audit_report_id, pa.reason, pa.suggested_action,
		pa.status, IFNULL(pa.reviewed_by,''), IFNULL(pa.reviewed_at,''), pa.created_at,
		IFNULL(k.title,''), IFNULL(k.category,''), IFNULL(k.project,''), IFNULL(k.priority,'')
		FROM sys_knowledge_pending_archive pa
		LEFT JOIN sys_knowledge k ON pa.knowledge_id = k.id` + whereClause +
		" ORDER BY pa.id DESC LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, knowledgeID, reportID int
		var reason, action, status2, reviewer, reviewedAt, createdAt, title, category, project, priority string
		if err := rows.Scan(&id, &knowledgeID, &reportID, &reason, &action, &status2,
			&reviewer, &reviewedAt, &createdAt, &title, &category, &project, &priority); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "knowledge_id": knowledgeID, "audit_report_id": reportID,
			"reason": reason, "suggested_action": action, "status": status2,
			"reviewed_by": reviewer, "reviewed_at": reviewedAt, "created_at": createdAt,
			"knowledge_title": title, "knowledge_category": category,
			"knowledge_project": project, "knowledge_priority": priority,
		})
	}
	return result, total, nil
}

// ApprovePendingArchive 审核通过：执行归档动作 + 标记 done
func ApprovePendingArchive(id int, reviewer string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 查待归档记录
	var knowledgeID int
	var action, status string
	err = conn.QueryRow("SELECT knowledge_id, suggested_action, status FROM sys_knowledge_pending_archive WHERE id=?", id).
		Scan(&knowledgeID, &action, &status)
	if err != nil {
		return fmt.Errorf("待归档记录不存在: %v", err)
	}
	if status != "pending" {
		return fmt.Errorf("该记录状态为 %s，不能审核", status)
	}

	// 执行归档动作
	switch action {
	case "archive":
		// 软删除 sys_knowledge
		var qdrantID string
		conn.QueryRow("SELECT IFNULL(qdrant_id,'') FROM sys_knowledge WHERE id=?", knowledgeID).Scan(&qdrantID)
		_, err = conn.Exec("UPDATE sys_knowledge SET status='archived' WHERE id=?", knowledgeID)
		if err != nil {
			return fmt.Errorf("归档失败: %v", err)
		}
		// 删 Qdrant 向量（不阻断，失败只 log）
		if qdrantID != "" {
			if err := QdrantDelete(qdrantID); err != nil {
				log.Printf("[audit] 审核通过但 Qdrant 删除失败（不阻断）knowledge_id=%d qdrant_id=%s: %v", knowledgeID, qdrantID, err)
			}
		}
	case "fix_category", "fix_project":
		// 这两种动作需要人工指定新值，暂不自动执行，只标记 done
		log.Printf("[audit] fix 类动作需人工处理 knowledge_id=%d action=%s", knowledgeID, action)
	default:
		return fmt.Errorf("未知动作: %s", action)
	}

	// 标记 done
	_, err = conn.Exec("UPDATE sys_knowledge_pending_archive SET status='done', reviewed_by=?, reviewed_at=NOW() WHERE id=?",
		reviewer, id)
	return err
}

// RejectPendingArchive 审核驳回
func RejectPendingArchive(id int, reviewer string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("UPDATE sys_knowledge_pending_archive SET status='rejected', reviewed_by=?, reviewed_at=NOW() WHERE id=?",
		reviewer, id)
	return err
}
