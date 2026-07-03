<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { formatTimeShort as formatTime } from '../../utils/time'

const totalPages = computed(() => Math.ceil(totalSuggestions.value / pageSize.value) || 1)

interface Suggestion {
  id: number
  report_date: string
  category: string
  title: string
  content: string
  priority: string
  status: string
  created_at: string
  processed_at: string | null
}

import { API_BASE } from '../../api'

const suggestions = ref<Suggestion[]>([])
const totalSuggestions = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const analyzing = ref(false)
const processingIds = ref<Set<number>>(new Set())

const filterStatus = ref<'all' | 'pending' | 'processed' | 'ignored'>('pending')
const filterDate = ref('')
const expandedIds = ref<Set<number>>(new Set())

const categoryMap: Record<string, string> = {
  skill: '技能封装',
  rule: '规则加强',
  bug: 'Bug 归因',
  tech_vision: '技术视野',
  prompt: '提示词优化',
  workflow: '流程工具',
  env: '环境配置',
  other: '其他建议',
}

const categoryColorMap: Record<string, string> = {
  skill: '#409eff',
  rule: '#f56c6c',
  bug: '#e6a23c',
  tech_vision: '#67c23a',
  prompt: '#67c23a',
  workflow: '#9b59b6',
  env: '#1abc9c',
  other: '#909399',
}

const priorityColorMap: Record<string, string> = {
  high: '#f56c6c',
  medium: '#e6a23c',
  low: '#409eff',
}

const priorityLabelMap: Record<string, string> = {
  high: '高',
  medium: '中',
  low: '低',
}

const filteredSuggestions = computed(() => {
  return suggestions.value.filter(s => {
    if (filterStatus.value !== 'all' && s.status !== filterStatus.value) return false
    if (filterDate.value && s.report_date !== filterDate.value) return false
    return true
  })
})

function toggleExpand(id: number) {
  if (expandedIds.value.has(id)) {
    expandedIds.value.delete(id)
  } else {
    expandedIds.value.add(id)
  }
}

function authHeaders() {
  const token = localStorage.getItem('aios_token') || ''
  return { 'Authorization': `Bearer ${token}` }
}

