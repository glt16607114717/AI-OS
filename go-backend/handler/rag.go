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
	stats, err := service.GetKnowledgeStats()
	if err != nil {
		stats = map[string]int{}
	}
	okResponse(w, map[string]interface{}{
		"engine":  "zhipu-embedding-3 + qdrant",
		"dim":     2048,
		"count":   stats["total"],
		"storage": "mysql + qdrant",
		"by_project": stats,
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

// RagList 知识库列表（支持按 project/category 过滤）
func RagList(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	project := r.URL.Query().Get("project")
	category := r.URL.Query().Get("category")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = v
		}
	}

	items, total, err := service.ListKnowledge(project, category, limit, offset)
	if err != nil {
		errResponse(w, "查询失败: "+err.Error(), 500)
		return
	}

	okResponse(w, map[string]interface{}{
		"documents": items,
		"total":     total,
	})
}

// RagSearch 语义搜索（Qdrant 向量检索 + 项目过滤）
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

	// 项目过滤：支持前端传入 projects 数组，默认搜全部项目
	var projects []string
	if ps, ok := body["projects"].([]interface{}); ok {
		for _, p := range ps {
			if s, ok := p.(string); ok && s != "" {
				projects = append(projects, s)
			}
		}
	}
	if len(projects) == 0 {
		projects = []string{"ai-os", "rmp", "general"}
	}

	// 类型过滤
	var categories []string
	if cs, ok := body["categories"].([]interface{}); ok {
		for _, c := range cs {
			if s, ok := c.(string); ok && s != "" {
				categories = append(categories, s)
			}
		}
	}

	results, err := service.SearchKnowledge(query, topK, projects, categories)
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

// RagDelete 删除知识库条目（软删除：归档）
func RagDelete(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		errResponse(w, "请指定 id 参数", 400)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errResponse(w, "无效的 ID", 400)
		return
	}
	if err := service.DeleteKnowledge(id); err != nil {
		errResponse(w, "删除失败: "+err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"deleted": id})
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

// RagMigrate 迁移旧 sys_embedding 数据到 sys_knowledge + Qdrant（管理员一次性操作）
func RagMigrate(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	if session == nil {
		errResponse(w, "未登录", 401)
		return
	}
	if !session.IsAdmin {
		errResponse(w, "需要管理员权限", 403)
		return
	}

	migrated, skipped, failed, err := service.MigrateEmbeddingsToKnowledge()
	if err != nil {
		errResponse(w, "迁移失败: "+err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{
		"migrated": migrated,
		"skipped":  skipped,
		"failed":   failed,
	})
}
