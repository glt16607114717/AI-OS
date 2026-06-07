<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { marked } from 'marked'

const agentRequest = window.aiOS.agentRequest

// 配置 marked
marked.use({ breaks: true, gfm: true })

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
}

const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const chatContainer = ref<HTMLDivElement | null>(null)
const currentModel = ref('')
const CACHE_KEY = 'ai-os-chat-messages'
let abortController: AbortController | null = null

// 缓存管理
function saveToCache() {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(messages.value))
  } catch {}
}

function loadFromCache(): Message[] {
  try {
    const cached = localStorage.getItem(CACHE_KEY)
    if (cached) {
      return JSON.parse(cached)
    }
  } catch {}
  return []
}

function clearCache() {
  try {
    localStorage.removeItem(CACHE_KEY)
  } catch {}
}

async function loadFromBackend() {
  try {
    const res = await agentRequest('chat_get_history', {})
    if (res?.messages) {
      messages.value = res.messages.map((m: any) => ({
        role: m.role,
        content: m.content,
        done: m.role === 'assistant'
      }))
      saveToCache()
    }
  } catch {}
}

async function saveMessage(role: 'user' | 'assistant', content: string) {
  try {
    await agentRequest('chat_add_message', { role, content })
  } catch {}
}

async function loadModel() {
  try {
    const res = await agentRequest('llm_get_strategies', {})
    const strategies = res?.strategies || []
    const active = strategies.find((s: any) => s.active)
    if (active?.options?.length) {
      currentModel.value = active.options[0].model_id || ''
    }
  } catch {}
}

function scrollToBottom() {
  nextTick(() => {
    if (chatContainer.value) {
      chatContainer.value.scrollTop = chatContainer.value.scrollHeight
    }
  })
}

function renderMarkdown(text: string, done: boolean, msgIndex: number): string {
  if (!text) return ''

  if (done) {
    try {
      return marked.parse(text) as string
    } catch {
      return escapeHtml(text)
    }
  }

  try {
    const html = marked.parse(text) as string
    return html + '<span class="cursor">|</span>'
  } catch {
    return escapeHtml(text) + '<span class="cursor">|</span>'
  }
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

  const aiMsg: Message = { role: 'assistant', content: '', done: false, toolCalls: [] }
  messages.value.push(aiMsg)
  scrollToBottom()

  abortController = new AbortController()

  try {
    // 使用独立的工作台接口（不走代理）
    const response = await fetch('http://127.0.0.1:18731/api/workspace/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: abortController.signal,
      body: JSON.stringify({
        messages: messages.value.slice(0, -1).map(m => ({
          role: m.role,
          content: m.content
        })),
        stream: true
      })
    })

    if (!response.ok) {
      const errText = await response.text()
      aiMsg.content = `请求失败: ${response.status} ${errText}`
      aiMsg.done = true
      loading.value = false
      return
    }

    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let sseBuffer = ''
    let contentBuffer = ''

    // 100ms 节流渲染
    const flushTimer = setInterval(() => {
      if (contentBuffer) {
        aiMsg.content += contentBuffer
        contentBuffer = ''
        scrollToBottom()
      }
    }, 100)

    while (true) {
      const { done: streamDone, value } = await reader.read()
      if (streamDone) break

      sseBuffer += decoder.decode(value, { stream: true })
      const lines = sseBuffer.split('\n')
      sseBuffer = lines.pop() || ''

      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed || !trimmed.startsWith('data: ')) continue
        const data = trimmed.slice(6)
        if (data === '[DONE]') continue

        try {
          const json = JSON.parse(data)

          // 错误
          if (json.error) {
            console.error('[Chat] Server error:', json.error)
            contentBuffer += `\n\n[错误] ${json.error}`
            continue
          }

          // 工具调用开始
          if (json.tool_call) {
            console.log('[Chat] Tool call:', json.tool_call.name)
            aiMsg.toolCalls!.push({
              name: json.tool_call.name,
              arguments: json.tool_call.arguments,
            })
            scrollToBottom()
            continue
          }

          // 工具调用完成
          if (json.tool_result) {
            const lastTool = aiMsg.toolCalls![aiMsg.toolCalls!.length - 1]
            if (lastTool && lastTool.name === json.tool_result.name) {
              lastTool.result = json.tool_result.result_preview
            }
            scrollToBottom()
            continue
          }

          // 正常内容
          const content = json.choices?.[0]?.delta?.content
          if (content) {
            contentBuffer += content
          }
        } catch (e) {
          console.error('[Chat] Parse error:', e, 'data:', data)
        }
      }
    }

    // 刷出剩余内容
    clearInterval(flushTimer)
    if (contentBuffer) {
      aiMsg.content += contentBuffer
    }
    aiMsg.done = true
    if (aiMsg.toolCalls?.length === 0) {
      delete aiMsg.toolCalls
    }
    console.log('[Chat] AI response done, length:', aiMsg.content.length)
    saveToCache()
    saveMessage('assistant', aiMsg.content)
    scrollToBottom()
  } catch (e: any) {
    if (e.name === 'AbortError') {
      aiMsg.content += '\n\n[已中断]'
    } else {
      // 保留已有内容，只在末尾追加错误信息
      const errInfo = `\n\n[错误] ${e.message || '连接失败'}`
      aiMsg.content += errInfo
    }
    aiMsg.done = true
    saveToCache()
    saveMessage('assistant', aiMsg.content)
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

function clearChat() {
  if (messages.value.length === 0) return
  if (confirm('确定清空所有对话记录？')) {
    messages.value = []
    clearCache()
    agentRequest('chat_clear_history', {})
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
  const cached = loadFromCache()
  if (cached.length > 0) {
    messages.value = cached
  } else {
    await loadFromBackend()
  }
  scrollToBottom()
})
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
          <!-- 工具调用过程 -->
          <div v-if="msg.toolCalls && msg.toolCalls.length" class="tool-calls">
            <div v-for="(tc, ti) in msg.toolCalls" :key="ti" class="tool-call-item">
              <div class="tool-call-header">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
                </svg>
                <span class="tool-name">{{ getToolDisplayName(tc.name) }}</span>
                <span v-if="!tc.result" class="tool-running">执行中...</span>
                <span v-else class="tool-done">完成</span>
              </div>
            </div>
          </div>
          <!-- AI 回复内容 -->
          <div v-if="msg.content" class="ai-text" v-html="renderMarkdown(msg.content, msg.done, i)"></div>
        </div>
      </div>
    </div>

    <div class="chat-input-area">
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
  border-top: 1px solid #e5e7eb;
  align-items: flex-end;
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

/* 工具调用 */
.tool-calls {
  margin-bottom: 6px;
}

.tool-call-item {
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: 4px;
  padding: 4px 8px;
  margin-bottom: 3px;
  font-size: 12px;
}

.tool-call-header {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #92400e;
}

.tool-name {
  font-weight: 600;
}

.tool-running {
  color: #f59e0b;
  font-size: 11px;
}

.tool-done {
  color: #10b981;
  font-size: 11px;
  font-weight: 500;
}
</style>
