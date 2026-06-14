<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

interface DocItem {
  id: string
  text: string
  metadata: {
    source?: string
    type?: string
    timestamp?: number
  }
}

interface SearchResult {
  text: string
  metadata: Record<string, any>
  distance: number
  similarity: number
}

const loading = ref(false)
const documents = ref<DocItem[]>([])
const total = ref(0)

const searchQuery = ref('')
const searchResults = ref<SearchResult[]>([])
const searching = ref(false)
const searchMode = ref(false)

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
    const res = await window.aiOS.agentRequest('rag_list', {})
    if (res.ok) {
      documents.value = res.documents || []
      total.value = res.total || 0
    }
  } catch (e) {
    console.error('Failed to load docs:', e)
  }
  loading.value = false
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
    const res = await window.aiOS.agentRequest('rag_search', { query: q, top_k: 10 })
    if (res.ok) {
      searchResults.value = res.results || []
    }
  } catch (e) {
    console.error(e)
  }
  searching.value = false
}

function clearSearch() {
  searchQuery.value = ''
  searchMode.value = false
  searchResults.value = []
}

function formatTime(ts: number | undefined): string {
  if (!ts) return '-'
  const d = new Date(ts * 1000)
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function sourceLabel(source: string | undefined): string {
  if (!source) return '未知'
  const map: Record<string, string> = { workspace: '工作台', proxy: '代理', manual: '手动' }
  return map[source] || source
}

onMounted(loadDocs)
</script>

<template>
  <div class="knowledge-page">
    <div class="page-header">
      <h2>知识库</h2>
      <p class="page-desc">向量检索增强生成 — 知识库浏览与语义搜索</p>
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
      <button class="btn" @click="loadDocs" :disabled="loading">
        {{ loading ? '加载中...' : '刷新' }}
      </button>
    </div>

    <!-- Stats -->
    <div class="stats-row">
      <div class="stat-item">
        <span class="stat-num">{{ total }}</span>
        <span class="stat-label">总文档数</span>
      </div>
      <div class="stat-item" v-if="searchMode">
        <span class="stat-num">{{ searchResults.length }}</span>
        <span class="stat-label">搜索结果</span>
      </div>
    </div>

    <!-- Search Results -->
    <template v-if="searchMode">
      <div v-if="searchResults.length" class="doc-list">
        <div v-for="(r, i) in searchResults" :key="i" class="doc-card search-hit">
          <div class="doc-meta">
            <span class="tag similarity" :class="r.similarity >= 0.7 ? 'high' : r.similarity >= 0.4 ? 'mid' : 'low'">
              相似度 {{ (r.similarity * 100).toFixed(1) }}%
            </span>
            <span class="tag source">{{ sourceLabel(r.metadata?.source) }}</span>
          </div>
          <div class="doc-text">{{ r.text }}</div>
        </div>
      </div>
      <div v-else-if="!searching" class="empty-state">
        <p>未找到相关内容</p>
      </div>
    </template>

    <!-- All Documents -->
    <template v-else>
      <div v-if="documents.length" class="doc-list">
        <div v-for="doc in pagedDocs" :key="doc.id" class="doc-card">
          <div class="doc-meta">
            <span class="tag source">{{ sourceLabel(doc.metadata?.source) }}</span>
            <span class="tag type">{{ doc.metadata?.type || '-' }}</span>
            <span class="tag time">{{ formatTime(doc.metadata?.timestamp) }}</span>
          </div>
          <div class="doc-text">{{ doc.text }}</div>
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
        <p class="hint">在工作台或代理中对话后，AI 的回答会自动存入知识库</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.knowledge-page {
  max-width: 900px;
}

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

.doc-meta {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
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

.tag.time {
  background: #f8fafc;
  color: #94a3b8;
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

.doc-text {
  font-size: 13px;
  color: #334155;
  line-height: 1.6;
  white-space: pre-wrap;
  max-height: 150px;
  overflow-y: auto;
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
