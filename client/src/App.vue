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
        <div class="hero">
          <div class="logo-ring">
            <div class="logo-core"></div>
          </div>
          <h1 class="hero-title">AI-OS</h1>
          <p class="hero-subtitle">Enterprise AI Asset Operating System</p>
        </div>

        <div class="status-panel">
          <div class="status-row">
            <span class="status-label">Agent Service</span>
            <div class="status-badge" :class="online ? 'badge-online' : 'badge-offline'">
              <span class="badge-dot"></span>
              <span>{{ online ? 'Online' : 'Offline' }}</span>
            </div>
          </div>
          <div class="status-row" v-if="online">
            <span class="status-label">Uptime</span>
            <span class="status-value">{{ formatUptime(uptime) }}</span>
          </div>
          <div class="status-row" v-if="online">
            <span class="status-label">Version</span>
            <span class="status-value">{{ version }}</span>
          </div>
          <div class="status-row" v-if="online">
            <span class="status-label">PID</span>
            <span class="status-value">{{ pid }}</span>
          </div>
          <div class="status-row" v-if="!online">
            <span class="status-label">Status</span>
            <span class="status-value status-waiting">Waiting for service...</span>
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
const setupMode = ref(false)

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

async function checkSetupNeeded() {
  const needed = await windowAiOS.checkSetupNeeded()
  if (needed) {
    setupMode.value = true
    startSetup()
  } else {
    setupMode.value = false
    checkHealth()
    timer = setInterval(checkHealth, 10000)
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
  timer = setInterval(checkHealth, 10000)
}

onMounted(() => {
  checkSetupNeeded()
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
  background: #09090b;
  color: #fafafa;
  overflow: hidden;
  user-select: none;
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
  align-items: center;
  justify-content: center;
  gap: 48px;
}

.hero {
  text-align: center;
}

.logo-ring {
  width: 80px;
  height: 80px;
  border-radius: 20px;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 50%, #a78bfa 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 24px;
  box-shadow: 0 0 40px rgba(99, 102, 241, 0.15);
  animation: logo-pulse 3s ease-in-out infinite;
}

.logo-core {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: #09090b;
}

@keyframes logo-pulse {
  0%, 100% { box-shadow: 0 0 40px rgba(99, 102, 241, 0.15); }
  50% { box-shadow: 0 0 60px rgba(99, 102, 241, 0.25); }
}

.hero-title {
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 4px;
  color: #fafafa;
  margin-bottom: 8px;
}

.hero-subtitle {
  font-size: 13px;
  color: #52525b;
  letter-spacing: 1px;
}

.status-panel {
  width: 380px;
  background: #111113;
  border: 1px solid #1e1e23;
  border-radius: 12px;
  padding: 20px 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-label {
  font-size: 13px;
  color: #71717a;
  font-weight: 500;
}

.status-value {
  font-size: 13px;
  color: #d4d4d8;
  font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
}

.status-waiting {
  color: #52525b;
  font-style: italic;
  font-family: inherit;
}

.status-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 20px;
  font-size: 12px;
  font-weight: 600;
}

.badge-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.badge-online {
  background: rgba(34, 197, 94, 0.1);
  color: #22c55e;
}

.badge-online .badge-dot {
  background: #22c55e;
  box-shadow: 0 0 8px rgba(34, 197, 94, 0.5);
  animation: dot-pulse 2s ease-in-out infinite;
}

.badge-offline {
  background: rgba(113, 113, 122, 0.1);
  color: #71717a;
}

.badge-offline .badge-dot {
  background: #52525b;
}

@keyframes dot-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
