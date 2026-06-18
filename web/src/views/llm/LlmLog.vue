<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'
import { formatTime } from '../../utils/time'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface ConvDetail {
  username: string
  models: string[]
  key_names: string[]
  prompt_tokens: number
  completion_tokens: number
  latency_ms: number
  errors: string[] | null
  user_message: string
  failover_count: number
}

interface LogEntry {
  id: number
  ts: string
  category: string
  level: string
  message: string
  detail: string
  // parsed
  parsed?: ConvDetail
}

const logs = ref<LogEntry[]>([])
const loading = ref(true)
const maxId = ref(0)
const paused = ref(false)
const expandedIds = ref<Set<number>>(new Set())

// 分页
const logPageSize = 50
const logCurrentPage = ref(1)
const pagedLogs = computed(() => {
  const start = (logCurrentPage.value - 1) * logPageSize
  return logs.value.slice(start, start + logPageSize)
})
const logTotalPages = computed(() => Math.ceil(logs.value.length / logPageSize))
function goLogPage(page: number) {
  if (page >= 1 && page <= logTotalPages.value) logCurrentPage.value = page
}

let refreshTimer: ReturnType<typeof setInterval> | null = null

function parseDetail(raw: string): ConvDetail | undefined {
  if (!raw) return undefined
  try {
    const d = JSON.parse(raw)
    return {
      username: d.username || '',
      models: d.models || [],
      key_names: d.key_names || [],
      prompt_tokens: d.prompt_tokens || 0,
      completion_tokens: d.completion_tokens || 0,
      latency_ms: d.latency_ms || 0,
      errors: d.errors || null,
      user_message: d.user_message || '',
      failover_count: d.failover_count || 0,
    }
  } catch {
    return undefined
  }
}

async function fetchLogs(incremental = false) {
  if (paused.value && incremental) return
  try {
    const params = new URLSearchParams({
      limit: '200',
      category: 'conversation',
    })
    if (incremental && maxId.value > 0) params.set('after_id', String(maxId.value))
    const response = await fetch(`${API_BASE}/api/logs?${params}`, { headers: authHeaders() })
    const result = await response.json()
    if (result?.ok) {
      const d = result.data || {}
      const newLogs: LogEntry[] = (d.logs || []).map((l: any) => ({
        ...l,
        parsed: parseDetail(l.detail),
      }))
      if (incremental && maxId.value > 0) {
        if (newLogs.length > 0) {
          logs.value = [...newLogs, ...logs.value].slice(0, 200)
        }
      } else {
        logs.value = newLogs
      }
      if (d.max_id) maxId.value = d.max_id
    }
  } catch (e: any) {
    console.error('fetch logs failed:', e)
    ElMessage.error('加载日志失败: ' + (e?.message || '未知错误'))
  }
  loading.value = false
}

async function clearLogs() {
  if (!confirm('确定清空所有对话记录？')) return
  try {
    const res = await fetch(`${API_BASE}/api/logs/clear`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({}),
    })
    const data = await res.json()
    if (data.ok) {
      logs.value = []
      maxId.value = 0
      ElMessage.success('对话记录已清空')
    } else {
      ElMessage.error(data.error || '清空失败')
    }
  } catch (e) {
    console.error('clear logs failed:', e)
    ElMessage.error('清空失败: ' + (e as Error).message)
  }
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

function formatLatency(ms: number) {
  if (ms >= 1000) return (ms / 1000).toFixed(1) + 's'
  return ms + 'ms'
}

