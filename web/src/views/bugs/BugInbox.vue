<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

interface Bug {
  id: number
  user_id: number
  username: string
  title: string
  scenario: string
  expected: string
  actual: string
  module: string
  status: string
  admin_note: string
  created_at: string
  updated_at: string
}

const bugs = ref<Bug[]>([])
const loading = ref(false)
const activeStatus = ref('')

const statusTabs = [
  { value: '', label: '全部' },
  { value: 'submitted', label: '已提交' },
  { value: 'reviewing', label: '审核中' },
  { value: 'accepted', label: '已确认' },
  { value: 'scheduled', label: '已排期' },
  { value: 'in_progress', label: '修复中' },
  { value: 'done', label: '已修复' },
  { value: 'rejected', label: '已拒绝' },
  { value: 'withdrawn', label: '已撤销' },
]

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    submitted: '已提交',
    reviewing: '审核中',
    accepted: '已确认',
    scheduled: '已排期',
    in_progress: '修复中',
    done: '已修复',
    rejected: '已拒绝',
    withdrawn: '已撤销',
  }
  return map[status] || status
}

function statusType(status: string): string {
  const map: Record<string, string> = {
    submitted: 'info',
    reviewing: 'warning',
    accepted: 'success',
    scheduled: '',
    in_progress: 'warning',
    done: 'success',
    rejected: 'danger',
    withdrawn: 'info',
  }
  return map[status] || 'info'
}

function formatTime(time: string): string {
  if (!time) return '-'
  const d = new Date(time)
  if (isNaN(d.getTime())) return time
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${m}-${day} ${h}:${min}`
}

const filteredBugs = computed(() => {
  if (!activeStatus.value) return bugs.value
  return bugs.value.filter(b => b.status === activeStatus.value)
})

async function fetchBugs() {
  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/bugs/all`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data?.ok) {
      bugs.value = data.data || []
    } else {
      ElMessage.error(data?.error || '获取 Bug 列表失败')
    }
  } catch (e: any) {
    ElMessage.error('获取 Bug 列表失败: ' + e.message)
  }
  loading.value = false
}

// 审核弹窗
const reviewDialog = ref(false)
const editingBug = ref<Bug | null>(null)
const newStatus = ref('')
const adminNote = ref('')

const statusOptions = [
  { value: 'reviewing', label: '审核中' },
  { value: 'accepted', label: '已确认' },
  { value: 'scheduled', label: '已排期' },
  { value: 'in_progress', label: '修复中' },
  { value: 'done', label: '已修复' },
  { value: 'rejected', label: '已拒绝' },
]

function openReview(bug: Bug) {
  editingBug.value = bug
  newStatus.value = ''
  adminNote.value = bug.admin_note || ''
  reviewDialog.value = true
}

async function saveReview() {
  if (!editingBug.value || !newStatus.value) {
    ElMessage.warning('请选择新状态')
    return
  }
  try {
    const res = await fetch(`${API_BASE}/api/bugs/update-status`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken()}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: editingBug.value.id, status: newStatus.value, admin_note: adminNote.value })
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('状态已更新')
      reviewDialog.value = false
      fetchBugs()
    } else {
      ElMessage.error(data?.error || '更新失败')
    }
  } catch (e: any) {
    ElMessage.error('更新失败: ' + e.message)
  }
}

onMounted(() => {
  fetchBugs()
})
</script>

