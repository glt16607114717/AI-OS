<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Microphone, CirclePlus, Delete, Position, VideoPause } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'

const router = useRouter()
const agentRequest = window.aiOS.agentRequest

interface VoiceCommand {
  phrase: string
  position: { x: number; y: number } | null
  enabled: boolean
  calibrated: boolean
}

const listening = ref(false)
const modelReady = ref(false)
const voiceEnabled = ref(false)
const commands = ref<VoiceCommand[]>([])
const loading = ref(false)

const calibratingIndex = ref<number | null>(null)
const mousePos = ref({ x: 0, y: 0 })

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
      enabled: c.enabled ?? true,
      calibrated: !!(c.position && c.position.length === 2),
    }))
  } catch {
    // ignore
  }
  loading.value = false
}

async function toggleEnabled(val: boolean) {
  if (val && !modelReady.value) {
    voiceEnabled.value = false
    ElMessageBox.alert(
      `<div style="line-height:1.8">
        <p><b>1.</b> 下载模型（约 50MB）：</p>
        <p style="margin-left:16px"><a href="https://alphacephei.com/vosk/models/vosk-model-small-cn-0.22.zip" target="_blank" style="color:#409eff">https://alphacephei.com/vosk/models/vosk-model-small-cn-0.22.zip</a></p>
        <p style="margin-top:8px"><b>2.</b> 解压 zip 文件</p>
        <p style="margin-top:8px"><b>3.</b> 将 <code>vosk-model-small-cn-0.22</code> 文件夹放到：</p>
        <p style="margin-left:16px"><code>C:\\ProgramData\\AI-OS\\models\\vosk\\vosk-model-small-cn-0.22\\</code></p>
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
  } catch {
    voiceEnabled.value = !val
  }
}

async function addCommand() {
  try {
    await agentRequest('voice_add_command', { phrase: '' })
    await fetchStatus()
  } catch {
    // ignore
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
  } catch {
    // ignore
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
  } catch {
    // ignore
  }
}

async function startCalibration(index: number) {
  try {
    await agentRequest('voice_start_calibration', { index })
    calibratingIndex.value = index
    startCalibrationPoll()
  } catch {
    // ignore
  }
}

async function cancelCalibration() {
  try {
    await agentRequest('voice_cancel_calibration', {})
  } catch {
    // ignore
  }
  calibratingIndex.value = null
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
      } else if (res && res.mouse_pos && (res.mouse_pos[0] !== 0 || res.mouse_pos[1] !== 0)) {
        mousePos.value = { x: res.mouse_pos[0], y: res.mouse_pos[1] }
      }
    } catch {}
  }, 200)
}

function stopCalibrationPoll() {
  if (calibrationPollTimer) {
    clearInterval(calibrationPollTimer)
    calibrationPollTimer = null
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
  if (calibratingIndex.value !== null) {
    cancelCalibration()
  }
})
</script>

<template>
  <div class="voice-assistant">
    <!-- Header -->
    <div class="page-header">
      <el-button text @click="router.push('/')">
        <el-icon><svg viewBox="0 0 24 24" width="16" height="16"><path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" fill="currentColor"/></svg></el-icon>
        <span style="margin-left: 4px">返回</span>
      </el-button>
      <h2 class="page-title">语音助手</h2>
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

    <!-- Calibration Overlay -->
    <div v-if="calibratingIndex !== null" class="calibration-hint">
      <div class="calibration-content">
        <el-icon :size="20" color="#409eff"><Position /></el-icon>
        <span>标定模式：将鼠标移到目标位置，按 <kbd>空格</kbd> 确认，<kbd>Esc</kbd> 取消</span>
        <span class="mouse-pos">当前坐标：({{ mousePos.x }}, {{ mousePos.y }})</span>
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
          <div class="position-display">
            <el-tag v-if="cmd.position" type="success" size="small">
              ({{ cmd.position.x }}, {{ cmd.position.y }})
            </el-tag>
            <el-tag v-else type="info" size="small">未标定</el-tag>
          </div>
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
            type="primary"
            size="small"
            :icon="Position"
            @click="startCalibration(i)"
          >
            标定
          </el-button>
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
  min-width: 160px;
}

.position-display {
  min-width: 80px;
}
</style>
