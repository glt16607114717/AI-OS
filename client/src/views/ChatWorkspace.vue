<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { marked } from 'marked'
import * as echarts from 'echarts'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../api'

// 统一获取鉴权请求头（Token 从 localStorage 读取）
function authHeaders(json = false): Record<string, string> {
  const headers: Record<string, string> = {}
  const token = localStorage.getItem('aios_token')
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (json) headers['Content-Type'] = 'application/json'
  return headers
}

// 配置 marked
marked.use({ breaks: true, gfm: true })

// ECharts 实例管理（防止重复初始化）
const chartInstances = new Map<HTMLElement, echarts.ECharts>()

// 流式渲染状态：控制是否渲染 ECharts（仅在消息完成后渲染）
let _renderingDone = true

// 自定义 renderer：检测 ECharts 配置并渲染
const renderer = new marked.Renderer()
const originalCode = renderer.code.bind(renderer)

renderer.code = function ({ text, lang }: { text: string; lang?: string }) {
  // 流式过程中不渲染 ECharts，只显示代码块
  if (!_renderingDone) {
    return originalCode({ type: 'code', raw: text, text, lang })
  }

  // 仅在消息完成后检测 ECharts
  const trimmed = text.trim()
  const stripped = trimmed.replace(/^\s*\/\/.*$/m, '').trim()
  const isEchartsConfig = (
    (/^(const\s+|let\s+|var\s+)?option\s*=\s*\{/.test(stripped)) &&
    stripped.includes('series')
  ) || (
    lang === 'echarts' || lang === 'chart'
  )

  if (isEchartsConfig) {
    let jsonStr = stripped
      .replace(/^(const\s+|let\s+|var\s+)?option\s*=\s*/, '')
      .replace(/;?\s*$/, '')

    const chartId = 'echarts-' + Math.random().toString(36).slice(2, 10)
    return `<div class="echarts-chart" id="${chartId}" style="width:100%;height:380px;margin:8px 0;border:1px solid #e5e7eb;border-radius:6px;overflow:hidden;"><script type="text/template">${jsonStr}<\/script></div>`
  }

  return originalCode({ type: 'code', raw: text, text, lang })
}

marked.use({ renderer })

interface ToolCall {
  name: string
  arguments: string
  result?: string
}

interface Message {
  role: 'user' | 'assistant' | 'system'
  content: string
  done?: boolean
  toolCalls?: ToolCall[]
  references?: { index: number; source: string; similarity: number; text: string }[]
}

const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const chatContainer = ref<HTMLDivElement | null>(null)
const currentModel = ref('')
const skills = ref<{id: string, name: string, description: string, example_queries: string[]}[]>([])
const showSkills = ref(false)
const CACHE_KEY = 'ai-os-chat-messages'
let abortController: AbortController | null = null

// 加载技能列表
async function loadSkills() {
  try {
    const res = await fetch(`${API_BASE}/api/skills`, { headers: authHeaders() })
    const json = await res.json()
    if (!json.ok) return
    const list = json.data || []
    if (Array.isArray(list)) {
      skills.value = list
    }
  } catch (e) {
    console.error('[ChatWorkspace] loadSkills error:', e)
    ElMessage.error('加载技能列表失败: ' + (e as Error).message)
  }
}

function useSkillQuery(query: string) {
  input.value = query
  showSkills.value = false
}

// 缓存管理
function saveToCache() {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(messages.value))
  } catch (e) {
    console.error('[ChatWorkspace] saveToCache error:', e)
  }
}

function loadFromCache(): Message[] {
  try {
    const cached = localStorage.getItem(CACHE_KEY)
    if (cached) {
      return JSON.parse(cached)
    }
  } catch (e: any) {
    console.error('[ChatWorkspace] loadFromCache error:', e)
    ElMessage.error('读取对话缓存失败: ' + (e?.message || '未知错误'))
  }
  return []
}

function clearCache() {
  try {
    localStorage.removeItem(CACHE_KEY)
  } catch (e) {
    console.error('[ChatWorkspace] clearCache error:', e)
  }
}

