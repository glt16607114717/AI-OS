<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

const loading = ref(true)
const embeddingCount = ref(0)
const testing = ref(false)
const testText = ref('采购单怎么审批？')
const testResult = ref<any>(null)
const searching = ref(false)
const searchQuery = ref('')
const searchResults = ref<any[]>([])

function getToken(): string {
  return localStorage.getItem('aios_token') || ''
}

async function fetchStatus() {
  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/rag/status`, {
      headers: { 'Authorization': `Bearer ${getToken()}` }
    })
    const data = await res.json()
    if (data.ok) {
      embeddingCount.value = data.data?.count ?? 0
    }
  } catch (e: any) {
    console.error('RAG status failed:', e)
  }
  loading.value = false
}

async function testEmbed() {
  testing.value = true
  testResult.value = null
  try {
    const res = await fetch(`${API_BASE}/api/rag/test-embed`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getToken()}` },
      body: JSON.stringify({ text: testText.value })
    })
    const data = await res.json()
    if (data.ok) {
      testResult.value = data.data
    } else {
      testResult.value = { error: data.error || '测试失败' }
    }
  } catch (e: any) {
    testResult.value = { error: e.message }
  }
  testing.value = false
}

async function doSearch() {
  if (!searchQuery.value.trim()) return
  searching.value = true
  searchResults.value = []
  try {
    const res = await fetch(`${API_BASE}/api/rag/search`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getToken()}` },
      body: JSON.stringify({ query: searchQuery.value, top_k: 5 })
    })
    const data = await res.json()
    if (data.ok) {
      searchResults.value = data.data?.results ?? []
    }
  } catch (e: any) {
    ElMessage.error('搜索失败: ' + e.message)
  }
  searching.value = false
}

onMounted(fetchStatus)
</script>

<template>
  <div class="rag-config">
    <div class="page-header">
      <h2>知识库配置</h2>
      <p class="page-desc">基于智谱 Embedding-3 的云端向量化检索（2048维）</p>
    </div>

    <el-card shadow="never" class="status-card" v-loading="loading">
      <div class="info-row">
        <div class="info-item">
          <span class="label">引擎</span>
          <el-tag type="success" size="small">智谱 Embedding-3</el-tag>
        </div>
        <div class="info-item">
          <span class="label">维度</span>
          <strong>2048</strong>
        </div>
        <div class="info-item">
          <span class="label">已存储向量</span>
          <strong>{{ embeddingCount }}</strong>
        </div>
        <div class="info-item">
          <span class="label">存储</span>
          <span>MySQL</span>
        </div>
      </div>
    </el-card>

    <!-- 功能测试 -->
    <el-card shadow="never" class="section">
      <template #header><span class="section-title">向量化测试</span></template>
      <div class="test-row">
        <el-input v-model="testText" placeholder="输入测试文本" @keyup.enter="testEmbed" />
        <el-button type="primary" @click="testEmbed" :loading="testing">测试</el-button>
      </div>
      <div v-if="testResult" class="test-result" :class="testResult.error ? 'error' : 'ok'">
        <template v-if="!testResult.error">
          <span>维度: {{ testResult.dimension }}</span>
          <span v-if="testResult.preview">前5维: [{{ testResult.preview.map((v: number) => v.toFixed(4)).join(', ') }}]</span>
        </template>
        <span v-else>{{ testResult.error }}</span>
      </div>
    </el-card>

    <!-- 语义搜索 -->
    <el-card shadow="never" class="section">
      <template #header><span class="section-title">语义搜索测试</span></template>
      <div class="test-row">
        <el-input v-model="searchQuery" placeholder="输入搜索内容" @keyup.enter="doSearch" />
        <el-button type="primary" @click="doSearch" :loading="searching">搜索</el-button>
      </div>
      <div v-if="searchResults.length" class="search-results">
        <div v-for="(r, i) in searchResults" :key="i" class="search-item">
          <div class="search-meta">相似度: {{ r.similarity }}</div>
          <div class="search-text">{{ r.text }}</div>
        </div>
      </div>
      <div v-else-if="searchQuery && !searching" class="no-results">暂无结果</div>
    </el-card>
  </div>
</template>

<style scoped>
.rag-config {
  max-width: 720px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header h2 {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 4px;
}

.page-desc {
  font-size: 13px;
  color: #909399;
  margin: 0;
}

.status-card, .section {
  border-radius: 10px;
}

.info-row {
  display: flex;
  gap: 24px;
  flex-wrap: wrap;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.info-item .label {
  font-size: 12px;
  color: #909399;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.test-row {
  display: flex;
  gap: 8px;
}

.test-result {
  margin-top: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 13px;
}

.test-result.ok {
  background: #f0f9eb;
  color: #67c23a;
}

.test-result.error {
  background: #fef0f0;
  color: #f56c6c;
}

.test-result span + span {
  margin-left: 16px;
}

.search-results {
  margin-top: 12px;
}

.search-item {
  padding: 10px 12px;
  background: #f5f7fa;
  border-radius: 8px;
  margin-bottom: 8px;
}

.search-meta {
  font-size: 11px;
  color: #909399;
  margin-bottom: 4px;
}

.search-text {
  font-size: 13px;
  color: #606266;
  white-space: pre-wrap;
  max-height: 100px;
  overflow-y: auto;
}

.no-results {
  text-align: center;
  padding: 20px;
  color: #c0c4cc;
  font-size: 13px;
}
</style>
