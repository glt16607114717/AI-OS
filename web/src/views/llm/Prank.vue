<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }
}

interface PrankConfig {
  id: number
  target_user_id: number
  target_name: string
  custom_text: string
  enabled: boolean
  created_by: number
}

interface UserOption {
  id: number
  username: string
}

const list = ref<PrankConfig[]>([])
const loading = ref(true)
const users = ref<UserOption[]>([])
const showDialog = ref(false)
const saving = ref(false)
const editForm = ref({ target_user_id: 0, custom_text: '', enabled: true })
const isEdit = ref(false)

onMounted(async () => {
  await Promise.all([loadList(), loadUsers()])
  loading.value = false
})

async function loadList() {
  const res = await fetch(`${API_BASE}/api/prank/list`, { headers: authHeaders() })
  const data = await res.json()
  if (data.ok) list.value = data.data || []
}

async function loadUsers() {
  const res = await fetch(`${API_BASE}/api/users`, { headers: authHeaders() })
  const data = await res.json()
  if (data.ok) users.value = data.data.users || []
}

function openCreate() {
  isEdit.value = false
  editForm.value = { target_user_id: 0, custom_text: '', enabled: true }
  showDialog.value = true
}

function openEdit(row: PrankConfig) {
  isEdit.value = true
  editForm.value = {
    target_user_id: row.target_user_id,
    custom_text: row.custom_text,
    enabled: row.enabled,
  }
  showDialog.value = true
}

async function handleSave() {
  if (!editForm.value.target_user_id || !editForm.value.custom_text) {
    ElMessage.warning('请选择用户并填写内容')
    return
  }
  saving.value = true
  try {
    const res = await fetch(`${API_BASE}/api/prank/save`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify(editForm.value),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(isEdit.value ? '已更新' : '已创建')
      showDialog.value = false
      loadList()
    } else {
      ElMessage.error(data.error || '操作失败')
    }
  } finally {
    saving.value = false
  }
}

async function handleDelete(row: PrankConfig) {
  await ElMessageBox.confirm(`确定删除对 ${row.target_name} 的逗你玩配置？`, '确认', { type: 'warning' })
  const res = await fetch(`${API_BASE}/api/prank/delete?id=${row.id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
  const data = await res.json()
  if (data.ok) {
    ElMessage.success('已删除')
    loadList()
  } else {
    ElMessage.error(data.error || '删除失败')
  }
}

function getUserName(uid: number) {
  const u = users.value.find((u) => u.id === uid)
  return u ? u.username : `用户#${uid}`
}
</script>

<template>
  <div class="prank-page">
    <div class="page-header">
      <h2>逗你玩</h2>
      <p class="subtitle">指定用户发起请求时，不调用大模型，而是无限循环返回自定义内容</p>
    </div>

    <div class="toolbar">
      <el-button type="primary" @click="openCreate">+ 新增配置</el-button>
    </div>

    <el-table :data="list" v-loading="loading" border stripe style="margin-top: 16px">
      <el-table-column prop="id" label="ID" width="60" />
      <el-table-column label="目标用户" width="120">
        <template #default="{ row }">
          {{ row.target_name || `用户#${row.target_user_id}` }}
        </template>
      </el-table-column>
      <el-table-column prop="custom_text" label="自定义内容" min-width="200" show-overflow-tooltip />
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'danger' : 'info'" size="small">
            {{ row.enabled ? '生效中' : '已停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="160">
        <template #default="{ row }">
          <el-button size="small" type="primary" link @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" link @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && list.length === 0" description="暂无配置" />

    <el-dialog v-model="showDialog" :title="isEdit ? '编辑逗你玩' : '新增逗你玩'" width="500px">
      <el-form label-width="80px">
        <el-form-item label="目标用户">
          <el-select v-model="editForm.target_user_id" placeholder="选择用户" filterable :disabled="isEdit">
            <el-option v-for="u in users" :key="u.id" :label="u.username" :value="u.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="自定义内容">
          <el-input v-model="editForm.custom_text" type="textarea" :rows="4" placeholder="输入要循环返回的内容" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="editForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.prank-page {
  padding: 24px;
  max-width: 1000px;
}
.page-header h2 {
  margin: 0 0 8px 0;
  font-size: 20px;
}
.subtitle {
  color: #909399;
  font-size: 13px;
  margin: 0;
}
.toolbar {
  margin-top: 16px;
}
</style>
