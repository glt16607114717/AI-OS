<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const agentOnline = ref(false)
const devExpanded = ref(false)

const isDevActive = computed(() => route.path.startsWith('/dev'))

watch(isDevActive, (val) => {
  if (val) devExpanded.value = true
}, { immediate: true })

function toggleDevMenu() {
  devExpanded.value = !devExpanded.value
}

let healthTimer: ReturnType<typeof setInterval> | null = null

async function checkHealth() {
  try {
    const res = await window.aiOS.agentHealth()
    agentOnline.value = res?.ok ?? false
  } catch {
    agentOnline.value = false
  }
}

onMounted(() => {
  checkHealth()
  healthTimer = setInterval(checkHealth, 10000)
})

onUnmounted(() => {
  if (healthTimer) clearInterval(healthTimer)
})
</script>

<template>
  <div class="home-layout">
    <!-- Sidebar -->
    <aside class="sidebar">
      <!-- Ambient glow effects -->
      <div class="sidebar-glow-top"></div>
      <div class="sidebar-glow-bottom"></div>

      <!-- Logo -->
      <div class="sidebar-brand">
        <div class="brand-icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" fill="url(#bolt-grad)" />
            <defs>
              <linearGradient id="bolt-grad" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
                <stop offset="0%" stop-color="#06b6d4" />
                <stop offset="100%" stop-color="#a78bfa" />
              </linearGradient>
            </defs>
          </svg>
        </div>
        <span class="brand-text">AI-OS</span>
      </div>

      <!-- Navigation -->
      <nav class="sidebar-nav">
        <div class="nav-label">功能</div>

        <!-- Voice Assistant -->
        <router-link to="/voice" class="nav-item" active-class="active">
          <div class="nav-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
              <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
              <line x1="12" y1="19" x2="12" y2="23" />
              <line x1="8" y1="23" x2="16" y2="23" />
            </svg>
          </div>
          <span class="nav-text">语音助手</span>
        </router-link>

        <!-- Dev Assistant (expandable) -->
        <div class="nav-group">
          <div class="nav-item" :class="{ active: isDevActive }" @click="toggleDevMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="16 18 22 12 16 6" />
                <polyline points="8 6 2 12 8 18" />
              </svg>
            </div>
            <span class="nav-text">开发助手</span>
            <svg class="nav-arrow" :class="{ expanded: devExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <transition name="sub-slide">
            <div v-show="devExpanded" class="nav-sub">
              <router-link to="/dev/json" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                JSON 格式化
              </router-link>
              <router-link to="/dev/timestamp" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                时间戳转换
              </router-link>
              <router-link to="/dev/ip" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                IP 展示
              </router-link>
            </div>
          </transition>
        </div>
      </nav>

      <!-- Bottom status -->
      <div class="sidebar-footer">
        <div class="status-indicator" :class="agentOnline ? 'online' : 'offline'">
          <span class="status-dot"></span>
          <span class="status-text">{{ agentOnline ? '服务运行中' : '服务离线' }}</span>
        </div>
      </div>
    </aside>

    <!-- Content Area -->
    <main class="content-area">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.home-layout {
  display: flex;
  height: calc(100% + 40px);
  margin: -20px;
  overflow: hidden;
}

/* ====== Sidebar ====== */
.sidebar {
  width: 240px;
  flex-shrink: 0;
  background: linear-gradient(180deg, #0f172a 0%, #161033 50%, #1e1b4b 100%);
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
  z-index: 1;
}

.sidebar-glow-top {
  position: absolute;
  top: -80px;
  left: -40px;
  width: 200px;
  height: 160px;
  background: radial-gradient(ellipse, rgba(6, 182, 212, 0.07) 0%, transparent 70%);
  pointer-events: none;
}

.sidebar-glow-bottom {
  position: absolute;
  bottom: -60px;
  right: -60px;
  width: 200px;
  height: 180px;
  background: radial-gradient(ellipse, rgba(139, 92, 246, 0.06) 0%, transparent 70%);
  pointer-events: none;
}

/* Brand */
.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 20px 20px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  position: relative;
}

