<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Microphone, CirclePlus, Delete, VideoCamera, VideoPause, Check } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const agentRequest = window.aiOS.agentRequest

interface VoiceCommand {
  phrase: string
  position: { x: number; y: number } | null
  actions: any[] | null
  enabled: boolean
}

const listening = ref(false)
const modelReady = ref(false)
const voiceEnabled = ref(false)
const commands = ref<VoiceCommand[]>([])
const loading = ref(false)

// 录制状态
const recordingIndex = ref<number | null>(null)
const recordingActionCount = ref(0)

// 录制预览
const previewVisible = ref(false)
const previewActions = ref<any[]>([])
const previewIndex = ref(-1)

// 标定（旧模式，保留兼容）
const calibratingIndex = ref<number | null>(null)
const mousePos = ref({ x: 0, y: 0 })

// 识别日志
const logVisible = ref(false)
const recognizeLogs = ref<{ time: string; text: string; matched: boolean; phrase: string }[]>([])
let logTimer: ReturnType<typeof setInterval> | null = null

let statusTimer: ReturnType<typeof setInterval> | null = null

// 识别日志
async function openLog() {
  logVisible.value = true
  await refreshLog()
  logTimer = setInterval(refreshLog, 1000)
}

function closeLog() {
  logVisible.value = false
  if (logTimer) { clearInterval(logTimer); logTimer = null }
}

async function refreshLog() {
  try {
    const res = await agentRequest('voice_recognize_log', {})
    if (res.ok) recognizeLogs.value = res.logs || []
  } catch {}
}

async function clearLog() {
  await agentRequest('voice_clear_recognize_log', {})
  recognizeLogs.value = []
}

async function fetchStatus() {
  loading.value = true
  try {
    const res = await agentRequest('voice_status', {})
    listening.value = res.listening ?? false
    modelReady.value = res.model_ready ?? false
    voiceEnabled.value = res.enabled ?? false
    commands.value = (res.commands ?? []).map((c: any) => ({
      phrase: c.phrase ?? '',
      position: c.position ?? null,
      actions: c.actions ?? null,
      enabled: c.enabled ?? true,
    }))
    // 录制状态
    if (res.recording) {
      recordingIndex.value = res.recording_index ?? null
      recordingActionCount.value = res.recording_action_count ?? 0
    } else {
      recordingIndex.value = null
      recordingActionCount.value = 0
    }
    // 标定状态
    if (res.calibrating) {
      calibratingIndex.value = res.calibration_index ?? null
      mousePos.value = { x: res.mouse_pos?.[0] ?? 0, y: res.mouse_pos?.[1] ?? 0 }
    } else {
      calibratingIndex.value = null
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] fetchStatus 失败:', e)
  }
  loading.value = false
}

async function toggleEnabled(val: boolean) {
  if (val && !modelReady.value) {
    voiceEnabled.value = false
    ElMessageBox.alert(
      `<div style="line-height:1.8">
        <p><b>1.</b> 下载语音识别模型：</p>
        <p style="margin-left:16px"><b>标准模型</b>（推荐，识别更准确，约 1.3GB）：</p>
        <p style="margin-left:32px"><a href="https://alphacephei.com/vosk/models/vosk-model-cn-0.22.zip" target="_blank" style="color:#409eff">vosk-model-cn-0.22.zip</a></p>
        <p style="margin-left:16px"><b>轻量模型</b>（备选，约 50MB）：</p>
        <p style="margin-left:32px"><a href="https://alphacephei.com/vosk/models/vosk-model-small-cn-0.22.zip" target="_blank" style="color:#409eff">vosk-model-small-cn-0.22.zip</a></p>
        <p style="margin-top:8px"><b>2.</b> 解压 zip 文件</p>
        <p style="margin-top:8px"><b>3.</b> 将解压出的文件夹放到：</p>
        <p style="margin-left:16px"><code>C:\\ProgramData\\AI-OS\\models\\vosk\\</code></p>
        <p style="margin-top:8px"><b>4.</b> 放好后重新打开此页面即可</p>
      </div>`,
      '请先安装语音识别模型',
      { dangerouslyUseHTMLString: true, confirmButtonText: '知道了', type: 'info' }
    )
    return
  }
  try {
    await agentRequest('voice_set_enabled', { enabled: val })
    voiceEnabled.value = val
  } catch (e: any) {
    console.error('[VoiceAssistant] toggleEnabled 失败:', e)
    voiceEnabled.value = !val
  }
}

