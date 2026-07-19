package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Bug Bug 结构体（完全独立于 Requirement，不共用表/结构体）
type Bug struct {
	ID              int    `json:"id"`
	UserID          int    `json:"user_id"`
	Username        string `json:"username"`
	Title           string `json:"title"`
	Scenario        string `json:"scenario"`  // 触发场景：做了什么操作
	Expected        string `json:"expected"`  // 预期结果
	Actual          string `json:"actual"`    // 实际结果
	RawConversation string `json:"raw_conversation"`
	Module          string `json:"module"`
	Status          string `json:"status"`
	AdminNote       string `json:"admin_note"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// bugStatusFlow Bug 状态流转规则（含 withdrawn 撤销状态）
// withdrawn 是终态，任何阶段都能撤销（不物理删除，保留沟通证据）
var bugStatusFlow = map[string][]string{
	"submitted":   {"reviewing", "rejected", "withdrawn"},
	"reviewing":   {"accepted", "rejected", "withdrawn"},
	"accepted":    {"scheduled", "withdrawn"},
	"scheduled":   {"in_progress", "withdrawn"},
	"in_progress": {"done", "withdrawn"},
	"done":        {},
	"rejected":    {},
	"withdrawn":   {}, // 终态，不可再变更
}

// SubmitBug AI 提交 Bug
func SubmitBug(userID int, username string, title string, scenario string, expected string, actual string, module string, rawConversation string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	res, err := conn.Exec(`INSERT INTO sys_bug (user_id, username, title, scenario, expected, actual, raw_conversation, module, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'submitted')`,
		userID, username, title, scenario, expected, actual, rawConversation, module)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	log.Printf("[bug] 新 Bug #%d: %s (来自 %s)", id, title, username)
	return id, nil
}

// UpdateBug AI 修改已有 Bug（仅限自己提的，且在 submitted/reviewing 阶段可改）
func UpdateBug(bugID int, userID int, title string, scenario string, expected string, actual string, module string, rawConversation string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	var ownerID int
	var status string
	err = conn.QueryRow("SELECT user_id, status FROM sys_bug WHERE id = ?", bugID).Scan(&ownerID, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("Bug #%d 不存在", bugID)
		}
		return err
	}
	if ownerID != userID {
		return fmt.Errorf("只能修改自己提交的 Bug")
	}
	if status != "submitted" && status != "reviewing" {
		return fmt.Errorf("Bug 已进入「%s」阶段，不可再修改", status)
	}
	_, err = conn.Exec(`UPDATE sys_bug SET title=?, scenario=?, expected=?, actual=?, module=?, raw_conversation=?, updated_at=? WHERE id=?`,
		title, scenario, expected, actual, module, rawConversation, time.Now().Format("2006-01-02 15:04:05"), bugID)
	if err != nil {
		return err
	}
	log.Printf("[bug] Bug #%d 已更新: %s", bugID, title)
	return nil
}

// GetMyBugs 查询用户的 Bug 列表
func GetMyBugs(userID int) ([]Bug, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`SELECT id, user_id, username, title, scenario, expected, actual, raw_conversation, module, status, admin_note, created_at, updated_at
		FROM sys_bug WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBugs(rows)
}