.brand-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(6, 182, 212, 0.12), rgba(139, 92, 246, 0.12));
  display: flex;
  align-items: center;
  justify-content: center;
}

.brand-text {
  font-size: 16px;
  font-weight: 700;
  color: #e2e8f0;
  letter-spacing: 3px;
}

/* Navigation */
.sidebar-nav {
  flex: 1;
  padding: 16px 12px;
  overflow-y: auto;
}

.sidebar-nav::-webkit-scrollbar {
  width: 4px;
}

.sidebar-nav::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar-nav::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 2px;
}

.nav-label {
  font-size: 10px;
  font-weight: 600;
  color: #475569;
  text-transform: uppercase;
  letter-spacing: 2px;
  padding: 0 8px 10px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  color: #8892a8;
  cursor: pointer;
  transition: all 0.2s ease;
  text-decoration: none;
  position: relative;
  margin-bottom: 2px;
  user-select: none;
}

.nav-item:hover {
  background: rgba(255, 255, 255, 0.04);
  color: #c8cee0;
}

.nav-item.active {
  background: linear-gradient(90deg, rgba(99, 102, 241, 0.1) 0%, rgba(139, 92, 246, 0.04) 100%);
  color: #e0e7ff;
}

.nav-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 6px;
  bottom: 6px;
  width: 3px;
  border-radius: 0 3px 3px 0;
  background: linear-gradient(180deg, #6366f1, #8b5cf6);
}

.nav-icon {
  width: 20px;
  height: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.nav-text {
  font-size: 13px;
  font-weight: 500;
}

.nav-arrow {
  margin-left: auto;
  transition: transform 0.25s ease;
  opacity: 0.4;
}

.nav-arrow.expanded {
  transform: rotate(180deg);
}

/* Sub menu */
.nav-sub {
  padding: 4px 0 4px 22px;
}

.nav-sub-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 6px;
  color: #6b7394;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
  text-decoration: none;
}

.nav-sub-item:hover {
  color: #a5b4fc;
  background: rgba(255, 255, 255, 0.03);
}

.nav-sub-item.active {
  color: #c4b5fd;
  background: rgba(139, 92, 246, 0.08);
}

.sub-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.4;
  transition: all 0.2s;
}

.nav-sub-item.active .sub-dot {
  opacity: 1;
  background: #a78bfa;
  box-shadow: 0 0 6px rgba(167, 139, 250, 0.4);
}

/* Sub menu transition */
.sub-slide-enter-active {
  transition: all 0.25s ease-out;
}
.sub-slide-leave-active {
  transition: all 0.2s ease-in;
}
.sub-slide-enter-from {
  opacity: 0;
  transform: translateY(-6px);
}
.sub-slide-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

/* Footer */
.sidebar-footer {
  padding: 16px 20px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  position: relative;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 500;
}

.status-indicator.online {
  color: #4ade80;
}

.status-indicator.offline {
  color: #f87171;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}

.status-indicator.online .status-dot {
  box-shadow: 0 0 8px rgba(74, 222, 128, 0.5);
  animation: glow-pulse 2s ease-in-out infinite;
}

@keyframes glow-pulse {
  0%, 100% { box-shadow: 0 0 6px rgba(74, 222, 128, 0.4); }
  50% { box-shadow: 0 0 14px rgba(74, 222, 128, 0.7); }
}

/* ====== Content Area ====== */
.content-area {
  flex: 1;
  background: linear-gradient(160deg, #f8fafc 0%, #f1f5f9 50%, #eef2ff 100%);
  overflow: auto;
  padding: 28px 32px;
  position: relative;
}

/* Left edge shadow for depth */
.content-area::before {
  content: '';
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 12px;
  background: linear-gradient(90deg, rgba(0, 0, 0, 0.03), transparent);
  pointer-events: none;
  z-index: 1;
}
</style>
