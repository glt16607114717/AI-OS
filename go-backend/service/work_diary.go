package service

import (
	"database/sql"
	"log"
	"strings"
)

// ── 工作日报 ──

// GetWorkDiaries 获取工作日报列表
// userID=0 表示管理员，获取所有；否则只获取指定用户的
func GetWorkDiaries(userID int, date string, limit int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT id, user_id, username, report_date, title, LEFT(content, 200) as content_preview, created_at, updated_at FROM sys_work_diary"
	var conditions []string
	var args []interface{}
	if userID > 0 {
		conditions = append(conditions, "user_id = ?")
		args = append(args, userID)
	}
	if date != "" {
		conditions = append(conditions, "report_date = ?")
		args = append(args, date)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY report_date DESC, id DESC LIMIT 100"

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id, uid int
		var username, reportDate, title, contentPreview, createdAt, updatedAt string
		if err := rows.Scan(&id, &uid, &username, &reportDate, &title, &contentPreview, &createdAt, &updatedAt); err != nil {
			log.Printf("[work_diary] scan 失败: %v", err)
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "user_id": uid, "username": username, "report_date": reportDate,
			"title": title, "content_preview": contentPreview, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	return result, nil
}

// GetWorkDiary 获取单条日报（含完整内容）
func GetWorkDiary(id, userID int, isAdmin bool) (*map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := "SELECT id, user_id, username, report_date, title, content, created_at, updated_at FROM sys_work_diary WHERE id = ?"
	var args []interface{} = []interface{}{id}
	if !isAdmin {
		query += " AND user_id = ?"
		args = append(args, userID)
	}

	var did, uid int
	var username, reportDate, title, content, createdAt, updatedAt string
	err = conn.QueryRow(query, args...).Scan(&did, &uid, &username, &reportDate, &title, &content, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &map[string]interface{}{
		"id": id, "user_id": uid, "username": username, "report_date": reportDate,
		"title": title, "content": content, "created_at": createdAt, "updated_at": updatedAt,
	}, nil
}

// SaveWorkDiary 创建或更新工作日报
func SaveWorkDiary(id, userID int, username, date, title, content string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	if id > 0 {
		// 更新
		_, err = conn.Exec("UPDATE sys_work_diary SET title=?, content=?, updated_at=NOW() WHERE id=? AND user_id=?", title, content, id, userID)
		return int64(id), err
	}
	// 插入（使用 REPLACE 支持同一用户同一天的更新）
	res, err := conn.Exec("INSERT INTO sys_work_diary (user_id, username, report_date, title, content) VALUES (?, ?, ?, ?, ?)", userID, username, date, title, content)
	if err != nil {
		return 0, err
	}
	newID, _ := res.LastInsertId()
	return newID, nil
}

// DeleteWorkDiary 删除工作日报
func DeleteWorkDiary(id, userID int, isAdmin bool) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	if isAdmin {
		_, err = conn.Exec("DELETE FROM sys_work_diary WHERE id = ?", id)
	} else {
		_, err = conn.Exec("DELETE FROM sys_work_diary WHERE id = ? AND user_id = ?", id, userID)
	}
	return err
}
