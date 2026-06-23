package service

import (
	"ai-os-server/config"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── 向量存储（MySQL）──

func EnsureEmbeddingTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS sys_embedding (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL DEFAULT 0,
		content TEXT NOT NULL,
		content_hash VARCHAR(64) NOT NULL,
		vector BLOB NOT NULL,
		source VARCHAR(100) DEFAULT '',
		analyzed TINYINT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_content_hash (content_hash),
		INDEX idx_user_id (user_id),
		INDEX idx_source (source),
		INDEX idx_analyzed (analyzed)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		log.Printf("[embedding] 建表失败: %v", err)
	}
}

// ── 智谱 Embedding-3 API 调用 ──

// GetEmbeddingCount 获取用户的向量数量
func GetEmbeddingCount(userID int) int {
	conn, _ := GetDB()
	if conn == nil {
		return 0
	}
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM sys_embedding WHERE user_id = ?", userID).Scan(&count); err != nil {
		log.Printf("[embedding] 查询数量失败: %v", err)
	}
	return count
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// GetEmbedding 获取单条文本的向量
func GetEmbedding(text string) ([]float64, error) {
	vectors, err := GetEmbeddings([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding 返回空")
	}
	return vectors[0], nil
}

// GetEmbeddings 批量获取向量（最多 64 条）
func GetEmbeddings(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if len(texts) > 64 {
		texts = texts[:64]
	}

	reqBody := embeddingRequest{
		Model:      config.Embedding.Model,
		Input:      texts,
		Dimensions: config.Embedding.Dimensions,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", config.Embedding.APIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Embedding.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding API 返回错误 %d: %s", resp.StatusCode, string(respBody))
	}

	var result embeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 embedding 响应失败: %v", err)
	}

	// 按 index 排序
	vectors := make([][]float64, len(result.Data))
	for _, d := range result.Data {
		vectors[d.Index] = d.Embedding
	}
	return vectors, nil
}

// ── 知识提炼（GLM-4-Flash 免费模型）──

