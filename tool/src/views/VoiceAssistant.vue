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
  Document,
  Refresh,
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

interface VoiceCommand {
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

// 识别日志
const recognizeLogs = ref<any[]>([])
const showLogPanel = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null
let recordingPollTimer: ReturnType<typeof setInterval> | null = null

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

/** 检测本地 Agent 健康状态 + 加载语音数据（全本地） */
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
        voiceEnabled.value = status.enabled ?? false
        // 只在首次加载（commands为空时）拉取指令列表，避免轮询覆盖用户编辑
        if (commands.value.length === 0 && status.commands) {
          commands.value = status.commands.map((c: any) => ({
            phrase: c.phrase ?? '',
            position: c.position ?? null,
            actions: c.actions ?? null,
            enabled: c.enabled ?? true,
          }))
        }
        // 如果开关已开但监听未启动，自动触发一次
        if (status.enabled && !status.listening && status.model_ready) {
          await safeAgent(() =>
            window.aiOS.agentRequest('voice_set_enabled', { enabled: true })
          )
          agentListening.value = true
        }
      }
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] checkAgent:', e)
    agentOnline.value = false
  }
}

async function toggleEnabled(val: boolean) {
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_set_enabled', { enabled: val })
  )
  if (!result?.ok) {
    ElMessage.error('设置失败')
    voiceEnabled.value = !val
  }
}

async function addCommand() {
  if (!agentOnline.value) {
    ElMessage.warning('本地服务未启动')
    return
  }
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_add_command', { phrase: '' })
  )
  if (result?.ok) {
    commands.value.push({
      phrase: '',
      position: null,
      actions: null,
      enabled: true,
    })
  } else {
    ElMessage.error('添加失败')
  }
}

async function updateCommand(index: number, updates: Record<string, any>) {
  if (!agentOnline.value) return
  await safeAgent(() =>
    window.aiOS.agentRequest('voice_update_command', { index, ...updates })
  )
}

async function removeCommand(index: number) {
  if (!agentOnline.value) return
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_remove_command', { index })
  )
  if (result?.ok) {
    commands.value.splice(index, 1)
  } else {
    ElMessage.error('删除失败')
  }
}

/** 标定：启动后轮询本地 Agent 状态，空格确认后保存 */
async function startCalibration(i: number) {
  if (!agentOnline.value) {
    ElMessage.warning('本地服务未启动')
    return
  }
  if (recording.value || calibrating.value) return

  calibrating.value = true
  calibratingIndex.value = i

  await safeAgent(() =>
    window.aiOS.agentRequest('voice_start_calibration', { index: i })
  )

  ElMessage.info('请将鼠标移动到目标位置，然后按空格键确认')

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
      if (status.mouse_pos) {
        currentMousePos.value = { x: status.mouse_pos[0], y: status.mouse_pos[1] }
      }
      if (!status.calibrating) {
        clearInterval(timer)
        const pos = status.mouse_pos
        if (pos && (pos[0] !== 0 || pos[1] !== 0)) {
          const position = { x: pos[0], y: pos[1] }
          commands.value[i].position = position
          commands.value[i].actions = null  // 标定与录制互斥
          await updateCommand(i, { position, actions: null })
          ElMessage.success(`标定成功：(${position.x}, ${position.y})`)
        } else {
          ElMessage.error('标定失败：未获取到有效坐标')
        }
        calibrating.value = false
        calibratingIndex.value = -1
        currentMousePos.value = null
      }
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

  // safeAgent 失败返回 null，需要一并处理
  if (!result || result.error) {
    ElMessage.error(result?.error || '启动录制失败')
    return
  }

  recording.value = true
  recordingIndex.value = i
  ElMessage.info('录制中，按 F9 停止')

  // 轮询后台录制状态：F9 在 Python 端停止录制后，前端自动同步 UI 并拉取结果
  recordingPollTimer = setInterval(async () => {
    if (!recording.value) {
      if (recordingPollTimer) { clearInterval(recordingPollTimer); recordingPollTimer = null }
      return
    }
    const status = await safeAgent(() =>
      window.aiOS.agentRequest('voice_recording_status', {})
    )
    if (status && !status.recording) {
      if (recordingPollTimer) { clearInterval(recordingPollTimer); recordingPollTimer = null }
      await stopRecording()
    }
  }, 500)
}

