<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface ErrorEntry {
  id: number
  ts: string
  user_id: number
  username: string
  vendor_id: number
  key_id: string
  model_id: string
  error: string
  error_type: string
  latency_ms: number
}

interface ErrorStats {
  total: number
  by_model: { model_id: string; count: number; last_ts: string }[]
  by_error: { error: string; count: number; last_ts: string; model_id: string }[]
  by_category: { type: string; count: number }[]
  by_day: { date: string; count: number }[]
}

const loading = ref(true)
const stats = ref<ErrorStats | null>(null)
const errors = ref<ErrorEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const modelFilter = ref('')
const userFilter = ref('')
const autoRefresh = ref(true)
let timer: ReturnType<typeof setInterval> | null = null

// 熔断状态
interface BreakerKey {
  model_id: string
  key_id: string
  opened_at: string
  reason: string
}
const breakerKeys = ref<BreakerKey[]>([])

const totalPages = computed(() => Math.ceil(total.value / pageSize))

async function fetchStats() {
  try {
    const res = await fetch(`${API_BASE}/api/llm/error-stats?days=7`, { headers: authHeaders(), cache: 'no-cache' })
    const data = await res.json()
    if (data.ok) stats.value = data.data
  } catch (e) {
    console.error('[ErrorLog] fetchStats error:', e)
  }
}

async function fetchBreakerStatus() {
  try {
    const res = await fetch(`${API_BASE}/api/llm/circuit-status`, { headers: authHeaders(), cache: 'no-cache' })
    const data = await res.json()
    if (data.ok) breakerKeys.value = data.data.open_keys || []
  } catch (e) {
    console.error('[ErrorLog] fetchBreakerStatus error:', e)
  }
}

async function fetchErrors() {
  try {
    const params = new URLSearchParams({ page: String(page.value), page_size: String(pageSize) })
    if (modelFilter.value) params.set('model', modelFilter.value)
    if (userFilter.value) params.set('user', userFilter.value)
    const res = await fetch(`${API_BASE}/api/llm/error-log?${params}`, { headers: authHeaders(), cache: 'no-cache' })
    const data = await res.json()
    if (data.ok) {
      errors.value = data.data.list
      total.value = data.data.total
    }
  } catch (e) {
    console.error('[ErrorLog] fetchErrors error:', e)
  } finally {
    loading.value = false
  }
}

function refresh() {
  fetchStats()
  fetchBreakerStatus()
  fetchErrors()
}

function goPage(p: number) {
  if (p < 1 || p > totalPages.value) return
  page.value = p
  fetchErrors()
}

function copyError(text: string, event: Event) {
  event.stopPropagation()
  navigator.clipboard.writeText(text).then(() => {
    // 短暂提示
  })
}

function formatTime(ts: string) {
  if (!ts) return ''
  return ts.replace('T', ' ').substring(0, 19)
}

