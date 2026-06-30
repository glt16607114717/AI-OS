package service

import (
	"ai-os-server/model"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ── LLM 统计 ──

func RecordStat(stat *model.LLMStat) {
	conn, err := GetDB()
	if err != nil {
		return
	}
	success := 0
	if stat.Success {
		success = 1
	}
	errMsg := stat.Error
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	conn.Exec(`INSERT INTO sys_llm_stats (ts, user_id, username, vendor_id, key_id, model_id,
		prompt_tokens, completion_tokens, total_tokens, latency_ms, success, error, session_id, msg_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Format("2006-01-02 15:04:05"),
		stat.UserID, stat.Username, stat.VendorID, stat.KeyID, stat.ModelID,
		stat.PromptTokens, stat.CompletionTokens, stat.TotalTokens,
		stat.LatencyMs, success, errMsg, stat.SessionID, stat.MsgID)
}

func CleanupStats() int {
	conn, err := GetDB()
	if err != nil {
		return 0
	}
	cutoff := time.Now().AddDate(0, 0, -90).Format("2006-01-02 15:04:05")
	res, err := conn.Exec("DELETE FROM sys_llm_stats WHERE ts < ?", cutoff)
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return int(n)
}

func GetStatsSummary(days int) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")

	// 总览
	var totalReqs, totalTokens, totalPrompt, totalCompletion, successCount, errorCount int
	var avgLatency float64
	if err := conn.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(prompt_tokens),0),
		COALESCE(SUM(completion_tokens),0), COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END),0),
		COALESCE(SUM(success),0), COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0)
		FROM sys_llm_stats WHERE ts >= ?`, cutoff).Scan(
		&totalReqs, &totalTokens, &totalPrompt, &totalCompletion, &avgLatency, &successCount, &errorCount); err != nil {
		log.Printf("[stat] 查询总览失败: %v", err)
		return nil, err
	}

	successRate := 0.0
	if totalReqs > 0 {
		successRate = float64(successCount) / float64(totalReqs)
	}

	// 按厂商
	byVendor := map[string]interface{}{}
	vRows, _ := conn.Query(`SELECT s.vendor_id, COALESCE(v.name, CONCAT('vendor_', s.vendor_id)) as name,
		COUNT(*) as requests,
		COALESCE(SUM(s.total_tokens),0) as tokens,
		COALESCE(SUM(s.prompt_tokens),0) as prompt_tokens,
		COALESCE(SUM(s.completion_tokens),0) as completion_tokens,
		COALESCE(AVG(CASE WHEN s.success=1 THEN s.latency_ms END),0) as avg_latency_ms,
		SUM(s.success) as success_count,
		SUM(CASE WHEN s.success=0 THEN 1 ELSE 0 END) as errors
		FROM sys_llm_stats s LEFT JOIN sys_vendor v ON s.vendor_id = v.id
		WHERE s.ts >= ? GROUP BY s.vendor_id`, cutoff)
	if vRows != nil {
		defer vRows.Close()
		for vRows.Next() {
			var vid, reqs, tokens, pt, ct, sc, errs int
			var name string
			var al float64
			if err := vRows.Scan(&vid, &name, &reqs, &tokens, &pt, &ct, &al, &sc, &errs); err != nil {
				log.Printf("[stats] scan vendor 失败: %v", err)
				continue
			}
			byVendor[fmt.Sprintf("%d", vid)] = map[string]interface{}{
				"name": name, "requests": reqs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	// 按模型
	byModel := map[string]interface{}{}
	mRows, _ := conn.Query(`SELECT model_id, COUNT(*),
		COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END),0),
		SUM(success), SUM(CASE WHEN success=0 THEN 1 ELSE 0 END)
		FROM sys_llm_stats WHERE ts >= ? GROUP BY model_id`, cutoff)
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var mid string
			var reqs, tokens, pt, ct, sc, errs int
			var al float64
			mRows.Scan(&mid, &reqs, &tokens, &pt, &ct, &al, &sc, &errs)
			byModel[mid] = map[string]interface{}{
				"requests": reqs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	// 按天
	daily := map[string]interface{}{}
	dRows, _ := conn.Query(`SELECT DATE_FORMAT(ts, '%Y-%m-%d') as day, COUNT(*),
		COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		SUM(success), SUM(CASE WHEN success=0 THEN 1 ELSE 0 END)
		FROM sys_llm_stats WHERE ts >= ? GROUP BY DATE_FORMAT(ts, '%Y-%m-%d') ORDER BY day DESC`, cutoff)
	if dRows != nil {
		defer dRows.Close()
		for dRows.Next() {
			var day string
			var reqs, tokens, pt, ct, sc, errs int
			dRows.Scan(&day, &reqs, &tokens, &pt, &ct, &sc, &errs)
			daily[day] = map[string]interface{}{
				"requests": reqs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"success_count": sc, "errors": errs,
			}
		}
	}

	// 按用户（只按 user_id 分组，JOIN sys_user 取最新用户名，避免改名后数据分裂）
	byUser := map[string]interface{}{}
	uRows, _ := conn.Query(`SELECT s.user_id, COALESCE(u.username, CONCAT('user_', s.user_id)) as username,
		COUNT(*) as requests,
		COALESCE(SUM(s.total_tokens),0) as tokens,
		COALESCE(SUM(s.prompt_tokens),0) as prompt_tokens,
		COALESCE(SUM(s.completion_tokens),0) as completion_tokens,
		COALESCE(AVG(CASE WHEN s.success=1 THEN s.latency_ms END),0) as avg_latency_ms,
		SUM(s.success) as success_count,
		SUM(CASE WHEN s.success=0 THEN 1 ELSE 0 END) as errors
		FROM sys_llm_stats s LEFT JOIN sys_user u ON s.user_id = u.id
		WHERE s.ts >= ? GROUP BY s.user_id ORDER BY requests DESC`, cutoff)
	if uRows != nil {
		defer uRows.Close()
		for uRows.Next() {
			var uid, reqs, tokens, pt, ct, sc, errs int
			var name string
			var al float64
			if err := uRows.Scan(&uid, &name, &reqs, &tokens, &pt, &ct, &al, &sc, &errs); err != nil {
				continue
			}
			byUser[fmt.Sprintf("%d", uid)] = map[string]interface{}{
				"username": name, "requests": reqs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	// 按 API Key
	byKey := map[string]interface{}{}
	kRows, _ := conn.Query(`SELECT COALESCE(s.key_id, ''), COALESCE(k.name, 'unknown'),
		COUNT(*) as requests, COALESCE(SUM(s.total_tokens),0) as tokens,
		SUM(s.success) as success_count,
		SUM(CASE WHEN s.success=0 THEN 1 ELSE 0 END) as errors
		FROM sys_llm_stats s LEFT JOIN sys_api_key k ON s.key_id = k.id
		WHERE s.ts >= ? AND s.key_id != '' GROUP BY s.key_id ORDER BY requests DESC`, cutoff)
	if kRows != nil {
		defer kRows.Close()
		for kRows.Next() {
			var kid, name string
			var reqs, tokens, sc, errs int
			kRows.Scan(&kid, &name, &reqs, &tokens, &sc, &errs)
			byKey[kid] = map[string]interface{}{
				"name": name, "requests": reqs, "tokens": tokens,
				"success_count": sc, "errors": errs,
			}
		}
	}

	// 图片识别按用户统计（只按 user_id 分组，JOIN sys_user 取最新用户名）
	visionByUser := map[string]interface{}{}
	vRows2, _ := conn.Query(`SELECT v.user_id, COALESCE(u.username, CONCAT('user_', v.user_id)),
		COUNT(*) as count, SUM(v.success) as success_count,
		SUM(CASE WHEN v.success=0 THEN 1 ELSE 0 END) as fail_count
		FROM sys_vision_log v LEFT JOIN sys_user u ON v.user_id = u.id
		WHERE v.created_at >= ? GROUP BY v.user_id ORDER BY count DESC`, cutoff)
	if vRows2 != nil {
		defer vRows2.Close()
		for vRows2.Next() {
			var uid, count, sc, fc int
			var name string
			if err := vRows2.Scan(&uid, &name, &count, &sc, &fc); err != nil {
				continue
			}
			visionByUser[fmt.Sprintf("%d", uid)] = map[string]interface{}{
				"username": name, "count": count, "success_count": sc, "fail_count": fc,
			}
		}
	}

	return map[string]interface{}{
		"total_requests": totalReqs,
		"total_tokens": totalTokens,
		"total_prompt_tokens": totalPrompt, "total_completion_tokens": totalCompletion,
		"total_errors": errorCount,
		"avg_latency_ms": int(avgLatency), "success_rate": successRate,
		"by_vendor": byVendor, "by_model": byModel, "daily": daily,
		"by_user": byUser, "by_key": byKey, "vision_by_user": visionByUser,
	}, nil
}

func GetRecentErrors(limit int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -7).Format("2006-01-02 15:04:05")
	rows, err := conn.Query(`SELECT s.ts, COALESCE(v.name, CONCAT('vendor_', s.vendor_id)),
		s.model_id, s.latency_ms, s.error
		FROM sys_llm_stats s LEFT JOIN sys_vendor v ON s.vendor_id = v.id
		WHERE s.ts >= ? AND s.success = 0 ORDER BY s.ts DESC LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var ts, vendorName, modelID, errMsg string
		var latency int
		if err := rows.Scan(&ts, &vendorName, &modelID, &latency, &errMsg); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"ts": ts, "vendor_name": vendorName,
			"model_id": modelID, "latency_ms": latency, "error": errMsg,
		})
	}
	return result, nil
}

// ── LLM 日志（MySQL） ──

func EnsureLogTable() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_llm_log (
		id INT AUTO_INCREMENT PRIMARY KEY,
		ts DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		category VARCHAR(50) NOT NULL,
		level VARCHAR(20) NOT NULL DEFAULT 'info',
		message TEXT NOT NULL,
		detail TEXT,
		user_id INT NOT NULL DEFAULT 0,
		INDEX idx_ts (ts DESC),
		INDEX idx_user (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func WriteLog(category, message, level, detail string, userID int, sessionID, msgID string) {
	conn, err := GetDB()
	if err != nil {
		return
	}
	if level == "" {
		level = "info"
	}
	conn.Exec("INSERT INTO sys_llm_log (category, level, message, detail, user_id, session_id, msg_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
		category, level, message, detail, userID, sessionID, msgID)

	// 自动清理超过 1000 条
	conn.Exec("DELETE FROM sys_llm_log WHERE id IN (SELECT id FROM (SELECT id FROM sys_llm_log ORDER BY ts ASC LIMIT 999) t) AND (SELECT COUNT(*) FROM sys_llm_log) > 1000")
}

func GetLogs(limit int, category string, afterID int, userID int, isAdmin bool) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}

	var rows *sql.Rows
	if isAdmin {
		// 管理员看全部
		if afterID > 0 && category != "" {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? AND category = ? ORDER BY id DESC LIMIT ?", afterID, category, limit)
		} else if afterID > 0 {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? ORDER BY id DESC LIMIT ?", afterID, limit)
		} else if category != "" {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE category = ? ORDER BY id DESC LIMIT ?", category, limit)
		} else {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log ORDER BY id DESC LIMIT ?", limit)
		}
	} else {
		// 普通用户只看自己
		if afterID > 0 && category != "" {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? AND category = ? AND user_id = ? ORDER BY id DESC LIMIT ?", afterID, category, userID, limit)
		} else if afterID > 0 {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? AND user_id = ? ORDER BY id DESC LIMIT ?", afterID, userID, limit)
		} else if category != "" {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE category = ? AND user_id = ? ORDER BY id DESC LIMIT ?", category, userID, limit)
		} else {
			rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE user_id = ? ORDER BY id DESC LIMIT ?", userID, limit)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var ts, cat, lvl, msg, det string
		if err := rows.Scan(&id, &ts, &cat, &lvl, &msg, &det); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "ts": ts, "category": cat,
			"level": lvl, "message": msg, "detail": det,
		})
	}

	var maxID int
	conn.QueryRow("SELECT COALESCE(MAX(id), 0) FROM sys_llm_log").Scan(&maxID)
	return result, maxID, nil
}

func ClearLogs() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	if _, err := conn.Exec("TRUNCATE TABLE sys_llm_log"); err != nil {
		log.Printf("[log] TRUNCATE sys_llm_log 失败: %v", err)
	}
}

// ── 聊天历史（MySQL） ──

func EnsureChatTable() {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_chat_history (
		id INT AUTO_INCREMENT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_id INT NOT NULL DEFAULT 0,
		role VARCHAR(20) NOT NULL,
		content TEXT NOT NULL,
		INDEX idx_created (created_at),
		INDEX idx_user_date (user_id, created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func AddChatMessage(userID int, role, content, sessionID, msgID string) int64 {
	conn, err := GetDB()
	if err != nil {
		return 0
	}
	res, err := conn.Exec("INSERT INTO sys_chat_history (user_id, role, content, session_id, msg_id) VALUES (?, ?, ?, ?, ?)", userID, role, content, sessionID, msgID)
	if err != nil {
		return 0
	}
	// 清理超过 10000 条
	conn.Exec("DELETE FROM sys_chat_history WHERE id IN (SELECT id FROM (SELECT id FROM sys_chat_history ORDER BY created_at ASC LIMIT 9999) t) AND (SELECT COUNT(*) FROM sys_chat_history) > 10000")
	id, _ := res.LastInsertId()
	return id
}

const CONVERSATION_LOG_DIR = "logs/conversations"

// SaveConversationLog 将请求+响应保存为JSON文件（供下载），异步执行
func SaveConversationLog(id int64, req map[string]interface{}, responseContent, modelID string,
	vendorID, promptTokens, completionTokens, totalTokens, latencyMs int) {
	SaveConversationLogWithUser(id, 0, "", req, responseContent, modelID,
		vendorID, promptTokens, completionTokens, totalTokens, latencyMs)
}

// SaveConversationLogWithUser 带用户信息的对话日志保存
func SaveConversationLogWithUser(id int64, userID int, username string, req map[string]interface{}, responseContent, modelID string,
	vendorID, promptTokens, completionTokens, totalTokens, latencyMs int) {
	go func() {
		os.MkdirAll(CONVERSATION_LOG_DIR, 0755)

		logData := map[string]interface{}{
			"id":        id,
			"user_id":   userID,
			"username":  username,
			"timestamp": time.Now().Format("2006-01-02T15:04:05Z07:00"),
			"request":   req,
			"response": map[string]interface{}{
				"content":   responseContent,
				"model":     modelID,
				"vendor_id": vendorID,
				"tokens": map[string]interface{}{
					"prompt":     promptTokens,
					"completion": completionTokens,
					"total":      totalTokens,
				},
				"latency_ms": latencyMs,
			},
		}

		data, err := json.MarshalIndent(logData, "", "  ")
		if err != nil {
			return
		}
		filePath := filepath.Join(CONVERSATION_LOG_DIR, fmt.Sprintf("%d.json", id))
		os.WriteFile(filePath, data, 0644)
	}()
}

// GetConversationLogPath 返回对话日志文件路径，文件不存在返回空
func GetConversationLogPath(id int) string {
	filePath := filepath.Join(CONVERSATION_LOG_DIR, fmt.Sprintf("%d.json", id))
	if _, err := os.Stat(filePath); err != nil {
		return ""
	}
	return filePath
}

// CleanupOldConversationLogs 清理7天前的对话日志文件
func CleanupOldConversationLogs() {
	files, err := filepath.Glob(filepath.Join(CONVERSATION_LOG_DIR, "*.json"))
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(f)
		}
	}
}

func GetChatHistory(limit int) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query("SELECT id, created_at, role, content FROM sys_chat_history ORDER BY created_at ASC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var createdAt, role, content string
		if err := rows.Scan(&id, &createdAt, &role, &content); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id": id, "created_at": createdAt, "role": role, "content": content,
		})
	}
	return result, nil
}

func ClearChatHistory() {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[chat] DB连接失败: %v", err)
		return
	}
	if _, err := conn.Exec("TRUNCATE TABLE sys_chat_history"); err != nil {
		log.Printf("[chat] TRUNCATE 失败: %v", err)
	}
}

func GetChatMaxID() int {
	conn, err := GetDB()
	if err != nil {
		return 0
	}
	var maxID int
	if err := conn.QueryRow("SELECT COALESCE(MAX(id), 0) FROM sys_chat_history").Scan(&maxID); err != nil {
		log.Printf("[chat] 查询 maxID 失败: %v", err)
	}
	return maxID
}

// ── 错误日志查询 ──

// GetErrorLog 分页查询错误记录
func GetErrorLog(page, pageSize int, modelFilter, userFilter string) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}

	// 构建条件
	where := "success = 0"
	args := []interface{}{}
	if modelFilter != "" {
		where += " AND model_id = ?"
		args = append(args, modelFilter)
	}
	if userFilter != "" {
		// 先查 user_id，避免改名后旧用户名匹配不到
		var uid int
		if err := conn.QueryRow("SELECT id FROM sys_user WHERE username = ?", userFilter).Scan(&uid); err == nil && uid > 0 {
			where += " AND user_id = ?"
			args = append(args, uid)
		} else {
			// 查不到用户，返回空
			return []map[string]interface{}{}, 0, nil
		}
	}

	// 查总数
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	conn.QueryRow("SELECT COUNT(*) FROM sys_llm_stats WHERE "+where, countArgs...).Scan(&total)

	// 分页查询
	offset := (page - 1) * pageSize
	queryArgs := append(args, pageSize, offset)
	rows, err := conn.Query(
		"SELECT s.id, s.ts, s.user_id, s.username, s.vendor_id, s.key_id, s.model_id, s.error, s.latency_ms, IFNULL(k.name,'') FROM sys_llm_stats s LEFT JOIN sys_api_key k ON k.id = s.key_id WHERE "+
			where+" ORDER BY s.ts DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]interface{}{}
	for rows.Next() {
		var id, userID, vendorID, latencyMs int
		var ts, username, keyID, modelID, errMsg, keyName string
		if err := rows.Scan(&id, &ts, &userID, &username, &vendorID, &keyID, &modelID, &errMsg, &latencyMs, &keyName); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":       id,
			"ts":       ts,
			"user_id":  userID,
			"username": username,
			"vendor_id": vendorID,
			"key_id":   keyID,
			"key_name": keyName,
			"model_id": modelID,
			"error":    errMsg,
			"latency_ms": latencyMs,
		})
	}
	return result, total, nil
}

// classifyError 根据错误内容分类
func classifyError(errMsg string) string {
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "429") || strings.Contains(lower, "限流") || strings.Contains(lower, "rate limit") {
		return "限流(429)"
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "超时") || strings.Contains(lower, "deadline exceeded") {
		return "超时"
	}
	if strings.Contains(lower, "sse") || strings.Contains(lower, "流") {
		return "SSE流中断"
	}
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "dial tcp") || strings.Contains(lower, "network") {
		return "网络错误"
	}
	if strings.Contains(lower, "500") || strings.Contains(lower, "502") || strings.Contains(lower, "503") {
		return "服务器错误(5xx)"
	}
	if strings.Contains(lower, "400") || strings.Contains(lower, "401") || strings.Contains(lower, "403") {
		return "客户端错误(4xx)"
	}
	return "其他"
}

// GetErrorStats 错误聚合统计
func GetErrorStats(days int) (map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")

	// 1. 错误总数
	var totalErrors int
	conn.QueryRow("SELECT COUNT(*) FROM sys_llm_stats WHERE success=0 AND ts >= ?", cutoff).Scan(&totalErrors)

	// 2. 按错误类型分组
	byType := []map[string]interface{}{}
	typeRows, _ := conn.Query(`
		SELECT model_id, COUNT(*) as cnt, MAX(ts) as last_ts
		FROM sys_llm_stats WHERE success=0 AND ts >= ?
		GROUP BY model_id ORDER BY cnt DESC`, cutoff)
	if typeRows != nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var modelID string
			var cnt int
			var lastTs string
			if err := typeRows.Scan(&modelID, &cnt, &lastTs); err != nil {
				continue
			}
			byType = append(byType, map[string]interface{}{
				"model_id": modelID, "count": cnt, "last_ts": lastTs,
			})
		}
	}

	// 3. 按具体错误内容分组（Top 错误类型）
	byError := []map[string]interface{}{}
	errRows, _ := conn.Query(`
		SELECT error, COUNT(*) as cnt, MAX(ts) as last_ts,
			(SELECT model_id FROM sys_llm_stats s2 WHERE s2.error = s1.error AND s2.success=0 AND s2.ts >= ? ORDER BY s2.ts DESC LIMIT 1) as model_id
		FROM sys_llm_stats s1 WHERE success=0 AND ts >= ?
		GROUP BY error ORDER BY cnt DESC LIMIT 20`, cutoff, cutoff)
	if errRows != nil {
		for errRows.Next() {
			var errMsg, modelID string
			var cnt int
			var lastTs string
			if err := errRows.Scan(&errMsg, &cnt, &lastTs, &modelID); err != nil {
				continue
			}
			byError = append(byError, map[string]interface{}{
				"error": errMsg, "count": cnt, "last_ts": lastTs, "model_id": modelID,
			})
		}
		errRows.Close()
	}

	// 4. 按错误分类分组（需要遍历）
	allRows, _ := conn.Query("SELECT error FROM sys_llm_stats WHERE success=0 AND ts >= ?", cutoff)
	typeCount := map[string]int{}
	if allRows != nil {
		for allRows.Next() {
			var errMsg string
			if err := allRows.Scan(&errMsg); err != nil {
				continue
			}
			typeCount[classifyError(errMsg)]++
		}
		allRows.Close()
	}
	byCategory := []map[string]interface{}{}
	for t, c := range typeCount {
		byCategory = append(byCategory, map[string]interface{}{
			"type": t, "count": c,
		})
	}

	// 5. 按天分组（近7天趋势）
	byDay := []map[string]interface{}{}
	dayRows, _ := conn.Query(`
		SELECT DATE(ts) as day, COUNT(*) as cnt
		FROM sys_llm_stats WHERE success=0 AND ts >= ?
		GROUP BY DATE(ts) ORDER BY day`, cutoff)
	if dayRows != nil {
		for dayRows.Next() {
			var day string
			var cnt int
			if err := dayRows.Scan(&day, &cnt); err != nil {
				continue
			}
			byDay = append(byDay, map[string]interface{}{
				"date": day, "count": cnt,
			})
		}
		dayRows.Close()
	}

	return map[string]interface{}{
		"total":       totalErrors,
		"by_model":    byType,
		"by_error":    byError,
		"by_category": byCategory,
		"by_day":      byDay,
	}, nil
}

// ── 按消息维度聚合的对话记录 ──

// ChatSessionRow 一条 msg_id 聚合后的对话记录
type ChatSessionRow struct {
	MsgID            string            `json:"msg_id"`
	SessionID        string            `json:"session_id"`
	Username         string            `json:"username"`
	FirstTs          string            `json:"first_ts"`
	TotalPrompt      int               `json:"total_prompt"`
	TotalCompletion  int               `json:"total_completion"`
	TotalTokens      int               `json:"total_tokens"`
	LatencySec       int               `json:"latency_sec"`
	RequestCount     int               `json:"request_count"`
	ErrorCount       int               `json:"error_count"`
	Models           []string          `json:"models"`
	UserMessage      string            `json:"user_message"`
	RequestChain     []RequestChainItem `json:"request_chain"`
}

// RequestChainItem 请求链路中的单次请求
type RequestChainItem struct {
	Ts          string `json:"ts"`
	ModelID     string `json:"model_id"`
	KeyName     string `json:"key_name"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	LatencyMs   int    `json:"latency_ms"`
	TotalTokens int    `json:"total_tokens"`
}

// GetChatSessions 按 msg_id 聚合查询对话记录
func GetChatSessions(limit int, userID int, isAdmin bool) ([]ChatSessionRow, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 1. 按 msg_id 聚合 stats
	baseWhere := "msg_id != ''"
	args := []interface{}{}
	if !isAdmin {
		baseWhere += " AND user_id = ?"
		args = append(args, userID)
	}

	aggSQL := `SELECT msg_id, MIN(session_id), MIN(username), MAX(ts),
		SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens),
		COUNT(*), SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),
		TIMESTAMPDIFF(SECOND, MIN(ts), MAX(ts))
		FROM sys_llm_stats WHERE ` + baseWhere + `
		GROUP BY msg_id ORDER BY MAX(ts) DESC LIMIT ?`
	args = append(args, limit)

	rows, err := conn.Query(aggSQL, args...)
	if err != nil {
		return nil, err
	}

	var sessions []ChatSessionRow
	msgIDs := make([]string, 0)
	for rows.Next() {
		var s ChatSessionRow
		var errCount int
		var latencySec sql.NullInt64
		if err := rows.Scan(&s.MsgID, &s.SessionID, &s.Username, &s.FirstTs,
			&s.TotalPrompt, &s.TotalCompletion, &s.TotalTokens,
			&s.RequestCount, &errCount, &latencySec); err != nil {
			continue
		}
		s.ErrorCount = errCount
		s.LatencySec = int(latencySec.Int64)
		sessions = append(sessions, s)
		msgIDs = append(msgIDs, s.MsgID)
	}
	rows.Close()

	if len(sessions) == 0 {
		return sessions, nil
	}

	// 2. 批量取用户问题（每个 msg_id 的第一条 user 消息）
	userMsgMap := batchGetFirstUserMessages(conn, msgIDs)

	// 3. 批量取请求链路 + 模型列表
	chainMap, modelMap := batchGetRequestChains(conn, msgIDs)

	// 4. 填充到 sessions
	for i := range sessions {
		sessions[i].UserMessage = userMsgMap[sessions[i].MsgID]
		sessions[i].RequestChain = chainMap[sessions[i].MsgID]
		sessions[i].Models = modelMap[sessions[i].MsgID]
	}

	return sessions, nil
}

// batchGetFirstUserMessages 批量获取每个 msg_id 的第一条 user 消息
func batchGetFirstUserMessages(conn *sql.DB, msgIDs []string) map[string]string {
	result := map[string]string{}
	if len(msgIDs) == 0 {
		return result
	}
	placeholders := strings.Repeat("?,", len(msgIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(msgIDs))
	for i, id := range msgIDs {
		args[i] = id
	}
	// 取每个 msg_id 的第一条 user 消息（用子查询 MIN(id)）
	query := `SELECT msg_id, content FROM sys_chat_history 
		WHERE role='user' AND msg_id IN (` + placeholders + `)
		AND id IN (SELECT MIN(id) FROM sys_chat_history WHERE role='user' AND msg_id IN (` + placeholders + `) GROUP BY msg_id)`
	// 双倍 args（两个 IN 子句）
	doubleArgs := append(args, args...)
	rows, err := conn.Query(query, doubleArgs...)
	if err != nil {
		log.Printf("[chat-sessions] 批量取用户消息失败: %v", err)
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var msgID, content string
		if err := rows.Scan(&msgID, &content); err != nil {
			continue
		}
		result[msgID] = content
	}
	return result
}

// batchGetRequestChains 批量获取请求链路和模型列表
func batchGetRequestChains(conn *sql.DB, msgIDs []string) (map[string][]RequestChainItem, map[string][]string) {
	chainResult := map[string][]RequestChainItem{}
	modelResult := map[string][]string{}
	if len(msgIDs) == 0 {
		return chainResult, modelResult
	}
	placeholders := strings.Repeat("?,", len(msgIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(msgIDs))
	for i, id := range msgIDs {
		args[i] = id
	}
	query := `SELECT s.msg_id, s.ts, s.model_id, COALESCE(k.name, s.key_id), s.success, s.error, s.latency_ms, s.total_tokens
		FROM sys_llm_stats s LEFT JOIN sys_api_key k ON s.key_id = k.id
		WHERE s.msg_id IN (` + placeholders + `) ORDER BY s.msg_id, s.ts DESC`
	rows, err := conn.Query(query, args...)
	if err != nil {
		log.Printf("[chat-sessions] 批量取请求链路失败: %v", err)
		return chainResult, modelResult
	}
	defer rows.Close()

	modelSet := map[string]map[string]bool{} // msg_id -> model set
	for rows.Next() {
		var msgID, ts, modelID, errMsg string
		var success bool
		var latencyMs, totalTokens int
		var successInt int
		var keyName string
		if err := rows.Scan(&msgID, &ts, &modelID, &keyName, &successInt, &errMsg, &latencyMs, &totalTokens); err != nil {
			continue
		}
		success = successInt == 1
		item := RequestChainItem{
			Ts: ts, ModelID: modelID, KeyName: keyName, Success: success,
			Error: errMsg, LatencyMs: latencyMs, TotalTokens: totalTokens,
		}
		chainResult[msgID] = append(chainResult[msgID], item)

		if modelSet[msgID] == nil {
			modelSet[msgID] = map[string]bool{}
		}
		if !modelSet[msgID][modelID] {
			modelSet[msgID][modelID] = true
			modelResult[msgID] = append(modelResult[msgID], modelID)
		}
	}
	return chainResult, modelResult
}
