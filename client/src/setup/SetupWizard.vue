<template>
  <div class="setup-wizard">
    <div class="setup-header">
      <div class="setup-logo">
        <div class="logo-ring">
          <div class="logo-core"></div>
        </div>
      </div>
      <h1 class="setup-title">AI-OS 环境配置</h1>
      <p class="setup-subtitle">正在配置运行环境，请稍候...</p>
    </div>

    <div class="setup-body">
      <div class="steps-list">
        <div
          v-for="(s, i) in allSteps"
          :key="i"
          class="step-item"
          :class="{
            'step-done': isDone || activeIndex > i,
            'step-active': !isDone && activeIndex === i,
            'step-pending': !isDone && activeIndex < i,
            'step-error': hasError && activeIndex === i,
          }"
        >
          <div class="step-icon">
            <svg v-if="isDone || activeIndex > i" width="16" height="16" viewBox="0 0 16 16">
              <path d="M6.5 12L2 7.5l1.4-1.4L6.5 9.2l6.1-6.1L14 4.5z" fill="currentColor"/>
            </svg>
            <svg v-else-if="hasError && activeIndex === i" width="16" height="16" viewBox="0 0 16 16">
              <path d="M8 1a7 7 0 100 14A7 7 0 008 1zm3.5 9.1l-1.4 1.4L8 9.4l-2.1 2.1-1.4-1.4L6.6 8 4.5 5.9l1.4-1.4L8 6.6l2.1-2.1 1.4 1.4L9.4 8z" fill="currentColor"/>
            </svg>
            <div v-else-if="!isDone && activeIndex === i" class="spinner"></div>
            <div v-else class="step-dot"></div>
          </div>
          <span class="step-text">{{ s }}</span>
        </div>
      </div>

      <div class="progress-section">
        <div class="progress-bar-track">
          <div class="progress-bar-fill" :style="{ width: percent + '%' }"></div>
        </div>
        <div class="progress-info">
          <span class="progress-detail">{{ detail }}</span>
          <span class="progress-pct">{{ percent }}%</span>
        </div>
      </div>

      <div v-if="hasError" class="error-section">
        <p class="error-msg">{{ errorMsg }}</p>
        <button class="btn-retry" @click="$emit('retry')">重试</button>
      </div>

      <div v-if="done" class="done-section">
        <p class="done-msg">AI-OS 环境配置完成！</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const allSteps = ['环境检测', '下载 Python', '安装依赖', '注册服务', '启动服务']

const props = defineProps<{
  currentStep: number
  stepName: string
  totalSteps: number
  percent: number
  detail: string
  error?: string
  done?: boolean
}>()

defineEmits<{
  retry: []
}>()

const hasError = computed(() => !!props.error)
const errorMsg = computed(() => props.error || '')
const isDone = computed(() => !!props.done)

const activeIndex = computed(() => {
  const idx = allSteps.indexOf(props.stepName)
  return idx >= 0 ? idx : props.currentStep
})
</script>

<style scoped>
.setup-wizard {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #09090b;
  color: #fafafa;
  font-family: 'Segoe UI', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
  overflow: hidden;
}

.setup-header {
  text-align: center;
  padding: 40px 20px 20px;
}

.setup-logo {
  margin-bottom: 16px;
}

.logo-ring {
  width: 56px;
  height: 56px;
  border-radius: 14px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto;
  box-shadow: 0 0 30px rgba(99, 102, 241, 0.2);
}

.logo-core {
  width: 24px;
  height: 24px;
  border-radius: 6px;
  background: #09090b;
}

.setup-title {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 2px;
  margin-bottom: 6px;
}

.setup-subtitle {
  font-size: 13px;
  color: #52525b;
}

.setup-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0 40px 30px;
  gap: 20px;
  overflow: hidden;
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: fit-content;
  margin: 0 auto;
}

.step-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  font-weight: 500;
}

.step-icon {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.step-done {
  color: #22c55e;
}

.step-active {
  color: #6366f1;
}

.step-pending {
  color: #3f3f46;
}

.step-error {
  color: #ef4444;
}

.step-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #3f3f46;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid #6366f133;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.progress-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.progress-bar-track {
  height: 6px;
  background: #1e1e23;
  border-radius: 3px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #6366f1, #8b5cf6);
  border-radius: 3px;
  transition: width 0.3s ease;
}

.progress-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.progress-detail {
  font-size: 12px;
  color: #71717a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 70%;
}

.progress-pct {
  font-size: 12px;
  font-weight: 600;
  color: #a1a1aa;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  flex-shrink: 0;
}

.error-section {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 8px;
  padding: 16px;
  text-align: center;
}

.error-msg {
  color: #ef4444;
  font-size: 13px;
  margin-bottom: 12px;
}

.btn-retry {
  background: #ef4444;
  color: #fff;
  border: none;
  padding: 8px 24px;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}

.btn-retry:hover {
  background: #dc2626;
}

.done-section {
  text-align: center;
  padding: 16px;
}

.done-msg {
  color: #22c55e;
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 16px;
}
</style>
