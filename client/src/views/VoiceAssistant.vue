<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from 'vue'
import {
  Microphone,
  CirclePlus,
  Delete,
  VideoCamera,
  VideoPause,
  Aim,
  Download,
} from '@element-plus/icons-vue'
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

// 本地 Agent 状态
const agentOnline = ref(false)
const agentListening = ref(false)
const modelReady = ref(false)

// 标定状态
const calibrating = ref(false)
const calibratingIndex = ref(-1)
const currentMousePos = ref<{ x: number; y: number } | null>(null)

// 录制状态
const recording = ref(false)
const recordingIndex = ref(-1)

let pollTimer: ReturnType<typeof setInterval> | null = null

/** 判断 window.aiOS 是否可用（浏览器开发环境下不存在） */
function hasAgent(): boolean {
  return !!window.aiOS?.agentHealth
}

/** 安全调用 Agent API，失败返回 null */
async function safeAgent<T = any>(fn: () => Promise<T>): Promise<T | null> {
  if (!hasAgent()) return null
  try {
    return await fn()
  } catch (e) {
    console.error('[VoiceAssistant] agent 调用失败:', e)
    return null
  }
}

/** 检测本地 Agent 健康状态 + 语音状态 */
async function checkAgent() {
  if (!hasAgent()) {
    agentOnline.value = false
    return
  }
  try {
    const health = await window.aiOS.agentHealth()
    agentOnline.value = !!health?.ok
    if (agentOnline.value) {
      const status = await safeAgent(() =>
        window.aiOS.agentRequest('voice_status', {})
      )
      if (status) {
        agentListening.value = status.listening ?? false
        modelReady.value = status.model_ready ?? false
      }
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] checkAgent:', e)
    ElMessage.error('检查本地服务状态失败: ' + (e?.message || '未知错误'))
    agentOnline.value = false
  }
}

async function fetchStatus() {
  loading.value = true
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/status`, {
      headers: { 'Authorization': `Bearer ${token}` }
    })
    const data = await res.json()
    if (data.ok) {
      // 后端 okResponse 包装：{ok: true, data: {enabled, commands}}
      const payload = data.data || data
      voiceEnabled.value = payload.enabled ?? false
      commands.value = (payload.commands ?? []).map((c: any) => ({
        id: c.id,
        phrase: c.phrase ?? '',
        position: c.position ? (typeof c.position === 'string' ? JSON.parse(c.position) : c.position) : null,
        actions: c.actions ? (typeof c.actions === 'string' ? JSON.parse(c.actions) : c.actions) : null,
        enabled: c.enabled ?? true,
      }))
    } else {
      ElMessage.error(data.error || '加载语音状态失败')
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] fetchStatus 失败:', e)
    ElMessage.error('加载语音状态失败: ' + e.message)
  }
  loading.value = false
}

/** 同步指令列表到本地 Agent 的 voice_config.json */
async function syncToAgent() {
  if (!agentOnline.value) return
  await safeAgent(() =>
    window.aiOS.agentRequest('voice_sync_commands', { commands: commands.value })
  )
}

async function toggleEnabled(val: boolean) {
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/set-enabled`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ enabled: val })
    })
    const data = await res.json()
    if (!data.ok) {
      ElMessage.error(data.error || '设置失败')
      voiceEnabled.value = !val
      return
    }
    // 通知本地 Agent 启动/停止监听
    if (agentOnline.value) {
      await safeAgent(() =>
        window.aiOS.agentRequest('voice_set_enabled', { enabled: val })
      )
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] toggleEnabled 失败:', e)
    ElMessage.error('设置失败: ' + e.message)
    voiceEnabled.value = !val
  }
}

async function addCommand() {
  try {
    const token = localStorage.getItem('aios_token')
    if (!token) {
      ElMessage.warning('请先登录')
      return
    }
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
      syncToAgent()
    } else {
      ElMessage.error(data.error || '添加失败')
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] addCommand 失败:', e)
    ElMessage.error('网络错误：' + e.message)
  }
}

