<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

interface Requirement {
  id: number
  title: string
  scenario: string
  pain_point: string
  module: string
  status: string
  admin_note: string
  created_at: string
  updated_at: string
}

const requirements = ref<Requirement[]>([])
const loading = ref(false)

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

function statusText(status: string): string {
  const map: Record<string, string> = {
    submitted: '已提交',
    reviewing: '审核中',
    accepted: '已采纳',
    scheduled: '已排期',
    in_progress: '开发中',
    done: '已完成',
    rejected: '已拒绝',
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

async function fetchRequirements() {
  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/requirements/my`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data?.ok) {
      requirements.value = data.data?.requirements || data.data || []
    } else {
      ElMessage.error(data?.error || '获取需求列表失败')
    }
  } catch (e: any) {
    ElMessage.error('获取需求列表失败: ' + e.message)
  }
  loading.value = false
}

onMounted(() => {
  fetchRequirements()
})
</script>

<template>
  <div class="my-requirements-page">
    <div class="page-header">
      <h2>我的需求</h2>
      <p class="page-desc">查看你提交的需求及其处理进度</p>
    </div>

    <el-table
      :data="requirements"
      v-loading="loading"
      stripe
      row-key="id"
      style="width: 100%"
    >
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="expand-detail">
            <div class="detail-item" v-if="row.scenario">
              <span class="detail-label">使用场景</span>
              <span class="detail-text">{{ row.scenario }}</span>
            </div>
            <div class="detail-item" v-if="row.pain_point">
              <span class="detail-label">痛点描述</span>
              <span class="detail-text">{{ row.pain_point }}</span>
            </div>
            <div class="detail-item" v-if="row.admin_note">
              <span class="detail-label">管理员备注</span>
              <span class="detail-text admin-note">{{ row.admin_note }}</span>
            </div>
            <div class="detail-empty" v-if="!row.scenario && !row.pain_point && !row.admin_note">
              暂无详细信息
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="title" label="需求标题" min-width="200" show-overflow-tooltip />

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

      <template #empty>
        <el-empty description="还没有提交过需求" />
      </template>
    </el-table>
  </div>
</template>

<style scoped>
.my-requirements-page {
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

/* Expand detail */
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
  color: #6366f1;
  background: #eef2ff;
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
