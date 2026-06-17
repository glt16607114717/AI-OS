package service

import (
	"ai-os-server/middleware"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ── 用户管理 ──

func EnsureUserTable() {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[user] DB连接失败: %v", err)
		return
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS sys_user (
		id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(64) NOT NULL UNIQUE,
		password VARCHAR(128) NOT NULL,
		api_key VARCHAR(64) NOT NULL DEFAULT '',
		status TINYINT UNSIGNED NOT NULL DEFAULT 1,
		is_admin TINYINT UNSIGNED NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		log.Printf("[user] 建表失败: %v", err)
	}
	// 存量用户补 api_key
	rows, _ := conn.Query("SELECT id FROM sys_user WHERE api_key = ''")
	if rows != nil {
		var ids []int
		for rows.Next() {
			var id int
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			key := generateAPIKey()
			conn.Exec("UPDATE sys_user SET api_key = ? WHERE id = ?", key, id)
		}
		if len(ids) > 0 {
			log.Printf("[user] 已为 %d 个存量用户生成 api_key", len(ids))
		}
	}
}

// generateAPIKey 生成 32 字符的随机 API Key
func generateAPIKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "sk-" + hex.EncodeToString(b)
}

func LoginUser(username, password string) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var id, status, isAdmin int
	var uname, hashedPassword, apiKey string
	err = conn.QueryRow("SELECT id, username, password, status, is_admin, api_key FROM sys_user WHERE username = ?",
		username).Scan(&id, &uname, &hashedPassword, &status, &isAdmin, &apiKey)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if err != nil {
		return nil, err
	}
	if !middleware.CheckPassword(password, hashedPassword) {
		return nil, fmt.Errorf("用户名或密码错误")
	}
	if status != 1 {
		return nil, fmt.Errorf("账号已停用")
	}

	token := middleware.CreateSession(id, uname, isAdmin == 1)
	expire := time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	return map[string]interface{}{
		"user_id": id, "username": uname, "is_admin": isAdmin == 1,
		"token": token, "expire": expire, "api_key": apiKey,
	}, nil
}

func ListUsers() ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, username, status, is_admin, api_key, created_at, updated_at FROM sys_user ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, status, isAdmin int
		var username, apiKey, createdAt, updatedAt string
		if err := rows.Scan(&id, &username, &status, &isAdmin, &apiKey, &createdAt, &updatedAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "username": username, "status": status,
			"is_admin": isAdmin == 1, "api_key": apiKey,
			"created_at": createdAt, "updated_at": updatedAt,
		})
	}
	return result, nil
}

func CreateUser(username, password string, isAdmin bool) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	hashed := middleware.HashPassword(password)
	apiKey := generateAPIKey()
	ia := 0
	if isAdmin {
		ia = 1
	}
	res, err := conn.Exec("INSERT INTO sys_user (username, password, api_key, is_admin) VALUES (?, ?, ?, ?)", username, hashed, apiKey, ia)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return nil, fmt.Errorf("用户名已存在")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return map[string]interface{}{"id": id, "username": username, "is_admin": isAdmin, "api_key": apiKey}, nil
}

// GetUserByAPIKey 根据 API Key 查询用户
func GetUserByAPIKey(apiKey string) (int, string, bool, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, "", false, err
	}
	var id, isAdmin int
	var username string
	err = conn.QueryRow("SELECT id, username, is_admin FROM sys_user WHERE api_key = ? AND status = 1", apiKey).
		Scan(&id, &username, &isAdmin)
	if err == sql.ErrNoRows {
		return 0, "", false, fmt.Errorf("invalid api key")
	}
	if err != nil {
		return 0, "", false, err
	}
	return id, username, isAdmin == 1, nil
}

// GetUserAPIKey 获取指定用户的 API Key
func GetUserAPIKey(userID int) (string, error) {
	conn, err := GetDB()
	if err != nil {
		return "", err
	}
	var apiKey string
	err = conn.QueryRow("SELECT api_key FROM sys_user WHERE id = ?", userID).Scan(&apiKey)
	if err != nil {
		return "", err
	}
	return apiKey, nil
}

func UpdateUserPassword(userID int, password string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	hashed := middleware.HashPassword(password)
	_, err = conn.Exec("UPDATE sys_user SET password = ? WHERE id = ?", hashed, userID)
	return err
}

func ToggleUserStatus(userID, status int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("UPDATE sys_user SET status = ? WHERE id = ?", status, userID)
	return err
}

func ToggleUserAdmin(userID int, isAdmin bool) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	ia := 0
	if isAdmin {
		ia = 1
	}
	_, err = conn.Exec("UPDATE sys_user SET is_admin = ? WHERE id = ?", ia, userID)
	return err
}

func DeleteUser(userID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("DELETE FROM sys_user WHERE id = ?", userID)
	return err
}

// ── AI 建议 ──

