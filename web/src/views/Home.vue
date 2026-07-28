<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { API_BASE } from '../api'
import { ElMessage } from 'element-plus'

const route = useRoute()
const router = useRouter()

const devExpanded = ref(false)
const llmExpanded = ref(false)
const ragExpanded = ref(false)       // 知识库（原信息沉淀拆分后只留知识库，但保留可展开结构便于后续扩展）
const adviceReportExpanded = ref(false)  // 建议 & 日报
const systemExpanded = ref(false)
const skillExpanded = ref(false)
const logsExpanded = ref(false)
const requirementsExpanded = ref(false)
const bugsExpanded = ref(false)
const isAdmin = ref(false)
const userType = ref('')
const currentUser = ref('')
const activeStrategy = ref('') // 当前生效的策略名称

// 侧边栏折叠状态，持久化到 localStorage
const sidebarCollapsed = ref(localStorage.getItem('aios_sidebar_collapsed') === 'true')

const isDevActive = computed(() => route.path.startsWith('/dev'))
const isLlmActive = computed(() => ['/llm/strategy', '/llm/god-rules', '/llm/quota'].includes(route.path))
const isRagActive = computed(() => route.path.startsWith('/rag'))  // 知识库及巡检报告
const isAdviceReportActive = computed(() => ['/rag/daily-report', '/llm/ai-advice'].includes(route.path))  // 建议 & 日报
const isStatsActive = computed(() => route.path === '/llm/stats')  // 统计仪表独立一级菜单（单链接）
const isSystemActive = computed(() => route.path.startsWith('/system') || route.path === '/llm/config')
const isSkillActive = computed(() => route.path.startsWith('/skills'))
const isLogsActive = computed(() => ['/llm/log', '/llm/errors'].includes(route.path))  // 日志（统计仪表移出）
const isRequirementsActive = computed(() => route.path.startsWith('/requirements'))
const isBugsActive = computed(() => route.path.startsWith('/bugs'))
const isBusinessUser = computed(() => userType.value === 'business')

watch(isDevActive, (val) => {
  if (val) devExpanded.value = true
}, { immediate: true })

watch(isLlmActive, (val) => {
  if (val) llmExpanded.value = true
}, { immediate: true })

watch(isRagActive, (val) => {
  if (val) ragExpanded.value = true
}, { immediate: true })

watch(isAdviceReportActive, (val) => {
  if (val) adviceReportExpanded.value = true
}, { immediate: true })

watch(isSystemActive, (val) => {
  if (val) systemExpanded.value = true
}, { immediate: true })

watch(isSkillActive, (val) => {
  if (val) skillExpanded.value = true
}, { immediate: true })

watch(isLogsActive, (val) => {
  if (val) logsExpanded.value = true
}, { immediate: true })

watch(isRequirementsActive, (val) => {
  if (val) requirementsExpanded.value = true
}, { immediate: true })

watch(isBugsActive, (val) => {
  if (val) bugsExpanded.value = true
}, { immediate: true })

function toggleDevMenu() {
  if (sidebarCollapsed.value) return
  devExpanded.value = !devExpanded.value
}

function toggleLlmMenu() {
  if (sidebarCollapsed.value) return
  llmExpanded.value = !llmExpanded.value
}

function toggleRagMenu() {
  if (sidebarCollapsed.value) return
  ragExpanded.value = !ragExpanded.value
}

function toggleAdviceReportMenu() {
  if (sidebarCollapsed.value) return
  adviceReportExpanded.value = !adviceReportExpanded.value
}

function toggleRequirementsMenu() {
  if (sidebarCollapsed.value) return
  requirementsExpanded.value = !requirementsExpanded.value
}

function toggleBugsMenu() {
  if (sidebarCollapsed.value) return
  bugsExpanded.value = !bugsExpanded.value
}

function toggleSystemMenu() {
  if (sidebarCollapsed.value) return
  systemExpanded.value = !systemExpanded.value
}

function toggleSkillMenu() {
  if (sidebarCollapsed.value) return
  skillExpanded.value = !skillExpanded.value
}

