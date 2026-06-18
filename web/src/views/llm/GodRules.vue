<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

const enabled = ref(true)
const promptOptimize = ref(true)
const rules = ref('')
const saving = ref(false)
const loading = ref(true)
const msg = ref('')
const msgType = ref<'success' | 'error'>('success')
const mode = ref<'edit' | 'preview'>('edit')

const previewHtml = computed(() => {
  return renderMarkdown(rules.value)
})

function renderMarkdown(text: string): string {
  if (!text) return '<p class="empty-hint">暂无内容，请切换到编辑模式添加</p>'

  // 1. 转义 HTML 特殊字符（保留 Markdown 语法符号）
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  // 2. 代码块 ```...``` → <pre><code>
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
    return `<pre class="md-code-block"><code class="lang-${lang || 'text'}">${code.trim()}</code></pre>`
  })

  // 3. 水平线 --- / *** / ___
  html = html.replace(/^(---|\*\*\*|___)\s*$/gm, '<hr class="md-hr">')

  // 4. 标题 h1-h6
  html = html.replace(/^######\s+(.+)$/gm, '<h6>$1</h6>')
  html = html.replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>')
  html = html.replace(/^####\s+(.+)$/gm, '<h4>$1</h4>')
  html = html.replace(/^###\s+(.+)$/gm, '<h3>$1</h3>')
  html = html.replace(/^##\s+(.+)$/gm, '<h2>$1</h2>')
  html = html.replace(/^#\s+(.+)$/gm, '<h1>$1</h1>')

  // 5. 引用块 > text
  html = html.replace(/^&gt;\s?(.+)$/gm, '<blockquote class="md-blockquote">$1</blockquote>')
  // 合并连续引用
  html = html.replace(/<\/blockquote>\n<blockquote class="md-blockquote">/g, '<br>')

  // 6. 加粗 + 斜体（顺序很重要）
  html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')

  // 7. 行内代码
  html = html.replace(/`([^`]+)`/g, '<code class="md-inline-code">$1</code>')

  // 8. 链接 [text](url)
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" class="md-link">$1</a>')

  // 9. 无序列表
  html = html.replace(/^[\-\*]\s+(.+)$/gm, '<li class="md-li">$1</li>')

  // 10. 有序列表
  html = html.replace(/^\d+\.\s+(.+)$/gm, '<li class="md-li md-li-ordered">$1</li>')

  // 11. 将连续 <li> 包裹在 <ul>
  html = html.replace(/((?:<li class="md-li[^"]*">.*?<\/li>\n?)+)/g, (match) => {
    return '<ul class="md-ul">' + match + '</ul>'
  })

  // 12. 段落（双换行 → 段落分隔）
  html = html.replace(/\n\n+/g, '</p><p>')
  // 单换行 → <br>（但不在块级元素后）
  html = html.replace(/\n/g, '<br>')

  // 13. 清理多余的 <p> 包裹块级元素
  html = html.replace(/<p>(<h[1-6]>)/g, '$1')
  html = html.replace(/(<\/h[1-6]>)<\/p>/g, '$1')
  html = html.replace(/<p>(<hr class="md-hr">)/g, '$1')
  html = html.replace(/(<hr class="md-hr">)<\/p>/g, '$1')
  html = html.replace(/<p>(<pre class="md-code-block">)/g, '$1')
  html = html.replace(/(<\/pre>)<\/p>/g, '$1')
  html = html.replace(/<p>(<ul class="md-ul">)/g, '$1')
  html = html.replace(/(<\/ul>)<\/p>/g, '$1')
  html = html.replace(/<p>(<blockquote)/g, '$1')
  html = html.replace(/(<\/blockquote>)<\/p>/g, '$1')

  return '<p>' + html + '</p>'
}

async function loadRules() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/god-rules`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      const d = result.data || {}
      enabled.value = d.enabled ?? true
      promptOptimize.value = d.prompt_optimize ?? true
      rules.value = d.rules ?? ''
    }
  } catch (e: any) {
    console.error('[GodRules] load error:', e)
    ElMessage.error('加载规则失败: ' + e.message)
  }
  loading.value = false
}

async function saveRules() {
  saving.value = true
  msg.value = ''
  try {
    const response = await fetch(`${API_BASE}/api/god-rules/save`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        enabled: enabled.value,
        rules: rules.value,
        prompt_optimize: promptOptimize.value
      }),
    })
    const result = await response.json()
    if (result.ok) {
      msg.value = '保存成功'
      msgType.value = 'success'
    } else {
      msg.value = result.error || '保存失败'
      msgType.value = 'error'
    }
  } catch (e: any) {
    msg.value = e.message || '保存失败'
    msgType.value = 'error'
  }
  saving.value = false
  setTimeout(() => { msg.value = '' }, 3000)
}

