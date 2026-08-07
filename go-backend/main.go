package main

import (
	// "ai-os-server/circuit" // [DISABLED 2025-07-22] 熔断机制暂时关闭
	"ai-os-server/config"
	"ai-os-server/handler"
	"ai-os-server/middleware"
	"ai-os-server/service"
	"fmt"
	"log"
	"net/http"
	// pprof 仅用于排查内存泄漏，注册到 DefaultServeMux，由下方独立端口 127.0.0.1:16060 暴露
	// 抓完 heap profile 后这行连同下方 goroutine 一并删除
	_ "net/http/pprof"
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
	service.EnsureAuditTables()    // 知识库巡检表
	service.EnsureImageGenLogTable() // 图片生成日志表
	handler.EnsurePrankTable()
	service.EnsureOtherSettingTable()

	// 启动额度监控
	service.StartQuotaMonitor()
	log.Println("[init] 额度监控已启动")

	// 启动智谱 5 小时窗口自动锚定调度器
	// 每天 6 点主动锚定，让高峰自动分摊到多个窗口
	service.StartZhipuAnchor()
	log.Println("[init] 智谱窗口自动锚定调度器已启动（每日 06:00 锚定）")

	// [DISABLED 2025-07-22] API key 大量熔断，暂时关闭熔断机制
	// cb := circuit.Init(service.GetDB)
	// go cb.StartMonitor()
	// log.Println("[init] 熔断器已启动")

	// 启动每日知识蒸馏（改为晚上 11 点，避免抢白天的锚点窗口）
	// 蒸馏会走智谱 API，锚定夜间窗口 23:00-04:00
	c := cron.New()
	c.AddFunc("0 23 * * *", func() {
		log.Println("[cron] 开始执行每日知识蒸馏...")
		if err := service.RunDailyDistill(); err != nil {
			log.Printf("[cron] 蒸馏失败: %v", err)
		} else {
			log.Println("[cron] 每日知识蒸馏完成")
		}
	})

	// 启动每日知识库巡检（改为晚上 11:30，与蒸馏错开，同样锚定夜间窗口）
	c.AddFunc("30 23 * * *", func() {
		log.Println("[cron] 开始执行每日知识库巡检...")
		if err := service.RunDailyAudit(); err != nil {
			log.Printf("[cron] 巡检失败: %v", err)
		} else {
			log.Println("[cron] 每日知识库巡检完成")
		}
	})

	// 启动每日对话历史清理（凌晨 1 点，保留 7 天）
	// 避免每天插入几千条时重复执行，改为凌晨一次定时清理
	c.AddFunc("0 1 * * *", func() {
		log.Println("[cron] 开始清理 7 天前的对话历史...")
		deleted := service.CleanupOldChatHistory()
		if deleted < 0 {
			log.Println("[cron] 对话历史清理失败")
		} else {
			log.Printf("[cron] 对话历史清理完成: 删除 %d 条", deleted)
		}
	})

	// 启动每日 dump 文件清理（凌晨 4 点，蒸馏后 1 小时）
	// 性能优化：请求时不再清理 dump 文件（避免并发 panic + 磁盘 IO 锁竞争）
	// 改为凌晨单线程定时清理，单日累积约 2.8G 完全可接受
	c.AddFunc("0 4 * * *", func() {
		service.CleanupDumpFiles()
	})

	c.Start()
	log.Println("[init] 定时任务已启动（晚上23:00 蒸馏，23:30 巡检，凌晨4:00 清理 dump）")

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
		r.Get("/api/chat/sessions", handler.GetChatSessions)

		// 图片下载代理（解决跨域，仅允许智谱图片域名）
		r.Get("/api/image/download", handler.ImageDownload)

		// 技能管理
		r.Get("/api/skills", handler.GetBuiltinSkills)
		r.Get("/api/skills/{code}/connections", handler.GetSkillConnections)

		// AI 建议 & 蒸馏知识
		r.Get("/api/ai-advisor/suggestions", handler.GetSuggestions)
		r.Post("/api/ai-advisor/process", handler.MarkSuggestionProcessed)
		r.Get("/api/ai-advisor/knowledge", handler.GetDistillKnowledge)
		r.Post("/api/ai-advisor/trigger", handler.TriggerDistill)     // 手动触发蒸馏
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
		r.Get("/api/stats/token-daily", handler.GetTokenDailyStats)
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
		// 全部选项（含禁用 key），供策略名称翻译使用
		r.Get("/api/llm/all-options", handler.GetAllOptions)
		r.Get("/api/llm/strategies", handler.GetStrategies)
		r.Post("/api/llm/save-strategy", handler.SaveStrategy)
		r.Post("/api/llm/delete-strategy", handler.DeleteStrategy)
		r.Post("/api/llm/set-active-strategy", handler.SetActiveStrategy)
		// 全局系统策略（管理员维护，user_id=0，全局生效）
		r.Get("/api/llm/global-system-strategy", handler.GetGlobalSystemStrategy)
		r.Post("/api/llm/global-system-strategy", handler.SaveGlobalSystemStrategy)

		// 用户管理（普通用户可访问，handler 内做权限隔离）
		r.Get("/api/users", handler.ListUsers)
		r.Post("/api/users/update-password", handler.UpdateUserPassword)

		// 额度监控（普通用户可强制检查自己的额度）
		r.Post("/api/quota/force-check", handler.ForceQuotaCheck)

		// 退出
		r.Post("/api/logout", handler.Logout)

		// 需求管理（所有登录用户均可访问，不做权限校验）
		r.Get("/api/requirements/my", handler.GetMyRequirements)
		r.Get("/api/requirements/all", handler.GetAllRequirementsAdmin)
		r.Post("/api/requirements/update-status", handler.UpdateRequirementStatus)
		r.Post("/api/requirements/withdraw", handler.WithdrawRequirement)

		// Bug 管理（所有登录用户均可访问，不做权限校验）
		r.Get("/api/bugs/my", handler.GetMyBugs)
		r.Get("/api/bugs/all", handler.GetAllBugs)
		r.Post("/api/bugs/update-status", handler.UpdateBugStatus)
		r.Post("/api/bugs/withdraw", handler.WithdrawBug)

		// 知识库巡检（只读路由对所有登录用户开放，写操作在管理员组）
		r.Get("/api/audit/reports", handler.GetAuditReports)
		r.Get("/api/audit/reports/{id}", handler.GetAuditReportDetail)
		r.Get("/api/audit/pending-archives", handler.GetPendingArchives)
	})

	// ── 需要管理员 ──
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAdmin)

		// LLM 配置管理
		r.Post("/api/llm/vendor-keys", handler.SaveVendorKeys)
		r.Post("/api/llm/toggle-vendor", handler.ToggleVendor)

		// 额度监控
		r.Post("/api/quota/set-enabled", handler.SetQuotaEnabled)

		// 用户管理（仅管理员）
		r.Post("/api/users/create", handler.CreateUser)
		r.Post("/api/users/toggle-status", handler.ToggleUserStatus)
		r.Post("/api/users/toggle-admin", handler.ToggleUserAdmin)
		r.Post("/api/users/delete", handler.DeleteUser)

		// 技能管理（管理员）
		r.Post("/api/skills/{code}/connection", handler.CreateSkillConnection)
		r.Put("/api/skills/{code}/connection/{id}", handler.UpdateSkillConnection)
		r.Delete("/api/skills/{code}/connection/{id}", handler.DeleteSkillConnection)
		r.Get("/api/skills/{code}/permissions", handler.GetSkillPermissions)
		r.Post("/api/skills/{code}/permission", handler.SetSkillPermission)

		// 逗你玩（管理员恶搞）
		r.Get("/api/prank/list", handler.GetPrankList)
		r.Post("/api/prank/save", handler.SavePrank)
		r.Delete("/api/prank/delete", handler.DeletePrank)

		// 其他设置（管理员）
		r.Get("/api/other-setting", handler.GetOtherSettingHandler)
		r.Post("/api/other-setting", handler.SaveOtherSettingHandler)

		// 用户类型切换（管理员）
		r.Post("/api/users/toggle-user-type", handler.ToggleUserType)

		// 知识库巡检（管理员写操作）
		r.Post("/api/audit/run", handler.RunAuditManually)
		r.Post("/api/audit/pending-archives/{id}/approve", handler.ApprovePendingArchive)
		r.Post("/api/audit/pending-archives/{id}/reject", handler.RejectPendingArchive)
	})

	// ── pprof 诊断端点（仅本机 127.0.0.1，排查内存泄漏用，抓完即删）──
	// 独立端口 16060，用 DefaultServeMux（pprof init 已注册 handler）
	// 不绑 0.0.0.0，外网和 Nginx 都访问不到，仅 SSH 进本机可 curl
	go func() {
		log.Println("[pprof] 诊断端点启动: http://127.0.0.1:16060/debug/pprof/")
		if err := http.ListenAndServe("127.0.0.1:16060", nil); err != nil {
			log.Printf("[pprof] 诊断端点异常: %v", err)
		}
	}()

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
