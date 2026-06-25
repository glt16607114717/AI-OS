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
          path: 'system/account',
          name: 'AccountSettings',
          component: () => import('../views/system/AccountSettings.vue'),
        },
        {
          path: 'skills/mysql_query',
          name: 'MySQLQuerySettings',
          component: () => import('../views/skills/MySQLQuerySettings.vue'),
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
        if (data.ok) {
          return '/workspace'
        } else {
          console.warn('[Router] Token 验证失败:', data.error)
          localStorage.removeItem('aios_token')
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
      return '/login'
    }
  } catch (e: any) {
    console.error('[Router] 验证 token 时发生错误:', e)
    return '/login'
  }

  return true
})

export default router