onMounted(loadRules)
</script>

<template>
  <div class="god-rules-page">
    <div class="page-header">
      <h2 class="page-title">上帝指令</h2>
      <p class="page-desc">配置注入到 system prompt 最前面的规则，提高大模型遵从度。支持 Markdown 格式。</p>
    </div>

    <div v-if="loading" class="empty-state">
      <p class="empty-text">加载中...</p>
    </div>

    <div v-else class="rules-editor">
      <!-- Toggle + Mode Tabs -->
      <div class="top-bar">
        <div class="toggles-row">
          <label class="toggle-label">
            <input type="checkbox" v-model="enabled" class="toggle-check" />
            <span class="toggle-switch"></span>
            <span class="toggle-text">{{ enabled ? '已启用' : '已关闭' }}</span>
          </label>
          <label class="toggle-label">
            <input type="checkbox" v-model="promptOptimize" class="toggle-check" />
            <span class="toggle-switch"></span>
            <span class="toggle-text">提示词优化 {{ promptOptimize ? '已启用' : '已关闭' }}</span>
          </label>
        </div>
        <div class="mode-tabs">
          <button class="mode-tab" :class="{ active: mode === 'edit' }" @click="mode = 'edit'">编辑</button>
          <button class="mode-tab" :class="{ active: mode === 'preview' }" @click="mode = 'preview'">预览</button>
        </div>
      </div>

      <!-- Editor -->
      <div v-if="mode === 'edit'" class="editor-wrap">
        <textarea
          v-model="rules"
          class="rules-textarea"
          placeholder="在此输入上帝指令，支持 Markdown 格式。&#10;&#10;示例：&#10;# 铁律规则&#10;&#10;- 回复必须使用中文&#10;- 代码注释使用英文&#10;- **不要**使用 emoji&#10;- 不要添加不必要的注释&#10;&#10;## 代码风格&#10;&#10;1. 使用 `const` 优先于 `let`&#10;2. 函数名使用 `camelCase`"
          :disabled="!enabled"
          spellcheck="false"
        ></textarea>
      </div>

      <!-- Preview -->
      <div v-else class="preview-wrap" :class="{ disabled: !enabled }">
        <div class="preview-content markdown-body" v-html="previewHtml"></div>
      </div>

      <!-- Actions -->
      <div class="actions-row">
        <button class="btn-save" @click="saveRules" :disabled="saving">
          {{ saving ? '保存中...' : '保存' }}
        </button>
        <span v-if="msg" class="msg" :class="msgType">{{ msg }}</span>
      </div>

      <!-- Tips -->
      <div class="tips-card">
        <div class="tips-title">说明</div>
        <ul class="tips-list">
          <li>上帝指令会注入到 <code>system prompt</code> 的最前面，优先级最高</li>
          <li>适合放置大模型经常违反的"铁律"，如语言要求、格式要求等</li>
          <li>建议控制在 500 字以内，太长会稀释每条规则的权重</li>
          <li>提示词优化开启后：自动将用户规则提升到 system 层级、去重重复内容、压缩工具描述</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.god-rules-page {
  padding: 24px 24px 60px;
  box-sizing: border-box;
}
.page-header {
  margin-bottom: 24px;
}
.page-title {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px 0;
}
.page-desc {
  font-size: 13px;
  color: #909399;
  margin: 0;
  line-height: 1.6;
}
.empty-state {
  text-align: center;
  padding: 60px 0;
  color: #c0c4cc;
}
.top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}
.toggles-row {
  display: flex;
  align-items: center;
  gap: 20px;
}
.toggle-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}
.toggle-check { display: none; }
.toggle-switch {
  width: 36px; height: 20px; border-radius: 10px;
  background: #dcdfe6; position: relative; transition: background 0.3s;
}
.toggle-switch::after {
  content: ''; position: absolute; top: 2px; left: 2px;
  width: 16px; height: 16px; border-radius: 50%;
  background: #fff; transition: transform 0.3s;
}
.toggle-check:checked + .toggle-switch { background: #67c23a; }
.toggle-check:checked + .toggle-switch::after { transform: translateX(16px); }
.toggle-text { font-size: 13px; color: #606266; }

.mode-tabs {
  display: flex; gap: 0; background: #f4f4f5;
  border-radius: 6px; padding: 2px;
}
.mode-tab {
  padding: 4px 14px; font-size: 12px; font-weight: 500;
  color: #909399; background: transparent; border: none;
  border-radius: 4px; cursor: pointer; transition: all 0.2s;
}
.mode-tab:hover { color: #606266; }
.mode-tab.active {
  background: #fff; color: #409eff;
  box-shadow: 0 1px 3px rgba(0,0,0,0.08);
}

.editor-wrap {
  border: 1px solid #e4e7ed; border-radius: 8px; overflow: hidden;
}
.rules-textarea {
  width: 100%; min-height: 500px; padding: 20px;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 13px; line-height: 1.8; color: #303133;
  background: #fafafa; border: none; outline: none;
  resize: vertical; box-sizing: border-box;
}
.rules-textarea:disabled { background: #f5f5f5; color: #c0c4cc; }
.rules-textarea::placeholder { color: #c0c4cc; }

.preview-wrap {
  border: 1px solid #e4e7ed; border-radius: 8px;
  background: #fff; min-height: 500px; padding: 24px;
}
.preview-wrap.disabled { background: #f5f5f5; }
.preview-content { font-size: 14px; line-height: 1.8; color: #303133; }

/* Markdown 渲染样式 */
.markdown-body :deep(p) { margin: 0 0 10px; }
.markdown-body :deep(h1) {
  font-size: 22px; font-weight: 700; margin: 20px 0 10px; color: #1a1a1a;
  border-bottom: 2px solid #e4e7ed; padding-bottom: 8px;
}
.markdown-body :deep(h2) {
  font-size: 18px; font-weight: 600; margin: 18px 0 8px; color: #303133;
  border-bottom: 1px solid #f0f0f0; padding-bottom: 6px;
}
.markdown-body :deep(h3) { font-size: 16px; font-weight: 600; margin: 14px 0 6px; color: #303133; }
.markdown-body :deep(h4) { font-size: 15px; font-weight: 600; margin: 12px 0 4px; color: #303133; }
.markdown-body :deep(h5) { font-size: 14px; font-weight: 600; margin: 10px 0 4px; color: #606266; }
.markdown-body :deep(h6) { font-size: 13px; font-weight: 600; margin: 10px 0 4px; color: #909399; }
.markdown-body :deep(strong) { font-weight: 700; color: #1a1a1a; }
.markdown-body :deep(em) { font-style: italic; color: #555; }
.markdown-body :deep(.md-inline-code) {
  padding: 2px 6px; background: #f5f5f5; border: 1px solid #e8e8e8;
  border-radius: 3px; font-size: 12px; color: #e74c3c;
  font-family: 'Cascadia Code', Consolas, monospace;
}
.markdown-body :deep(.md-code-block) {
  background: #1e1e2e; border-radius: 8px; padding: 16px;
  overflow-x: auto; margin: 12px 0;
}
.markdown-body :deep(.md-code-block code) {
  color: #cdd6f4; font-family: 'Cascadia Code', Consolas, monospace;
  font-size: 12px; line-height: 1.6;
}
.markdown-body :deep(.md-hr) {
  border: none; height: 1px; background: #e4e7ed; margin: 16px 0;
}
.markdown-body :deep(.md-blockquote) {
  border-left: 3px solid #409eff; padding: 4px 12px; margin: 10px 0;
  background: #f0f7ff; border-radius: 0 4px 4px 0;
  color: #606266; font-style: italic;
}
.markdown-body :deep(.md-ul) {
  padding-left: 22px; margin: 8px 0; list-style: disc;
}
.markdown-body :deep(.md-li) { margin: 4px 0; line-height: 1.7; }
.markdown-body :deep(.md-li-ordered) { list-style: decimal; }
.markdown-body :deep(.md-link) {
  color: #409eff; text-decoration: none; border-bottom: 1px solid #b3d8ff;
}
.markdown-body :deep(.md-link:hover) { color: #66b1ff; }
.markdown-body :deep(.empty-hint) { color: #c0c4cc; text-align: center; padding: 40px 0; }

.actions-row {
  display: flex; align-items: center; gap: 12px; margin-top: 16px;
}
.btn-save {
  padding: 8px 24px; font-size: 14px; font-weight: 500;
  color: #fff; background: #409eff; border: none;
  border-radius: 6px; cursor: pointer; transition: background 0.2s;
}
.btn-save:hover { background: #66b1ff; }
.btn-save:disabled { background: #a0cfff; cursor: not-allowed; }
.msg { font-size: 13px; }
.msg.success { color: #67c23a; }
.msg.error { color: #f56c6c; }

.tips-card {
  margin-top: 24px; padding: 16px; background: #f4f4f5; border-radius: 8px;
}
.tips-title { font-size: 13px; font-weight: 600; color: #606266; margin-bottom: 8px; }
.tips-list { margin: 0; padding-left: 18px; font-size: 12px; color: #909399; line-height: 2; }
.tips-list code {
  padding: 1px 5px; background: #e4e7ed; border-radius: 3px;
  font-size: 11px; color: #606266;
}
</style>