async function stopRecording() {
  // 清除轮询定时器
  if (recordingPollTimer) { clearInterval(recordingPollTimer); recordingPollTimer = null }

  const idx = recordingIndex.value
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_stop_recording', {})
  )

  recording.value = false
  recordingIndex.value = -1

  if (!result || result.error) {
    ElMessage.error(result?.error || '停止录制失败')
    return
  }

  if (result?.ok && result.actions) {
    commands.value[idx].actions = result.actions
    commands.value[idx].position = null  // 录制与标定互斥
    await updateCommand(idx, { actions: result.actions, position: null })
    ElMessage.success(`录制完成，共 ${result.actions.length} 步`)
  }
}

// 模型下载状态
const downloading = ref(false)
const downloadProgress = ref(0)
const downloadMessage = ref('')
const downloadError = ref('')
let downloadPollTimer: ReturnType<typeof setInterval> | null = null

/** 一键下载语音模型（调用后端 modelscope 命令行下载） */
async function downloadModel() {
  if (downloading.value) return
  if (!agentOnline.value) {
    ElMessage.warning('本地服务未启动')
    return
  }
  downloading.value = true
  downloadProgress.value = 0
  downloadMessage.value = '准备下载...'
  downloadError.value = ''

  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_download_model', {})
  )

  if (!result?.ok) {
    downloading.value = false
    ElMessage.error(result?.error || '下载启动失败')
    return
  }

  // 轮询下载进度
  downloadPollTimer = setInterval(async () => {
    const status = await safeAgent(() =>
      window.aiOS.agentRequest('voice_download_status', {})
    )
    if (!status) return

    downloadProgress.value = status.progress ?? 0
    downloadMessage.value = status.message || ''

    // 下载失败
    if (status.error) {
      if (downloadPollTimer) { clearInterval(downloadPollTimer); downloadPollTimer = null }
      downloading.value = false
      downloadError.value = status.error
      return
    }

    // 下载完成
    if (status.done || status.progress >= 100) {
      if (downloadPollTimer) { clearInterval(downloadPollTimer); downloadPollTimer = null }
      downloading.value = false
      modelReady.value = true
      downloadProgress.value = 100
      ElMessage.success('语音模型下载完成')
      await checkAgent()
    }
  }, 1500)
}

function cancelDownload() {
  if (downloadPollTimer) { clearInterval(downloadPollTimer); downloadPollTimer = null }
  downloading.value = false
  ElMessage.info('已取消进度刷新（后台仍在下载，可稍后点击刷新检测）')
}

/** 刷新模型状态 */
async function refreshModelStatus() {
  await checkAgent()
}

/** 加载识别日志 */
async function loadRecognizeLogs() {
  const result = await safeAgent(() =>
    window.aiOS.agentRequest('voice_recognize_log', {})
  )
  if (result?.ok && result.logs) {
    recognizeLogs.value = result.logs
  }
}

/** 清空识别日志 */
async function clearRecognizeLogs() {
  await safeAgent(() =>
    window.aiOS.agentRequest('voice_clear_recognize_log', {})
  )
  recognizeLogs.value = []
}

function toggleLogPanel() {
  showLogPanel.value = !showLogPanel.value
  if (showLogPanel.value) {
    loadRecognizeLogs()
  }
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
  checkAgent()
  pollTimer = setInterval(checkAgent, 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  if (recordingPollTimer) clearInterval(recordingPollTimer)
  if (downloadPollTimer) clearInterval(downloadPollTimer)
})
</script>

