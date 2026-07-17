package service

import (
	"database/sql"
	"encoding/json"
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
var statusFlow = map[string][]string{
	"submitted":   {"reviewing", "rejected"},
	"reviewing":   {"accepted", "rejected"},
	"accepted":    {"scheduled"},
	"scheduled":   {"in_progress"},
	"in_progress": {"done"},
	"done":        {},
	"rejected":    {},
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
func UpdateRequirementStatus(reqID int, status string, adminNote string) error {
	// 校验状态值
	validStatuses := []string{"submitted", "reviewing", "accepted", "scheduled", "in_progress", "done", "rejected"}
	valid := false
	for _, s := range validStatuses {
		if s == status {
			valid = true
			break
		}
	}
	if !valid {
		return sql.ErrNoRows // 用这个不太合适，但我们用简单的错误
	}

	conn, err := GetDB()
	if err != nil {
		return err
	}
	if adminNote != "" {
		_, err = conn.Exec("UPDATE sys_requirement SET status = ?, admin_note = ?, updated_at = ? WHERE id = ?",
			status, adminNote, time.Now().Format("2006-01-02 15:04:05"), reqID)
	} else {
		_, err = conn.Exec("UPDATE sys_requirement SET status = ?, updated_at = ? WHERE id = ?",
			status, time.Now().Format("2006-01-02 15:04:05"), reqID)
	}
	if err != nil {
		return err
	}
	log.Printf("[requirement] 需求 #%d 状态变更: %s", reqID, status)
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
	Title    string `json:"title"`
	Scenario string `json:"scenario"`
	PainPoint string `json:"pain_point"`
	Module   string `json:"module"`
}

// ExecuteSubmitRequirement 执行需求提交技能
func ExecuteSubmitRequirement(userID int, args map[string]interface{}) (map[string]interface{}, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, nil // 标题为空时静默跳过，让 AI 继续引导
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