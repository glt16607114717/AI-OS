<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { formatTimeShort as formatTime } from '../../utils/time'
import { marked } from 'marked'

marked.use({ breaks: true, gfm: true })

interface Diary {
  id: number
  user_id: number
  username: string
  report_date: string
  title: string
  content: string
  created_at: string
  updated_at: string
}

import { API_BASE } from '../../api'

const diaries = ref<Diary[]>([])
const loading = ref(false)
const isAdmin = ref(false)

// 筛选
const filterDate = ref('')

// 查看弹窗
const viewVisible = ref(false)
const viewingDiary = ref<Diary | null>(null)

const authHeaders = () => {
  const token = localStorage.getItem('aios_token') || ''
  return { 'Authorization': `Bearer ${token}` }
}

async function fetchDiaries() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (filterDate.value) params.set('date', filterDate.value)
    const qs = params.toString()
    const res = await fetch(`${API_BASE}/api/work-diary/list${qs ? '?' + qs : ''}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      diaries.value = data.data || []
    } else {
      ElMessage.error(data.error || '加载日报失败')
    }
  } catch (e: any) {
    console.error('[DailyReport] fetchDiaries error:', e)
    ElMessage.error('加载日报失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

async function fetchUserInfo() {
  try {
    const token = localStorage.getItem('aios_token') || ''
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    const data = await res.json()
    if (data?.ok && data?.data) {
      isAdmin.value = !!data.data.is_admin
    }
  } catch (e) {}
}

async function viewDiary(item: Diary) {
  try {
    const res = await fetch(`${API_BASE}/api/work-diary/get?id=${item.id}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      viewingDiary.value = data.data
      viewVisible.value = true
    }
  } catch (e: any) {
    ElMessage.error('加载详情失败: ' + e.message)
  }
}

function formatDate(iso: string): string {
  if (!iso) return ''
  return iso.split('T')[0]
}

function renderMarkdown(text: string): string {
  if (!text) return ''
  try {
    return marked.parse(text) as string
  } catch {
    return text.replace(/\n/g, '<br>')
  }
}

// 按日期分组
const groupedDiaries = computed(() => {
  const groups: Record<string, Diary[]> = {}
  for (const d of diaries.value) {
    if (!groups[d.report_date]) groups[d.report_date] = []
    groups[d.report_date].push(d)
  }
  return groups
})

onMounted(() => {
  fetchUserInfo()
  fetchDiaries()
})
</script>

