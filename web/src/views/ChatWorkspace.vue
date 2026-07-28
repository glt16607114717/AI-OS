<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { marked } from 'marked'
import * as echarts from 'echarts'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../api'

// 声明组件名供 keep-alive include 匹配（工作台需要保活：切走时 SSE 流不中断）
defineOptions({ name: 'ChatWorkspace' })

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

// 自定义 image 渲染：给 AI 生成的图片加下载按钮（点击图片放大、点按钮下载）
// 原生 marked 只输出 <img>，这里包一层容器并加交互按钮
renderer.image = function ({ href, title, text }: { href: string; title?: string | null; text: string }) {
  if (!href) return text || ''
  const safeHref = href.replace(/"/g, '&quot;')
  const safeAlt = (text || '').replace(/"/g, '&quot;')
  return `<div class="ai-image-wrap"><img src="${safeHref}" alt="${safeAlt}" class="ai-gen-image" data-preview="${safeHref}" title="点击放大" /><button class="ai-image-download-btn" data-download="${safeHref}" title="下载图片">下载</button></div>`
}

marked.use({ renderer })

interface ToolCall {
  name: string
  arguments: string
  result?: string
}

interface Message {
  id?: number
  role: 'user' | 'assistant' | 'system'
  content: string
  images?: string[] // base64 格式的图片（仅 user 消息）
  done?: boolean
  toolCalls?: ToolCall[]
  references?: { index: number; source: string; similarity: number; text: string }[]
  progress?: string[] // 技能执行进度提示（生图等慢操作的实时反馈）
}

const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const pendingImages = ref<string[]>([]) // 待发送的图片（base64）
const chatContainer = ref<HTMLDivElement | null>(null)
const currentModel = ref('')
const CACHE_KEY = 'ai-os-chat-messages'
const SESSION_ID_KEY = 'ai-os-chat-session-id'
let abortController: AbortController | null = null

// 工作台会话 ID（localStorage 持久化，清空对话时重置）
// 用于后端对话记录按会话聚合，等同于 ZCode/Trae 的 session_id
function getSessionId(): string {
  let sid = localStorage.getItem(SESSION_ID_KEY)
  if (!sid) {
    sid = 'ws_' + (crypto.randomUUID ? crypto.randomUUID() : Date.now().toString(36) + Math.random().toString(36).slice(2))
    localStorage.setItem(SESSION_ID_KEY, sid)
  }
  return sid
}

function resetSessionId() {
  localStorage.removeItem(SESSION_ID_KEY)
}

// 每次发消息生成唯一 msg_id（后端按 msg_id 聚合同一轮对话的所有请求）
function genMsgId(): string {
  return 'ws_' + (crypto.randomUUID ? crypto.randomUUID() : Date.now().toString(36) + Math.random().toString(36).slice(2))
}


// 缓存管理：localStorage 最多存 50 条消息，超过的丢弃更早的（避免容量溢出）
// 注意：前端 messages.value 仍保留完整历史（供显示），这里只是限制持久化数量
const CACHE_MAX_MESSAGES = 50
function saveToCache() {
  try {
    const toSave = messages.value.length > CACHE_MAX_MESSAGES
      ? messages.value.slice(messages.value.length - CACHE_MAX_MESSAGES)
      : messages.value
    localStorage.setItem(CACHE_KEY, JSON.stringify(toSave))
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
        id: m.id,
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
    // 延迟二次滚动：markdown 渲染可能导致 DOM 高度变化，确保滚到底
    setTimeout(() => {
      if (chatContainer.value) {
        chatContainer.value.scrollTop = chatContainer.value.scrollHeight
      }
    }, 100)
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

// ── 图片处理 ──
const MAX_IMAGE_SIZE = 4 * 1024 * 1024 // 4MB 上限
const fileInput = ref<HTMLInputElement | null>(null)

// 文件转 base64
function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as string)
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

// 压缩图片（超过 1MB 时按比例缩放）
async function compressImage(base64: string): Promise<string> {
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      let { width, height } = img
      const maxDim = 1568 // 智谱推荐的最大边长
      if (width > maxDim || height > maxDim) {
        const ratio = maxDim / Math.max(width, height)
        width = Math.round(width * ratio)
        height = Math.round(height * ratio)
      }
      const canvas = document.createElement('canvas')
      canvas.width = width
      canvas.height = height
      const ctx = canvas.getContext('2d')!
      ctx.drawImage(img, 0, 0, width, height)
      resolve(canvas.toDataURL('image/jpeg', 0.85))
    }
    img.onerror = () => resolve(base64) // 压缩失败用原图
    img.src = base64
  })
}

