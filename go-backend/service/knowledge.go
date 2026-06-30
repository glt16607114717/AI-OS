package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ── 知识库服务层 ──
// 职责：蒸馏入库（存全字段 + 向量存 Qdrant）+ 检索（按项目过滤）+ 迁移
// 元数据存 MySQL sys_knowledge，向量存 Qdrant

// KnowledgeItem 知识条目（完整结构）
type KnowledgeItem struct {
	ID        int64    `json:"id"`
	UserID    int      `json:"user_id"`
	Username  string   `json:"username"` // 产生该知识的用户名
	Project   string   `json:"project"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Content   string   `json:"content"`
	Context   string   `json:"context"`
	Tags      []string `json:"tags"`
	Source    string   `json:"source"`
	Priority  string   `json:"priority"`
	Status    string   `json:"status"`
	Score     float64  `json:"score,omitempty"` // 检索时带相似度
	CreatedAt string   `json:"created_at"`
}

// 维度 → category 映射
// 蒸馏 prompt 产出的 dimension: decisions/pitfalls/business/habits
var dimensionToCategory = map[string]string{
	"decisions": "decision",
	"pitfalls":  "pitfall",
	"business":  "business",
	"habits":    "habit",
}

// sha256Hash 计算 SHA-256（替代旧的 FNV-1a，降低碰撞）
func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// StoreDistillKnowledge 存储蒸馏产出的知识（完整字段 + 向量）
// 参数：
//   - userID: 入库者（记录用，检索不按用户隔离）
//   - date: 蒸馏日期
//   - k: 蒸馏产出的结构化知识
// 返回：是否新存入（false=重复跳过）
func StoreDistillKnowledge(userID int, date string, k DistillKnowledge) (bool, error) {
	if k.Content == "" || k.Dimension == "" {
		return false, nil
	}

	conn, err := GetDB()
	if err != nil {
		return false, err
	}

	// 归一化 category
	category := dimensionToCategory[strings.ToLower(k.Dimension)]
	if category == "" {
		category = "other"
	}

	// 归一化 priority
	priority := k.Priority
	if priority != "high" && priority != "medium" && priority != "low" {
		priority = "medium"
	}

	// 归一化 project（由 LLM 判断，兜底 general）
	project := k.Project
	if project == "" {
		project = "general"
	}

	// 内容去重（SHA-256，跨用户去重——所有用户共享一套知识库）
	hash := sha256Hash(k.Content)
	var exists int
	if err := conn.QueryRow("SELECT 1 FROM sys_knowledge WHERE content_hash = ? AND status = 'active'", hash).Scan(&exists); err != nil && err != sql.ErrNoRows {
		log.Printf("[knowledge] 去重查询失败: %v", err)
	}
	if exists == 1 {
		return false, nil
	}

	// tags 转 JSON
	tagsJSON := "[]"
	if len(k.Tags) > 0 {
		if b, err := json.Marshal(k.Tags); err == nil {
			tagsJSON = string(b)
		}
	}

	// summary：没有则从 title 或 content 截取
	summary := k.Title
	if summary == "" && len(k.Content) > 0 {
		if len(k.Content) > 100 {
			summary = k.Content[:100]
		} else {
			summary = k.Content
		}
	}
	if len(summary) > 500 {
		summary = summary[:500]
	}

	source := fmt.Sprintf("distill:%s:%s:%s", date, k.Dimension, k.Title)

	// 1. 先写入 MySQL（拿到自增 id）
	res, err := conn.Exec(`INSERT INTO sys_knowledge
		(user_id, project, category, title, summary, content, context, tags, source, priority, status, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?)`,
		userID, project, category, k.Title, summary, k.Content, k.Context, tagsJSON, source, priority, hash)
	if err != nil {
		return false, fmt.Errorf("写入 sys_knowledge 失败: %v", err)
	}
	knowledgeID, _ := res.LastInsertId()

	// 2. 向量化 + 存 Qdrant
	vector, err := GetEmbedding(k.Content)
	if err != nil {
		log.Printf("[knowledge] 向量化失败（知识已入库但无向量，不可检索）: %v", err)
		return true, nil
	}

	pointID := GeneratePointID(k.Content)
	if err := QdrantUpsert(pointID, vector, knowledgeID, project, category, "active"); err != nil {
		log.Printf("[knowledge] Qdrant 存入失败（知识已入库但向量未存）: %v", err)
		return true, nil
	}

	// 3. 回填 qdrant_id
	conn.Exec("UPDATE sys_knowledge SET qdrant_id = ? WHERE id = ?", pointID, knowledgeID)

	return true, nil
}

// SearchKnowledge 检索知识（带项目过滤，忽略 user_id）
// 参数：
//   - query: 查询文本
//   - topK: 返回条数
//   - projects: 允许的项目（如 ["ai-os", "general"]）
//   - categories: 允许的类型（空则不过滤）
func SearchKnowledge(query string, topK int, projects []string, categories []string) ([]KnowledgeItem, error) {
	if topK <= 0 {
		topK = 10
	}

	// 1. 查询向量
	vector, err := GetEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("查询向量化失败: %v", err)
	}

	// 2. Qdrant 检索（带过滤）
	hits, err := QdrantSearch(vector, topK, projects, categories)
	if err != nil {
		log.Printf("[knowledge] Qdrant 检索失败，降级返回空: %v", err)
		return nil, nil
	}
	if len(hits) == 0 {
		return nil, nil
	}

	// 3. 回 MySQL 取完整内容
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 构建 IN 查询
	ids := make([]interface{}, len(hits))
	idScore := make(map[int64]float64, len(hits))
	for i, h := range hits {
		ids[i] = h.KnowledgeID
		idScore[h.KnowledgeID] = h.Score
	}

	placeholders := make([]string, len(ids))
	for i := range ids {
		placeholders[i] = "?"
	}
	querySQL := fmt.Sprintf(
		"SELECT k.id, k.user_id, COALESCE(u.username, '') as username, k.project, k.category, k.title, k.summary, k.content, k.context, k.tags, k.source, k.priority, k.status, k.created_at FROM sys_knowledge k LEFT JOIN sys_user u ON k.user_id = u.id WHERE k.id IN (%s) AND k.status = 'active'",
		strings.Join(placeholders, ","),
	)

	rows, err := conn.Query(querySQL, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 按 score 排序
	var items []KnowledgeItem
	for rows.Next() {
		var item KnowledgeItem
		var tagsJSON string
		var contextStr sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Project, &item.Category, &item.Title, &item.Summary, &item.Content, &contextStr, &tagsJSON, &item.Source, &item.Priority, &item.Status, &item.CreatedAt); err != nil {
			continue
		}
		item.Context = contextStr.String
		if tagsJSON != "" && tagsJSON != "null" {
			json.Unmarshal([]byte(tagsJSON), &item.Tags)
		}
		item.Score = idScore[item.ID]
		items = append(items, item)
	}

	// 按分数降序
	sortKnowledgeByScore(items)
	return items, nil
}

func sortKnowledgeByScore(items []KnowledgeItem) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Score > items[i].Score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

// ListKnowledge 列出知识（支持按 project/category 过滤，分页）
func ListKnowledge(project, category string, limit, offset int) ([]KnowledgeItem, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}

	var conditions []string
	var args []interface{}
	conditions = append(conditions, "k.status = 'active'")
	if project != "" && project != "all" {
		conditions = append(conditions, "k.project = ?")
		args = append(args, project)
	}
	if category != "" && category != "all" {
		conditions = append(conditions, "k.category = ?")
		args = append(args, category)
	}
	where := strings.Join(conditions, " AND ")

	// 总数
	var total int
	conn.QueryRow("SELECT COUNT(*) FROM sys_knowledge k WHERE "+where, args...).Scan(&total)

	// 列表（JOIN sys_user 获取用户名）
	querySQL := fmt.Sprintf(
		"SELECT k.id, k.user_id, COALESCE(u.username, '') as username, k.project, k.category, k.title, k.summary, k.content, k.context, k.tags, k.source, k.priority, k.status, k.created_at FROM sys_knowledge k LEFT JOIN sys_user u ON k.user_id = u.id WHERE %s ORDER BY k.id DESC LIMIT %d OFFSET %d",
		where, limit, offset,
	)
	rows, err := conn.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []KnowledgeItem
	for rows.Next() {
		var item KnowledgeItem
		var tagsJSON string
		var contextStr sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Project, &item.Category, &item.Title, &item.Summary, &item.Content, &contextStr, &tagsJSON, &item.Source, &item.Priority, &item.Status, &item.CreatedAt); err != nil {
			log.Printf("[knowledge] Scan 失败: %v", err)
			continue
		}
		item.Context = contextStr.String
		if tagsJSON != "" && tagsJSON != "null" {
			json.Unmarshal([]byte(tagsJSON), &item.Tags)
		}
		items = append(items, item)
	}
	return items, total, nil
}

// DeleteKnowledge 删除知识（MySQL + Qdrant 同步）
func DeleteKnowledge(id int64) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 查 qdrant_id
	var qdrantID string
	conn.QueryRow("SELECT qdrant_id FROM sys_knowledge WHERE id = ?", id).Scan(&qdrantID)

	// 软删除（归档）
	_, err = conn.Exec("UPDATE sys_knowledge SET status = 'archived' WHERE id = ?", id)
	if err != nil {
		return err
	}

	// 删 Qdrant 向量
	if qdrantID != "" {
		if err := QdrantDelete(qdrantID); err != nil {
			log.Printf("[knowledge] 删除 Qdrant 向量失败: %v", err)
		}
	}
	return nil
}

// ── 上传文档专用：存储到 sys_knowledge + Qdrant ──

// StoreUploadKnowledge 上传文档分块入库（覆盖上传：先删旧，再插新）
// 参数：
//   - userID: 上传者
//   - filename: 文件名
//   - chunks: 分块后的文本数组
// 返回：实际存入的块数、错误
func StoreUploadKnowledge(userID int, filename string, chunks []string) (int, error) {
	if len(chunks) == 0 {
		return 0, nil
	}

	conn, err := GetDB()
	if err != nil {
		return 0, err
	}

	source := "upload:" + filename

	// 1. 删除旧版本（MySQL + Qdrant 同步）
	rows, err := conn.Query("SELECT id, qdrant_id FROM sys_knowledge WHERE source = ? AND status = 'active'", source)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var oldID int64
			var qdrantID string
			if rows.Scan(&oldID, &qdrantID) == nil {
				if qdrantID != "" {
					QdrantDelete(qdrantID)
				}
			}
		}
	}
	conn.Exec("UPDATE sys_knowledge SET status = 'archived' WHERE source = ? AND status = 'active'", source)

	// 2. 内容级去重（SHA-256，跨用户）
	var newChunks []string
	for _, c := range chunks {
		hash := sha256Hash(c)
		var exists int
		if err := conn.QueryRow("SELECT 1 FROM sys_knowledge WHERE content_hash = ? AND status = 'active'", hash).Scan(&exists); err != nil && err != sql.ErrNoRows {
			log.Printf("[knowledge] 去重查询失败: %v", err)
		}
		if exists == 0 {
			newChunks = append(newChunks, c)
		}
	}

	if len(newChunks) == 0 {
		return 0, nil
	}

	// 3. 逐块入库
	stored := 0
	for i, chunk := range newChunks {
		hash := sha256Hash(chunk)
		title := filename
		if len(newChunks) > 1 {
			title = fmt.Sprintf("%s (第%d块)", filename, i+1)
		}
		summary := chunk
		if len(summary) > 200 {
			summary = summary[:200]
		}

		// 3a. 写 MySQL
		res, err := conn.Exec(`INSERT INTO sys_knowledge
			(user_id, project, category, title, summary, content, context, tags, source, priority, status, content_hash)
			VALUES (?, 'general', 'document', ?, ?, ?, '', '[]', ?, 'medium', 'active', ?)`,
			userID, title, summary, chunk, source, hash)
		if err != nil {
			log.Printf("[knowledge] 上传入库失败: %v", err)
			continue
		}
		knowledgeID, _ := res.LastInsertId()

		// 3b. 向量化 + 存 Qdrant
		vector, err := GetEmbedding(chunk)
		if err != nil {
			log.Printf("[knowledge] 向量化失败（知识已入库但无向量）: %v", err)
			stored++
			continue
		}

		pointID := GeneratePointID(chunk)
		if err := QdrantUpsert(pointID, vector, knowledgeID, "general", "document", "active"); err != nil {
			log.Printf("[knowledge] Qdrant 存入失败: %v", err)
			stored++
			continue
		}

		// 3c. 回填 qdrant_id
		conn.Exec("UPDATE sys_knowledge SET qdrant_id = ? WHERE id = ?", pointID, knowledgeID)
		stored++
	}

	return stored, nil
}

// GetUploadedFilesFromKnowledge 获取用户上传的文件列表（从 sys_knowledge 按 source 聚合）
func GetUploadedFilesFromKnowledge(userID int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(
		`SELECT source, COUNT(*) as chunks, MAX(created_at) as uploaded_at
		 FROM sys_knowledge WHERE user_id = ? AND source LIKE 'upload:%' AND status = 'active'
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
		filename := strings.TrimPrefix(source, "upload:")
		files = append(files, map[string]interface{}{
			"filename":    filename,
			"chunks":      chunks,
			"uploaded_at": uploadedAt,
		})
	}
	return files, nil
}