function formatTokens(n: number) {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

onMounted(() => {
  fetchLogs(false)
  refreshTimer = setInterval(() => fetchLogs(true), 3000)
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
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
        <span>对话记录</span>
        <span class="log-count">{{ logs.length }} 条</span>
      </div>
      <div class="log-actions">
        <button class="btn-icon" :class="{ active: paused }" @click="togglePause" :title="paused ? '继续刷新' : '暂停刷新'">
          <svg v-if="!paused" width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        </button>
        <button class="btn-icon" @click="clearLogs" title="清空记录">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Log List -->
    <div class="log-list" v-if="!loading">
      <div v-if="logs.length === 0" class="log-empty">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
        <span>暂无对话记录</span>
      </div>
      <template v-else>
        <div
          v-for="log in pagedLogs"
          :key="log.id"
          class="log-item"
          :class="{ 'has-detail': log.parsed && log.parsed.user_message }"
        >
          <div class="log-row" @click="toggleDetail(log.id)">
            <!-- 时间 -->
            <span class="log-time">{{ formatTime(log.ts) }}</span>

            <!-- 用户名 badge -->
            <span class="user-badge">
              {{ log.parsed?.username || '未知' }}
            </span>

            <!-- 成功/失败图标 -->
            <span class="status-icon" :class="log.parsed?.errors?.length ? 'fail' : 'success'">
              <svg v-if="log.parsed?.errors?.length" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
              <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12"/>
              </svg>
            </span>

            <!-- 模型列表 -->
            <span class="model-list" v-if="log.parsed?.models?.length">
              <span v-for="(m, i) in log.parsed.models" :key="i" class="model-tag">
                {{ m }} ({{ log.parsed.key_names?.[i] || '?' }})<span v-if="i < log.parsed.models.length - 1">,</span>
              </span>
            </span>

            <!-- Token & 耗时 -->
            <span class="meta-info" v-if="log.parsed">
              <span class="meta-item">输入 {{ formatTokens(log.parsed.prompt_tokens) }}</span>
              <span class="meta-sep">/</span>
              <span class="meta-item">输出 {{ formatTokens(log.parsed.completion_tokens) }}</span>
              <span class="meta-sep">/</span>
              <span class="meta-item">耗时 {{ formatLatency(log.parsed.latency_ms) }}</span>
            </span>

            <!-- 展开箭头 -->
            <svg
              v-if="log.parsed && log.parsed.user_message"
              class="log-expand"
              :class="{ rotated: expandedIds.has(log.id) }"
              width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
            >
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </div>

          <!-- 展开详情 -->
          <transition name="expand">
            <div v-if="expandedIds.has(log.id) && log.parsed" class="log-detail">
              <!-- 用户问题 -->
              <div v-if="log.parsed.user_message" class="detail-block">
                <div class="detail-label user-label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
                  问
                </div>
                <div class="detail-content user-content">{{ log.parsed.user_message }}</div>
              </div>
              <!-- 错误信息 -->
              <div v-if="log.parsed.errors?.length" class="detail-block">
                <div class="detail-label error-label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                  错误
                </div>
                <div class="detail-content error-content">
                  <div v-for="(err, i) in log.parsed.errors" :key="i">{{ err }}</div>
                </div>
              </div>
            </div>
          </transition>
        </div>

        <!-- Pagination -->
        <div v-if="logTotalPages > 1" class="log-pagination">
          <button class="pg-btn" :disabled="logCurrentPage <= 1" @click="goLogPage(logCurrentPage - 1)">上一页</button>
          <template v-for="p in logTotalPages" :key="p">
            <button v-if="p === 1 || p === logTotalPages || Math.abs(p - logCurrentPage) <= 1" class="pg-btn" :class="{ active: p === logCurrentPage }" @click="goLogPage(p)">{{ p }}</button>
            <span v-else-if="p === 2 && logCurrentPage > 3 || p === logTotalPages - 1 && logCurrentPage < logTotalPages - 2" class="pg-ellipsis">...</span>
          </template>
          <button class="pg-btn" :disabled="logCurrentPage >= logTotalPages" @click="goLogPage(logCurrentPage + 1)">下一页</button>
          <span class="pg-info">共 {{ logs.length }} 条，第 {{ logCurrentPage }}/{{ logTotalPages }} 页</span>
        </div>
      </template>
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
  min-height: 0;
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

/* Log List */
.log-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: #f1f5f9;
  border-radius: 8px;
  padding: 2px;
}

.log-list::-webkit-scrollbar { width: 5px; }
.log-list::-webkit-scrollbar-track { background: transparent; }
.log-list::-webkit-scrollbar-thumb { background: #cbd5e1; border-radius: 3px; }

.log-item {
  background: #fff;
  border-radius: 6px;
  transition: background 0.15s;
}
.log-item:hover { background: #f8fafc; }
.log-item.has-detail { cursor: pointer; }

.log-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  user-select: none;
}

.log-time {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  color: #94a3b8;
  flex-shrink: 0;
  min-width: 140px;
}

/* User badge */
.user-badge {
  font-size: 12px;
  font-weight: 600;
  color: #fff;
  background: #6366f1;
  padding: 2px 10px;
  border-radius: 4px;
  flex-shrink: 0;
  max-width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Status icon */
.status-icon {
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.status-icon.success { color: #22c55e; }
.status-icon.fail { color: #ef4444; }

/* Model list */
.model-list {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  color: #3b82f6;
  flex-shrink: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.model-tag { display: inline; }

/* Meta info */
.meta-info {
  font-size: 12px;
  color: #64748b;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
}
.meta-item { white-space: nowrap; }
.meta-sep { color: #cbd5e1; }

/* Expand arrow */
.log-expand {
  flex-shrink: 0;
  color: #94a3b8;
  transition: transform 0.2s;
}
.log-expand.rotated { transform: rotate(180deg); }

/* Detail panel */
.log-detail {
  padding: 8px 12px 12px 162px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: #f8fafc;
  border-radius: 0 0 6px 6px;
  border-top: 1px solid #f1f5f9;
}

.detail-block {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.detail-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  flex-shrink: 0;
  padding: 2px 8px;
  border-radius: 4px;
  min-width: 32px;
  justify-content: center;
}
.user-label { background: #eff6ff; color: #3b82f6; }
.ai-label { background: #f0fdf4; color: #22c55e; }
.error-label { background: #fef2f2; color: #ef4444; }

.detail-content {
  font-size: 13px;
  line-height: 1.6;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
  min-width: 0;
}
.user-content { background: #f8fafc; padding: 8px 12px; border-radius: 6px; }
.ai-content { background: #f8fafc; padding: 8px 12px; border-radius: 6px; max-height: 300px; overflow-y: auto; }
.error-content { background: #fef2f2; padding: 8px 12px; border-radius: 6px; color: #dc2626; font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace; font-size: 12px; }

/* Transition */
.expand-enter-active { transition: all 0.2s ease-out; }
.expand-leave-active { transition: all 0.15s ease-in; }
.expand-enter-from, .expand-leave-to {
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

@keyframes spin { to { transform: rotate(360deg); } }

/* Pagination */
.log-pagination {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 12px 0 4px;
  border-top: 1px solid #e2e8f0;
  margin-top: 4px;
  flex-wrap: wrap;
}

.pg-btn {
  padding: 5px 10px;
  border: 1px solid #e2e8f0;
  border-radius: 5px;
  background: #fff;
  color: #475569;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.15s;
}
.pg-btn:hover:not(:disabled):not(.active) {
  background: #f1f5f9;
  border-color: #cbd5e1;
}
.pg-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.pg-btn.active { background: #6366f1; color: #fff; border-color: #6366f1; }
.pg-ellipsis { padding: 0 4px; color: #94a3b8; font-size: 12px; }
.pg-info { margin-left: 8px; font-size: 11px; color: #94a3b8; }
</style>
