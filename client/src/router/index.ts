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

      ],
    },
  ],
})

export default router
