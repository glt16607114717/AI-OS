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

// summarizeForKnowledge 用 GLM-4-Flash 提炼对话核心知识
func summarizeForKnowledge(content string) (string, error) {
	if len(content) < 50 {
		return content, nil // 太短不提炼
	}

	prompt := fmt.Sprintf(`你是一个知识提炼助手。请从以下对话中提取知识，要求：

1. 用户提问：完整保留，一字不改
2. AI 回复：精简提炼，去除以下内容：
   - 过渡语（"让我看看"、"我来帮你"等）
   - 寒暄和客套话
   - 重复表述
   - 纯操作过程描述（"正在编译..."、"部署完成"等状态信息）
3. AI 回复中必须保留：
   - 技术方案和架构决策
   - 代码片段、配置、命令
   - Bug 根因和修复方法
   - 最佳实践和经验总结
4. 输出格式：
【用户提问】
（原始提问内容）

【AI 回复要点】
（提炼后的核心知识）

对话内容：
%s`, content)

	body := map[string]interface{}{
		"model": "glm-4-flash",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}
	bodyJSON, _ := json.Marshal(body)

	// 获取智谱 key
	conn, err := GetDB()
	if err != nil {
		return content, nil
	}
	var apiKey string
	if err := conn.QueryRow(`SELECT k.api_key FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey); err != nil || apiKey == "" {
		return content, nil // 没 key 直接用原文
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
		return content, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[embedding] 知识提炼 HTTP %d: %s，使用原文", resp.StatusCode, string(respBody)[:200])
		return content, nil
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if summary, ok := msg["content"].(string); ok && summary != "" {
					return summary, nil
				}
			}
		}
	}
	return content, nil
}

// ── 向量存储 ──

func StoreEmbedding(userID int, content, source string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 先用 GLM-4-Flash 提炼核心知识（异步场景下可接受延迟）
	summarized, err := summarizeForKnowledge(content)
	if err == nil && summarized != "" {
		content = summarized
	}

	// 计算内容 hash 去重（按用户隔离）
	hash := contentHash(content)
	var exists int
	if err := conn.QueryRow("SELECT 1 FROM sys_embedding WHERE content_hash = ? AND user_id = ?", hash, userID).Scan(&exists); err != nil && err != sql.ErrNoRows {
		log.Printf("[embedding] 查询去重失败: %v", err)
	}
	if exists == 1 {
		return nil // 已存在，跳过
	}

	// 获取向量
	vector, err := GetEmbedding(content)
	if err != nil {
		return err
	}

	// 序列化向量
	vectorJSON, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("序列化向量失败: %v", err)
	}

	_, err = conn.Exec(
		"INSERT INTO sys_embedding (user_id, content, content_hash, vector, source) VALUES (?, ?, ?, ?, ?)",
		userID, content, hash, string(vectorJSON), source,
	)
	if err != nil {
		return fmt.Errorf("插入 embedding 失败: %v", err)
	}
	return nil
}

// StoreEmbeddings 批量存储
func StoreEmbeddings(userID int, contents []string, source string) error {
	if len(contents) == 0 {
		return nil
	}

	// 过滤已存在的
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

	// 获取查询向量
	queryVector, err := GetEmbedding(query)
	if err != nil {
		return nil, err
	}

	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 查询所有向量（小数据量方案，后续可换 pgvector/milvus）
	var whereSQL string
	var args []interface{}
	if userID > 0 {
		whereSQL = "WHERE user_id = ?"
		args = append(args, userID)
	}

	rows, err := conn.Query(
		fmt.Sprintf("SELECT id, content, source, vector, created_at FROM sys_embedding %s ORDER BY id DESC LIMIT 1000", whereSQL),
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

	// 计算余弦相似度
	var results []SearchResult
	for _, it := range items {
		score := cosineSimilarity(queryVector, it.vector)
		results = append(results, SearchResult{
			ID:        it.id,
			Content:   it.content,
			Source:    it.source,
			Score:     score,
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