<template>
  <div class="bug-inbox-page">
    <div class="page-header">
      <h2>Bug 收件箱</h2>
      <p class="page-desc">查看和处理所有用户反馈的 Bug</p>
    </div>

    <div class="status-tabs">
      <el-tag
        v-for="tab in statusTabs"
        :key="tab.value"
        :type="activeStatus === tab.value ? 'primary' : 'info'"
        :effect="activeStatus === tab.value ? 'dark' : 'plain'"
        class="status-tab"
        @click="activeStatus = tab.value"
      >
        {{ tab.label }}
      </el-tag>
    </div>

    <el-table :data="filteredBugs" v-loading="loading" stripe row-key="id" style="width: 100%">
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="expand-detail">
            <div class="detail-item" v-if="row.scenario">
              <span class="detail-label">触发场景</span>
              <span class="detail-text">{{ row.scenario }}</span>
            </div>
            <div class="detail-item" v-if="row.expected">
              <span class="detail-label">预期结果</span>
              <span class="detail-text">{{ row.expected }}</span>
            </div>
            <div class="detail-item" v-if="row.actual">
              <span class="detail-label">实际结果</span>
              <span class="detail-text">{{ row.actual }}</span>
            </div>
            <div class="detail-item" v-if="row.admin_note">
              <span class="detail-label">管理员备注</span>
              <span class="detail-text admin-note">{{ row.admin_note }}</span>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="id" label="ID" width="70" align="center" />
      <el-table-column prop="username" label="提交人" width="120" />
      <el-table-column prop="title" label="Bug 标题" min-width="200" show-overflow-tooltip />
      <el-table-column prop="module" label="模块" width="140">
        <template #default="{ row }"><span>{{ row.module || '-' }}</span></template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="110" align="center">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="提交时间" width="170" align="center">
        <template #default="{ row }"><span class="time-text">{{ formatTime(row.created_at) }}</span></template>
      </el-table-column>
      <el-table-column label="操作" width="100" align="center">
        <template #default="{ row }">
          <el-button
            v-if="row.status !== 'withdrawn' && row.status !== 'done' && row.status !== 'rejected'"
            size="small"
            type="primary"
            plain
            @click="openReview(row)"
          >处理</el-button>
        </template>
      </el-table-column>

      <template #empty><el-empty description="暂无 Bug 记录" /></template>
    </el-table>

    <el-dialog v-model="reviewDialog" title="处理 Bug" width="500px">
      <div v-if="editingBug" class="review-form">
        <div class="review-item">
          <label>Bug 标题</label>
          <span>{{ editingBug.title }}</span>
        </div>
        <div class="review-item">
          <label>新状态</label>
          <el-select v-model="newStatus" placeholder="选择新状态" style="width: 100%">
            <el-option v-for="opt in statusOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </div>
        <div class="review-item">
          <label>管理员备注</label>
          <el-input v-model="adminNote" type="textarea" :rows="3" placeholder="处理说明（可选）" />
        </div>
      </div>
      <template #footer>
        <el-button @click="reviewDialog = false">取消</el-button>
        <el-button type="primary" @click="saveReview">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.bug-inbox-page { max-width: 1200px; }
.page-header { margin-bottom: 20px; }
.page-header h2 { font-size: 18px; font-weight: 600; color: #1e293b; margin: 0 0 4px; }
.page-desc { font-size: 13px; color: #64748b; margin: 0; }
.status-tabs { margin-bottom: 16px; display: flex; gap: 8px; flex-wrap: wrap; }
.status-tab { cursor: pointer; }
.expand-detail { padding: 12px 20px 12px 48px; display: flex; flex-direction: column; gap: 10px; }
.detail-item { display: flex; gap: 12px; align-items: flex-start; }
.detail-label { flex-shrink: 0; font-size: 12px; font-weight: 600; color: #ef4444; background: #fef2f2; padding: 2px 8px; border-radius: 4px; min-width: 70px; text-align: center; }
.detail-text { font-size: 13px; color: #334155; line-height: 1.6; white-space: pre-wrap; }
.detail-text.admin-note { color: #b45309; background: #fffbeb; padding: 6px 10px; border-radius: 6px; border: 1px solid #fde68a; }
.time-text { font-size: 13px; color: #64748b; font-variant-numeric: tabular-nums; }
.review-form { display: flex; flex-direction: column; gap: 16px; }
.review-item { display: flex; flex-direction: column; gap: 6px; }
.review-item label { font-size: 13px; font-weight: 600; color: #64748b; }
.review-item span { font-size: 14px; color: #1e293b; }
</style>
