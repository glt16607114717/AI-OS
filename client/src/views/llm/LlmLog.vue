<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

const agentRequest = window.aiOS.agentRequest

interface LogEntry {
  id: number
  ts: string
  category: string
  level: string
  message: string
  detail: string
}

const logs = ref<LogEntry[]>([])
const loading = ref(true)
const maxId = ref(0)
const activeCategory = ref('')
const paused = ref(false)
const expandedIds = ref<Set<number>>(new Set())
const autoScroll = ref(true)

let refreshTimer: ReturnType<typeof setInterval> | null = null

const categories = [
  { key: '', label: '全部' },
  { key: 'request', label: '请求' },
  { key: 'route', label: '路由' },
  { key: 'downgrade', label: '降级' },
  { key: 'quota', label: '用量' },
  { key: 'failover', label: '故障' },
  { key: 'error', label: '错误' },
]

async function fetchLogs(incremental = false) {
  if (paused.value && incremental) return
  try {
    const res = await agentRequest('llm_get_logs', {
      limit: 200,
      category: activeCategory.value || '',
      ...(incremental && maxId.value > 0 ? { after_id: maxId.value } : {}),
    })
    if (res?.ok) {
      const newLogs: LogEntry[] = res.logs || []
      if (incremental && maxId.value > 0) {
        if (newLogs.length > 0) {
          // 增量：追加到前面（日志按 id DESC 排序）
          logs.value = [...newLogs, ...logs.value].slice(0, 200)
        }
      } else {
        logs.value = newLogs
      }
      if (res.max_id) maxId.value = res.max_id
    }
  } catch (e) {
    console.error('fetch logs failed:', e)
  }
  loading.value = false
}

async function clearLogs() {
  if (!confirm('确定清空所有日志？')) return
  try {
    await agentRequest('llm_clear_logs', {})
    logs.value = []
    maxId.value = 0
  } catch (e) {
    console.error('clear logs failed:', e)
  }
}

function switchCategory(key: string) {
  activeCategory.value = key
  maxId.value = 0
  loading.value = true
  fetchLogs(false)
}

function toggleDetail(id: number) {
  if (expandedIds.value.has(id)) {
    expandedIds.value.delete(id)
  } else {
    expandedIds.value.add(id)
  }
}

function togglePause() {
  paused.value = !paused.value
}

const categoryColorMap: Record<string, string> = {
  request: '#3b82f6',
  route: '#8b5cf6',
  downgrade: '#f59e0b',
  quota: '#06b6d4',
  failover: '#ef4444',
  error: '#ef4444',
  config: '#10b981',
}

const levelIcon: Record<string, string> = {
  info: '',
  warning: '!',
  error: 'x',
}

onMounted(() => {
  fetchLogs(false)
  refreshTimer = setInterval(() => fetchLogs(true), 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <div class="log-page">
    <!-- Header -->
    <div class="log-header">
      <div class="log-title">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="16" y1="13" x2="8" y2="13" />
          <line x1="16" y1="17" x2="8" y2="17" />
        </svg>
        <span>操作日志</span>
        <span class="log-count">{{ logs.length }} 条</span>
      </div>
      <div class="log-actions">
        <button class="btn-icon" :class="{ active: paused }" @click="togglePause" :title="paused ? '继续刷新' : '暂停刷新'">
          <svg v-if="!paused" width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        </button>
        <button class="btn-icon" @click="clearLogs" title="清空日志">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Category Tabs -->
    <div class="log-tabs">
      <button
        v-for="cat in categories"
        :key="cat.key"
        class="tab-btn"
        :class="{ active: activeCategory === cat.key }"
        @click="switchCategory(cat.key)"
      >
        {{ cat.label }}
      </button>
    </div>

    <!-- Log List -->
    <div class="log-list" v-if="!loading">
      <div v-if="logs.length === 0" class="log-empty">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
        </svg>
        <span>暂无日志</span>
      </div>
      <div
        v-for="log in logs"
        :key="log.id"
        class="log-item"
        :class="[`level-${log.level}`]"
      >
        <div class="log-row" @click="toggleDetail(log.id)">
          <span class="log-time">{{ log.ts.split(' ')[1] || log.ts }}</span>
          <span class="log-badge" :style="{ background: categoryColorMap[log.category] || '#6b7280' }">
            {{ categories.find(c => c.key === log.category)?.label || log.category }}
          </span>
          <span class="log-level-icon" v-if="log.level !== 'info'" :class="`lv-${log.level}`">
            {{ levelIcon[log.level] }}
          </span>
          <span class="log-msg">{{ log.message }}</span>
          <svg v-if="log.detail" class="log-expand" :class="{ rotated: expandedIds.has(log.id) }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>
        </div>
        <transition name="expand">
          <div v-if="expandedIds.has(log.id) && log.detail" class="log-detail">
            {{ log.detail }}
          </div>
        </transition>
      </div>
    </div>

    <!-- Loading -->
    <div v-else class="log-loading">
      <div class="spinner"></div>
      <span>加载中...</span>
    </div>
  </div>
</template>

<style scoped>
.log-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 0;
}

.log-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 12px;
  border-bottom: 1px solid #e5e7eb;
}

.log-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 700;
  color: #1e293b;
}