async function addCommand() {
  try {
    await agentRequest('voice_add_command', { phrase: '' })
    await fetchStatus()
  } catch (e: any) {
    console.error('[VoiceAssistant] addCommand 失败:', e)
    ElMessage.error('添加指令失败: ' + (e?.message || e))
  }
}

async function updateCommand(index: number, updates: Partial<VoiceCommand>) {
  try {
    await agentRequest('voice_update_command', { index, ...updates })
    if (updates.phrase !== undefined) {
      commands.value[index].phrase = updates.phrase
    }
    if (updates.enabled !== undefined) {
      commands.value[index].enabled = updates.enabled
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] updateCommand 失败:', e)
  }
}

async function removeCommand(index: number) {
  try {
    await agentRequest('voice_remove_command', { index })
    commands.value.splice(index, 1)
    if (calibratingIndex.value === index) {
      cancelCalibration()
    } else if (calibratingIndex.value !== null && calibratingIndex.value > index) {
      calibratingIndex.value--
    }
    if (recordingIndex.value === index) {
      recordingIndex.value = null
    } else if (recordingIndex.value !== null && recordingIndex.value > index) {
      recordingIndex.value--
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] removeCommand 失败:', e)
    ElMessage.error('删除指令失败: ' + (e?.message || e))
  }
}

// ── 标定（旧模式）──

async function startCalibration(index: number) {
  try {
    // 标定和录制互斥：清除旧录制数据
    if (commands.value[index].actions) {
      await updateCommand(index, { actions: null })
    }
    await agentRequest('voice_start_calibration', { index })
    calibratingIndex.value = index
    startCalibrationPoll()
  } catch (e: any) {
    console.error('[VoiceAssistant] startCalibration 失败:', e)
    ElMessage.error('启动标定失败: ' + (e?.message || e))
  }
}

async function cancelCalibration() {
  try {
    await agentRequest('voice_cancel_calibration', {})
  } catch (e: any) {
    console.error('[VoiceAssistant] cancelCalibration 失败:', e)
  }
  calibratingIndex.value = null
}

// ── 录制（新模式）──

async function startRecording(index: number) {
  try {
    const res = await agentRequest('voice_start_recording', { index })
    if (res.ok) {
      recordingIndex.value = index
      recordingActionCount.value = 0
      startRecordingPoll()
      ElMessage.success('录制已开始，按 F9 停止')
    } else {
      ElMessage.error(res.error || '启动录制失败')
    }
  } catch {
    ElMessage.error('启动录制失败')
  }
}

function startRecordingPoll() {
  stopRecordingPoll()
  statusTimer = setInterval(async () => {
    try {
      const res = await agentRequest('voice_recording_status', {})
      recordingActionCount.value = res.action_count ?? 0
      if (!res.recording) {
        // F9 已停止录制，获取结果
        stopRecordingPoll()
        await handleRecordingComplete()
      }
    } catch (e: any) {
      console.error('[VoiceAssistant] 录制状态轮询失败:', e)
    }
  }, 500)
}

function stopRecordingPoll() {
  if (statusTimer) {
    clearInterval(statusTimer)
    statusTimer = null
  }
}

async function handleRecordingComplete() {
  try {
    const res = await agentRequest('voice_stop_recording', {})
    if (res.ok && res.actions?.length > 0) {
      previewActions.value = res.actions
      previewIndex.value = recordingIndex.value ?? -1
      previewVisible.value = true
      recordingIndex.value = null
      recordingActionCount.value = 0
    } else if (res.ok && (!res.actions || res.actions.length === 0)) {
      ElMessage.warning('录制结果为空')
      recordingIndex.value = null
    } else {
      ElMessage.error(res.error || '获取录制结果失败')
    }
  } catch {
    ElMessage.error('获取录制结果失败')
  }
}

