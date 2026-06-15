<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { CirclePlus, Delete, Edit } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

import { API_BASE as BASE_URL } from '../../api'

interface User {
  id: number
  username: string
  status: number
  is_admin: number
  created_at: string
  updated_at: string
}

const users = ref<User[]>([])
const loading = ref(false)
const showAddDialog = ref(false)
const showEditDialog = ref(false)
const editTarget = ref<User | null>(null)

const addForm = reactive({ username: '', password: '', is_admin: 0 })
const editForm = reactive({ password: '' })

// ── Auth token ──

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

function authHeaders(): Record<string, string> {
  return { 'Authorization': `Bearer ${getToken()}`, 'Content-Type': 'application/json' }
}

// ── 数据加载 ──

async function fetchUsers() {
  loading.value = true
  try {
    const res = await fetch(`${BASE_URL}/api/users`, { headers: authHeaders() })
    const data = await res.json()
    if (data.ok) {
      // 兼容两种返回格式：data.data.users 和 data.data（数组）
      const raw = data.data
      users.value = Array.isArray(raw) ? raw : (raw?.users || [])
    } else if (data.error === '未登录' || res.status === 401) {
      ElMessage.error('请先登录管理员账号')
    } else {
      ElMessage.error(data.error || '加载用户列表失败')
    }
  } catch (e: any) {
    console.error('[AccountSettings] fetchUsers error:', e)
    ElMessage.error('加载用户列表失败: ' + e.message)
  }
  loading.value = false
}

// ── 新增用户 ──

function openAddDialog() {
  addForm.username = ''
  addForm.password = ''
  addForm.is_admin = 0
  showAddDialog.value = true
}

async function doAdd() {
  if (!addForm.username.trim()) { ElMessage.warning('请输入用户名'); return }
  if (addForm.password.length < 4) { ElMessage.warning('密码至少 4 个字符'); return }
  try {
    const res = await fetch(`${BASE_URL}/api/users/create`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify(addForm),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('用户创建成功')
      showAddDialog.value = false
      fetchUsers()
    } else {
      ElMessage.error(data.error || '创建失败')
    }
  } catch (e: any) {
    console.error('[AccountSettings] doAdd error:', e)
    ElMessage.error('请求失败: ' + e.message)
  }
}

// ── 编辑用户 ──

function openEditDialog(user: User) {
  editTarget.value = user
  editForm.password = ''
  showEditDialog.value = true
}

async function doEdit() {
  if (!editTarget.value) return
  if (editForm.password.length < 4) { ElMessage.warning('密码至少 4 个字符'); return }
  try {
    const res = await fetch(`${BASE_URL}/api/users/update-password`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ id: editTarget.value.id, password: editForm.password }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('密码已更新')
      showEditDialog.value = false
    } else {
      ElMessage.error(data.error || '更新失败')
    }
  } catch (e: any) {
    console.error('[AccountSettings] doEdit error:', e)
    ElMessage.error('请求失败: ' + e.message)
  }
}

// ── 启用/停用 ──

async function toggleStatus(user: User) {
  const newStatus = user.status === 1 ? 2 : 1
  const label = newStatus === 1 ? '启用' : '停用'
  try {
    await ElMessageBox.confirm(`确认${label}用户「${user.username}」？`, '提示', { type: 'warning' })
    const res = await fetch(`${BASE_URL}/api/users/toggle-status`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ id: user.id, status: newStatus }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(`已${label}`)
      fetchUsers()
    } else {
      ElMessage.error(data.error || `${label}失败`)
    }
  } catch { /* 取消 */ }
}

// ── 管理员切换 ──

async function toggleAdmin(user: User) {
  const newIsAdmin = !user.is_admin
  const label = newIsAdmin ? '设为管理员' : '取消管理员'
  try {
    await ElMessageBox.confirm(`确认${label}「${user.username}」？`, '提示', { type: 'warning' })
    const res = await fetch(`${BASE_URL}/api/users/toggle-admin`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ id: user.id, is_admin: newIsAdmin }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(`已${label}`)
      fetchUsers()
    } else {
      ElMessage.error(data.error || `${label}失败`)
    }
  } catch { /* 取消 */ }
}

// ── 删除用户 ──

async function doDelete(user: User) {
  try {
    await ElMessageBox.confirm(`确认删除用户「${user.username}」？此操作不可恢复。`, '警告', { type: 'error' })
    const res = await fetch(`${BASE_URL}/api/users/delete`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ id: user.id }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('已删除')
      fetchUsers()
    } else {
      ElMessage.error(data.error || '删除失败')
    }
  } catch { /* 取消 */ }
}

function formatTime(t: string) {
  if (!t) return '-'
  return t.replace('T', ' ').slice(0, 19)
}

onMounted(fetchUsers)
</script>

<template>
  <div class="account-settings">
    <div class="page-header">
      <div>
        <h2 class="page-title">账户设置</h2>
        <p class="page-desc">管理系统登录账户</p>
      </div>
      <el-button type="primary" :icon="CirclePlus" @click="openAddDialog">添加用户</el-button>
    </div>

    <el-table :data="users" v-loading="loading" stripe class="user-table">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column prop="username" label="用户名" width="150" />
      <el-table-column label="管理员" width="100">
        <template #default="{ row }">
          <el-tag :type="row.is_admin ? 'danger' : 'info'" size="small" style="cursor:pointer" @click="toggleAdmin(row)">
            {{ row.is_admin ? '管理员' : '普通' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">
            {{ row.status === 1 ? '启用' : '停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="创建时间" width="180">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" min-width="240">
        <template #default="{ row }">
          <el-button size="small" :icon="Edit" @click="openEditDialog(row)">改密码</el-button>
          <el-button size="small" :type="row.status === 1 ? 'warning' : 'success'" @click="toggleStatus(row)">
            {{ row.status === 1 ? '停用' : '启用' }}
          </el-button>
          <el-button size="small" type="danger" :icon="Delete" @click="doDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增对话框 -->
    <el-dialog v-model="showAddDialog" title="添加用户" width="420px">
      <el-form label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="addForm.username" placeholder="中文名字" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="addForm.password" type="password" show-password placeholder="至少 4 个字符" />
        </el-form-item>
        <el-form-item label="管理员">
          <el-switch v-model="addForm.is_admin" :active-value="1" :inactive-value="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="doAdd">确认</el-button>
      </template>
    </el-dialog>

    <!-- 改密码对话框 -->
    <el-dialog v-model="showEditDialog" title="修改密码" width="420px">
      <el-form label-width="80px">
        <el-form-item label="用户名">
          <el-input :model-value="editTarget?.username" disabled />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="editForm.password" type="password" show-password placeholder="至少 4 个字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditDialog = false">取消</el-button>
        <el-button type="primary" @click="doEdit">确认</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.account-settings {
  max-width: 900px;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 24px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
  color: #1e293b;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #94a3b8;
  margin: 0;
}

.user-table {
  border-radius: 8px;
  overflow: hidden;
}
</style>
