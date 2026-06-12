package middleware

import (
	"ai-os-server/model"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	Sessions   = make(map[string]*model.Session) // token -> session
	UserTokens = make(map[int]string)             // user_id -> token
	authMutex  sync.RWMutex
)

// HashPassword 使用 bcrypt
func HashPassword(password string) string {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash)
}

// CheckPassword 校验密码（兼容旧的 SHA256 和新的 bcrypt）
func CheckPassword(password, hash string) bool {
	// 先尝试 bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
		return true
	}
	return false
}

// CreateSession 创建会话，单点互踢
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

	return token
}

// GetSession 从请求获取会话
func GetSession(r *http.Request) *model.Session {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		// 尝试从 cookie 获取
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

	authMutex.RLock()
	session, ok := Sessions[token]
	authMutex.RUnlock()

	if !ok {
		return nil
	}
	if time.Now().After(session.Expire) {
		authMutex.Lock()
		delete(Sessions, token)
		if uid := session.UserID; UserTokens[uid] == token {
			delete(UserTokens, uid)
		}
		authMutex.Unlock()
		return nil
	}
	return session
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
