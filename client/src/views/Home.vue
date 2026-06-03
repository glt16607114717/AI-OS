<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const voiceOnline = ref(false)
const voiceCommands = ref(0)
const voiceLoading = ref(true)
const agentOnline = ref(false)

let voiceTimer: ReturnType<typeof setInterval> | null = null

async function fetchVoiceStatus() {
  try {
    const res = await window.aiOS.agentRequest('voice_status', {})
    voiceOnline.value = res?.online ?? false
    voiceCommands.value = res?.commands ?? 0
  } catch {
    voiceOnline.value = false
    voiceCommands.value = 0
  } finally {
    voiceLoading.value = false
  }
}

async function checkAgent() {
  try {
    const res = await window.aiOS.agentHealth()
    agentOnline.value = res?.ok ?? false
  } catch {
    agentOnline.value = false
  }
}

function goToVoice() {
  router.push('/voice')
}

onMounted(() => {
  fetchVoiceStatus()
  checkAgent()
  voiceTimer = setInterval(() => { fetchVoiceStatus(); checkAgent() }, 10000)
})

onUnmounted(() => {
  if (voiceTimer) clearInterval(voiceTimer)
})
</script>

<template>
  <div class="dashboard">
    <!-- Header -->
    <div class="dash-header">
      <div class="dash-header-left">
        <h1 class="dash-title">AI-OS</h1>
        <span class="dash-subtitle">企业 AI 资产操作系统</span>
      </div>
      <div class="dash-header-right">
        <div class="status-badge" :class="agentOnline ? 'status-online' : 'status-offline'">
          <span class="status-dot"></span>
          {{ agentOnline ? '服务运行中' : '服务离线' }}
        </div>
      </div>
    </div>

    <!-- Feature Grid -->
    <div class="feature-grid">
      <!-- Voice Assistant - Hero Card -->
      <div class="feature-card hero-card" @click="goToVoice">
        <div class="hero-bg">
          <div class="hero-ring ring-1"></div>
          <div class="hero-ring ring-2"></div>
          <div class="hero-ring ring-3"></div>
        </div>
        <div class="hero-content">
          <div class="hero-icon">
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
              <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
              <line x1="12" y1="19" x2="12" y2="23"/>
              <line x1="8" y1="23" x2="16" y2="23"/>
            </svg>
          </div>
          <h2 class="hero-name">语音助手</h2>
          <p class="hero-desc">本地语音识别与指令执行引擎，支持自然语言控制桌面应用</p>
          <div class="hero-status">
            <span v-if="voiceLoading" class="tag tag-loading">检测中</span>
            <span v-else :class="voiceOnline ? 'tag tag-online' : 'tag tag-offline'">
              {{ voiceOnline ? '运行中' : '未启动' }}
            </span>
            <span v-if="voiceOnline" class="cmd-count">{{ voiceCommands }} 条指令</span>
          </div>
          <div class="hero-action">
            <span>进入 <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M12 5l7 7-7 7"/></svg></span>
          </div>
        </div>
      </div>

      <!-- Side Cards -->
      <div class="side-cards">
        <div class="feature-card mini-card disabled-card">
          <div class="mini-icon">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
            </svg>
          </div>
          <div class="mini-body">
            <h3>大模型配置</h3>
            <p>多模型路由与网关管理</p>
          </div>
          <span class="coming-badge">即将推出</span>
        </div>

        <div class="feature-card mini-card disabled-card">
          <div class="mini-icon">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><polyline points="10 9 9 9 8 9"/>
            </svg>
          </div>
          <div class="mini-body">
            <h3>提示词引擎</h3>
            <p>模板管理与版本控制</p>
          </div>
          <span class="coming-badge">即将推出</span>
        </div>

        <div class="feature-card mini-card disabled-card">
          <div class="mini-icon">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>
            </svg>
          </div>
          <div class="mini-body">
            <h3>知识库</h3>
            <p>向量化存储与语义检索</p>
          </div>
          <span class="coming-badge">即将推出</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dashboard {
  padding: 24px 32px;
  max-width: 1100px;
  margin: 0 auto;
  height: 100%;
  display: flex;
  flex-direction: column;
}

