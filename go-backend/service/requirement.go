package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Requirement 需求结构体
type Requirement struct {
	ID              int    `json:"id"`
	UserID          int    `json:"user_id"`
	Username        string `json:"username"`
	Title           string `json:"title"`
	Scenario        string `json:"scenario"`
	PainPoint       string `json:"pain_point"`
	RawConversation string `json:"raw_conversation"`
	Module          string `json:"module"`
	Status          string `json:"status"`
	AdminNote       string `json:"admin_note"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// StatusFlow 状态流转规则
// withdrawn（已撤销）是终态，任何阶段都能撤销（不物理删除，保留沟通证据）
var statusFlow = map[string][]string{
	"submitted":   {"reviewing", "rejected", "withdrawn"},
	"reviewing":   {"accepted", "rejected", "withdrawn"},
	"accepted":    {"scheduled", "withdrawn"},
	"scheduled":   {"in_progress", "withdrawn"},
	"in_progress": {"done", "withdrawn"},
	"done":        {},
	"rejected":    {},
	"withdrawn":   {}, // 终态，不可再变更
}

// SubmitRequirement AI 提交需求
func SubmitRequirement(userID int, username string, title string, scenario string, painPoint string, module string, rawConversation string) (int64, error) {
	conn, err := GetDB()
	if err != nil {
		return 0, err
	}
	res, err := conn.Exec(`INSERT INTO sys_requirement (user_id, username, title, scenario, pain_point, raw_conversation, module, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'submitted')`,
		userID, username, title, scenario, painPoint, rawConversation, module)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	log.Printf("[requirement] 新需求 #%d: %s (来自 %s)", id, title, username)
	return id, nil
}

// UpdateRequirement AI 修改已有需求
// 仅允许用户修改自己提交的需求，且只有在未开始评估前（submitted/reviewing）可修改
func UpdateRequirement(reqID int, userID int, title string, scenario string, painPoint string, module string, rawConversation string) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	// 校验：必须是自己提的，且还在可修改阶段（submitted/reviewing）
	var ownerID int
	var status string
	err = conn.QueryRow("SELECT user_id, status FROM sys_requirement WHERE id = ?", reqID).Scan(&ownerID, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("需求 #%d 不存在", reqID)
		}
		return err
	}
	if ownerID != userID {
		return fmt.Errorf("只能修改自己提交的需求")
	}
	if status != "submitted" && status != "reviewing" {
		return fmt.Errorf("需求已进入「%s」阶段，不可再修改", status)
	}
	_, err = conn.Exec(`UPDATE sys_requirement SET title=?, scenario=?, pain_point=?, module=?, raw_conversation=?, updated_at=? WHERE id=?`,
		title, scenario, painPoint, module, rawConversation, time.Now().Format("2006-01-02 15:04:05"), reqID)
	if err != nil {
		return err
	}
	log.Printf("[requirement] 需求 #%d 已更新: %s", reqID, title)
	return nil
}

// GetMyRequirements 用户的我的需求列表
func GetMyRequirements(userID int) ([]Requirement, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`SELECT id, user_id, username, title, scenario, pain_point, raw_conversation, module, status, IFNULL(admin_note,''), created_at, updated_at
		FROM sys_requirement WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequirements(rows)
}

