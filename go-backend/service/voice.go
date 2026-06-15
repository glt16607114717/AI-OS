package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
)

// VoiceCommand 语音指令
type VoiceCommand struct {
	ID        int     `json:"id"`
	UserID    int     `json:"user_id"`
	Phrase    string  `json:"phrase"`
	Position  *string `json:"position"` // JSON: {"x":100,"y":200}
	Actions   *string `json:"actions"`  // JSON: [...]
	Enabled   bool    `json:"enabled"`
	SortOrder int     `json:"sort_order"`
}

// EnsureVoiceTable 自动建表
func EnsureVoiceTable() {
	db, err := GetDB()
	if err != nil {
		log.Printf("[voice] DB连接失败: %v", err)
		return
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS voice_commands (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL,
		phrase TEXT,
		position JSON DEFAULT NULL,
		actions JSON DEFAULT NULL,
		enabled TINYINT(1) DEFAULT 1,
		sort_order INT DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_user (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		log.Printf("[voice] 建表失败: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sys_config (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL,
		cfg_key VARCHAR(100) NOT NULL,
		cfg_value TEXT,
		UNIQUE KEY uk_user_key (user_id, cfg_key)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		log.Printf("[voice] sys_config 建表失败: %v", err)
	}
}

// GetVoiceCommands 获取用户的所有语音指令
func GetVoiceCommands(userID int) ([]VoiceCommand, error) {
	db, dbErr := GetDB()
	if dbErr != nil {
		return nil, dbErr
	}
	rows, err := db.Query(
		"SELECT id, user_id, phrase, position, actions, enabled, sort_order FROM voice_commands WHERE user_id = ? ORDER BY sort_order, id",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []VoiceCommand
	for rows.Next() {
		var c VoiceCommand
		if err := rows.Scan(&c.ID, &c.UserID, &c.Phrase, &c.Position, &c.Actions, &c.Enabled, &c.SortOrder); err != nil {
			continue
		}
		cmds = append(cmds, c)
	}
	if cmds == nil {
		cmds = []VoiceCommand{}
	}
	return cmds, nil
}

// AddVoiceCommand 添加语音指令
func AddVoiceCommand(userID int, phrase string) (*VoiceCommand, error) {
	db, dbErr := GetDB()
	if dbErr != nil {
		return nil, dbErr
	}
	result, err := db.Exec(
		"INSERT INTO voice_commands (user_id, phrase) VALUES (?, ?)",
		userID, phrase,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &VoiceCommand{ID: int(id), UserID: userID, Phrase: phrase, Enabled: true}, nil
}

// UpdateVoiceCommand 更新语音指令
func UpdateVoiceCommand(userID, cmdID int, updates map[string]interface{}) error {
	db, dbErr := GetDB()
	if dbErr != nil {
		return dbErr
	}

	// 验证归属
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM voice_commands WHERE id = ? AND user_id = ?", cmdID, userID).Scan(&count); err != nil {
		return fmt.Errorf("验证归属失败: %v", err)
	}
	if count == 0 {
		return sql.ErrNoRows
	}

	sets := ""
	args := []interface{}{}

	if phrase, ok := updates["phrase"].(string); ok {
		if sets != "" { sets += ", " }
		sets += "phrase = ?"
		args = append(args, phrase)
	}
	if enabled, ok := updates["enabled"].(bool); ok {
		if sets != "" { sets += ", " }
		sets += "enabled = ?"
		args = append(args, enabled)
	}
	if pos, ok := updates["position"]; ok {
		if sets != "" { sets += ", " }
		sets += "position = ?"
		b, _ := json.Marshal(pos)
		args = append(args, string(b))
	}
	if actions, ok := updates["actions"]; ok {
		if sets != "" { sets += ", " }
		sets += "actions = ?"
		b, _ := json.Marshal(actions)
		args = append(args, string(b))
	}

	if sets == "" {
		return nil
	}

	args = append(args, cmdID)
	_, err := db.Exec("UPDATE voice_commands SET "+sets+" WHERE id = ?", args...)
	return err
}

// DeleteVoiceCommand 删除语音指令
func DeleteVoiceCommand(userID, cmdID int) error {
	db, dbErr := GetDB()
	if dbErr != nil {
		return dbErr
	}
	_, err := db.Exec("DELETE FROM voice_commands WHERE id = ? AND user_id = ?", cmdID, userID)
	return err
}

// SetVoiceEnabled 设置语音启用状态
func SetVoiceEnabled(userID int, enabled bool) error {
	db, dbErr := GetDB()
	if dbErr != nil {
		return dbErr
	}
	val := boolToStr(enabled)
	_, err := db.Exec(
		"INSERT INTO sys_config (user_id, cfg_key, cfg_value) VALUES (?, 'voice_enabled', ?) ON DUPLICATE KEY UPDATE cfg_value = ?",
		userID, val, val,
	)
	return err
}

// GetVoiceEnabled 获取语音启用状态
func GetVoiceEnabled(userID int) bool {
	db, dbErr := GetDB()
	if dbErr != nil {
		return false
	}
	var val string
	err := db.QueryRow(
		"SELECT cfg_value FROM sys_config WHERE user_id = ? AND cfg_key = 'voice_enabled'",
		userID,
	).Scan(&val)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[voice] 查询 voice_enabled 失败: %v", err)
		}
		return false
	}
	return val == "true"
}

func boolToStr(b bool) string {
	if b { return "true" }
	return "false"
}
