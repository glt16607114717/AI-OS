package service

import (
	"ai-os-server/config"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ── Qdrant 向量库客户端（REST API 版本）──
// 职责：存向量 + 语义检索（带过滤）
// 元数据存 MySQL sys_knowledge，向量存这里
// 用 HTTP REST API（端口 6333），不依赖 gRPC，编译体积小

var qdrantHTTPClient *http.Client

func init() {
	qdrantHTTPClient = &http.Client{Timeout: 30 * time.Second}
}

// qdrantURL 拼接完整 URL
func qdrantURL(path string) string {
	return fmt.Sprintf("http://%s:%d%s", config.Qdrant.Host, 6333, path)
}

// qdrantDo 发送请求
func qdrantDo(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, qdrantURL(path), reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := qdrantHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant 请求失败: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		return nil, fmt.Errorf("qdrant HTTP %d: %s", resp.StatusCode, preview)
	}
	return respBody, nil
}

// InitQdrant 初始化 Qdrant 客户端并确保集合存在
func InitQdrant() error {
	addr := fmt.Sprintf("%s:%d", config.Qdrant.Host, 6333)

	// 健康检查
	_, err := qdrantDo("GET", "/healthz", nil)
	if err != nil {
		return fmt.Errorf("qdrant 连接失败 %s: %v", addr, err)
	}

	// 确保集合存在
	if err := ensureQdrantCollection(); err != nil {
		return fmt.Errorf("qdrant 集合初始化失败: %v", err)
	}

	log.Printf("[qdrant] 连接成功 %s, 集合: %s", addr, config.Qdrant.Collection)
	return nil
}

// ensureQdrantCollection 创建集合（如果不存在）
// 向量维度 2048（智谱 Embedding-3），距离用余弦
func ensureQdrantCollection() error {
	collection := config.Qdrant.Collection

	// 检查是否存在
	respBytes, err := qdrantDo("GET", fmt.Sprintf("/collections/%s", collection), nil)
	if err == nil {
		// 集合已存在
		var info map[string]interface{}
		if json.Unmarshal(respBytes, &info) == nil {
			if _, ok := info["result"]; ok {
				return nil
			}
		}
	}

	// 创建集合
	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     config.Embedding.Dimensions,
			"distance": "Cosine",
		},
	}
	_, err = qdrantDo("PUT", fmt.Sprintf("/collections/%s?wait=true", collection), createBody)
	if err != nil {
		return fmt.Errorf("创建集合失败: %v", err)
	}

	// 创建 payload 索引（用于过滤的字段）
	indexFields := []string{"project", "category", "status", "knowledge_id"}
	for _, field := range indexFields {
		indexBody := map[string]interface{}{
			"field_name": field,
			"field_schema": map[string]interface{}{
				"type": "keyword",
			},
		}
		_, err := qdrantDo("PUT", fmt.Sprintf("/collections/%s/index?wait=true", collection), indexBody)
		if err != nil {
			log.Printf("[qdrant] 创建索引 %s 失败（不致命）: %v", field, err)
		}
	}

	log.Printf("[qdrant] 集合 %s 创建完成，维度 %d", collection, config.Embedding.Dimensions)
	return nil
}

// QdrantUpsert 存入一条向量
func QdrantUpsert(pointID string, vector []float64, knowledgeID int64, project, category, status string) error {
	collection := config.Qdrant.Collection

	// float64 → float32（Qdrant REST 也接受 float32 数组）
	vec32 := make([]float32, len(vector))
	for i, v := range vector {
		vec32[i] = float32(v)
	}

	upsertBody := map[string]interface{}{
		"points": []map[string]interface{}{
			{
				"id": pointID,
				"vector": vec32,
				"payload": map[string]interface{}{
					"knowledge_id": knowledgeID,
					"project":      project,
					"category":     category,
					"status":       status,
				},
			},
		},
	}

	_, err := qdrantDo("PUT", fmt.Sprintf("/collections/%s/points?wait=true", collection), upsertBody)
	if err != nil {
		return fmt.Errorf("qdrant upsert 失败: %v", err)
	}
	return nil
}

