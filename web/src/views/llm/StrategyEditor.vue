<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface Option {
  vendor_id: string
  vendor_name: string
  key_id: string
  key_name: string
  model_id: string
  display_name: string
}

interface Strategy {
  id: string
  name: string
  type: 'fixed' | 'round_robin' | 'system'
  options: string[]
  active: boolean
  user_id?: number
  username?: string
}

interface VendorGroup {
  vendor_id: string
  vendor_name: string
  keys: KeyGroup[]
}

interface KeyGroup {
  key_id: string
  key_name: string
  models: ModelGroup[]
}

interface ModelGroup {
  model_id: string
  display_name: string
}

const availableOptions = ref<Option[]>([])
// 全部选项（含已禁用 key），专门用于策略名称翻译
// 历史策略可能引用 enabled=0 的 key，翻译时必须能查到真实名称
const allOptions = ref<Option[]>([])
const strategies = ref<Strategy[]>([])
const loading = ref(false)

const selectedId = ref<string | null>(null)
const editForm = ref<{ name: string; type: 'fixed' | 'round_robin' | 'system'; options: string[] }>({
  name: '',
  type: 'fixed',
  options: [],
})
const showAddOption = ref(false)

// 三级级联选择状态
const cascadeVendor = ref('')
const cascadeKey = ref('')
const cascadeModel = ref('')

const selectedStrategy = () => strategies.value.find(s => s.id === selectedId.value)

// ── 全局系统策略（管理员维护）──
const isAdmin = ref(false)
const showSystemDialog = ref(false)
const systemStrategyLoading = ref(false)
// 全局系统策略编辑表单（只支持轮询模式）
const systemForm = ref<{ options: string[] }>({ options: [] })
// 系统策略弹窗内的级联选择状态（独立于用户策略的级联，避免互相干扰）
const sysCascadeVendor = ref('')
const sysCascadeKey = ref('')
const sysCascadeModel = ref('')
const sysShowAddOption = ref(false)

// 检查当前用户是否为管理员（通过 API 查询，与 Home.vue 一致）
async function checkAdmin() {
  try {
    const token = localStorage.getItem('aios_token') || ''
    if (!token) return
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    const data = await res.json()
    if (data?.ok && data?.data) {
      isAdmin.value = !!data.data.is_admin
    }
  } catch (e: any) {
    console.error('[StrategyEditor] checkAdmin error:', e)
    isAdmin.value = false
  }
}

// 系统策略弹窗：级联变化时重置下级
function onSysVendorChange() {
  sysCascadeKey.value = ''
  sysCascadeModel.value = ''
}
function onSysKeyChange() {
  sysCascadeModel.value = ''
}
function onSysModelChange(modelId: string) {
  if (!modelId || !sysCascadeVendor.value || !sysCascadeKey.value) return
  const val = optionValue(sysCascadeVendor.value, sysCascadeKey.value, modelId)
  if (!systemForm.value.options.includes(val)) {
    systemForm.value.options.push(val)
  }
  sysShowAddOption.value = false
  sysCascadeVendor.value = ''
  sysCascadeKey.value = ''
  sysCascadeModel.value = ''
}
function removeSysOption(idx: number) {
  systemForm.value.options.splice(idx, 1)
}

