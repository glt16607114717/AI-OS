package handler

import (
	"ai-os-server/service"
	"encoding/json"
	"net/http"
	"strconv"
)

// ── LLM 配置管理（厂商/密钥/模型目录） ──

func GetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := service.GetCatalog()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, catalog)
}

func SaveVendorKeys(w http.ResponseWriter, r *http.Request) {
	vendorIDStr := r.URL.Query().Get("vendor_id")
	vendorID, err := strconv.Atoi(vendorIDStr)
	if err != nil {
		errResponse(w, "vendor_id 无效", 400)
		return
	}
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResponse(w, "请求体解析失败", 400)
		return
	}
	keysRaw, _ := body["keys"].([]interface{})
	var keys []map[string]interface{}
	for _, k := range keysRaw {
		if m, ok := k.(map[string]interface{}); ok {
			keys = append(keys, m)
		}
	}
	if err := service.SaveVendorKeys(vendorID, keys); err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, "保存成功")
}

func ToggleVendor(w http.ResponseWriter, r *http.Request) {
	vendorIDStr := r.URL.Query().Get("vendor_id")
	vendorID, err := strconv.Atoi(vendorIDStr)
	if err != nil {
		errResponse(w, "vendor_id 无效", 400)
		return
	}
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)
	enabled, _ := body["enabled"].(bool)
	updated, err := service.ToggleVendor(vendorID, enabled)
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, map[string]interface{}{"updated": updated})
}

func GetAvailableOptions(w http.ResponseWriter, r *http.Request) {
	options, err := service.GetAvailableOptions()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, options)
}

// GetAllOptions 返回全部选项（含已禁用的 key），供策略名称翻译使用
// 历史策略可能引用 enabled=0 的 key，翻译时需要查到真实名称
func GetAllOptions(w http.ResponseWriter, r *http.Request) {
	options, err := service.GetAllOptionsIncludeDisabled()
	if err != nil {
		errResponse(w, err.Error(), 500)
		return
	}
	okResponse(w, options)
}
