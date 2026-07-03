package middleware

import (
	"ai-os-server/config"
	"ai-os-server/model"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// APIKeyLookup 由 service 包在初始化时注入，避免循环依赖
var APIKeyLookup func(apiKey string) (userID int, username string, isAdmin bool, err error)

var (
	Sessions   = make(map[string]*model.Session) // token -> session（内存缓存）
	UserTokens = make(map[int]string)             // user_id -> token（内存缓存）
	authMutex  sync.RWMutex
	rdb        *redis.Client
)

// GetRedis 导出 Redis 客户端供其他包使用
func GetRedis() *redis.Client {
	return rdb
}

// InitRedis 初始化 Redis 连接，并从 Redis 加载未过期的 session 到内存
func InitRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[auth] Redis 连接失败: %v，降级为纯内存模式", err)
		rdb = nil
		return
	}
	log.Println("[auth] Redis 连接成功")

	// 从 Redis 加载所有未过期的 session 到内存
	keys, err := rdb.Keys(ctx, "session:*").Result()
	if err != nil {
		log.Printf("[auth] 加载 Redis session 失败: %v", err)
		return
	}

	authMutex.Lock()
	defer authMutex.Unlock()

	loaded := 0
	for _, key := range keys {
		token := strings.TrimPrefix(key, "session:")
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var s model.Session
		if err := json.Unmarshal([]byte(data), &s); err != nil {
			continue
		}
		if time.Now().After(s.Expire) {
			rdb.Del(ctx, key)
			continue
		}
		Sessions[token] = &s
		UserTokens[s.UserID] = token
		loaded++
	}
	if loaded > 0 {
		log.Printf("[auth] 从 Redis 恢复 %d 个 session", loaded)
	}
}

// HashPassword 使用 bcrypt
func HashPassword(password string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash)
}

// CheckPassword 校验密码（兼容旧的 SHA256 和新的 bcrypt）
func CheckPassword(password, hash string) bool {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
		return true
	}
	return false
}

// CreateSession 创建会话，单点互踢，持久化到 Redis
func CreateSession(userID int, username string, isAdmin bool) string {
	token := generateToken()
	now := time.Now()
	expire := now.Add(30 * 24 * time.Hour)

	session := &model.Session{
		UserID:   userID,
		Username: username,
		IsAdmin:  isAdmin,
		Expire:   expire,
	}

	authMutex.Lock()
	// 踢掉旧会话
	if oldToken, ok := UserTokens[userID]; ok {
		delete(Sessions, oldToken)
	}
	Sessions[token] = session
	UserTokens[userID] = token
	authMutex.Unlock()

	// 持久化到 Redis
	if rdb != nil {
		data, _ := json.Marshal(session)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		rdb.Set(ctx, "session:"+token, data, 30*24*time.Hour)
	}

	return token
}

// GetSession 从请求获取会话（内存 → Redis → API Key 三级回退）
func GetSession(r *http.Request) *model.Session {
	// 方式1: URL query ?key=xxx（API Key 认证）
	if apiKey := r.URL.Query().Get("key"); apiKey != "" && APIKeyLookup != nil {
		if idx := strings.IndexByte(apiKey, '/'); idx > 0 {
			apiKey = apiKey[:idx]
		}
		uid, username, isAdmin, err := APIKeyLookup(apiKey)
		if err == nil && uid > 0 {
			return &model.Session{
				UserID:   uid,
				Username: username,
				IsAdmin:  isAdmin,
				Expire:   time.Now().Add(24 * time.Hour),
			}
		}
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		if c, err := r.Cookie("token"); err == nil {
			auth = c.Value
		}
	}
	token := auth
	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}
	if token == "" {
		return nil
	}

	// 1. 先查内存
	authMutex.RLock()
	session, ok := Sessions[token]
	authMutex.RUnlock()

	if ok {
		if time.Now().After(session.Expire) {
			removeSession(token, session.UserID)
			return nil
		}
		return session
	}

	// 2. 内存没命中，查 Redis
	if rdb != nil && !strings.HasPrefix(token, "sk-") {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		data, err := rdb.Get(ctx, "session:"+token).Result()
		if err == nil {
			var s model.Session
			if json.Unmarshal([]byte(data), &s) == nil {
				if time.Now().After(s.Expire) {
					rdb.Del(ctx, "session:"+token)
					return nil
				}
				// 回写到内存
				authMutex.Lock()
				Sessions[token] = &s
				UserTokens[s.UserID] = token
				authMutex.Unlock()
				return &s
			}
		}
	}

	// 3. 尝试作为 API Key（sk- 开头）
	if strings.HasPrefix(token, "sk-") && APIKeyLookup != nil {
		uid, username, isAdmin, err := APIKeyLookup(token)
		if err == nil && uid > 0 {
			return &model.Session{
				UserID:   uid,
				Username: username,
				IsAdmin:  isAdmin,
				Expire:   time.Now().Add(24 * time.Hour),
			}
		}
	}

	return nil
}

// removeSession 删除 session（内存 + Redis）
func removeSession(token string, userID int) {
	authMutex.Lock()
	delete(Sessions, token)
	if UserTokens[userID] == token {
		delete(UserTokens, userID)
	}
	authMutex.Unlock()

	if rdb != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rdb.Del(ctx, "session:"+token)
	}
}

// RequireAuth 鉴权中间件
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSession(r)
		if session == nil {
			http.Error(w, `{"ok":false,"error":"未登录"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), "session", session)
		ctx = context.WithValue(ctx, "token", getToken(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin 管理员中间件
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSession(r)
		if session == nil {
			http.Error(w, `{"ok":false,"error":"未登录"}`, http.StatusUnauthorized)
			return
		}
		if !session.IsAdmin {
			http.Error(w, `{"ok":false,"error":"需要管理员权限"}`, http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), "session", session)
		ctx = context.WithValue(ctx, "token", getToken(r))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetSessionFromCtx 从 context 获取会话
func GetSessionFromCtx(r *http.Request) *model.Session {
	if s, ok := r.Context().Value("session").(*model.Session); ok {
		return s
	}
	return nil
}

// ExportSessions 导出会话表（给 llm handler 用）
func ExportSessions() map[string]*model.Session {
	authMutex.RLock()
	defer authMutex.RUnlock()
	cp := make(map[string]*model.Session, len(Sessions))
	for k, v := range Sessions {
		cp[k] = v
	}
	return cp
}

// generateToken 使用 crypto/rand 生成真随机 token
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	if c, err := r.Cookie("token"); err == nil {
		return c.Value
	}
	return ""
}

// RegisterRoutes 注册占位（chi router 要求）
func RegisterRoutes(r chi.Router) {}