// 加载全局系统策略
async function fetchSystemStrategy() {
  systemStrategyLoading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/llm/global-system-strategy`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok && result.data && result.data.length > 0) {
      systemForm.value.options = (result.data[0].options || []).map((o: any) =>
        typeof o === 'string' ? o : `${o.vendor_id}|${o.key_id || ''}|${o.model_id}`
      )
    }
  } catch (e: any) {
    console.error('[StrategyEditor] fetchSystemStrategy error:', e)
    ElMessage.error('加载系统策略失败: ' + e.message)
  }
  systemStrategyLoading.value = false
}

// 打开系统策略弹窗
async function openSystemDialog() {
  showSystemDialog.value = true
  await fetchSystemStrategy()
}

// 保存全局系统策略
async function saveSystemStrategy() {
  if (systemForm.value.options.length === 0) {
    ElMessage.warning('系统策略至少需要选择一个选项')
    return
  }
  try {
    const response = await fetch(`${API_BASE}/api/llm/global-system-strategy`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        strategy: {
          options: systemForm.value.options.map(parseOptionValue),
        },
      }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('系统策略已保存')
      showSystemDialog.value = false
    } else {
      ElMessage.error(result.error || '保存失败')
    }
  } catch (e: any) {
    console.error('[StrategyEditor] saveSystemStrategy error:', e)
    ElMessage.error('保存失败: ' + e.message)
  }
}

// 按厂商→密钥→模型三级分组
const groupedOptions = computed<VendorGroup[]>(() => {
  const vendorMap = new Map<string, VendorGroup>()
  for (const opt of availableOptions.value) {
    if (!vendorMap.has(opt.vendor_id)) {
      vendorMap.set(opt.vendor_id, {
        vendor_id: opt.vendor_id,
        vendor_name: opt.vendor_name,
        keys: [],
      })
    }
    const vendor = vendorMap.get(opt.vendor_id)!
    let keyGroup = vendor.keys.find(k => k.key_id === opt.key_id)
    if (!keyGroup) {
      keyGroup = { key_id: opt.key_id, key_name: opt.key_name, models: [] }
      vendor.keys.push(keyGroup)
    }
    if (!keyGroup.models.find(m => m.model_id === opt.model_id)) {
      keyGroup.models.push({ model_id: opt.model_id, display_name: opt.display_name })
    }
  }
  return Array.from(vendorMap.values())
})

// 全部选项的三级分组（含已禁用 key），专供 resolveOptionLabel 翻译用
// 与 groupedOptions 区分：groupedOptions 仅含启用项（级联选择用），allOptionsGrouped 含全部（翻译用）
const allOptionsGrouped = computed<VendorGroup[]>(() => {
  const vendorMap = new Map<string, VendorGroup>()
  for (const opt of allOptions.value) {
    if (!vendorMap.has(opt.vendor_id)) {
      vendorMap.set(opt.vendor_id, {
        vendor_id: opt.vendor_id,
        vendor_name: opt.vendor_name,
        keys: [],
      })
    }
    const vendor = vendorMap.get(opt.vendor_id)!
    let keyGroup = vendor.keys.find(k => k.key_id === opt.key_id)
    if (!keyGroup) {
      keyGroup = { key_id: opt.key_id, key_name: opt.key_name, models: [] }
      vendor.keys.push(keyGroup)
    }
    if (!keyGroup.models.find(m => m.model_id === opt.model_id)) {
      keyGroup.models.push({ model_id: opt.model_id, display_name: opt.display_name })
    }
  }
  return Array.from(vendorMap.values())
})

// 当前选中厂商下的密钥列表
const cascadeKeys = computed<KeyGroup[]>(() => {
  const vendor = groupedOptions.value.find(v => v.vendor_id === cascadeVendor.value)
  return vendor ? vendor.keys : []
})

// 当前选中密钥下的模型列表
const cascadeModels = computed<ModelGroup[]>(() => {
  const keyGroup = cascadeKeys.value.find(k => k.key_id === cascadeKey.value)
  return keyGroup ? keyGroup.models : []
})

// 系统策略弹窗的级联列表（独立状态，避免与用户策略编辑互相干扰）
const sysCascadeKeys = computed<KeyGroup[]>(() => {
  const vendor = groupedOptions.value.find(v => v.vendor_id === sysCascadeVendor.value)
  return vendor ? vendor.keys : []
})
const sysCascadeModels = computed<ModelGroup[]>(() => {
  const keyGroup = sysCascadeKeys.value.find(k => k.key_id === sysCascadeKey.value)
  return keyGroup ? keyGroup.models : []
})

function optionValue(vendorId: string, keyId: string, modelId: string) {
  return `${vendorId}|${keyId}|${modelId}`
}

function resolveOptionLabel(val: string) {
  const parts = val.split('|')
  const vendorId = parts[0]
  const keyId = parts[1]
  const modelId = parts[2]
  // 用 allOptionsGrouped（含禁用 key）做翻译，保证历史策略引用的 enabled=0 的 key 也能显示真实名称
  const vendor = allOptionsGrouped.value.find(v => String(v.vendor_id) === vendorId)
  const keyGroup = vendor?.keys.find(k => String(k.key_id) === keyId)
  const model = keyGroup?.models.find(m => m.model_id === modelId)
  if (vendor && keyGroup && model) {
    return `${vendor.vendor_name} / ${keyGroup.key_name} / ${model.display_name}`
  }
  return val
}

// 级联选择变化时重置下级
function onVendorChange() {
  cascadeKey.value = ''
  cascadeModel.value = ''
}

function onKeyChange() {
  cascadeModel.value = ''
}

// 当模型选中后，自动添加选项
function onModelChange(modelId: string) {
  if (!modelId || !cascadeVendor.value || !cascadeKey.value) return
  const val = optionValue(cascadeVendor.value, cascadeKey.value, modelId)
  if (editForm.value.type === 'fixed') {
    editForm.value.options = [val]
  } else {
    if (!editForm.value.options.includes(val)) {
      editForm.value.options.push(val)
    }
  }
  showAddOption.value = false
  cascadeVendor.value = ''
  cascadeKey.value = ''
  cascadeModel.value = ''
}

// ── 数据加载 ──

async function fetchOptions() {
  try {
    const response = await fetch(`${API_BASE}/api/llm/available-options`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      availableOptions.value = result.data || []
    }
  } catch (e: any) {
    console.error('[StrategyEditor] fetchOptions error:', e)
    ElMessage.error('加载选项失败: ' + e.message)
  }
}

// 加载全部选项（含已禁用 key），供 resolveOptionLabel 翻译用
async function fetchAllOptions() {
  try {
    const response = await fetch(`${API_BASE}/api/llm/all-options`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      allOptions.value = result.data || []
    }
  } catch (e: any) {
    console.error('[StrategyEditor] fetchAllOptions error:', e)
  }
}

async function fetchStrategies() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/llm/strategies`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      // 全局系统策略改造后，用户策略列表只展示 fixed/round_robin，system 不再属于用户
      strategies.value = (result.data || []).filter((s: any) => s.type !== 'system').map((s: any) => ({
        id: s.id,
        name: s.name,
        type: s.type,
        options: (s.options || []).map((o: any) =>
          typeof o === 'string' ? o : `${o.vendor_id}|${o.key_id || ''}|${o.model_id}`
        ),
        active: s.active,
        user_id: s.user_id,
        username: s.username,
      }))
    }
  } catch (e: any) {
    console.error('[StrategyEditor] fetchStrategies error:', e)
    ElMessage.error('加载策略失败: ' + e.message)
  }
  loading.value = false
}