// DeleteUploadKnowledgeBySource 按来源删除上传文档（MySQL + Qdrant 同步）
func DeleteUploadKnowledgeBySource(userID int, source string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}

	// 查所有 qdrant_id 并删除 Qdrant 向量
	rows, err := conn.Query("SELECT qdrant_id FROM sys_knowledge WHERE user_id = ? AND source = ? AND status = 'active'", userID, source)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var qdrantID string
			if rows.Scan(&qdrantID) == nil && qdrantID != "" {
				QdrantDelete(qdrantID)
			}
		}
	}

	// 软删除 MySQL
	result, err := conn.Exec("UPDATE sys_knowledge SET status = 'archived' WHERE user_id = ? AND source = ? AND status = 'active'", userID, source)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// GetKnowledgeStats 知识库统计
func GetKnowledgeStats() (map[string]int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	stats := map[string]int{}
	var total int
	conn.QueryRow("SELECT COUNT(*) FROM sys_knowledge WHERE status = 'active'").Scan(&total)
	stats["total"] = total

	rows, err := conn.Query("SELECT project, COUNT(*) FROM sys_knowledge WHERE status = 'active' GROUP BY project")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p string
			var c int
			rows.Scan(&p, &c)
			stats["project_"+p] = c
		}
	}
	return stats, nil
}

