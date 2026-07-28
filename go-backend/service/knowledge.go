package service

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
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
	Score       float64  `json:"score,omitempty"`        // 检索排序分数（多向量时为 RRF 融合分数）
	VectorScore float64  `json:"vector_score,omitempty"` // 最高向量相似度（用于阈值过滤和展示）
	CreatedAt string   `json:"created_at"`
}

// 维度 → category 映射
// 蒸馏 prompt 产出的 dimension: 技术规范/架构决策/开发流程/Bug修复/工具技巧/环境配置
// 修复 B2：之前 map key 是英文（decisions/pitfalls/business/habits），与 prompt 输出的中文不匹配，导致全部兜底 other
var dimensionToCategory = map[string]string{
	"技术规范": "norm",
	"架构决策": "decision",
	"开发流程": "workflow",
	"Bug修复": "bugfix",
	"工具技巧": "tip",
	"环境配置": "env",
	// 兼容旧英文输出（迁移期防御）
	"decisions": "decision",
	"pitfalls":  "pitfall",
	"business":  "business",
	"habits":    "habit",
}

// normalizeDimension 归一化 dimension：去空格、去冒号
func normalizeDimension(d string) string {
	d = strings.TrimSpace(d)
	d = strings.Trim(d, "：:")
	return d
}

