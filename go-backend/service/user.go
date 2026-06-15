package service

import (
	"ai-os-server/middleware"
	"database/sql"
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
		status TINYINT UNSIGNED NOT NULL DEFAULT 1,
		is_admin TINYINT UNSIGNED NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		log.Printf("[user] 建表失败: %v", err)
	}
}

func LoginUser(username, password string) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var id, status, isAdmin int
	var uname, hashedPassword string
	err = conn.QueryRow("SELECT id, username, password, status, is_admin FROM sys_user WHERE username = ?",
		username).Scan(&id, &uname, &hashedPassword, &status, &isAdmin)
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
	expire := time.Now().Add(30 * 24 * time.Hour).Format("2006-01-02T15:04:05Z07:00")

	return map[string]interface{}{
		"user_id": id, "username": uname, "is_admin": isAdmin == 1,
		"token": token, "expire": expire,
	}, nil
}

func ListUsers() ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, username, status, is_admin, created_at, updated_at FROM sys_user ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, status, isAdmin int
		var username, createdAt, updatedAt string
		if err := rows.Scan(&id, &username, &status, &isAdmin, &createdAt, &updatedAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "username": username, "status": status,
			"is_admin": isAdmin == 1, "created_at": createdAt, "updated_at": updatedAt,
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
	ia := 0
	if isAdmin {
		ia = 1
	}
	res, err := conn.Exec("INSERT INTO sys_user (username, password, is_admin) VALUES (?, ?, ?)", username, hashed, ia)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			return nil, fmt.Errorf("用户名已存在")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return map[string]interface{}{"id": id, "username": username, "is_admin": isAdmin}, nil
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
		report_date DATE NOT NULL,
		category VARCHAR(50) NOT NULL,
		title VARCHAR(500) NOT NULL,
		content TEXT NOT NULL,
		priority VARCHAR(20) DEFAULT 'medium',
		status VARCHAR(20) DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		processed_at DATETIME NULL,
		INDEX idx_date (report_date),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func GetSuggestions(status, date string) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT id, report_date, category, title, content, priority, status, created_at, processed_at FROM sys_ai_suggestion"
	var conditions []string
	var args []interface{}
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
		var id int
		var reportDate, category, title, content, priority, status2, createdAt string
		var processedAt sql.NullString
		if err := rows.Scan(&id, &reportDate, &category, &title, &content, &priority, &status2, &createdAt, &processedAt); err != nil {
			log.Printf("[suggestion] scan 失败: %v", err)
			continue
		}
		pa := ""
		if processedAt.Valid {
			pa = processedAt.String
		}
		result = append(result, map[string]interface{}{
			"id": id, "report_date": reportDate, "category": category,
			"title": title, "content": content, "priority": priority,
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

	// 获取今天 LLM 日志作为分析材料
	cutoff := time.Now().Format("2006-01-02") + " 00:00:00"
	rows, err := conn.Query("SELECT message, detail FROM sys_llm_log WHERE ts >= ? ORDER BY id DESC LIMIT 50", cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()

	var docs []string
	for rows.Next() {
		var msg, det string
		if err := rows.Scan(&msg, &det); err != nil {
			continue
		}
		docs = append(docs, msg+" "+det)
	}
	if len(docs) == 0 {
		return fmt.Errorf("今日无日志数据，跳过分析")
	}

	docsContent := strings.Join(docs, "\n---\n")
	if len(docsContent) > 12000 {
		docsContent = docsContent[:12000] + "\n...(内容已截断)"
	}

	prompt := fmt.Sprintf(`你是一个 AI 使用习惯分析专家。请根据以下今天的开发/使用记录，给出优化建议。

要求：
1. 分析维度包括：技能封装建议、规则加强建议、提示词优化建议、工作流优化建议、其他建议
2. 每条建议用 JSON 格式输出，包含：category（skill/rule/prompt/workflow/other）、title、content、priority（high/medium/low）
3. 输出一个 JSON 数组，不要输出其他内容
4. 建议要具体、可执行，不要泛泛而谈

今天的开发/使用记录：
%s`, docsContent)

	// 调用智谱 API
	body := map[string]interface{}{
		"model": "glm-5.1",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens": 4000,
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://open.bigmodel.cn/api/paas/v4/chat/completions", strings.NewReader(string(bodyJSON)))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return err
	}

	content := ""
	if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				content, _ = msg["content"].(string)
			}
		}
	}
	if content == "" {
		return fmt.Errorf("AI 未返回有效内容")
	}

	// 解析建议
	var suggestions []map[string]interface{}
	// 尝试直接解析
	json.Unmarshal([]byte(content), &suggestions)
	if len(suggestions) == 0 {
		// 尝试从代码块提取
		start := strings.Index(content, "[")
		end := strings.LastIndex(content, "]")
		if start >= 0 && end > start {
			json.Unmarshal([]byte(content[start:end+1]), &suggestions)
		}
	}

	today := time.Now().Format("2006-01-02")
	for _, s := range suggestions {
		cat, _ := s["category"].(string)
		if cat == "" {
			cat = "other"
		}
		title, _ := s["title"].(string)
		if title == "" {
			title = "未命名建议"
		}
		cont, _ := s["content"].(string)
		pri, _ := s["priority"].(string)
		if pri == "" {
			pri = "medium"
		}
		conn.Exec("INSERT INTO sys_ai_suggestion (report_date, category, title, content, priority) VALUES (?, ?, ?, ?, ?)",
			today, cat, title, cont, pri)
	}

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
