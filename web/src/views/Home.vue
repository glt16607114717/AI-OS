<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { API_BASE } from '../api'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()

const devExpanded = ref(false)
const llmExpanded = ref(false)
const ragExpanded = ref(false)
const systemExpanded = ref(false)
const skillExpanded = ref(false)
const isAdmin = ref(false)
const currentUser = ref('')
const activeStrategy = ref('') // 当前生效的策略名称

const isDevActive = computed(() => route.path.startsWith('/dev'))
const isLlmActive = computed(() => route.path.startsWith('/llm'))
const isRagActive = computed(() => route.path.startsWith('/rag'))
const isSystemActive = computed(() => route.path.startsWith('/system'))
const isSkillActive = computed(() => route.path.startsWith('/skills'))

watch(isDevActive, (val) => {
  if (val) devExpanded.value = true
}, { immediate: true })

watch(isLlmActive, (val) => {
  if (val) llmExpanded.value = true
}, { immediate: true })

watch(isRagActive, (val) => {
  if (val) ragExpanded.value = true
}, { immediate: true })

watch(isSystemActive, (val) => {
  if (val) systemExpanded.value = true
}, { immediate: true })

watch(isSkillActive, (val) => {
  if (val) skillExpanded.value = true
}, { immediate: true })

function toggleDevMenu() {
  devExpanded.value = !devExpanded.value
}

function toggleLlmMenu() {
  llmExpanded.value = !llmExpanded.value
}

function toggleRagMenu() {
  ragExpanded.value = !ragExpanded.value
}

function toggleSystemMenu() {
  systemExpanded.value = !systemExpanded.value
}

function toggleSkillMenu() {
  skillExpanded.value = !skillExpanded.value
}

async function checkAdmin() {
  try {
    const token = localStorage.getItem('aios_token') || ''
    if (!token) return
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    const data = await res.json()
    if (data?.ok && data?.data) {
      isAdmin.value = !!data.data.is_admin
      currentUser.value = data.data.username || ''
      // 获取当前用户的策略信息
      await fetchActiveStrategy()
    }
  } catch (e: any) {
    console.error('[Home] checkAdmin error:', e)
    isAdmin.value = false
  }
}

async function fetchActiveStrategy() {
  try {
    const token = localStorage.getItem('aios_token') || ''
    if (!token) return

    const res = await fetch(`${API_BASE}/api/llm/strategies`, {
      headers: { Authorization: `Bearer ${token}` }
    })

    const data = await res.json()
    if (data?.ok && data?.data) {
      // 找到当前激活的策略
      const active = data.data.find((s: any) => s.active)
      if (active) {
        // 如果是管理员，显示用户名，否则只显示策略名
        activeStrategy.value = isAdmin.value && active.username
          ? `${active.username} - ${active.name}`
          : active.name
      } else {
        activeStrategy.value = '未设置策略'
      }
    }
  } catch (e: any) {
    console.error('[Home] fetchActiveStrategy error:', e)
    activeStrategy.value = '获取失败'
  }
}

function logout() {
  localStorage.removeItem('aios_token')
  localStorage.removeItem('aios_username')
  localStorage.removeItem('aios_is_admin')
  router.push('/login')
}

// 监听路由变化，更新策略信息
watch(() => route.path, () => {
  if (currentUser.value) {
    fetchActiveStrategy()
  }
})

// 监听策略更新事件
const handleStrategyUpdate = () => {
  if (currentUser.value) {
    fetchActiveStrategy()
  }
}

onMounted(() => {
  checkAdmin()
  // 监听策略更新事件
  window.addEventListener('strategy-updated', handleStrategyUpdate)
})

