<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Microphone, Setting, Document, FolderOpened } from '@element-plus/icons-vue'

const router = useRouter()

// 语音助手状态
const voiceOnline = ref(false)
const voiceCommands = ref(0)
const voiceLoading = ref(true)

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

function goToVoice() {
  router.push('/voice')
}

onMounted(() => {
  fetchVoiceStatus()
  voiceTimer = setInterval(fetchVoiceStatus, 5000)
})

onUnmounted(() => {
  if (voiceTimer) clearInterval(voiceTimer)
})
</script>

<template>
  <div class="home-dashboard">
    <el-row :gutter="20">
      <!-- 语音助手 -->
      <el-col :span="12">
        <el-card class="feature-card" shadow="hover">
          <div class="card-content">
            <div class="card-header">
              <el-icon :size="28" class="card-icon blue"><Microphone /></el-icon>
              <span class="card-title">语音助手</span>
            </div>
            <p class="card-desc">本地语音识别与指令执行引擎，支持自然语言控制</p>
            <div class="card-status">
              <el-tag v-if="voiceLoading" type="info" size="small">检测中…</el-tag>
              <el-tag v-else :type="voiceOnline ? 'success' : 'danger'" size="small">
                {{ voiceOnline ? '运行中' : '未启动' }}
              </el-tag>
              <span v-if="voiceOnline" class="cmd-count">指令数量：{{ voiceCommands }}</span>
            </div>
            <el-button type="primary" size="small" class="card-action" @click="goToVoice">
              打开
            </el-button>
          </div>
        </el-card>
      </el-col>

      <!-- 大模型配置 -->
      <el-col :span="12">
        <el-card class="feature-card" shadow="hover">
          <div class="card-content">
            <div class="card-header">
              <el-icon :size="28" class="card-icon blue"><Setting /></el-icon>
              <span class="card-title">大模型配置</span>
            </div>
            <p class="card-desc">多模型路由与网关管理，统一调度 AI 推理资源</p>
            <div class="card-status">
              <el-tag type="warning" size="small">即将推出</el-tag>
            </div>
            <el-button type="info" size="small" class="card-action" disabled>
              敬请期待
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px">
      <!-- 提示词引擎 -->
      <el-col :span="12">
        <el-card class="feature-card" shadow="hover">
          <div class="card-content">
            <div class="card-header">
              <el-icon :size="28" class="card-icon blue"><Document /></el-icon>
              <span class="card-title">提示词引擎</span>
            </div>
            <p class="card-desc">模板管理、变量注入与版本控制，优化 AI 交互质量</p>
            <div class="card-status">
              <el-tag type="warning" size="small">即将推出</el-tag>
            </div>
            <el-button type="info" size="small" class="card-action" disabled>
              敬请期待
            </el-button>
          </div>
        </el-card>
      </el-col>

      <!-- 知识库 -->
      <el-col :span="12">
        <el-card class="feature-card" shadow="hover">
          <div class="card-content">
            <div class="card-header">
              <el-icon :size="28" class="card-icon blue"><FolderOpened /></el-icon>
              <span class="card-title">知识库</span>
            </div>
            <p class="card-desc">向量化存储与语义检索，构建企业知识资产体系</p>
            <div class="card-status">
              <el-tag type="warning" size="small">即将推出</el-tag>
            </div>
            <el-button type="info" size="small" class="card-action" disabled>
              敬请期待
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<style scoped>
.home-dashboard {
  max-width: 900px;
  margin: 0 auto;
}

.feature-card {
  border-radius: 8px;
  transition: box-shadow 0.25s ease;
}
.feature-card:hover {
  box-shadow: 0 4px 16px rgba(64, 158, 255, 0.15);
}

.card-content {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 10px;
}
.card-icon.blue {
  color: #409eff;
}
.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.card-desc {
  font-size: 13px;
  color: #909399;
  line-height: 1.5;
  margin: 0;
}

.card-status {
  display: flex;
  align-items: center;
  gap: 10px;
}
.cmd-count {
  font-size: 12px;
  color: #606266;
}

.card-action {
  align-self: flex-start;
  margin-top: 4px;
}
</style>
