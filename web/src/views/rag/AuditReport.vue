<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { marked } from 'marked'
import { API_BASE } from '../../api'

marked.use({ breaks: true, gfm: true })

// ── 类型定义 ──
interface AuditReport {
  id: number
  report_date: string
  overall_score: number
  score_level: string // green|yellow|red
  status: string // completed|failed|running
  error_msg: string
  llm_model: string
  llm_tokens_used: number
  llm_api_url: string
  created_at: string
}

interface AuditReportDetail extends AuditReport {
  metrics_retrieve: any
  metrics_store: any
  report_content: string
  problem_cases: ProblemCase[]
}

interface ProblemCase {
  log_id: number
  knowledge_id: number
  type: string
  reason: string
  suggested_action: string
}

interface PendingArchive {
  id: number
  knowledge_id: number
  audit_report_id: number
  reason: string
  suggested_action: string
  status: string // pending|approved|rejected|done
  reviewed_by: string
  reviewed_at: string
  created_at: string
  knowledge_title: string
  knowledge_category: string
  knowledge_project: string
  knowledge_priority: string
}

// ── 状态 ──
const reports = ref<AuditReport[]>([])
const loading = ref(false)
const isAdmin = ref(false)
const totalCount = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)

// 日期筛选
const startDate = ref('')
const endDate = ref('')

// 详情弹窗
const detailVisible = ref(false)
const detail = ref<AuditReportDetail | null>(null)
const detailLoading = ref(false)

// 待归档列表
const pendingArchives = ref<PendingArchive[]>([])
const pendingTotal = ref(0)
const pendingStatus = ref('pending')
const pendingLoading = ref(false)

// 趋势图
const trendChart = ref<HTMLElement | null>(null)

// ── 工具函数 ──
const authHeaders = () => {
  const token = localStorage.getItem('aios_token') || ''
  return { 'Authorization': `Bearer ${token}` }
}

const levelText = (level: string): string => {
  if (level === 'green') return '健康'
  if (level === 'yellow') return '关注'
  if (level === 'red') return '严重'
  return level
}

const levelColor = (level: string): string => {
  if (level === 'green') return '#52c41a'
  if (level === 'yellow') return '#faad14'
  if (level === 'red') return '#f5222d'
  return '#999'
}

const statusText = (status: string): string => {
  const map: Record<string, string> = {
    completed: '已完成',
    failed: '失败',
    running: '执行中',
    pending: '待审核',
    approved: '已通过',
    rejected: '已驳回',
    done: '已处理',
  }
  return map[status] || status
}

const actionText = (action: string): string => {
  const map: Record<string, string> = {
    archive: '归档',
    fix_category: '修正分类',
    fix_project: '修正项目',
  }
  return map[action] || action
}

function formatDate(iso: string): string {
  if (!iso) return ''
  return iso.split('T')[0]
}

function renderMarkdown(text: string): string {
  if (!text) return '<p style="color:#999">暂无报告内容</p>'
  try {
    return marked.parse(text) as string
  } catch {
    return text.replace(/\n/g, '<br>')
  }
}

// 检查 URL 是否走 coding plan（关键约束验证）
const isCodingPlan = (url: string): boolean => {
  return url.includes('/coding/')
}

