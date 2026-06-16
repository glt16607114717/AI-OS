package service

import (
	"ai-os-server/model"
	"database/sql"
	"fmt"
	"log"
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
	conn.Exec(`INSERT INTO sys_llm_stats (ts, user_id, username, vendor_id, model_id,
		prompt_tokens, completion_tokens, total_tokens, latency_ms, success, error, conversation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Format("2006-01-02 15:04:05"),
		stat.UserID, stat.Username, stat.VendorID, stat.ModelID,
		stat.PromptTokens, stat.CompletionTokens, stat.TotalTokens,
		stat.LatencyMs, success, errMsg, stat.ConversationID)
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
	var totalReqs, totalConvs, totalTokens, totalPrompt, totalCompletion, successCount int
	var avgLatency float64
	if err := conn.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT NULLIF(conversation_id, '')),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(prompt_tokens),0),
		COALESCE(SUM(completion_tokens),0), COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END),0),
		COALESCE(SUM(success),0)
		FROM sys_llm_stats WHERE ts >= ?`, cutoff).Scan(
		&totalReqs, &totalConvs, &totalTokens, &totalPrompt, &totalCompletion, &avgLatency, &successCount); err != nil {
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
		COUNT(*) as requests, COUNT(DISTINCT NULLIF(s.conversation_id, '')) as conversations,
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
			var vid, reqs, convs, tokens, pt, ct, sc, errs int
			var name string
			var al float64
			if err := vRows.Scan(&vid, &name, &reqs, &convs, &tokens, &pt, &ct, &al, &sc, &errs); err != nil {
				log.Printf("[stats] scan vendor 失败: %v", err)
				continue
			}
			byVendor[fmt.Sprintf("%d", vid)] = map[string]interface{}{
				"name": name, "requests": reqs, "conversations": convs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	// 按模型
	byModel := map[string]interface{}{}
	mRows, _ := conn.Query(`SELECT model_id, COUNT(*), COUNT(DISTINCT NULLIF(conversation_id, '')),
		COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END),0),
		SUM(success), SUM(CASE WHEN success=0 THEN 1 ELSE 0 END)
		FROM sys_llm_stats WHERE ts >= ? GROUP BY model_id`, cutoff)
	if mRows != nil {
		defer mRows.Close()
		for mRows.Next() {
			var mid string
			var reqs, convs, tokens, pt, ct, sc, errs int
			var al float64
			mRows.Scan(&mid, &reqs, &convs, &tokens, &pt, &ct, &al, &sc, &errs)
			byModel[mid] = map[string]interface{}{
				"requests": reqs, "conversations": convs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	// 按天
	daily := map[string]interface{}{}
	dRows, _ := conn.Query(`SELECT DATE(ts) as day, COUNT(*), COALESCE(SUM(total_tokens),0),
		COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		SUM(success), SUM(CASE WHEN success=0 THEN 1 ELSE 0 END)
		FROM sys_llm_stats WHERE ts >= ? GROUP BY DATE(ts) ORDER BY day DESC`, cutoff)
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

	// 按用户
	byUser := map[string]interface{}{}
	uRows, _ := conn.Query(`SELECT user_id, COALESCE(username, CONCAT('user_', user_id)) as username,
		COUNT(*) as requests, COUNT(DISTINCT NULLIF(conversation_id, '')) as conversations,
		COALESCE(SUM(total_tokens),0) as tokens,
		COALESCE(SUM(prompt_tokens),0) as prompt_tokens,
		COALESCE(SUM(completion_tokens),0) as completion_tokens,
		COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END),0) as avg_latency_ms,
		SUM(success) as success_count,
		SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
		FROM sys_llm_stats WHERE ts >= ? GROUP BY user_id, username ORDER BY requests DESC`, cutoff)
	if uRows != nil {
		defer uRows.Close()
		for uRows.Next() {
			var uid, reqs, convs, tokens, pt, ct, sc, errs int
			var name string
			var al float64
			if err := uRows.Scan(&uid, &name, &reqs, &convs, &tokens, &pt, &ct, &al, &sc, &errs); err != nil {
				continue
			}
			byUser[fmt.Sprintf("%d", uid)] = map[string]interface{}{
				"username": name, "requests": reqs, "conversations": convs, "tokens": tokens,
				"prompt_tokens": pt, "completion_tokens": ct,
				"avg_latency_ms": int(al), "success_count": sc, "errors": errs,
			}
		}
	}

	return map[string]interface{}{
		"total_requests": totalReqs, "total_conversations": totalConvs,
		"total_tokens": totalTokens,
		"total_prompt_tokens": totalPrompt, "total_completion_tokens": totalCompletion,
		"avg_latency_ms": int(avgLatency), "success_rate": successRate,
		"by_vendor": byVendor, "by_model": byModel, "daily": daily,
		"by_user": byUser,
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
		INDEX idx_ts (ts DESC)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func WriteLog(category, message, level, detail string) {
	conn, err := GetDB()
	if err != nil {
		return
	}
	if level == "" {
		level = "info"
	}
	conn.Exec("INSERT INTO sys_llm_log (category, level, message, detail) VALUES (?, ?, ?, ?)",
		category, level, message, detail)

	// 自动清理超过 1000 条
	conn.Exec("DELETE FROM sys_llm_log WHERE id IN (SELECT id FROM (SELECT id FROM sys_llm_log ORDER BY ts ASC LIMIT 999) t) AND (SELECT COUNT(*) FROM sys_llm_log) > 1000")
}

func GetLogs(limit int, category string, afterID int) ([]map[string]interface{}, int, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, 0, err
	}

	var rows *sql.Rows
	if afterID > 0 && category != "" {
		rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? AND category = ? ORDER BY id DESC LIMIT ?", afterID, category, limit)
	} else if afterID > 0 {
		rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE id > ? ORDER BY id DESC LIMIT ?", afterID, limit)
	} else if category != "" {
		rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log WHERE category = ? ORDER BY id DESC LIMIT ?", category, limit)
	} else {
		rows, err = conn.Query("SELECT id, ts, category, level, message, detail FROM sys_llm_log ORDER BY id DESC LIMIT ?", limit)
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
		role VARCHAR(20) NOT NULL,
		content TEXT NOT NULL,
		INDEX idx_created (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func AddChatMessage(role, content string) {
	conn, err := GetDB()
	if err != nil {
		return
	}
	conn.Exec("INSERT INTO sys_chat_history (role, content) VALUES (?, ?)", role, content)
	// 清理超过 10000 条
	conn.Exec("DELETE FROM sys_chat_history WHERE id IN (SELECT id FROM (SELECT id FROM sys_chat_history ORDER BY created_at ASC LIMIT 9999) t) AND (SELECT COUNT(*) FROM sys_chat_history) > 10000")
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