async function saveRecording() {
  try {
    const res = await agentRequest('voice_save_recording', {
      index: previewIndex.value,
      actions: JSON.parse(JSON.stringify(previewActions.value)),
    })
    if (res.ok) {
      previewVisible.value = false
      ElMessage.success(`已保存 ${previewActions.value.length} 步操作`)
      await fetchStatus()
    } else {
      ElMessage.error(res.error || '保存失败')
    }
  } catch (e: any) {
    console.error('[VoiceAssistant] saveRecording error', e)
    ElMessage.error('保存失败: ' + (e?.message || e))
  }
}

function discardRecording() {
  previewVisible.value = false
  ElMessage.info('录制已丢弃')
}

function cancelRecording() {
  recordingIndex.value = null
  recordingActionCount.value = 0
  stopRecordingPoll()
  agentRequest('voice_stop_recording', {}).catch((e: any) => {
    console.error('[VoiceAssistant] cancelRecording stop 失败:', e)
  })
}

function getActionLabel(action: any): string {
  const t = action.type
  if (t === 'mouse_move') return `移动鼠标 → (${action.x}, ${action.y})`
  if (t === 'click') return `${action.button === 'right' ? '右键' : action.button === 'middle' ? '中键' : '左键'}点击 (${action.x}, ${action.y})`
  if (t === 'key_down') return `按下 ${action.key_name || 'Key'}`
  if (t === 'key_up') return `释放 ${action.key_name || 'Key'}`
  if (t === 'scroll') return `滚动 ${action.delta > 0 ? '↑' : '↓'}`
  if (t === 'delay') return `等待 ${action.ms}ms`
  return t
}

function getActionIcon(action: any): string {
  const t = action.type
  if (t === 'mouse_move') return '🖱'
  if (t === 'click') return '👆'
  if (t === 'key_down' || t === 'key_up') return '⌨'
  if (t === 'scroll') return '📜'
  return '⏱'
}

function getCommandMode(cmd: VoiceCommand): string {
  if (cmd.actions && cmd.actions.length > 0) return 'recording'
  if (cmd.position) return 'calibration'
  return 'none'
}

function getCommandModeLabel(cmd: VoiceCommand): string {
  const mode = getCommandMode(cmd)
  if (mode === 'recording') return `${cmd.actions!.length} 步操作`
  if (mode === 'calibration') return `(${cmd.position![0]}, ${cmd.position![1]})`
  return '未配置'
}

// 标定轮询
let calibrationPollTimer: ReturnType<typeof setInterval> | null = null

function startCalibrationPoll() {
  stopCalibrationPoll()
  calibrationPollTimer = setInterval(async () => {
    try {
      const res = await agentRequest('voice_status', {})
      if (res && !res.calibrating) {
        calibratingIndex.value = null
        await fetchStatus()
        stopCalibrationPoll()
      } else if (res && res.mouse_pos) {
        mousePos.value = { x: res.mouse_pos[0], y: res.mouse_pos[1] }
      }
    } catch (e: any) {
      console.error('[VoiceAssistant] 标定状态轮询失败:', e)
    }
  }, 200)
}

function stopCalibrationPoll() {
  if (calibrationPollTimer) {
    clearInterval(calibrationPollTimer)
    calibrationPollTimer = null
  }
}

function onMouseMove(e: MouseEvent) {
  mousePos.value = { x: e.screenX, y: e.screenY }
}

async function onKeydown(e: KeyboardEvent) {
  if (e.code === 'Escape' && calibratingIndex.value !== null) {
    e.preventDefault()
    await cancelCalibration()
  }
}

onMounted(() => {
  fetchStatus()
  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('mousemove', onMouseMove)
  window.removeEventListener('keydown', onKeydown)
  stopCalibrationPoll()
  stopRecordingPoll()
  closeLog()
  if (calibratingIndex.value !== null) {
    cancelCalibration()
  }
})
</script>