// GetAllRequirements 管理员查看所有需求
func GetAllRequirements(status string) ([]Requirement, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	var rows *sql.Rows
	if status != "" {
		rows, err = conn.Query(`SELECT id, user_id, username, title, scenario, pain_point, raw_conversation, module, status, IFNULL(admin_note,''), created_at, updated_at
			FROM sys_requirement WHERE status = ? ORDER BY id DESC`, status)
	} else {
		rows, err = conn.Query(`SELECT id, user_id, username, title, scenario, pain_point, raw_conversation, module, status, IFNULL(admin_note,''), created_at, updated_at
			FROM sys_requirement ORDER BY id DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequirements(rows)
}

// UpdateRequirementStatus 管理员更新需求状态
// 校验规则：
//   1. status 必须是合法值（7 个之一）
//   2. 必须符合 statusFlow 流转规则（不能跳过中间状态）
//   3. admin_note 支持清空（传空字符串时显式清空，而非忽略）
func UpdateRequirementStatus(reqID int, status string, adminNote string) error {
	// 校验状态值合法性
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

	// 查当前状态，校验流转规则
	var currentStatus string
	if err := conn.QueryRow("SELECT status FROM sys_requirement WHERE id = ?", reqID).Scan(&currentStatus); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("需求 #%d 不存在", reqID)
		}
		return err
	}

	// 校验流转规则：当前状态允许流转到哪些状态（statusFlow）
	// 终态（done/rejected）不允许再变更
	allowed, exists := statusFlow[currentStatus]
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
		return fmt.Errorf("状态流转非法: %s -> %s（允许: %s -> %v）", currentStatus, status, currentStatus, allowed)
	}

	// 更新状态 + admin_note（admin_note 支持显式清空）
	_, err = conn.Exec("UPDATE sys_requirement SET status = ?, admin_note = ?, updated_at = ? WHERE id = ?",
		status, adminNote, time.Now().Format("2006-01-02 15:04:05"), reqID)
	if err != nil {
		return err
	}
	log.Printf("[requirement] 需求 #%d 状态变更: %s -> %s", reqID, currentStatus, status)
	return nil
}

// WithdrawRequirement 撤销需求（改 status=withdrawn，不删除）
// 撤销无状态限制，但只能撤销自己提的
// 已撤销的需求保留在数据库里作为沟通证据，不物理删除
func WithdrawRequirement(reqID int, userID int) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	var ownerID int
	var status string
	err = conn.QueryRow("SELECT user_id, status FROM sys_requirement WHERE id = ?", reqID).Scan(&ownerID, &status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("需求 #%d 不存在", reqID)
		}
		return err
	}
	if ownerID != userID {
		return fmt.Errorf("只能撤销自己提交的需求")
	}
	if status == "withdrawn" {
		return fmt.Errorf("需求已撤销，无需重复操作")
	}
	_, err = conn.Exec("UPDATE sys_requirement SET status = 'withdrawn', updated_at = ? WHERE id = ?",
		time.Now().Format("2006-01-02 15:04:05"), reqID)
	if err != nil {
		return err
	}
	log.Printf("[requirement] 需求 #%d 已撤销 (%s -> withdrawn)", reqID, status)
	return nil
}

func scanRequirements(rows *sql.Rows) ([]Requirement, error) {
	var result []Requirement
	for rows.Next() {
		var r Requirement
		var scenario, painPoint, rawConv, module, adminNote sql.NullString
		if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.Title, &scenario, &painPoint, &rawConv, &module, &r.Status, &adminNote, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		r.Scenario = scenario.String
		r.PainPoint = painPoint.String
		r.RawConversation = rawConv.String
		r.Module = module.String
		r.AdminNote = adminNote.String
		result = append(result, r)
	}
	if result == nil {
		result = []Requirement{}
	}
	return result, nil
}

// SubmitRequirementArgs AI 调用技能时传入的参数
type SubmitRequirementArgs struct {
	Title         string `json:"title"`
	Scenario      string `json:"scenario"`
	PainPoint     string `json:"pain_point"`
	Module        string `json:"module"`
	RequirementID int    `json:"requirement_id"` // 可选：传则修改已有需求，不传则新增
}

// ExecuteSubmitRequirement 执行需求管理技能
// 支持三种场景（通过 action 参数区分）：
//   - action=submit + 无 requirement_id：新增需求
//   - action=submit + 有 requirement_id：修改已有需求（仅限自己提的、且在 submitted/reviewing 阶段）
//   - action=list：查询当前用户最近的需求列表（用于 AI 确认要修改哪条）
func ExecuteSubmitRequirement(userID int, args map[string]interface{}) (map[string]interface{}, error) {
	action, _ := args["action"].(string)
	if action == "" {
		action = "submit" // 向后兼容：不传 action 默认 submit
	}

	// 查询用户需求列表
	if action == "list" {
		reqs, err := GetMyRequirements(userID)
		if err != nil {
			return nil, err
		}
		// 只返回 id/title/status/updated_at，精简数据量（避免历史长对话爆 token）
		list := make([]map[string]interface{}, 0, len(reqs))
		for _, r := range reqs {
			list = append(list, map[string]interface{}{
				"id":         r.ID,
				"title":      r.Title,
				"status":     r.Status,
				"updated_at": r.UpdatedAt,
			})
		}
		return map[string]interface{}{
			"action":  "list",
			"count":   len(list),
			"items":   list,
			"message": fmt.Sprintf("您共有 %d 条需求", len(list)),
		}, nil
	}

	// action=submit：新增或修改
	title, _ := args["title"].(string)
	if action == "submit" && title == "" && args["requirement_id"] == nil {
		// 既没 title 又没 requirement_id，无法操作
		return nil, fmt.Errorf("新增需求需要 title 参数")
	}

	scenario, _ := args["scenario"].(string)
	painPoint, _ := args["pain_point"].(string)
	module, _ := args["module"].(string)

	// 通过 userID 查询 username
	userInfo, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	username, _ := userInfo["username"].(string)

	// 构建原始对话摘要
	rawConv := map[string]interface{}{
		"title":      title,
		"scenario":   scenario,
		"pain_point": painPoint,
		"module":     module,
	}
	rawJSON, _ := json.Marshal(rawConv)

	// 判断模式：requirement_id > 0 是修改，否则是新增
	var reqIDInt int
	if idFloat, ok := args["requirement_id"].(float64); ok && idFloat > 0 {
		reqIDInt = int(idFloat)
	}

	if reqIDInt > 0 {
		// 修改模式
		if err := UpdateRequirement(reqIDInt, userID, title, scenario, painPoint, module, string(rawJSON)); err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"id":      int64(reqIDInt),
			"title":   title,
			"status":  "updated",
			"message": "需求已更新",
			"trace":   "📋 需求已更新: " + title,
		}, nil
	}

	// 新增模式
	id, err := SubmitRequirement(userID, username, title, scenario, painPoint, module, string(rawJSON))
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":      id,
		"title":   title,
		"status":  "submitted",
		"message": "需求已提交成功，可在「我的需求」页面查看进度",
		"trace":   "📋 需求已提交: " + title,
	}, nil
}