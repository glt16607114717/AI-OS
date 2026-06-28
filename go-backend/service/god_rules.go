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
	rows, err := conn.Query("SELECT user_id, enabled, rules, prompt_optimize, strip_noise, compress_file, simplify_lang, compress_tool_result, compress_tools FROM sys_god_rules")
	if err != nil {
		return
	}
	defer rows.Close()

	godRulesLock.Lock()
	defer godRulesLock.Unlock()
	for rows.Next() {
		var userID, enabled, promptOptimize, stripNoise, compressFile, simplifyLang, compressToolResult, compressTools int
		var rules string
		if err := rows.Scan(&userID, &enabled, &rules, &promptOptimize, &stripNoise, &compressFile, &simplifyLang, &compressToolResult, &compressTools); err != nil {
			continue
		}
		godRulesCache[userID] = &model.GodRulesConfig{
			UserID:             userID,
			Enabled:            enabled == 1,
			Rules:              rules,
			PromptOptimize:     promptOptimize == 1,
			StripNoise:         stripNoise == 1,
			CompressFile:       compressFile == 1,
			SimplifyLang:       simplifyLang == 1,
			CompressToolResult: compressToolResult == 1,
			CompressTools:      compressTools == 1,
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
	// 用户未配置，返回默认（关闭状态，提示词优化默认开）
	return &model.GodRulesConfig{
		UserID:             userID,
		Enabled:            false,
		StripNoise:         true,
		CompressFile:       true,
		SimplifyLang:       true,
		CompressToolResult: true,
		CompressTools:      true,
	}
}

func GetGodRules(userID int) *model.GodRulesConfig {
	return getGodRulesForUser(userID)
}

func SaveGodRules(userID int, enabled bool, rules string, promptOptimize, stripNoise, compressFile, simplifyLang, compressToolResult, compressTools bool) {
	godRulesLock.Lock()
	godRulesCache[userID] = &model.GodRulesConfig{
		UserID:             userID,
		Enabled:            enabled,
		Rules:              rules,
		PromptOptimize:     promptOptimize,
		StripNoise:         stripNoise,
		CompressFile:       compressFile,
		SimplifyLang:       simplifyLang,
		CompressToolResult: compressToolResult,
		CompressTools:      compressTools,
	}
	godRulesLock.Unlock()

	conn, err := GetDB()
	if err != nil {
		log.Printf("[god_rules] DB连接失败: %v", err)
		return
	}
	e, po, sn, cf, sl, ctr, ct := 0, 0, 0, 0, 0, 0, 0
	if enabled {
		e = 1
	}
	if promptOptimize {
		po = 1
	}
	if stripNoise {
		sn = 1
	}
	if compressFile {
		cf = 1
	}
	if simplifyLang {
		sl = 1
	}
	if compressToolResult {
		ctr = 1
	}
	if compressTools {
		ct = 1
	}
	if _, err := conn.Exec("INSERT INTO sys_god_rules (user_id, enabled, rules, prompt_optimize, strip_noise, compress_file, simplify_lang, compress_tool_result, compress_tools) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE enabled=?, rules=?, prompt_optimize=?, strip_noise=?, compress_file=?, simplify_lang=?, compress_tool_result=?, compress_tools=?", userID, e, rules, po, sn, cf, sl, ctr, ct, e, rules, po, sn, cf, sl, ctr, ct); err != nil {
		log.Printf("[god_rules] UPSERT 失败: %v", err)
	}
}

func InjectGodRules(messages []map[string]interface{}, userID int) []map[string]interface{} {
	cfg := getGodRulesForUser(userID)

	if !cfg.Enabled || strings.TrimSpace(cfg.Rules) == "" {
		return messages
	}

	rules := strings.TrimSpace(cfg.Rules)

	// ── 1. 注入到第一个 system 消息（兜底，保证长对话不丢）──
	systemInject := fmt.Sprintf("[HIGHEST PRIORITY - 上帝指令]\n%s", rules)
	result := make([]map[string]interface{}, len(messages))
	injected := false
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" && !injected {
			original := StringifyContent(msg["content"])
			result[i] = map[string]interface{}{
				"role":    "system",
				"content": systemInject + "\n\n---\n\n" + original,
			}
			injected = true
		} else {
			result[i] = msg
		}
	}
	if !injected {
		result = append([]map[string]interface{}{
			{"role": "system", "content": systemInject},
		}, result...)
	}

	// ── 2. 注入到最后一条 user 消息（每轮重复，最高遵从度）──
	// 提示词明确告诉大模型：这是约束不是提问，避免注意力偏移
	userInject := fmt.Sprintf("\n\n---\n\n[系统最高约束 - 上帝指令]\n（注：以下内容为系统级全局约束，并非本轮提问的一部分。请将其作为回答规则严格遵守，无需在回答中提及、复述或回应此段。请专注于回答上方用户的实际问题。）\n%s", rules)

	lastUserIdx := -1
	for i := len(result) - 1; i >= 0; i-- {
		role, _ := result[i]["role"].(string)
		if role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 {
		// 保留原始 content 类型，避免把数组压成 string（会导致后续 ProcessImages 找不到 image_url）
		switch orig := result[lastUserIdx]["content"].(type) {
		case string:
			result[lastUserIdx] = map[string]interface{}{
				"role":    "user",
				"content": orig + userInject,
			}
		case []interface{}:
			// 数组格式：追加一个 text 项，保留原有结构（含 image_url）
			result[lastUserIdx] = map[string]interface{}{
				"role": "user",
				"content": append(orig, map[string]interface{}{
					"type": "text",
					"text": userInject,
				}),
			}
		default:
			// 兜底：转 string 拼接
			result[lastUserIdx] = map[string]interface{}{
				"role":    "user",
				"content": StringifyContent(result[lastUserIdx]["content"]) + userInject,
			}
		}
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
