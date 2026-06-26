package circuit

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── 熔断器（独立进程，内存 map）──

// BreakerKey 熔断维度：模型ID + API Key
type BreakerKey struct {
	ModelID string
	KeyID   string
}

// BreakerState 熔断状态
type BreakerState struct {
	OpenedAt time.Time // 熔断开始时间
	Reason   string    // 熔断原因（最近的错误信息）
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	mu     sync.RWMutex
	states map[BreakerKey]*BreakerState
	dbFunc func() (*sql.DB, error) // 数据库连接工厂
}

// 全局单例
var globalBreaker *CircuitBreaker

// Init 初始化熔断器，返回全局实例
func Init(dbFunc func() (*sql.DB, error)) *CircuitBreaker {
	globalBreaker = &CircuitBreaker{
		states: make(map[BreakerKey]*BreakerState),
		dbFunc: dbFunc,
	}
	return globalBreaker
}

// GetBreaker 获取全局熔断器实例
func GetBreaker() *CircuitBreaker {
	return globalBreaker
}

// IsOpen 检查某个 "模型+Key" 是否被熔断
func (cb *CircuitBreaker) IsOpen(modelID, keyID string) bool {
	if cb == nil {
		return false
	}
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	_, ok := cb.states[BreakerKey{ModelID: modelID, KeyID: keyID}]
	return ok
}

// GetOpenKeys 获取所有被熔断的 key 列表（供管理后台使用）
func (cb *CircuitBreaker) GetOpenKeys() []map[string]interface{} {
	if cb == nil {
		return nil
	}
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(cb.states))
	for k, v := range cb.states {
		result = append(result, map[string]interface{}{
			"model_id":  k.ModelID,
			"key_id":    k.KeyID,
			"opened_at": v.OpenedAt.Format("2006-01-02 15:04:05"),
			"reason":    v.Reason,
		})
	}
	return result
}

// ── 定时检测 ──

const (
	checkInterval   = 1 * time.Minute  // 检测频率
	windowDuration  = 5 * time.Minute  // 统计窗口
	cooldownPeriod  = 5 * time.Minute  // 冷却时间
	timeoutThreshold = 30000           // 超时阈值（毫秒）
	minSampleCount  = 3               // 最少样本数
)

// StartMonitor 启动熔断检测循环（独立 goroutine）
func (cb *CircuitBreaker) StartMonitor() {
	log.Println("[circuit] 熔断检测已启动（每1分钟扫库，5分钟窗口，3次全超时触发，5分钟冷却）")
	ticker := time.NewTicker(checkInterval)
	for range ticker.C {
		cb.checkAndUpdate()
	}
}

// checkAndUpdate 扫描数据库，更新熔断状态
func (cb *CircuitBreaker) checkAndUpdate() {
	if cb.dbFunc == nil {
		return
	}
	conn, err := cb.dbFunc()
	if err != nil {
		log.Printf("[circuit] 数据库连接失败: %v", err)
		return
	}
	// 注意：GetDB() 返回全局单例连接池，不能 Close

	now := time.Now()
	windowStart := now.Add(-windowDuration).Format("2006-01-02 15:04:05")

	// 1. 查询最近 5 分钟内所有 success=0 的记录，按 model_id + key_id 分组
	rows, err := conn.Query(`
		SELECT model_id, key_id, COUNT(*) as total,
		       SUM(CASE WHEN latency_ms > ? THEN 1 ELSE 0 END) as timeout_cnt,
		       MAX(error) as last_error
		FROM sys_llm_stats
		WHERE ts >= ? AND success = 0 AND error != ''
		GROUP BY model_id, key_id
		HAVING total >= ? AND timeout_cnt = total
	`, timeoutThreshold, windowStart, minSampleCount)
	if err != nil {
		log.Printf("[circuit] 查询失败: %v", err)
		return
	}
	defer rows.Close()

	// 2. 标记需要熔断的组合
	toOpen := make(map[BreakerKey]string)
	for rows.Next() {
		var modelID, keyID, lastError string
		var total, timeoutCnt int
		if err := rows.Scan(&modelID, &keyID, &total, &timeoutCnt, &lastError); err != nil {
			continue
		}
		key := BreakerKey{ModelID: modelID, KeyID: keyID}
		toOpen[key] = lastError
	}

	cb.mu.Lock()
	// 3. 新增熔断
	for key, reason := range toOpen {
		if _, exists := cb.states[key]; !exists {
			cb.states[key] = &BreakerState{
				OpenedAt: now,
				Reason:   reason,
			}
			log.Printf("[circuit] 熔断 %s/%s: %s", key.ModelID, key.KeyID, truncate(reason, 100))
		}
	}

	// 4. 检查冷却期满的，发送试探请求
	for key, state := range cb.states {
		if now.Sub(state.OpenedAt) >= cooldownPeriod {
			// 半开：发送试探请求
			log.Printf("[circuit] 试探 %s/%s...", key.ModelID, key.KeyID)
			if cb.probeHealth(key) {
				delete(cb.states, key)
				log.Printf("[circuit] 恢复 %s/%s", key.ModelID, key.KeyID)
			} else {
				// 试探失败，重置冷却计时
				state.OpenedAt = now
				log.Printf("[circuit] 试探失败 %s/%s，重新计时5分钟", key.ModelID, key.KeyID)
			}
		}
	}
	cb.mu.Unlock()
}

// probeHealth 发送试探请求，验证模型是否恢复
func (cb *CircuitBreaker) probeHealth(key BreakerKey) bool {
	if cb.dbFunc == nil {
		return false
	}
	conn, err := cb.dbFunc()
	if err != nil {
		return false
	}
	// 注意：GetDB() 返回全局单例连接池，不能 Close

	// 查询 API Key 和 BaseURL
	var apiKey, baseURL string
	err = conn.QueryRow(`
		SELECT k.api_key, v.base_url
		FROM sys_api_key k
		JOIN sys_vendor v ON v.id = k.vendor_id
		WHERE k.id = ?
	`, key.KeyID).Scan(&apiKey, &baseURL)
	if err != nil {
		log.Printf("[circuit] 查询 key 信息失败 %s: %v", key.KeyID, err)
		return false
	}

	// 构造试探请求（不带上下文，纯数学题）
	testPrompt := []map[string]interface{}{
		{
			"role":    "user",
			"content": "鸡兔同笼，头35个，脚94只，问鸡兔各几只？只输出答案，不要解释。",
		},
	}
	body := map[string]interface{}{
		"model":    key.ModelID,
		"messages": testPrompt,
		"stream":   false,
	}
	bodyJSON, _ := json.Marshal(body)

	httpReq, _ := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/chat/completions", strings.NewReader(string(bodyJSON)))
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[circuit] 试探请求失败 %s/%s: %v", key.ModelID, key.KeyID, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[circuit] 试探 HTTP %d %s/%s: %s", resp.StatusCode, key.ModelID, key.KeyID, truncate(string(respBody), 100))
		return false
	}

	// 解析响应，检查是否包含正确答案
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false
	}
	choices, _ := result["choices"].([]interface{})
	if len(choices) == 0 {
		return false
	}
	choice, _ := choices[0].(map[string]interface{})
	msg, _ := choice["message"].(map[string]interface{})
	content, _ := msg["content"].(string)

	// 验证答案包含关键数字
	hasAnswer := strings.Contains(content, "23") && strings.Contains(content, "12")
	log.Printf("[circuit] 试探 %s/%s: 响应=%s, 通过=%v", key.ModelID, key.KeyID, truncate(content, 80), hasAnswer)
	return hasAnswer
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
