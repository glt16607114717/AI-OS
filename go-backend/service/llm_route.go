package service

import (
	"ai-os-server/config"
	"ai-os-server/model"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"sync"
	"sync/atomic"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db   *sql.DB
	once struct {
		db   *sql.DB
		done bool
	}
)

func GetDB() (*sql.DB, error) {
	if once.db != nil {
		return once.db, nil
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		config.DB.User,
		config.DB.Password,
		config.DB.Host,
		config.DB.Port,
		config.DB.Database,
	)
	var err error
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(20)
	conn.SetMaxIdleConns(5)
	once.db = conn
	return conn, nil
}

// ── 厂商/模型/密钥 ──

func GetCatalog() ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}

	// 查厂商
	vendorRows, err := conn.Query("SELECT id, code, name, base_url, enabled FROM sys_vendor ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer vendorRows.Close()

	vendors := []map[string]interface{}{}
	vendorMap := map[int]map[string]interface{}{}
	for vendorRows.Next() {
		var v model.Vendor
		if err := vendorRows.Scan(&v.ID, &v.Code, &v.Name, &v.BaseURL, &v.Enabled); err != nil {
			continue
		}
		m := map[string]interface{}{
			"id": v.ID, "code": v.Code, "name": v.Name,
			"base_url": v.BaseURL, "enabled": v.Enabled,
			"models": []map[string]interface{}{},
			"keys":   []map[string]interface{}{},
		}
		vendors = append(vendors, m)
		vendorMap[v.ID] = m
	}

	// 查模型
	modelRows, err := conn.Query("SELECT id, vendor_id, model_id, name, model_type, enabled, max_tokens FROM sys_model ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var m model.ModelEntry
		if err := modelRows.Scan(&m.ID, &m.VendorID, &m.ModelID, &m.Name, &m.ModelType, &m.Enabled, &m.MaxTokens); err != nil {
			continue
		}
		if vm, ok := vendorMap[m.VendorID]; ok {
			models := vm["models"].([]map[string]interface{})
			vm["models"] = append(models, map[string]interface{}{
				"id": m.ID, "model_id": m.ModelID, "display_name": m.Name,
				"model_type": m.ModelType.String, "enabled": m.Enabled, "max_tokens": m.MaxTokens,
			})
		}
	}

	// 查密钥
	keyRows, err := conn.Query("SELECT id, vendor_id, name, api_key, access_key, secret_key, enabled FROM sys_api_key ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer keyRows.Close()
	for keyRows.Next() {
		var k model.APIKey
		if err := keyRows.Scan(&k.ID, &k.VendorID, &k.Name, &k.APIKey, &k.AccessKey, &k.SecretKey, &k.Enabled); err != nil {
			continue
		}
		if vm, ok := vendorMap[k.VendorID]; ok {
			keys := vm["keys"].([]map[string]interface{})
			vm["keys"] = append(keys, map[string]interface{}{
				"id": k.ID, "name": k.Name, "api_key": k.APIKey,
				"access_key": k.AccessKey, "secret_key": k.SecretKey,
				"enabled": k.Enabled,
			})
		}
	}

	return vendors, nil
}

func SaveVendorKeys(vendorID int, keys []map[string]interface{}) error {
	conn, err := GetDB()
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 验证厂商存在
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM sys_vendor WHERE id = ?", vendorID).Scan(&exists); err != nil {
		return fmt.Errorf("厂商不存在")
	}

	// 删除旧密钥
	if _, err := tx.Exec("DELETE FROM sys_api_key WHERE vendor_id = ?", vendorID); err != nil {
		return err
	}

	// 插入新密钥
	for _, k := range keys {
		name, _ := k["name"].(string)
		apiKey, _ := k["api_key"].(string)
		accessKey, _ := k["access_key"].(string)
		secretKey, _ := k["secret_key"].(string)
		enabled := 1
		if v, ok := k["enabled"]; ok {
			if b, ok := v.(bool); ok && !b {
				enabled = 0
			}
		}
		if _, err := tx.Exec("INSERT INTO sys_api_key (vendor_id, name, api_key, access_key, secret_key, enabled) VALUES (?, ?, ?, ?, ?, ?)",
			vendorID, name, apiKey, accessKey, secretKey, enabled); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ToggleVendor(vendorID int, enabled bool) (bool, error) {
	conn, err := GetDB()
	if err != nil {
		return false, err
	}
	e := 0
	if enabled {
		e = 1
	}
	res, err := conn.Exec("UPDATE sys_vendor SET enabled = ? WHERE id = ?", e, vendorID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── 可用选项 ──

func GetAvailableOptions() ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`
		SELECT v.id as vendor_id, v.name as vendor_name, v.base_url,
		       k.id as key_id, k.name as key_name, k.api_key,
		       m.model_id, m.name as display_name
		FROM sys_vendor v
		JOIN sys_api_key k ON k.vendor_id = v.id AND k.enabled = 1
		JOIN sys_model m ON m.vendor_id = v.id
		WHERE v.enabled = 1 AND k.api_key != ''
		ORDER BY v.id, m.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var vendorID int
		var vendorName, baseURL string
		var keyID int
		var keyName, apiKey, modelID, displayName string
		if err := rows.Scan(&vendorID, &vendorName, &baseURL, &keyID, &keyName, &apiKey, &modelID, &displayName); err != nil {
			log.Printf("[options] scan 失败: %v", err)
			continue
		}
		result = append(result, map[string]interface{}{
			"vendor_id": vendorID, "vendor_name": vendorName, "base_url": baseURL,
			"key_id": keyID, "key_name": keyName, "api_key": apiKey,
			"model_id": modelID, "display_name": displayName,
		})
	}
	return result, nil
}

// ── 策略路由（持久化到 MySQL）──

// 用户独立的轮询计数器：避免多用户共享导致顺序错乱
// key=userID, value=*int64（按用户定义顺序严格轮询）
var userRRCounters sync.Map

// getUserRRCounter 获取（或初始化）指定用户的轮询计数器
func getUserRRCounter(userID int) *int64 {
	key := fmt.Sprintf("%d", userID)
	if v, ok := userRRCounters.Load(key); ok {
		return v.(*int64)
	}
	newCounter := int64(0)
	actual, _ := userRRCounters.LoadOrStore(key, &newCounter)
	return actual.(*int64)
}

// randIntn 密码学安全的随机整数 [0, n)
func randIntn(n int) int {
	if n <= 0 {
		return 0
	}
	bigN, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(bigN.Int64())
}

func EnsureStrategyTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_strategy (
		id INT AUTO_INCREMENT PRIMARY KEY,
		data JSON NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
}

func loadStrategiesFromDB(userID int, isAdmin bool) []model.Strategy {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[strategy] DB连接失败: %v", err)
		return nil
	}

	var rows *sql.Rows
	var errQuery error

	if isAdmin {
		// 管理员查看所有策略，包含用户信息
		rows, errQuery = conn.Query(`
			SELECT s.id, s.user_id, s.data, u.username 
			FROM sys_strategy s 
			LEFT JOIN sys_user u ON u.id = s.user_id 
			ORDER BY s.user_id, s.id DESC
		`)
	} else {
		// 普通用户只查看自己的策略
		rows, errQuery = conn.Query(`
			SELECT s.id, s.user_id, s.data, u.username 
			FROM sys_strategy s 
			LEFT JOIN sys_user u ON u.id = s.user_id 
			WHERE s.user_id = ? 
			ORDER BY s.id DESC
		`, userID)
	}

	if errQuery != nil {
		if errQuery != sql.ErrNoRows {
			log.Printf("[strategy] 加载失败: %v", errQuery)
		}
		return nil
	}
	defer rows.Close()

	var strategies []model.Strategy
	for rows.Next() {
		var id int
		var data string
		var userID int
		var username sql.NullString
		
		if err := rows.Scan(&id, &userID, &data, &username); err != nil {
			log.Printf("[strategy] 扫描失败: %v", err)
			continue
		}

		var userStrategies []model.Strategy
		if err := parseJSON(data, &userStrategies); err != nil {
			log.Printf("[strategy] 解析 JSON 失败: %v", err)
			continue
		}

		// 为每个策略添加用户信息
		for i := range userStrategies {
			userStrategies[i].UserID = userID
			if username.Valid {
				userStrategies[i].Username = username.String
			} else {
				userStrategies[i].Username = fmt.Sprintf("用户%d", userID)
			}
		}
		
		strategies = append(strategies, userStrategies...)
	}
	
	return strategies
}

func saveStrategiesToDB(strategies []model.Strategy, userID int) {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	data := toJSON(strategies)
	// 先删除该用户的策略，再插入
	conn.Exec("DELETE FROM sys_strategy WHERE user_id = ?", userID)
	conn.Exec("INSERT INTO sys_strategy (user_id, data) VALUES (?, ?)", userID, data)
}

func GetStrategies(userID int, isAdmin bool) []model.Strategy {
	return loadStrategiesFromDB(userID, isAdmin)
}

func SaveStrategy(s model.Strategy, userID int) {
	strategies := loadStrategiesFromDB(userID, false)

	if s.Active {
		for i := range strategies {
			strategies[i].Active = false
		}
	}
	if s.ID == "" {
		s.ID = randomID(12)
		strategies = append(strategies, s)
	} else {
		found := false
		for i, st := range strategies {
			if st.ID == s.ID {
				strategies[i] = s
				found = true
				break
			}
		}
		if !found {
			strategies = append(strategies, s)
		}
	}
	saveStrategiesToDB(strategies, userID)
}

func DeleteStrategy(id string, userID int) {
	strategies := loadStrategiesFromDB(userID, false)
	var filtered []model.Strategy
	for _, s := range strategies {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	saveStrategiesToDB(filtered, userID)
}

func SetActiveStrategy(id string, userID int) bool {
	strategies := loadStrategiesFromDB(userID, false)
	found := false
	for i := range strategies {
		if strategies[i].ID == id {
			strategies[i].Active = true
			found = true
		} else {
			strategies[i].Active = false
		}
	}
	saveStrategiesToDB(strategies, userID)
	return found
}

func GetRouteByStrategy(userID int) *model.RouteInfo {
	strategies := loadStrategiesFromDB(userID, false)

	var active *model.Strategy
	for i := range strategies {
		if strategies[i].Active {
			active = &strategies[i]
			break
		}
	}
	if active == nil || len(active.Options) == 0 {
		return nil
	}

	var selected *model.StrategyOption
	if active.Type == "round_robin" {
		total := len(active.Options)
		userCounter := getUserRRCounter(userID)
		for i := 0; i < total; i++ {
			// 按用户独立计数，严格按定义顺序轮询 A→B→C→A→B→C
			idx := int(atomic.AddInt64(userCounter, 1)-1) % total
			opt := active.Options[idx]
			if !IsKeyExhausted(opt.KeyID) {
				s := opt
				selected = &s
				break
			}
		}
	} else {
		selected = &active.Options[0]
	}
	if selected == nil {
		return nil
	}

	return enrichRouteInfo(selected)
}

// GetAllRoutesForFailover 通用故障转移机制（非策略类型本身）
//   - fixed：排除当前已选，从剩余可用模型中【随机】挑兜底，避免单点压力
//   - round_robin：按用户定义顺序输出剩余可用，自然衔接"跳到下一个"的语义
//
// 策略类型决定选路方式，故障转移是所有策略都具备的通用兜底机制
func GetAllRoutesForFailover(userID int) []model.RouteInfo {
	strategies := loadStrategiesFromDB(userID, false)

	var active *model.Strategy
	for i := range strategies {
		if strategies[i].Active {
			active = &strategies[i]
			break
		}
	}
	// 任何策略都应支持故障转移；无策略则无兜底
	if active == nil {
		return nil
	}

	// 收集所有可用模型
	var available []model.StrategyOption
	for _, opt := range active.Options {
		if IsKeyExhausted(opt.KeyID) {
			continue
		}
		available = append(available, opt)
	}

	switch active.Type {
	case "fixed":
		// 固定策略失败 → 随机打散剩余可用模型做兜底
		// （含 primary 自身，调用方会跳过 primary.KeyID）
		shuffled := make([]model.StrategyOption, len(available))
		copy(shuffled, available)
		// Fisher-Yates 洗牌（crypto/rand）
		for i := len(shuffled) - 1; i > 0; i-- {
			j := randIntn(i + 1)
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}
		available = shuffled

	case "round_robin":
		// 轮询策略失败 → 按用户定义顺序返回，自然跳到下一个
		// available 已是按 Options 定义顺序收集，无需调整
	}

	var routes []model.RouteInfo
	for _, opt := range available {
		if ri := enrichRouteInfo(&opt); ri != nil {
			routes = append(routes, *ri)
		}
	}
	return routes
}

func enrichRouteInfo(opt *model.StrategyOption) *model.RouteInfo {
	conn, err := GetDB()
	if err != nil {
		return nil
	}

	var vendorName, baseURL string
	if err := conn.QueryRow("SELECT name, base_url FROM sys_vendor WHERE id = ?", opt.VendorID).Scan(&vendorName, &baseURL); err != nil {
		return nil
	}

	var apiKey, keyName string
	if err := conn.QueryRow("SELECT api_key, name FROM sys_api_key WHERE id = ?", opt.KeyID).Scan(&apiKey, &keyName); err != nil || apiKey == "" {
		return nil
	}

	ri := &model.RouteInfo{
		VendorID:   opt.VendorID,
		VendorName: vendorName,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		KeyID:      opt.KeyID,
		KeyName:    keyName,
		ModelID:    opt.ModelID,
	}
	if IsKeyExhausted(opt.KeyID) {
		ri.QuotaExhausted = true
	}
	return ri
}

// randomID 使用 crypto/rand 生成真随机 ID
func randomID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	const charset = "0123456789abcdef"
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}

// ── JSON 辅助 ──

func parseJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("[json] 序列化失败: %v", err)
		return "null"
	}
	return string(b)
}
