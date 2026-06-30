<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'
import { formatTime } from '../../utils/time'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface RequestChainItem {
  ts: string
  model_id: string
  key_name: string
  success: boolean
  error?: string
  latency_ms: number
  total_tokens: number
}

interface ChatSession {
  msg_id: string
  session_id: string
  username: string
  first_ts: string
  total_prompt: number
  total_completion: number
  total_tokens: number
  latency_sec: number
  request_count: number
  error_count: number
  models: string[]
  user_message: string
  request_chain: RequestChainItem[]
}

const sessions = ref<ChatSession[]>([])
const loading = ref(true)
const paused = ref(false)
const expandedMsgIds = ref<Set<string>>(new Set())

// 筛选
const filterUser = ref('')
const filterDateRange = ref<[string, string] | null>(null)

// 用户选项（从已加载数据提取）
const userOptions = computed(() => {
  const names = new Set<string>()
  for (const s of sessions.value) {
    if (s.username) names.add(s.username)
  }
  return Array.from(names).sort()
})

// 筛选后的数据
const filteredSessions = computed(() => {
  let result = sessions.value
  if (filterUser.value) {
    result = result.filter(s => s.username === filterUser.value)
  }
  if (filterDateRange.value && filterDateRange.value.length === 2) {
    const [start, end] = filterDateRange.value
    result = result.filter(s => {
      const d = s.first_ts.substring(0, 10)
      return d >= start && d <= end
    })
  }
  return result
})

// 分页
const pageSize = 50
const currentPage = ref(1)
const pagedSessions = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return filteredSessions.value.slice(start, start + pageSize)
})
const totalPages = computed(() => Math.ceil(filteredSessions.value.length / pageSize))
function goPage(page: number) {
  if (page >= 1 && page <= totalPages.value) currentPage.value = page
}

let refreshTimer: ReturnType<typeof setInterval> | null = null

async function fetchSessions() {
  if (paused.value) return
  try {
    const params = new URLSearchParams({ limit: '200' })
    const response = await fetch(`${API_BASE}/api/chat/sessions?${params}`, { headers: authHeaders() })
    const result = await response.json()
    if (result?.ok) {
      sessions.value = result.data?.sessions || []
    }
  } catch (e: any) {
    console.error('fetch sessions failed:', e)
  }
  loading.value = false
}

function toggleDetail(msgId: string) {
  if (expandedMsgIds.value.has(msgId)) {
    expandedMsgIds.value.delete(msgId)
  } else {
    expandedMsgIds.value.add(msgId)
  }
}

function togglePause() {
  paused.value = !paused.value
  if (!paused.value) fetchSessions()
}

function formatLatency(ms: number) {
  if (ms >= 1000) return (ms / 1000).toFixed(1) + 's'
  return ms + 'ms'
}

function formatDuration(sec: number) {
  if (!sec || sec <= 0) return '0秒'
  const m = Math.floor(sec / 60)
  const s = sec % 60
  if (m > 0) return `${m}分${s}秒`
  return `${s}秒`
}

