<template>
  <div class="shell">
    <div class="titlebar">
      <div class="titlebar-left">
        <div class="app-icon"></div>
        <span class="app-name">AI-OS</span>
      </div>
      <div class="titlebar-actions">
        <button class="tb-btn" @click="windowAiOS.windowMinimize()">
          <svg width="10" height="1" viewBox="0 0 10 1"><rect width="10" height="1" fill="currentColor"/></svg>
        </button>
        <button class="tb-btn" @click="windowAiOS.windowMaximize()">
          <svg width="10" height="10" viewBox="0 0 10 10">
            <rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1"/>
          </svg>
        </button>
        <button class="tb-btn tb-btn-close" @click="windowAiOS.windowClose()">
          <svg width="10" height="10" viewBox="0 0 10 10">
            <line x1="0" y1="0" x2="10" y2="10" stroke="currentColor" stroke-width="1.2"/>
            <line x1="10" y1="0" x2="0" y2="10" stroke="currentColor" stroke-width="1.2"/>
          </svg>
        </button>
      </div>
    </div>

    <div class="content">
      <SetupWizard
        v-if="setupMode"
        :current-step="setupProgress.stepIndex"
        :step-name="setupProgress.step || ''"
        :total-steps="setupProgress.totalSteps"
        :percent="setupProgress.percent"
        :detail="setupProgress.detail"
        :error="setupProgress.error"
        :done="setupProgress.done"
        @retry="retrySetup"
      />

      <template v-else>
        <!-- Splash: pulsing logo, shown for 2.5s -->
        <div v-if="!showDashboard" class="splash-page">
          <div class="logo-ring">
            <div class="logo-core"></div>
          </div>
          <h1 class="splash-title">AI-OS</h1>
        </div>

        <!-- Dashboard -->
        <div v-if="showDashboard" class="dashboard">
          <div class="dash-body">
            <router-view />
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import SetupWizard from './setup/SetupWizard.vue'

const windowAiOS = window.aiOS

const online = ref(false)
const uptime = ref(0)
const version = ref('')
const pid = ref(0)
const setupMode = ref(true)
const showDashboard = ref(false)

const setupProgress = ref<{
  step: string
  stepIndex: number
  totalSteps: number
  percent: number
  detail: string
  error?: string
  done?: boolean
}>({
  step: '',
  stepIndex: 0,
  totalSteps: 5,
  percent: 0,
  detail: '',
})

let timer: ReturnType<typeof setInterval> | null = null
let healthChecking = false

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

async function checkHealth() {
  if (healthChecking) return
  healthChecking = true
  try {
    const res = await windowAiOS.agentHealth()
    online.value = res.ok
    if (res.ok) {
      uptime.value = res.uptime ?? 0
      version.value = res.version ?? ''
      pid.value = res.pid ?? 0
    }
  } catch {
    online.value = false
  } finally {
    healthChecking = false
  }
}

async function startSetup() {
  try {
    await windowAiOS.runSetup((info: any) => {
      setupProgress.value = { ...info }
      if (info.done) {
        setTimeout(() => {
          finishSetup()
        }, 1500)
      }
    })
  } catch {}
}

async function retrySetup() {
  setupProgress.value = {
    step: '',
    stepIndex: 0,
    totalSteps: 5,
    percent: 0,
    detail: '',
    error: undefined,
    done: undefined,
  }
  await startSetup()
}

function finishSetup() {
  setupMode.value = false
  checkHealth()
  // Show splash for 2.5s, then switch to dashboard
  setTimeout(() => {
    showDashboard.value = true
  }, 2500)
}

onMounted(async () => {
  const needed = await windowAiOS.checkSetupNeeded()
  if (needed) {
    startSetup()
  } else {
    setupMode.value = false
    checkHealth()
    setTimeout(() => {
      showDashboard.value = true
      timer = setInterval(checkHealth, 10000)
    }, 2500)
  }
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: 'Segoe UI', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
  overflow: hidden;
  -webkit-font-smoothing: antialiased;
}

.shell {
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.titlebar {
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #09090b;
  border-bottom: 1px solid #1a1a1f;
  -webkit-app-region: drag;
  flex-shrink: 0;
}

.titlebar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 14px;
}

.app-icon {
  width: 16px;
  height: 16px;
  border-radius: 3px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
}

.app-name {
  font-size: 12px;
  font-weight: 600;
  color: #a1a1aa;
  letter-spacing: 1.5px;
  text-transform: uppercase;
}

.titlebar-actions {
  display: flex;
  -webkit-app-region: no-drag;
}

.tb-btn {
  width: 46px;
  height: 38px;
  border: none;
  background: transparent;
  color: #71717a;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}

.tb-btn:hover {
  background: #18181b;
  color: #e4e4e7;
}

.tb-btn-close:hover {
  background: #dc2626;
  color: #ffffff;
}

.content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.loading-page {
  flex: 1;
  background: #09090b;
}

/* Splash page - light background, pulsing text */
.splash-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: #f5f7fa;
}

.logo-ring {
  width: 64px;
  height: 64px;
  border-radius: 16px;
  background: rgba(255,255,255,0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 20px;
  animation: splash-pulse 2s ease-in-out infinite;
}

.logo-core {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  background: rgba(99, 102, 241, 0.9);
}

@keyframes splash-pulse {
  0%, 100% { transform: scale(1); opacity: 0.7; box-shadow: 0 0 20px rgba(99,102,241,0.1); }
  50% { transform: scale(1.08); opacity: 1; box-shadow: 0 0 40px rgba(99,102,241,0.3); }
}

.splash-title {
  font-size: 32px;
  font-weight: 700;
  letter-spacing: 6px;
  color: #409eff;
  animation: text-pulse 1.5s ease-in-out infinite;
}

@keyframes text-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

/* Dashboard - light background for Element Plus */
.dashboard {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

.dash-body {
  flex: 1;
  overflow: auto;
  padding: 20px;
}
</style>