<template>
  <div class="voice-assistant">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">语音助手</h2>
      <button class="btn-log" @click="openLog">识别日志</button>
    </div>

    <!-- Status Card -->
    <el-card shadow="never" class="status-card">
      <div class="status-row">
        <div class="status-left">
          <div class="mic-icon-wrap" :class="{ active: listening }">
            <el-icon :size="28"><Microphone /></el-icon>
          </div>
          <div class="status-text">
            <div class="status-label">{{ listening ? '正在监听' : '未启动' }}</div>
            <el-tag :type="modelReady ? 'success' : 'info'" size="small" class="model-tag">
              {{ modelReady ? '模型就绪' : '模型未安装' }}
            </el-tag>
          </div>
        </div>
        <div class="status-right">
          <span class="switch-label">{{ voiceEnabled ? '已启用' : '已关闭' }}</span>
          <el-switch v-model="voiceEnabled" @change="toggleEnabled" />
        </div>
      </div>
    </el-card>

    <!-- Recording Banner -->
    <div v-if="recordingIndex !== null" class="recording-banner">
      <div class="rec-dot"></div>
      <span>录制中... 已捕获 <b>{{ recordingActionCount }}</b> 个事件</span>
      <span class="rec-hint">按 F9 停止录制</span>
      <el-button size="small" @click="cancelRecording">取消</el-button>
    </div>

    <!-- Calibration Overlay -->
    <div v-if="calibratingIndex !== null" class="calibration-hint">
      <div class="calibration-content">
        <el-icon :size="20" color="#409eff"><VideoCamera /></el-icon>
        <span>标定模式：将鼠标移到目标位置，按 <kbd>空格</kbd> 确认，<kbd>Esc</kbd> 取消</span>
        <span class="mouse-pos">当前坐标：({{ mousePos.x }}, {{ mousePos.y }})</span>
      </div>
    </div>

    <!-- Preview Dialog -->
    <div v-if="previewVisible" class="preview-overlay">
      <div class="preview-card">
        <div class="preview-header">
          <h3>录制预览</h3>
          <span class="preview-count">{{ previewActions.length }} 步操作</span>
        </div>
        <div class="preview-list">
          <div v-for="(a, i) in previewActions" :key="i" class="preview-item">
            <span class="preview-icon">{{ getActionIcon(a) }}</span>
            <span class="preview-label">{{ getActionLabel(a) }}</span>
            <span v-if="a.ms > 0" class="preview-delay">+{{ a.ms }}ms</span>
          </div>
        </div>
        <div class="preview-actions">
          <el-button @click="discardRecording">丢弃</el-button>
          <el-button type="primary" :icon="Check" @click="saveRecording">保存</el-button>
        </div>
      </div>
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

      <div v-for="(cmd, i) in commands" :key="i" class="command-item">
        <div class="command-fields">
          <el-input
            v-model="cmd.phrase"
            placeholder="语音短语"
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
          <el-button
            type="primary"
            size="small"
            :icon="VideoCamera"
            @click="startRecording(i)"
            :disabled="recordingIndex !== null"
          >
            录制
          </el-button>
          <el-button
            v-if="calibratingIndex === i"
            type="warning"
            size="small"
            :icon="VideoPause"
            @click="cancelCalibration"
          >
            取消标定
          </el-button>
          <el-button
            v-else
            size="small"
            @click="startCalibration(i)"
            :disabled="recordingIndex !== null"
          >
            标定
          </el-button>
          <el-switch v-model="cmd.enabled" @change="updateCommand(i, { enabled: cmd.enabled })" />
          <el-button type="danger" text size="small" :icon="Delete" @click="removeCommand(i)" />
        </div>
      </div>
    </el-card>

    <!-- 识别日志弹窗 -->
    <div v-if="logVisible" class="log-overlay" @click.self="closeLog">
      <div class="log-dialog">
        <div class="log-header">
          <span>识别日志</span>
          <div class="log-actions">
            <button class="btn btn-sm" @click="clearLog">清空</button>
            <button class="btn btn-sm" @click="closeLog">关闭</button>
          </div>
        </div>
        <div class="log-body">
          <div v-if="recognizeLogs.length === 0" class="log-empty">暂无识别记录</div>
          <div v-for="(log, i) in recognizeLogs.slice().reverse()" :key="i" class="log-item" :class="{ matched: log.matched, unmatched: !log.matched }">
            <span class="log-time">{{ log.time }}</span>
            <span class="log-text">"{{ log.text }}"</span>
            <span v-if="log.matched" class="log-match">-> {{ log.phrase }}</span>
            <span v-else class="log-nomatch">未匹配</span>
          </div>
        </div>
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