function toggleLogsMenu() {
  if (sidebarCollapsed.value) return
  logsExpanded.value = !logsExpanded.value
}

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  localStorage.setItem('aios_sidebar_collapsed', String(sidebarCollapsed.value))
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
      userType.value = data.data.user_type || ''
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
    <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
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

        <!-- 需求收集（可展开，所有用户可见） -->
        <div class="nav-group">
          <div class="nav-item" :class="{ active: isRequirementsActive }" @click="toggleRequirementsMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/>
              </svg>
            </div>
            <span class="nav-text">需求收集</span>
            <svg class="nav-arrow" :class="{ expanded: requirementsExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <div class="nav-sub" :class="{ 'show-inline': requirementsExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">需求收集</div>
            <router-link to="/requirements/my" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11H1v12h8V11zM23 1h-8v6h8V1zM23 13h-8v10h8V13zM15 7h-4v4h4V7z"/></svg>
              <span>我的需求</span>
            </router-link>
            <router-link to="/requirements/inbox" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
              <span>需求收件箱</span>
            </router-link>
          </div>
        </div>

        <!-- Bug 收集（可展开，所有用户可见） -->
        <div class="nav-group">
          <div class="nav-item" :class="{ active: isBugsActive }" @click="toggleBugsMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="8" y="6" width="8" height="14" rx="4"/>
                <path d="m19 7-3 2"/>
                <path d="m5 7 3 2"/>
                <path d="m19 13-3-2"/>
                <path d="m5 13 3-2"/>
                <path d="M19 19l-3-2"/>
                <path d="M5 19l3-2"/>
                <path d="M12 2v4"/>
              </svg>
            </div>
            <span class="nav-text">Bug 收集</span>
            <svg class="nav-arrow" :class="{ expanded: bugsExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <div class="nav-sub" :class="{ 'show-inline': bugsExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">Bug 收集</div>
            <router-link to="/bugs/my" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11H1v12h8V11zM23 1h-8v6h8V1zM23 13h-8v10h8V13zM15 7h-4v4h4V7z"/></svg>
              <span>我的 Bug</span>
            </router-link>
            <router-link to="/bugs/inbox" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/><path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/></svg>
              <span>Bug 收件箱</span>
            </router-link>
          </div>
        </div>

        <!-- 知识库（独立一级菜单，可展开，后续会加更多二级项） -->
        <div class="nav-group" v-if="!isBusinessUser">
          <div class="nav-item" :class="{ active: isRagActive }" @click="toggleRagMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
                <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
                <line x1="8" y1="7" x2="16" y2="7" />
                <line x1="8" y1="11" x2="14" y2="11" />
              </svg>
            </div>
            <span class="nav-text">知识库</span>
            <svg class="nav-arrow" :class="{ expanded: ragExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <div class="nav-sub" :class="{ 'show-inline': ragExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">知识库</div>
            <router-link to="/rag/knowledge" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
              <span>知识浏览</span>
            </router-link>
            <router-link to="/rag/audit-report" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
              <span>巡检报告</span>
            </router-link>
          </div>
        </div>

        <!-- 建议 & 日报（可展开） -->
        <div class="nav-group" v-if="!isBusinessUser">
          <div class="nav-item" :class="{ active: isAdviceReportActive }" @click="toggleAdviceReportMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="16" y1="13" x2="8" y2="13"/>
                <line x1="16" y1="17" x2="8" y2="17"/>
              </svg>
            </div>
            <span class="nav-text">建议 &amp; 日报</span>
            <svg class="nav-arrow" :class="{ expanded: adviceReportExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <div class="nav-sub" :class="{ 'show-inline': adviceReportExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">建议 &amp; 日报</div>
            <router-link to="/rag/daily-report" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>
              <span>工作日报</span>
            </router-link>
            <router-link to="/llm/ai-advice" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18h6"/><path d="M10 22h4"/><path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/></svg>
              <span>建议</span>
            </router-link>
          </div>
        </div>

        <!-- 统计仪表（独立一级菜单，单链接） -->
        <router-link to="/llm/stats" class="nav-item" active-class="active" v-if="!isBusinessUser">
          <div class="nav-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="12" y1="20" x2="12" y2="10"/>
              <line x1="18" y1="20" x2="18" y2="4"/>
              <line x1="6" y1="20" x2="6" y2="16"/>
            </svg>
          </div>
          <span class="nav-text">统计仪表</span>
        </router-link>

        <!-- LLM (expandable) -->
        <div class="nav-group" v-if="!isBusinessUser">
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
          <div class="nav-sub" :class="{ 'show-inline': llmExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">大模型</div>
            <router-link to="/llm/strategy" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/></svg>
              <span>策略编辑</span>
            </router-link>
            <router-link to="/llm/god-rules" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 4l3 12h14l3-12-6 7-4-7-4 7-6-7zm3 16h14"/></svg>
              <span>上帝指令</span>
            </router-link>
            <router-link to="/llm/quota" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 6 13.5 15.5 8.5 10.5 1 18"/><polyline points="17 6 23 6 23 12"/></svg>
              <span>用量统计</span>
            </router-link>
          </div>
        </div>

        <!-- 日志（可展开） -->
        <div class="nav-group" v-if="!isBusinessUser">
          <div class="nav-item" :class="{ active: isLogsActive }" @click="toggleLogsMenu">
            <div class="nav-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
                <line x1="16" y1="13" x2="8" y2="13"/>
                <line x1="16" y1="17" x2="8" y2="17"/>
              </svg>
            </div>
            <span class="nav-text">日志</span>
            <svg class="nav-arrow" :class="{ expanded: logsExpanded }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </div>
          <div class="nav-sub" :class="{ 'show-inline': logsExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">日志</div>
            <router-link to="/llm/log" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
              <span>对话记录</span>
            </router-link>
            <router-link to="/llm/errors" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
              <span>错误记录</span>
            </router-link>
          </div>
        </div>

        <!-- Dev Assistant (expandable) -->
        <div class="nav-group" v-if="!isBusinessUser">
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
          <div class="nav-sub" :class="{ 'show-inline': devExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">开发助手</div>
            <router-link to="/dev/json" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 3H7a2 2 0 0 0-2 2v5a2 2 0 0 1-2 2 2 2 0 0 1 2 2v5a2 2 0 0 0 2 2h1"/><path d="M16 21h1a2 2 0 0 0 2-2v-5a2 2 0 0 1 2-2 2 2 0 0 1-2-2V5a2 2 0 0 0-2-2h-1"/></svg>
              <span>JSON 格式化</span>
            </router-link>
            <router-link to="/dev/timestamp" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
              <span>时间戳转换</span>
            </router-link>
          </div>
        </div>

        <!-- Skills (expandable, admin only) -->
        <div class="nav-group" v-if="isAdmin && !isBusinessUser">
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
          <div class="nav-sub" :class="{ 'show-inline': skillExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">技能</div>
            <router-link to="/skills/mysql_query" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg>
              <span>MySQL 查询</span>
            </router-link>
          </div>
        </div>

        <!-- System Settings (expandable) -->
        <div class="nav-group" v-if="!isBusinessUser">
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
          <div class="nav-sub" :class="{ 'show-inline': systemExpanded && !sidebarCollapsed, 'flyout': sidebarCollapsed }">
            <div class="flyout-title">系统设置</div>
            <router-link to="/system/account" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
              <span>账户设置</span>
            </router-link>
            <router-link to="/system/prank" class="nav-sub-item" active-class="active">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
              <span>逗你玩</span>
            </router-link>
            <router-link to="/system/other" class="nav-sub-item" active-class="active" v-if="isAdmin">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
              <span>其他设置</span>
            </router-link>
            <router-link to="/llm/config" class="nav-sub-item" active-class="active" v-if="isAdmin">
              <svg class="sub-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/></svg>
              <span>基础配置</span>
            </router-link>
          </div>
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
          <button class="collapse-btn" @click="toggleSidebar" :title="sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'">
            <svg v-if="sidebarCollapsed" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
            <svg v-else width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </button>
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
        <router-view v-slot="{ Component }">
          <keep-alive :include="['ChatWorkspace']">
            <component :is="Component" />
          </keep-alive>
        </router-view>
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
  z-index: 100;
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

/* ===== Collapsed Sidebar ===== */
.sidebar {
  transition: width 0.25s ease;
}

.sidebar.collapsed {
  width: 60px;
}

.sidebar.collapsed .sidebar-nav {
  overflow: visible;
}

.sidebar.collapsed .brand-text {
  display: none;
}

.sidebar.collapsed .sidebar-brand {
  justify-content: center;
  padding: 20px 12px 18px;
}

.sidebar.collapsed .nav-label {
  display: none;
}

.sidebar.collapsed .nav-item {
  justify-content: center;
  padding: 10px 0;
}

.sidebar.collapsed .nav-text,
.sidebar.collapsed .nav-arrow {
  display: none;
}

.sidebar.collapsed .user-name,
.sidebar.collapsed .admin-tag {
  display: none;
}

.sidebar.collapsed .user-info {
  justify-content: center;
}

.sidebar.collapsed .sidebar-footer {
  padding: 12px 8px;
  align-items: center;
}

.sidebar.collapsed .footer-actions {
  flex-direction: column;
  gap: 6px;
}

/* ===== Collapse Button ===== */
.collapse-btn {
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

.collapse-btn:hover {
  background: rgba(99, 102, 241, 0.15);
  color: #a5b4fc;
}

/* ===== Nav Group Flyout ===== */
.nav-group {
  position: relative;
}

/* Submenu visibility: inline mode */
.nav-sub {
  display: none;
  padding: 4px 0 4px 22px;
}

.nav-sub.show-inline {
  display: block;
}

/* Flyout mode (collapsed sidebar) */
.nav-sub.flyout {
  position: absolute;
  left: calc(100% + 16px);
  top: 0;
  min-width: 180px;
  background: #1e1b4b;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  padding: 6px;
  z-index: 1000;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
  animation: flyout-in 0.15s ease-out;
}

.nav-group:hover > .nav-sub.flyout {
  display: block;
}

/* 填充 flyout 与一级菜单之间的间隙，避免鼠标穿过时 hover 断开 */
.nav-sub.flyout::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  right: 100%;
  width: 16px;
}

@keyframes flyout-in {
  from {
    opacity: 0;
    transform: translateX(-8px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

.flyout-title {
  display: none;
}

.nav-sub.flyout .flyout-title {
  display: block;
  font-size: 11px;
  font-weight: 700;
  color: #475569;
  padding: 4px 12px 8px;
  text-transform: uppercase;
  letter-spacing: 1.5px;
}

.nav-sub.flyout .nav-sub-item {
  padding: 8px 12px;
}

/* ===== Sub-menu Icons ===== */
.sub-icon {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  opacity: 0.5;
  transition: all 0.2s;
}

.nav-sub-item:hover .sub-icon {
  opacity: 0.8;
}

.nav-sub-item.active .sub-icon {
  opacity: 1;
  color: #a78bfa;
}

.nav-sub-item {
  gap: 8px;
}
</style>
