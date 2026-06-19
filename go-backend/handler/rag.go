package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
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

// RagList 知识库列表（按用户过滤）
func RagList(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	results, err := service.GetRecentEmbeddings(session.UserID, 1000)
	if err != nil {
		errResponse(w, "查询失败: "+err.Error(), 500)
		return
	}

	// 转换为前端需要的格式
	type DocItem struct {
		ID       int64  `json:"id"`
		Text     string `json:"text"`
		Metadata struct {
			Source    string `json:"source"`
			CreatedAt string `json:"created_at"`
		} `json:"metadata"`
	}

	docs := make([]DocItem, 0, len(results))
	for _, r := range results {
		var doc DocItem
		doc.ID = r.ID
		doc.Text = r.Content
		doc.Metadata.Source = r.Source
		doc.Metadata.CreatedAt = r.CreatedAt
		docs = append(docs, doc)
	}

	okResponse(w, map[string]interface{}{
		"documents": docs,
		"total":     len(docs),
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

// RagUpload 上传文件并向量化入库
func RagUpload(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	// 限制 50MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		errResponse(w, "文件太大（最大50MB）", 400)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		errResponse(w, "请选择文件", 400)
		return
	}
	defer file.Close()

	filename := header.Filename
	ext := strings.ToLower(filepath.Ext(filename))

	// 校验文件类型
	allowed := map[string]bool{
		".txt": true, ".md": true, ".markdown": true,
		".go": true, ".py": true, ".js": true, ".ts": true, ".vue": true,
		".java": true, ".c": true, ".cpp": true, ".h": true, ".rs": true,
		".sql": true, ".yaml": true, ".yml": true, ".json": true,
		".xml": true, ".html": true, ".css": true, ".sh": true,
		".bat": true, ".ini": true, ".cfg": true, ".toml": true,
		".pdf": true, ".docx": true, ".xlsx": true,
	}
	if !allowed[ext] {
		errResponse(w, fmt.Sprintf("不支持的文件格式: %s", ext), 400)
		return
	}

	// 解析文件内容
	text, err := service.ParseFile(filename, file)
	if err != nil {
		errResponse(w, "文件解析失败: "+err.Error(), 400)
		return
	}

	if len(strings.TrimSpace(text)) < 50 {
		errResponse(w, "文件内容太少（至少50字符）", 400)
		return
	}

	// 智能分块（根据文件类型选择策略）
	chunks := service.ChunkByType(text, ext, service.ChunkConfig{
		MinSize: 200,
		MaxSize: 2000,
	})

	if len(chunks) == 0 {
		chunks = []string{text}
	}

	// 批量向量化入库（上传文件不做提炼，原样存储）
	source := "upload:" + filename
	err = service.StoreEmbeddingsRaw(session.UserID, chunks, source)
	if err != nil {
		errResponse(w, "向量化入库失败: "+err.Error(), 500)
		return
	}

	okResponse(w, map[string]interface{}{
		"filename": filename,
		"chunks":   len(chunks),
		"size":     len(text),
	})
}

// RagDelete 删除知识库条目
func RagDelete(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	// 支持按 ID 删除单条
	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			errResponse(w, "无效的 ID", 400)
			return
		}
		if err := service.DeleteEmbedding(id, session.UserID); err != nil {
			errResponse(w, "删除失败: "+err.Error(), 500)
			return
		}
		okResponse(w, map[string]interface{}{"deleted": id})
		return
	}

	// 支持按 source 批量删除（删除整个文件的所有分块）
	source := r.URL.Query().Get("source")
	if source != "" {
		n, err := service.DeleteEmbeddingsBySource(session.UserID, source)
		if err != nil {
			errResponse(w, "删除失败: "+err.Error(), 500)
			return
		}
		okResponse(w, map[string]interface{}{"deleted": n})
		return
	}

	errResponse(w, "请指定 id 或 source 参数", 400)
}

// RagFiles 获取已上传文件列表
func RagFiles(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	files, err := service.GetUploadedFiles(session.UserID)
	if err != nil {
		errResponse(w, "查询失败: "+err.Error(), 500)
		return
	}

	okResponse(w, map[string]interface{}{
		"files": files,
	})
}