<template>
  <div class="daily-report">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">工作日报</h2>
      <div class="header-actions">
        <input
          type="date"
          class="date-picker"
          v-model="filterDate"
          @change="fetchDiaries"
        />
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>加载中...</span>
    </div>

    <!-- Empty -->
    <div v-else-if="diaries.length === 0" class="empty-state">
      <svg class="empty-icon" width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.4">
        <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/>
      </svg>
      <p class="empty-text">暂无日报，由每日蒸馏自动生成</p>
    </div>

    <!-- 列表 -->
    <div v-else class="diary-list">
      <div v-for="(items, date) in groupedDiaries" :key="date" class="diary-group">
        <div class="group-header">
          <span class="group-date">{{ formatDate(date) }}</span>
          <span class="group-count">{{ items.length }}篇</span>
        </div>
        <div
          v-for="item in items"
          :key="item.id"
          class="diary-card"
          @click="viewDiary(item)"
        >
          <div class="card-left">
            <div class="card-title">{{ item.title || '工作日报' }}</div>
            <div class="card-meta">
              <span v-if="isAdmin" class="card-author">{{ item.username }}</span>
              <span class="card-time">{{ formatDate(item.updated_at) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 查看弹窗 -->
    <div v-if="viewVisible" class="dialog-overlay" @click.self="viewVisible = false">
      <div class="dialog">
        <div class="dialog-header">
          <span class="dialog-title">{{ viewingDiary?.title || '工作日报' }}</span>
          <button class="dialog-close" @click="viewVisible = false">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
        <div class="dialog-body">
          <div class="diary-meta">
            <span v-if="isAdmin && viewingDiary?.username" class="diary-author">{{ viewingDiary.username }}</span>
            <span class="diary-date">{{ formatDate(viewingDiary?.report_date || '') }}</span>
          </div>
          <div class="diary-content" v-html="renderMarkdown(viewingDiary?.content || '')"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.daily-report {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 24px;
  gap: 16px;
  overflow: auto;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-shrink: 0;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
  margin: 0;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.date-picker {
  padding: 6px 12px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
}

.primary-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  background: #409eff;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.2s;
}
.primary-btn:hover { background: #66b1ff; }
.primary-btn:disabled { background: #a0cfff; cursor: not-allowed; }

.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #909399;
  padding: 40px;
}
.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid #e4e7ed;
  border-top-color: #409eff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 60px 40px;
  color: #909399;
}
.empty-text { font-size: 14px; text-align: center; }

.diary-list {
  display: flex;
  flex-direction: column;
  gap: 20px;
  overflow: auto;
}

.diary-group { display: flex; flex-direction: column; gap: 8px; }

.group-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.group-date {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}
.group-count {
  font-size: 12px;
  color: #909399;
  background: #f4f4f5;
  padding: 2px 8px;
  border-radius: 10px;
}

.diary-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  cursor: pointer;
  transition: box-shadow 0.2s, border-color 0.2s;
}
.diary-card:hover {
  box-shadow: 0 2px 12px rgba(0,0,0,0.08);
  border-color: #409eff;
}

.card-left { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.card-title {
  font-size: 14px;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.card-meta { display: flex; gap: 12px; font-size: 12px; color: #909399; }
.card-author { color: #409eff; }

.card-right { display: flex; gap: 4px; flex-shrink: 0; }
.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
  color: #909399;
  transition: background 0.2s, color 0.2s;
}
.delete-btn:hover { background: #fef0f0; color: #f56c6c; }

/* 弹窗 */
.dialog-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.dialog { width: 1200px; max-width: 90vw; background: #fff; border-radius: 10px; display: flex; flex-direction: column; box-shadow: 0 8px 32px rgba(0,0,0,0.15); max-height: 85vh; }
.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #ebeef5;
}
.dialog-title { font-size: 15px; font-weight: 600; color: #303133; }
.dialog-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: #909399;
  cursor: pointer;
  border-radius: 4px;
}
.dialog-close:hover { background: #f4f4f5; color: #303133; }

.dialog-body {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow: auto;
  flex: 1;
}
.form-row { display: flex; align-items: center; gap: 12px; }
.form-row-grow { flex: 1; align-items: flex-start; }
.form-label { width: 50px; font-size: 13px; color: #606266; flex-shrink: 0; }
.form-input {
  flex: 1;
  padding: 8px 12px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
}
.form-textarea {
  flex: 1;
  width: 100%;
  min-height: 200px;
  padding: 8px 12px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  font-size: 13px;
  font-family: inherit;
  resize: vertical;
  outline: none;
}
.form-input:focus, .form-textarea:focus { border-color: #409eff; }

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 20px;
  border-top: 1px solid #ebeef5;
}
.cancel-btn {
  padding: 8px 16px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  font-size: 13px;
  background: #fff;
  color: #606266;
  cursor: pointer;
}
.cancel-btn:hover { background: #f4f4f5; }

.dialog-view { }
.diary-meta { display: flex; gap: 12px; font-size: 13px; color: #909399; margin-bottom: 8px; }
.diary-author { color: #409eff; }
.diary-content { font-size: 14px; line-height: 1.8; color: #303133; word-break: break-word; }
.diary-content :deep(strong) { color: #303133; font-weight: 600; }
.diary-content :deep(h1), .diary-content :deep(h2), .diary-content :deep(h3) { margin: 12px 0 6px; color: #303133; }
.diary-content :deep(p) { margin: 6px 0; }
.diary-content :deep(ul), .diary-content :deep(ol) { margin: 6px 0; padding-left: 24px; }
.diary-content :deep(li) { margin: 3px 0; }
.diary-content :deep(code) { background: #f4f4f5; padding: 2px 4px; border-radius: 3px; font-size: 13px; }
</style>
