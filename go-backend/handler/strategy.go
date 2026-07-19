package handler

import (
	"ai-os-server/middleware"
	"ai-os-server/service"
	"encoding/json"
	"net/http"
)

// ── 路由策略管理 ──

func GetStrategies(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	strategies := service.GetStrategies(session.UserID, session.IsAdmin)
	okResponse(w, strategies)
}

func SaveStrategy(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	var s map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	strategy := ParseStrategy(s)
	service.SaveStrategy(strategy, session.UserID)
	okResponse(w, strategy)
}

func DeleteStrategy(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		errResponse(w, "缺少 id", 400)
		return
	}
	service.DeleteStrategy(id, session.UserID)
	okResponse(w, "已删除")
}

func SetActiveStrategy(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	id, _ := body["id"].(string)
	if id == "" {
		errResponse(w, "缺少 id", 400)
		return
	}
	found := service.SetActiveStrategy(id, session.UserID)
	okResponse(w, map[string]interface{}{"found": found})
}

// ── 全局系统策略（管理员维护，user_id=0，全局生效）──

// GetGlobalSystemStrategy 查询全局系统策略（仅管理员）
func GetGlobalSystemStrategy(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !session.IsAdmin {
		errResponse(w, "仅管理员可查看全局系统策略", http.StatusForbidden)
		return
	}
	strategies := service.GetGlobalSystemStrategy()
	okResponse(w, strategies)
}

// SaveGlobalSystemStrategy 保存全局系统策略（仅管理员）
// 请求体：{"strategy": {...}}（单条策略对象）
func SaveGlobalSystemStrategy(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSessionFromCtx(r)
	if session == nil {
		errResponse(w, "未登录", http.StatusUnauthorized)
		return
	}
	if !session.IsAdmin {
		errResponse(w, "仅管理员可编辑全局系统策略", http.StatusForbidden)
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	// 兼容两种入参格式：{strategy:{...}} 或直接 {...}
	var strategyMap map[string]interface{}
	if s, ok := body["strategy"].(map[string]interface{}); ok {
		strategyMap = s
	} else {
		strategyMap = body
	}
	strategy := ParseStrategy(strategyMap)
	// 强制保证全局系统策略的字段约束
	strategy.ID = "global_system"
	strategy.Name = "系统接管"
	strategy.Type = "system"
	strategy.Active = true
	if len(strategy.Options) == 0 {
		errResponse(w, "系统策略至少需要 1 个选项", 400)
		return
	}
	service.SaveGlobalSystemStrategy([]service.StrategyType{strategy})
	okResponse(w, strategy)
}

// ParseStrategy 从 map 解析策略
func ParseStrategy(s map[string]interface{}) service.StrategyType {
	id, _ := s["id"].(string)
	name, _ := s["name"].(string)
	typeStr, _ := s["type"].(string)
	active, _ := s["active"].(bool)

	var options []service.StrategyOptionType
	if optsRaw, ok := s["options"].([]interface{}); ok {
		for _, o := range optsRaw {
			if om, ok := o.(map[string]interface{}); ok {
				opt := service.StrategyOptionType{
					VendorID:    intFloat(om["vendor_id"]),
					KeyID:       intFloatStr(om["key_id"]),
					ModelID:     strVal(om["model_id"]),
					VendorName:  strVal(om["vendor_name"]),
					BaseURL:     strVal(om["base_url"]),
					KeyName:     strVal(om["key_name"]),
					APIKey:      strVal(om["api_key"]),
					DisplayName: strVal(om["display_name"]),
				}
				options = append(options, opt)
			}
		}
	}

	return service.StrategyType{
		ID: id, Name: name, Type: typeStr,
		Active: active, Options: options,
	}
}