// ── API 调用 ──
async function fetchUserInfo() {
  try {
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${localStorage.getItem('aios_token') || ''}` }
    })
    const data = await res.json()
    if (data?.ok && data?.data) {
      isAdmin.value = !!data.data.is_admin
    }
  } catch (e) {}
}

async function fetchReports() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    params.set('page', String(currentPage.value))
    params.set('page_size', String(pageSize.value))
    if (startDate.value) params.set('start_date', startDate.value)
    if (endDate.value) params.set('end_date', endDate.value)

    const res = await fetch(`${API_BASE}/api/audit/reports?${params}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      reports.value = data.data.list || []
      totalCount.value = data.data.total || 0
    } else {
      ElMessage.error(data.error || '加载报告失败')
    }
  } catch (e: any) {
    ElMessage.error('加载报告失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

async function viewReport(item: AuditReport) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const res = await fetch(`${API_BASE}/api/audit/reports/${item.id}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      detail.value = data.data
    } else {
      ElMessage.error(data.error || '加载详情失败')
    }
  } catch (e: any) {
    ElMessage.error('加载详情失败: ' + e.message)
  } finally {
    detailLoading.value = false
  }
}

async function runAuditManually() {
  try {
    await ElMessageBox.confirm(
      '手动触发巡检会立即评审昨日的知识库数据（约 1-2 分钟），确认继续？',
      '确认',
      { type: 'info' }
    )
  } catch {
    return
  }

  try {
    const res = await fetch(`${API_BASE}/api/audit/run`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({})
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('巡检任务已触发，1-2 分钟后刷新查看')
      setTimeout(() => fetchReports(), 60000)
    } else {
      ElMessage.error(data.error || '触发失败')
    }
  } catch (e: any) {
    ElMessage.error('触发失败: ' + e.message)
  }
}

async function fetchPendingArchives() {
  pendingLoading.value = true
  try {
    const params = new URLSearchParams()
    params.set('status', pendingStatus.value)
    params.set('page', '1')
    params.set('page_size', '50')

    const res = await fetch(`${API_BASE}/api/audit/pending-archives?${params}`, {
      headers: authHeaders()
    })
    const data = await res.json()
    if (data?.ok) {
      pendingArchives.value = data.data.list || []
      pendingTotal.value = data.data.total || 0
    }
  } catch (e: any) {
    ElMessage.error('加载待归档失败: ' + e.message)
  } finally {
    pendingLoading.value = false
  }
}

async function approveArchive(item: PendingArchive) {
  try {
    await ElMessageBox.confirm(
      `确认归档知识 id=${item.knowledge_id} "${item.knowledge_title}"？\n归档后将自动软删除并清除 Qdrant 向量。`,
      '确认归档',
      { type: 'warning' }
    )
  } catch {
    return
  }

  try {
    const res = await fetch(`${API_BASE}/api/audit/pending-archives/${item.id}/approve`, {
      method: 'POST',
      headers: { ...authHeaders() }
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('已归档')
      fetchPendingArchives()
    } else {
      ElMessage.error(data.error || '归档失败')
    }
  } catch (e: any) {
    ElMessage.error('归档失败: ' + e.message)
  }
}

async function rejectArchive(item: PendingArchive) {
  try {
    await ElMessageBox.confirm(`确认驳回这条建议？`, '确认驳回', { type: 'info' })
  } catch {
    return
  }

  try {
    const res = await fetch(`${API_BASE}/api/audit/pending-archives/${item.id}/reject`, {
      method: 'POST',
      headers: { ...authHeaders() }
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('已驳回')
      fetchPendingArchives()
    } else {
      ElMessage.error(data.error || '驳回失败')
    }
  } catch (e: any) {
    ElMessage.error('驳回失败: ' + e.message)
  }
}

// ── 生命周期 ──
onMounted(() => {
  fetchUserInfo()
  fetchReports()
  fetchPendingArchives()
})
</script>

<template>
  <div class="audit-report">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">知识库巡检报告</h2>
      <div class="header-actions">
        <input type="date" class="date-picker" v-model="startDate" @change="fetchReports" placeholder="开始日期" />
        <span class="date-sep">至</span>
        <input type="date" class="date-picker" v-model="endDate" @change="fetchReports" placeholder="结束日期" />
        <button v-if="isAdmin" class="btn btn-primary" @click="runAuditManually">手动触发巡检</button>
      </div>
    </div>

    <!-- 报告列表 -->
    <div class="section">
      <div class="section-title">📋 巡检报告</div>
      <div v-if="loading" class="loading-state">加载中...</div>
      <div v-else-if="reports.length === 0" class="empty-state">暂无报告，每天早上 8 点自动生成</div>
      <div v-else class="report-list">
        <div v-for="r in reports" :key="r.id" class="report-card" @click="viewReport(r)">
          <div class="card-left">
            <div class="card-date">{{ r.report_date }}</div>
            <div class="card-meta">
              <span v-if="r.status === 'completed'" class="score-badge" :style="{ background: levelColor(r.score_level) }">
                {{ r.overall_score }}分 {{ levelText(r.score_level) }}
              </span>
              <span v-else class="status-badge" :class="r.status">{{ statusText(r.status) }}</span>
              <span class="card-tokens">{{ r.llm_tokens_used }} tokens</span>
            </div>
          </div>
          <div class="card-right">
            <span v-if="r.llm_api_url && !isCodingPlan(r.llm_api_url)" class="warn-badge">⚠️ 非coding地址</span>
            <span class="card-arrow">→</span>
          </div>
        </div>
        <div class="pagination" v-if="totalCount > pageSize">
          <button class="btn" :disabled="currentPage <= 1" @click="currentPage--; fetchReports()">上一页</button>
          <span>{{ currentPage }} / {{ Math.ceil(totalCount / pageSize) }}</span>
          <button class="btn" :disabled="currentPage * pageSize >= totalCount" @click="currentPage++; fetchReports()">下一页</button>
        </div>
      </div>
    </div>

    <!-- 待归档列表 -->
    <div class="section">
      <div class="section-title">
        ⚠️ 待归档审核
        <span class="section-count" v-if="pendingTotal > 0">{{ pendingTotal }}</span>
      </div>
      <div class="pending-filter">
        <select v-model="pendingStatus" @change="fetchPendingArchives">
          <option value="pending">待审核</option>
          <option value="done">已处理</option>
          <option value="rejected">已驳回</option>
          <option value="">全部</option>
        </select>
      </div>
      <div v-if="pendingLoading" class="loading-state">加载中...</div>
      <div v-else-if="pendingArchives.length === 0" class="empty-state">暂无待归档</div>
      <div v-else class="pending-list">
        <div v-for="p in pendingArchives" :key="p.id" class="pending-card">
          <div class="pending-main">
            <div class="pending-title">{{ p.knowledge_title || '(无标题)' }}</div>
            <div class="pending-meta">
              <span class="meta-item">ID: {{ p.knowledge_id }}</span>
              <span class="meta-item">分类: {{ p.knowledge_category }}</span>
              <span class="meta-item">项目: {{ p.knowledge_project }}</span>
              <span class="meta-item action">{{ actionText(p.suggested_action) }}</span>
            </div>
            <div class="pending-reason">{{ p.reason }}</div>
          </div>
          <div class="pending-actions" v-if="p.status === 'pending'">
            <button class="btn btn-danger btn-sm" @click="approveArchive(p)">通过归档</button>
            <button class="btn btn-sm" @click="rejectArchive(p)">驳回</button>
          </div>
          <div class="pending-status" v-else>
            <span class="status-tag" :class="p.status">{{ statusText(p.status) }}</span>
            <span class="reviewer" v-if="p.reviewed_by">{{ p.reviewed_by }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 详情弹窗 -->
    <div v-if="detailVisible" class="dialog-overlay" @click.self="detailVisible = false">
      <div class="dialog">
        <div class="dialog-header">
          <span class="dialog-title">
            {{ detail?.report_date }} 巡检报告
            <span v-if="detail" class="score-badge" :style="{ background: levelColor(detail.score_level) }">
              {{ detail.overall_score }}分 {{ levelText(detail.score_level) }}
            </span>
          </span>
          <button class="dialog-close" @click="detailVisible = false">✕</button>
        </div>
        <div class="dialog-body">
          <div v-if="detailLoading" class="loading-state">加载中...</div>
          <template v-else-if="detail">
            <!-- 元数据 -->
            <div class="detail-meta">
              <span>模型: {{ detail.llm_model || '-' }}</span>
              <span>消耗: {{ detail.llm_tokens_used }} tokens</span>
              <span :class="{ 'warn-text': !isCodingPlan(detail.llm_api_url) }">
                {{ isCodingPlan(detail.llm_api_url) ? '✓ coding plan' : '⚠️ 非coding地址' }}
              </span>
              <span class="error-text" v-if="detail.error_msg">错误: {{ detail.error_msg }}</span>
            </div>

            <!-- 报告正文 -->
            <div class="report-content" v-html="renderMarkdown(detail.report_content)"></div>

            <!-- 问题案例 -->
            <div v-if="detail.problem_cases && detail.problem_cases.length > 0" class="problem-cases">
              <h4>问题案例（{{ detail.problem_cases.length }} 条）</h4>
              <div v-for="(pc, idx) in detail.problem_cases" :key="idx" class="problem-case">
                <div class="case-header">
                  <span class="case-type" :class="pc.type">{{ pc.type === 'retrieve' ? '检索' : '入库' }}</span>
                  <span class="case-action">{{ actionText(pc.suggested_action) }}</span>
                  <span class="case-id">log_id={{ pc.log_id }} knowledge_id={{ pc.knowledge_id }}</span>
                </div>
                <div class="case-reason">{{ pc.reason }}</div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.audit-report {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 20px;
  overflow-y: auto;
  background: #f5f7fa;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #1f2937;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.date-picker {
  padding: 6px 10px;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  font-size: 13px;
}

.date-sep {
  color: #999;
  font-size: 13px;
}

.section {
  background: #fff;
  border-radius: 8px;
  padding: 16px 20px;
  margin-bottom: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
}

.section-title {
  font-size: 15px;
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-count {
  background: #f5222d;
  color: #fff;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: normal;
}

.loading-state, .empty-state {
  text-align: center;
  color: #999;
  padding: 40px 0;
  font-size: 14px;
}

.report-list, .pending-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.report-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.report-card:hover {
  border-color: #4096ff;
  background: #f0f7ff;
}

.card-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.card-date {
  font-size: 14px;
  font-weight: 600;
  color: #1f2937;
  min-width: 100px;
}

.card-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.score-badge {
  color: #fff;
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 600;
}

.status-badge {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 10px;
  background: #f0f0f0;
  color: #666;
}

.status-badge.failed {
  background: #fff1f0;
  color: #f5222d;
}

.status-badge.running {
  background: #e6f7ff;
  color: #1890ff;
}

.card-tokens {
  font-size: 12px;
  color: #999;
}

.card-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.warn-badge {
  background: #fff7e6;
  color: #fa8c16;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 4px;
}

.card-arrow {
  color: #ccc;
}

.btn {
  padding: 6px 14px;
  border: 1px solid #d9d9d9;
  background: #fff;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
}

.btn:hover {
  border-color: #4096ff;
  color: #4096ff;
}

.btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.btn-primary {
  background: #1890ff;
  border-color: #1890ff;
  color: #fff;
}

.btn-primary:hover {
  background: #40a9ff;
  border-color: #40a9ff;
  color: #fff;
}

.btn-danger {
  background: #ff4d4f;
  border-color: #ff4d4f;
  color: #fff;
}

.btn-danger:hover {
  background: #ff7875;
  border-color: #ff7875;
  color: #fff;
}

.btn-sm {
  padding: 4px 10px;
  font-size: 12px;
}

.pending-filter {
  margin-bottom: 12px;
}

.pending-filter select {
  padding: 4px 8px;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  font-size: 13px;
}

.pending-card {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 12px 16px;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.pending-main {
  flex: 1;
}

.pending-title {
  font-size: 14px;
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 4px;
}

.pending-meta {
  display: flex;
  gap: 12px;
  font-size: 12px;
  color: #666;
  margin-bottom: 6px;
}

.meta-item.action {
  color: #fa8c16;
  font-weight: 600;
}

.pending-reason {
  font-size: 13px;
  color: #555;
  background: #fafafa;
  padding: 6px 10px;
  border-radius: 4px;
}

.pending-actions {
  display: flex;
  gap: 8px;
  margin-left: 16px;
}

.pending-status {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.status-tag {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: 10px;
  background: #f0f0f0;
  color: #666;
}

.status-tag.done {
  background: #f6ffed;
  color: #52c41a;
}

.status-tag.rejected {
  background: #fff1f0;
  color: #f5222d;
}

.reviewer {
  font-size: 11px;
  color: #999;
}

.dialog-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 1000;
}

.dialog {
  background: #fff;
  border-radius: 8px;
  width: 80%;
  max-width: 900px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #f0f0f0;
}

.dialog-title {
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 12px;
}

.dialog-close {
  border: none;
  background: none;
  font-size: 18px;
  cursor: pointer;
  color: #999;
}

.dialog-body {
  padding: 20px;
  overflow-y: auto;
  flex: 1;
}

.detail-meta {
  display: flex;
  gap: 16px;
  font-size: 12px;
  color: #666;
  padding-bottom: 12px;
  margin-bottom: 16px;
  border-bottom: 1px dashed #f0f0f0;
  flex-wrap: wrap;
}

.warn-text {
  color: #fa8c16;
  font-weight: 600;
}

.error-text {
  color: #f5222d;
}

.report-content {
  font-size: 14px;
  line-height: 1.7;
  color: #1f2937;
}

.report-content :deep(h1),
.report-content :deep(h2),
.report-content :deep(h3) {
  margin-top: 16px;
  margin-bottom: 8px;
}

.report-content :deep(ul),
.report-content :deep(ol) {
  padding-left: 20px;
}

.problem-cases {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 2px solid #f0f0f0;
}

.problem-cases h4 {
  color: #f5222d;
  margin-bottom: 12px;
}

.problem-case {
  padding: 10px 12px;
  background: #fff7e6;
  border-radius: 4px;
  margin-bottom: 8px;
  border-left: 3px solid #fa8c16;
}

.case-header {
  display: flex;
  gap: 8px;
  font-size: 12px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.case-type {
  padding: 1px 6px;
  border-radius: 3px;
  font-weight: 600;
}

.case-type.retrieve {
  background: #e6f7ff;
  color: #1890ff;
}

.case-type.store {
  background: #f6ffed;
  color: #52c41a;
}

.case-action {
  color: #fa8c16;
  font-weight: 600;
}

.case-id {
  color: #999;
}

.case-reason {
  font-size: 13px;
  color: #555;
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 12px;
  margin-top: 12px;
  font-size: 13px;
  color: #666;
}
</style>
