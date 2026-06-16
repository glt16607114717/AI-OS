package main

import (
	"ai-os-server/config"
	"ai-os-server/handler"
	"ai-os-server/middleware"
	"ai-os-server/service"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/robfig/cron/v3"
)

func main() {
	// 配置
	config.Init()
	port := os.Getenv("AIOS_PORT")
	if port == "" {
		port = "18731"
	}

	// 初始化数据库表
	log.Println("[init] 正在初始化数据库表...")
	log.Println("[init] 数据库表初始化完成")

	// 注入 API Key 查询函数（避免循环依赖）
	middleware.APIKeyLookup = service.GetUserByAPIKey

	// 启动额度监控
	service.StartQuotaMonitor()
	log.Println("[init] 额度监控已启动")

	// 启动 AI 建议定时任务
	c := cron.New()
	c.AddFunc("0 0 18 * * *", func() {
		log.Println("[cron] 开始执行 AI 建议分析...")
		if err := service.RunAIAnalysis(); err != nil {
			log.Printf("[cron] AI 分析失败: %v", err)
		} else {
			log.Println("[cron] AI 建议分析完成")
		}
	})
	c.Start()
	log.Println("[init] 定时任务已启动（每天18:00 AI分析）")

	// 路由
	r := chi.NewRouter()

	// 全局中间件
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware)

	// ── 公开接口 ──
	r.Post("/api/login", handler.Login)
	r.Get("/api/health", handler.HealthCheck)
	r.Get("/api/system/me", handler.SystemMe)

	// ── 需要登录 ──
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		// LLM 代理
		r.Post("/v1/chat/completions", handler.ProxyChatCompletions)
		r.Get("/v1/models", handler.ProxyModels)
		r.Post("/api/workspace/chat", handler.WorkspaceChat)

		// 聊天历史
		r.Get("/api/chat/history", handler.GetChatHistory)
		r.Post("/api/chat/clear", handler.ClearChatHistory)

		// AI 建议
		r.Get("/api/ai-advisor/suggestions", handler.GetSuggestions)
		r.Post("/api/ai-advisor/analyze", handler.RunAIAnalysis)
		r.Post("/api/ai-advisor/process", handler.MarkSuggestionProcessed)

		// 技能
		r.Get("/api/skills", handler.GetSkillList)

		// RAG 知识库
		r.Get("/api/rag/status", handler.RagStatus)
		r.Get("/api/rag/list", handler.RagList)
		r.Post("/api/rag/test-embed", handler.RagTestEmbed)
		r.Post("/api/rag/search", handler.RagSearch)

		// 统计
		r.Get("/api/stats/summary", handler.GetStatsSummary)
		r.Get("/api/stats/errors", handler.GetRecentErrors)
		r.Post("/api/stats/cleanup", handler.CleanupStats)

		// 日志
		r.Get("/api/logs", handler.GetLogs)
		r.Post("/api/logs/clear", handler.ClearLogs)

		// 上帝指令
		r.Get("/api/god-rules", handler.GetGodRules)

		// 额度监控
		r.Get("/api/quota/status", handler.GetQuotaStatus)

		// LLM 配置
		r.Get("/api/llm/catalog", handler.GetCatalog)
		r.Get("/api/llm/available-options", handler.GetAvailableOptions)
		r.Get("/api/llm/strategies", handler.GetStrategies)

		// 退出
		r.Post("/api/logout", handler.Logout)
	})

	// ── 需要管理员 ──
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin)

		// LLM 配置管理
		r.Post("/api/llm/vendor-keys", handler.SaveVendorKeys)
		r.Post("/api/llm/toggle-vendor", handler.ToggleVendor)
		r.Post("/api/llm/save-strategy", handler.SaveStrategy)
		r.Post("/api/llm/delete-strategy", handler.DeleteStrategy)
		r.Post("/api/llm/set-active-strategy", handler.SetActiveStrategy)

		// 上帝指令
		r.Post("/api/god-rules/save", handler.SaveGodRules)

		// 额度监控
		r.Post("/api/quota/set-enabled", handler.SetQuotaEnabled)
		r.Post("/api/quota/force-check", handler.ForceQuotaCheck)

		// 用户管理
		r.Get("/api/users", handler.ListUsers)
		r.Post("/api/users/create", handler.CreateUser)
		r.Post("/api/users/update-password", handler.UpdateUserPassword)
		r.Post("/api/users/toggle-status", handler.ToggleUserStatus)
		r.Post("/api/users/toggle-admin", handler.ToggleUserAdmin)
		r.Post("/api/users/delete", handler.DeleteUser)
	})

	// 启动
	addr := fmt.Sprintf("0.0.0.0:%s", port)
	log.Printf("========================================")
	log.Printf("  AI-OS Server v2.0 (Go)")
	log.Printf("  Listening on http://%s", addr)
	log.Printf("  MySQL: %s:%d/%s", config.DB.Host, config.DB.Port, config.DB.Database)
	log.Printf("========================================")

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

// CORS 中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
