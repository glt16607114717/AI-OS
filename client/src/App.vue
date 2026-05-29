<template>
  <div class="app-shell">
    <div class="titlebar">
      <div class="titlebar-drag">
        <span class="titlebar-title">AI-OS</span>
      </div>
      <div class="titlebar-actions">
        <button class="titlebar-btn" @click="minimize">&#x2500;</button>
        <button class="titlebar-btn" @click="maximize">&#x25A1;</button>
        <button class="titlebar-btn titlebar-btn-close" @click="close">&#x2715;</button>
      </div>
    </div>
    <div class="main-content">
      <div class="status-card">
        <div class="status-indicator" :class="agentOnline ? 'online' : 'offline'"></div>
        <div class="status-info">
          <h2>Agent Service</h2>
          <p>{{ agentOnline ? `Online · Uptime ${agentUptime}s` : 'Offline' }}</p>
          <p class="version" v-if="agentOnline">v{{ agentVersion }}</p>
        </div>
      </div>
      <div class="placeholder">
        <p>AI-OS Shell Ready</p>
        <p class="hint">Backend service is running as Windows Service via WinSW</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const agentOnline = ref(false)
const agentUptime = ref(0)
const agentVersion = ref('')

let timer: ReturnType<typeof setInterval> | null = null

async function checkHealth() {
  try {
    const result = await window.aiOS.agentHealth()
    agentOnline.value = result.ok
    if (result.ok) {
      agentUptime.value = result.uptime ?? 0
      agentVersion.value = result.version ?? ''
    }
  } catch {
    agentOnline.value = false
  }
}

function minimize() { window.aiOS.windowMinimize() }
function maximize() { window.aiOS.windowMaximize() }
function close() { window.aiOS.windowClose() }

onMounted(() => {
  checkHealth()
  timer = setInterval(checkHealth, 5000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style>
* { margin: 0; padding: 0; box-sizing: border-box; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #0a0a0a;
  color: #e0e0e0;
  overflow: hidden;
  user-select: none;
}

.app-shell {
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.titlebar {
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #1a1a2e;
  -webkit-app-region: drag;
}

.titlebar-drag {
  flex: 1;
  padding-left: 12px;
}

.titlebar-title {
  font-size: 12px;
  color: #888;
  letter-spacing: 2px;
}

.titlebar-actions {
  display: flex;
  -webkit-app-region: no-drag;
}

.titlebar-btn {
  width: 46px;
  height: 36px;
  border: none;
  background: transparent;
  color: #888;
  font-size: 14px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.titlebar-btn:hover { background: #2a2a3e; color: #fff; }
.titlebar-btn-close:hover { background: #e81123; color: #fff; }

.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 40px;
}

.status-card {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 30px 50px;
  background: #141422;
  border-radius: 12px;
  border: 1px solid #222;
}

.status-indicator {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  transition: all 0.3s;
}

.status-indicator.online {
  background: #00d26a;
  box-shadow: 0 0 12px #00d26a55;
}

.status-indicator.offline {
  background: #555;
}

.status-info h2 {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 4px;
}

.status-info p {
  font-size: 13px;
  color: #888;
}

.status-info .version {
  color: #555;
  font-size: 11px;
}

.placeholder {
  text-align: center;
}

.placeholder p:first-child {
  font-size: 14px;
  color: #444;
  letter-spacing: 3px;
}

.placeholder .hint {
  font-size: 11px;
  color: #333;
  margin-top: 8px;
}
</style>
