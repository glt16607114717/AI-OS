<script setup lang="ts">
import { ref, onMounted } from 'vue'

const publicIp = ref('')
const loading = ref(false)
const error = ref('')
const lastUpdate = ref('')

async function fetchIp() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetch('https://api.ipify.org?format=json')
    const data = await res.json()
    publicIp.value = data.ip
    lastUpdate.value = new Date().toLocaleTimeString('zh-CN')
  } catch (e: any) {
    error.value = '获取失败: ' + e.message
  } finally {
    loading.value = false
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {}
}

onMounted(() => {
  fetchIp()
})
</script>

<template>
  <div class="tool-page">
    <h2 class="page-title">IP 展示</h2>
    <p class="page-desc">查看本机公网 IP 地址</p>

    <!-- Public IP -->
    <div class="ip-card main-card">
      <div class="card-label">公网 IP</div>
      <div class="ip-display">
        <template v-if="loading">
          <div class="loading-dots">
            <span></span><span></span><span></span>
          </div>
        </template>
        <template v-else-if="error">
          <span class="error-text">{{ error }}</span>
        </template>
        <template v-else>
          <span class="ip-value">{{ publicIp }}</span>
        </template>
      </div>
      <div class="card-footer">
        <span v-if="lastUpdate" class="update-time">更新于 {{ lastUpdate }}</span>
        <div class="card-actions">
          <button class="btn btn-ghost" @click="fetchIp" :disabled="loading">刷新</button>
          <button class="btn btn-accent" @click="copy(publicIp)" :disabled="!publicIp">复制</button>
        </div>
      </div>
    </div>

    <!-- Info section -->
    <div class="info-section">
      <div class="info-card">
        <div class="info-icon">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10" />
            <line x1="2" y1="12" x2="22" y2="12" />
            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
          </svg>
        </div>
        <div class="info-body">
          <div class="info-title">数据来源</div>
          <div class="info-desc">通过 ipify.org 获取公网 IP，仅用于展示，不存储任何数据</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tool-page {
  max-width: 560px;
  margin: 0 auto;
}

.page-title {
  font-size: 20px;
  font-weight: 700;
  color: #1e293b;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #94a3b8;
  margin: 0 0 20px;
}

/* IP Card */
.main-card {
  background: linear-gradient(135deg, #0f172a 0%, #1a1040 60%, #1e1b4b 100%);
  border-radius: 14px;
  padding: 24px;
  margin-bottom: 16px;
  position: relative;
  overflow: hidden;
}

.main-card::before {
  content: '';
  position: absolute;
  top: -40px;
  right: -40px;
  width: 140px;
  height: 140px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.12) 0%, transparent 70%);
  pointer-events: none;
}

.card-label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 1.5px;
  margin-bottom: 12px;
}

.ip-display {
  min-height: 48px;
  display: flex;
  align-items: center;
}

.ip-value {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 32px;
  font-weight: 700;
  color: #e0e7ff;
  letter-spacing: 2px;
}

.error-text {
  color: #f87171;
  font-size: 14px;
}

.loading-dots {
  display: flex;
  gap: 6px;
}

.loading-dots span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #6366f1;
  animation: dot-bounce 1.2s infinite ease-in-out;
}

.loading-dots span:nth-child(2) { animation-delay: 0.2s; }
.loading-dots span:nth-child(3) { animation-delay: 0.4s; }

@keyframes dot-bounce {
  0%, 80%, 100% { opacity: 0.3; transform: scale(0.8); }
  40% { opacity: 1; transform: scale(1); }
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.update-time {
  font-size: 11px;
  color: #475569;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.btn {
  padding: 5px 14px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: all 0.15s;
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-ghost {
  background: rgba(255, 255, 255, 0.06);
  color: #94a3b8;
}

.btn-ghost:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.1);
  color: #e2e8f0;
}

.btn-accent {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
}

.btn-accent:hover:not(:disabled) {
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.4);
}

/* Info section */
.info-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.info-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: #fff;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
}

.info-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: #f0f9ff;
  color: #0ea5e9;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.info-title {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
  margin-bottom: 2px;
}

.info-desc {
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.4;
}
</style>
