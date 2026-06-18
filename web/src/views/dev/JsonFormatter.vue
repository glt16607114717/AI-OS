<script setup lang="ts">
import { ref, reactive, watch, computed } from 'vue'

const input = ref('')
const output = ref('')
const parsedData = ref<any>(undefined)
const error = ref('')
const collapsed = reactive(new Set<string>())
const searchKeyword = ref('')

// 实时解析：watch input 变化，300ms debounce
let debounceTimer: ReturnType<typeof setTimeout> | null = null
watch(input, () => {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    error.value = ''
    output.value = ''
    parsedData.value = undefined
    collapsed.clear()
    if (!input.value.trim()) return
    try {
      const parsed = JSON.parse(input.value)
      parsedData.value = parsed
      output.value = JSON.stringify(parsed, null, 2)
    } catch (e: any) {
      error.value = e.message
    }
  }, 300)
})

async function copyResult() {
  try {
    await navigator.clipboard.writeText(output.value)
  } catch {}
}

function clearAll() {
  input.value = ''
  output.value = ''
  parsedData.value = undefined
  error.value = ''
  collapsed.clear()
  searchKeyword.value = ''
}

function toggleCollapse(path: string) {
  if (collapsed.has(path)) {
    collapsed.delete(path)
  } else {
    collapsed.add(path)
  }
}

const MAX_STRING_LEN = 80

function truncateStr(s: string): string {
  if (s.length > MAX_STRING_LEN) return s.slice(0, MAX_STRING_LEN) + '…'
  return s
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

// 高亮搜索关键字：在已渲染的 HTML 中对文本节点内容进行标记
function highlightSearch(html: string, keyword: string): string {
  if (!keyword.trim()) return html
  const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const regex = new RegExp(`(${escaped})`, 'gi')
  return html.replace(/>([^<]+)</g, (match, textContent: string) => {
    const highlighted = textContent.replace(regex, '<mark class="jv-highlight">$1</mark>')
    return `>${highlighted}<`
  })
}

const renderedJsonRaw = computed(() => {
  if (parsedData.value === undefined) return ''
  return renderValue(parsedData.value, '$', 0)
})

const renderedJson = computed(() => {
  const raw = renderedJsonRaw.value
  if (!raw) return ''
  return highlightSearch(raw, searchKeyword.value)
})

function renderValue(val: any, path: string, indent: number): string {
  if (val === null) return `<span class="jv-null">null</span>`
  if (typeof val === 'boolean') return `<span class="jv-bool">${val}</span>`
  if (typeof val === 'number') return `<span class="jv-num">${val}</span>`
  if (typeof val === 'string') return `<span class="jv-str">"${truncateStr(escapeHtml(val))}"</span>`
  if (Array.isArray(val)) return renderArray(val, path, indent)
  if (typeof val === 'object') return renderObject(val, path, indent)
  return `<span>${String(val)}</span>`
}

function renderArray(arr: any[], path: string, indent: number): string {
  if (arr.length === 0) return '<span class="jv-bracket">[]</span>'
  const isCollapsed_ = collapsed.has(path)
  const arrow = isCollapsed_ ? '▶' : '▼'
  const count = arr.length
  const pad = '&nbsp;&nbsp;&nbsp;&nbsp;'.repeat(indent)
  const innerPad = '&nbsp;&nbsp;&nbsp;&nbsp;'.repeat(indent + 1)

  let html = `<span class="jv-toggle" data-path="${path}">${arrow}</span><span class="jv-bracket">[</span>`
  if (isCollapsed_) {
    html += `<span class="jv-bracket">...</span><span class="jv-count">${count} 项</span><span class="jv-bracket">]</span>`
  } else {
    html += '\n'
    const items = arr.map((item, i) => {
      const itemPath = `${path}[${i}]`
      return `${innerPad}${renderValue(item, itemPath, indent + 1)}${i < arr.length - 1 ? '<span class="jv-comma">,</span>' : ''}`
    })
    html += items.join('\n')
    html += `\n${pad}<span class="jv-bracket">]</span>`
  }
  return html
}

function renderObject(obj: Record<string, any>, path: string, indent: number): string {
  const keys = Object.keys(obj)
  if (keys.length === 0) return '<span class="jv-bracket">{}</span>'
  const isCollapsed_ = collapsed.has(path)
  const arrow = isCollapsed_ ? '▶' : '▼'
  const count = keys.length
  const pad = '&nbsp;&nbsp;&nbsp;&nbsp;'.repeat(indent)
  const innerPad = '&nbsp;&nbsp;&nbsp;&nbsp;'.repeat(indent + 1)

  let html = `<span class="jv-toggle" data-path="${path}">${arrow}</span><span class="jv-bracket">{</span>`
  if (isCollapsed_) {
    html += `<span class="jv-bracket">...</span><span class="jv-count">${count} 项</span><span class="jv-bracket">}</span>`
  } else {
    html += '\n'
    const items = keys.map((key, i) => {
      const childPath = `${path}.${key}`
      return `${innerPad}<span class="jv-key">"${escapeHtml(key)}"</span>: ${renderValue(obj[key], childPath, indent + 1)}${i < keys.length - 1 ? '<span class="jv-comma">,</span>' : ''}`
    })
    html += items.join('\n')
    html += `\n${pad}<span class="jv-bracket">}</span>`
  }
  return html
}

function onTreeClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (target.classList.contains('jv-toggle')) {
    const path = target.getAttribute('data-path')
    if (path) toggleCollapse(path)
  }
}
</script>