// summarizeForKnowledge 用 GLM-4-Flash 提炼对话中的可复用知识
// 返回值改为 []string：每个元素是一个独立的知识点
func summarizeForKnowledge(content string) ([]string, error) {
	if len(content) < 50 {
		return []string{content}, nil
	}

	prompt := fmt.Sprintf(`你是一个企业知识库提炼助手。请从以下对话中提取可复用的技术知识，要求：

1. 每个知识点必须独立成条，自带上下文标签，能脱离原文独立理解
2. 识别并保留以下内容：
   - 技术方案和架构决策
   - 代码片段、配置模板、命令行操作
   - Bug 根因和修复方法
   - 最佳实践和经验总结
   - API 用法、数据库操作、部署流程
   - 项目结构、模块职责说明
3. 去除以下内容：
   - 过渡语和寒暄
   - 纯操作过程描述
   - AI 的内部推理过程
   - 元对话
4. 每条知识点格式要求：
   - 以【分类标签】开头，如【架构决策】【Bug修复】【配置模板】【最佳实践】等
   - 包含完整的上下文信息，能独立理解
   - 控制在 50-300 字以内
5. 输出格式：JSON 数组，不要加任何额外标记
   示例：["【架构决策】数据库选用 MySQL 8.0，阿里云主从架构，主库写从库读","【配置模板】Go 后端端口 18731，Python Agent 端口 18732"]

如果对话中没有可复用的技术知识，输出空数组 []。

对话内容：
%s`, content)

	body := map[string]interface{}{
		"model": "glm-4-flash",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"response_format": map[string]string{"type": "json_object"},
	}
	bodyJSON, _ := json.Marshal(body)

	// 获取智谱 key
	conn, err := GetDB()
	if err != nil {
		return []string{content}, nil
	}
	var apiKey string
	if err := conn.QueryRow(`SELECT k.api_key FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey); err != nil || apiKey == "" {
		return []string{content}, nil // 没 key 直接用原文
	}

	req, _ := http.NewRequest("POST",
		"https://open.bigmodel.cn/api/coding/paas/v4/chat/completions",
		bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[embedding] 知识提炼失败: %v，使用原文", err)
		return []string{content}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		preview := string(respBody)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		log.Printf("[embedding] 知识提炼 HTTP %d: %s，使用原文", resp.StatusCode, preview)
		return []string{content}, nil
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if summary, ok := msg["content"].(string); ok && summary != "" {
					// 解析 JSON 数组
					points := parseKnowledgeJSON(summary)
					if len(points) > 0 {
						return points, nil
					}
				}
			}
		}
	}
	return []string{content}, nil
}

// parseKnowledgeJSON 解析 LLM 输出的知识点 JSON
// 兼容两种格式：["a","b"] 和 {"items":["a","b"]} 或 {"knowledge":["a","b"]}
func parseKnowledgeJSON(raw string) []string {
	raw = strings.TrimSpace(raw)

	// 尝试直接解析为 JSON 数组
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		var result []string
		for _, s := range arr {
			s = strings.TrimSpace(s)
			if len(s) >= 10 {
				result = append(result, s)
			}
		}
		return result
	}

	// 尝试解析为 JSON 对象，取常见字段名
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		for _, key := range []string{"items", "knowledge", "points", "result", "data"} {
			if arr, ok := obj[key].([]interface{}); ok {
				var result []string
				for _, v := range arr {
					if s, ok := v.(string); ok && len(strings.TrimSpace(s)) >= 10 {
						result = append(result, strings.TrimSpace(s))
					}
				}
				if len(result) > 0 {
					return result
				}
			}
		}
	}

	// JSON 解析失败，退化处理：按换行分割
	lines := strings.Split(raw, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, " \t\"[],")
		if len(line) >= 10 {
			result = append(result, line)
		}
	}
	return result
}

// ── 向量存储 ──

func StoreEmbedding(userID int, content, source string) error {
	// 质量门槛：过滤无价值的对话
	if !isQualityContent(content) {
		return nil
	}

	// 用 GLM-4-Flash 提炼，拆分为多个独立知识点
	points, err := summarizeForKnowledge(content)
	if err == nil && len(points) > 0 {
		// 过滤掉无价值标记
		var validPoints []string
		for _, p := range points {
			p = strings.TrimSpace(p)
			if p != "无价值" && p != "[]" && p != "" && isQualityContent(p) {
				validPoints = append(validPoints, p)
			}
		}
		if len(validPoints) == 0 {
			return nil
		}
		points = validPoints
	} else {
		points = []string{content}
	}

	// 逐条存储（每条独立向量化）
	conn, err := GetDB()
	if err != nil {
		return err
	}

	for _, point := range points {
		// 内容 hash 去重（按用户隔离）
		hash := contentHash(point)
		var exists int
		if err := conn.QueryRow("SELECT 1 FROM sys_embedding WHERE content_hash = ? AND user_id = ?", hash, userID).Scan(&exists); err != nil && err != sql.ErrNoRows {
			log.Printf("[embedding] 查询去重失败: %v", err)
		}
		if exists == 1 {
			continue // 已存在，跳过
		}

		vector, err := GetEmbedding(point)
		if err != nil {
			log.Printf("[embedding] 向量化失败: %v", err)
			continue
		}

		vectorJSON, err := json.Marshal(vector)
		if err != nil {
			continue
		}

		_, err = conn.Exec(
			"INSERT INTO sys_embedding (user_id, content, content_hash, vector, source) VALUES (?, ?, ?, ?, ?)",
			userID, point, hash, string(vectorJSON), source,
		)
		if err != nil {
			log.Printf("[embedding] 插入失败: %v", err)
		}
	}
	return nil
}

// isQualityContent 判断内容是否有知识价值
func isQualityContent(content string) bool {
	content = strings.TrimSpace(content)
	if len(content) < 100 {
		return false
	}

	// 过滤元对话：AI 自说自话、操作过程描述、提炼格式标记
	metaPatterns := []string{
		"The user wants to", "The user is asking",
		"Let me", "I'll", "I need to", "I should",
		"First,", "Next,", "Then I'll",
		"编译成功", "编译失败", "部署完成", "上传完成",
		"正在编译", "正在部署", "正在上传",
		"已删除", "已清空", "已重启",
		"OK,", "好的，", "让我", "我来",
		"【用户提问】", "【ai 回复要点】",
	}
	lower := strings.ToLower(content)
	for _, p := range metaPatterns {
		if strings.HasPrefix(lower, strings.ToLower(p)) {
			return false
		}
	}

	// 必须有实质内容：代码块、技术关键词、或结构化内容
	hasCode := strings.Contains(content, "```") || strings.Contains(content, "`")
	hasTechnical := strings.Contains(lower, "api") ||
		strings.Contains(lower, "config") ||
		strings.Contains(lower, "数据库") ||
		strings.Contains(lower, "sql") ||
		strings.Contains(lower, "架构") ||
		strings.Contains(lower, "方案") ||
		strings.Contains(lower, "bug") ||
		strings.Contains(lower, "修复") ||
		strings.Contains(lower, "优化") ||
		strings.Contains(lower, "部署") ||
		strings.Contains(lower, "docker") ||
		strings.Contains(lower, "nginx") ||
		strings.Contains(lower, "linux") ||
		strings.Contains(lower, "function") ||
		strings.Contains(lower, "class") ||
		strings.Contains(lower, "import") ||
		strings.Contains(lower, "package") ||
		strings.Contains(lower, "模块") ||
		strings.Contains(lower, "接口") ||
		strings.Contains(lower, "路由") ||
		strings.Contains(lower, "中间件")

	if !hasCode && !hasTechnical {
		return false
	}

	return true
}

// StoreEmbeddings 批量存储（用于对话知识，会走提炼）
func StoreEmbeddings(userID int, contents []string, source string) error {
	if len(contents) == 0 {
		return nil
	}

	conn, err := GetDB()
	if err != nil {
		return err
	}

	var newContents []string
	for _, c := range contents {
		hash := contentHash(c)
		var exists int
		if err := conn.QueryRow("SELECT 1 FROM sys_embedding WHERE content_hash = ? AND user_id = ?", hash, userID).Scan(&exists); err != nil && err != sql.ErrNoRows {
			log.Printf("[embedding] 查询去重失败: %v", err)
		}
		if exists == 0 {
			newContents = append(newContents, c)
		}
	}

	if len(newContents) == 0 {
		return nil
	}

	// 批量获取向量
	vectors, err := GetEmbeddings(newContents)
	if err != nil {
		return err
	}

	// 批量插入
	for i, content := range newContents {
		vectorJSON, _ := json.Marshal(vectors[i])
		hash := contentHash(content)
		if _, err := conn.Exec(
			"INSERT INTO sys_embedding (user_id, content, content_hash, vector, source) VALUES (?, ?, ?, ?, ?)",
			userID, content, hash, string(vectorJSON), source,
		); err != nil {
			log.Printf("[embedding] 插入失败: %v", err)
		}
	}
	return nil
}

// StoreEmbeddingsRaw 批量存储（上传文件专用，不做提炼，原样存储）
func StoreEmbeddingsRaw(userID int, contents []string, source string) error {
	if len(contents) == 0 {
		return nil
	}

	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 覆盖上传：删除旧版本分块
	conn.Exec("DELETE FROM sys_embedding WHERE user_id = ? AND source = ?", userID, source)

	// 内容级去重
	var newContents []string
	for _, c := range contents {
		hash := contentHash(c)
		var exists int
		if err := conn.QueryRow("SELECT 1 FROM sys_embedding WHERE content_hash = ? AND user_id = ?", hash, userID).Scan(&exists); err != nil && err != sql.ErrNoRows {
			log.Printf("[embedding] 查询去重失败: %v", err)
		}
		if exists == 0 {
			newContents = append(newContents, c)
		}
	}

	if len(newContents) == 0 {
		return nil
	}

	// 批量获取向量
	vectors, err := GetEmbeddings(newContents)
	if err != nil {
		return err
	}

	// 批量插入
	for i, content := range newContents {
		vectorJSON, _ := json.Marshal(vectors[i])
		hash := contentHash(content)
		if _, err := conn.Exec(
			"INSERT INTO sys_embedding (user_id, content, content_hash, vector, source) VALUES (?, ?, ?, ?, ?)",
			userID, content, hash, string(vectorJSON), source,
		); err != nil {
			log.Printf("[embedding] 插入失败: %v", err)
		}
	}
	return nil
}

// ── 语义搜索 ──

type SearchResult struct {
	ID        int64
	Content   string
	Source    string
	Score     float64
	CreatedAt string
}

func SearchSimilar(query string, topK int, userID int) ([]SearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// 查询分片：把长问题拆成多个子查询，独立检索后合并
	subQueries := splitQuery(query)
	if len(subQueries) == 0 {
		subQueries = []string{query}
	}

	// 批量获取所有子查询的向量
	queryVectors, err := GetEmbeddings(subQueries)
	if err != nil {
		return nil, err
	}
	if len(queryVectors) == 0 {
		return nil, nil
	}

	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 查询全量向量（不再限制 1000 条，让老知识也能被检索到）
	var whereSQL string
	var args []interface{}
	if userID > 0 {
		whereSQL = "WHERE user_id = ?"
		args = append(args, userID)
	}

	rows, err := conn.Query(
		fmt.Sprintf("SELECT id, content, source, vector, created_at FROM sys_embedding %s ORDER BY id DESC", whereSQL),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		id        int64
		content   string
		source    string
		vector    []float64
		createdAt string
	}
	var items []item
	for rows.Next() {
		var id int64
		var content, source, vectorStr, createdAt string
		if err := rows.Scan(&id, &content, &source, &vectorStr, &createdAt); err != nil {
			log.Printf("[embedding] scan 失败: %v", err)
			continue
		}
		var vector []float64
		if err := json.Unmarshal([]byte(vectorStr), &vector); err != nil {
			log.Printf("[embedding] 解析 vector 失败: %v", err)
			continue
		}
		items = append(items, item{id, content, source, vector, createdAt})
	}

	// 对每个知识条目，取所有子查询中的最高相似度作为最终分数
	// （一个问题里只要有一个子句匹配到，就应该召回）
	scoreMap := make(map[int64]float64) // id → 最高分
	for _, it := range items {
		bestScore := 0.0
		for _, qv := range queryVectors {
			score := cosineSimilarity(qv, it.vector)
			if score > bestScore {
				bestScore = score
			}
		}
		scoreMap[it.id] = bestScore
	}

	var results []SearchResult
	for _, it := range items {
		results = append(results, SearchResult{
			ID:        it.id,
			Content:   it.content,
			Source:    it.source,
			Score:     scoreMap[it.id],
			CreatedAt: it.createdAt,
		})
	}

	// 排序取 topK
	sortResults(results)
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

// splitQuery 将长查询拆分为多个子查询（按标点分句）
// 目的：一个复杂问题里的多个意图独立检索，避免语义稀释
// 例如 "Vue 3 前端，Go 后端，数据库迁移" → 3 个子查询各自检索
func splitQuery(query string) []string {
	query = strings.TrimSpace(query)
	if len(query) <= 50 {
		// 短查询不需要分片
		return []string{query}
	}

	// 按中文标点（，。；！？）和英文标点（,.;!?\n）分句
	var parts []string
	for _, sep := range []string{"，", "。", "；", "！", "？", ",", ";", "!", "?", "\n"} {
		if strings.Contains(query, sep) {
			for _, p := range strings.Split(query, sep) {
				p = strings.TrimSpace(p)
				if len(p) >= 10 { // 太短的片段跳过（如"请问"、"帮我"）
					parts = append(parts, p)
				}
			}
			break // 用第一个匹配的分隔符切一次就够了
		}
	}

	if len(parts) <= 1 {
		return []string{query}
	}

	// 最多拆 4 个子查询（太多会导致 embedding API 调用过多）
	if len(parts) > 4 {
		parts = parts[:4]
	}

	return parts
}

// ── 辅助函数 ──

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func sortResults(r []SearchResult) {
	for i := 0; i < len(r); i++ {
		for j := i + 1; j < len(r); j++ {
			if r[j].Score > r[i].Score {
				r[i], r[j] = r[j], r[i]
			}
		}
	}
}

func contentHash(s string) string {
	// 简单 hash（用 SHA256 更好，但这里先用简单方案）
	h := uint32(2166136261)
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// ── 用于 AI 建议等场景：获取最近的知识库内容 ──

func GetRecentEmbeddings(userID int, limit int) ([]SearchResult, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := conn.Query(
		"SELECT id, content, source, created_at FROM sys_embedding WHERE user_id = ? ORDER BY created_at DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.Content, &r.Source, &r.CreatedAt); err != nil {
			log.Printf("[embedding] scan 失败: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// 确保 import 不报错
var (
	_ = strings.TrimSpace
	_ = sql.ErrNoRows
	_ = sync.Mutex{}
)

// DeleteEmbedding 删除单条知识
func DeleteEmbedding(id int64, userID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("DELETE FROM sys_embedding WHERE id = ? AND user_id = ?", id, userID)
	return err
}

// DeleteEmbeddingsBySource 按来源批量删除
func DeleteEmbeddingsBySource(userID int, source string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	result, err := conn.Exec("DELETE FROM sys_embedding WHERE user_id = ? AND source = ?", userID, source)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// GetUploadedFiles 获取用户上传的文件列表（按 source 聚合）
func GetUploadedFiles(userID int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(
		`SELECT source, COUNT(*) as chunks, MAX(created_at) as uploaded_at
		 FROM sys_embedding WHERE user_id = ? AND source LIKE 'upload:%'
		 GROUP BY source ORDER BY uploaded_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []map[string]interface{}
	for rows.Next() {
		var source string
		var chunks int
		var uploadedAt string
		if err := rows.Scan(&source, &chunks, &uploadedAt); err != nil {
			continue
		}
		// source 格式: "upload:filename.pdf"
		filename := strings.TrimPrefix(source, "upload:")
		files = append(files, map[string]interface{}{
			"filename":    filename,
			"chunks":      chunks,
			"uploaded_at": uploadedAt,
		})
	}
	return files, nil
}