// 处理选择的图片文件
async function handleImageFiles(files: FileList | File[]) {
  for (const file of Array.from(files)) {
    if (!file.type.startsWith('image/')) continue
    if (file.size > MAX_IMAGE_SIZE) {
      alert(`图片 ${file.name} 超过 4MB 限制`)
      continue
    }
    let base64 = await fileToBase64(file)
    base64 = await compressImage(base64)
    pendingImages.value.push(base64)
  }
}

// 点击上传按钮
function triggerFileInput() {
  fileInput.value?.click()
}

// 文件选择回调
async function onFileChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files) await handleImageFiles(target.files)
  target.value = '' // 清空，允许重复选同一文件
}

// 粘贴图片
async function onPaste(e: ClipboardEvent) {
  if (!e.clipboardData) return
  const items = e.clipboardData.items
  for (const item of Array.from(items)) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) await handleImageFiles([file])
      e.preventDefault()
    }
  }
}

// 拖拽图片
async function onDrop(e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer?.files) await handleImageFiles(e.dataTransfer.files)
}

function onDragOver(e: DragEvent) {
  e.preventDefault()
}

// 移除待发送图片
function removeImage(idx: number) {
  pendingImages.value.splice(idx, 1)
}

// 点击图片预览（新窗口打开原图）
function previewImage(src: string) {
  const w = window.open()
  if (w) w.document.write(`<img src="${src}" style="max-width:100%" />`)
}

