<script setup lang="ts">
import { ref } from 'vue'

const input = ref('')
const output = ref('')
const error = ref('')

function formatJson() {
  error.value = ''
  if (!input.value.trim()) { output.value = ''; return }
  try {
    const parsed = JSON.parse(input.value)
    output.value = JSON.stringify(parsed, null, 2)
  } catch (e: any) {
    error.value = e.message
    output.value = ''
  }
}

function minifyJson() {
  error.value = ''
  if (!input.value.trim()) { output.value = ''; return }
  try {
    const parsed = JSON.parse(input.value)
    output.value = JSON.stringify(parsed)
  } catch (e: any) {
    error.value = e.message
    output.value = ''
  }
}

async function copyResult() {
  try {
    await navigator.clipboard.writeText(output.value)
  } catch {}
}

function clearAll() {
  input.value = ''
  output.value = ''
  error.value = ''
}
</script>

<template>
  <div class="tool-page">
    <h2 class="page-title">JSON 格式化</h2>
    <p class="page-desc">格式化、压缩或校验 JSON 数据</p>

    <div class="editor-row">
      <div class="editor-pane">
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
          @input="error = ''"
        ></textarea>
      </div>

      <div class="editor-actions">
        <button class="btn btn-primary" @click="formatJson">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg>
          格式化
        </button>
        <button class="btn btn-secondary" @click="minifyJson">压缩</button>
      </div>

      <div class="editor-pane">
        <div class="pane-header">
          <span>输出</span>
          <div class="pane-actions">
            <button class="btn btn-sm" @click="copyResult" :disabled="!output">复制</button>
          </div>
        </div>
        <div v-if="error" class="error-box">{{ error }}</div>
        <textarea
          v-else
          :value="output"
          class="code-output"
          readonly
          placeholder="格式化结果将显示在这里"
          spellcheck="false"
        ></textarea>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tool-page {
  max-width: 960px;
  margin: 0 auto;
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
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  gap: 12px;
  align-items: start;
}

.editor-pane {
  background: #fff;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
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
}

.pane-actions {
  display: flex;
  gap: 6px;
}

.code-input,
.code-output {
  width: 100%;
  min-height: 320px;
  padding: 12px;
  border: none;
  outline: none;
  resize: vertical;
  font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #334155;
  background: transparent;
}

.code-output {
  color: #475569;
}

.error-box {
  padding: 12px;
  min-height: 320px;
  color: #dc2626;
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  background: #fef2f2;
}

.editor-actions {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 44px;
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

.btn-primary {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  border: none;
}

.btn-primary:hover {
  box-shadow: 0 2px 12px rgba(99, 102, 241, 0.3);
}

.btn-secondary {
  background: #f8fafc;
  color: #6366f1;
  border-color: #c7d2fe;
}

.btn-secondary:hover {
  background: #eef2ff;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