function formatLatency(ms: number) {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

onMounted(() => {
  refresh()
  timer = setInterval(() => {
    if (autoRefresh.value) refresh()
  }, 10000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="error-log">
    <!-- 统计卡片 -->
    <div class="stats-row" v-if="stats">
      <div class="stat-card danger">
        <div class="stat-value">{{ stats.total }}</div>
        <div class="stat-label">近7天错误总数</div>
      </div>
      <div class="stat-card" v-for="c in stats.by_category?.slice(0, 4)" :key="c.type">
        <div class="stat-value">{{ c.count }}</div>
        <div class="stat-label">{{ c.type }}</div>
      </div>
    </div>

    <!-- 熔断状态 -->
    <div class="breaker-section" v-if="breakerKeys.length > 0">
      <div class="breaker-header">
        <span class="breaker-badge">{{ breakerKeys.length }}</span>
        <span class="breaker-title">个模型+Key 组合已熔断</span>
      </div>
      <div class="breaker-list">
        <div class="breaker-item" v-for="b in breakerKeys" :key="b.model_id + b.key_id">
          <span class="breaker-model">{{ b.model_id }}</span>
          <span class="breaker-key">Key: {{ b.key_id }}</span>
          <span class="breaker-time">熔断于 {{ formatTime(b.opened_at) }}</span>
          <span class="breaker-reason" :title="b.reason">{{ b.reason }}</span>
        </div>
      </div>
    </div>

    <!-- Top 模型 & Top 错误类型 -->
    <div class="top-row" v-if="stats">
      <div class="top-card">
        <div class="top-title">Top 错误模型</div>
        <div class="top-item" v-for="m in stats.by_model?.slice(0, 5)" :key="m.model_id">
          <span class="top-name">{{ m.model_id }}</span>
          <span class="top-count">{{ m.count }} 次</span>
        </div>
        <div v-if="!stats.by_model?.length" class="top-empty">暂无数据</div>
      </div>
      <div class="top-card">
        <div class="top-title">Top 错误类型</div>
        <div class="top-item" v-for="e in stats.by_error?.slice(0, 5)" :key="e.error">
          <span class="top-name error-text" :title="e.error">{{ e.error }}</span>
          <span class="top-error-model">{{ e.model_id }}</span>
          <span class="top-count">{{ e.count }} 次</span>
          <button class="copy-btn-sm" @click="copyError(e.error, $event)" title="复制错误消息">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
          </button>
        </div>
        <div v-if="!stats.by_error?.length" class="top-empty">暂无数据</div>
      </div>
    </div>

    <!-- 筛选栏 -->
    <div class="filter-bar">
      <input v-model="modelFilter" placeholder="按模型筛选..." class="filter-input" @keyup.enter="page=1;fetchErrors()" />
      <input v-model="userFilter" placeholder="按用户筛选..." class="filter-input" @keyup.enter="page=1;fetchErrors()" />
      <button class="filter-btn" @click="page=1;fetchErrors()">筛选</button>
      <button class="filter-btn secondary" @click="modelFilter='';userFilter='';page=1;fetchErrors()">重置</button>
      <label class="auto-refresh">
        <input type="checkbox" v-model="autoRefresh" /> 自动刷新
      </label>
    </div>

    <!-- 错误列表 -->
    <div class="error-list" v-if="!loading">
      <div v-if="errors.length === 0" class="empty-state">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#c0c4cc" stroke-width="1.5">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <p>暂无错误记录</p>
      </div>

      <div v-for="entry in errors" :key="entry.id" class="error-item">
        <div class="error-row">
          <span class="error-time">{{ formatTime(entry.ts) }}</span>
          <span class="error-model">{{ entry.model_id }}</span>
          <span class="error-user">{{ entry.username }}</span>
          <span class="error-keyname">{{ entry.key_name || entry.key_id }}</span>
          <button class="copy-btn" @click="copyError(entry.error, $event)" title="复制错误消息">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
          </button>
        </div>
        <div class="error-detail">{{ entry.error }}</div>
      </div>
    </div>

    <!-- 分页 -->
    <div class="pagination" v-if="totalPages > 1">
      <button :disabled="page <= 1" @click="goPage(page - 1)">上一页</button>
      <span class="page-info">{{ page }} / {{ totalPages }}（共 {{ total }} 条）</span>
      <button :disabled="page >= totalPages" @click="goPage(page + 1)">下一页</button>
    </div>

    <!-- 加载 -->
    <div v-if="loading" class="loading">
      <div class="spinner"></div>
      <span>加载中...</span>
    </div>
  </div>
</template>

<style scoped>
.error-log {
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 1200px;
}

/* 统计卡片 */
.stats-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}
.stat-card {
  background: #fff;
  border-radius: 10px;
  padding: 16px;
  text-align: center;
  box-shadow: 0 1px 3px rgba(0,0,0,.08);
}
.stat-card.danger {
  background: #fef2f2;
  border: 1px solid #fecaca;
}
.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #303133;
}
.stat-card.danger .stat-value {
  color: #ef4444;
}
.stat-label {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

/* Top 排行 */
.top-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

/* 熔断状态 */
.breaker-section {
  background: #fff7ed;
  border: 1px solid #fdba74;
  border-radius: 10px;
  padding: 14px 16px;
}
.breaker-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}
.breaker-badge {
  background: #ef4444;
  color: #fff;
  font-size: 14px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 4px;
}
.breaker-title {
  font-size: 14px;
  font-weight: 600;
  color: #c2410c;
}
.breaker-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.breaker-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: #fff;
  border-radius: 6px;
  font-size: 13px;
}
.breaker-model {
  color: #6366f1;
  font-weight: 500;
  min-width: 120px;
}
.breaker-key {
  color: #909399;
  font-size: 12px;
  min-width: 80px;
}
.breaker-time {
  color: #909399;
  font-size: 12px;
}
.breaker-reason {
  color: #e34d59;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-left: auto;
  max-width: 300px;
}
.top-card {
  background: #fff;
  border-radius: 10px;
  padding: 16px;
  box-shadow: 0 1px 3px rgba(0,0,0,.08);
}
.top-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 12px;
}
.top-item {
  display: flex;
  justify-content: space-between;
  padding: 6px 0;
  border-bottom: 1px solid #f5f5f5;
  font-size: 13px;
}
.top-name {
  color: #606266;
}
.top-name.error-text {
  color: #ef4444;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 200px;
}
.top-error-model {
  color: #6366f1;
  font-size: 11px;
  white-space: nowrap;
  margin-left: auto;
}
.copy-btn-sm {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: 1px solid #e5e7eb;
  border-radius: 4px;
  background: #fff;
  color: #909399;
  cursor: pointer;
  flex-shrink: 0;
  transition: all .15s;
}
.copy-btn-sm:hover {
  color: #6366f1;
  border-color: #6366f1;
}
.top-count {
  color: #ef4444;
  font-weight: 600;
}
.top-empty {
  color: #c0c4cc;
  font-size: 13px;
  text-align: center;
  padding: 12px;
}

