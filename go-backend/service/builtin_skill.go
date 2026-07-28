package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 技能相关表自动建表
func EnsureBuiltinSkillTables() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_builtin_skill (
		id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		code VARCHAR(64) NOT NULL UNIQUE,
		name VARCHAR(64) NOT NULL,
		description TEXT NOT NULL,
		icon VARCHAR(64) DEFAULT 'database',
		config_schema JSON NOT NULL,
		tool_schema JSON NOT NULL,
		enabled TINYINT NOT NULL DEFAULT 1,
		sort_order INT NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_skill_connection (
		id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		skill_id INT UNSIGNED NOT NULL,
		name VARCHAR(64) NOT NULL,
		config JSON NOT NULL,
		enabled TINYINT NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		FOREIGN KEY (skill_id) REFERENCES sys_builtin_skill(id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_skill_permission (
		id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		skill_id INT UNSIGNED,
		connection_id INT UNSIGNED,
		user_id INT UNSIGNED NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (skill_id) REFERENCES sys_builtin_skill(id) ON DELETE CASCADE,
		FOREIGN KEY (connection_id) REFERENCES sys_skill_connection(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES sys_user(id) ON DELETE CASCADE,
		UNIQUE KEY uk_user_skill_conn (user_id, skill_id, connection_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

// ── AES 加密/解密 ──

var aesKey = []byte("ai-os-skill-key!") // 16 bytes for AES-128

func EncryptPassword(plain string) (string, error) {
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	encrypted := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func DecryptPassword(encrypted string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// ── 技能注册表 ──

type BuiltinSkill struct {
	ID           int             `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Icon         string          `json:"icon"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	ToolSchema   json.RawMessage `json:"tool_schema"`
	Enabled      bool            `json:"enabled"`
	SortOrder    int             `json:"sort_order"`
}

// GetBuiltinSkills 获取所有启用的技能（用于前端菜单）
func GetBuiltinSkills() ([]BuiltinSkill, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, code, name, description, icon, config_schema, tool_schema, enabled, sort_order FROM sys_builtin_skill WHERE enabled = 1 ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []BuiltinSkill
	for rows.Next() {
		var s BuiltinSkill
		var enabled int
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Icon, &s.ConfigSchema, &s.ToolSchema, &enabled, &s.SortOrder); err != nil {
			continue
		}
		s.Enabled = enabled == 1
		skills = append(skills, s)
	}
	return skills, nil
}

// GetBuiltinSkillByCode 根据code获取技能
func GetBuiltinSkillByCode(code string) (*BuiltinSkill, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var s BuiltinSkill
	var enabled int
	err = conn.QueryRow("SELECT id, code, name, description, icon, config_schema, tool_schema, enabled, sort_order FROM sys_builtin_skill WHERE code = ? AND enabled = 1", code).
		Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Icon, &s.ConfigSchema, &s.ToolSchema, &enabled, &s.SortOrder)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled == 1
	return &s, nil
}

// ── AI 工具注入 ──

// universalSkills 不需要连接和权限的通用技能（所有用户可用）
var universalSkills = map[string]bool{
	"submit_requirement": true,
	"submit_bug":         true,
	"generate_image":     true,
}

// GetBuiltinSkillToolDefinitions 根据用户权限构造 tools 注入给 AI
func GetBuiltinSkillToolDefinitions(userID int, isAdmin bool) ([]map[string]interface{}, error) {
	skills, err := GetBuiltinSkills()
	if err != nil {
		return nil, err
	}

	var tools []map[string]interface{}
	for _, s := range skills {
		// 通用技能跳过权限检查
		if !universalSkills[s.Code] {
			// 权限过滤：非管理员需要检查是否有权限
			if !isAdmin {
				hasPerm, err := checkUserSkillPermission(userID, s.ID)
				if err != nil || !hasPerm {
					continue
				}
			}
		}

		// 解析 tool_schema
		var params interface{}
		json.Unmarshal(s.ToolSchema, &params)

		// 构造 description（包含可用连接名 + 每个库的用途说明 + 调用约束）
		desc := s.Description
		connections, _ := getUserSkillConnections(userID, s.ID, isAdmin)
		if len(connections) > 0 {
			var parts []string
			for _, c := range connections {
				// 把每个连接的描述一起带上，让 AI 知道每个库装的是什么
				if c.Description != "" {
					parts = append(parts, fmt.Sprintf("%s[%s](%s/%s)", c.Name, c.Description, getConfigField(c.Config, "host"), getConfigField(c.Config, "database")))
				} else {
					parts = append(parts, fmt.Sprintf("%s(%s/%s)", c.Name, getConfigField(c.Config, "host"), getConfigField(c.Config, "database")))
				}
			}
			desc = fmt.Sprintf("%s\n可用连接：%s\n使用原则：1) 先根据每个连接的描述判断是否真的需要查这个库——如果用户的问题与该库存储的数据无关，则不要调用；2) 不要用于验证连接可用性（禁止 SELECT 1 这类探活语句），也不要出于好奇去探索数据库里有什么；3) 若本对话上下文中已出现过某连接的表清单，直接复用，不要重复 SHOW TABLES；4) 仅当上下文中确实没有该连接的表清单且确实需要查表时，才执行一次 SHOW TABLES；5) 不同连接的表结构不同，不要跨连接假设表名。", desc, strings.Join(parts, "、"))
		}

		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "skill_" + s.Code,
				"description": desc,
				"parameters":  params,
			},
		})
	}
	return tools, nil
}

// checkUserSkillPermission 检查用户是否有技能权限
func checkUserSkillPermission(userID int, skillID int) (bool, error) {
	conn, err := GetDB()
	if err != nil {
		return false, err
	}
	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM sys_skill_permission WHERE user_id = ? AND skill_id = ?", userID, skillID).Scan(&count)
	return count > 0, err
}

// getConfigField 从 JSON config 中安全提取字段
func getConfigField(configRaw json.RawMessage, field string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(configRaw, &m); err != nil {
		return ""
	}
	v, _ := m[field].(string)
	return v
}

// ── 连接管理 ──

type SkillConnection struct {
	ID          int             `json:"id"`
	SkillID     int             `json:"skill_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	Masked      bool            `json:"masked,omitempty"` // 前端显示时密码是否脱敏
}

// GetSkillConnections 获取技能的所有连接（管理员）
func GetSkillConnections(skillID int) ([]SkillConnection, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, skill_id, name, description, config, enabled FROM sys_skill_connection WHERE skill_id = ? ORDER BY id", skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SkillConnection
	for rows.Next() {
		var c SkillConnection
		var enabled int
		var desc sql.NullString
		if err := rows.Scan(&c.ID, &c.SkillID, &c.Name, &desc, &c.Config, &enabled); err != nil {
			continue
		}
		if desc.Valid {
			c.Description = desc.String
		}
		c.Enabled = enabled == 1
		c.Config = maskPassword(c.Config)
		result = append(result, c)
	}
	return result, nil
}

// getUserSkillConnections 获取用户可用的连接
func getUserSkillConnections(userID int, skillID int, isAdmin bool) ([]SkillConnection, error) {
	if isAdmin {
		return GetSkillConnections(skillID)
	}
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`SELECT c.id, c.skill_id, c.name, c.description, c.config, c.enabled
		FROM sys_skill_connection c
		INNER JOIN sys_skill_permission p ON p.connection_id = c.id
		WHERE c.skill_id = ? AND p.user_id = ? AND c.enabled = 1
		ORDER BY c.id`, skillID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SkillConnection
	for rows.Next() {
		var c SkillConnection
		var enabled int
		var desc sql.NullString
		if err := rows.Scan(&c.ID, &c.SkillID, &c.Name, &desc, &c.Config, &enabled); err != nil {
			continue
		}
		if desc.Valid {
			c.Description = desc.String
		}
		c.Enabled = enabled == 1
		c.Config = maskPassword(c.Config)
		result = append(result, c)
	}
	return result, nil
}

// maskPassword 脱敏密码字段
func maskPassword(configRaw json.RawMessage) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(configRaw, &m); err != nil {
		return configRaw
	}
	if _, ok := m["password"]; ok {
		m["password"] = "***"
	}
	out, _ := json.Marshal(m)
	return out
}

// CreateSkillConnection 新建连接
func CreateSkillConnection(skillID int, name string, description string, config map[string]interface{}) error {
	// 加密密码
	if pwd, ok := config["password"].(string); ok && pwd != "" {
		encrypted, err := EncryptPassword(pwd)
		if err != nil {
			return fmt.Errorf("密码加密失败: %v", err)
		}
		config["password"] = encrypted
	}

	configJSON, _ := json.Marshal(config)
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("INSERT INTO sys_skill_connection (skill_id, name, description, config) VALUES (?, ?, ?, ?)", skillID, name, description, string(configJSON))
	return err
}

// UpdateSkillConnection 更新连接
func UpdateSkillConnection(connID int, name string, description string, config map[string]interface{}) error {
	// 如果密码是 *** 则保持原密码
	if pwd, ok := config["password"].(string); ok && pwd == "***" {
		// 读取原密码
		conn, err := GetDB()
		if err != nil {
			return err
		}
		var oldConfigJSON string
		conn.QueryRow("SELECT config FROM sys_skill_connection WHERE id = ?", connID).Scan(&oldConfigJSON)
		var oldConfig map[string]interface{}
		json.Unmarshal([]byte(oldConfigJSON), &oldConfig)
		if oldPwd, ok := oldConfig["password"].(string); ok {
			config["password"] = oldPwd
		}
	} else if pwd, ok := config["password"].(string); ok && pwd != "" {
		encrypted, err := EncryptPassword(pwd)
		if err != nil {
			return fmt.Errorf("密码加密失败: %v", err)
		}
		config["password"] = encrypted
	}

	configJSON, _ := json.Marshal(config)
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("UPDATE sys_skill_connection SET name = ?, description = ?, config = ? WHERE id = ?", name, description, string(configJSON), connID)
	return err
}

// DeleteSkillConnection 删除连接
func DeleteSkillConnection(connID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("DELETE FROM sys_skill_connection WHERE id = ?", connID)
	return err
}

// ToggleSkillConnection 启用/停用连接
func ToggleSkillConnection(connID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec("UPDATE sys_skill_connection SET enabled = 1 - enabled WHERE id = ?", connID)
	return err
}

// ── 权限管理 ──

type SkillPermissionInfo struct {
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
	Connections  []int  `json:"connection_ids"`
	ConnectionNames string `json:"connection_names"`
}

// GetSkillPermissions 获取技能的权限分配
func GetSkillPermissions(skillID int) ([]SkillPermissionInfo, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`SELECT p.user_id, u.username, p.connection_id, c.name
		FROM sys_skill_permission p
		INNER JOIN sys_user u ON u.id = p.user_id
		LEFT JOIN sys_skill_connection c ON c.id = p.connection_id
		WHERE p.skill_id = ?
		ORDER BY p.user_id`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userMap := make(map[int]*SkillPermissionInfo)
	for rows.Next() {
		var userID int
		var username string
		var connID sql.NullInt64
		var connName sql.NullString
		if err := rows.Scan(&userID, &username, &connID, &connName); err != nil {
			continue
		}
		info, ok := userMap[userID]
		if !ok {
			info = &SkillPermissionInfo{UserID: userID, Username: username}
			userMap[userID] = info
		}
		if connID.Valid {
			info.Connections = append(info.Connections, int(connID.Int64))
			if connName.Valid {
				if info.ConnectionNames != "" {
					info.ConnectionNames += ", "
				}
				info.ConnectionNames += connName.String
			}
		}
	}

	var result []SkillPermissionInfo
	for _, v := range userMap {
		result = append(result, *v)
	}
	return result, nil
}

// SetSkillPermission 设置用户权限（全量替换）
func SetSkillPermission(skillID int, userID int, connectionIDs []int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	// 先删除旧权限
	tx.Exec("DELETE FROM sys_skill_permission WHERE skill_id = ? AND user_id = ?", skillID, userID)
	// 插入新权限
	for _, connID := range connectionIDs {
		tx.Exec("INSERT INTO sys_skill_permission (skill_id, connection_id, user_id) VALUES (?, ?, ?)", skillID, connID, userID)
	}
	return tx.Commit()
}

// SetSkillPermissions 批量设置用户权限
func SetSkillPermissions(skillID int, permissions []struct {
	UserID        int   `json:"user_id"`
	ConnectionIDs []int `json:"connection_ids"`
}) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	for _, p := range permissions {
		tx.Exec("DELETE FROM sys_skill_permission WHERE skill_id = ? AND user_id = ?", skillID, p.UserID)
		for _, connID := range p.ConnectionIDs {
			tx.Exec("INSERT INTO sys_skill_permission (skill_id, connection_id, user_id) VALUES (?, ?, ?)", skillID, connID, p.UserID)
		}
	}
	return tx.Commit()
}

// ── 技能执行 ──

// ExecuteBuiltinSkill 执行内置技能
func ExecuteBuiltinSkill(code string, userID int, username string, isAdmin bool, args map[string]interface{}) (map[string]interface{}, error) {
	skill, err := GetBuiltinSkillByCode(code)
	if err != nil {
		return nil, fmt.Errorf("未知技能: %s", code)
	}

	// 权限二次校验：通用技能（如 submit_requirement）跳过，所有用户可用
	if !isAdmin && !universalSkills[skill.Code] {
		hasPerm, err := checkUserSkillPermission(userID, skill.ID)
		if err != nil || !hasPerm {
			return nil, fmt.Errorf("无权限执行此技能")
		}
	}

	switch skill.Code {
	case "mysql_query":
		return executeMySQLQuery(skill, userID, isAdmin, args)
	case "submit_requirement":
		return ExecuteSubmitRequirement(userID, args)
	case "submit_bug":
		return ExecuteSubmitBug(userID, args)
	case "generate_image":
		return ExecuteGenerateImage(userID, username, args)
	default:
		return nil, fmt.Errorf("未实现的技能: %s", code)
	}
}

// executeMySQLQuery 执行 MySQL 查询
func executeMySQLQuery(skill *BuiltinSkill, userID int, isAdmin bool, args map[string]interface{}) (map[string]interface{}, error) {
	sqlText, _ := args["sql"].(string)
	if sqlText == "" {
		return nil, fmt.Errorf("缺少 SQL 参数")
	}

	// 确定连接
	connectionName, _ := args["connection_name"].(string)
	connections, err := getUserSkillConnections(userID, skill.ID, isAdmin)
	if err != nil || len(connections) == 0 {
		return nil, fmt.Errorf("没有可用的数据库连接")
	}

	var selectedConn *SkillConnection
	if connectionName != "" {
		for i := range connections {
			if connections[i].Name == connectionName {
				selectedConn = &connections[i]
				break
			}
		}
	}
	if selectedConn == nil {
		selectedConn = &connections[0] // 默认第一个
	}

	// 解密连接配置
	var config map[string]interface{}
	json.Unmarshal(selectedConn.Config, &config)

	// 如果是脱敏的，需要重新读取原始配置
	if pwd, ok := config["password"].(string); ok && pwd == "***" {
		conn, _ := GetDB()
		var rawConfig string
		conn.QueryRow("SELECT config FROM sys_skill_connection WHERE id = ?", selectedConn.ID).Scan(&rawConfig)
		json.Unmarshal([]byte(rawConfig), &config)
	}

	host, _ := config["host"].(string)
	port := 3306
	if p, ok := config["port"].(float64); ok {
		port = int(p)
	}
	user, _ := config["user"].(string)
	encPass, _ := config["password"].(string)
	dbName, _ := config["database"].(string)

	password, err := DecryptPassword(encPass)
	if err != nil {
		return nil, fmt.Errorf("密码解密失败: %v", err)
	}

	// SQL 安全检查
	upperSQL := strings.TrimSpace(strings.ToUpper(sqlText))
	if !strings.HasPrefix(upperSQL, "SELECT") && !strings.HasPrefix(upperSQL, "SHOW") &&
		!strings.HasPrefix(upperSQL, "DESCRIBE") && !strings.HasPrefix(upperSQL, "DESC") &&
		!strings.HasPrefix(upperSQL, "EXPLAIN") {
		return nil, fmt.Errorf("仅允许 SELECT/SHOW/DESCRIBE/EXPLAIN 查询")
	}

	// 自动追加 LIMIT
	if strings.HasPrefix(upperSQL, "SELECT") && !strings.Contains(upperSQL, "LIMIT") {
		sqlText = sqlText + " LIMIT 100"
	}

	// 连接目标 MySQL
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=10s",
		user, password, host, port, dbName)
	targetDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %v", err)
	}
	defer targetDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	rows, err := targetDB.QueryContext(ctx, sqlText)
	if err != nil {
		// 表不存在等错误时，自动返回表列表帮助 AI 自纠正
		errMsg := err.Error()
		if strings.Contains(errMsg, "doesn't exist") || strings.Contains(errMsg, "Unknown table") || strings.Contains(errMsg, "1146") {
			tableRows, tableErr := targetDB.Query("SHOW TABLES")
			if tableErr == nil {
				var tables []string
				for tableRows.Next() {
					var t string
					tableRows.Scan(&t)
					tables = append(tables, t)
				}
				tableRows.Close()
			return map[string]interface{}{
				"error":      errMsg,
				"hint":       "该数据库中不存在此表，以下是可用的表列表，请使用正确的表名重新查询",
				"tables":     tables,
				"connection": selectedConn.Name,
				"trace":      fmt.Sprintf("🔍 %s\n%s\n⚠️ %s。可用表：%s", selectedConn.Name, sqlText, errMsg, strings.Join(tables, ", ")),
			}, nil
			}
		}
		return nil, fmt.Errorf("SQL执行失败: %v", err)
	}
	defer rows.Close()

	latency := time.Since(startTime).Milliseconds()

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
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	log.Printf("[skill] mysql_query user=%d conn=%s rows=%d latency=%dms", userID, selectedConn.Name, len(result), latency)

	return map[string]interface{}{
		"columns":    cols,
		"rows":       result,
		"row_count":  len(result),
		"latency_ms": latency,
		"connection": selectedConn.Name,
		"sql":        sqlText,
		"trace":      fmt.Sprintf("🔍 %s\n%s\n✅ 返回 %d 行", selectedConn.Name, sqlText, len(result)),
	}, nil
}