// normalizeProject 归一化 project（修复 B3）
// 规则：小写化 + 同义词合并 + 兜底 general
// 合法输出只有：ai-os / rmp / general
func normalizeProject(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ToLower(p)

	// 同义词归并到 ai-os
	aiOsAliases := map[string]bool{
		"ai-os":   true,
		"ai_os":   true,
		"aios":    true,
		"ai-os项目": true,
		"ai-os 后端": true,
	}
	if aiOsAliases[p] {
		return "ai-os"
	}

	// 同义词归并到 rmp（含 rmp-api/rmp-prd/nnd-robot/nnd-flow-api/chartsapi/socket 等所有 RMP 系）
	rmpAliases := map[string]bool{
		"rmp":          true,
		"rmp-api":      true,
		"rmp_api":      true,
		"rmp-prd":      true,
		"rmp-prd文档":    true,
		"nnd-robot":    true,
		"nnd_robot":    true,
		"nnd-flow-api": true,
		"nnd_flow_api": true,
		"chartsapi":    true,
		"charts-api":   true,
		"socket":       true,
		"rmp-api项目":    true,
		"rmp系统":        true,
	}
	if rmpAliases[p] {
		return "rmp"
	}

	// 其他所有值（含空串、"未知"、"德塔"、各种子项目名）统一归 general
	return "general"
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
//   - sessionID: 来源对话 session，不同 session 的同标题知识视为不同条目
// 返回：是否新存入（false=重复跳过）
func StoreDistillKnowledge(userID int, date string, k DistillKnowledge, sessionID string) (bool, error) {
	if k.Content == "" || k.Dimension == "" {
		return false, nil
	}

	conn, err := GetDB()
	if err != nil {
		return false, err
	}

	// 归一化 category（修复 B2：用 normalizeDimension 处理空格/冒号，再用 map 映射）
	dimKey := normalizeDimension(k.Dimension)
	category := dimensionToCategory[dimKey]
	if category == "" {
		// 二次尝试：原 ToLower 兜底（保留旧逻辑防御）
		category = dimensionToCategory[strings.ToLower(dimKey)]
	}
	if category == "" {
		category = "other"
	}

	// 归一化 priority
	// 兼容 LLM 输出中文"高/中/低"的情况（蒸馏 prompt 已改为英文枚举，此处兜底防御旧模型输出）
	priority := k.Priority
	priorityCNMap := map[string]string{
		"高": "high", "中": "medium", "低": "low",
	}
	if mapped, ok := priorityCNMap[strings.TrimSpace(priority)]; ok {
		priority = mapped
	}
	if priority != "high" && priority != "medium" && priority != "low" {
		priority = "medium"
	}

	// 归一化 project（修复 B3：大小写不敏感 + 同义词合并 + 兜底 general）
	project := normalizeProject(k.Project)

	// 内容去重（SHA-256 + session_id：不同 session 的同标题知识不视为重复）
	hash := sha256Hash(k.Content)
	var exists int
	if err := conn.QueryRow("SELECT 1 FROM sys_knowledge WHERE content_hash = ? AND session_id = ? AND status = 'active'", hash, sessionID).Scan(&exists); err != nil && err != sql.ErrNoRows {
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
		(user_id, project, category, title, summary, content, context, tags, source, priority, status, content_hash, session_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`,
		userID, project, category, k.Title, summary, k.Content, k.Context, tagsJSON, source, priority, hash, sessionID)
	if err != nil {
		return false, fmt.Errorf("写入 sys_knowledge 失败: %v", err)
	}
	knowledgeID, _ := res.LastInsertId()

	// 2. 向量化 + 存 Qdrant
	// 修复 B5：向量化失败时回滚 MySQL（软删除 archived），避免产生孤儿数据（active 但无 qdrant_id）
	vector, err := GetEmbedding(k.Content)
	if err != nil {
		log.Printf("[knowledge] 向量化失败，回滚 MySQL 软删除（id=%d）: %v", knowledgeID, err)
		rollbackKnowledgeInsert(conn, knowledgeID, "embedding_failed")
		return false, nil
	}

	pointID := GeneratePointID(k.Content)
	if err := QdrantUpsert(pointID, vector, knowledgeID, project, category, "active"); err != nil {
		log.Printf("[knowledge] Qdrant 存入失败，回滚 MySQL 软删除（id=%d）: %v", knowledgeID, err)
		rollbackKnowledgeInsert(conn, knowledgeID, "qdrant_failed")
		return false, nil
	}

	// 3. 回填 qdrant_id
	conn.Exec("UPDATE sys_knowledge SET qdrant_id = ? WHERE id = ?", pointID, knowledgeID)

	// 4. 采集入库日志（供每日巡检评审，source 带 distill: 前缀）
	LogStoreAudit(userID, knowledgeID, project, category, k.Title, priority, k.Content, source)

	return true, nil
}

// rollbackKnowledgeInsert 向量化失败时回滚刚插入的 MySQL 行（软删除 archived，留痕）
// 不做物理删除是为了保留排查线索，archived 状态不会被检索到
func rollbackKnowledgeInsert(conn *sql.DB, knowledgeID int64, reason string) {
	if _, err := conn.Exec(
		"UPDATE sys_knowledge SET status = 'archived' WHERE id = ? AND status = 'active'",
		knowledgeID,
	); err != nil {
		log.Printf("[knowledge] 回滚失败（id=%d reason=%s）: %v", knowledgeID, reason, err)
	} else {
		log.Printf("[knowledge] 已回滚 id=%d reason=%s", knowledgeID, reason)
	}
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

		// 3b. 向量化 + 存 Qdrant（修复 B5：失败时回滚 MySQL，避免孤儿数据）
		vector, err := GetEmbedding(chunk)
		if err != nil {
			log.Printf("[knowledge] 上传向量化失败，回滚 MySQL（id=%d）: %v", knowledgeID, err)
			rollbackKnowledgeInsert(conn, knowledgeID, "upload_embedding_failed")
			continue
		}

		pointID := GeneratePointID(chunk)
		if err := QdrantUpsert(pointID, vector, knowledgeID, "general", "document", "active"); err != nil {
			log.Printf("[knowledge] 上传 Qdrant 存入失败，回滚 MySQL（id=%d）: %v", knowledgeID, err)
			rollbackKnowledgeInsert(conn, knowledgeID, "upload_qdrant_failed")
			continue
		}

		// 3c. 回填 qdrant_id
		conn.Exec("UPDATE sys_knowledge SET qdrant_id = ? WHERE id = ?", pointID, knowledgeID)

		// 3d. 采集入库日志（供每日巡检评审）
		LogStoreAudit(userID, knowledgeID, "general", "document", title, "medium", chunk, source)
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

// ── 多向量检索（Multi-Vector Retrieval）──
//
// 背景：纯单向量检索（整段 query → 1 个向量 → Qdrant 检索）对两种场景效果差：
//   1. 超长 prompt（含源码路径、Agent 指令模板）→ 主意图被噪声淹没
//   2. 极短 prompt（如"继续"）→ 信息不足，召回碎片化
//
// 方案：将 query 拆分成多个语义片段，每片独立向量化检索，用 RRF 融合结果。
//   - 片段提取是纯字符串处理，零 LLM 调用，纳秒级完成
//   - 多片段共享一次 GetEmbeddings 批量调用，只多 1 次 HTTP RTT
//   - RRF 融合保证被多片段共同召回的知识排到前面（语义双重确认）

// ExtractQueryFragments 从一段（已做过 IDE 噪声清洗的）query 中提取多个语义片段
// 返回去重后的片段列表（已去除空串），用于批量 embedding + 多路检索
//
// 策略：
//   - 提取 <query> 标签内容（Agent 工具/技能调用时的标准格式）
//   - 提取 <task> 标签内容（Agent 任务描述）
//   - 提取「疑问句」——包含问号、或「帮我/怎么/如何/排查/实现/修复」等意图关键词的句子
//   - 提取「代码标识符」——通过路径引用 (file.go:123) 或反引号 `xxx` 的技术实体名
//   - 如果提取出的片段列表为空，回退用原始 query 兜底（保证不退化）
func ExtractQueryFragments(query string) []string {
	if query == "" {
		return nil
	}

	var fragments []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len(s) < 3 {
			return
		}
		// 截断超长片段（向量模型输入也有限制，512 token ≈ 1500 字符）
		if len(s) > 500 {
			s = s[:500]
		}
		if !seen[s] {
			seen[s] = true
			fragments = append(fragments, s)
		}
	}

	// 1. <query> / <task> 标签内容
	for _, tag := range []string{"query", "task"} {
		re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
		for _, m := range re.FindAllStringSubmatch(query, -1) {
			add(m[1])
		}
	}

	// 2. 按句子拆分，提取意图明确的句子
	intentRe := regexp.MustCompile(`(帮我|怎么|如何|为什么|为何|排查|排查一下|实现|修复|解决|调研|审查|检查|分析|报错|错误|异常|失败|不支持|优化|重构|添加|新增|删除|修改|为什么不能|为什么没法)`)
	sentences := splitSentences(query)
	for _, s := range sentences {
		if intentRe.MatchString(s) {
			add(s)
		}
	}

	// 3. 反引号包裹的技术标识符 `xxx`
	backtickRe := regexp.MustCompile("`([^`]{3,80})`")
	for _, m := range backtickRe.FindAllStringSubmatch(query, -1) {
		add(m[1])
	}

	// 4. 文件路径引用（如 xxx/yyy.go:123）
	pathRe := regexp.MustCompile(`[\w\-./]+\.\w+(:\d+)?`)
	for _, m := range pathRe.FindAllString(query, -1) {
		add(m)
	}

	// 5. 如果一条都没提取出来，回退用原始 query（保证不退化）
	if len(fragments) == 0 {
		add(query)
	}

	return fragments
}

// splitSentences 将文本拆分成句子（中英文标点、换行符）
func splitSentences(text string) []string {
	// 按中英文句号、问号、换行拆分
	re := regexp.MustCompile(`[。？?！!\n]+`)
	parts := re.Split(text, -1)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) >= 5 { // 过滤过短的碎片
			result = append(result, p)
		}
	}
	return result
}

// SearchKnowledgeMulti 多向量检索：将 query 拆分为多个语义片段，分路检索后用 RRF 融合
//
// 参数同 SearchKnowledge。融合策略：
//   - 每个片段各检索 topK 条
//   - RRF（Reciprocal Rank Fusion）：score = Σ 1/(k+rank)，默认 k=60
//   - 被多个片段同时召回的知识会因多次累加而排到前面（语义双重确认提权）
//   - VectorScore 保留每条知识的最高向量相似度，用于阈值过滤（兼容现有 0.5 阈值）
func SearchKnowledgeMulti(query string, topK int, projects []string, categories []string) ([]KnowledgeItem, error) {
	if topK <= 0 {
		topK = 10
	}

	// 1. 提取语义片段
	fragments := ExtractQueryFragments(query)
	if len(fragments) == 0 {
		// 兜底：回退到单向量检索
		return SearchKnowledge(query, topK, projects, categories)
	}

	// 2. 批量获取所有片段的向量（一次 API 调用）
	vectors, err := GetEmbeddings(fragments)
	if err != nil {
		log.Printf("[knowledge-multi] 批量 embedding 失败，降级单向量: %v", err)
		return SearchKnowledge(query, topK, projects, categories)
	}

	// 3. 每个片段独立检索 topK，收集结果
	// RRF: 每条知识的 rank 从 1 开始
	type hitAccum struct {
		rrfScore    float64
		maxVecScore float64
		hits        []QdrantHit
	}
	accum := make(map[int64]*hitAccum)

	for _, vec := range vectors {
		hits, err := QdrantSearch(vec, topK, projects, categories)
		if err != nil {
			log.Printf("[knowledge-multi] Qdrant 检索失败（片段已跳过）: %v", err)
			continue
		}
		for rank, h := range hits {
			acc, exists := accum[h.KnowledgeID]
			if !exists {
				acc = &hitAccum{}
				accum[h.KnowledgeID] = acc
			}
			// RRF 融合：k=60
			acc.rrfScore += 1.0 / float64(60+rank+1)
			if h.Score > acc.maxVecScore {
				acc.maxVecScore = h.Score
			}
			acc.hits = append(acc.hits, h)
		}
	}

	if len(accum) == 0 {
		return nil, nil
	}

	// 4. 按 RRF 分数排序，取 topK
	type scored struct {
		id     int64
		rrf    float64
		vecMax float64
	}
	scoredList := make([]scored, 0, len(accum))
	for id, acc := range accum {
		scoredList = append(scoredList, scored{id: id, rrf: acc.rrfScore, vecMax: acc.maxVecScore})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].rrf > scoredList[j].rrf
	})
	if len(scoredList) > topK {
		scoredList = scoredList[:topK]
	}

	// 5. 回 MySQL 取完整内容
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	ids := make([]interface{}, len(scoredList))
	rrfMap := make(map[int64]float64, len(scoredList))
	vecMap := make(map[int64]float64, len(scoredList))
	for i, s := range scoredList {
		ids[i] = s.id
		rrfMap[s.id] = s.rrf
		vecMap[s.id] = s.vecMax
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
		item.Score = rrfMap[item.ID]
		item.VectorScore = vecMap[item.ID]
		items = append(items, item)
	}

	// 按 RRF 分数降序
	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	log.Printf("[knowledge-multi] query=%q fragments=%d candidates=%d returned=%d",
		truncateForLog(query, 60), len(fragments), len(accum), len(items))

	return items, nil
}

func truncateForLog(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
