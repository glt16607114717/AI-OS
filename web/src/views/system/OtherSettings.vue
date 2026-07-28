<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

const messageRounds = ref(10)
const loading = ref(true)
const saving = ref(false)

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('aios_token')
  return { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' }
}

onMounted(async () => {
  await loadSetting()
  loading.value = false
})

async function loadSetting() {
  try {
    const res = await fetch(`${API_BASE}/api/other-setting`, { headers: authHeaders() })
    const data = await res.json()
    if (data.ok && data.data) {
      messageRounds.value = data.data.message_rounds ?? 10
    }
  } catch (e) {
    console.error('加载其他设置失败', e)
    ElMessage.error('加载设置失败')
  }
}

async function saveSetting() {
  if (messageRounds.value < 1) {
    ElMessage.warning('消息对话轮数最小为 1')
    messageRounds.value = 1
    return
  }
  saving.value = true
  try {
    const res = await fetch(`${API_BASE}/api/other-setting`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ message_rounds: messageRounds.value }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('保存成功')
    } else {
      ElMessage.error(data.error || '保存失败')
    }
  } catch (e) {
    console.error('保存其他设置失败', e)
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="other-settings" v-loading="loading">
    <div class="settings-card">
      <h3>其他设置</h3>

      <div class="setting-row">
        <div class="setting-label">
          <span class="label-text">消息对话轮数</span>
          <span class="label-desc">
            控制 Trae/ZCode 代理链路保留多少轮对话历史。
            数值越小越省 token 但 AI 记忆越短；设为 1 只保留最近一轮。
          </span>
        </div>
        <div class="setting-control">
          <el-input-number
            v-model="messageRounds"
            :min="1"
            :max="500"
            :step="1"
            size="large"
            controls-position="right"
            style="width: 200px"
          />
        </div>
      </div>

      <div class="setting-footer">
        <el-button type="primary" @click="saveSetting" :loading="saving" size="large">
          保存设置
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.other-settings {
  padding: 20px;
}

.settings-card {
  max-width: 800px;
  background: var(--bg-card, #1a1a2e);
  border-radius: 12px;
  padding: 32px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

h3 {
  margin: 0 0 32px 0;
  font-size: 20px;
  color: var(--text-primary, #e0e0e0);
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.setting-label {
  flex: 1;
}

.label-text {
  display: block;
  font-size: 15px;
  font-weight: 500;
  color: var(--text-primary, #e0e0e0);
  margin-bottom: 6px;
}

.label-desc {
  display: block;
  font-size: 13px;
  color: var(--text-secondary, #888);
  line-height: 1.5;
}

.setting-control {
  flex-shrink: 0;
}

.setting-footer {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid var(--border-color, rgba(255, 255, 255, 0.08));
}
</style>