<template>
  <div class="voice-assistant">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">语音助手</h2>
      <el-button
        :type="showLogPanel ? 'primary' : 'default'"
        :icon="Document"
        size="small"
        @click="toggleLogPanel"
        class="log-btn"
      >
        识别日志
      </el-button>
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
          :loading="downloading"
          @click="downloadModel"
        >
          {{ downloading ? '下载中...' : '下载模型' }}
        </el-button>
        <el-button
          v-if="agentOnline && !modelReady && !downloading"
          size="small"
          :icon="Refresh"
          @click="refreshModelStatus"
        >
          刷新检测
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
        :key="i"
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

    <!-- 识别日志弹窗 -->
    <el-dialog
      v-model="showLogPanel"
      title="语音识别日志"
      width="520px"
      @open="loadRecognizeLogs"
    >
      <div v-if="recognizeLogs.length === 0" class="empty-hint">
        暂无识别记录
      </div>
      <div v-else class="log-list">
        <div
          v-for="(log, i) in recognizeLogs.slice().reverse()"
          :key="i"
          class="log-item"
          :class="{ 'log-matched': log.matched }"
        >
          <span class="log-time">{{ log.time || '' }}</span>
          <span class="log-text">"{{ log.text }}"</span>
          <el-tag v-if="log.matched" type="success" size="small">已匹配</el-tag>
          <el-tag v-else type="info" size="small">未匹配</el-tag>
        </div>
      </div>
      <template #footer>
        <el-button size="small" @click="loadRecognizeLogs">刷新</el-button>
        <el-button size="small" type="danger" plain @click="clearRecognizeLogs">清空</el-button>
        <el-button size="small" type="primary" @click="showLogPanel = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 模型下载遮罩 -->
    <div v-if="downloading || downloadError" class="download-mask">
      <div class="download-modal">
        <div class="download-title">语音模型下载</div>
        <template v-if="downloadError">
          <div class="download-error">{{ downloadError }}</div>
          <el-button type="primary" size="small" @click="downloadError = ''">关闭</el-button>
        </template>
        <template v-else>
          <div class="download-info">正在下载 FunASR 语音识别模型（约 800MB），请勿关闭窗口</div>
          <div class="download-bar-track">
            <div class="download-bar-fill" :style="{ width: downloadProgress + '%' }"></div>
          </div>
          <div class="download-progress-row">
            <span class="download-message">{{ downloadMessage }}</span>
            <span class="download-pct">{{ downloadProgress }}%</span>
          </div>
          <el-button size="small" @click="cancelDownload">取消</el-button>
        </template>
      </div>
    </div>
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
  flex: 1;
}

.log-btn {
  margin-left: auto;
}

/* 识别日志弹窗 */
.log-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.log-list {
  max-height: 240px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.log-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  background: #f5f7fa;
  font-size: 13px;
}

.log-item.log-matched {
  background: #f0f9eb;
}

.log-time {
  color: #909399;
  font-size: 12px;
  white-space: nowrap;
}

.log-text {
  flex: 1;
  color: #303133;
  font-weight: 500;
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

/* 模型下载遮罩 */
.download-mask {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.download-modal {
  background: #fff;
  border-radius: 12px;
  padding: 28px 32px;
  width: 420px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.25);
  text-align: center;
}

.download-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 16px;
}

.download-info {
  font-size: 13px;
  color: #909399;
  margin-bottom: 18px;
}

.download-bar-track {
  height: 8px;
  background: #ebeef5;
  border-radius: 4px;
  overflow: hidden;
  margin-bottom: 10px;
}

.download-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, #6366f1, #8b5cf6);
  border-radius: 4px;
  transition: width 0.4s ease;
}

.download-progress-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  margin-bottom: 18px;
}

.download-message {
  color: #909399;
}

.download-pct {
  color: #6366f1;
  font-weight: 600;
  font-family: 'Cascadia Code', 'Consolas', monospace;
}

.download-error {
  color: #f56c6c;
  font-size: 13px;
  margin-bottom: 16px;
  word-break: break-all;
}
</style>
