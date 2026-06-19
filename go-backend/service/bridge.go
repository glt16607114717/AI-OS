package service

import (
	"ai-os-server/model"
	"strconv"
)

// ── 类型别名（Handler 层引用） ──

type StrategyType = model.Strategy
type StrategyOptionType = model.StrategyOption
type RouteInfoType = model.RouteInfo
type LLMStatType = model.LLMStat

// ── 缺失的辅助函数 ──

// GetDefaultRoute 无策略时取第一个可用厂商+密钥+模型
func GetDefaultRoute() *model.RouteInfo {
	options, err := GetAvailableOptions()
	if err != nil || len(options) == 0 {
		return nil
	}
	first := options[0]
	vendorID, _ := first["vendor_id"].(int)
	vendorName, _ := first["vendor_name"].(string)
	baseURL, _ := first["base_url"].(string)
	keyID := first["key_id"] // 可能是 int 或 float64
	apiKey, _ := first["api_key"].(string)
	modelID, _ := first["model_id"].(string)

	keyIDStr := ""
	switch v := keyID.(type) {
	case int:
		keyIDStr = strconv.Itoa(v)
	case float64:
		keyIDStr = strconv.Itoa(int(v))
	case string:
		keyIDStr = v
	}

	return &model.RouteInfo{
		VendorID:   vendorID,
		VendorName: vendorName,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		KeyID:      keyIDStr,
		ModelID:    modelID,
	}
}