// GetAllBugs 查询所有 Bug（支持 status 过滤）
func GetAllBugs(status string) ([]Bug, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if status != "" {
		rows, err = conn.Query(`SELECT id, user_id, username, title, scenario, expected, actual, raw_conversation, module, status, admin_note, created_at, updated_at
			FROM sys_bug WHERE status = ? ORDER BY id DESC`, status)
	} else {
		rows, err = conn.Query(`SELECT id, user_id, username, title, scenario, expected, actual, raw_conversation, module, status, admin_note, created_at, updated_at
			FROM sys_bug ORDER BY id DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBugs(rows)
}

// scanBugs 扫描 rows 到 Bug 切片
func scanBugs(rows *sql.Rows) ([]Bug, error) {
	var bugs []Bug
	for rows.Next() {
		var b Bug
		var scenario, expected, actual, rawConv, module, adminNote sql.NullString
		if err := rows.Scan(&b.ID, &b.UserID, &b.Username, &b.Title, &scenario, &expected, &actual, &rawConv, &module, &b.Status, &adminNote, &b.CreatedAt, &b.UpdatedAt); err != nil {
			log.Printf("[bug] scan 失败: %v", err)
			continue
		}
		b.Scenario = scenario.String
		b.Expected = expected.String
		b.Actual = actual.String
		b.RawConversation = rawConv.String
		b.Module = module.String
		b.AdminNote = adminNote.String
		bugs = append(bugs, b)
	}
	return bugs, nil
}

// UpdateBugStatus 更新 Bug 状态（校验流转规则）
func UpdateBugStatus(bugID int, status string, adminNote string) error {
	validStatuses := []string{"submitted", "reviewing", "accepted", "scheduled", "in_progress", "done", "rejected", "withdrawn"}
	valid := false
	for _, s := range validStatuses {
		if s == status {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("非法状态值: %s", status)
	}

	conn, err := GetDB()
	if err != nil {
		return err
	}

	var currentStatus string
	if err := conn.QueryRow("SELECT status FROM sys_bug WHERE id = ?", bugID).Scan(&currentStatus); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("Bug #%d 不存在", bugID)
		}
		return err
	}

	allowed, exists := bugStatusFlow[currentStatus]
	if !exists {
		return fmt.Errorf("未知当前状态: %s", currentStatus)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("当前状态 %s 为终态，不可再变更", currentStatus)
	}
	canTransition := false
	for _, s := range allowed {
		if s == status {
			canTransition = true
			break
		}
	}
	if !canTransition {
		return fmt.Errorf("状态流转非法: %s -> %s", currentStatus, status)
	}

	_, err = conn.Exec("UPDATE sys_bug SET status = ?, admin_note = ?, updated_at = ? WHERE id = ?",
		status, adminNote, time.Now().Format("2006-01-02 15:04:05"), bugID)
	if err != nil {
		return err
	}
	log.Printf("[bug] Bug #%d 状态变更: %s -> %s", bugID, currentStatus, status)
	return nil
}

// WithdrawBug 撤销 Bug（改 status=withdrawn，不删除）
// 撤销无状态限制，但只能撤销自己提的
func WithdrawBug(bugID int, userID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	var ownerID int
	var status string
	err = conn.QueryRow("SELECT user_id, status FROM sys_bug WHERE id = ?", bugID).Scan(&ownerID, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("Bug #%d 不存在", bugID)
		}
		return err
	}
	if ownerID != userID {
		return fmt.Errorf("只能撤销自己提交的 Bug")
	}
	if status == "withdrawn" {
		return fmt.Errorf("Bug 已撤销，无需重复操作")
	}
	_, err = conn.Exec("UPDATE sys_bug SET status = 'withdrawn', updated_at = ? WHERE id = ?",
		time.Now().Format("2006-01-02 15:04:05"), bugID)
	if err != nil {
		return err
	}
	log.Printf("[bug] Bug #%d 已撤销 (%s -> withdrawn)", bugID, status)
	return nil
}

// ExecuteSubmitBug 执行 Bug 管理技能（模仿 ExecuteSubmitRequirement）
// 支持三种场景（通过 action 参数区分）：
//   - action=submit + 无 bug_id：新增 Bug
//   - action=submit + 有 bug_id：修改已有 Bug
//   - action=list：查询当前用户最近提交的 Bug 列表
func ExecuteSubmitBug(userID int, args map[string]interface{}) (map[string]interface{}, error) {
	action, _ := args["action"].(string)
	if action == "" {
		action = "submit"
	}

	if action == "list" {
		bugs, err := GetMyBugs(userID)
		if err != nil {
			return nil, err
		}
		list := make([]map[string]interface{}, 0, len(bugs))
		for _, b := range bugs {
			list = append(list, map[string]interface{}{
				"id":         b.ID,
				"title":      b.Title,
				"status":     b.Status,
				"updated_at": b.UpdatedAt,
			})
		}
		return map[string]interface{}{
			"action":  "list",
			"count":   len(list),
			"items":   list,
			"message": fmt.Sprintf("您共有 %d 条 Bug 反馈", len(list)),
		}, nil
	}

	// action=submit
	title, _ := args["title"].(string)
	if action == "submit" && title == "" && args["bug_id"] == nil {
		return nil, fmt.Errorf("新增 Bug 需要 title 参数")
	}

	scenario, _ := args["scenario"].(string)
	expected, _ := args["expected"].(string)
	actual, _ := args["actual"].(string)
	module, _ := args["module"].(string)

	userInfo, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	username, _ := userInfo["username"].(string)

	rawConv := map[string]interface{}{
		"title":     title,
		"scenario":  scenario,
		"expected":  expected,
		"actual":    actual,
		"module":    module,
	}
	rawJSON, _ := json.Marshal(rawConv)

	var bugIDInt int
	if idFloat, ok := args["bug_id"].(float64); ok && idFloat > 0 {
		bugIDInt = int(idFloat)
	}

	if bugIDInt > 0 {
		if err := UpdateBug(bugIDInt, userID, title, scenario, expected, actual, module, string(rawJSON)); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"id":      int64(bugIDInt),
			"title":   title,
			"status":  "updated",
			"message": "Bug 已更新",
			"trace":   "🐛 Bug 已更新: " + title,
		}, nil
	}

	id, err := SubmitBug(userID, username, title, scenario, expected, actual, module, string(rawJSON))
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":      id,
		"title":   title,
		"status":  "submitted",
		"message": "Bug 已提交成功，开发同学会尽快确认",
		"trace":   "🐛 Bug 已提交: " + title,
	}, nil
}