/* 筛选栏 */
.filter-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.filter-input {
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 13px;
  width: 140px;
  outline: none;
}
.filter-input:focus {
  border-color: #6366f1;
}
.filter-btn {
  border: none;
  border-radius: 6px;
  padding: 6px 14px;
  font-size: 13px;
  cursor: pointer;
  background: #6366f1;
  color: #fff;
}
.filter-btn.secondary {
  background: #f0f0f0;
  color: #606266;
}
.auto-refresh {
  font-size: 13px;
  color: #909399;
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}

/* 错误列表 */
.error-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.error-item {
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 1px 3px rgba(0,0,0,.06);
  cursor: pointer;
  transition: box-shadow .2s;
}
.error-item:hover {
  box-shadow: 0 2px 8px rgba(0,0,0,.1);
}
.error-row {
  display: flex;
  align-items: center;
  padding: 10px 14px;
  gap: 12px;
  font-size: 13px;
}
.error-time {
  color: #909399;
  font-size: 12px;
  min-width: 130px;
}
.error-model {
  color: #6366f1;
  font-weight: 500;
  min-width: 100px;
}
.error-user {
  color: #606266;
  min-width: 60px;
}
.error-keyname {
  color: #909399;
  font-size: 12px;
  min-width: 80px;
  flex: 1;
}
.copy-btn {
  margin-left: auto;
}

.copy-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fff;
  color: #909399;
  cursor: pointer;
  flex-shrink: 0;
  transition: all .15s;
}
.copy-btn:hover {
  color: #6366f1;
  border-color: #6366f1;
  background: #f5f3ff;
}

/* 展开详情 */
.error-detail {
  padding: 8px 14px 10px;
  font-size: 12px;
  color: #e34d59;
  line-height: 1.6;
  word-break: break-all;
  white-space: pre-wrap;
}

/* 分页 */
.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 12px;
}
.pagination button {
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  padding: 6px 14px;
  background: #fff;
  cursor: pointer;
  font-size: 13px;
}
.pagination button:disabled {
  opacity: .4;
  cursor: not-allowed;
}
.page-info {
  font-size: 13px;
  color: #606266;
}

/* 空状态 */
.empty-state {
  text-align: center;
  padding: 48px;
  color: #c0c4cc;
}
.empty-state p {
  margin-top: 12px;
  font-size: 14px;
}

/* 加载 */
.loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: #909399;
  font-size: 13px;
}
.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid #e5e7eb;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin .6s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* 展开动画 */
.expand-enter-active, .expand-leave-active {
  transition: all .2s ease;
  overflow: hidden;
}
.expand-enter-from, .expand-leave-to {
  opacity: 0;
  max-height: 0;
}
.expand-enter-to, .expand-leave-from {
  opacity: 1;
  max-height: 200px;
}
</style>
