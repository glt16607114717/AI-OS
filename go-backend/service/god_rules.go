package service

import (
	"ai-os-server/model"
	"fmt"
	"log"
	"strings"
	"sync"
)

// ── 上帝指令（持久化到 MySQL）──

var (
	godRulesConfig = &model.GodRulesConfig{
		Enabled:        true,
		Rules:          "",
		PromptOptimize: true,
	}
	godRulesLock   sync.RWMutex
	godRulesLoaded = false
)

func EnsureGodRulesTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_god_rules (
		id INT AUTO_INCREMENT PRIMARY KEY,
		enabled TINYINT DEFAULT 1,
		rules TEXT,
		prompt_optimize TINYINT DEFAULT 1,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func loadGodRulesFromDB() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	var enabled, promptOptimize int
	var rules string
	err := conn.QueryRow("SELECT enabled, rules, prompt_optimize FROM sys_god_rules ORDER BY id DESC LIMIT 1").Scan(&enabled, &rules, &promptOptimize)
	if err != nil {
		return // 表空，用默认值
	}
	godRulesLock.Lock()
	godRulesConfig = &model.GodRulesConfig{
		Enabled:        enabled == 1,
		Rules:          rules,
		PromptOptimize: promptOptimize == 1,
	}
	godRulesLock.Unlock()
}

func GetGodRules() *model.GodRulesConfig {
	if !godRulesLoaded {
		loadGodRulesFromDB()
		godRulesLoaded = true
	}
	godRulesLock.RLock()
	defer godRulesLock.RUnlock()
	return godRulesConfig
}

func SaveGodRules(enabled bool, rules string, promptOptimize bool) {
	godRulesLock.Lock()
	godRulesConfig = &model.GodRulesConfig{
		Enabled:        enabled,
		Rules:          rules,
		PromptOptimize: promptOptimize,
	}
	godRulesLock.Unlock()

	// 持久化
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
	if _, err := conn.Exec("TRUNCATE TABLE sys_god_rules"); err != nil {
		log.Printf("[god_rules] TRUNCATE 失败: %v", err)
	}
	if _, err := conn.Exec("INSERT INTO sys_god_rules (enabled, rules, prompt_optimize) VALUES (?, ?, ?)", e, rules, po); err != nil {
		log.Printf("[god_rules] INSERT 失败: %v", err)
	}
}

func InjectGodRules(messages []map[string]interface{}) []map[string]interface{} {
	godRulesLock.RLock()
	cfg := godRulesConfig
	godRulesLock.RUnlock()

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

func IsOptimizeEnabled() bool {
	godRulesLock.RLock()
	defer godRulesLock.RUnlock()
	return godRulesConfig.PromptOptimize
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
