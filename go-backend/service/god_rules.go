package service

import (
	"ai-os-server/model"
	"fmt"
	"log"
	"strings"
	"sync"
)

// ── 上帝指令（持久化到 MySQL，按用户隔离）──

var (
	// userID → config 内存缓存
	godRulesCache   = make(map[int]*model.GodRulesConfig)
	godRulesLock    sync.RWMutex
	godRulesLoaded  = false
)

func EnsureGodRulesTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_god_rules (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL DEFAULT 0,
		enabled TINYINT DEFAULT 1,
		rules TEXT,
		prompt_optimize TINYINT DEFAULT 1,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uk_user (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func loadGodRulesFromDB() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	rows, err := conn.Query("SELECT user_id, enabled, rules, prompt_optimize FROM sys_god_rules")
	if err != nil {
		return
	}
	defer rows.Close()

	godRulesLock.Lock()
	defer godRulesLock.Unlock()
	for rows.Next() {
		var userID, enabled, promptOptimize int
		var rules string
		if err := rows.Scan(&userID, &enabled, &rules, &promptOptimize); err != nil {
			continue
		}
		godRulesCache[userID] = &model.GodRulesConfig{
			UserID:         userID,
			Enabled:        enabled == 1,
			Rules:          rules,
			PromptOptimize: promptOptimize == 1,
		}
	}
}

// getGodRulesForUser 获取指定用户的上帝指令配置（带缓存）
func getGodRulesForUser(userID int) *model.GodRulesConfig {
	if !godRulesLoaded {
		loadGodRulesFromDB()
		godRulesLoaded = true
	}
	godRulesLock.RLock()
	cfg, ok := godRulesCache[userID]
	godRulesLock.RUnlock()
	if ok {
		return cfg
	}
	// 用户未配置，返回默认（关闭状态）
	return &model.GodRulesConfig{
		UserID:  userID,
		Enabled: false,
	}
}

func GetGodRules(userID int) *model.GodRulesConfig {
	return getGodRulesForUser(userID)
}

func SaveGodRules(userID int, enabled bool, rules string, promptOptimize bool) {
	godRulesLock.Lock()
	godRulesCache[userID] = &model.GodRulesConfig{
		UserID:         userID,
		Enabled:        enabled,
		Rules:          rules,
		PromptOptimize: promptOptimize,
	}
	godRulesLock.Unlock()

	conn, err := GetDB()
	if err != nil {
		log.Printf("[god_rules] DB连接失败: %v", err)
		return
	}
	e, po := 0, 0
	if enabled {
		e = 1
	}
	if promptOptimize {
		po = 1
	}
	if _, err := conn.Exec("INSERT INTO sys_god_rules (user_id, enabled, rules, prompt_optimize) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE enabled=?, rules=?, prompt_optimize=?", userID, e, rules, po, e, rules, po); err != nil {
		log.Printf("[god_rules] UPSERT 失败: %v", err)
	}
}

func InjectGodRules(messages []map[string]interface{}, userID int) []map[string]interface{} {
	cfg := getGodRulesForUser(userID)

	if !cfg.Enabled || strings.TrimSpace(cfg.Rules) == "" {
		return messages
	}

	inject := fmt.Sprintf("[HIGHEST PRIORITY - 上帝指令]\n%s", strings.TrimSpace(cfg.Rules))

	// 注入到第一个 system 消息
	result := make([]map[string]interface{}, len(messages))
	injected := false
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" && !injected {
			original := StringifyContent(msg["content"])
			result[i] = map[string]interface{}{
				"role":    "system",
				"content": inject + "\n\n---\n\n" + original,
			}
			injected = true
		} else {
			result[i] = msg
		}
	}
	if !injected {
		result = append([]map[string]interface{}{
			{"role": "system", "content": inject},
		}, result...)
	}
	return result
}

func IsOptimizeEnabled(userID int) bool {
	cfg := getGodRulesForUser(userID)
	return cfg.PromptOptimize
}

// StringifyContent 兼容 string 和 []interface{} 格式的 content，统一转成 string
// 用于上帝指令/RAG 注入时避免直接断言 content.(string) 丢掉数组格式的数据
func StringifyContent(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if m, ok := item.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "text" {
					if text, _ := m["text"].(string); text != "" {
						sb.WriteString(text)
					}
				}
			}
		}
		return sb.String()
	}
	return ""
}
