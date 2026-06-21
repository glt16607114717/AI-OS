<script setup lang="ts">
import { ref, onMounted, reactive, computed } from 'vue'
import { API_BASE } from '../../api'
import { ElMessage, ElMessageBox } from 'element-plus'

interface Connection {
  id: number
  name: string
  description: string
  config: Record<string, any>
  enabled: boolean
}

interface User {
  id: number
  username: string
}

const connections = ref<Connection[]>([])
const allUsers = ref<User[]>([])
const permMatrix = ref<Record<number, number[]>>({}) // user_id -> [conn_id, ...]
const loading = ref(false)
const saving = ref(false)

// 连接编辑对话框
const connDialog = ref(false)
const editingConn = reactive({ id: 0, name: '', description: '', host: '', port: 3306, user: 'root', password: '', database: '' })

// 获取连接名称（截断）
function connLabel(conn: Connection) {
  const host = conn.config.host || ''
  const db = conn.config.database || ''
  return `${conn.name} (${host}/${db})`
}

function connShortLabel(conn: Connection) {
  return conn.name
}

async function loadData() {
  loading.value = true
  const token = localStorage.getItem('aios_token') || ''
  const headers = { Authorization: `Bearer ${token}` }

  try {
    // 加载连接
    const connRes = await fetch(`${API_BASE}/api/skills/mysql_query/connections`, { headers })
    const connData = await connRes.json()
    if (connData.ok) {
      connections.value = (connData.data || []).map((c: any) => ({
        ...c,
        config: typeof c.config === 'string' ? JSON.parse(c.config) : c.config,
      }))
    }

    // 加载用户
    const userRes = await fetch(`${API_BASE}/api/users`, { headers })
    const userData = await userRes.json()
    if (userData.ok) {
      const users = userData.data?.users || userData.users || []
      allUsers.value = users.map((u: any) => ({ id: u.id, username: u.username }))
    }

    // 加载权限矩阵
    const permRes = await fetch(`${API_BASE}/api/skills/mysql_query/permissions`, { headers })
    const permData = await permRes.json()
    if (permData.ok) {
      const matrix: Record<number, number[]> = {}
      for (const p of permData.data || []) {
        matrix[p.user_id] = p.connection_ids || []
      }
      permMatrix.value = matrix
    }
  } catch (e) {
    ElMessage.error('加载数据失败')
  }
  loading.value = false
}

function hasPerm(userId: number, connId: number): boolean {
  return (permMatrix.value[userId] || []).includes(connId)
}

function togglePerm(userId: number, connId: number) {
  if (!permMatrix.value[userId]) {
    permMatrix.value[userId] = []
  }
  const arr = permMatrix.value[userId]
  const idx = arr.indexOf(connId)
  if (idx >= 0) {
    arr.splice(idx, 1)
  } else {
    arr.push(connId)
  }
}

async function savePermissions() {
  saving.value = true
  const token = localStorage.getItem('aios_token') || ''
  const permissions = Object.entries(permMatrix.value).map(([userId, connIds]) => ({
    user_id: parseInt(userId),
    connection_ids: connIds,
  }))

  try {
    const res = await fetch(`${API_BASE}/api/skills/mysql_query/permission`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ permissions }),
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('权限保存成功')
    } else {
      ElMessage.error(data.error || '保存失败')
    }
  } catch (e) {
    ElMessage.error('网络错误')
  }
  saving.value = false
}

// ── 连接管理 ──
function openCreate() {
  editingConn.id = 0
  editingConn.name = ''
  editingConn.description = ''
  editingConn.host = ''
  editingConn.port = 3306
  editingConn.user = 'root'
  editingConn.password = ''
  editingConn.database = ''
  connDialog.value = true
}

function openEdit(conn: Connection) {
  editingConn.id = conn.id
  editingConn.name = conn.name
  editingConn.description = conn.description || ''
  editingConn.host = conn.config.host || ''
  editingConn.port = conn.config.port || 3306
  editingConn.user = conn.config.user || ''
  editingConn.password = conn.config.password || ''
  editingConn.database = conn.config.database || ''
  connDialog.value = true
}

async function saveConnection() {
  if (!editingConn.name || !editingConn.host || !editingConn.database) {
    ElMessage.warning('请填写完整信息')
    return
  }
  const token = localStorage.getItem('aios_token') || ''
  const body = {
    name: editingConn.name,
    description: editingConn.description,
    config: {
      host: editingConn.host,
      port: editingConn.port,
      user: editingConn.user,
      password: editingConn.password,
      database: editingConn.database,
    },
  }
  try {
    let res
    if (editingConn.id) {
      res = await fetch(`${API_BASE}/api/skills/mysql_query/connection/${editingConn.id}`, {
        method: 'PUT', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(body),
      })
    } else {
      res = await fetch(`${API_BASE}/api/skills/mysql_query/connection`, {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(body),
      })
    }
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(editingConn.id ? '更新成功' : '创建成功')
      connDialog.value = false
      loadData()
    } else {
      ElMessage.error(data.error || '操作失败')
    }
  } catch (e) {
    ElMessage.error('网络错误')
  }
}