// ── 策略操作 ──

function selectStrategy(id: string) {
  selectedId.value = id
  const s = strategies.value.find(s => s.id === id)
  if (s) {
    editForm.value = { name: s.name, type: s.type, options: [...s.options] }
  }
}

function createStrategy() {
  const newId = `strategy_${Date.now()}`
  const s: Strategy = { id: newId, name: '', type: 'fixed', options: [], active: false }
  strategies.value.push(s)
  selectStrategy(newId)
}

function removeOption(idx: number) {
  editForm.value.options.splice(idx, 1)
}

function addOption(val: string) {
  if (!val) return
  if (editForm.value.type === 'fixed') {
    editForm.value.options = [val]
  } else {
    if (!editForm.value.options.includes(val)) {
      editForm.value.options.push(val)
    }
  }
  showAddOption.value = false
}

function parseOptionValue(val: string) {
  const parts = val.split('|')
  return { vendor_id: parts[0] || '', key_id: parts[1] || '', model_id: parts[2] || '' }
}

async function saveStrategy() {
  const s = selectedStrategy()
  if (!s) return
  if (!editForm.value.name.trim()) {
    ElMessage.warning('请输入策略名称')
    return
  }
  if (editForm.value.options.length === 0) {
    ElMessage.warning('请至少选择一个选项')
    return
  }

  try {
    const response = await fetch(`${API_BASE}/api/llm/save-strategy`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        id: s.id,
        name: editForm.value.name.trim(),
        type: editForm.value.type,
        options: editForm.value.options.map(parseOptionValue),
      }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('保存成功')
      await fetchStrategies()
      // 重新选中
      if (result.data?.id) selectedId.value = result.data.id
      else selectStrategy(s.id)

      // 通知Home组件更新策略标识
      window.dispatchEvent(new CustomEvent('strategy-updated'))
    } else {
      ElMessage.error(result.error || '保存失败')
    }
  } catch (e: any) {
    console.error('[StrategyEditor] saveStrategy error:', e)
    ElMessage.error('保存失败: ' + e.message)
  }
}

