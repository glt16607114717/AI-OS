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
      <!-- Setup Wizard: shown when Python env not ready -->
      <div v-if="needSetup" class="loading-page">
        <SetupWizard
          :current-step="setupStep"
          :step-name="setupStepName"
          :total-steps="5"
          :percent="setupPercent"
          :detail="setupDetail"
          :error="setupError"
          :done="setupDone"
          @retry="runSetup"
        />
      </div>

      <!-- Main -->
      <div v-else class="dashboard">
        <router-view />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import SetupWizard from './setup/SetupWizard.vue'

const windowAiOS = window.aiOS

const needSetup = ref(false)
const setupStep = ref(0)
const setupStepName = ref('')
const setupPercent = ref(0)
const setupDetail = ref('')
const setupError = ref('')
const setupDone = ref(false)

onMounted(async () => {
  let setupRequired = false
  if (windowAiOS?.checkSetupNeeded) {
    try {
      setupRequired = await windowAiOS.checkSetupNeeded()
    } catch {
      setupRequired = true
    }
  }

  if (setupRequired) {
    needSetup.value = true
    runSetup()
  }
})

async function runSetup() {
  setupError.value = ''
  setupDone.value = false

  try {
    const result = await windowAiOS.runSetup((info: any) => {
      setupStep.value = info.stepIndex ?? 0
      setupStepName.value = info.step ?? ''
      setupPercent.value = info.percent ?? 0
      setupDetail.value = info.detail ?? ''
      if (info.error) {
        setupError.value = info.error
      }
    })

    if (result?.ok) {
      setupDone.value = true
      setTimeout(() => {
        needSetup.value = false
      }, 2000)
    } else if (result?.error) {
      setupError.value = result.error
    }
  } catch (e: any) {
    setupError.value = e.message || '环境配置失败'
  }
}
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
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
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
  color: #606266;
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
  color: #909399;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}

.tb-btn:hover {
  background: #f5f7fa;
  color: #303133;
}

.tb-btn-close:hover {
  background: #dc2626;
  color: #ffffff;
}

.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.loading-page {
  flex: 1;
  background: #09090b;
}

.dashboard {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}
</style>