function formatTokens(n: number) {
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

function shortMsgId(id: string) {
  return id.length > 8 ? id.substring(0, 8) : id
}

// 筛选变化时重置页码
watch([filterUser, filterDateRange], () => { currentPage.value = 1 })

function updateDateRange(idx: number, val: string) {
  const cur = filterDateRange.value || ['', '']
  cur[idx] = val
  if (cur[0] && cur[1]) {
    filterDateRange.value = [cur[0], cur[1]]
  } else if (!cur[0] && !cur[1]) {
    filterDateRange.value = null
  } else {
    filterDateRange.value = [cur[0], cur[1]]
  }
}

function clearFilters() {
  filterUser.value = ''
  filterDateRange.value = null
}

onMounted(() => {
  fetchSessions()
  refreshTimer = setInterval(() => fetchSessions(), 5000)
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
        <span class="log-count">{{ filterUser || (filterDateRange && filterDateRange[0]) ? `${filteredSessions.length}/${sessions.length} 条` : `${sessions.length} 条` }}</span>
      </div>
      <div class="log-actions">
        <select v-model="filterUser" class="filter-select" title="按用户筛选">
          <option value="">全部用户</option>
          <option v-for="u in userOptions" :key="u" :value="u">{{ u }}</option>
        </select>
        <div class="date-filter">
          <input type="date" :value="filterDateRange?.[0] || ''" @input="updateDateRange(0, ($event.target as HTMLInputElement).value)" class="filter-date" title="开始日期">
          <span class="date-sep">~</span>
          <input type="date" :value="filterDateRange?.[1] || ''" @input="updateDateRange(1, ($event.target as HTMLInputElement).value)" class="filter-date" title="结束日期">
          <button v-if="filterUser || (filterDateRange && (filterDateRange[0] || filterDateRange[1]))" class="filter-clear" @click="clearFilters" title="清除筛选">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <button class="btn-icon" :class="{ active: paused }" @click="togglePause" :title="paused ? '继续刷新' : '暂停刷新'">
          <svg v-if="!paused" width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        </button>
      </div>
    </div>

    <!-- Session List -->
    <div class="log-list" v-if="!loading">
      <div v-if="filteredSessions.length === 0" class="log-empty">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
        <span>暂无对话记录</span>
      </div>
      <template v-else>
        <div
          v-for="s in pagedSessions"
          :key="s.msg_id"
          class="log-item"
        >
          <!-- 汇总行 -->
          <div class="log-row" @click="toggleDetail(s.msg_id)">
            <span class="log-time">{{ formatTime(s.first_ts) }}</span>
            <span class="user-badge">{{ s.username || '未知' }}</span>
            <span class="status-icon" :class="s.error_count > 0 ? 'fail' : 'success'">
              <svg v-if="s.error_count > 0" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
              </svg>
              <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12"/>
              </svg>
            </span>
            <span class="msg-id-tag" :title="s.msg_id">{{ shortMsgId(s.msg_id) }}</span>
            <span class="meta-info">
              <span class="meta-item">输入 {{ formatTokens(s.total_prompt) }}</span>
              <span class="meta-sep">/</span>
              <span class="meta-item">输出 {{ formatTokens(s.total_completion) }}</span>
              <span class="meta-sep">/</span>
              <span class="meta-item">耗时 {{ formatDuration(s.latency_sec) }}</span>
              <span class="meta-sep">/</span>
              <span class="meta-item req-count">{{ s.request_count }} 次请求</span>
            </span>
            <svg
              class="log-expand"
              :class="{ rotated: expandedMsgIds.has(s.msg_id) }"
              width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
            >
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </div>

          <!-- 展开详情 -->
          <transition name="expand">
            <div v-if="expandedMsgIds.has(s.msg_id)" class="log-detail">
              <!-- 用户问题 -->
              <div v-if="s.user_message" class="detail-block">
                <div class="detail-label user-label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
                  问
                </div>
                <div class="detail-content user-content">{{ s.user_message }}</div>
              </div>

              <!-- 请求链路 -->
              <div v-if="s.request_chain?.length" class="detail-block">
                <div class="detail-label chain-label">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
                  链路
                </div>
                <div class="detail-content chain-content">
                  <div v-for="(req, i) in s.request_chain" :key="i" class="chain-item" :class="{ 'has-error': !req.success }">
                    <div class="chain-main">
                      <span class="chain-index">{{ i + 1 }}</span>
                      <span class="chain-model">{{ req.model_id }}</span>
                      <span class="chain-key" v-if="req.key_name">{{ req.key_name }}</span>
                      <span class="chain-tokens">{{ formatTokens(req.total_tokens) }} tokens</span>
                      <span class="chain-time">{{ formatLatency(req.latency_ms) }}</span>
                      <span class="chain-ts">{{ formatTime(req.ts) }}</span>
                      <span v-if="req.success" class="chain-status ok">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                      </span>
                      <span v-else class="chain-status fail">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                      </span>
                    </div>
                    <div v-if="!req.success && req.error" class="chain-error-detail">{{ req.error }}</div>
                  </div>
                </div>
              </div>
            </div>
          </transition>
        </div>

        <!-- Pagination -->
        <div v-if="totalPages > 1" class="log-pagination">
          <button class="pg-btn" :disabled="currentPage <= 1" @click="goPage(currentPage - 1)">上一页</button>
          <template v-for="p in totalPages" :key="p">
            <button v-if="p === 1 || p === totalPages || Math.abs(p - currentPage) <= 1" class="pg-btn" :class="{ active: p === currentPage }" @click="goPage(p)">{{ p }}</button>
            <span v-else-if="p === 2 && currentPage > 3 || p === totalPages - 1 && currentPage < totalPages - 2" class="pg-ellipsis">...</span>
          </template>
          <button class="pg-btn" :disabled="currentPage >= totalPages" @click="goPage(currentPage + 1)">下一页</button>
          <span class="pg-info">共 {{ filteredSessions.length }} 条，第 {{ currentPage }}/{{ totalPages }} 页</span>
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
  align-items: center;
}

.filter-select {
  font-size: 12px;
  padding: 4px 8px;
  border: 1px solid #e2e8f0;
  border-radius: 5px;
  background: #fff;
  color: #475569;
  cursor: pointer;
  max-width: 100px;
}
.filter-select:focus { outline: none; border-color: #6366f1; }

.date-filter { display: flex; align-items: center; gap: 4px; }
.filter-date {
  font-size: 12px;
  padding: 4px 6px;
  border: 1px solid #e2e8f0;
  border-radius: 5px;
  background: #fff;
  color: #475569;
  width: 120px;
}
.filter-date:focus { outline: none; border-color: #6366f1; }
.date-sep { font-size: 11px; color: #94a3b8; }
.filter-clear {
  width: 24px; height: 24px;
  border-radius: 4px;
  border: 1px solid #fee2e2;
  background: #fef2f2;
  color: #ef4444;
  cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  padding: 0;
}
.filter-clear:hover { background: #fee2e2; }

.btn-icon {
  width: 32px; height: 32px;
  border-radius: 6px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #64748b;
  cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  transition: all 0.15s;
}
.btn-icon:hover { background: #f8fafc; color: #334155; border-color: #cbd5e1; }
.btn-icon.active { background: #fef3c7; color: #d97706; border-color: #fcd34d; }

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

.log-item { background: #fff; border-radius: 6px; transition: background 0.15s; }
.log-item:hover { background: #f8fafc; }
.log-item:hover .log-detail { background: #f8fafc; }

.log-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  user-select: none;
  cursor: pointer;
}

.log-time {
  font-size: 12px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  color: #94a3b8;
  flex-shrink: 0;
  min-width: 140px;
}

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

.status-icon {
  flex-shrink: 0;
  width: 18px; height: 18px;
  display: flex; align-items: center; justify-content: center;
}
.status-icon.success { color: #22c55e; }
.status-icon.fail { color: #ef4444; }

.msg-id-tag {
  font-size: 11px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  color: #94a3b8;
  background: #f1f5f9;
  padding: 1px 6px;
  border-radius: 3px;
  flex-shrink: 0;
}

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
.req-count { color: #94a3b8; }

.log-expand { flex-shrink: 0; color: #94a3b8; transition: transform 0.2s; }
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

.detail-block { display: flex; gap: 10px; align-items: flex-start; }

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
.chain-label { background: #f0fdf4; color: #22c55e; }

.detail-content {
  font-size: 13px;
  line-height: 1.6;
  color: #334155;
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
  min-width: 0;
}
.user-content { background: #fff; padding: 8px 12px; border-radius: 6px; }

/* Request chain */
.chain-content { display: flex; flex-direction: column; gap: 4px; }
.chain-item {
  padding: 4px 8px;
  background: #fff;
  border-radius: 4px;
  font-size: 12px;
}
.chain-item.has-error { background: #fef2f2; }
.chain-main {
  display: flex;
  align-items: center;
  gap: 8px;
}
.chain-error-detail {
  margin-top: 4px;
  padding: 6px 8px;
  background: #fee2e2;
  border-radius: 4px;
  color: #dc2626;
  font-size: 11px;
  font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  border-left: 3px solid #ef4444;
}
.chain-index {
  width: 18px; height: 18px;
  border-radius: 50%;
  background: #6366f1;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.chain-model { color: #3b82f6; font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace; }
.chain-key { color: #8b5cf6; font-size: 11px; background: #f5f3ff; padding: 1px 6px; border-radius: 3px; }
.chain-tokens { color: #64748b; }
.chain-time { color: #94a3b8; }
.chain-ts { color: #94a3b8; font-family: 'SF Mono', 'Cascadia Code', Consolas, monospace; font-size: 11px; margin-left: auto; }
.chain-status.ok { color: #22c55e; display: flex; align-items: center; }
.chain-status.fail { color: #ef4444; display: flex; align-items: center; gap: 4px; }

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
  width: 18px; height: 18px;
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
.pg-btn:hover:not(:disabled):not(.active) { background: #f1f5f9; border-color: #cbd5e1; }
.pg-btn:disabled { opacity: 0.4; cursor: not-allowed; }
.pg-btn.active { background: #6366f1; color: #fff; border-color: #6366f1; }
.pg-ellipsis { padding: 0 4px; color: #94a3b8; font-size: 12px; }
.pg-info { margin-left: 8px; font-size: 11px; color: #94a3b8; }
</style>