async function loadFromBackend() {
  try {
    const res = await fetch(`${API_BASE}/api/chat/history?page=1&page_size=20`, { headers: authHeaders() })
    const json = await res.json()
    if (!json.ok) return
    const list = json.data?.list || json.data || []
    if (Array.isArray(list) && list.length > 0) {
      messages.value = list.map((m: any) => ({
        role: m.role,
        content: m.content,
        done: m.role === 'assistant'
      }))
      saveToCache()
    }
  } catch (e) {
    console.error('[ChatWorkspace] loadFromBackend error:', e)
    ElMessage.error('加载聊天记录失败: ' + (e as Error).message)
  }
}

// chat_add_message 是本地操作：消息已通过 messages.value 维护，无需调用后端
async function saveMessage(_role: 'user' | 'assistant', _content: string) {
  // no-op
}

async function loadModel() {
  try {
    const res = await fetch(`${API_BASE}/api/llm/strategies`, { headers: authHeaders() })
    const json = await res.json()
    if (!json.ok) return
    const strategies = json.data || []
    const active = strategies.find((s: any) => s.active)
    if (active?.options?.length) {
      currentModel.value = active.options[0].model_id || ''
    }
  } catch (e) {
    console.error('[ChatWorkspace] loadModel error:', e)
    ElMessage.error('加载模型信息失败: ' + (e as Error).message)
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (chatContainer.value) {
      chatContainer.value.scrollTop = chatContainer.value.scrollHeight
    }
  })
}

// 初始化页面中的 ECharts 图表
function initCharts() {
  nextTick(() => {
    const containers = document.querySelectorAll('.echarts-chart:not([data-initialized])')
    containers.forEach((el) => {
      const htmlEl = el as HTMLElement
      const scriptEl = htmlEl.querySelector('script[type="text/template"]')
      if (!scriptEl) return
      const optionsStr = scriptEl.textContent
      if (!optionsStr) return

      try {
        // 用 Function 解析 JS 对象字面量，传入 echarts 供配置中引用
        const options = new Function('echarts', 'return ' + optionsStr)(echarts)
        const instance = echarts.init(htmlEl)
        instance.setOption(options)
        chartInstances.set(htmlEl, instance)
        htmlEl.setAttribute('data-initialized', 'true')
        // 移除 script 标签，避免重复初始化
        scriptEl.remove()
      } catch (e) {
        console.error('[ECharts] 初始化失败:', e)
        htmlEl.innerHTML = `<div style="padding:12px;color:#ef4444;font-size:12px;">图表渲染失败: ${(e as Error).message}</div>`
      }
    })
  })
}

// 清理图表实例
function disposeCharts() {
  chartInstances.forEach((instance) => instance.dispose())
  chartInstances.clear()
}

function renderMarkdown(text: string, done: boolean, msgIndex: number): string {
  if (!text) return ''

  _renderingDone = done

  if (done) {
    try {
      return marked.parse(text) as string
    } catch {
      return escapeHtml(text)
    }
  }

  // 流式模式：自动闭合不完整的 markdown 标签
  const closed = autoCloseMarkdown(text)
  try {
    return (marked.parse(closed) as string) + '<span class="cursor">|</span>'
  } catch {
    return escapeHtml(text) + '<span class="cursor">|</span>'
  }
}