async function deleteConnection(conn: Connection) {
  try {
    await ElMessageBox.confirm(`确定删除连接 "${conn.name}" 吗？`, '确认', { type: 'warning' })
    const token = localStorage.getItem('aios_token') || ''
    const res = await fetch(`${API_BASE}/api/skills/mysql_query/connection/${conn.id}`, {
      method: 'DELETE', headers: { Authorization: `Bearer ${token}` },
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('删除成功')
      loadData()
    } else {
      ElMessage.error(data.error || '删除失败')
    }
  } catch (e) { /* cancelled */ }
}

onMounted(loadData)
</script>

<template>
  <div class="skill-page">
    <h2>MySQL 查询</h2>
    <p class="desc">配置 MySQL 连接和用户权限。AI 在代理场景中会自动识别用户意图并调用此技能。</p>

    <!-- 连接管理 -->
    <div class="section">
      <div class="section-header">
        <h3>连接管理</h3>
        <button class="btn-primary" @click="openCreate">+ 新建连接</button>
      </div>
      <table class="data-table">
        <thead>
          <tr>
            <th style="width:120px">名称</th>
            <th>库用途说明</th>
            <th>主机</th>
            <th style="width:70px">端口</th>
            <th style="width:100px">用户</th>
            <th style="width:160px">数据库</th>
            <th style="width:70px">状态</th>
            <th style="width:100px">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="conn in connections" :key="conn.id">
            <td><strong>{{ conn.name }}</strong></td>
            <td class="desc-cell">{{ conn.description || '—' }}</td>
            <td class="mono">{{ conn.config.host }}</td>
            <td>{{ conn.config.port }}</td>
            <td>{{ conn.config.user }}</td>
            <td class="mono">{{ conn.config.database }}</td>
            <td>
              <span :class="['tag', conn.enabled ? 'tag-green' : 'tag-gray']">
                {{ conn.enabled ? '启用' : '停用' }}
              </span>
            </td>
            <td>
              <button class="btn-sm" @click="openEdit(conn)">编辑</button>
              <button class="btn-sm btn-danger" @click="deleteConnection(conn)">删除</button>
            </td>
          </tr>
          <tr v-if="connections.length === 0">
            <td colspan="8" class="empty">暂无连接，请先创建</td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 权限矩阵 -->
    <div class="section">
      <div class="section-header">
        <h3>权限管理</h3>
        <button class="btn-primary" @click="savePermissions" :disabled="saving">
          {{ saving ? '保存中...' : '保存权限' }}
        </button>
      </div>
      <div class="matrix-wrap">
        <table class="matrix-table">
          <thead>
            <tr>
              <th class="user-col">用户</th>
              <th v-for="conn in connections" :key="conn.id" class="conn-col" :title="connLabel(conn)">
                {{ connShortLabel(conn) }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in allUsers" :key="user.id">
              <td class="user-col">{{ user.username }}</td>
              <td v-for="conn in connections" :key="conn.id" class="conn-col check-cell">
                <label class="check-label">
                  <input
                    type="checkbox"
                    :checked="hasPerm(user.id, conn.id)"
                    @change="togglePerm(user.id, conn.id)"
                  />
                </label>
              </td>
            </tr>
            <tr v-if="allUsers.length === 0">
              <td :colspan="connections.length + 1" class="empty">暂无用户数据</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="hint">勾选后点击「保存权限」生效。管理员自动拥有全部连接权限，无需在此配置。</p>
    </div>

    <!-- 连接编辑对话框 -->
    <div v-if="connDialog" class="modal-overlay" @click.self="connDialog = false">
      <div class="modal">
        <h3>{{ editingConn.id ? '编辑连接' : '新建连接' }}</h3>
        <div class="form-group">
          <label>连接名称</label>
          <input v-model="editingConn.name" placeholder="如：生产库" />
        </div>
        <div class="form-group">
          <label>库用途说明 <span class="hint-inline">（告诉 AI 这个库里存的是什么，帮助判断是否需要查询）</span></label>
          <textarea v-model="editingConn.description" rows="3" placeholder="如：存储用户、订单、商品等业务数据，用于排查业务问题"></textarea>
        </div>
        <div class="form-row">
          <div class="form-group flex-1">
            <label>主机地址</label>
            <input v-model="editingConn.host" placeholder="如：8.163.127.182" />
          </div>
          <div class="form-group" style="width: 90px">
            <label>端口</label>
            <input v-model.number="editingConn.port" type="number" />
          </div>
        </div>
        <div class="form-row">
          <div class="form-group flex-1">
            <label>用户名</label>
            <input v-model="editingConn.user" />
          </div>
          <div class="form-group flex-1">
            <label>密码</label>
            <input v-model="editingConn.password" type="password" placeholder="编辑时 *** 表示不变" />
          </div>
        </div>
        <div class="form-group">
          <label>数据库名</label>
          <input v-model="editingConn.database" placeholder="如：ai_os" />
        </div>
        <div class="modal-footer">
          <button class="btn-default" @click="connDialog = false">取消</button>
          <button class="btn-primary" @click="saveConnection">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.skill-page {
  padding: 24px;
  max-width: 1200px;
}
.skill-page h2 {
  margin: 0 0 8px;
  font-size: 20px;
}
.desc {
  color: #666;
  font-size: 13px;
  margin-bottom: 24px;
}
.section {
  margin-bottom: 32px;
}
.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.section-header h3 {
  margin: 0;
  font-size: 16px;
}

/* 通用表格 */
.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.data-table th {
  text-align: left;
  padding: 8px 12px;
  background: #f5f7fa;
  border-bottom: 2px solid #ebeef5;
  font-weight: 600;
  color: #606266;
  white-space: nowrap;
}
.data-table td {
  padding: 8px 12px;
  border-bottom: 1px solid #ebeef5;
}
.data-table tr:hover td {
  background: #f5f7fa;
}
.mono {
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 12px;
}

/* 权限矩阵 */
.matrix-wrap {
  overflow-x: auto;
  border: 1px solid #ebeef5;
  border-radius: 6px;
}
.matrix-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  min-width: 600px;
}
.matrix-table th,
.matrix-table td {
  padding: 10px 12px;
  border-bottom: 1px solid #ebeef5;
  text-align: center;
}
.matrix-table thead th {
  background: #f5f7fa;
  font-weight: 600;
  color: #606266;
  white-space: nowrap;
  position: sticky;
  top: 0;
  z-index: 1;
}
.matrix-table .user-col {
  text-align: left;
  font-weight: 500;
  background: #fafafa;
  min-width: 100px;
  position: sticky;
  left: 0;
  z-index: 1;
}
.matrix-table thead .user-col {
  z-index: 2;
}
.matrix-table .conn-col {
  min-width: 80px;
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.matrix-table tbody tr:hover td {
  background: #f0f7ff;
}
.matrix-table tbody tr:hover .user-col {
  background: #e8f2fc;
}
.check-cell {
  padding: 6px !important;
}
.check-label {
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  padding: 4px;
}
.check-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
  accent-color: #409eff;
}

.hint {
  margin-top: 8px;
  font-size: 12px;
  color: #999;
}

/* 标签 */
.tag {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
}
.tag-green { background: #e6f7ec; color: #52c41a; }
.tag-gray { background: #f0f0f0; color: #999; }

/* 按钮 */
.btn-primary {
  background: #409eff;
  color: #fff;
  border: none;
  padding: 6px 16px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
}
.btn-primary:hover { background: #66b1ff; }
.btn-primary:disabled { background: #a0cfff; cursor: not-allowed; }
.btn-default {
  background: #fff;
  color: #606266;
  border: 1px solid #dcdfe6;
  padding: 6px 16px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
}
.btn-default:hover { border-color: #409eff; color: #409eff; }
.btn-sm {
  background: none;
  border: 1px solid #dcdfe6;
  padding: 2px 8px;
  border-radius: 3px;
  cursor: pointer;
  font-size: 12px;
  margin-right: 4px;
  color: #606266;
}
.btn-sm:hover { border-color: #409eff; color: #409eff; }
.btn-danger { border-color: #f56c6c; color: #f56c6c; }
.btn-danger:hover { background: #fef0f0; }

.empty {
  text-align: center;
  color: #999;
  padding: 24px;
}

.desc-cell {
  font-size: 12px;
  color: #666;
  line-height: 1.5;
  max-width: 280px;
}

.hint-inline {
  font-size: 12px;
  color: #999;
  font-weight: normal;
}

textarea {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  font-size: 13px;
  resize: vertical;
  font-family: inherit;
  box-sizing: border-box;
}
textarea:focus {
  outline: none;
  border-color: #409eff;
}

/* 对话框 */
.modal-overlay {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.modal {
  background: #fff;
  border-radius: 8px;
  padding: 24px;
  width: 520px;
  max-height: 80vh;
  overflow-y: auto;
}
.modal h3 { margin: 0 0 20px; font-size: 16px; }
.form-group { margin-bottom: 16px; }
.form-group label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  color: #606266;
  font-weight: 500;
}
.form-group input, .form-group select {
  width: 100%;
  padding: 6px 10px;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  font-size: 13px;
  box-sizing: border-box;
}
.form-group input:focus, .form-group select:focus {
  outline: none;
  border-color: #409eff;
}
.form-row { display: flex; gap: 12px; }
.flex-1 { flex: 1; }
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 20px;
}
</style>
