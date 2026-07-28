package service

import (
	"ai-os-server/model"
	"log"
	"sync"
)

// ── 其他设置（全局单行配置，内存缓存）──

const defaultMessageRounds = 10 // 默认保留 10 轮对话

var (
	otherSettingCache  *model.OtherSetting
	otherSettingLock   sync.RWMutex
	otherSettingLoaded = false
)

// EnsureOtherSettingTable 建表并初始化默认行（id=1 单行设计）
func EnsureOtherSettingTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_other_setting (
		id INT AUTO_INCREMENT PRIMARY KEY,
		message_rounds INT NOT NULL DEFAULT 10,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	// 初始化默认行（仅在表为空时插入）
	var count int
	conn.QueryRow("SELECT COUNT(*) FROM sys_other_setting").Scan(&count)
	if count == 0 {
		conn.Exec("INSERT INTO sys_other_setting (id, message_rounds) VALUES (1, ?)", defaultMessageRounds)
		log.Printf("[other_setting] 初始化默认配置: message_rounds=%d", defaultMessageRounds)
	}
}

// loadOtherSettingFromDB 从数据库加载配置到缓存
func loadOtherSettingFromDB() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	var rounds int
	err := conn.QueryRow("SELECT message_rounds FROM sys_other_setting WHERE id = 1").Scan(&rounds)
	if err != nil {
		log.Printf("[other_setting] 读取配置失败，使用默认值: %v", err)
		rounds = defaultMessageRounds
	}
	if rounds < 1 {
		rounds = 1
	}

	otherSettingLock.Lock()
	otherSettingCache = &model.OtherSetting{MessageRounds: rounds}
	otherSettingLoaded = true
	otherSettingLock.Unlock()
}

// GetOtherSetting 获取其他设置（带缓存，惰性加载）
func GetOtherSetting() *model.OtherSetting {
	otherSettingLock.RLock()
	if !otherSettingLoaded || otherSettingCache == nil {
		otherSettingLock.RUnlock()
		loadOtherSettingFromDB()
		otherSettingLock.RLock()
	}
	cfg := otherSettingCache
	otherSettingLock.RUnlock()
	return cfg
}

// SaveOtherSetting 保存其他设置（同步更新内存缓存）
func SaveOtherSetting(cfg *model.OtherSetting) {
	if cfg.MessageRounds < 1 {
		cfg.MessageRounds = 1
	}

	conn, err := GetDB()
	if err != nil {
		log.Printf("[other_setting] DB连接失败: %v", err)
		return
	}
	_, err = conn.Exec("UPDATE sys_other_setting SET message_rounds = ? WHERE id = 1", cfg.MessageRounds)
	if err != nil {
		log.Printf("[other_setting] 保存配置失败: %v", err)
		return
	}

	// 同步更新内存缓存
	otherSettingLock.Lock()
	otherSettingCache = cfg
	otherSettingLock.Unlock()
	log.Printf("[other_setting] 配置已更新: message_rounds=%d", cfg.MessageRounds)
}