.log-count {
  font-size: 12px;
  font-weight: 400;
  color: #94a3b8;
  margin-left: 4px;
}

.log-actions {
  display: flex;
  gap: 6px;
}

.btn-icon {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #64748b;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s;
}
.btn-icon:hover {
  background: #f8fafc;
  color: #334155;
  border-color: #cbd5e1;
}
.btn-icon.active {
  background: #fef3c7;
  color: #d97706;
  border-color: #fcd34d;
}

/* Tabs */
.log-tabs {
  display: flex;
  gap: 4px;
  padding: 12px 0;
  overflow-x: auto;
}

.tab-btn {
  padding: 5px 14px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #64748b;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}
.tab-btn:hover {
  background: #f1f5f9;
  color: #334155;
}
.tab-btn.active {
  background: #4f46e5;
  color: #fff;
  border-color: #4f46e5;
}

/* Log List */
.log-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: #f1f5f9;
  border-radius: 8px;
  padding: 2px;
}

.log-list::-webkit-scrollbar {
  width: 5px;
}
.log-list::-webkit-scrollbar-track {
  background: transparent;
}
.log-list::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 3px;
}

.log-item {
  background: #fff;
  border-radius: 6px;
  transition: background 0.15s;
}
.log-item:hover {
  background: #f8fafc;
}
.log-item.level-warning {
  border-left: 3px solid #f59e0b;
}
.log-item.level-error {
  border-left: 3px solid #ef4444;
}

.log-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  user-select: none;
}

.log-time {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  color: #94a3b8;
  flex-shrink: 0;
  width: 64px;
}

.log-badge {
  font-size: 10px;
  font-weight: 600;
  color: #fff;
  padding: 1px 7px;
  border-radius: 4px;
  flex-shrink: 0;
  letter-spacing: 0.5px;
}

.log-level-icon {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  font-size: 10px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  color: #fff;
}
.log-level-icon.lv-warning {
  background: #f59e0b;
}
.log-level-icon.lv-error {
  background: #ef4444;
}

.log-msg {
  font-size: 13px;
  color: #334155;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-expand {
  flex-shrink: 0;
  color: #94a3b8;
  transition: transform 0.2s;
}
.log-expand.rotated {
  transform: rotate(180deg);
}

.log-detail {
  padding: 6px 12px 10px 84px;
  font-size: 12px;
  color: #64748b;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  word-break: break-all;
  background: #f8fafc;
  border-radius: 0 0 6px 6px;
}

/* Transition */
.expand-enter-active {
  transition: all 0.2s ease-out;
}
.expand-leave-active {
  transition: all 0.15s ease-in;
}
.expand-enter-from,
.expand-leave-to {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  padding-bottom: 0;
}

/* Empty */
.log-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 60px 0;
  color: #94a3b8;
  font-size: 14px;
}

/* Loading */
.log-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 60px 0;
  color: #94a3b8;
  font-size: 14px;
}

.spinner {
  width: 18px;
  height: 18px;
  border: 2px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
