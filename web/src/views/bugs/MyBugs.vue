<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { API_BASE } from '../../api'

interface Bug {
  id: number
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

async function fetchBugs() {
  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/bugs/my`, {
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

async function withdrawBug(bug: Bug) {
  try {
    await ElMessageBox.confirm(`确定撤销 Bug「${bug.title}」吗？撤销后仍保留记录作为沟通证据。`, '确认撤销', {
      type: 'warning',
      confirmButtonText: '确定撤销',
      cancelButtonText: '取消'
    })
    const res = await fetch(`${API_BASE}/api/bugs/withdraw`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken()}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: bug.id })
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('Bug 已撤销')
      fetchBugs()
    } else {
      ElMessage.error(data?.error || '撤销失败')
    }
  } catch (e: any) {
    // 用户取消
  }
}

onMounted(() => {
  fetchBugs()
})
</script>

<template>
  <div class="my-bugs-page">
    <div class="page-header">
      <h2>我的 Bug</h2>
      <p class="page-desc">查看你反馈的 Bug 及其修复进度</p>
    </div>

    <el-table :data="bugs" v-loading="loading" stripe row-key="id" style="width: 100%">
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
            <div class="detail-empty" v-if="!row.scenario && !row.expected && !row.actual && !row.admin_note">
              暂无详细信息
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="title" label="Bug 标题" min-width="200" show-overflow-tooltip />

      <el-table-column prop="module" label="所属模块" width="140">
        <template #default="{ row }">
          <span>{{ row.module || '-' }}</span>
        </template>
      </el-table-column>

      <el-table-column prop="status" label="当前状态" width="120" align="center">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" size="small">
            {{ statusText(row.status) }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="created_at" label="提交时间" width="170" align="center">
        <template #default="{ row }">
          <span class="time-text">{{ formatTime(row.created_at) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="操作" width="100" align="center">
        <template #default="{ row }">
          <el-button
            v-if="row.status !== 'withdrawn' && row.status !== 'done'"
            size="small"
            type="warning"
            plain
            @click="withdrawBug(row)"
          >
            撤销
          </el-button>
        </template>
      </el-table-column>

      <template #empty>
        <el-empty description="还没有反馈过 Bug" />
      </template>
    </el-table>
  </div>
</template>

<style scoped>
.my-bugs-page {
  max-width: 960px;
}

.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  font-size: 18px;
  font-weight: 600;
  color: #1e293b;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

.expand-detail {
  padding: 12px 20px 12px 48px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.detail-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.detail-label {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: #ef4444;
  background: #fef2f2;
  padding: 2px 8px;
  border-radius: 4px;
  min-width: 70px;
  text-align: center;
}

.detail-text {
  font-size: 13px;
  color: #334155;
  line-height: 1.6;
  white-space: pre-wrap;
}

.detail-text.admin-note {
  color: #b45309;
  background: #fffbeb;
  padding: 6px 10px;
  border-radius: 6px;
  border: 1px solid #fde68a;
}

.detail-empty {
  font-size: 13px;
  color: #94a3b8;
}

.time-text {
  font-size: 13px;
  color: #64748b;
  font-variant-numeric: tabular-nums;
}
</style>