<template>
  <div class="tool-page">
    <h2 class="page-title">JSON 格式化</h2>
    <p class="page-desc">格式化和校验 JSON 数据</p>

    <div class="editor-row">
      <div class="editor-pane editor-pane--input">
        <div class="pane-header">
          <span>输入</span>
          <div class="pane-actions">
            <button class="btn btn-sm" @click="clearAll">清空</button>
          </div>
        </div>
        <textarea
          v-model="input"
          class="code-input"
          placeholder='粘贴 JSON，例如：{"name":"AI-OS","version":"1.0"}'
          spellcheck="false"
        ></textarea>
      </div>

      <div class="editor-pane editor-pane--output">
        <div class="pane-header">
          <span>输出</span>
          <div class="pane-actions">
            <input
              v-model="searchKeyword"
              class="search-input"
              type="text"
              placeholder="搜索..."
              spellcheck="false"
            />
            <button class="btn btn-sm" @click="copyResult" :disabled="!output">复制</button>
          </div>
        </div>
        <div v-if="error" class="error-box">{{ error }}</div>
        <div
          v-else-if="parsedData !== undefined"
          class="json-tree"
          @click="onTreeClick"
          v-html="renderedJson"
        ></div>
        <div v-else class="json-tree json-tree--placeholder">格式化结果将显示在这里</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tool-page {
  /* 无 max-width 限制，填满内容区 */
}

.page-title {
  font-size: 20px;
  font-weight: 700;
  color: #1e293b;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #94a3b8;
  margin: 0 0 20px;
}

.editor-row {
  display: flex;
  gap: 0;
  align-items: stretch;
  height: calc(100% - 80px);
}

.editor-pane {
  flex: 1;
  background: #fff;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.editor-pane--input {
  border-radius: 10px 0 0 10px;
  border: 1px solid #e2e8f0;
  border-right: none;
}

.editor-pane--output {
  border-radius: 0 10px 10px 0;
  border: 1px solid #e2e8f0;
}

/* 竖线分隔 */
.editor-pane--input::after {
  content: '';
  position: absolute;
  right: 0;
  top: 0;
  bottom: 0;
  width: 1px;
  background: #e2e8f0;
}

.editor-pane--input {
  position: relative;
}

.pane-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  font-size: 12px;
  font-weight: 600;
  color: #64748b;
  flex-shrink: 0;
}

.pane-actions {
  display: flex;
  gap: 6px;
  align-items: center;
}

.code-input {
  flex: 1;
  width: 100%;
  padding: 12px;
  border: none;
  outline: none;
  resize: none;
  font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #334155;
  background: transparent;
  overflow-y: auto;
}

.json-tree {
  flex: 1;
  padding: 12px;
  font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.7;
  color: #475569;
  white-space: pre-wrap;
  word-break: break-all;
  overflow-y: auto;
}

.json-tree--placeholder {
  color: #94a3b8;
}

.error-box {
  flex: 1;
  padding: 12px;
  color: #dc2626;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  background: #fef2f2;
  overflow-y: auto;
}

.search-input {
  padding: 3px 8px;
  font-size: 11px;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  outline: none;
  background: #fff;
  color: #334155;
  width: 120px;
  transition: border-color 0.15s;
}

.search-input:focus {
  border-color: #6366f1;
}

.search-input::placeholder {
  color: #94a3b8;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  border: 1px solid transparent;
  cursor: pointer;
  transition: all 0.15s;
  white-space: nowrap;
}

.btn-sm {
  padding: 3px 10px;
  font-size: 11px;
  background: #f1f5f9;
  border-color: #e2e8f0;
  color: #64748b;
}

.btn-sm:hover {
  background: #e2e8f0;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* JSON 树语法高亮 */
:deep(.jv-key) {
  color: #2563eb;
}

:deep(.jv-str) {
  color: #16a34a;
}

:deep(.jv-num) {
  color: #ea580c;
}

:deep(.jv-bool) {
  color: #7c3aed;
}

:deep(.jv-null) {
  color: #94a3b8;
}

:deep(.jv-bracket) {
  color: #64748b;
}

:deep(.jv-comma) {
  color: #64748b;
}

:deep(.jv-toggle) {
  display: inline-block;
  width: 16px;
  font-size: 10px;
  color: #94a3b8;
  cursor: pointer;
  user-select: none;
  transition: color 0.15s;
}

:deep(.jv-toggle:hover) {
  color: #6366f1;
}

:deep(.jv-count) {
  color: #94a3b8;
  font-size: 12px;
  margin-left: 2px;
}

:deep(.jv-highlight) {
  background: #fde68a;
  color: #1e293b;
  border-radius: 2px;
  padding: 0 1px;
}
</style>