func EnsureSuggestionTable() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_ai_suggestion (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL DEFAULT 0,
		report_date DATE NOT NULL,
		category VARCHAR(50) NOT NULL,
		project VARCHAR(200) DEFAULT '',
		title VARCHAR(500) NOT NULL,
		content TEXT NOT NULL,
		priority VARCHAR(20) DEFAULT 'medium',
		status VARCHAR(20) DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		processed_at DATETIME NULL,
		INDEX idx_user_id (user_id),
		INDEX idx_date (report_date),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func GetSuggestions(userID int, status, date string) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT id, user_id, report_date, category, project, title, content, priority, status, created_at, processed_at FROM sys_ai_suggestion"
	var conditions []string
	var args []interface{}
	conditions = append(conditions, "user_id = ?")
	args = append(args, userID)
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if date != "" {
		conditions = append(conditions, "report_date = ?")
		args = append(args, date)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id DESC"

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, uid int
		var reportDate, category, project, title, content, priority, status2, createdAt string
		var processedAt sql.NullString
		if err := rows.Scan(&id, &uid, &reportDate, &category, &project, &title, &content, &priority, &status2, &createdAt, &processedAt); err != nil {
			log.Printf("[suggestion] scan 失败: %v", err)
			continue
		}
		pa := ""
		if processedAt.Valid {
			pa = processedAt.String
		}
		result = append(result, map[string]interface{}{
			"id": id, "user_id": uid, "report_date": reportDate, "category": category,
			"project": project, "title": title, "content": content, "priority": priority,
			"status": status2, "created_at": createdAt, "processed_at": pa,
		})
	}
	return result, nil
}

func MarkSuggestionProcessed(id int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	var exists int
	if err := conn.QueryRow("SELECT 1 FROM sys_ai_suggestion WHERE id = ?", id).Scan(&exists); err != nil {
		return fmt.Errorf("建议不存在")
	}
	_, err = conn.Exec("UPDATE sys_ai_suggestion SET status='processed', processed_at=NOW() WHERE id = ?", id)
	return err
}

func RunAIAnalysis() error {
	conn, err := GetDB()
	if err != nil {
		return err
	}

	// 获取智谱 API Key
	var apiKey string
	err = conn.QueryRow(`SELECT k.api_key FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''
		LIMIT 1`).Scan(&apiKey)
	if err != nil {
		return fmt.Errorf("未找到智谱 API Key")
	}

	// 获取所有有未分析知识库记录的用户
	cutoff := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
	userRows, err := conn.Query(`SELECT DISTINCT user_id FROM sys_embedding WHERE analyzed = 0 AND created_at >= ?`, cutoff)
	if err != nil {
		return err
	}
	defer userRows.Close()

	var userIDs []int
	for userRows.Next() {
		var uid int
		if err := userRows.Scan(&uid); err == nil {
			userIDs = append(userIDs, uid)
		}
	}
	if len(userIDs) == 0 {
		return fmt.Errorf("无待分析数据")
	}

	today := time.Now().Format("2006-01-02")
	totalInserted := 0

	for _, uid := range userIDs {
		// 获取该用户一周内未分析的知识库记录
		rows, err := conn.Query(`SELECT content, source FROM sys_embedding WHERE user_id = ? AND analyzed = 0 AND created_at >= ? ORDER BY created_at`, uid, cutoff)
		if err != nil {
			log.Printf("[analysis] 查询用户 %d 知识库失败: %v", uid, err)
			continue
		}

		var records []string
		for rows.Next() {
			var content, source string
			if err := rows.Scan(&content, &source); err != nil {
				continue
			}
			records = append(records, fmt.Sprintf("[来源:%s] %s", source, content))
		}
		rows.Close()

		if len(records) == 0 {
			continue
		}

		// 不截断，全部拼接
		docsContent := strings.Join(records, "\n---\n")

		prompt := fmt.Sprintf(`你是一个资深技术架构师和研发效能专家。以下是用户过去一周的所有工作对话记录。

请仔细阅读这些记录，站在架构层面进行深度分析，找出用户开发过程中的痛点、反复出现的问题、可以优化的流程。

分析维度：
1. 技能封装建议 - 哪些重复操作可以封装成技能？当前技能有什么改进空间？
2. 规则加强建议 - 代码规范、架构约束方面有什么需要加强？AI反复犯了哪些错误？
3. 提示词优化建议 - 用户如何写出更精准的提示词？有没有沟通低效的模式？
4. 工作流优化建议 - 开发流程、部署流程有什么可以改进？
5. 知识沉淀建议 - 哪些知识应该沉淀到知识库？
6. 代码质量建议 - 发现的代码坏味道、技术债
7. Bug 分析 - 本周总共涉及多少个 bug？每个 bug 产生的原因是什么？是开发自身问题、产品需求问题还是测试遗漏问题？如何从流程上减少 bug？

要求：
- 每条建议包含：category（skill/rule/prompt/workflow/knowledge/quality/bug）、project（关联的项目名称，从对话中推断）、title、content（至少200字，要具体可执行，引用原始记录作为依据）、priority（high/medium/low）
- 重点关注AI反复犯错、反复修改的地方，这些是最急需解决的
- 输出一个 JSON 数组，不要输出其他内容

用户一周工作记录：
%s`, docsContent)

		body := map[string]interface{}{
		"model": "glm-5.1",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", strings.NewReader(string(bodyJSON)))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 300 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[analysis] 用户 %d API 调用失败: %v", uid, err)
			continue
		}

		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			resp.Body.Close()
			log.Printf("[analysis] 用户 %d 响应解析失败: %v", uid, err)
			continue
		}
		resp.Body.Close()

		content := ""
		if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					content, _ = msg["content"].(string)
				}
			}
		}
		if content == "" {
			log.Printf("[analysis] 用户 %d AI 未返回有效内容", uid)
			continue
		}

		// 解析建议
		var suggestions []map[string]interface{}
		json.Unmarshal([]byte(content), &suggestions)
		if len(suggestions) == 0 {
			start := strings.Index(content, "[")
			end := strings.LastIndex(content, "]")
			if start >= 0 && end > start {
				json.Unmarshal([]byte(content[start:end+1]), &suggestions)
			}
		}

		for _, s := range suggestions {
			cat, _ := s["category"].(string)
			if cat == "" {
				cat = "other"
			}
			project, _ := s["project"].(string)
			title, _ := s["title"].(string)
			if title == "" {
				title = "未命名建议"
			}
			cont, _ := s["content"].(string)
			pri, _ := s["priority"].(string)
			if pri == "" {
				pri = "medium"
			}
			conn.Exec("INSERT INTO sys_ai_suggestion (user_id, report_date, category, project, title, content, priority) VALUES (?, ?, ?, ?, ?, ?, ?)",
				uid, today, cat, project, title, cont, pri)
			totalInserted++
		}

		// 标记该用户已分析的知识库记录
		conn.Exec("UPDATE sys_embedding SET analyzed = 1 WHERE user_id = ? AND analyzed = 0 AND created_at >= ?", uid, cutoff)

		log.Printf("[analysis] 用户 %d 分析完成，生成 %d 条建议", uid, len(suggestions))
	}

	if totalInserted == 0 {
		return fmt.Errorf("未生成有效建议")
	}
	log.Printf("[analysis] 全部完成，共生成 %d 条建议", totalInserted)
	return nil
}

