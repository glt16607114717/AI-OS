<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

const loading = ref(false)
const refreshing = ref(false)
const keys = ref<any[]>([])
const autoRefreshInterval = ref<number | null>(null)

function statusLabel(status: string) {
  const map: Record<string, string> = {
    normal: '正常',
    degraded: '降级中',
    exhausted: '已耗尽',
  }
  return map[status] || status || '—'
}

async function fetchKeys() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/quota/status`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      keys.value = result.data?.records || []
    }
  } catch (e: any) {
    console.error('[QuotaMonitor] fetchKeys error:', e)
    ElMessage.error('加载用量数据失败: ' + (e?.message || '未知错误'))
  }
  loading.value = false
}

async function refreshNow() {
  refreshing.value = true
  try {
    const res = await fetch(`${API_BASE}/api/quota/force-check`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({}),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('已触发刷新')
    } else {
      ElMessage.error(data.error || '刷新失败')
    }
    // 等待一小段时间让检查完成
    await new Promise((resolve) => setTimeout(resolve, 1000))
    await fetchKeys()
  } catch (e) {
    console.error('[QuotaMonitor] refreshNow error:', e)
    ElMessage.error('刷新失败: ' + (e as Error).message)
  }
  refreshing.value = false
}

onMounted(() => {
  fetchKeys()
  autoRefreshInterval.value = window.setInterval(fetchKeys, 10000) // 每10秒刷新
})

onUnmounted(() => {
  if (autoRefreshInterval.value) {
    clearInterval(autoRefreshInterval.value)
  }
})
</script>

<template>
  <div class="quota-monitor">
    <div class="header">
      <div class="title">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M12 2C13.1 2 14 2.9 14 4C14 5.1 13.1 6 12 6C10.9 6 10 5.1 10 4C10 2.9 10.9 2 12 2ZM21 9H15V22H13V16H11V22H9V9H3V7H21V9Z" fill="currentColor"/>
        </svg>
        <span>用量统计</span>
      </div>
      <button class="refresh-btn" :class="{ refreshing }" @click="refreshNow" :disabled="refreshing">
        <svg v-if="!refreshing" width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M17.65 6.35C16.2 4.9 14.21 4 12 4C7.58 4 4 7.58 4 12C4 16.42 7.58 20 12 20C15.73 20 18.84 17.45 19.73 14H17.65C16.83 16.33 14.61 18 12 18C8.69 18 6 15.31 6 12C6 8.69 8.69 6 12 6C13.66 6 15.14 6.69 16.22 7.78L13 11H20V4L17.65 6.35Z" fill="currentColor"/>
        </svg>
        <svg v-else width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M12 4V2C6.48 2 2 6.48 2 12C2 17.52 6.48 22 12 22C17.52 22 22 17.52 22 12H20C20 16.42 16.42 20 12 20C7.58 20 4 16.42 4 12C4 7.58 7.58 4 12 4Z" fill="currentColor"/>
        </svg>
        <span>{{ refreshing ? '刷新中...' : '立即刷新' }}</span>
      </button>
    </div>

    <div v-if="loading && keys.length === 0" class="loading">加载中...</div>
    <div v-else-if="keys.length === 0" class="empty">暂无已配置的 Key</div>
    <div v-else class="keys-grid">
      <div
        v-for="k in keys"
        :key="`${k.key_id}`"
        class="key-card"
      >
        <div class="key-header">
          <div class="vendor-name">智谱 AI</div>
          <div class="key-name">{{ k.key_name }}</div>
        </div>
        <div class="key-body">
          <div class="pct-bar">
            <div class="bar-bg"></div>
            <div class="bar-fill" :style="{ width: `${k.pct * 100}%` }"></div>
          </div>
          <div class="pct-text">
            {{ (k.pct * 100).toFixed(1) }}%
            <span v-if="k.level_tier && k.level_tier !== ''" class="level">({{ k.level_tier }})</span>
          </div>
          <div class="status-text">{{ statusLabel(k.status) }}</div>
          <div v-if="k.next_reset" class="reset-time">重置时间：{{ k.next_reset }}</div>
          <div v-if="k.updated_at" class="updated-at">最后更新：{{ k.updated_at }}</div>
          <div v-if="k.error" class="error">{{ k.error }}</div>
        </div>
      </div>
    </div>

    <div class="footer">
      <p>说明：自动用量查询当前仅支持智谱 AI。</p>
    </div>
  </div>
</template>

<style scoped>
.quota-monitor {
  padding: 24px;
  color: #e1e1e1;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.title {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 24px;
  font-weight: 600;
}

.refresh-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: #3a3a3a;
  border: 1px solid #4a4a4a;
  border-radius: 8px;
  color: #e1e1e1;
  cursor: pointer;
  transition: all 0.2s;
}

.refresh-btn:hover:not(:disabled) {
  background: #4a4a4a;
}

.refresh-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.loading, .empty {
  text-align: center;
  padding: 48px;
  color: #999;
}

.keys-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.key-card {
  background: #2a2a2a;
  border: 1px solid #3a3a3a;
  border-radius: 12px;
  overflow: hidden;
  transition: all 0.2s;
}

.key-card:hover {
  border-color: #5a5a5a;
}

.key-header {
  padding: 16px;
  background: #333;
  border-bottom: 1px solid #3a3a3a;
}

.vendor-name {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 4px;
}

.key-name {
  font-size: 14px;
  color: #999;
}

.key-body {
  padding: 16px;
}

.pct-bar {
  position: relative;
  height: 8px;
  margin-bottom: 8px;
}

.bar-bg {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: #333;
  border-radius: 4px;
}

.bar-fill {
  position: absolute;
  top: 0;
  left: 0;
  bottom: 0;
  background: #6366f1;
  border-radius: 4px;
  transition: width 0.3s;
}

.pct-text {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 8px;
}

.level {
  font-size: 14px;
  color: #999;
  font-weight: 400;
}

.status-text {
  font-size: 14px;
  color: #999;
  margin-bottom: 8px;
}

.reset-time, .updated-at {
  font-size: 12px;
  color: #777;
  margin-bottom: 4px;
}

.error {
  font-size: 12px;
  color: #f87171;
  margin-top: 8px;
  padding: 8px;
  background: rgba(248, 113, 113, 0.1);
  border-radius: 4px;
}

.footer {
  margin-top: 32px;
  padding: 16px;
  background: #2a2a2a;
  border: 1px solid #3a3a3a;
  border-radius: 8px;
  font-size: 14px;
  color: #999;
}

.footer p {
  margin: 0;
}
</style>