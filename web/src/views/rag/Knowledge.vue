<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { API_BASE } from '../../api'
import { formatTimeShort as formatTime } from '../../utils/time'
import { marked } from 'marked'

interface KnowledgeItem {
  id: number
  user_id: number
  username: string
  project: string
  category: string
  title: string
  summary: string
  content: string
  context: string
  tags: string[]
  source: string
  priority: string
  status: string
  created_at: string
  score?: number
}

interface UploadedFile {
  filename: string
  chunks: number
  uploaded_at: string
}

const loading = ref(false)
const documents = ref<KnowledgeItem[]>([])
const total = ref(0)

const searchQuery = ref('')
const searchResults = ref<KnowledgeItem[]>([])
const searching = ref(false)
const searchMode = ref(false)

// 项目过滤
const filterProject = ref('')
const filterCategory = ref('')

// 文件上传
const uploading = ref(false)
const uploadedFiles = ref<UploadedFile[]>([])
const uploadRef = ref<HTMLInputElement>()

// Tab
const activeTab = ref<'docs' | 'files'>('docs')

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

// 分页
const pageSize = 20
const currentPage = ref(1)
const pagedDocs = computed(() => {
  const start = (currentPage.value - 1) * pageSize
  return documents.value.slice(start, start + pageSize)
})
const totalPages = computed(() => Math.ceil(documents.value.length / pageSize))
function goPage(page: number) {
  if (page >= 1 && page <= totalPages.value) currentPage.value = page
}

async function loadDocs() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (filterProject.value) params.set('project', filterProject.value)
    if (filterCategory.value) params.set('category', filterCategory.value)
    params.set('limit', '200')
    const res = await fetch(`${API_BASE}/api/rag/list?${params}`, {
      headers: { 'Authorization': `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data.ok) {
      documents.value = data.data?.documents || []
      total.value = data.data?.total || 0
    }
  } catch (e: any) {
    ElMessage.error('加载知识库失败: ' + e.message)
  }
  loading.value = false
}

async function loadFiles() {
  try {
    const res = await fetch(`${API_BASE}/api/rag/files`, {
      headers: { 'Authorization': `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data.ok) {
      uploadedFiles.value = data.data?.files || []
    }
  } catch (e: any) {
    console.error('加载文件列表失败:', e)
  }
}

async function doSearch() {
  const q = searchQuery.value.trim()
  if (!q) {
    searchMode.value = false
    searchResults.value = []
    return
  }
  searching.value = true
  searchMode.value = true
  try {
    const res = await fetch(`${API_BASE}/api/rag/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getToken()}` },
      body: JSON.stringify({ query: q, top_k: 10 })
    })
    const data = await res.json()
    if (data.ok) {
      searchResults.value = data.data?.results || []
    }
  } catch (e: any) {
    ElMessage.error('搜索失败: ' + e.message)
  }
  searching.value = false
}

function clearSearch() {
  searchQuery.value = ''
  searchMode.value = false
  searchResults.value = []
}

function onFilterChange() {
  currentPage.value = 1
  loadDocs()
}

// 文件上传
function triggerUpload() {
  uploadRef.value?.click()
}

async function handleUpload(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)

    const res = await fetch(`${API_BASE}/api/rag/upload`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${getToken()}` },
      body: formData
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(`上传成功！${data.data?.filename} → ${data.data?.chunks} 个分块`)
      loadDocs()
      loadFiles()
    } else {
      ElMessage.error(data.error || '上传失败')
    }
  } catch (e: any) {
    ElMessage.error('上传失败: ' + e.message)
  }
  uploading.value = false
  input.value = ''
}

// 删除
async function deleteFile(filename: string) {
  try {
    await ElMessageBox.confirm(`确定要删除 "${filename}" 的所有知识块吗？`, '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }

  try {
    const res = await fetch(`${API_BASE}/api/rag/delete?source=upload:${encodeURIComponent(filename)}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success(`已删除 ${data.data?.deleted} 条记录`)
      loadDocs()
      loadFiles()
    } else {
      ElMessage.error(data.error || '删除失败')
    }
  } catch (e: any) {
    ElMessage.error('删除失败: ' + e.message)
  }
}

