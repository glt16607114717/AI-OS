import { createRouter, createWebHashHistory } from 'vue-router'
import { API_BASE } from '../api'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
    },
    {
      path: '/',
      component: () => import('../views/Home.vue'),
      redirect: '/workspace',
      children: [
        {
          path: 'workspace',
          name: 'Workspace',
          component: () => import('../views/ChatWorkspace.vue'),
        },
        {
          path: 'dev/json',
          name: 'dev-json',
          component: () => import('../views/dev/JsonFormatter.vue'),
        },
        {
          path: 'dev/timestamp',
          name: 'dev-timestamp',
          component: () => import('../views/dev/TimestampConverter.vue'),
        },
        {
          path: 'llm/config',
          name: 'llm-config',
          component: () => import('../views/llm/VendorConfig.vue'),
        },
        {
          path: 'llm/strategy',
          name: 'LLMStrategy',
          component: () => import('../views/llm/StrategyEditor.vue'),
        },
        {
          path: 'llm/stats',
          name: 'LlmStats',
          component: () => import('../views/llm/StatsDashboard.vue'),
        },
        {
          path: 'llm/god-rules',
          name: 'LlmGodRules',
          component: () => import('../views/llm/GodRules.vue'),
        },
        {
          path: 'llm/log',
          name: 'LlmLog',
          component: () => import('../views/llm/LlmLog.vue'),
        },
        {
          path: 'llm/quota',
          name: 'LlmQuota',
          component: () => import('../views/llm/QuotaMonitor.vue'),
        },
        {
          path: 'llm/ai-advice',
          name: 'LlmAiAdvice',
          component: () => import('../views/llm/AiAdvice.vue'),
        },
        {
          path: 'llm/errors',
          name: 'LlmErrors',
          component: () => import('../views/llm/ErrorLog.vue'),
        },
        {
          path: 'rag/config',
          name: 'RagConfig',
          component: () => import('../views/rag/RagConfig.vue'),
        },
        {
          path: 'rag/knowledge',
          name: 'Knowledge',
          component: () => import('../views/rag/Knowledge.vue'),
        },
        {
          path: 'rag/daily-report',
          name: 'DailyReport',
          component: () => import('../views/rag/DailyReport.vue'),
        },
        {
          path: 'rag/audit-report',
          name: 'AuditReport',
          component: () => import('../views/rag/AuditReport.vue'),
        },
        {
          path: 'system/account',
          name: 'AccountSettings',
          component: () => import('../views/system/AccountSettings.vue'),
        },
        {
          path: 'system/prank',
          name: 'Prank',
          component: () => import('../views/llm/Prank.vue'),
        },
        {
          path: 'system/other',
          name: 'OtherSettings',
          component: () => import('../views/system/OtherSettings.vue'),
        },
        {
          path: 'skills/mysql_query',
          name: 'MySQLQuerySettings',
          component: () => import('../views/skills/MySQLQuerySettings.vue'),
        },
        {
          path: 'requirements/my',
          name: 'MyRequirements',
          component: () => import('../views/requirements/MyRequirements.vue'),
        },
        {
          path: 'requirements/inbox',
          name: 'RequirementInbox',
          component: () => import('../views/requirements/RequirementInbox.vue'),
        },
        {
          path: 'bugs/my',
          name: 'MyBugs',
          component: () => import('../views/bugs/MyBugs.vue'),
        },
        {
          path: 'bugs/inbox',
          name: 'BugInbox',
          component: () => import('../views/bugs/BugInbox.vue'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const token = localStorage.getItem('aios_token')

  if (to.path === '/login') {
    if (token) {
      try {
        const res = await fetch(`${API_BASE}/api/system/me`, {
          headers: { Authorization: `Bearer ${token}` },
        })
        const data = await res.json()
        if (data.ok && data.data) {
          localStorage.setItem('aios_user_type', data.data.user_type || '')
        }
        if (data.ok) {
          return '/workspace'
        } else {
          console.warn('[Router] Token 验证失败:', data.error)
          localStorage.removeItem('aios_token')
          localStorage.removeItem('aios_user_type')
        }
      } catch (e: any) {
        console.error('[Router] Token 验证异常:', e)
        localStorage.removeItem('aios_token')
      }
    }
    return true
  }

  if (!token) {
    console.warn('[Router] 未登录，跳转到登录页')
    return '/login'
  }

  try {
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    const data = await res.json()
    if (!data.ok) {
      console.warn('[Router] Token 已失效，跳转到登录页:', data.error)
      localStorage.removeItem('aios_token')
      localStorage.removeItem('aios_user_type')
      return '/login'
    }
    if (data.data) {
      localStorage.setItem('aios_user_type', data.data.user_type || '')
    }
  } catch (e: any) {
    console.error('[Router] 验证 token 时发生错误:', e)
    return '/login'
  }

  // business 用户访问范围：工作台、需求管理（我的需求+收件箱）
  const userType = localStorage.getItem('aios_user_type')
  if (userType === 'business') {
    const allowedPaths = ['/workspace', '/requirements/my', '/requirements/inbox', '/bugs/my', '/bugs/inbox', '/login']
    if (!allowedPaths.some(p => to.path.startsWith(p))) {
      return '/workspace'
    }
  }

  return true
})

// 捕获动态导入失败（chunk 文件因部署更新而 404）
// 表现：点击菜单无反应，控制台报 "Failed to fetch dynamically imported module"
// 处理：自动刷新页面（重新请求 index.html 拿到新的 chunk 文件名）
// 加防抖：避免连续多次失败导致循环刷新
let reloadGuard = false
router.onError((error) => {
  if (
    error instanceof TypeError &&
    error.message.includes('Failed to fetch dynamically imported module')
  ) {
    console.error('[Router] 动态导入失败（可能是部署更新导致），自动刷新页面')
    if (!reloadGuard) {
      reloadGuard = true
      window.location.reload()
    }
  }
})

export default router
