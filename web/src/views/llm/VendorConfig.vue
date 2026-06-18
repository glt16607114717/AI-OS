<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { CirclePlus, Delete, ArrowDown, Check } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface ApiKey {
  id: string
  name: string
  api_key: string
  enabled: boolean
}

interface Model {
  model_id: string
  display_name: string
  description: string
}

interface Vendor {
  id: number
  code: string
  name: string
  base_url: string
  models: Model[]
  keys: ApiKey[]
  enabled: boolean
  expanded: boolean
}

interface NewKeyForm {
  name: string
  api_key: string
}

const vendors = ref<Vendor[]>([])
const loading = ref(false)

// 正在添加密钥的厂商 ID
const addingKeyVendorId = ref<number | null>(null)
const newKeyForm = reactive<NewKeyForm>({ name: '', api_key: '' })

// ── 数据加载 ──

async function fetchCatalog() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/llm/catalog`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      vendors.value = (result.data || []).map((v: any) => ({
        id: v.id,
        code: v.code || '',
        name: v.name,
        base_url: v.base_url || '',
        models: (v.models || []).map((m: any) => ({
          model_id: m.model_id || '',
          display_name: m.display_name || '',
          description: m.description || '',
        })),
        keys: (v.keys || []).map((k: any) => ({
          id: k.id,
          name: k.name || '',
          api_key: k.api_key || '',
          enabled: k.enabled !== false,
        })),
        enabled: v.enabled !== false,
        expanded: false,
      }))
    }
  } catch (e: any) {
    console.error('[VendorConfig] fetchCatalog error:', e)
    ElMessage.error('加载厂商配置失败: ' + e.message)
  }
  loading.value = false
}

// ── 厂商开关 ──

async function toggleVendor(vendor: Vendor) {
  const newVal = !vendor.enabled
  try {
    const response = await fetch(`${API_BASE}/api/llm/toggle-vendor?vendor_id=${vendor.id}`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ enabled: newVal }),
    })
    const result = await response.json()
    if (result.ok) {
      vendor.enabled = newVal
    } else {
      ElMessage.error(result.error || '操作失败')
    }
  } catch (e: any) {
    console.error('[VendorConfig] toggleVendor error:', e)
    ElMessage.error('操作失败: ' + e.message)
  }
}

// ── 展开/收起 ──

function toggleExpand(vendor: Vendor) {
  vendor.expanded = !vendor.expanded
}

// ── 密钥管理 ──

function startAddKey(vendorId: number) {
  addingKeyVendorId.value = vendorId
  newKeyForm.name = ''
  newKeyForm.api_key = ''
}

function cancelAddKey() {
  addingKeyVendorId.value = null
  newKeyForm.name = ''
  newKeyForm.api_key = ''
}

async function confirmAddKey(vendor: Vendor) {
  if (!newKeyForm.name.trim()) {
    ElMessage.warning('请输入密钥名称')
    return
  }
  if (!newKeyForm.api_key.trim()) {
    ElMessage.warning('请输入 API Key')
    return
  }

  const updatedKeys = [
    ...vendor.keys.map(k => ({ name: k.name, api_key: k.api_key, enabled: k.enabled })),
    { name: newKeyForm.name.trim(), api_key: newKeyForm.api_key.trim(), enabled: true },
  ]

  try {
    const response = await fetch(`${API_BASE}/api/llm/vendor-keys?vendor_id=${vendor.id}`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ keys: updatedKeys }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('密钥已添加')
      addingKeyVendorId.value = null
      await fetchCatalog()
      const updated = vendors.value.find(v => v.id === vendor.id)
      if (updated) updated.expanded = true
    } else {
      ElMessage.error(result.error || '添加失败')
    }
  } catch (e: any) {
    console.error('[VendorConfig] confirmAddKey error:', e)
    ElMessage.error('添加失败: ' + e.message)
  }
}

async function removeKey(vendor: Vendor, key: ApiKey) {
  try {
    await ElMessageBox.confirm(
      `确定删除密钥「${key.name}」？`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }

  const updatedKeys = vendor.keys
    .filter(k => k.id !== key.id)
    .map(k => ({ name: k.name, api_key: k.api_key, enabled: k.enabled }))

  try {
    const response = await fetch(`${API_BASE}/api/llm/vendor-keys?vendor_id=${vendor.id}`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ keys: updatedKeys }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('密钥已删除')
      await fetchCatalog()
      const updated = vendors.value.find(v => v.id === vendor.id)
      if (updated) updated.expanded = true
    } else {
      ElMessage.error(result.error || '删除失败')
    }
  } catch (e: any) {
    console.error('[VendorConfig] removeKey error:', e)
    ElMessage.error('删除失败: ' + e.message)
  }
}

async function toggleKey(vendor: Vendor, key: ApiKey) {
  const updatedKeys = vendor.keys.map(k =>
    k.id === key.id
      ? { name: k.name, api_key: k.api_key, enabled: !k.enabled }
      : { name: k.name, api_key: k.api_key, enabled: k.enabled },
  )

  try {
    const response = await fetch(`${API_BASE}/api/llm/vendor-keys?vendor_id=${vendor.id}`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ keys: updatedKeys }),
    })
    const result = await response.json()
    if (result.ok) {
      key.enabled = !key.enabled
    } else {
      ElMessage.error(result.error || '操作失败')
    }
  } catch (e: any) {
    console.error('[VendorConfig] toggleKey error:', e)
    ElMessage.error('操作失败: ' + e.message)
  }
}

onMounted(() => {
  fetchCatalog()
})
</script>

<template>
  <div class="vendor-config">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">基础配置</h2>
    </div>

    <!-- Loading -->
    <div v-if="loading && vendors.length === 0" class="empty-state">
      <p class="empty-text">加载中...</p>
    </div>

    <!-- Empty State -->
    <div v-else-if="vendors.length === 0" class="empty-state">
      <p class="empty-text">暂无厂商配置</p>
    </div>

    <!-- Vendor Cards -->
    <div
      v-for="vendor in vendors"
      :key="vendor.id"
      class="vendor-card"
      :class="{ disabled: !vendor.enabled }"
    >
      <!-- Card Header -->
      <div class="vendor-header" @click="toggleExpand(vendor)">
        <div class="vendor-info">
          <div class="vendor-name-row">
            <span class="vendor-name">{{ vendor.name }}</span>
            <span class="vendor-url">{{ vendor.base_url }}</span>
          </div>
          <div class="vendor-meta">
            <span class="meta-tag">{{ vendor.models.length }} 个模型</span>
            <span class="meta-tag">{{ vendor.keys.length }} 个密钥</span>
          </div>
        </div>
        <div class="vendor-switch" @click.stop>
          <el-switch
            :model-value="vendor.enabled"
            size="small"
            @change="toggleVendor(vendor)"
          />
        </div>
        <el-icon class="expand-arrow" :class="{ expanded: vendor.expanded }">
          <ArrowDown />
        </el-icon>
      </div>

      <!-- Expanded Content -->
      <transition name="card-expand">
        <div v-if="vendor.expanded" class="vendor-body">
          <!-- 密钥管理区 -->
          <div class="section">
            <div class="section-header">
              <span class="section-title">密钥管理</span>
              <el-button
                v-if="addingKeyVendorId !== vendor.id"
                type="primary"
                :icon="CirclePlus"
                size="small"
                @click="startAddKey(vendor.id)"
              >
                添加密钥
              </el-button>
            </div>

            <!-- 添加密钥内联表单 -->
            <transition name="card-expand">
              <div v-if="addingKeyVendorId === vendor.id" class="key-form">
                <el-input
                  v-model="newKeyForm.name"
                  placeholder="密钥名称"
                  size="small"
                  class="key-input"
                />
                <el-input
                  v-model="newKeyForm.api_key"
                  placeholder="API Key"
                  size="small"
                  class="key-input key-input-wide"
                />
                <el-button
                  size="small"
                  type="primary"
                  :icon="Check"
                  @click="confirmAddKey(vendor)"
                >
                  保存
                </el-button>
                <el-button size="small" @click="cancelAddKey">取消</el-button>
              </div>
            </transition>

            <!-- 密钥列表 -->
            <div v-if="vendor.keys.length === 0 && addingKeyVendorId !== vendor.id" class="key-empty">
              暂无密钥，点击上方按钮添加
            </div>

            <div v-for="key in vendor.keys" :key="key.id" class="key-row">
              <span class="key-name">{{ key.name }}</span>
              <span class="key-value">{{ key.api_key }}</span>
              <el-switch
                :model-value="key.enabled"
                size="small"
                @change="toggleKey(vendor, key)"
              />
              <el-button
                size="small"
                text
                type="danger"
                :icon="Delete"
                @click="removeKey(vendor, key)"
              />
            </div>
          </div>

          <!-- 模型列表区（只读） -->
          <div class="section">
            <div class="section-header">
              <span class="section-title">模型列表</span>
              <el-tag size="small" type="info">{{ vendor.models.length }} 个模型</el-tag>
            </div>

            <div v-if="vendor.models.length === 0" class="model-empty">
              暂无模型
            </div>

            <div v-for="model in vendor.models" :key="model.model_id" class="model-row">
              <span class="model-id">{{ model.model_id }}</span>
              <span class="model-display">{{ model.display_name }}</span>
              <span v-if="model.description" class="model-desc">{{ model.description }}</span>
            </div>
          </div>
        </div>
      </transition>
    </div>
  </div>
</template>

<style scoped>
.vendor-config {
  padding-bottom: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

/* Empty State */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 60px 0;
  color: #c0c4cc;
}

.empty-text {
  font-size: 15px;
  font-weight: 500;
  color: #909399;
}

/* Vendor Card */
.vendor-card {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  overflow: hidden;
  transition: box-shadow 0.2s;
}

.vendor-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.vendor-card.disabled {
  opacity: 0.6;
}

.vendor-header {
  display: flex;
  align-items: center;
  padding: 16px 20px;
  cursor: pointer;
  gap: 12px;
}

.vendor-info {
  flex: 1;
  min-width: 0;
}

.vendor-name-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 6px;
}

.vendor-name {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.vendor-url {
  font-size: 12px;
  color: #909399;
  font-family: 'Cascadia Code', 'Consolas', monospace;
}

.vendor-meta {
  display: flex;
  gap: 8px;
}

.meta-tag {
  font-size: 12px;
  color: #a8abb2;
}

.vendor-switch {
  flex-shrink: 0;
}

.expand-arrow {
  flex-shrink: 0;
  font-size: 16px;
  color: #c0c4cc;
  transition: transform 0.25s ease;
}

.expand-arrow.expanded {
  transform: rotate(180deg);
}

/* Vendor Body */
.vendor-body {
  border-top: 1px solid #f0f0f0;
  padding: 16px 20px;
  background: #fafbfc;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.card-expand-enter-active {
  transition: all 0.25s ease-out;
}

.card-expand-leave-active {
  transition: all 0.2s ease-in;
}

.card-expand-enter-from,
.card-expand-leave-to {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  padding-bottom: 0;
}

/* Section */
.section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #606266;
}

/* Key Form */
.key-form {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #dcdfe6;
  animation: fade-in 0.15s ease;
}

.key-input {
  flex: 1;
  min-width: 120px;
}

.key-input-wide {
  min-width: 200px;
}

/* Key Empty */
.key-empty {
  text-align: center;
  padding: 12px 0;
  font-size: 13px;
  color: #c0c4cc;
}

/* Key Row */
.key-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #f0f0f0;
  transition: border-color 0.15s;
}

.key-row:hover {
  border-color: #dcdfe6;
}

.key-name {
  font-size: 13px;
  font-weight: 500;
  color: #303133;
  min-width: 80px;
  flex-shrink: 0;
}

.key-value {
  flex: 1;
  font-size: 12px;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  color: #606266;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Model Empty */
.model-empty {
  text-align: center;
  padding: 12px 0;
  font-size: 13px;
  color: #c0c4cc;
}

/* Model Row */
.model-row {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 8px 12px;
  background: #fff;
  border-radius: 8px;
  border: 1px solid #f0f0f0;
  transition: border-color 0.15s;
  flex-wrap: wrap;
}

.model-row:hover {
  border-color: #dcdfe6;
}

.model-id {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 13px;
  color: #409eff;
  flex-shrink: 0;
}

.model-display {
  font-size: 13px;
  color: #606266;
  flex-shrink: 0;
}

.model-desc {
  font-size: 12px;
  color: #909399;
  flex: 1;
  min-width: 0;
}

@keyframes fade-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
