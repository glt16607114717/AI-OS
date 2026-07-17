package model

// PrankConfig 逗你玩配置
type PrankConfig struct {
	ID           int    `json:"id"`
	TargetUserID int    `json:"target_user_id"`
	TargetName   string `json:"target_name"`
	CustomText   string `json:"custom_text"`
	Enabled      bool   `json:"enabled"`
	CreatedBy    int    `json:"created_by"`
}