/* Header */
.dash-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 28px;
}
.dash-title {
  font-size: 22px;
  font-weight: 700;
  color: #1d1d1f;
  margin: 0;
  letter-spacing: 2px;
}
.dash-subtitle {
  font-size: 12px;
  color: #86868b;
  margin-left: 12px;
}
.dash-header-left {
  display: flex;
  align-items: baseline;
}
.status-badge {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  padding: 4px 12px;
  border-radius: 20px;
}
.status-online {
  background: #e8f9ee;
  color: #1a7f37;
}
.status-offline {
  background: #fef0f0;
  color: #c45656;
}
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.status-online .status-dot {
  background: #1a7f37;
  box-shadow: 0 0 6px rgba(26,127,55,0.4);
}
.status-offline .status-dot {
  background: #c45656;
}

/* Feature Grid */
.feature-grid {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 20px;
  flex: 1;
}

/* Hero Card */
.hero-card {
  position: relative;
  border-radius: 16px;
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
}
.hero-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 30px rgba(15, 52, 96, 0.3);
}
.hero-bg {
  position: absolute;
  inset: 0;
  overflow: hidden;
}
.hero-ring {
  position: absolute;
  border-radius: 50%;
  border: 1px solid rgba(99, 102, 241, 0.15);
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  animation: ring-expand 3s ease-out infinite;
}
.ring-1 { width: 100px; height: 100px; animation-delay: 0s; }
.ring-2 { width: 200px; height: 200px; animation-delay: 0.8s; }
.ring-3 { width: 320px; height: 320px; animation-delay: 1.6s; }
@keyframes ring-expand {
  0% { opacity: 1; transform: translate(-50%, -50%) scale(0.8); }
  100% { opacity: 0; transform: translate(-50%, -50%) scale(1.5); }
}
.hero-content {
  position: relative;
  z-index: 1;
  text-align: center;
  color: #fff;
  padding: 32px;
}
.hero-icon {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  background: rgba(99, 102, 241, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
  color: #a5b4fc;
}
.hero-name {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 8px;
  letter-spacing: 2px;
}
.hero-desc {
  font-size: 13px;
  color: rgba(255,255,255,0.5);
  margin: 0 0 16px;
  line-height: 1.5;
}
.hero-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-bottom: 16px;
}
.tag {
  font-size: 11px;
  padding: 2px 10px;
  border-radius: 10px;
  font-weight: 500;
}
.tag-online { background: rgba(34,197,94,0.15); color: #4ade80; }
.tag-offline { background: rgba(239,68,68,0.15); color: #f87171; }
.tag-loading { background: rgba(99,102,241,0.15); color: #a5b4fc; }
.cmd-count { font-size: 11px; color: rgba(255,255,255,0.4); }
.hero-action {
  font-size: 13px;
  color: #a5b4fc;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}
.hero-card:hover .hero-action {
  opacity: 1;
}

/* Side Cards */
.side-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.mini-card {
  border-radius: 12px;
  background: #fff;
  border: 1px solid #e5e7eb;
  padding: 16px 18px;
  display: flex;
  align-items: center;
  gap: 14px;
  transition: box-shadow 0.2s;
}
.mini-card:hover {
  box-shadow: 0 2px 12px rgba(0,0,0,0.06);
}
.mini-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: #f3f4f6;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9ca3af;
  flex-shrink: 0;
}
.mini-body h3 {
  font-size: 14px;
  font-weight: 600;
  color: #1d1d1f;
  margin: 0 0 2px;
}
.mini-body p {
  font-size: 12px;
  color: #86868b;
  margin: 0;
}
.coming-badge {
  font-size: 10px;
  color: #9ca3af;
  background: #f3f4f6;
  padding: 2px 8px;
  border-radius: 6px;
  white-space: nowrap;
  margin-left: auto;
}
.disabled-card {
  opacity: 0.65;
}

/* Responsive */
@media (max-width: 768px) {
  .feature-grid {
    grid-template-columns: 1fr;
  }
  .hero-card { min-height: 220px; }
}
</style>
