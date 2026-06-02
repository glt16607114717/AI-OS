import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('../views/Home.vue'),
    },
    {
      path: '/voice',
      name: 'voice',
      component: () => import('../views/VoiceAssistant.vue'),
    },
  ],
})

export default router
