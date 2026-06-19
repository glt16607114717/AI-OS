package service

import (
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
	"glm-5.1":     "glm-4.7",
	"glm-5-turbo": "glm-4.7",
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
			"status":     calcStatus(info.Pct),
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
			var info *QuotaInfo
			switch strings.ToLower(code) {
			case "zhipu":
				info = checkZhipuKey(apiKey, keyID, keyName)
			default:
				continue
			}
			quotaLock.Lock()
			quotaCache[keyID] = info
			quotaLock.Unlock()
		}
	}
}

func checkZhipuKey(apiKey, keyID, keyName string) *QuotaInfo {
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