// ── 技能管理 ──

func EnsureSkillTable() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_skill (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(200) NOT NULL,
		description TEXT,
		queries JSON,
		post_instruction TEXT,
		example_queries JSON,
		enabled TINYINT DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func GetSkillList() ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, name, description, example_queries FROM sys_skill WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, name, description string
		var exampleJSON string
		if err := rows.Scan(&id, &name, &description, &exampleJSON); err != nil {
			log.Printf("[skill] scan 失败: %v", err)
			continue
		}
		var examples []string
		if err := json.Unmarshal([]byte(exampleJSON), &examples); err != nil {
			log.Printf("[skill] 解析 example_queries 失败: %v", err)
		}
		result = append(result, map[string]interface{}{
			"id": id, "name": name, "description": description,
			"example_queries": examples,
		})
	}
	return result, nil
}

func GetSkillToolDefinitions() ([]map[string]interface{}, error) {
	skills, err := GetSkillList()
	if err != nil {
		return nil, err
	}
	var tools []map[string]interface{}
	for _, s := range skills {
		id, _ := s["id"].(string)
		desc, _ := s["description"].(string)
		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "skill_" + id,
				"description": desc,
				"parameters":  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			},
		})
	}
	return tools, nil
}

func ExecuteSkill(skillID string) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var name, queriesJSON, postInstr string
	err = conn.QueryRow("SELECT name, queries, post_instruction FROM sys_skill WHERE id = ?", skillID).Scan(&name, &queriesJSON, &postInstr)
	if err != nil {
		return nil, fmt.Errorf("未知技能: %s", skillID)
	}

	var queries []struct {
		Name  string `json:"name"`
		Label string `json:"label"`
		SQL   string `json:"sql"`
	}
	json.Unmarshal([]byte(queriesJSON), &queries)

	results := map[string]interface{}{}
	for _, q := range queries {
		data, rows, err := executeReadonlySQL(q.SQL)
		if err != nil {
			return nil, fmt.Errorf("查询 %s 失败: %v", q.Name, err)
		}
		results[q.Name] = map[string]interface{}{
			"label": q.Label, "data": data, "rows": rows,
		}
	}

	return map[string]interface{}{
		"ok":              true,
		"skill":           name,
		"query_results":   results,
		"post_instruction": postInstr,
	}, nil
}

func executeReadonlySQL(sqlQuery string) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}
	// 只允许 SELECT
	if !strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sqlQuery)), "SELECT") {
		return nil, 0, fmt.Errorf("只允许 SELECT 查询")
	}
	rows, err := conn.Query(sqlQuery)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		rows.Scan(ptrs...)
		row := map[string]interface{}{}
		for i, col := range cols {
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}
	return result, len(result), nil
}
