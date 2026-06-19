package service

import (
	"ai-os-server/middleware"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
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
