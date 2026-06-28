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
)

// ── 智谱 Embedding-3 API 调用 ──
// 这个文件只负责：文字转向量 + 上传文件入库（旧的 sys_embedding 表）
// 知识蒸馏入库走 knowledge.go（sys_knowledge + Qdrant）
// 检索走 knowledge.go（Qdrant）

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

// GetEmbeddings 批量获取向量（分批 10 条，失败逐条重试）
func GetEmbeddings(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	batchSize := 10
	var allVectors [][]float64

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		vectors, err := getEmbeddingsBatch(batch)
		if err != nil {
			// 批量失败，逐条重试
			log.Printf("[embedding] 批量失败(%d条): %v，逐条重试", len(batch), err)
			for _, t := range batch {
				v, e := getEmbeddingsBatch([]string{t})
				if e != nil {
					log.Printf("[embedding] 单条也失败: %v, text[:50]=%s", e, t[:min(50, len(t))])
					return nil, fmt.Errorf("embedding 失败: %v", e)
				}
				allVectors = append(allVectors, v...)
			}
		} else {
			allVectors = append(allVectors, vectors...)
		}
	}

	return allVectors, nil
}

func getEmbeddingsBatch(texts []string) ([][]float64, error) {

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
		// 调试：打印请求体大小和前200字符
		log.Printf("[embedding-DEBUG] 条数=%d, body_size=%d, body[:200]=%s, resp=%s",
			len(texts), len(body), string(body[:min(200, len(body))]), string(respBody))
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

// ── 上传文件入库（适配新 sys_knowledge + Qdrant）──

// StoreEmbeddingsRaw 批量存储（上传文件专用，不做提炼，原样存储到新表）
func StoreEmbeddingsRaw(userID int, contents []string, source string) error {
	if len(contents) == 0 {
		return nil
	}

	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 覆盖上传：删除旧版本（按 source 前缀匹配 + 删 Qdrant）
	oldRows, _ := conn.Query("SELECT qdrant_id FROM sys_knowledge WHERE source = ? AND status = 'active'", source)
	var oldIDs []string
	for oldRows.Next() {
		var id string
		oldRows.Scan(&id)
		oldIDs = append(oldIDs, id)
	}
	oldRows.Close()
	for _, id := range oldIDs {
		QdrantDelete(id)
	}
	conn.Exec("UPDATE sys_knowledge SET status = 'archived' WHERE source = ?", source)

	// 内容级去重（SHA-256）
	var newContents []string
	for _, c := range contents {
		hash := sha256Hash(c)
		var exists int
		if err := conn.QueryRow("SELECT 1 FROM sys_knowledge WHERE content_hash = ? AND status = 'active'", hash).Scan(&exists); err != nil && err != sql.ErrNoRows {
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

	// 逐条写入 sys_knowledge + Qdrant
	for i, content := range newContents {
		hash := sha256Hash(content)
		pointID := GeneratePointID(content)
		summary := content
		if len(summary) > 100 {
			summary = summary[:100]
		}

		res, err := conn.Exec(`INSERT INTO sys_knowledge
			(user_id, project, category, title, summary, content, context, tags, source, priority, status, content_hash, qdrant_id)
			VALUES (?, 'general', 'document', ?, ?, ?, '', '["upload"]', ?, 'low', 'active', ?, ?)`,
			userID, source, summary, content, source, hash, pointID)
		if err != nil {
			log.Printf("[embedding] 插入 sys_knowledge 失败: %v", err)
			continue
		}
		knowledgeID, _ := res.LastInsertId()

		if err := QdrantUpsert(pointID, vectors[i], knowledgeID, "general", "document", "active"); err != nil {
			log.Printf("[embedding] Qdrant upsert 失败: %v", err)
		}
	}
	return nil
}

// GetUploadedFiles 获取用户上传的文件列表（按 source 聚合，从新表查）
func GetUploadedFiles(userID int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(
		`SELECT source, COUNT(*) as chunks, MAX(created_at) as uploaded_at
		 FROM sys_knowledge WHERE source LIKE 'upload:%' AND status = 'active'
		 GROUP BY source ORDER BY uploaded_at DESC`)
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
		filename := strings.TrimPrefix(source, "upload:")
		files = append(files, map[string]interface{}{
			"filename":    filename,
			"chunks":      chunks,
			"uploaded_at": uploadedAt,
		})
	}
	return files, nil
}
