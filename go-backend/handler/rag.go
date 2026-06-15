package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// RagStatus 知识库状态
func RagStatus(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	count := service.GetEmbeddingCount(session.UserID)
	okResponse(w, map[string]interface{}{
		"engine":  "zhipu-embedding-3",
		"dim":     2048,
		"count":   count,
		"storage": "mysql",
	})
}

// RagTestEmbed 测试向量化
func RagTestEmbed(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]string
	json.NewDecoder(r.Body).Decode(&body)
	text := body["text"]
	if text == "" {
		errResponse(w, "请输入文本", 400)
		return
	}

	vec, err := service.GetEmbedding(text)
	if err != nil {
		errResponse(w, "向量化失败: "+err.Error(), 500)
		return
	}

	preview := []float64{}
	for i := 0; i < 5 && i < len(vec); i++ {
		preview = append(preview, vec[i])
	}

	okResponse(w, map[string]interface{}{
		"dimension": len(vec),
		"preview":   preview,
	})
}

// RagSearch 语义搜索
func RagSearch(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	query, _ := body["query"].(string)
	topK := 5
	if tk, ok := body["top_k"].(float64); ok {
		topK = int(tk)
	}

	results, err := service.SearchSimilar(query, topK, session.UserID)
	if err != nil {
		errResponse(w, "搜索失败: "+err.Error(), 500)
		return
	}

	okResponse(w, map[string]interface{}{
		"results": results,
	})
}