// 自动闭合不完整的 Markdown 结构，让 marked 能正确解析
function autoCloseMarkdown(text: string): string {
  let result = text

  // 1. 闭合未完成的代码块（``` 开了没闭）
  const fenceCount = (result.match(/^```/gm) || []).length
  if (fenceCount % 2 !== 0) {
    result += '\n```'
  }

  // 2. 闭合未完成的表格（| header | ... 没有 | 分隔行 |）
  const lines = result.split('\n')
  let inTable = false
  let tableLines: number[] = []
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (line.startsWith('|') && line.endsWith('|')) {
      inTable = true
      tableLines.push(i)
    } else if (inTable && line.match(/^\|[\s\-:|]+\|$/)) {
      // 分隔行，表格是完整的，继续
      tableLines.push(i)
    } else if (inTable) {
      // 遇到非表格行，检查表格是否完整
      // 至少需要 header + separator 两行
      const hasSeparator = tableLines.some(li => lines[li].match(/^\|[\s\-:|]+\|$/))
      if (!hasSeparator && tableLines.length >= 1) {
        // 补上分隔行
        const colCount = lines[tableLines[0]].split('|').length - 2
        const sep = '|' + Array(Math.max(colCount, 1)).fill('---').join('|') + '|'
        lines.splice(tableLines[tableLines.length - 1] + 1, 0, sep)
        result = lines.join('\n')
      }
      inTable = false
      tableLines = []
    }
  }
  // 文件末尾的表格也要检查
  if (inTable && tableLines.length >= 1) {
    const hasSeparator = tableLines.some(li => lines[li] && lines[li].match(/^\|[\s\-:|]+\|$/))
    if (!hasSeparator) {
      const colCount = lines[tableLines[0]].split('|').length - 2
      const sep = '|' + Array(Math.max(colCount, 1)).fill('---').join('|') + '|'
      result += '\n' + sep
    }
  }

  return result
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
}

function getToolDisplayName(name: string): string {
  const map: Record<string, string> = {
    'db_query': '查询数据库',
    'db_list_tables': '列出数据表',
    'db_describe_table': '查看表结构',
  }
  return map[name] || name
}

async function sendMessage() {
  const text = input.value.trim()
  if (!text || loading.value) return

  if (!currentModel.value) {
    alert('模型未加载，请先配置策略')
    return
  }

  console.log('[Chat] Sending message:', text)

  messages.value.push({ role: 'user', content: text })
  input.value = ''
  loading.value = true
  saveToCache()
  saveMessage('user', text)

  // push 后通过 messages.value[idx] 访问响应式 Proxy，才能触发 Vue 重渲染
  messages.value.push({ role: 'assistant', content: '', done: false, toolCalls: [] })
  const aiIdx = messages.value.length - 1
  scrollToBottom()

  abortController = new AbortController()

  try {
    // 使用独立的工作台接口（不走代理）
    const response = await fetch(`${API_BASE}/api/workspace/chat`, {
      method: 'POST',
      headers: authHeaders(true),
      signal: abortController.signal,
      body: JSON.stringify({
        messages: messages.value.slice(0, aiIdx).map(m => ({
          role: m.role,
          content: m.content
        })),
        stream: true
      })
    })

    if (!response.ok) {
      let errMsg = `HTTP ${response.status}`
      try {
        const errJson = JSON.parse(await response.text())
        if (errJson.error) errMsg = errJson.error
      } catch {}
      messages.value[aiIdx].content = errMsg
      messages.value[aiIdx].done = true
      loading.value = false
      return
    }

    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let sseBuffer = ''

    // 渲染节流：用 requestAnimationFrame 合并同一帧内的多次内容更新
    let renderScheduled = false
    function scheduleScroll() {
      if (renderScheduled) return
      renderScheduled = true
      requestAnimationFrame(() => {
        renderScheduled = false
        scrollToBottom()
      })
    }

    let currentEventType = 'message' // 跟踪 SSE event 类型

    while (true) {
      const { done: streamDone, value } = await reader.read()
      if (streamDone) break

      sseBuffer += decoder.decode(value, { stream: true })
      const lines = sseBuffer.split('\n')
      sseBuffer = lines.pop() || ''

      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed) continue

        // 解析 event 类型行
        if (trimmed.startsWith('event: ')) {
          currentEventType = trimmed.slice(7).trim()
          continue
        }

        if (!trimmed.startsWith('data: ')) continue
        const data = trimmed.slice(6)
        if (data === '[DONE]') continue

        // RAG 引用事件
        if (currentEventType === 'references') {
          try {
            const refs = JSON.parse(data)
            if (Array.isArray(refs) && refs.length > 0) {
              messages.value[aiIdx].references = refs
            }
          } catch (e) {
            console.error('[Chat] References parse error:', e)
          }
          currentEventType = 'message' // 重置
          continue
        }
        currentEventType = 'message' // 重置

        try {
          const json = JSON.parse(data)
          const msg = messages.value[aiIdx] // 通过响应式 Proxy 访问

          // 错误
          if (json.error) {
            console.error('[Chat] Server error:', json.error)
            msg.content += `\n\n[错误] ${json.error}`
            scheduleScroll()
            continue
          }

          // 工具调用开始（隐藏底层工具名，只计数）
          if (json.tool_call) {
            console.log('[Chat] Tool call:', json.tool_call.name)
            msg.toolCalls!.push({
              name: json.tool_call.name,
              arguments: json.tool_call.arguments,
            })
            scheduleScroll()
            continue
          }

          // 工具调用完成
          if (json.tool_result) {
            const lastTool = msg.toolCalls![msg.toolCalls!.length - 1]
            if (lastTool && lastTool.name === json.tool_result.name) {
              lastTool.result = json.tool_result.result_preview
            }
            scheduleScroll()
            continue
          }

          // 正常内容 — 直接写入响应式对象，触发 Vue 重渲染
          const content = json.choices?.[0]?.delta?.content
          if (content) {
            msg.content += content
            scheduleScroll()
          }
        } catch (e) {
          console.error('[Chat] Parse error:', e, 'data:', data)
        }
      }
    }

    // 流结束
    const finalMsg = messages.value[aiIdx]
    finalMsg.done = true
    if (finalMsg.toolCalls?.length === 0) {
      delete finalMsg.toolCalls
    }
    console.log('[Chat] AI response done, length:', finalMsg.content.length)
    saveToCache()
    saveMessage('assistant', finalMsg.content)
    scrollToBottom()
    onMessageDone()
  } catch (e: any) {
    const msg = messages.value[aiIdx]
    if (e.name === 'AbortError') {
      msg.content += '\n\n[已中断]'
    } else {
      msg.content += `\n\n[错误] ${e.message || '连接失败'}`
    }
    msg.done = true
    saveToCache()
    saveMessage('assistant', msg.content)
  }

  abortController = null
  loading.value = false
  scrollToBottom()
}

