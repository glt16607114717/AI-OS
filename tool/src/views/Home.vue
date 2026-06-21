<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const agentOnline = ref(false)
const restarting = ref(false)

let healthTimer: ReturnType<typeof setInterval> | null = null

async function checkHealth() {
  try {
    const res = await window.aiOS.agentHealth()
    agentOnline.value = res?.ok ?? false
  } catch {
    agentOnline.value = false
  }
}

async function restartAgent() {
  if (restarting.value) return
  restarting.value = true
  try {
    const res = await window.aiOS.agentRestart()
    if (res?.ok) {
      agentOnline.value = true
    }
  } catch {}
  restarting.value = false
}

onMounted(() => {
  checkHealth()
  healthTimer = setInterval(checkHealth, 10000)
})
</script>

<template>
  <div class="toolbox">
    <div class="toolbox-header">
      <div class="status-row">
        <div class="status-indicator" :class="agentOnline ? 'online' : 'offline'">
          <span class="status-dot"></span>
          <span>{{ agentOnline ? '服务运行中' : '服务离线' }}</span>
        </div>
        <button class="restart-btn" @click="restartAgent" :disabled="restarting" title="重启服务">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
        </button>
      </div>
    </div>

    <div class="tool-grid">
      <!-- 语音助手 -->
      <div class="tool-card" @click="router.push('/voice')">
        <div class="card-icon voice-icon">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
            <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
            <line x1="12" y1="19" x2="12" y2="23" />
            <line x1="8" y1="23" x2="16" y2="23" />
          </svg>
        </div>
        <div class="card-info">
          <div class="card-name">语音助手</div>
          <div class="card-desc">语音指令控制电脑操作</div>
        </div>
      </div>
    </div>

    <router-view />
  </div>
</template>

<style scoped>
.toolbox {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.toolbox-header {
  padding: 16px 24px 12px;
  flex-shrink: 0;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
}
.status-indicator.online { color: #16a34a; }
.status-indicator.offline { color: #dc2626; }

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
}
.status-indicator.online .status-dot {
  box-shadow: 0 0 6px rgba(22, 163, 74, 0.4);
}

.restart-btn {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #64748b;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s;
}
.restart-btn:hover:not(:disabled) {
  background: #f1f5f9;
  color: #334155;
}
.restart-btn:disabled { opacity: 0.5; cursor: not-allowed; }

.tool-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  padding: 0 24px 16px;
  flex-shrink: 0;
}

.tool-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.15s;
}
.tool-card:hover {
  border-color: #6366f1;
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.1);
}

.card-icon {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.voice-icon {
  background: linear-gradient(135deg, #eef2ff, #e0e7ff);
  color: #6366f1;
}

.card-info {
  min-width: 0;
}

.card-name {
  font-size: 14px;
  font-weight: 600;
  color: #1e293b;
}

.card-desc {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 2px;
}
</style>