async function deleteDoc(id: number) {
  try {
    const res = await fetch(`${API_BASE}/api/rag/delete?id=${id}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data.ok) {
      ElMessage.success('已删除')
      loadDocs()
    } else {
      ElMessage.error(data.error || '删除失败')
    }
  } catch (e: any) {
    ElMessage.error('删除失败: ' + e.message)
  }
}

const CATEGORY_LABELS: Record<string, string> = {
  decision: '决策',
  pitfall: '踩坑',
  business: '业务',
  habit: '习惯',
  document: '文档',
  other: '其他',
}

const PROJECT_LABELS: Record<string, string> = {
  'ai-os': 'AI-OS',
  'rmp': 'RMP',
  'general': '通用',
}

function projectLabel(p: string): string {
  return PROJECT_LABELS[p] || p || '未知'
}

function categoryLabel(c: string): string {
  return CATEGORY_LABELS[c] || c || '未知'
}

function sourceLabel(source: string): string {
  if (!source) return '未知'
  if (source.startsWith('upload:')) return '📄 ' + source.slice(7)
  if (source.startsWith('distill:')) return '🧠 蒸馏'
  return source
}

function renderMarkdown(text: string): string {
  if (!text) return ''
  return marked.parse(text) as string
}

onMounted(() => {
  loadDocs()
  loadFiles()
})
</script>

<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h2>知识库</h2>
      <p class="page-desc">向量检索增强生成 — 上传文档或从对话中自动积累知识</p>
    </div>

    <!-- Search Bar -->
    <div class="search-bar">
      <div class="search-input-wrap">
        <svg class="search-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8" /><line x1="21" y1="21" x2="16.65" y2="16.65" />
        </svg>
        <input
          v-model="searchQuery"
          class="search-input"
          placeholder="输入关键词进行语义搜索..."
          @keyup.enter="doSearch"
        />
        <button v-if="searchQuery" class="clear-btn" @click="clearSearch">✕</button>
      </div>
      <button class="btn btn-primary" @click="doSearch" :disabled="searching">
        {{ searching ? '搜索中...' : '搜索' }}
      </button>
      <button class="btn" @click="loadDocs(); loadFiles()" :disabled="loading">
        {{ loading ? '加载中...' : '刷新' }}
      </button>
    </div>

    <!-- Stats -->
    <div class="stats-row">
      <div class="stat-item">
        <span class="stat-num">{{ total }}</span>
        <span class="stat-label">总知识块</span>
      </div>
      <div class="stat-item">
        <span class="stat-num">{{ uploadedFiles.length }}</span>
        <span class="stat-label">已上传文件</span>
      </div>
      <div class="stat-item" v-if="searchMode">
        <span class="stat-num">{{ searchResults.length }}</span>
        <span class="stat-label">搜索结果</span>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tabs" v-if="!searchMode">
      <button class="tab-btn" :class="{ active: activeTab === 'docs' }" @click="activeTab = 'docs'">知识块</button>
      <button class="tab-btn" :class="{ active: activeTab === 'files' }" @click="activeTab = 'files'">上传文件</button>
    </div>

    <!-- Search Results -->
    <template v-if="searchMode">
      <div v-if="searchResults.length" class="doc-list">
        <div v-for="r in searchResults" :key="r.id" class="doc-card search-hit">
          <div class="doc-meta">
            <span v-if="r.score" class="tag similarity" :class="r.score >= 0.7 ? 'high' : r.score >= 0.5 ? 'mid' : 'low'">
              {{ (r.score * 100).toFixed(0) }}%
            </span>
            <span class="tag project">{{ projectLabel(r.project) }}</span>
            <span class="tag type">{{ categoryLabel(r.category) }}</span>
            <span v-if="r.username" class="tag user">{{ r.username }}</span>
            <span class="tag source">{{ sourceLabel(r.source) }}</span>
          </div>
          <div v-if="r.title" class="doc-title">{{ r.title }}</div>
          <div class="doc-text" v-html="renderMarkdown(r.content)"></div>
        </div>
      </div>
      <div v-else-if="!searching" class="empty-state">
        <p>未找到相关内容</p>
      </div>
    </template>

    <!-- Upload Files Tab -->
    <template v-else-if="activeTab === 'files'">
      <div class="upload-area" @click="triggerUpload" :class="{ uploading }">
        <input ref="uploadRef" type="file" style="display:none"
          accept=".txt,.md,.markdown,.go,.py,.js,.ts,.vue,.java,.c,.cpp,.h,.rs,.sql,.yaml,.yml,.json,.xml,.html,.css,.sh,.bat,.ini,.cfg,.toml,.pdf,.docx,.xlsx"
          @change="handleUpload" />
        <div class="upload-icon">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
            <polyline points="17 8 12 3 7 8" />
            <line x1="12" y1="3" x2="12" y2="15" />
          </svg>
        </div>
        <p class="upload-text">{{ uploading ? '上传中...' : '点击上传文件或拖拽到此处' }}</p>
        <p class="upload-hint">支持 TXT / MD / DOCX / XLSX / 代码文件，最大 50MB</p>
      </div>

      <div v-if="uploadedFiles.length" class="file-list">
        <div v-for="f in uploadedFiles" :key="f.filename" class="file-item">
          <div class="file-info">
            <span class="file-name">{{ f.filename }}</span>
            <span class="file-meta">{{ f.chunks }} 个分块 · {{ formatTime(f.uploaded_at) }}</span>
          </div>
          <button class="btn-delete" @click="deleteFile(f.filename)" title="删除此文件的所有知识块">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
            </svg>
          </button>
        </div>
      </div>
      <div v-else-if="!uploading" class="empty-state">
        <p>还没有上传文件</p>
        <p class="hint">上传文档后会自动分块并向量化入库</p>
      </div>
    </template>

    <!-- All Documents Tab -->
    <template v-else>
      <!-- Filters -->
      <div class="filter-bar">
        <select v-model="filterProject" @change="onFilterChange" class="filter-select">
          <option value="">全部项目</option>
          <option value="ai-os">AI-OS</option>
          <option value="rmp">RMP</option>
          <option value="general">通用</option>
        </select>
        <select v-model="filterCategory" @change="onFilterChange" class="filter-select">
          <option value="">全部类型</option>
          <option value="decision">决策</option>
          <option value="pitfall">踩坑</option>
          <option value="business">业务</option>
          <option value="habit">习惯</option>
          <option value="document">文档</option>
          <option value="other">其他</option>
        </select>
      </div>

      <div v-if="documents.length" class="doc-list">
        <div v-for="doc in pagedDocs" :key="doc.id" class="doc-card">
          <div class="doc-meta">
            <span class="tag project">{{ projectLabel(doc.project) }}</span>
            <span class="tag type">{{ categoryLabel(doc.category) }}</span>
            <span v-if="doc.username" class="tag user">{{ doc.username }}</span>
            <span v-if="doc.priority === 'high'" class="tag priority-high">高优</span>
            <span class="tag source">{{ sourceLabel(doc.source) }}</span>
            <span class="tag time">{{ formatTime(doc.created_at) }}</span>
            <button class="btn-delete-sm" @click="deleteDoc(doc.id)" title="删除">✕</button>
          </div>
          <div v-if="doc.title" class="doc-title">{{ doc.title }}</div>
          <div v-if="doc.summary && doc.summary !== doc.title" class="doc-summary">{{ doc.summary }}</div>
          <div class="doc-text" v-html="renderMarkdown(doc.content)"></div>
          <div v-if="doc.tags && doc.tags.length" class="doc-tags">
            <span v-for="t in doc.tags" :key="t" class="tag tag-custom">{{ t }}</span>
          </div>
        </div>
      </div>
      <!-- Pagination -->
      <div v-if="totalPages > 1" class="pagination">
        <button class="page-btn" :disabled="currentPage <= 1" @click="goPage(currentPage - 1)">上一页</button>
        <template v-for="p in totalPages" :key="p">
          <button v-if="p === 1 || p === totalPages || Math.abs(p - currentPage) <= 1" class="page-btn" :class="{ active: p === currentPage }" @click="goPage(p)">{{ p }}</button>
          <span v-else-if="p === 2 && currentPage > 3 || p === totalPages - 1 && currentPage < totalPages - 2" class="page-ellipsis">...</span>
        </template>
        <button class="page-btn" :disabled="currentPage >= totalPages" @click="goPage(currentPage + 1)">下一页</button>
        <span class="page-info">共 {{ total }} 条，第 {{ currentPage }}/{{ totalPages }} 页</span>
      </div>
      <div v-else-if="!loading" class="empty-state">
        <p>知识库为空</p>
        <p class="hint">上传文档或在工作台/代理中对话后，AI 的回答会自动存入知识库</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  font-size: 18px;
  font-weight: 600;
  color: #1e293b;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #64748b;
  margin: 0;
}

/* Search Bar */
.search-bar {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.search-input-wrap {
  flex: 1;
  position: relative;
  display: flex;
  align-items: center;
}

.search-icon {
  position: absolute;
  left: 12px;
  color: #94a3b8;
  pointer-events: none;
}

.search-input {
  width: 100%;
  padding: 10px 36px 10px 36px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s;
  background: #fff;
}

.search-input:focus {
  border-color: #6366f1;
}

.clear-btn {
  position: absolute;
  right: 8px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: none;
  background: #e2e8f0;
  color: #64748b;
  font-size: 11px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.clear-btn:hover {
  background: #cbd5e1;
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  padding: 10px 16px;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  background: #fff;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  color: #475569;
}

.btn:hover:not(:disabled) {
  background: #f8fafc;
  border-color: #cbd5e1;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: #6366f1;
  color: #fff;
  border-color: #6366f1;
}

.btn-primary:hover:not(:disabled) {
  background: #4f46e5;
}

/* Stats */
.stats-row {
  display: flex;
  gap: 16px;
  margin-bottom: 16px;
}

.stat-item {
  display: flex;
  align-items: baseline;
  gap: 6px;
  padding: 8px 14px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.stat-num {
  font-size: 18px;
  font-weight: 700;
  color: #6366f1;
}

.stat-label {
  font-size: 12px;
  color: #94a3b8;
}

/* Tabs */
.tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 16px;
  border-bottom: 1px solid #e2e8f0;
  padding-bottom: 0;
}

.tab-btn {
  padding: 8px 16px;
  border: none;
  background: none;
  font-size: 13px;
  font-weight: 500;
  color: #64748b;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: all 0.2s;
}

.tab-btn:hover {
  color: #475569;
}

.tab-btn.active {
  color: #6366f1;
  border-bottom-color: #6366f1;
}

/* Filter Bar */
.filter-bar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.filter-select {
  padding: 6px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 13px;
  background: #fff;
  color: #475569;
  cursor: pointer;
  outline: none;
}

.filter-select:focus {
  border-color: #6366f1;
}

/* Upload Area */
.upload-area {
  border: 2px dashed #cbd5e1;
  border-radius: 12px;
  padding: 40px 20px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
  background: #fafbfc;
}

.upload-area:hover {
  border-color: #6366f1;
  background: #f5f3ff;
}

.upload-area.uploading {
  opacity: 0.6;
  cursor: not-allowed;
}

.upload-icon {
  color: #94a3b8;
  margin-bottom: 12px;
}

.upload-text {
  font-size: 14px;
  color: #475569;
  margin: 0 0 4px;
}

.upload-hint {
  font-size: 12px;
  color: #94a3b8;
  margin: 0;
}

/* File List */
.file-list {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.file-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
}

.file-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.file-name {
  font-size: 14px;
  font-weight: 500;
  color: #1e293b;
}

.file-meta {
  font-size: 12px;
  color: #94a3b8;
}

.btn-delete {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  background: none;
  color: #94a3b8;
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.15s;
}

.btn-delete:hover {
  background: #fef2f2;
  color: #ef4444;
}

.btn-delete-sm {
  border: none;
  background: none;
  color: #94a3b8;
  cursor: pointer;
  font-size: 12px;
  padding: 2px 6px;
  border-radius: 4px;
  margin-left: auto;
  transition: all 0.15s;
}

.btn-delete-sm:hover {
  background: #fef2f2;
  color: #ef4444;
}

/* Doc List */
.doc-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.doc-card {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
  transition: border-color 0.2s;
}

.doc-card:hover {
  border-color: #c7d2fe;
}

.doc-card.search-hit {
  border-left: 3px solid #6366f1;
}

.doc-title {
  font-size: 14px;
  font-weight: 600;
  color: #1e293b;
  margin-bottom: 4px;
}

.doc-summary {
  font-size: 12px;
  color: #64748b;
  margin-bottom: 6px;
}

.doc-meta {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
  align-items: center;
}

.tag {
  display: inline-flex;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 500;
}

.tag.source {
  background: #eff6ff;
  color: #3b82f6;
}

.tag.type {
  background: #f0fdf4;
  color: #22c55e;
}

.tag.project {
  background: #faf5ff;
  color: #9333ea;
}

.tag.user {
  background: #fff7ed;
  color: #ea580c;
}

.tag.time {
  background: #f8fafc;
  color: #94a3b8;
}

.tag.priority-high {
  background: #fef2f2;
  color: #dc2626;
}

.tag.tag-custom {
  background: #f1f5f9;
  color: #64748b;
}

.tag.similarity {
  font-weight: 600;
}

.tag.similarity.high {
  background: #f0fdf4;
  color: #16a34a;
}

.tag.similarity.mid {
  background: #fffbeb;
  color: #d97706;
}

.tag.similarity.low {
  background: #fef2f2;
  color: #dc2626;
}

.doc-tags {
  display: flex;
  gap: 4px;
  margin-top: 6px;
  flex-wrap: wrap;
}

.doc-text {
  font-size: 13px;
  color: #334155;
  line-height: 1.6;
  white-space: pre-wrap;
  max-height: 150px;
  overflow-y: auto;
}

/* Markdown */
.doc-text :deep(h1),
.doc-text :deep(h2),
.doc-text :deep(h3),
.doc-text :deep(h4) {
  margin: 8px 0 4px;
  font-weight: 600;
  color: #1e293b;
}
.doc-text :deep(h1) { font-size: 16px; }
.doc-text :deep(h2) { font-size: 15px; }
.doc-text :deep(h3) { font-size: 14px; }

.doc-text :deep(p) {
  margin: 4px 0;
}

.doc-text :deep(strong) {
  font-weight: 600;
  color: #1e293b;
}

.doc-text :deep(code) {
  background: #f1f5f9;
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 12px;
  font-family: 'Consolas', 'Monaco', monospace;
}

.doc-text :deep(pre) {
  background: #1e293b;
  color: #e2e8f0;
  padding: 10px 14px;
  border-radius: 6px;
  overflow-x: auto;
  font-size: 12px;
  line-height: 1.5;
  margin: 6px 0;
}

.doc-text :deep(pre code) {
  background: none;
  padding: 0;
  color: inherit;
}

.doc-text :deep(ul),
.doc-text :deep(ol) {
  padding-left: 20px;
  margin: 4px 0;
}

.doc-text :deep(li) {
  margin: 2px 0;
}

.doc-text :deep(blockquote) {
  border-left: 3px solid #6366f1;
  padding-left: 12px;
  margin: 6px 0;
  color: #64748b;
}

.doc-text :deep(table) {
  border-collapse: collapse;
  width: 100%;
  margin: 6px 0;
  font-size: 12px;
}

.doc-text :deep(th),
.doc-text :deep(td) {
  border: 1px solid #e2e8f0;
  padding: 4px 8px;
  text-align: left;
}

.doc-text :deep(th) {
  background: #f8fafc;
  font-weight: 600;
}

.doc-text :deep(hr) {
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 8px 0;
}

.doc-text::-webkit-scrollbar {
  width: 3px;
}

.doc-text::-webkit-scrollbar-thumb {
  background: #e2e8f0;
  border-radius: 2px;
}

/* Empty */
.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: #94a3b8;
}

.empty-state p {
  margin: 4px 0;
  font-size: 14px;
}

.empty-state .hint {
  font-size: 12px;
  color: #cbd5e1;
}

/* Pagination */
.pagination {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #e2e8f0;
  flex-wrap: wrap;
}

.page-btn {
  padding: 6px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #fff;
  color: #475569;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s;
}

.page-btn:hover:not(:disabled):not(.active) {
  background: #f1f5f9;
  border-color: #cbd5e1;
}

.page-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.page-btn.active {
  background: #6366f1;
  color: #fff;
  border-color: #6366f1;
}

.page-ellipsis {
  padding: 0 4px;
  color: #94a3b8;
}

.page-info {
  margin-left: 12px;
  font-size: 12px;
  color: #94a3b8;
}
</style>
