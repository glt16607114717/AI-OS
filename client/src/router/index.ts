import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
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

      ],
    },
  ],
})

export default router
