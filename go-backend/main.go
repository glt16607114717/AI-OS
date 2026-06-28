package main

import (
	"ai-os-server/circuit"
	"ai-os-server/config"
	"ai-os-server/handler"
	"ai-os-server/middleware"
	"ai-os-server/service"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/robfig/cron/v3"
)

func main() {
	// 清理过期对话日志，每小时一次
	go func() {
		service.CleanupOldConversationLogs()
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			service.CleanupOldConversationLogs()
		}
	}()
	// 配置
	config.Init()
	port := os.Getenv("AIOS_PORT")
	if port == "" {
		port = "18731"
	}

	// 初始化 Redis（Session 持久化）
	middleware.InitRedis()

	// 注入 API Key 查询函数（避免循环依赖）
	middleware.APIKeyLookup = service.GetUserByAPIKey

	// 确保技能表存在
	service.EnsureBuiltinSkillTables()
	service.EnsureSuggestionTable()
	service.EnsureDistillTable()
	service.EnsureChatTable()

	// 启动额度监控
	service.StartQuotaMonitor()
	log.Println("[init] 额度监控已启动")

	// 启动熔断器（独立 goroutine，每1分钟扫库检测）
	cb := circuit.Init(service.GetDB)
	go cb.StartMonitor()

	// 初始化 Qdrant 向量库（连接失败不阻塞启动，蒸馏/检索会降级）
	if err := service.InitQdrant(); err != nil {
		log.Printf("[init] Qdrant 初始化失败（知识库检索将不可用）: %v", err)
	}

	// 启动每日知识蒸馏（凌晨3点）
	c := cron.New()
	c.AddFunc("0 3 * * *", func() {
		log.Println("[cron] 开始执行每日知识蒸馏...")
		if err := service.RunDailyDistill(); err != nil {
			log.Printf("[cron] 蒸馏失败: %v", err)
		} else {
			log.Println("[cron] 每日知识蒸馏完成")
		}
	})
	c.Start()
	log.Println("[init] 定时任务已启动（每天凌晨3:00 蒸馏）")

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
	r.Post("/api/vision/recognize", handler.VisionRecognize)

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
		r.Get("/api/chat/download/{id}", handler.DownloadConversationLog)

		// 技能管理
		r.Get("/api/skills", handler.GetBuiltinSkills)
		r.Get("/api/skills/{code}/connections", handler.GetSkillConnections)

		// AI 建议 & 蒸馏知识
		r.Get("/api/ai-advisor/suggestions", handler.GetSuggestions)
		r.Post("/api/ai-advisor/process", handler.MarkSuggestionProcessed)
		r.Get("/api/ai-advisor/knowledge", handler.GetDistillKnowledge)
		r.Post("/api/ai-advisor/trigger", handler.TriggerDistill) // 手动触发蒸馏
		r.Get("/api/ai-advisor/distill-logs", handler.GetDistillLogs) // 蒸馏日志


		// RAG 知识库
		r.Get("/api/rag/status", handler.RagStatus)
		r.Get("/api/rag/list", handler.RagList)
		r.Get("/api/rag/files", handler.RagFiles)
		r.Post("/api/rag/upload", handler.RagUpload)
		r.Post("/api/rag/test-embed", handler.RagTestEmbed)
		r.Post("/api/rag/search", handler.RagSearch)
		r.Delete("/api/rag/delete", handler.RagDelete)

		// 工作日报
		r.Get("/api/work-diary/list", handler.GetWorkDiaries)
		r.Get("/api/work-diary/get", handler.GetWorkDiary)
		r.Post("/api/work-diary/save", handler.SaveWorkDiary)
		r.Delete("/api/work-diary/delete", handler.DeleteWorkDiary)

		// 统计
		r.Get("/api/stats/summary", handler.GetStatsSummary)
		r.Get("/api/stats/errors", handler.GetRecentErrors)
		r.Post("/api/stats/cleanup", handler.CleanupStats)

		// 错误日志
		r.Get("/api/llm/error-log", handler.GetErrorLog)
		r.Get("/api/llm/error-stats", handler.GetErrorStats)
		r.Get("/api/llm/circuit-status", handler.GetCircuitBreakerStatus)

		// 日志
		r.Get("/api/logs", handler.GetLogs)
		r.Post("/api/logs/clear", handler.ClearLogs)

		// 上帝指令
		r.Get("/api/god-rules", handler.GetGodRules)
		r.Post("/api/god-rules/save", handler.SaveGodRules)

		// 额度监控
		r.Get("/api/quota/status", handler.GetQuotaStatus)

		// LLM 配置
		r.Get("/api/llm/catalog", handler.GetCatalog)
		r.Get("/api/llm/available-options", handler.GetAvailableOptions)
		r.Get("/api/llm/strategies", handler.GetStrategies)
		r.Post("/api/llm/save-strategy", handler.SaveStrategy)
		r.Post("/api/llm/delete-strategy", handler.DeleteStrategy)
		r.Post("/api/llm/set-active-strategy", handler.SetActiveStrategy)

		// 退出
		r.Post("/api/logout", handler.Logout)
	})

	// ── 需要管理员 ──
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin)

		// LLM 配置管理
		r.Post("/api/llm/vendor-keys", handler.SaveVendorKeys)
		r.Post("/api/llm/toggle-vendor", handler.ToggleVendor)

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

		// 技能管理（管理员）
		r.Post("/api/skills/{code}/connection", handler.CreateSkillConnection)
		r.Put("/api/skills/{code}/connection/{id}", handler.UpdateSkillConnection)
		r.Delete("/api/skills/{code}/connection/{id}", handler.DeleteSkillConnection)
		r.Get("/api/skills/{code}/permissions", handler.GetSkillPermissions)
		r.Post("/api/skills/{code}/permission", handler.SetSkillPermission)

		// 知识库迁移（管理员一次性操作）
		r.Post("/api/rag/migrate", handler.RagMigrate)
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
