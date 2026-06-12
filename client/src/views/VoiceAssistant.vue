<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Microphone, CirclePlus, Delete, VideoCamera, VideoPause, Check } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../api'

interface VoiceCommand {
  id: number
  phrase: string
  position: { x: number; y: number } | null
  actions: any[] | null
  enabled: boolean
}

const voiceEnabled = ref(false)
const commands = ref<VoiceCommand[]>([])
const loading = ref(false)

async function fetchStatus() {
  loading.value = true
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/status`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    const data = await res.json()
    if (data.ok) {
      voiceEnabled.value = data.enabled ?? false
      commands.value = (data.commands ?? []).map((c: any) => ({
        id: c.id,
        phrase: c.phrase ?? '',
        position: c.position ? (typeof c.position === 'string' ? JSON.parse(c.position) : c.position) : null,
        actions: c.actions ? (typeof c.actions === 'string' ? JSON.parse(c.actions) : c.actions) : null,
        enabled: c.enabled ?? true,
      }))
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] fetchStatus 失败:', e)
  }
  loading.value = false
}

async function toggleEnabled(val: boolean) {
  try {
    const token = localStorage.getItem('aios_token')
    await fetch(`${API_BASE}/api/voice/set-enabled`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ enabled: val })
    })
  } catch { voiceEnabled.value = !val }
}

async function addCommand() {
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ phrase: '' })
    })
    const data = await res.json()
    if (data.ok && data.data) {
      commands.value.push({
        id: data.data.id,
        phrase: '',
        position: null,
        actions: null,
        enabled: true,
      })
    }
  } catch {}
}

async function updateCommand(index: number, updates: Record<string, any>) {
  const cmd = commands.value[index]
  if (!cmd) return
  try {
    const token = localStorage.getItem('aios_token')
    await fetch(`${API_BASE}/api/voice/update?id=${cmd.id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify(updates)
    })
  } catch {}
}

async function removeCommand(index: number) {
  const cmd = commands.value[index]
  if (!cmd) return
  try {
    const token = localStorage.getItem('aios_token')
    await fetch(`${API_BASE}/api/voice/delete?id=${cmd.id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({})
    })
    commands.value.splice(index, 1)
  } catch {}
}

function getCommandMode(cmd: VoiceCommand): string {
  if (cmd.actions && cmd.actions.length > 0) return 'recording'
  if (cmd.position) return 'calibration'
  return 'none'
}

function getCommandModeLabel(cmd: VoiceCommand): string {
  const mode = getCommandMode(cmd)
  if (mode === 'recording') return `录制(${cmd.actions!.length}步)`
  if (mode === 'calibration') return `标定(${cmd.position!.x},${cmd.position!.y})`
  return '未配置'
}

onMounted(() => { fetchStatus() })
</script>

<template>
  <div class="voice-assistant">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">语音助手</h2>
    </div>

    <!-- Status Card -->
    <el-card shadow="never" class="status-card">
      <div class="status-row">
        <div class="status-left">
          <div class="mic-icon-wrap">
            <el-icon :size="28"><Microphone /></el-icon>
          </div>
          <div class="status-text">
            <div class="status-label">语音助手</div>
            <el-tag type="info" size="small" class="model-tag">
              云端模式（指令管理）
            </el-tag>
          </div>
        </div>
        <div class="status-right">
          <span class="switch-label">{{ voiceEnabled ? '已启用' : '已关闭' }}</span>
          <el-switch v-model="voiceEnabled" @change="toggleEnabled" />
        </div>
      </div>
    </el-card>

    <!-- Commands List -->
    <el-card shadow="never" class="commands-card">
      <template #header>
        <div class="commands-header">
          <span class="commands-title">语音指令</span>
          <el-button type="primary" :icon="CirclePlus" size="small" @click="addCommand">
            添加指令
          </el-button>
        </div>
      </template>

      <div v-if="commands.length === 0" class="empty-hint">
        暂无语音指令，点击上方按钮添加
      </div>

      <div v-for="(cmd, i) in commands" :key="cmd.id" class="command-item">
        <div class="command-fields">
          <el-input
            v-model="cmd.phrase"
            type="textarea"
            :autosize="{ minRows: 1, maxRows: 4 }"
            placeholder="触发词，多个用逗号分隔，如：对话,废话,对换"
            size="default"
            class="phrase-input"
            @blur="updateCommand(i, { phrase: cmd.phrase })"
          />
          <div class="mode-display">
            <el-tag v-if="getCommandMode(cmd) === 'recording'" type="success" size="small">
              {{ getCommandModeLabel(cmd) }}
            </el-tag>
            <el-tag v-else-if="getCommandMode(cmd) === 'calibration'" size="small">
              {{ getCommandModeLabel(cmd) }}
            </el-tag>
            <el-tag v-else type="info" size="small">未配置</el-tag>
          </div>
          <el-switch v-model="cmd.enabled" @change="updateCommand(i, { enabled: cmd.enabled })" />
          <el-button type="danger" text size="small" :icon="Delete" @click="removeCommand(i)" />
        </div>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.voice-assistant {
  max-width: 720px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

/* Status Card */
.status-card {
  border-radius: 10px;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.status-left {
  display: flex;
  align-items: center;
  gap: 14px;
}

.mic-icon-wrap {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #ecf5ff;
  color: #409eff;
}

.status-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.status-label {
  font-size: 15px;
  font-weight: 500;
  color: #303133;
}

.model-tag {
  width: fit-content;
}

.status-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-label {
  font-size: 13px;
  color: #909399;
}

/* Commands Card */
.commands-card {
  border-radius: 10px;
}

.commands-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.commands-title {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.empty-hint {
  text-align: center;
  color: #c0c4cc;
  padding: 24px 0;
  font-size: 13px;
}

.command-item {
  padding: 10px 0;
  border-bottom: 1px solid #f0f0f0;
}

.command-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.command-fields {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.phrase-input {
  flex: 1;
  min-width: 140px;
}

.mode-display {
  min-width: 80px;
}
</style>