async function updateCommand(index: number, updates: Record<string, any>) {
  const cmd = commands.value[index]
  if (!cmd) return
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/update?id=${cmd.id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify(updates)
    })
    const data = await res.json()
    if (!data.ok) {
      ElMessage.error(data.error || '更新失败')
      return
    }
    syncToAgent()
  } catch (e: any) {
    console.error('[VoiceAssistant] updateCommand 失败:', e)
    ElMessage.error('更新指令失败: ' + e.message)
  }
}

async function removeCommand(index: number) {
  const cmd = commands.value[index]
  if (!cmd) return
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/voice/delete?id=${cmd.id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({})
    })
    const data = await res.json()
    if (!data.ok) {
      ElMessage.error(data.error || '删除失败')
      return
    }
    commands.value.splice(index, 1)
    syncToAgent()
  } catch (e: any) {
    console.error('[VoiceAssistant] removeCommand 失败:', e)
    ElMessage.error('删除指令失败: ' + e.message)
  }
}

/** 标定：启动后轮询本地 Agent 状态，空格确认后保存到云端 */
async function startCalibration(i: number) {
  if (!agentOnline.value) {
    ElMessage.warning('本地服务未启动')
    return
  }
  if (recording.value || calibrating.value) return

  calibrating.value = true
  calibratingIndex.value = i

  // 调用本地 Agent 启动标定（非阻塞，后台线程监听空格）
  await safeAgent(() =>
    window.aiOS.agentRequest('voice_start_calibration', { index: i })
  )

  ElMessage.info('请将鼠标移动到目标位置，然后按空格键确认')

  // 轮询标定状态，直到完成或取消
  const pollCalibration = async () => {
    let elapsed = 0
    const timer = setInterval(async () => {
      elapsed += 0.3
      const status = await safeAgent(() =>
        window.aiOS.agentRequest('voice_status', {})
      )
      if (!status || !calibrating.value) {
        clearInterval(timer)
        return
      }
      // 实时更新当前鼠标位置
      if (status.mouse_pos) {
        currentMousePos.value = { x: status.mouse_pos[0], y: status.mouse_pos[1] }
      }
      // 标定完成（calibrating 变为 false）
      if (!status.calibrating) {
        clearInterval(timer)
        const pos = status.mouse_pos
        if (pos && (pos[0] !== 0 || pos[1] !== 0)) {
          const position = { x: pos[0], y: pos[1] }
          // 更新本地状态，避免重复调用 updateCommand
          commands.value[i].position = position
          await updateCommand(i, { position })
          ElMessage.success(`标定成功：(${position.x}, ${position.y})`)
        } else {
          ElMessage.error('标定失败：未获取到有效坐标')
        }
        calibrating.value = false
        calibratingIndex.value = -1
        currentMousePos.value = null
      }
      // 超时 60 秒自动取消
      if (elapsed > 60) {
        clearInterval(timer)
        calibrating.value = false
        calibratingIndex.value = -1
        currentMousePos.value = null
        ElMessage.warning('标定超时')
      }
    }, 300)
  }
  pollCalibration()
}

async function cancelCalibration() {
  await safeAgent(() =>
    window.aiOS.agentRequest('voice_cancel_calibration', {})
  )
  calibrating.value = false
  calibratingIndex.value = -1
}

/** 录制键鼠操作 */
async function startRecording(i: number) {
  if (!agentOnline.value) {
    ElMessage.warning('本地服务未启动')
    return
  }
  if (recording.value || calibrating.value) return

  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_start_recording', { index: i })
  )

  if (result?.error) {
    ElMessage.error(result.error)
    return
  }

  recording.value = true
  recordingIndex.value = i
  ElMessage.info('录制中，按 F9 停止')
}