// 组件卸载时移除事件监听
onBeforeUnmount(() => {
  window.removeEventListener('strategy-updated', handleStrategyUpdate)
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

        <!-- Chat Workspace -->
        <router-link to="/workspace" class="nav-item" active-class="active">
          <div class="nav-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            </svg>
          </div>
          <span class="nav-text">工作台</span>
        </router-link>

        <!-- RAG Knowledge (expandable) -->
        <div class="nav-group">
          <div class="nav-item" :class="{ active: isRagActive }" @click="toggleRagMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
                <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
                <line x1="8" y1="7" x2="16" y2="7" />
                <line x1="8" y1="11" x2="14" y2="11" />
              </svg>
            </div>
            <span class="nav-text">信息沉淀</span>
            <svg class="nav-arrow" :class="{ expanded: ragExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <transition name="sub-slide">
            <div v-show="ragExpanded" class="nav-sub">
              <router-link to="/rag/knowledge" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                知识库
              </router-link>
              <router-link to="/rag/daily-report" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                工作日报
              </router-link>
              <router-link to="/llm/stats" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                统计仪表
              </router-link>
              <router-link to="/llm/log" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                对话记录
              </router-link>
              <router-link to="/llm/ai-advice" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                建议
              </router-link>
            </div>
          </transition>
        </div>

        <!-- LLM (expandable) -->
        <div class="nav-group">
          <div class="nav-item" :class="{ active: isLlmActive }" @click="toggleLlmMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M12 2a4 4 0 0 1 4 4v2a4 4 0 0 1-8 0V6a4 4 0 0 1 4-4z" />
                <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
              </svg>
            </div>
            <span class="nav-text">大模型</span>
            <svg class="nav-arrow" :class="{ expanded: llmExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <transition name="sub-slide">
            <div v-show="llmExpanded" class="nav-sub">
              <router-link to="/llm/config" class="nav-sub-item" active-class="active" v-if="isAdmin">
                <span class="sub-dot"></span>
                基础配置
              </router-link>
              <router-link to="/llm/strategy" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                策略编辑
              </router-link>
              <router-link to="/llm/god-rules" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                上帝指令
              </router-link>
              <router-link to="/llm/quota" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                用量统计
              </router-link>
            </div>
          </transition>
        </div>

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
            </div>
          </transition>
        </div>

        <!-- Skills (expandable, admin only) -->
        <div class="nav-group" v-if="isAdmin">
          <div class="nav-item" :class="{ active: isSkillActive }" @click="toggleSkillMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
              </svg>
            </div>
            <span class="nav-text">技能</span>
            <svg class="nav-arrow" :class="{ expanded: skillExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <transition name="sub-slide">
            <div v-show="skillExpanded" class="nav-sub">
              <router-link to="/skills/mysql_query" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                MySQL 查询
              </router-link>
            </div>
          </transition>
        </div>

        <!-- System Settings (expandable, admin only) -->
        <div class="nav-group" v-if="isAdmin">
          <div class="nav-item" :class="{ active: isSystemActive }" @click="toggleSystemMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
            </div>
            <span class="nav-text">系统设置</span>
            <svg class="nav-arrow" :class="{ expanded: systemExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <transition name="sub-slide">
            <div v-show="systemExpanded" class="nav-sub">
              <router-link to="/system/account" class="nav-sub-item" active-class="active">
                <span class="sub-dot"></span>
                账户设置
              </router-link>
            </div>
          </transition>
        </div>
      </nav>

      <!-- Bottom status -->
      <div class="sidebar-footer">
        <div class="user-info" v-if="currentUser">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
            <circle cx="12" cy="7" r="4" />
          </svg>
          <span class="user-name">{{ currentUser }}</span>
          <el-tag v-if="isAdmin" size="small" type="danger" class="admin-tag">管理员</el-tag>
        </div>
        <div class="footer-actions">
          <button class="logout-btn" @click="logout" title="退出登录">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <polyline points="16 17 21 12 16 7" />
              <line x1="21" y1="12" x2="9" y2="12" />
            </svg>
          </button>
        </div>
      </div>
    </aside>

    <!-- Content Area -->
    <main class="content-area">
      <!-- 策略标识区域 -->
      <div class="strategy-indicator" v-if="activeStrategy">
        <div class="strategy-label">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
          </svg>
          <span>当前策略:</span>
        </div>
        <div class="strategy-name">{{ activeStrategy }}</div>
      </div>

      <div class="content-main">
        <router-view />
      </div>
      <div class="page-footer">
        <span>AI-OS v1.0.0</span>
      </div>
    </main>
  </div>
</template>

<style scoped>
.home-layout {
  display: flex;
  height: 100%;
  width: 100%;
  overflow: hidden;
}

/* ===== Sidebar ===== */
.sidebar {
  width: 240px;
  flex-shrink: 0;
  background: linear-gradient(180deg, #0f172a 0%, #161033 50%, #1e1b4b 100%);
  display: flex;
  flex-direction: column;
  position: relative;
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
  font-weight: 700;
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
  width: 8px;
  height: 8px;
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
  padding: 12px 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #a5b4fc;
  font-size: 12px;
  padding: 4px 4px 0;
}

.user-name {
  font-weight: 500;
}

.admin-tag {
  margin-left: auto;
  transform: scale(0.85);
}

.footer-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  justify-content: flex-end;
}

.logout-btn {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: none;
  background: rgba(255, 255, 255, 0.06);
  color: #8892a8;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}
.logout-btn:hover {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

/* ===== Content Area ===== */
.content-area {
  flex: 1;
  min-height: 0;
  background: linear-gradient(160deg, #f8fafc 0%, #f1f5f9 50%, #eef2ff 100%);
  display: flex;
  flex-direction: column;
  position: relative;
}

.content-main {
  flex: 1;
  overflow-y: auto;
  padding: 28px 32px;
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

/* Page Footer */
.page-footer {
  padding: 12px 0;
  border-top: 1px solid #e2e8f0;
  text-align: center;
  font-size: 11px;
  color: #94a3b8;
}

/* 策略标识区域 */
.strategy-indicator {
  padding: 16px 32px;
  background: linear-gradient(90deg, #f8fafc 0%, #f1f5f9 100%);
  border-bottom: 1px solid #e2e8f0;
  display: flex;
  align-items: center;
  gap: 16px;
  position: relative;
  z-index: 2;
}

.strategy-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #64748b;
  font-weight: 500;
  flex-shrink: 0;
}

.strategy-label svg {
  color: #06b6d4;
}

.strategy-name {
  background: linear-gradient(135deg, #06b6d4 0%, #8b5cf6 100%);
  color: white;
  padding: 6px 16px;
  border-radius: 20px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.5px;
  box-shadow: 0 2px 8px rgba(6, 182, 212, 0.15);
  transition: all 0.3s ease;
}

.strategy-name:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(6, 182, 212, 0.25);
}
</style>
