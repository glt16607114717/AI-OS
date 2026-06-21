package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ── 技能列表（前端菜单用） ──

func GetBuiltinSkills(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	isAdmin := session != nil && session.IsAdmin

	skills, err := service.GetBuiltinSkills()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}

	// 非管理员过滤无权限的技能
	if !isAdmin {
		var filtered []service.BuiltinSkill
		for _, s := range skills {
			conns, _ := service.GetSkillConnections(s.ID)
			// 检查是否有权限（有连接记录即表示有权限）
			hasPerm := false
			for _, c := range conns {
				if c.Enabled {
					hasPerm = true
					break
				}
			}
			if hasPerm {
				filtered = append(filtered, s)
			}
		}
		skills = filtered
	}

	okResponse(w, skills)
}

// ── 连接管理 ──

func GetSkillConnections(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	skill, err := service.GetBuiltinSkillByCode(code)
	if err != nil {
		errResponse(w, "技能不存在", 404)
		return
	}

	session := middleware.GetSessionFromCtx(r)
	isAdmin := session != nil && session.IsAdmin

	conns, err := service.GetSkillConnections(skill.ID)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}

	// 非管理员只看有权限的
	if !isAdmin {
		var filtered []service.SkillConnection
		for _, c := range conns {
			// 简化：只要连接启用就显示（权限在 AI 调用时再校验）
			if c.Enabled {
				filtered = append(filtered, c)
			}
		}
		conns = filtered
	}

	okResponse(w, conns)
}

func CreateSkillConnection(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	skill, err := service.GetBuiltinSkillByCode(code)
	if err != nil {
		errResponse(w, "技能不存在", 404)
		return
	}

	var body struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	if err := service.CreateSkillConnection(skill.ID, body.Name, body.Description, body.Config); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, nil)
}

func UpdateSkillConnection(w http.ResponseWriter, r *http.Request) {
	connID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		errResponse(w, "无效的连接ID", 400)
		return
	}

	var body struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Config      map[string]interface{} `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	if err := service.UpdateSkillConnection(connID, body.Name, body.Description, body.Config); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, nil)
}

func DeleteSkillConnection(w http.ResponseWriter, r *http.Request) {
	connID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		errResponse(w, "无效的连接ID", 400)
		return
	}
	if err := service.DeleteSkillConnection(connID); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, nil)
}

// ── 权限管理 ──

func GetSkillPermissions(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	skill, err := service.GetBuiltinSkillByCode(code)
	if err != nil {
		errResponse(w, "技能不存在", 404)
		return
	}

	perms, err := service.GetSkillPermissions(skill.ID)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, perms)
}

func SetSkillPermission(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	skill, err := service.GetBuiltinSkillByCode(code)
	if err != nil {
		errResponse(w, "技能不存在", 404)
		return
	}

	var body struct {
		Permissions []struct {
			UserID        int   `json:"user_id"`
			ConnectionIDs []int `json:"connection_ids"`
		} `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}

	if err := service.SetSkillPermissions(skill.ID, body.Permissions); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, nil)
}
