package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── 额度监控 + 智谱窗口自动锚定 ──

var (
	quotaCache   = make(map[string]*QuotaInfo)
	quotaLock    sync.RWMutex
	quotaEnabled = true
)

// ZhipuAnchorConfig 智谱窗口自动锚定配置
// 每天定点锚定窗口，让高峰期自动分摊到多个窗口
type ZhipuAnchorConfig struct {
	Enabled bool `json:"enabled"`
	// 每日锚定时间（小时，UTC+8，0-23）
	// 多个锚点，每个锚点会自然滚动出 5 小时窗口
	// 例如：[6] → 每天 6 点锚定，自然滚动出 6→11→16→21
	AnchorHours []int `json:"anchor_hours"`
}

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

// ════════════════════════════════════════════════════════════════
// 智谱 5 小时窗口自动锚定调度器
//
// 原理（已通过 logs/quota 历史数据验证）：
//   智谱 coding plan 的额度窗口是「以首次请求为锚点」的 5 小时滚动窗口，
//   重置时间 = 首次请求时间 + 5 小时（严格 5 小时，误差 < 5 分钟）。
//   闲置时时钟冻结，直到下一次请求才开新窗口。
//
// 策略：
//   每天早上 6:00 主动发一条心跳请求，锚定 06:00→11:00 窗口。
//   之后团队自然使用会链式滚动出 11:00→16:00、16:00→21:00 等窗口，
//   使下午 14:00-18:00 高峰期被劈进两个独立窗口，配额翻倍，避免击穿。
//
//   注意：cron 蒸馏/巡检已改到 23:00，锚定夜间窗口，不会抢占白天锚点。
// ════════════════════════════════════════════════════════════════

var zhipuAnchorHours = []int{6} // 每日锚定时间（小时，服务器本地时区），默认早上 6 点

// StartZhipuAnchor 启动智谱窗口自动锚定调度器
func StartZhipuAnchor() {
	go func() {
		for {
			now := time.Now()
			next := nextAnchorTime(now, zhipuAnchorHours)
			wait := next.Sub(now)
			log.Printf("[anchor] 下次窗口锚定时间: %s（%.1f 小时后）",
				next.Format("2006-01-02 15:04:05"), wait.Hours())
			time.Sleep(wait)
			runZhipuAnchor()
		}
	}()
}

// nextAnchorTime 计算下一个锚点时刻
func nextAnchorTime(now time.Time, hours []int) time.Time {
	if len(hours) == 0 {
		hours = []int{6}
	}
	var candidates []time.Time
	for _, h := range hours {
		// 今天的锚点
		t := time.Date(now.Year(), now.Month(), now.Day(), h, 0, 0, 0, now.Location())
		if t.After(now) {
			candidates = append(candidates, t)
		}
		// 明天的锚点（保证一定有未来时间点）
		candidates = append(candidates, t.Add(24*time.Hour))
	}
	// 取最近的一个
	earliest := candidates[0]
	for _, t := range candidates[1:] {
		if t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

// runZhipuAnchor 对所有启用的智谱 key 发送心跳请求，锚定新窗口
func runZhipuAnchor() {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[anchor] 获取数据库连接失败: %v", err)
		return
	}

	rows, err := conn.Query(`SELECT k.id, k.name, k.api_key, v.base_url
		FROM sys_api_key k
		JOIN sys_vendor v ON k.vendor_id = v.id
		WHERE v.code = 'zhipu' AND k.enabled = 1 AND k.api_key != ''`)
	if err != nil {
		log.Printf("[anchor] 查询智谱 key 失败: %v", err)
		return
	}
	defer rows.Close()

	type keyRow struct {
		id, name, apiKey, baseURL string
	}
	var keys []keyRow
	for rows.Next() {
		var kr keyRow
		var baseURL *string
		if err := rows.Scan(&kr.id, &kr.name, &kr.apiKey, &baseURL); err != nil {
			continue
		}
		if baseURL != nil {
			kr.baseURL = *baseURL
		}
		keys = append(keys, kr)
	}

	if len(keys) == 0 {
		log.Printf("[anchor] 未找到启用的智谱 key，跳过锚定")
		return
	}

	log.Printf("[anchor] 开始锚定，共 %d 个智谱 key", len(keys))
	for _, kr := range keys {
		sendZhipuHeartbeat(kr.id, kr.name, kr.apiKey, kr.baseURL)
	}

	// 锚定后立即触发一次额度检查，把新窗口的 next_reset 采集下来（便于观察）
	go func() {
		time.Sleep(5 * time.Second)
		runQuotaCheck()
	}()
}

// sendZhipuHeartbeat 给单个智谱 key 发送一条最小心跳请求，触发窗口锚定
// 走 coding plan 地址（与蒸馏/巡检一致），消耗极少 token
func sendZhipuHeartbeat(keyID, keyName, apiKey, baseURL string) {
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	apiURL := strings.TrimRight(baseURL, "/") + "/chat/completions"

	body := map[string]interface{}{
		"model":      "glm-4.7",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyJSON)))
	if err != nil {
		log.Printf("[anchor] key_id=%s(%s) 构造请求失败: %v", keyID, keyName, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[anchor] key_id=%s(%s) 心跳请求失败: %v", keyID, keyName, err)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		detail := string(respBody)
		if len(detail) > 300 {
			detail = detail[:300]
		}
		log.Printf("[anchor] key_id=%s(%s) 心跳返回 %d: %s", keyID, keyName, resp.StatusCode, detail)
		return
	}

	log.Printf("[anchor] key_id=%s(%s) 窗口锚定成功 ✓（新窗口: 现在 → 5 小时后）", keyID, keyName)
}