async function fetchSuggestions() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (filterStatus.value !== 'all') params.set('status', filterStatus.value)
    if (filterDate.value) params.set('date', filterDate.value)
    params.set('page', String(currentPage.value))
    params.set('page_size', String(pageSize.value))
    const qs = params.toString()
    const res = await fetch(`${API_BASE}/api/ai-advisor/suggestions${qs ? '?' + qs : ''}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      const payload = data.data || {}
      suggestions.value = payload.data || []
      totalSuggestions.value = payload.total || 0
    } else {
      ElMessage.error(data.error || '加载建议失败')
    }
  } catch (e: any) {
    console.error('[AiAdvice] fetchSuggestions error:', e)
    ElMessage.error('加载建议失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

async function pollAfterTrigger(maxAttempts = 20) {
  const prevCount = totalSuggestions.value
  for (let i = 0; i < maxAttempts; i++) {
    await sleep(5000)
    await fetchSuggestions()
    if (totalSuggestions.value > prevCount) {
      ElMessage.success('分析完成，已更新结果')
      return
    }
  }
  ElMessage.warning('分析仍在后台执行，请稍后刷新查看')
}

function changePage(page: number) {
  currentPage.value = page
  fetchSuggestions()
}

async function processSuggestion(id: number, status: string = 'processed') {
  processingIds.value.add(id)
  try {
    const token = localStorage.getItem('aios_token')
    const res = await fetch(`${API_BASE}/api/ai-advisor/process`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
      body: JSON.stringify({ suggestion_id: id, status })
    })
    const data = await res.json()
    if (data?.ok) {
      await fetchSuggestions()
    }
  } catch (e: any) {
    console.error('[AiAdvice] processSuggestion error:', e)
    ElMessage.error('操作失败: ' + e.message)
  }
  processingIds.value.delete(id)
}

async function triggerAnalyze() {
  analyzing.value = true
  try {
    const res = await fetch(`${API_BASE}/api/ai-advisor/trigger`, {
      method: 'POST',
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('分析已启动，正在等待结果…')
      await pollAfterTrigger()
    } else {
      ElMessage.error(data.error || '分析失败')
    }
  } catch (e: any) {
    console.error('[AiAdvice] triggerAnalyze error:', e)
    ElMessage.error('分析失败: ' + e.message)
  }
  analyzing.value = false
}

onMounted(() => {
  fetchSuggestions()
})
</script>

<template>
  <div class="ai-advice">
    <div class="page-header">
      <h2 class="page-title">AI 建议</h2>
      <div class="header-actions">
        <input type="date" class="date-picker" v-model="filterDate" @change="currentPage = 1; fetchSuggestions()" />
        <div class="status-tabs">
          <button class="status-tab" :class="{ active: filterStatus === 'all' }" @click="filterStatus = 'all'; currentPage = 1; fetchSuggestions()">全部</button>
          <button class="status-tab" :class="{ active: filterStatus === 'pending' }" @click="filterStatus = 'pending'; currentPage = 1; fetchSuggestions()">待处理</button>
          <button class="status-tab" :class="{ active: filterStatus === 'processed' }" @click="filterStatus = 'processed'; currentPage = 1; fetchSuggestions()">已处理</button>
          <button class="status-tab" :class="{ active: filterStatus === 'ignored' }" @click="filterStatus = 'ignored'; currentPage = 1; fetchSuggestions()">已忽略</button>
        </div>
        <button class="analyze-btn" :disabled="analyzing" @click="triggerAnalyze">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
          </svg>
          {{ analyzing ? '分析中...' : '分析今日对话' }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>加载中...</span>
    </div>

    <template v-else>
      <div v-if="filteredSuggestions.length === 0" class="empty-state">
        <svg class="empty-icon" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.4">
          <path d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
        </svg>
        <p class="empty-text">暂无优化建议，点击「分析今日对话」让 AI 分析当天对话记录，或等待每天凌晨自动分析</p>
      </div>

      <div v-else class="suggestion-list">
        <div v-for="item in filteredSuggestions" :key="item.id" class="suggestion-card">
          <div class="card-top">
            <span class="category-tag" :style="{ background: categoryColorMap[item.category] || '#909399' }">{{ categoryMap[item.category] || item.category }}</span>
            <span class="card-title">{{ item.title }}</span>
          </div>
          <div class="card-content-wrapper">
            <div class="card-content" :class="{ expanded: expandedIds.has(item.id) }">{{ item.content }}</div>
            <button v-if="item.content && item.content.length > 120" class="expand-btn" @click="toggleExpand(item.id)">{{ expandedIds.has(item.id) ? '收起' : '展开' }}</button>
          </div>
          <div class="card-footer">
            <div class="footer-left">
              <span class="priority-tag" :style="{ color: priorityColorMap[item.priority] || '#909399', borderColor: priorityColorMap[item.priority] || '#909399' }">{{ priorityLabelMap[item.priority] || item.priority }}</span>
              <span class="card-time">{{ formatTime(item.created_at) }}</span>
            </div>
            <div class="footer-right">
              <template v-if="item.status === 'pending'">
                <button class="process-btn" :disabled="processingIds.has(item.id)" @click="processSuggestion(item.id, 'processed')">{{ processingIds.has(item.id) ? '处理中...' : '标记已处理' }}</button>
                <button class="ignore-btn" :disabled="processingIds.has(item.id)" @click="processSuggestion(item.id, 'ignored')">{{ processingIds.has(item.id) ? '处理中...' : '忽略' }}</button>
              </template>
              <template v-else>
                <span v-if="item.status === 'processed'" class="processed-badge">已处理</span>
                <span v-else class="ignored-badge">{{ item.status === 'ignored' ? '已忽略' : item.status }}</span>
                <span v-if="item.processed_at" class="processed-time">{{ formatTime(item.processed_at) }}</span>
              </template>
            </div>
          </div>
        </div>

        <div v-if="totalSuggestions > pageSize" class="pagination">
          <button class="page-btn" :disabled="currentPage <= 1" @click="changePage(currentPage - 1)">上一页</button>
          <span class="page-info">第 {{ currentPage }} 页 / 共 {{ totalPages }} 页（{{ totalSuggestions }} 条）</span>
          <button class="page-btn" :disabled="currentPage >= totalPages" @click="changePage(currentPage + 1)">下一页</button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.ai-advice {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #1e293b;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.date-picker {
  padding: 5px 10px;
  font-size: 13px;
  color: #475569;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  outline: none;
  cursor: pointer;
  transition: border-color 0.2s;
}
.date-picker:hover { border-color: #cbd5e1; }
.date-picker:focus { border-color: #6366f1; }

.status-tabs {
  display: flex;
  background: #f1f5f9;
  border-radius: 8px;
  padding: 3px;
}

.status-tab {
  padding: 5px 16px;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.status-tab:hover { color: #334155; }
.status-tab.active { background: #fff; color: #6366f1; box-shadow: 0 1px 3px rgba(0,0,0,0.06); }

.analyze-btn {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 6px 14px;
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  background: #6366f1;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.analyze-btn:hover { background: #4f46e5; }
.analyze-btn:disabled { opacity: 0.6; cursor: not-allowed; }

.loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 60px 0;
  color: #94a3b8;
  font-size: 14px;
}

.spinner {
  width: 28px;
  height: 28px;
  border: 3px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 60px 0;
  gap: 12px;
}

.empty-icon { color: #94a3b8; }

.empty-text {
  font-size: 15px;
  font-weight: 500;
  color: #64748b;
}

.suggestion-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.suggestion-card {
  background: #fff;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.suggestion-card:hover {
  border-color: #cbd5e1;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.card-top {
  display: flex;
  align-items: center;
  gap: 10px;
}

.category-tag {
  display: inline-block;
  padding: 2px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #fff;
  border-radius: 4px;
  white-space: nowrap;
  flex-shrink: 0;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
  line-height: 1.4;
}

.card-content-wrapper {
  position: relative;
}

.card-content {
  font-size: 13px;
  color: #475569;
  line-height: 1.7;
  max-height: 66px;
  overflow: hidden;
  transition: max-height 0.3s ease;
  white-space: pre-line;
}
.card-content.expanded {
  max-height: 600px;
}

.expand-btn {
  margin-top: 4px;
  padding: 0;
  font-size: 12px;
  color: #6366f1;
  background: none;
  border: none;
  cursor: pointer;
  transition: color 0.2s;
}
.expand-btn:hover { color: #4f46e5; }

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 10px;
  border-top: 1px solid #f1f5f9;
}

.footer-left,
.footer-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.priority-tag {
  padding: 1px 8px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid;
  border-radius: 4px;
}

.card-time {
  font-size: 12px;
  color: #94a3b8;
}

.process-btn {
  padding: 4px 14px;
  font-size: 12px;
  font-weight: 500;
  color: #16a34a;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.process-btn:hover {
  background: #dcfce7;
  border-color: #16a34a;
}
.process-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ignore-btn {
  padding: 4px 14px;
  font-size: 12px;
  font-weight: 500;
  color: #94a3b8;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.ignore-btn:hover {
  background: #f1f5f9;
  border-color: #94a3b8;
}
.ignore-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.processed-badge {
  padding: 2px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #94a3b8;
  background: #f1f5f9;
  border-radius: 4px;
}

.ignored-badge {
  padding: 2px 10px;
  font-size: 12px;
  font-weight: 500;
  color: #94a3b8;
  background: #f1f5f9;
  border-radius: 4px;
}

.processed-time {
  font-size: 12px;
  color: #94a3b8;
}

.pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 16px 0 4px;
}

.page-btn {
  padding: 4px 14px;
  font-size: 13px;
  color: #475569;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.page-btn:hover:not(:disabled) { border-color: #6366f1; color: #6366f1; }
.page-btn:disabled { opacity: 0.4; cursor: not-allowed; }

.page-info {
  font-size: 13px;
  color: #94a3b8;
}

@media (max-width: 640px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }
  .header-actions {
    width: 100%;
    flex-wrap: wrap;
  }
  .suggestion-card {
    padding: 12px 14px;
  }
  .card-top {
    flex-wrap: wrap;
  }
}
</style>
