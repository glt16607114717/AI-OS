package service

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// ── AI 建议查询 ──

// EnsureSuggestionTable 确保建议表存在
func EnsureSuggestionTable() {
	conn, _ := GetDB()
	if conn == nil {
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

// GetSuggestions 获取用户的建议列表（API 调用）
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

// MarkSuggestionProcessed 标记建议已处理
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