// MigrateEmbeddingsToKnowledge 一次性迁移：把旧 sys_embedding 数据迁移到 sys_knowledge + Qdrant
// 返回：成功数、跳过数、失败数
func MigrateEmbeddingsToKnowledge() (migrated, skipped, failed int, err error) {
	conn, err := GetDB()
	if err != nil {
		return 0, 0, 0, err
	}

	// 读取全部旧数据
	rows, err := conn.Query("SELECT id, user_id, content, content_hash, vector, source, created_at FROM sys_embedding ORDER BY id")
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	type oldRecord struct {
		ID        int64
		UserID    int
		Content   string
		Hash      string
		Vector    string
		Source    string
		CreatedAt string
	}
	var records []oldRecord
	for rows.Next() {
		var r oldRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.Content, &r.Hash, &r.Vector, &r.Source, &r.CreatedAt); err != nil {
			failed++
			continue
		}
		records = append(records, r)
	}

	log.Printf("[migrate] 开始迁移 %d 条旧数据", len(records))

	for _, r := range records {
		// 跳过空内容
		if len(strings.TrimSpace(r.Content)) < 20 {
			skipped++
			continue
		}

		// 解析 source 提取元数据
		// distill:date:dimension:title  或  upload:filename
		project := "general"
		category := "other"
		title := ""
		context := ""
		source := r.Source

		if strings.HasPrefix(r.Source, "distill:") {
			parts := strings.SplitN(r.Source, ":", 4)
			if len(parts) >= 3 {
				dim := strings.ToLower(parts[2])
				category = dimensionToCategory[dim]
				if category == "" {
					category = "other"
				}
			}
			if len(parts) >= 4 {
				title = parts[3]
			}
		} else if strings.HasPrefix(r.Source, "upload:") {
			title = strings.TrimPrefix(r.Source, "upload:")
			category = "other"
		}

		// 新的 SHA-256 hash（跨用户去重）
		newHash := sha256Hash(r.Content)
		var exists int
		conn.QueryRow("SELECT 1 FROM sys_knowledge WHERE content_hash = ? AND status = 'active'", newHash).Scan(&exists)
		if exists == 1 {
			skipped++
			continue
		}

		// summary
		summary := title
		if summary == "" {
			if len(r.Content) > 100 {
				summary = r.Content[:100]
			} else {
				summary = r.Content
			}
		}

		// 写入 sys_knowledge
		res, err := conn.Exec(`INSERT INTO sys_knowledge
			(user_id, project, category, title, summary, content, context, tags, source, priority, status, content_hash, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '[]', ?, 'medium', 'active', ?, ?)`,
			r.UserID, project, category, title, summary, r.Content, context, source, newHash, r.CreatedAt)
		if err != nil {
			log.Printf("[migrate] 写入失败 id=%d: %v", r.ID, err)
			failed++
			continue
		}
		knowledgeID, _ := res.LastInsertId()

		// 解析旧向量 JSON
		var vector []float64
		if err := json.Unmarshal([]byte(r.Vector), &vector); err != nil {
			log.Printf("[migrate] 向量解析失败 id=%d: %v", r.ID, err)
			migrated++ // 元数据已入库，向量缺失
			continue
		}

		// 存入 Qdrant
		pointID := GeneratePointID(r.Content)
		if err := QdrantUpsert(pointID, vector, knowledgeID, project, category, "active"); err != nil {
			log.Printf("[migrate] Qdrant 存入失败 id=%d: %v", r.ID, err)
			migrated++ // 元数据已入库
			continue
		}

		// 回填 qdrant_id
		conn.Exec("UPDATE sys_knowledge SET qdrant_id = ? WHERE id = ?", pointID, knowledgeID)
		migrated++
	}

	log.Printf("[migrate] 迁移完成：成功=%d, 跳过=%d, 失败=%d", migrated, skipped, failed)
	return migrated, skipped, failed, nil
}