async function stopRecording() {
  const idx = recordingIndex.value
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_stop_recording', {})
  )

  recording.value = false
  recordingIndex.value = -1

  if (result?.error) {
    ElMessage.error(result.error)
    return
  }

  if (result?.ok && result.actions) {
    await updateCommand(idx, { actions: result.actions })
    ElMessage.success(`录制完成，共 ${result.actions.length} 步`)
  }
}

/** 下载语音模型 */
async function downloadModel() {
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_download_model', {})
  )

  if (result?.error) {
    ElMessage.error(result.error)
    return
  }

  ElMessage.success('模型下载已开始，请等待几分钟')
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

function isCommandActive(i: number): boolean {
  return (
    (calibrating.value && calibratingIndex.value === i) ||
    (recording.value && recordingIndex.value === i)
  )
}

const agentStatusText = computed(() => {
  if (!hasAgent()) return '浏览器模式'
  if (!agentOnline.value) return '本地服务未启动'
  if (!modelReady.value) return '模型未就绪'
  if (agentListening.value) return '监听中'
  return '在线'
})

const agentStatusType = computed<'success' | 'info' | 'warning' | 'danger'>(() => {
  if (!hasAgent() || !agentOnline.value) return 'danger'
  if (!modelReady.value) return 'warning'
  if (agentListening.value) return 'success'
  return 'info'
})

