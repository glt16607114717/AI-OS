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
      ],
    },
  ],
})

export default router