async function sendMessage() {
  const text = input.value.trim()
  const imgs = [...pendingImages.value]
  // 有图片或文字都可以发送
  if ((!text && imgs.length === 0) || loading.value) return

  if (!currentModel.value) {
    alert('模型未加载，请先配置策略')
    return
  }

  console.log('[Chat] Sending message:', text, `images: ${imgs.length}`)

  // 保存用户消息（带图片）
  messages.value.push({ role: 'user', content: text, done: true, images: imgs.length > 0 ? imgs : undefined })
  input.value = ''
  pendingImages.value = [] // 清空待发送图片
  loading.value = true
  saveToCache()
  saveMessage('user', text)

  // push 后通过 messages.value[idx] 访问响应式 Proxy，才能触发 Vue 重渲染
  messages.value.push({ role: 'assistant', content: '', done: false, toolCalls: [] })
  const aiIdx = messages.value.length - 1
  scrollToBottom()

  abortController = new AbortController()

  // 历史压缩策略（三段式）：
  //   第 1~10 轮（最新）：原样保留，不做任何改动
  //   第 11~20 轮：tool 结果由后端 OptimizeMessages 截断（已有逻辑，阈值 10 轮）
  //   第 20 轮之前：直接丢弃，不发给后端（前端仍显示完整历史）
  // 一轮 = 1 个 user + 1 个 assistant = 2 条消息，20 轮 = 40 条
  const MAX_ROUNDS = 20
  const MAX_MESSAGES = MAX_ROUNDS * 2
  const allMessages = messages.value.slice(0, aiIdx)
  const recentMessages = allMessages.length > MAX_MESSAGES
    ? allMessages.slice(allMessages.length - MAX_MESSAGES)
    : allMessages

  const reqMessages = recentMessages.map(m => {
    if (m.role === 'user' && m.images && m.images.length > 0) {
      const content: any[] = []
      if (m.content) content.push({ type: 'text', text: m.content })
      for (const img of m.images) {
        content.push({ type: 'image_url', image_url: { url: img } })
      }
      return { role: m.role, content }
    }
    return { role: m.role, content: m.content }
  })

  // 注入 trace 标记到最后一条 user 消息（供后端对话记录聚合）
  // 后端 extractAndStripTrace 会解析并剥离这些标记，写入 sys_llm_stats
  const sessionId = getSessionId()
  const msgId = genMsgId()
  const traceMarker = `\n[TRACE:session=${sessionId}][TRACE:msg=${msgId}]`
  if (reqMessages.length > 0) {
    const lastMsg = reqMessages[reqMessages.length - 1]
    if (typeof lastMsg.content === 'string') {
      lastMsg.content = lastMsg.content + traceMarker
    } else if (Array.isArray(lastMsg.content)) {
      // 多模态格式：追加到第一个 text 项，或新增 text 项
      const textItem = lastMsg.content.find((c: any) => c.type === 'text')
      if (textItem) {
        textItem.text = textItem.text + traceMarker
      } else {
        lastMsg.content.push({ type: 'text', text: traceMarker })
      }
    }
  }

  try {
    // 使用独立的工作台接口（不走代理）
    const response = await fetch(`${API_BASE}/api/workspace/chat`, {
      method: 'POST',
      headers: authHeaders(true),
      signal: abortController.signal,
      body: JSON.stringify({
        messages: reqMessages,
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

    await consumeSSE(response, aiIdx)
  } catch (e: any) {
    const msg = messages.value[aiIdx]
    if (e.name === 'AbortError') {
      msg.content += '\n\n[已中断]'
      msg.done = true
      saveToCache()
      saveMessage('assistant', msg.content)
    } else {
      // 网络中断：自动重连一次
      console.log('[Chat] 网络中断，1.5 秒后自动重连...', e.message)
      // 标记重连状态（前端展示）
      msg.content = '> 🔄 网络中断，正在重连...'
      msg.done = false
      scheduleScrollGlobal()
      // 延迟重连，避免立刻重试又失败
      await new Promise(r => setTimeout(r, 1500))
      try {
        // 重新发起请求（复用同样的 reqMessages 和 aiIdx）
        abortController = new AbortController()
        const retryResp = await fetch(`${API_BASE}/api/workspace/chat`, {
          method: 'POST',
          headers: authHeaders(true),
          signal: abortController.signal,
          body: JSON.stringify({ messages: reqMessages, stream: true })
        })
        // 清空"正在重连"提示，准备接收新内容
        msg.content = ''
        msg.toolCalls = []
        await consumeSSE(retryResp, aiIdx)
      } catch (e2: any) {
        // 重连也失败
        const partialLen = msg.content.length
        if (partialLen > 0) {
          msg.content += '\n\n> ⚠️ 网络中断，以上为部分回答'
        } else {
          msg.content = `\n\n[错误] 重连失败：${e2.message || '连接失败'}`
        }
        msg.done = true
        saveToCache()
        saveMessage('assistant', msg.content)
      }
    }
  }

  abortController = null
  loading.value = false
  scrollToBottom()
}

// consumeSSE 消费 SSE 流，解析内容/工具调用/进度，写入 messages.value[aiIdx]
// 抽成独立函数，主请求和网络中断重连都复用
async function consumeSSE(response: Response, aiIdx: number) {
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

      // 忽略 SSE 注释行（keepalive 心跳）
      if (trimmed.startsWith(':')) continue

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

      // 进度事件（工具调用过程提示）-- 不写入 content，单独展示
      // 避免进度信息的换行符污染最终回答
      if (currentEventType === 'progress') {
        try {
          const json = JSON.parse(data)
          if (json.content) {
            const msg = messages.value[aiIdx]
            if (!msg.progress) msg.progress = []
            msg.progress.push(json.content)
            scheduleScroll()
          }
        } catch (e) {
          console.error('[Chat] Progress parse error:', e)
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
}

// scheduleScrollGlobal 全局滚动节流（catch 块里重连时用）
let _renderScheduled = false
function scheduleScrollGlobal() {
  if (_renderScheduled) return
  _renderScheduled = true
  requestAnimationFrame(() => {
    _renderScheduled = false
    scrollToBottom()
  })
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
    resetSessionId() // 清空对话时重置会话 ID，后续消息归到新会话
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

async function downloadLog(id: number) {
  try {
    const res = await fetch(`${API_BASE}/api/chat/download/${id}`, { headers: authHeaders() })
    if (!res.ok) {
      const err = await res.json()
      ElMessage.error(err.error || '下载失败')
      return
    }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `conversation_${id}.json`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    ElMessage.error('下载失败: ' + (e as Error).message)
  }
}

// 下载 AI 生成的图片（走后端代理，绕开跨域）
async function downloadImage(url: string) {
  try {
    const res = await fetch(`${API_BASE}/api/image/download?url=${encodeURIComponent(url)}`, { headers: authHeaders() })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'HTTP ' + res.status }))
      ElMessage.error(err.error || '下载失败')
      return
    }
    const blob = await res.blob()
    const blobUrl = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = `aios_image_${Date.now()}.png`
    a.click()
    URL.revokeObjectURL(blobUrl)
  } catch (e) {
    ElMessage.error('图片下载失败: ' + (e as Error).message)
  }
}

// 统一的图片交互事件委托（点击图片放大、点击下载按钮下载）
// 因为图片是 v-html 渲染的，无法直接 @click，用事件委托挂在容器上
function handleImageClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  // 点下载按钮
  const downloadBtn = target.closest('.ai-image-download-btn') as HTMLElement
  if (downloadBtn) {
    e.preventDefault()
    e.stopPropagation()
    const url = downloadBtn.dataset.download
    if (url) downloadImage(url)
    return
  }
  // 点图片 → 放大预览
  const img = target.closest('.ai-gen-image') as HTMLElement
  if (img) {
    const url = img.dataset.preview || (img as HTMLImageElement).src
    if (url) previewImage(url)
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
        <div class="message-role">
          {{ msg.role === 'user' ? '你' : 'AI' }}
          <button v-if="msg.role === 'assistant' && msg.id" class="download-log-btn" @click="downloadLog(msg.id)" title="下载请求日志">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="7 10 12 15 17 10" />
              <line x1="12" y1="15" x2="12" y2="3" />
            </svg>
          </button>
        </div>
        <div class="message-content">
          <!-- 用户发送的图片 -->
          <div v-if="msg.role === 'user' && msg.images && msg.images.length" class="user-images">
            <img v-for="(img, idx) in msg.images" :key="idx" :src="img" class="user-image" @click="previewImage(img)" />
          </div>
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
          <!-- 技能执行进度（生图等慢操作的实时反馈，来自后端 event:progress） -->
          <div v-if="msg.progress && msg.progress.length && !msg.done" class="skill-progress">
            <div v-for="(p, pidx) in msg.progress" :key="pidx" class="progress-item">
              <span class="progress-spinner"></span>{{ p }}
            </div>
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
          <div v-if="msg.content" class="ai-text" @click="handleImageClick" v-html="renderMarkdown(msg.content, msg.done ?? false, i)"></div>
        </div>
      </div>
    </div>

    <div class="chat-input-area" @drop="onDrop" @dragover="onDragOver">
      <!-- 隐藏的文件选择 -->
      <input type="file" ref="fileInput" accept="image/*" multiple style="display:none" @change="onFileChange" />
      
      <!-- 待发送图片预览 -->
      <div v-if="pendingImages.length > 0" class="pending-images">
        <div v-for="(img, idx) in pendingImages" :key="idx" class="pending-image-item">
          <img :src="img" />
          <button class="remove-image-btn" @click="removeImage(idx)" title="移除">&times;</button>
        </div>
      </div>
      
      <div class="input-actions">
        <textarea
          v-model="input"
          placeholder="输入消息... (Enter 发送，Shift+Enter 换行，可粘贴/拖拽图片)"
          @keydown="handleKeydown"
          @paste="onPaste"
          :disabled="loading"
          rows="5"
        ></textarea>
        <button class="upload-btn" @click="triggerFileInput" :disabled="loading" title="上传图片">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
            <circle cx="8.5" cy="8.5" r="1.5" />
            <polyline points="21 15 16 10 5 21" />
          </svg>
        </button>
        <button v-if="!loading" class="send-btn" @click="sendMessage" :disabled="!input.trim() && pendingImages.length === 0">
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
  </div>
</template>

<style scoped>
.chat-workspace {
  display: flex;
  flex-direction: column;
  height: 100%;
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
  display: flex;
  align-items: center;
  gap: 4px;
}

.download-log-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: #9ca3af;
  padding: 2px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  transition: color 0.15s, background 0.15s;
}

.download-log-btn:hover {
  color: #3b82f6;
  background: #eff6ff;
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
  flex-direction: column;
  gap: 8px;
  padding-top: 8px;
  padding-bottom: 8px;
  border-top: 1px solid #e5e7eb;
  border-bottom: 1px solid #e5e7eb;
}

.chat-input-area > .input-row {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}

.pending-images {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 4px 0;
}

.pending-image-item {
  position: relative;
  width: 64px;
  height: 64px;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid #e5e7eb;
}

.pending-image-item img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.remove-image-btn {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: 50%;
  background: rgba(0,0,0,0.6);
  color: #fff;
  font-size: 14px;
  line-height: 1;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
}

.remove-image-btn:hover {
  background: rgba(0,0,0,0.8);
}

/* textarea + 按钮行 */
.chat-input-area > textarea,
.chat-input-area > .input-row > textarea {
  flex: 1;
}

.upload-btn {
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
  flex-shrink: 0;
  transition: all 0.2s;
}

.upload-btn:hover:not(:disabled) {
  border-color: #6366f1;
  color: #6366f1;
}

.upload-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 用户消息里的图片 */
.user-images {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}

.user-image {
  max-width: 200px;
  max-height: 200px;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid #e5e7eb;
  transition: transform 0.2s;
}

.user-image:hover {
  transform: scale(1.02);
}

/* 输入区底部按钮行（textarea + 上传 + 发送） */
.input-actions {
  display: flex;
  gap: 8px;
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

/* AI 生成图片容器（带下载按钮） */
.ai-image-wrap {
  position: relative;
  display: inline-block;
  max-width: 100%;
  margin: 0.5rem 0;
}

.ai-image-wrap .ai-gen-image {
  max-width: 100%;
  max-height: 500px;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
  cursor: pointer;
  display: block;
  transition: opacity 0.15s;
}

.ai-image-wrap .ai-gen-image:hover {
  opacity: 0.92;
}

.ai-image-download-btn {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 4px 12px;
  font-size: 12px;
  color: #fff;
  background: rgba(0, 0, 0, 0.6);
  border: none;
  border-radius: 4px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.2s;
}

.ai-image-wrap:hover .ai-image-download-btn {
  opacity: 1;
}

.ai-image-download-btn:hover {
  background: rgba(0, 0, 0, 0.8);
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

/* 技能执行进度（生图等待期间的实时反馈） */
.skill-progress {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 6px;
}

.skill-progress .progress-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  font-size: 12px;
  color: #6366f1;
  background: #eef2ff;
  border-radius: 4px;
}

.skill-progress .progress-spinner {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 2px solid #c7d2fe;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
  flex-shrink: 0;
}

@keyframes spin {
  to { transform: rotate(360deg); }
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
