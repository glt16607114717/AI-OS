package service

import (
	"ai-os-server/config"
	"ai-os-server/model"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
				"model_type": m.ModelType, "enabled": m.Enabled, "max_tokens": m.MaxTokens,
			})
		}
	}

	// 查密钥
	keyRows, err := conn.Query("SELECT id, vendor_id, name, api_key, enabled FROM sys_api_key ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer keyRows.Close()
	for keyRows.Next() {
		var k model.APIKey
		if err := keyRows.Scan(&k.ID, &k.VendorID, &k.Name, &k.APIKey, &k.Enabled); err != nil {
			continue
		}
		if vm, ok := vendorMap[k.VendorID]; ok {
			keys := vm["keys"].([]map[string]interface{})
			vm["keys"] = append(keys, map[string]interface{}{
				"id": k.ID, "name": k.Name, "api_key": k.APIKey, "enabled": k.Enabled,
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
		enabled := 1
		if v, ok := k["enabled"]; ok {
			if b, ok := v.(bool); ok && !b {
				enabled = 0
			}
		}
		if _, err := tx.Exec("INSERT INTO sys_api_key (vendor_id, name, api_key, enabled) VALUES (?, ?, ?, ?)",
			vendorID, name, apiKey, enabled); err != nil {
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
		JOIN sys_model m ON m.vendor_id = v.id AND m.enabled = 1
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

var rrCounter int64

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

func loadStrategiesFromDB() []model.Strategy {
	conn, err := GetDB()
	if err != nil {
		log.Printf("[strategy] DB连接失败: %v", err)
		return nil
	}
	var data string
	err = conn.QueryRow("SELECT data FROM sys_strategy ORDER BY id DESC LIMIT 1").Scan(&data)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[strategy] 加载失败: %v", err)
		}
		return nil
	}
	var strategies []model.Strategy
	if err := parseJSON(data, &strategies); err != nil {
		log.Printf("[strategy] 解析 JSON 失败: %v", err)
		return nil
	}
	return strategies
}

func saveStrategiesToDB(strategies []model.Strategy) {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	data := toJSON(strategies)
	// 先清空再插入
	conn.Exec("TRUNCATE TABLE sys_strategy")
	conn.Exec("INSERT INTO sys_strategy (data) VALUES (?)", data)
}

func GetStrategies() []model.Strategy {
	return loadStrategiesFromDB()
}

func SaveStrategy(s model.Strategy) {
	strategies := loadStrategiesFromDB()

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
	saveStrategiesToDB(strategies)
}

func DeleteStrategy(id string) {
	strategies := loadStrategiesFromDB()
	var filtered []model.Strategy
	for _, s := range strategies {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	saveStrategiesToDB(filtered)
}

func SetActiveStrategy(id string) bool {
	strategies := loadStrategiesFromDB()
	found := false
	for i := range strategies {
		if strategies[i].ID == id {
			strategies[i].Active = true
			found = true
		} else {
			strategies[i].Active = false
		}
	}
	saveStrategiesToDB(strategies)
	return found
}

func GetRouteByStrategy() *model.RouteInfo {
	strategies := loadStrategiesFromDB()

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
		for i := 0; i < total; i++ {
			idx := int(atomic.AddInt64(&rrCounter, 1)) % total
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

func GetAllRoutesForFailover() []model.RouteInfo {
	strategies := loadStrategiesFromDB()

	var active *model.Strategy
	for i := range strategies {
		if strategies[i].Active {
			active = &strategies[i]
			break
		}
	}
	if active == nil || active.Type != "round_robin" {
		return nil
	}

	var routes []model.RouteInfo
	for _, opt := range active.Options {
		if IsKeyExhausted(opt.KeyID) {
			continue
		}
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

	var apiKey string
	if err := conn.QueryRow("SELECT api_key FROM sys_api_key WHERE id = ?", opt.KeyID).Scan(&apiKey); err != nil || apiKey == "" {
		return nil
	}

	ri := &model.RouteInfo{
		VendorID:   opt.VendorID,
		VendorName: vendorName,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		KeyID:      opt.KeyID,
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