async function deleteStrategy() {
  const s = selectedStrategy()
  if (!s) return

  try {
    await ElMessageBox.confirm(
      `确定删除策略「${s.name || '未命名'}」？`,
      '删除确认',
      { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' },
    )
  } catch {
    return
  }

  try {
    const response = await fetch(`${API_BASE}/api/llm/delete-strategy?id=${s.id}`, {
      method: 'POST',
      headers: authHeaders(),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('已删除')
      selectedId.value = null
      await fetchStrategies()
    } else {
      ElMessage.error(result.error || '删除失败')
    }
  } catch (e: any) {
    console.error('[StrategyEditor] deleteStrategy error:', e)
    ElMessage.error('删除失败: ' + e.message)
  }
}

async function setActive() {
  const s = selectedStrategy()
  if (!s) return

  try {
    const response = await fetch(`${API_BASE}/api/llm/set-active-strategy`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({ id: s.id }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('已激活')
      await fetchStrategies()
    } else {
      ElMessage.error(result.error || '激活失败')
    }
  } catch (e: any) {
    console.error('[StrategyEditor] setActive error:', e)
    ElMessage.error('激活失败: ' + e.message)
  }
}

function onTypeChange() {
  if (editForm.value.type === 'fixed' && editForm.value.options.length > 1) {
    editForm.value.options = [editForm.value.options[0]]
  }
}

onMounted(async () => {
  checkAdmin()
  await Promise.all([fetchOptions(), fetchAllOptions(), fetchStrategies()])
})
</script>

<template>
  <div class="strategy-editor">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">策略编辑</h2>
      <el-button v-if="isAdmin" type="warning" @click="openSystemDialog">
        系统策略编辑
      </el-button>
    </div>

    <!-- 全局系统策略弹窗（仅管理员可见） -->
    <el-dialog v-model="showSystemDialog" title="系统策略编辑" width="640px">
      <el-alert
        type="warning"
        :closable="false"
        show-icon
        style="margin-bottom: 16px;"
      >
        <template #title>全局生效</template>
        当用户没有激活任何自定义策略时，将统一走此系统策略（轮询模式）。
      </el-alert>

      <div class="detail-section">
        <label class="field-label">轮询选项列表</label>
        <div class="option-tags">
          <el-tag
            v-for="(opt, idx) in systemForm.options"
            :key="idx"
            closable
            size="large"
            @close="removeSysOption(idx)"
          >
            {{ resolveOptionLabel(opt) }}
          </el-tag>

          <el-popover :visible="sysShowAddOption" placement="bottom" :width="480" trigger="click">
            <template #reference>
              <el-button size="small" @click="sysShowAddOption = true">+ 添加选项</el-button>
            </template>
            <div class="cascade-selects">
              <div class="cascade-row">
                <span class="cascade-label">厂商</span>
                <el-select
                  v-model="sysCascadeVendor"
                  placeholder="选择厂商"
                  filterable
                  style="width: 100%;"
                  @change="onSysVendorChange"
                >
                  <el-option
                    v-for="v in groupedOptions"
                    :key="v.vendor_id"
                    :label="v.vendor_name"
                    :value="v.vendor_id"
                  />
                </el-select>
              </div>
              <div class="cascade-row">
                <span class="cascade-label">密钥</span>
                <el-select
                  v-model="sysCascadeKey"
                  placeholder="选择密钥"
                  filterable
                  style="width: 100%;"
                  :disabled="!sysCascadeVendor"
                  @change="onSysKeyChange"
                >
                  <el-option
                    v-for="k in sysCascadeKeys"
                    :key="k.key_id"
                    :label="k.key_name"
                    :value="k.key_id"
                  />
                </el-select>
              </div>
              <div class="cascade-row">
                <span class="cascade-label">模型</span>
                <el-select
                  v-model="sysCascadeModel"
                  placeholder="选择模型"
                  filterable
                  style="width: 100%;"
                  :disabled="!sysCascadeKey"
                  @change="onSysModelChange"
                >
                  <el-option
                    v-for="m in sysCascadeModels"
                    :key="m.model_id"
                    :label="m.display_name"
                    :value="m.model_id"
                    :disabled="systemForm.options.includes(optionValue(sysCascadeVendor, sysCascadeKey, m.model_id))"
                  />
                </el-select>
              </div>
            </div>
          </el-popover>
        </div>
      </div>

      <template #footer>
        <el-button @click="showSystemDialog = false">取消</el-button>
        <el-button type="primary" :loading="systemStrategyLoading" @click="saveSystemStrategy">
          保存
        </el-button>
      </template>
    </el-dialog>

    <div class="editor-layout">
      <!-- Left: Strategy List -->
      <div class="strategy-list">
        <div class="list-scroll">
          <div
            v-for="s in strategies"
            :key="s.id"
            class="strategy-card"
            :class="{ active: s.active, selected: selectedId === s.id }"
            @click="selectStrategy(s.id)"
          >
            <div class="card-main">
              <span class="card-name">{{ s.username ? `${s.username} - ${s.name || '未命名策略'}` : (s.name || '未命名策略') }}</span>
              <div class="card-tags">
                <el-tag size="small" :type="s.type === 'fixed' ? 'primary' : s.type === 'system' ? 'warning' : 'success'" effect="plain">
                  {{ s.type === 'fixed' ? '固定' : s.type === 'system' ? '系统接管' : '轮询' }}
                </el-tag>
                <span v-if="s.active" class="active-dot"></span>
              </div>
            </div>
          </div>

          <div v-if="strategies.length === 0 && !loading" class="list-empty">
            暂无策略
          </div>
        </div>

        <el-button type="primary" style="width: 100%; margin-top: 12px;" @click="createStrategy">
          + 新增策略
        </el-button>
      </div>

      <!-- Right: Detail Editor -->
      <div class="strategy-detail">
        <div v-if="!selectedId" class="detail-empty">
          请从左侧选择或新建一个策略
        </div>

        <template v-else>
          <!-- 普通策略：可编辑（fixed / round_robin） -->
          <div class="detail-section">
            <label class="field-label">策略名称</label>
            <el-input v-model="editForm.name" placeholder="请输入策略名称" />
          </div>

          <div class="detail-section">
            <label class="field-label">策略类型</label>
            <el-radio-group v-model="editForm.type" @change="onTypeChange">
              <el-radio value="fixed">固定 (Fixed)</el-radio>
              <el-radio value="round_robin">轮询 (Round Robin)</el-radio>
            </el-radio-group>
          </div>

          <div class="detail-section">
            <label class="field-label">选项列表</label>

            <div class="option-tags">
              <el-tag
                v-for="(opt, idx) in editForm.options"
                :key="idx"
                closable
                size="large"
                @close="removeOption(idx)"
              >
                {{ resolveOptionLabel(opt) }}
              </el-tag>

              <el-popover :visible="showAddOption" placement="bottom" :width="480" trigger="click">
                <template #reference>
                  <el-button size="small" @click="showAddOption = true">+ 添加选项</el-button>
                </template>
                <div class="cascade-selects">
                  <div class="cascade-row">
                    <span class="cascade-label">厂商</span>
                    <el-select
                      v-model="cascadeVendor"
                      placeholder="选择厂商"
                      filterable
                      style="width: 100%;"
                      @change="onVendorChange"
                    >
                      <el-option
                        v-for="v in groupedOptions"
                        :key="v.vendor_id"
                        :label="v.vendor_name"
                        :value="v.vendor_id"
                      />
                    </el-select>
                  </div>
                  <div class="cascade-row">
                    <span class="cascade-label">密钥</span>
                    <el-select
                      v-model="cascadeKey"
                      placeholder="选择密钥"
                      filterable
                      style="width: 100%;"
                      :disabled="!cascadeVendor"
                      @change="onKeyChange"
                    >
                      <el-option
                        v-for="k in cascadeKeys"
                        :key="k.key_id"
                        :label="k.key_name"
                        :value="k.key_id"
                      />
                    </el-select>
                  </div>
                  <div class="cascade-row">
                    <span class="cascade-label">模型</span>
                    <el-select
                      v-model="cascadeModel"
                      placeholder="选择模型"
                      filterable
                      style="width: 100%;"
                      :disabled="!cascadeKey"
                      @change="onModelChange"
                    >
                      <el-option
                        v-for="m in cascadeModels"
                        :key="m.model_id"
                        :label="m.display_name"
                        :value="m.model_id"
                        :disabled="editForm.options.includes(optionValue(cascadeVendor, cascadeKey, m.model_id))"
                      />
                    </el-select>
                  </div>
                </div>
              </el-popover>
            </div>

            <div v-if="editForm.type === 'fixed' && editForm.options.length >= 1" class="field-hint">
              固定模式仅支持选择一个选项
            </div>
          </div>

          <div class="detail-actions">
            <el-button type="primary" @click="setActive">激活此策略</el-button>
            <el-button @click="saveStrategy">保存</el-button>
            <el-button type="danger" @click="deleteStrategy">删除策略</el-button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.strategy-editor {
  display: flex;
  flex-direction: column;
  gap: 16px;
  height: 100%;
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

.editor-layout {
  display: flex;
  gap: 20px;
  flex: 1;
  min-height: 0;
}

/* Left Panel */
.strategy-list {
  width: 300px;
  flex-shrink: 0;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 16px;
  display: flex;
  flex-direction: column;
  transition: box-shadow 0.2s;
}

.strategy-list:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.list-scroll {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.list-empty {
  text-align: center;
  padding: 40px 0;
  font-size: 13px;
  color: #c0c4cc;
}

.strategy-card {
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid #f0f0f0;
  cursor: pointer;
  transition: all 0.2s;
}

.strategy-card:hover {
  border-color: #dcdfe6;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
}

.strategy-card.selected {
  border-color: #409eff;
  background: #f0f7ff;
}

.strategy-card.active {
  border-left: 3px solid #67c23a;
}

.card-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-name {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
  min-width: 0;
}

.card-tags {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.active-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #67c23a;
  box-shadow: 0 0 6px rgba(103, 194, 58, 0.5);
}

/* Right Panel */
.strategy-detail {
  flex: 1;
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 24px;
  overflow-y: auto;
  transition: box-shadow 0.2s;
}

.strategy-detail:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.detail-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  font-size: 14px;
  color: #c0c4cc;
}

.detail-section {
  margin-bottom: 24px;
}

.field-label {
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: #606266;
  margin-bottom: 8px;
}

.field-hint {
  font-size: 12px;
  color: #909399;
  margin-top: 6px;
}

.option-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.detail-actions {
  display: flex;
  gap: 10px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}

/* 三级级联选择 */
.cascade-selects {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cascade-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cascade-label {
  font-size: 13px;
  font-weight: 500;
  color: #606266;
  white-space: nowrap;
  min-width: 36px;
}
</style>