function stopChat() {
  if (abortController) {
    abortController.abort()
    abortController = null
  }
}

async function clearChat() {
  if (messages.value.length === 0) return
  if (confirm('确定清空所有对话记录？')) {
    disposeCharts()
    messages.value = []
    clearCache()
    try {
      const res = await fetch(`${API_BASE}/api/chat/clear`, {
        method: 'POST',
        headers: authHeaders(true)
      })
      const data = await res.json()
      if (!data.ok) {
        ElMessage.error(data.error || '清空失败')
      }
    } catch (e: any) {
      ElMessage.error('清空失败: ' + e.message)
    }
  }
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

onMounted(async () => {
  loadModel()
  loadSkills()
  const cached = loadFromCache()
  if (cached.length > 0) {
    messages.value = cached
  } else {
    await loadFromBackend()
  }
  scrollToBottom()
  // 窗口大小变化时重绘图表
  window.addEventListener('resize', () => {
    chartInstances.forEach((instance) => instance.resize())
  })
})

// 消息完成后渲染图表
function onMessageDone() {
  nextTick(() => initCharts())
}
</script>

<template>
  <div class="chat-workspace">
    <div class="chat-header">
      <span class="chat-title">工作台</span>
      <button class="clear-btn" @click="clearChat" :disabled="messages.length === 0">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="3 6 5 6 21 6" />
          <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
        </svg>
        清空对话
      </button>
    </div>

    <div ref="chatContainer" class="chat-messages">
      <div v-if="messages.length === 0" class="empty-hint">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3">
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
        <p>发送消息开始对话</p>
      </div>

      <div v-for="(msg, i) in messages" :key="i" class="message" :class="msg.role">
        <div class="message-role">{{ msg.role === 'user' ? '你' : 'AI' }}</div>
        <div class="message-content">
          <!-- 工具调用过程：只显示简洁状态，不暴露底层工具 -->
          <div v-if="msg.toolCalls && msg.toolCalls.length && !msg.done" class="tool-status">
            <div class="thinking-indicator">
              <span class="thinking-dot"></span>
              <span class="thinking-dot"></span>
              <span class="thinking-dot"></span>
            </div>
            <span>思考中...</span>
          </div>
          <div v-else-if="msg.toolCalls && msg.toolCalls.length && msg.done" class="tool-status done">
            已完成 {{ msg.toolCalls.length }} 次数据查询
          </div>
          <!-- RAG 知识库引用 -->
          <div v-if="msg.references && msg.references.length && msg.done" class="rag-references">
            <div class="rag-ref-header">
              <span class="rag-ref-icon">&#128218;</span>
              <span>参考了 {{ msg.references.length }} 条知识库记录</span>
            </div>
            <div v-for="ref in msg.references" :key="ref.index" class="rag-ref-item">
              <div class="rag-ref-meta">
                <span class="rag-ref-source">来源: {{ ref.source }}</span>
                <span class="rag-ref-sim">相关度: {{ ref.similarity }}%</span>
              </div>
              <div class="rag-ref-text">{{ ref.text }}</div>
            </div>
          </div>
          <!-- AI 回复内容 -->
          <div v-if="msg.content" class="ai-text" v-html="renderMarkdown(msg.content, msg.done ?? false, i)"></div>
        </div>
      </div>
    </div>

    <div class="chat-input-area">
      <!-- 技能选择器 -->
      <div class="skill-selector" v-if="skills.length > 0">
        <button class="skill-toggle-btn" @click="showSkills = !showSkills" :class="{ active: showSkills }">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2L2 7l10 5 10-5-10-5z"/>
            <path d="M2 17l10 5 10-5"/>
            <path d="M2 12l10 5 10-5"/>
          </svg>
        </button>
        <div v-if="showSkills" class="skill-dropdown">
          <div class="skill-dropdown-header">可用技能</div>
          <div v-for="skill in skills" :key="skill.id" class="skill-item">
            <div class="skill-item-name">{{ skill.name }}</div>
            <div class="skill-item-desc">{{ skill.description.slice(0, 60) }}...</div>
            <div class="skill-item-queries">
              <button v-for="q in skill.example_queries" :key="q" class="skill-query-btn" @click="useSkillQuery(q)">{{ q }}</button>
            </div>
          </div>
        </div>
      </div>
      <textarea
        v-model="input"
        placeholder="输入消息... (Enter 发送，Shift+Enter 换行)"
        @keydown="handleKeydown"
        :disabled="loading"
        rows="5"
      ></textarea>
      <button v-if="!loading" class="send-btn" @click="sendMessage" :disabled="!input.trim()">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="22" y1="2" x2="11" y2="13" />
          <polygon points="22 2 15 22 11 13 2 9 22 2" />
        </svg>
      </button>
      <button v-else class="stop-btn" @click="stopChat">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
          <rect x="6" y="6" width="12" height="12" rx="2" />
        </svg>
      </button>
    </div>
  </div>
</template>

<style scoped>
.chat-workspace {
  display: flex;
  flex-direction: column;
  height: 100%;
  max-height: calc(100vh - 56px);
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
  border-bottom: 1px solid #e5e7eb;
}

.chat-title {
  font-size: 18px;
  font-weight: 600;
  color: #1f2937;
}

.clear-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: #fff;
  color: #6b7280;
  cursor: pointer;
  font-size: 13px;
  transition: all 0.2s;
}