onMounted(() => {
  fetchStatus()
  checkAgent()
  pollTimer = setInterval(checkAgent, 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
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
          <div class="mic-icon-wrap" :class="{ 'mic-active': agentListening }">
            <el-icon :size="28"><Microphone /></el-icon>
          </div>
          <div class="status-text">
            <div class="status-label">语音助手</div>
            <el-tag :type="agentStatusType" size="small" class="model-tag">
              {{ agentStatusText }}
            </el-tag>
          </div>
        </div>
        <div class="status-right">
          <span class="switch-label">{{ voiceEnabled ? '已启用' : '已关闭' }}</span>
          <el-switch v-model="voiceEnabled" @change="toggleEnabled" />
        </div>
      </div>

      <!-- Agent 状态详情 -->
      <div class="agent-status-row">
        <div class="agent-status-item">
          <span class="agent-status-label">本地服务</span>
          <el-tag
            :type="agentOnline ? 'success' : 'danger'"
            size="small"
            effect="light"
          >
            {{ agentOnline ? '在线' : (hasAgent() ? '离线' : '不可用') }}
          </el-tag>
        </div>
        <div class="agent-status-item">
          <span class="agent-status-label">语音模型</span>
          <el-tag
            v-if="agentOnline && modelReady"
            type="success"
            size="small"
            effect="light"
          >就绪</el-tag>
          <el-tag
            v-else-if="agentOnline && !modelReady"
            type="warning"
            size="small"
            effect="light"
          >未就绪</el-tag>
          <el-tag v-else type="info" size="small" effect="light">-</el-tag>
        </div>
        <div class="agent-status-item">
          <span class="agent-status-label">监听状态</span>
          <el-tag
            v-if="agentOnline && agentListening"
            type="success"
            size="small"
            effect="light"
          >监听中</el-tag>
          <el-tag v-else type="info" size="small" effect="light">未监听</el-tag>
        </div>
        <el-button
          v-if="agentOnline && !modelReady"
          type="primary"
          size="small"
          :icon="Download"
          @click="downloadModel"
        >
          下载语音模型
        </el-button>
      </div>
    </el-card>

    <!-- 标定提示条 -->
    <div v-if="calibrating" class="calibration-banner">
      <div class="calibration-banner-text">
        <el-icon class="calibration-icon"><Aim /></el-icon>
        <span v-if="currentMousePos">
          标定中：当前坐标 ({{ currentMousePos.x }}, {{ currentMousePos.y }})，移动到目标位置后按 <kbd>空格键</kbd> 确认
        </span>
        <span v-else>
          标定中：将鼠标移动到目标位置，按 <kbd>空格键</kbd> 确认
        </span>
      </div>
      <el-button size="small" @click="cancelCalibration">取消标定</el-button>
    </div>

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

      <div
        v-for="(cmd, i) in commands"
        :key="cmd.id"
        class="command-item"
        :class="{ 'command-active': isCommandActive(i) }"
      >
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

          <!-- 标定按钮 -->
          <el-tooltip
            :content="agentOnline ? '标定鼠标位置（空格键确认）' : '本地服务未启动'"
            placement="top"
          >
            <span>
              <el-button
                size="small"
                :icon="Aim"
                :disabled="!agentOnline || recording || calibrating"
                :type="calibrating && calibratingIndex === i ? 'warning' : 'default'"
                @click="startCalibration(i)"
              >
                标定
              </el-button>
            </span>
          </el-tooltip>

          <!-- 录制按钮 -->
          <el-tooltip
            :content="agentOnline
              ? (recording && recordingIndex === i ? '按 F9 停止录制' : '录制键鼠操作（F9 停止）')
              : '本地服务未启动'"
            placement="top"
          >
            <span>
              <el-button
                v-if="!(recording && recordingIndex === i)"
                size="small"
                :icon="VideoCamera"
                :disabled="!agentOnline || recording || calibrating"
                @click="startRecording(i)"
              >
                录制
              </el-button>
              <el-button
                v-else
                size="small"
                type="danger"
                :icon="VideoPause"
                @click="stopRecording"
              >
                停止 (F9)
              </el-button>
            </span>
          </el-tooltip>

          <el-button type="danger" text size="small" :icon="Delete" @click="removeCommand(i)" />
        </div>

        <!-- 录制中提示 -->
        <div v-if="recording && recordingIndex === i" class="recording-hint">
          <span class="recording-dot"></span> 录制中，按 F9 停止
        </div>

        <!-- 标定中提示 -->
        <div v-if="calibrating && calibratingIndex === i" class="calibrating-hint">
          按空格键确认鼠标位置
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
  transition: all 0.3s;
}

.mic-icon-wrap.mic-active {
  background: #f0f9eb;
  color: #67c23a;
  animation: mic-pulse 1.5s ease-in-out infinite;
}

@keyframes mic-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(103, 194, 58, 0.4); }
  50% { box-shadow: 0 0 0 8px rgba(103, 194, 58, 0); }
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

/* Agent 状态详情 */
.agent-status-row {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid #f0f0f0;
  flex-wrap: wrap;
}

.agent-status-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.agent-status-label {
  font-size: 12px;
  color: #909399;
}

/* 标定提示条 */
.calibration-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: #ecf5ff;
  border: 1px solid #d9ecff;
  border-radius: 8px;
  color: #409eff;
}

.calibration-banner-text {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.calibration-icon {
  animation: calibration-blink 1s ease-in-out infinite;
}

@keyframes calibration-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}

.calibration-banner kbd {
  display: inline-block;
  padding: 1px 6px;
  background: #fff;
  border: 1px solid #d9ecff;
  border-radius: 4px;
  font-size: 12px;
  font-family: monospace;
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
  transition: background-color 0.2s;
}

.command-item:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.command-item.command-active {
  background-color: #fff8e6;
  margin: 0 -12px;
  padding: 10px 12px;
  border-radius: 6px;
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

/* 录制/标定提示 */
.recording-hint,
.calibrating-hint {
  margin-top: 6px;
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.recording-hint {
  color: #f56c6c;
}

.calibrating-hint {
  color: #e6a23c;
}

.recording-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #f56c6c;
  animation: recording-pulse 1s ease-in-out infinite;
}

@keyframes recording-pulse {
  0%, 100% { opacity: 1; transform: scale(1); }
  50% { opacity: 0.5; transform: scale(0.8); }
}
</style>
