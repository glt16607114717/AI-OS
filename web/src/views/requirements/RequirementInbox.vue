<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { API_BASE } from '../../api'
import { ElMessage } from 'element-plus'

interface Requirement {
  id: number
  submitter: string
  title: string
  module: string
  status: string
  scenario: string
  pain_point: string
  raw_conversation: string
  admin_note: string
  created_at: string
}

const loading = ref(false)
const tableData = ref<Requirement[]>([])
const activeStatus = ref<string>('')

const dialogVisible = ref(false)
const dialogLoading = ref(false)
const currentRow = ref<Requirement | null>(null)
const newStatus = ref<string>('')
const adminNote = ref<string>('')

const statusTabs = [
  { value: '', label: '全部' },
  { value: 'submitted', label: '已提交' },
  { value: 'reviewing', label: '审核中' },
  { value: 'accepted', label: '已采纳' },
  { value: 'scheduled', label: '已排期' },
  { value: 'in_progress', label: '开发中' },
  { value: 'done', label: '已完成' },
  { value: 'rejected', label: '已拒绝' },
]

const statusOptions = [
  { value: 'reviewing', label: '审核中' },
  { value: 'accepted', label: '已采纳' },
  { value: 'scheduled', label: '已排期' },
  { value: 'in_progress', label: '开发中' },
  { value: 'done', label: '已完成' },
  { value: 'rejected', label: '已拒绝' },
]

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

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

async function fetchRequirements() {
  loading.value = true
  try {
    const url = activeStatus.value
      ? `${API_BASE}/api/requirements/all?status=${activeStatus.value}`
      : `${API_BASE}/api/requirements/all`
    const res = await fetch(url, {
      headers: { Authorization: `Bearer ${getToken()}` },
    })
    const data = await res.json()
    if (data?.ok && Array.isArray(data.data)) {
      tableData.value = data.data
    } else {
      tableData.value = []
      if (data?.error) {
        ElMessage.error(data.error)
      }
    }
  } catch (e: any) {
    console.error('[RequirementInbox] fetchRequirements error:', e)
    ElMessage.error('获取需求列表失败')
  } finally {
    loading.value = false
  }
}

function handleTabChange(status: string) {
  activeStatus.value = status
  fetchRequirements()
}

function openStatusDialog(row: Requirement) {
  currentRow.value = row
  newStatus.value = row.status
  adminNote.value = row.admin_note || ''
  dialogVisible.value = true
}

async function submitStatusUpdate() {
  if (!currentRow.value) return
  if (!newStatus.value) {
    ElMessage.warning('请选择新状态')
    return
  }
  dialogLoading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/requirements/update-status`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${getToken()}`,
      },
      body: JSON.stringify({
        id: currentRow.value.id,
        status: newStatus.value,
        admin_note: adminNote.value,
      }),
    })
    const data = await res.json()
    if (data?.ok) {
      ElMessage.success('状态更新成功')
      dialogVisible.value = false
      fetchRequirements()
    } else {
      ElMessage.error(data?.error || '更新失败')
    }
  } catch (e: any) {
    console.error('[RequirementInbox] updateStatus error:', e)
    ElMessage.error('更新状态失败')
  } finally {
    dialogLoading.value = false
  }
}

onMounted(() => {
  fetchRequirements()
})
</script>

<template>
  <div class="requirement-inbox">
    <!-- 标题 -->
    <div class="page-header">
      <h2>需求收件箱</h2>
      <p class="subtitle">审核和管理业务人员提交的需求</p>
    </div>

    <!-- 状态过滤标签 -->
    <div class="status-tabs">
      <button
        v-for="tab in statusTabs"
        :key="tab.value"
        class="status-tab"
        :class="{ active: activeStatus === tab.value }"
        @click="handleTabChange(tab.value)"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- 需求表格 -->
    <el-table
      :data="tableData"
      v-loading="loading"
      style="width: 100%"
      row-key="id"
      :default-expand-all="false"
      stripe
    >
      <!-- 展开行：详情 -->
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="expand-detail">
            <el-descriptions :column="1" border size="small">
              <el-descriptions-item label="场景描述">
                {{ row.scenario || '无' }}
              </el-descriptions-item>
              <el-descriptions-item label="痛点">
                {{ row.pain_point || '无' }}
              </el-descriptions-item>
              <el-descriptions-item label="原始对话">
                <pre class="raw-conversation">{{ row.raw_conversation || '无' }}</pre>
              </el-descriptions-item>
              <el-descriptions-item v-if="row.admin_note" label="管理员备注">
                {{ row.admin_note }}
              </el-descriptions-item>
            </el-descriptions>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="submitter" label="提交人" width="120" />

      <el-table-column prop="title" label="需求标题" min-width="200" show-overflow-tooltip />

      <el-table-column prop="module" label="所属模块" width="130" show-overflow-tooltip />

      <el-table-column label="当前状态" width="110" align="center">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)" size="small">
            {{ statusText(row.status) }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="created_at" label="提交时间" width="180" />

      <el-table-column label="操作" width="110" align="center" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="openStatusDialog(row)">
            审核
          </el-button>
        </template>
      </el-table-column>

      <template #empty>
        <el-empty description="暂无需求" />
      </template>
    </el-table>

    <!-- 审核弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      title="审核需求"
      width="520px"
      :close-on-click-modal="false"
    >
      <div class="dialog-body" v-if="currentRow">
        <div class="dialog-req-info">
          <span class="info-label">需求：</span>
          <span class="info-value">{{ currentRow.title }}</span>
        </div>

        <el-form label-position="top" class="dialog-form">
          <el-form-item label="新状态">
            <el-select v-model="newStatus" placeholder="选择新状态" style="width: 100%">
              <el-option
                v-for="opt in statusOptions"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="管理员备注">
            <el-input
              v-model="adminNote"
              type="textarea"
              :rows="4"
              placeholder="填写审核意见、排期说明或拒绝理由"
            />
          </el-form-item>
        </el-form>
      </div>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="dialogLoading" @click="submitStatusUpdate">
          确认
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.requirement-inbox {
  max-width: 1200px;
}

.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0 0 6px 0;
  font-size: 22px;
  font-weight: 700;
  color: #1e293b;
}

.subtitle {
  margin: 0;
  font-size: 13px;
  color: #64748b;
}

.status-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.status-tab {
  padding: 6px 16px;
  border: 1px solid #e2e8f0;
  border-radius: 20px;
  background: #fff;
  color: #64748b;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.status-tab:hover {
  border-color: #c7d2fe;
  color: #4f46e5;
}

.status-tab.active {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  border-color: transparent;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.25);
}

.expand-detail {
  padding: 12px 20px;
}

.raw-conversation {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
  line-height: 1.6;
  color: #475569;
  max-height: 240px;
  overflow-y: auto;
}

.dialog-body {
  padding: 0 4px;
}

.dialog-req-info {
  margin-bottom: 20px;
  padding: 12px 16px;
  background: #f8fafc;
  border-radius: 8px;
  border-left: 3px solid #6366f1;
}

.info-label {
  font-size: 13px;
  color: #64748b;
  font-weight: 500;
}

.info-value {
  font-size: 14px;
  color: #1e293b;
  font-weight: 600;
}

.dialog-form {
  margin-top: 4px;
}
</style>