.mic-icon-wrap.active {
  background: #409eff;
  color: #fff;
  box-shadow: 0 0 12px rgba(64, 158, 255, 0.4);
  animation: pulse-ring 1.5s ease-in-out infinite;
}

@keyframes pulse-ring {
  0%, 100% { box-shadow: 0 0 12px rgba(64, 158, 255, 0.4); }
  50% { box-shadow: 0 0 24px rgba(64, 158, 255, 0.7); }
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

/* Recording Banner */
.recording-banner {
  background: linear-gradient(135deg, #fef0f0, #fff1f0);
  border: 1px solid #fab6b6;
  border-radius: 8px;
  padding: 10px 16px;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: #c45656;
  animation: fade-in 0.2s ease;
}

.rec-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #f56c6c;
  animation: rec-blink 1s ease-in-out infinite;
}

@keyframes rec-blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.rec-hint {
  margin-left: auto;
  font-size: 12px;
  color: #f89898;
}

/* Calibration Hint */
.calibration-hint {
  background: #ecf5ff;
  border: 1px solid #b3d8ff;
  border-radius: 8px;
  padding: 10px 16px;
  animation: fade-in 0.2s ease;
}

.calibration-content {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #409eff;
  flex-wrap: wrap;
}

.calibration-content kbd {
  background: #fff;
  border: 1px solid #d9ecff;
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 12px;
  font-family: inherit;
}

.mouse-pos {
  margin-left: auto;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
  color: #909399;
}

@keyframes fade-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}

/* Preview Overlay */
.preview-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  animation: fade-in 0.15s ease;
}

.preview-card {
  background: #fff;
  border-radius: 14px;
  width: 480px;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.2);
}

.preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid #f0f0f0;
}

.preview-header h3 {
  margin: 0;
  font-size: 16px;
  color: #303133;
}

.preview-count {
  font-size: 12px;
  color: #909399;
}

.preview-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 20px;
  max-height: 50vh;
}

.preview-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid #fafafa;
  font-size: 13px;
  color: #606266;
}

.preview-item:last-child {
  border-bottom: none;
}

.preview-icon {
  width: 22px;
  text-align: center;
  flex-shrink: 0;
}

.preview-label {
  flex: 1;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
}

.preview-delay {
  font-size: 11px;
  color: #c0c4cc;
  white-space: nowrap;
}

.preview-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid #f0f0f0;
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

/* 日志按钮 */
.btn-log {
  margin-left: auto;
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  border: none;
  cursor: pointer;
}

.btn-log:hover {
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.4);
}

/* 日志弹窗 */
.log-overlay {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.log-dialog {
  width: 600px;
  max-height: 70vh;
  background: #1e1b4b;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: rgba(255, 255, 255, 0.05);
  color: #e0e7ff;
  font-weight: 600;
  font-size: 14px;
}

.log-actions {
  display: flex;
  gap: 6px;
}

.log-actions .btn-sm {
  background: rgba(255, 255, 255, 0.1);
  color: #c4b5fd;
  border: none;
  padding: 3px 10px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
}

.log-body {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.log-empty {
  color: #64748b;
  text-align: center;
  padding: 40px;
  font-size: 13px;
}

.log-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 13px;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  margin-bottom: 4px;
}

.log-item.matched {
  background: rgba(74, 222, 128, 0.1);
}

.log-item.unmatched {
  background: rgba(248, 113, 113, 0.08);
}

.log-time {
  color: #64748b;
  font-size: 11px;
  flex-shrink: 0;
}

.log-text {
  color: #e0e7ff;
}

.log-match {
  color: #4ade80;
  font-weight: 600;
}

.log-nomatch {
  color: #f87171;
  font-size: 11px;
}
</style>