.clear-btn:hover:not(:disabled) {
  background: #f9fafb;
  border-color: #9ca3af;
}

.clear-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.chat-messages {
  flex: 1;
  overflow-y: auto;
  padding: 10px 0;
  min-height: 0;
}

.empty-hint {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #9ca3af;
}

.empty-hint p {
  margin-top: 12px;
  font-size: 14px;
}

.message {
  display: flex;
  gap: 10px;
  margin-bottom: 8px;
  padding: 8px 10px;
  border-radius: 6px;
}

.message.user {
  background: #f0f9ff;
  border: 1px solid #bae6fd;
}

.message.assistant {
  background: #f9fafb;
  border: 1px solid #e5e7eb;
}

.message-role {
  font-size: 12px;
  font-weight: 600;
  color: #6b7280;
  min-width: 20px;
  padding-top: 1px;
}

.message-content {
  flex: 1;
  font-size: 13px;
  line-height: 1.5;
  color: #374151;
  word-break: break-word;
}

.chat-input-area {
  display: flex;
  gap: 8px;
  padding-top: 8px;
  padding-bottom: 8px;
  border-top: 1px solid #e5e7eb;
  border-bottom: 1px solid #e5e7eb;
  align-items: flex-end;
}

.skill-selector {
  position: relative;
  flex-shrink: 0;
}

