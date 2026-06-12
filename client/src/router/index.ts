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
      redirect: '/voice',
      children: [
        {
          path: 'voice',
          name: 'voice',
          component: () => import('../views/VoiceAssistant.vue'),
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
          path: 'workspace',
          name: 'Workspace',
          component: () => import('../views/ChatWorkspace.vue'),
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
          path: 'system/account',
          name: 'AccountSettings',
          component: () => import('../views/system/AccountSettings.vue'),
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
          return '/voice'
        }
      } catch {
        // 验证失败，清除无效 token，留在登录页
        localStorage.removeItem('aios_token')
      }
    }
    return true
  }

  // 非 /login 页面，需要 token
  if (!token) {
    return '/login'
  }

  return true
})

export default router
