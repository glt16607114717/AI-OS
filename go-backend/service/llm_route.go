package service

import (
	"ai-os-server/config"
	"ai-os-server/middleware"
	"ai-os-server/model"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

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
	modelRows, err := conn.Query("SELECT id, vendor_id, model_id, name, description, model_type, enabled, max_tokens FROM sys_model ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var m model.ModelEntry
		if err := modelRows.Scan(&m.ID, &m.VendorID, &m.ModelID, &m.Name, &m.Description, &m.ModelType, &m.Enabled, &m.MaxTokens); err != nil {
			continue
		}
		if vm, ok := vendorMap[m.VendorID]; ok {
			models := vm["models"].([]map[string]interface{})
			vm["models"] = append(models, map[string]interface{}{
				"id": m.ID, "model_id": m.ModelID, "display_name": m.Name,
				"description": m.Description.String, "model_type": m.ModelType.String, "enabled": m.Enabled, "max_tokens": m.MaxTokens,
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

	// 查现有 key 建立 name→id 映射（不改 ID，保持系统策略硬编码 KeyID 有效）
	existingRows, err := tx.Query("SELECT id, name FROM sys_api_key WHERE vendor_id = ?", vendorID)
	if err != nil {
		return err
	}
	defer existingRows.Close()

	existingMap := make(map[string]int) // name → id
	for existingRows.Next() {
		var id int
		var name string
		if err := existingRows.Scan(&id, &name); err != nil {
			continue
		}
		existingMap[name] = id
	}

	// 传入 key 名称集合
	incomingNames := make(map[string]bool)
	for _, k := range keys {
		name, _ := k["name"].(string)
		incomingNames[name] = true
	}

	// 软删除不在传入列表中的 key（改 enabled=0，保留记录供历史策略翻译名称）
	// 注意：不能物理删除，否则策略 JSON 里引用的 key_id 会查不到名称，前端只能显示 ID
	for name, id := range existingMap {
		if !incomingNames[name] {
			if _, err := tx.Exec("UPDATE sys_api_key SET enabled = 0 WHERE id = ?", id); err != nil {
				return err
			}
		}
	}

	// 更新已有 key / 插入新 key
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
		if existingID, ok := existingMap[name]; ok {
			// UPDATE 已有 key，ID 不变
			if _, err := tx.Exec("UPDATE sys_api_key SET name=?, api_key=?, access_key=?, secret_key=?, enabled=? WHERE id=?",
				name, apiKey, accessKey, secretKey, enabled, existingID); err != nil {
				return err
			}
		} else {
			// INSERT 新 key
			if _, err := tx.Exec("INSERT INTO sys_api_key (vendor_id, name, api_key, access_key, secret_key, enabled) VALUES (?, ?, ?, ?, ?, ?)",
				vendorID, name, apiKey, accessKey, secretKey, enabled); err != nil {
				return err
			}
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
	return getOptions(false)
}

// GetAllOptionsIncludeDisabled 返回全部选项（含已禁用的 key），供策略名称翻译使用
// 历史策略可能引用 enabled=0 的 key，翻译时需要查到真实名称，否则前端只能显示 ID
func GetAllOptionsIncludeDisabled() ([]map[string]interface{}, error) {
	return getOptions(true)
}

// getOptions 查询厂商/密钥/模型选项
//   - includeDisabled=false：只返回 enabled=1 的 key（策略编辑页级联选择用，避免选到禁用项）
//   - includeDisabled=true：返回全部 key（策略名称翻译用，保证 resolveOptionLabel 能解析）
func getOptions(includeDisabled bool) ([]map[string]interface{}, error) {
	conn, err := GetDB()
	if err != nil {
		return nil, err
	}
	query := `
		SELECT v.id as vendor_id, v.name as vendor_name, v.base_url,
		       k.id as key_id, k.name as key_name, k.api_key, k.enabled,
		       m.model_id, m.name as display_name
		FROM sys_vendor v
		JOIN sys_api_key k ON k.vendor_id = v.id
		JOIN sys_model m ON m.vendor_id = v.id
		WHERE v.enabled = 1 AND k.api_key != ''
	`
	if !includeDisabled {
		query += " AND k.enabled = 1"
	}
	query += " ORDER BY v.id, m.id"

	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var vendorID, keyID, keyEnabled int
		var vendorName, baseURL string
		var keyName, apiKey, modelID, displayName string
		if err := rows.Scan(&vendorID, &vendorName, &baseURL, &keyID, &keyName, &apiKey, &keyEnabled, &modelID, &displayName); err != nil {
			log.Printf("[options] scan 失败: %v", err)
			continue
		}
		result = append(result, map[string]interface{}{
			"vendor_id": vendorID, "vendor_name": vendorName, "base_url": baseURL,
			"key_id": keyID, "key_name": keyName, "api_key": apiKey, "key_enabled": keyEnabled,
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

// ── 策略 Redis 缓存层（cache-aside 模式）──
//
// 策略数据超高频查询且相对固定，走 Redis 缓存：
//   - 读：先查 Redis，miss 查 MySQL 并回写（TTL 30 天兜底，防永久脏数据）
//   - 写：DB 更新成功后 DEL 对应 key（不回写，下次读自动重建）
//
// key 规范：
//   - 用户策略：strategy:user:{userID}
//   - 全局系统策略：strategy:global
//
// 一致性保障：
//   - 所有写操作（save/saveGlobal/setActive/delete）都在 DB 成功后 DEL key
//   - Redis 不可用时降级直查 DB（GetRedis() 返回 nil 时跳过缓存）
const (
	strategyCacheTTL = 30 * 24 * time.Hour // 30 天兜底 TTL
)

func strategyCacheKey(userID int) string  { return fmt.Sprintf("strategy:user:%d", userID) }
const strategyGlobalCacheKey = "strategy:global"

// strategyCacheGet 读缓存，miss 返回 nil
// 注意：返回的切片是反序列化副本，调用方可安全修改
func strategyCacheGet(ctx context.Context, key string) []model.Strategy {
	rdb := middleware.GetRedis()
	if rdb == nil {
		return nil // Redis 不可用，降级直查 DB
	}
	data, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return nil // cache miss 或 Redis 异常，统一走 DB
	}
	var strategies []model.Strategy
	if err := json.Unmarshal([]byte(data), &strategies); err != nil {
		log.Printf("[strategyCache] 反序列化失败 key=%s: %v", key, err)
		return nil
	}
	return strategies
}

// strategyCacheSet 回写缓存（读 DB 后调用）
func strategyCacheSet(ctx context.Context, key string, strategies []model.Strategy) {
	rdb := middleware.GetRedis()
	if rdb == nil {
		return
	}
	data, err := json.Marshal(strategies)
	if err != nil {
		log.Printf("[strategyCache] 序列化失败 key=%s: %v", key, err)
		return
	}
	if err := rdb.Set(ctx, key, data, strategyCacheTTL).Err(); err != nil {
		log.Printf("[strategyCache] 回写失败 key=%s: %v", key, err)
	}
}

// strategyCacheDel 删缓存（DB 写操作成功后调用）
func strategyCacheDel(ctx context.Context, key string) {
	rdb := middleware.GetRedis()
	if rdb == nil {
		return
	}
	if err := rdb.Del(ctx, key).Err(); err != nil {
		log.Printf("[strategyCache] 删除失败 key=%s: %v", key, err)
	}
}

func EnsureStrategyTable() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	conn.Exec(`CREATE TABLE IF NOT EXISTS sys_strategy (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL DEFAULT 0,
		data JSON NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	// 确保全局系统策略行存在（user_id=0）
	// 无激活策略的所有用户都走这条策略，缺失会导致请求失败
	ensureGlobalSystemStrategy()
}

// ensureGlobalSystemStrategy 启动时确保 user_id=0 的全局系统策略行存在
// 若不存在，用 SystemStrategyOptions 初始化；若已存在则不覆盖（管理员可能已修改）
func ensureGlobalSystemStrategy() {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	var id int
	err := conn.QueryRow("SELECT id FROM sys_strategy WHERE user_id = 0").Scan(&id)
	if err == sql.ErrNoRows {
		// 不存在，用默认系统策略初始化
		defaultStrategy := []model.Strategy{{
			ID:      "global_system",
			Name:    "系统接管",
			Type:    "system",
			Active:  true,
			Options: SystemStrategyOptions(),
		}}
		data := toJSON(defaultStrategy)
		conn.Exec("INSERT INTO sys_strategy (user_id, data) VALUES (0, ?)", data)
		log.Println("[strategy] 全局系统策略初始化完成（user_id=0）")
	} else if err != nil {
		log.Printf("[strategy] 检查全局系统策略失败: %v", err)
	}
}

// SystemStrategyOptions 系统接管预置的6个API配置
// 智谱2个key + DeepSeek 1个key + 火山方舟3个key = 6个
func SystemStrategyOptions() []model.StrategyOption {
	return []model.StrategyOption{
		{VendorID: 1, KeyID: "1", ModelID: "glm-5.2", VendorName: "智谱", KeyName: "桂良涛", DisplayName: "GLM-5.2"},
		{VendorID: 1, KeyID: "2", ModelID: "glm-5.2", VendorName: "智谱", KeyName: "李现成", DisplayName: "GLM-5.2"},
		{VendorID: 2, KeyID: "6", ModelID: "deepseek-chat", VendorName: "DeepSeek", KeyName: "卞成龙", DisplayName: "DeepSeek-Chat"},
		{VendorID: 3, KeyID: "3", ModelID: "ark-code-latest", VendorName: "火山方舟", KeyName: "桂良涛", DisplayName: "Ark Code Latest"},
		{VendorID: 3, KeyID: "4", ModelID: "glm-5.2", VendorName: "火山方舟", KeyName: "陈梓健", DisplayName: "GLM-5.2 (火山)"},
		{VendorID: 3, KeyID: "5", ModelID: "deepseek-v4-pro", VendorName: "火山方舟", KeyName: "李伟男", DisplayName: "DeepSeek-V4-Pro (火山)"},
	}
}

// ── 全局系统策略（user_id=0，管理员维护，全局生效）──
//
// 生效逻辑：
//   - 用户有激活策略（fixed/round_robin）→ 走自己的策略
//   - 用户无激活策略 → 走全局系统策略（不再 fallback GetDefaultRoute）
//
// 缓存：走 strategy:global key，cache-aside 模式

// loadGlobalSystemStrategy 从 DB 加载全局系统策略（带 Redis 缓存）
// 返回 *Strategy（单条），若不存在返回 nil
func loadGlobalSystemStrategy() *model.Strategy {
	ctx := context.Background()
	cacheKey := strategyGlobalCacheKey

	// cache-aside 读：先查 Redis
	if cached := strategyCacheGet(ctx, cacheKey); cached != nil && len(cached) > 0 {
		s := cached[0]
		return &s
	}

	// cache miss：查 DB
	conn, _ := GetDB()
	if conn == nil {
		return nil
	}
	var data string
	err := conn.QueryRow("SELECT data FROM sys_strategy WHERE user_id = 0").Scan(&data)
	if err != nil {
		log.Printf("[strategy] 加载全局系统策略失败: %v", err)
		return nil
	}
	var strategies []model.Strategy
	if err := json.Unmarshal([]byte(data), &strategies); err != nil {
		log.Printf("[strategy] 全局系统策略 JSON 解析失败: %v", err)
		return nil
	}
	if len(strategies) == 0 {
		return nil
	}

	// 回写缓存
	strategyCacheSet(ctx, cacheKey, strategies)
	s := strategies[0]
	return &s
}

// SaveGlobalSystemStrategy 管理员保存全局系统策略（user_id=0）
// strategies 应为单条系统策略（type=system）
func SaveGlobalSystemStrategy(strategies []model.Strategy) {
	conn, _ := GetDB()
	if conn == nil {
		return
	}
	data := toJSON(strategies)
	// upsert：先删后插，保持 user_id=0 唯一
	conn.Exec("DELETE FROM sys_strategy WHERE user_id = 0")
	conn.Exec("INSERT INTO sys_strategy (user_id, data) VALUES (0, ?)", data)
	// cache-aside 写：DB 成功后删缓存
	strategyCacheDel(context.Background(), strategyGlobalCacheKey)
	log.Printf("[strategy] 全局系统策略已保存，options=%d", len(strategies[0].Options))
}

// GetGlobalSystemStrategy 对外暴露的查询接口（handler 调用）
// 返回切片形式（与前端策略列表数据结构一致）
func GetGlobalSystemStrategy() []model.Strategy {
	s := loadGlobalSystemStrategy()
	if s == nil {
		// 兜底：DB 无记录时返回 SystemStrategyOptions 默认值（只读）
		return []model.Strategy{{
			ID:      "global_system",
			Name:    "系统接管",
			Type:    "system",
			Active:  true,
			Options: SystemStrategyOptions(),
		}}
	}
	return []model.Strategy{*s}
}

// CreateSystemStrategy 为用户创建并激活系统接管策略
// [已废弃] 全局系统策略改造后，用户不再持有 system 策略，无激活策略时自动走全局系统策略
// 保留函数签名仅为向后兼容，实际为空操作
func CreateSystemStrategy(userID int) {
	// no-op：全局系统策略由管理员在 user_id=0 行维护，用户无需创建
}

func loadStrategiesFromDB(userID int, isAdmin bool) []model.Strategy {
	// admin 查询所有策略不走缓存（数据量大且需要跨用户聚合）
	if !isAdmin {
		// cache-aside 读：先查 Redis
		ctx := context.Background()
		cacheKey := strategyCacheKey(userID)
		if cached := strategyCacheGet(ctx, cacheKey); cached != nil {
			return cached
		}
		// cache miss：查 DB 并回写
		dbResult := loadStrategiesFromDBDirect(userID, false)
		if dbResult != nil {
			strategyCacheSet(ctx, cacheKey, dbResult)
		}
		return dbResult
	}
	return loadStrategiesFromDBDirect(userID, true)
}

// loadStrategiesFromDBDirect 直查 MySQL（不走缓存）
func loadStrategiesFromDBDirect(userID int, isAdmin bool) []model.Strategy {
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
	// cache-aside 写：DB 成功后删缓存，下次读自动重建
	strategyCacheDel(context.Background(), strategyCacheKey(userID))
}

func GetStrategies(userID int, isAdmin bool) []model.Strategy {
	return loadStrategiesFromDB(userID, isAdmin)
}

func SaveStrategy(s model.Strategy, userID int) {
	// 全局系统策略改造后，用户策略只允许 fixed/round_robin
	// system 类型由管理员通过 SaveGlobalSystemStrategy 维护，普通保存强制改为 round_robin
	if s.Type == "system" {
		s.Type = "round_robin"
	}
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

// getEnabledKeyIDs 查询所有 enabled=1 的 keyID 集合（一次查库，多次判断）
func getEnabledKeyIDs() map[string]bool {
	conn, err := GetDB()
	if err != nil {
		return nil
	}
	rows, err := conn.Query("SELECT id FROM sys_api_key WHERE enabled = 1")
	if err != nil {
		return nil
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var id int
		rows.Scan(&id)
		m[fmt.Sprintf("%d", id)] = true
	}
	return m
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
	// 无激活策略 → 走全局系统策略（管理员维护，user_id=0）
	if active == nil || len(active.Options) == 0 {
		active = loadGlobalSystemStrategy()
		if active == nil || len(active.Options) == 0 {
			return nil
		}
	}

	// 预查 enabled key 列表，禁用的 key 一开始就不参与轮询
	enabledKeys := getEnabledKeyIDs()

	var selected *model.StrategyOption
	if active.Type == "round_robin" || active.Type == "system" {
		total := len(active.Options)
		userCounter := getUserRRCounter(userID)
		for i := 0; i < total; i++ {
			// 按用户独立计数，严格按定义顺序轮询 A→B→C→A→B→C
			idx := int(atomic.AddInt64(userCounter, 1)-1) % total
			opt := active.Options[idx]
			// 跳过：额度耗尽 或 key 已禁用
			if !IsKeyExhausted(opt.KeyID) && enabledKeys[opt.KeyID] {
				s := opt
				selected = &s
				break
			}
		}
	} else {
		// fixed 策略：如果第一个 key 被禁用，直接返回 nil（交由故障转移处理）
		selected = &active.Options[0]
		if !enabledKeys[selected.KeyID] {
			selected = nil
		}
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
	// 无激活策略 → 走全局系统策略做故障转移
	if active == nil {
		active = loadGlobalSystemStrategy()
		if active == nil {
			return nil
		}
	}

	// 收集所有可用模型
	enabledKeys := getEnabledKeyIDs()
	var available []model.StrategyOption
	for _, opt := range active.Options {
		if IsKeyExhausted(opt.KeyID) || !enabledKeys[opt.KeyID] {
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

	case "round_robin", "system":
		// 轮询/系统接管策略失败 → 按用户定义顺序返回，自然跳到下一个
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
	// 路由解析时不带 enabled 过滤：软删除的 key 仍可能被历史策略引用
	// 实际有效性由 IsKeyExhausted 和故障转移控制，这里只负责拿 api_key 和名称
	if err := conn.QueryRow("SELECT api_key, name FROM sys_api_key WHERE id = ?", opt.KeyID).Scan(&apiKey, &keyName); err != nil || apiKey == "" {
		return nil
	}

	// 查模型的 max_tokens（按 vendor_id + model_id 精确定位，同模型不同厂商上限可能不同）
	// 实测：智谱 glm-5.2 上限 131072，火山 glm-5.2 上限 128000，deepseek-v4-pro 上限 393216
	var maxTokens sql.NullInt64
	conn.QueryRow("SELECT max_tokens FROM sys_model WHERE vendor_id = ? AND model_id = ?", opt.VendorID, opt.ModelID).Scan(&maxTokens)

	ri := &model.RouteInfo{
		VendorID:   opt.VendorID,
		VendorName: vendorName,
		BaseURL:    baseURL,
		APIKey:     apiKey,
		KeyID:      opt.KeyID,
		KeyName:    keyName,
		ModelID:    opt.ModelID,
		MaxTokens:  int(maxTokens.Int64),
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