.skill-toggle-btn {
  width: 36px;
  height: 36px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: #fff;
  color: #6b7280;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.skill-toggle-btn:hover, .skill-toggle-btn.active {
  background: #eef2ff;
  border-color: #6366f1;
  color: #6366f1;
}

.skill-dropdown {
  position: absolute;
  bottom: 42px;
  left: 0;
  width: 320px;
  max-height: 400px;
  overflow-y: auto;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.1);
  z-index: 100;
}

.skill-dropdown-header {
  padding: 8px 12px;
  font-size: 12px;
  font-weight: 600;
  color: #6b7280;
  border-bottom: 1px solid #e5e7eb;
}

.skill-item {
  padding: 8px 12px;
  border-bottom: 1px solid #f3f4f6;
}

.skill-item:last-child {
  border-bottom: none;
}

.skill-item-name {
  font-size: 13px;
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 2px;
}

.skill-item-desc {
  font-size: 11px;
  color: #9ca3af;
  margin-bottom: 6px;
}

.skill-item-queries {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.skill-query-btn {
  font-size: 11px;
  padding: 3px 8px;
  border: 1px solid #c7d2fe;
  border-radius: 4px;
  background: #eef2ff;
  color: #4338ca;
  cursor: pointer;
  transition: all 0.15s;
}

.skill-query-btn:hover {
  background: #6366f1;
  color: #fff;
  border-color: #6366f1;
}

.chat-input-area textarea {
  flex: 1;
  resize: none;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 13px;
  font-family: inherit;
  outline: none;
  transition: border-color 0.2s;
  line-height: 1.5;
  max-height: 6.5rem;
}

.chat-input-area textarea:focus {
  border-color: #6366f1;
}

.chat-input-area textarea:disabled {
  background: #f9fafb;
  cursor: not-allowed;
}

.send-btn, .stop-btn {
  width: 36px;
  height: 36px;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
  flex-shrink: 0;
}

.send-btn {
  background: #6366f1;
  color: #fff;
}

.send-btn:hover:not(:disabled) {
  background: #4f46e5;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.stop-btn {
  background: #ef4444;
  color: #fff;
}

.stop-btn:hover {
  background: #dc2626;
}
</style>

<!-- 非 scoped：让 v-html 内容也能被样式命中 -->
<style>
.ai-text {
  margin: 0;
  line-height: 1.5;
}

/* Markdown 标题 */
.ai-text h1, .ai-text h2, .ai-text h3, .ai-text h4, .ai-text h5, .ai-text h6 {
  margin: 0.4rem 0 0.2rem;
  font-weight: 600;
  line-height: 1.3;
  color: #1f2937;
}

.ai-text h1 { font-size: 1.3rem; border-bottom: 1px solid #e5e7eb; padding-bottom: 0.2rem; }
.ai-text h2 { font-size: 1.15rem; border-bottom: 1px solid #e5e7eb; padding-bottom: 0.2rem; }
.ai-text h3 { font-size: 1.05rem; }
.ai-text h4 { font-size: 0.95rem; }

/* 段落 */
.ai-text p {
  margin: 0.2rem 0 0.4rem;
}

/* 代码块 */
.ai-text pre {
  background: #1e1e1e;
  border-radius: 6px;
  padding: 10px 12px;
  overflow-x: auto;
  margin: 0.4rem 0;
  font-size: 12px;
  line-height: 1.4;
}

.ai-text code {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 0.85em;
  background: #f3f4f6;
  padding: 0.15em 0.3em;
  border-radius: 3px;
  color: #e11d48;
}

.ai-text pre code {
  background: transparent;
  padding: 0;
  border-radius: 0;
  color: #d4d4d4;
  font-size: inherit;
}

/* 列表 */
.ai-text ul, .ai-text ol {
  margin: 0.3rem 0;
  padding-left: 1.3rem;
}

.ai-text li {
  margin-bottom: 0.15rem;
  line-height: 1.5;
}

.ai-text ul { list-style-type: disc; }
.ai-text ol { list-style-type: decimal; }
.ai-text li::marker { color: #6b7280; }

/* 引用 */
.ai-text blockquote {
  border-left: 3px solid #d1d5db;
  margin: 0.4rem 0;
  padding: 0.3rem 0.8rem;
  color: #6b7280;
  background: #f9fafb;
  border-radius: 0 4px 4px 0;
}

/* 表格 */
.ai-text table {
  border-collapse: collapse;
  width: 100%;
  margin: 0.4rem 0;
  font-size: 12px;
}

.ai-text th, .ai-text td {
  border: 1px solid #e5e7eb;
  padding: 5px 8px;
  text-align: left;
}

.ai-text th {
  background: #f9fafb;
  font-weight: 600;
  color: #1f2937;
}

.ai-text tr:nth-child(even) { background: #f9fafb; }
.ai-text tr:hover { background: #f3f4f6; }

/* 水平线 */
.ai-text hr {
  border: none;
  border-top: 1px solid #e5e7eb;
  margin: 0.6rem 0;
}

/* 链接 */
.ai-text a { color: #3b82f6; text-decoration: none; }
.ai-text a:hover { text-decoration: underline; }

/* 图片 */
.ai-text img {
  max-width: 100%;
  border-radius: 4px;
  margin: 0.3rem 0;
}

/* 流式光标 */
.cursor {
  display: inline-block;
  animation: blink 1s infinite;
  color: #3b82f6;
}

@keyframes blink {
  0%, 50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}

/* 工具调用状态 */
.tool-status {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  margin-bottom: 6px;
  font-size: 12px;
  color: #6366f1;
  background: #eef2ff;
  border-radius: 4px;
}

.tool-status.done {
  color: #6b7280;
  background: #f3f4f6;
  font-size: 11px;
}

/* RAG 知识库引用卡片 */
.rag-references {
  margin-bottom: 10px;
  border: 1px solid #e0e7ff;
  border-radius: 8px;
  background: #f5f3ff;
  overflow: hidden;
}
.rag-ref-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: #ede9fe;
  font-size: 12px;
  color: #5b21b6;
  font-weight: 500;
}
.rag-ref-icon {
  font-size: 14px;
}
.rag-ref-item {
  padding: 6px 10px;
  border-top: 1px solid #e0e7ff;
}
.rag-ref-meta {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: #7c3aed;
  margin-bottom: 3px;
}
.rag-ref-text {
  font-size: 12px;
  color: #4b5563;
  line-height: 1.5;
  max-height: 60px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
}
.rag-ref-sim {
  color: #059669;
  font-weight: 500;
}

.thinking-indicator {
  display: flex;
  gap: 3px;
}

.thinking-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: #6366f1;
  animation: thinking 1.4s infinite;
}

.thinking-dot:nth-child(2) { animation-delay: 0.2s; }
.thinking-dot:nth-child(3) { animation-delay: 0.4s; }

@keyframes thinking {
  0%, 80%, 100% { opacity: 0.3; transform: scale(0.8); }
  40% { opacity: 1; transform: scale(1.1); }
}
</style>
