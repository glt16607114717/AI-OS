package model

// OtherSetting 其他设置（全局单行配置）
type OtherSetting struct {
	MessageRounds int `json:"message_rounds"` // 消息对话轮数，控制历史保留多少轮 user 消息。默认10，最小1
}
