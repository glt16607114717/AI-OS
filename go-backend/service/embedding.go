package service

import (
	"ai-os-server/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ── 智谱 Embedding-3 API 调用 ──
// 职责：调用智谱 embedding API 获取向量
// 向量存储和检索由 qdrant.go + knowledge.go 负责

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