// QdrantHit 检索命中结果
type QdrantHit struct {
	KnowledgeID int64
	Score       float64
}

// QdrantSearch 检索向量（带项目过滤，忽略 user_id）
func QdrantSearch(queryVector []float64, topK int, projects []string, categories []string) ([]QdrantHit, error) {
	if topK <= 0 {
		topK = 10
	}
	collection := config.Qdrant.Collection

	// float64 → float32
	vec32 := make([]float32, len(queryVector))
	for i, v := range queryVector {
		vec32[i] = float32(v)
	}

	// 构建过滤条件
	// 过滤逻辑：status=active 必须，project 必须（OR 语义），category 可选（OR 语义）
	// 用嵌套 filter 保证 project 和 category 是 AND 关系
	must := []map[string]interface{}{
		{"key": "status", "match": map[string]interface{}{"value": "active"}},
	}

	// project：用 must 嵌套 should（project 之间是 OR，但 project 整体是 must）
	if len(projects) > 0 {
		projectShould := make([]map[string]interface{}, 0, len(projects))
		for _, p := range projects {
			projectShould = append(projectShould, map[string]interface{}{
				"key":   "project",
				"match": map[string]interface{}{"value": p},
			})
		}
		must = append(must, map[string]interface{}{
			"should": projectShould,
		})
	}

	// category：同理，用 must 嵌套 should
	if len(categories) > 0 {
		categoryShould := make([]map[string]interface{}, 0, len(categories))
		for _, c := range categories {
			categoryShould = append(categoryShould, map[string]interface{}{
				"key":   "category",
				"match": map[string]interface{}{"value": c},
			})
		}
		must = append(must, map[string]interface{}{
			"should": categoryShould,
		})
	}

	filter := map[string]interface{}{
		"must": must,
	}

	searchBody := map[string]interface{}{
		"vector":      vec32,
		"filter":      filter,
		"limit":       topK,
		"with_payload": true,
	}

	respBytes, err := qdrantDo("POST", fmt.Sprintf("/collections/%s/points/search", collection), searchBody)
	if err != nil {
		return nil, fmt.Errorf("qdrant 搜索失败: %v", err)
	}

	// 解析响应
	var resp struct {
		Result []struct {
			ID      string                 `json:"id"`
			Score   float64                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("解析 qdrant 响应失败: %v", err)
	}

	hits := make([]QdrantHit, 0, len(resp.Result))
	for _, point := range resp.Result {
		var knowledgeID int64
		if v, ok := point.Payload["knowledge_id"]; ok {
			switch val := v.(type) {
			case float64:
				knowledgeID = int64(val)
			case json.Number:
				knowledgeID, _ = val.Int64()
			}
		}
		hits = append(hits, QdrantHit{
			KnowledgeID: knowledgeID,
			Score:       point.Score,
		})
	}
	return hits, nil
}

// QdrantDelete 删除一个 point（按 UUID）
func QdrantDelete(pointID string) error {
	collection := config.Qdrant.Collection
	body := map[string]interface{}{
		"points": []string{pointID},
	}
	_, err := qdrantDo("POST", fmt.Sprintf("/collections/%s/points/delete?wait=true", collection), body)
	return err
}

// QdrantDeleteByKnowledgeID 按 knowledge_id 删除
func QdrantDeleteByKnowledgeID(knowledgeID int64) error {
	collection := config.Qdrant.Collection
	body := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{"key": "knowledge_id", "match": map[string]interface{}{"value": knowledgeID}},
			},
		},
	}
	_, err := qdrantDo("POST", fmt.Sprintf("/collections/%s/points/delete?wait=true", collection), body)
	return err
}

// GeneratePointID 根据内容 hash 生成确定性 UUID
// 同样的内容生成同样的 UUID，避免重复存储
func GeneratePointID(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(h[0:4]),
		hex.EncodeToString(h[4:6]),
		hex.EncodeToString(h[6:8]),
		hex.EncodeToString(h[8:10]),
		hex.EncodeToString(h[10:16]),
	)
}

// 确保 context 包被引用（预留扩展）
var _ = context.Background
