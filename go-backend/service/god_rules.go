package service

import (
	"ai-os-server/model"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── 额度监控 ──

var (
	quotaCache   = make(map[string]*QuotaInfo)
	quotaLock    sync.RWMutex
	quotaEnabled = true
)

type QuotaInfo struct {
	KeyID     string  `json:"key_id"`
	KeyName   string  `json:"key_name"`
	Pct       float64 `json:"pct"`
	Level     string  `json:"level"`
	NextReset string  `json:"next_reset"`
	UpdatedAt string  `json:"updated_at"`
	Error     *string `json:"error"`
}

var downgradeMap = map[string]string{
	"glm-5.1":      "glm-4.7",
	"glm-5-turbo":  "glm-4.7",
}

func GetKeyStatus(keyID string) string {
	if !quotaEnabled {
		return "normal"
	}
	quotaLock.RLock()
	info, ok := quotaCache[keyID]
	quotaLock.RUnlock()
	if !ok || info.Error != nil {
		return "normal"
	}
	if info.Pct >= 0.9 {
		return "exhausted"
	}
	if info.Pct >= 0.7 {
		return "degraded"
	}
	return "normal"
}

func ShouldDowngradeModel(keyID, modelID string) string {
	if GetKeyStatus(keyID) == "degraded" {
		if dm, ok := downgradeMap[modelID]; ok {
			return dm
		}
	}
	return modelID
}

func IsKeyExhausted(keyID string) bool {
	return GetKeyStatus(keyID) == "exhausted"
}

func GetQuotaStatus() map[string]interface{} {
	quotaLock.RLock()
	defer quotaLock.RUnlock()
	records := []map[string]interface{}{}
	for _, info := range quotaCache {
		records = append(records, map[string]interface{}{
			"key_id": info.KeyID, "key_name": info.KeyName,
			"pct": info.Pct, "level_tier": info.Level,
			"status": calcStatus(info.Pct),
			"next_reset": info.NextReset, "updated_at": info.UpdatedAt,
			"error": info.Error,
		})
	}
	return map[string]interface{}{
		"enabled": quotaEnabled, "records": records,
	}
}

func SetQuotaEnabled(enabled bool) {
	quotaLock.Lock()
	defer quotaLock.Unlock()
	quotaEnabled = enabled
}

func ForceQuotaCheck() {
	runQuotaCheck()
}

func StartQuotaMonitor() {
	go func() {
		time.Sleep(30 * time.Second)
		for {
			if quotaEnabled {
				runQuotaCheck()
			}
			time.Sleep(10 * time.Minute)
		}
	}()
}

func runQuotaCheck() {
	catalog, err := GetCatalog()
	if err != nil {
		return
	}
	for _, v := range catalog {
		code, _ := v["code"].(string)
		if code != "zhipu" {
			continue
		}
		enabled, _ := v["enabled"].(bool)
		if !enabled {
			continue
		}
		keys, _ := v["keys"].([]map[string]interface{})
		for _, k := range keys {
			keyEnabled, _ := k["enabled"].(bool)
			if !keyEnabled {
				continue
			}
			apiKey, _ := k["api_key"].(string)
			keyID := fmt.Sprintf("%v", k["id"])
			keyName, _ := k["name"].(string)
			if apiKey == "" {
				continue
			}
			info := checkSingleKey(apiKey, keyID, keyName)
			quotaLock.Lock()
			quotaCache[keyID] = info
			quotaLock.Unlock()
		}
	}
}

func checkSingleKey(apiKey, keyID, keyName string) *QuotaInfo {
	info := &QuotaInfo{KeyID: keyID, KeyName: keyName}
	now := time.Now().Format("2006-01-02 15:04:05")
	info.UpdatedAt = now

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", "https://bigmodel.cn/api/monitor/usage/quota/limit", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		errStr := err.Error()
		info.Error = &errStr
		return info
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		errStr := "parse error"
		info.Error = &errStr
		return info
	}
	if code, ok := data["code"].(float64); !ok || int(code) != 200 {
		if msg, ok := data["msg"].(string); ok {
			info.Error = &msg
		} else {
			errStr := "unknown error"
			info.Error = &errStr
		}
		return info
	}

	d, _ := data["data"].(map[string]interface{})
	if d != nil {
		if level, ok := d["level"].(string); ok {
			info.Level = level
		}
		if limits, ok := d["limits"].([]interface{}); ok {
			for _, l := range limits {
				if lm, ok := l.(map[string]interface{}); ok {
					if lm["type"] == "TOKENS_LIMIT" {
						if pct, ok := lm["percentage"].(float64); ok {
							info.Pct = pct / 100.0
						}
						if reset, ok := lm["nextResetTime"].(float64); ok && reset > 0 {
							if reset > 1e12 {
								reset /= 1000
							}
							info.NextReset = time.Unix(int64(reset), 0).Format("2006-01-02 15:04:05")
						}
						break
					}
				}
			}
		}
	}
	return info
}

func calcStatus(pct float64) string {
	if pct >= 0.9 {
		return "exhausted"
	}
	if pct >= 0.7 {
		return "degraded"
	}
	return "normal"
}

// ── 上帝指令（持久化到 MySQL）──

var (
	godRulesConfig = &model.GodRulesConfig{
		Enabled:        true,
		Rules:          "",
		PromptOptimize: true,
	}
	godRulesLock sync.RWMutex
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
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	e, po := 0, 0
	if enabled {
		e = 1
	}
	if promptOptimize {
		po = 1
	}
	conn.Exec("TRUNCATE TABLE sys_god_rules")
	conn.Exec("INSERT INTO sys_god_rules (enabled, rules, prompt_optimize) VALUES (?, ?, ?)", e, rules, po)
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
			original, _ := msg["content"].(string)
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
