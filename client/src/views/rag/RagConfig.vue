<script setup lang="ts">
import { ref, onMounted } from 'vue'

interface Dependency {
  installed: boolean
  version: string | null
}

interface ModelFile {
  name: string
  size_mb: number
}

interface ModelStatus {
  model_ready: boolean
  model_loaded: boolean
  model_dir: string
  files: {
    found: ModelFile[]
    missing: string[]
  }
}

interface VectorStore {
  ok: boolean
  count: number
  name: string
  path: string
  error?: string
}

interface DownloadFile {
  name: string
  size: string
  required: boolean
}

const loading = ref(true)
const status = ref<any>(null)
const searchQuery = ref('')
const searchResults = ref<any[]>([])
const searching = ref(false)
const testText = ref('采购单怎么审批？')
const testResult = ref<any>(null)
const testing = ref(false)

async function fetchStatus() {
  loading.value = true
  try {
    const res = await window.aiOS.agentPost('/api/rag', { action: 'rag_status' })
    status.value = res
  } catch (e: any) {
    console.error('Failed to fetch RAG status:', e)
  }
  loading.value = false
}

async function doSearch() {
  if (!searchQuery.value.trim()) return
  searching.value = true
  searchResults.value = []
  try {
    const res = await window.aiOS.agentPost('/api/rag', {
      action: 'rag_search',
      payload: { query: searchQuery.value, top_k: 5 },
    })
    if (res.ok) {
      searchResults.value = res.results
    }
  } catch (e) {
    console.error(e)
  }
  searching.value = false
}

async function testEmbed() {
  testing.value = true
  testResult.value = null
  try {
    const res = await window.aiOS.agentPost('/api/rag', {
      action: 'rag_test_embed',
      payload: { text: testText.value },
    })
    testResult.value = res
  } catch (e) {
    console.error(e)
  }
  testing.value = false
}

async function resetStore() {
  if (!confirm('确定要清空向量库吗？此操作不可恢复。')) return
  try {
    await window.aiOS.agentPost('/api/rag', { action: 'rag_reset' })
    await fetchStatus()
  } catch (e) {
    console.error(e)
  }
}

onMounted(fetchStatus)
</script>

<template>
  <div class="rag-config">
    <div class="page-header">
      <h2>RAG 知识库配置</h2>
      <p class="page-desc">向量化检索增强生成 — 让 AI 回答更有依据</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>检测环境中...</span>
    </div>

    <template v-else-if="status">
      <!-- 总体状态 -->
      <div class="status-card" :class="status.ready ? 'ready' : 'not-ready'">
        <div class="status-icon">{{ status.ready ? '✓' : '!' }}</div>
        <div class="status-info">
          <div class="status-title">{{ status.ready ? 'RAG 环境就绪' : 'RAG 环境未就绪' }}</div>
          <div class="status-desc">
            {{ status.ready ? '向量化检索增强已启用，所有 AI 对话将自动检索相关知识' : '请按以下步骤完成配置' }}
          </div>
        </div>
        <button v-if="status.ready" class="btn btn-sm" @click="fetchStatus">刷新状态</button>
      </div>

      <!-- 1. Python 依赖 -->
      <div class="section">
        <h3>1. Python 依赖</h3>
        <div class="dep-grid">
          <div v-for="(dep, name) in status.components.dependencies" :key="name"
               class="dep-item" :class="{ ok: dep.installed, missing: !dep.installed }">
            <span class="dep-icon">{{ dep.installed ? '✓' : '✗' }}</span>
            <span class="dep-name">{{ name }}</span>
            <span class="dep-version">{{ dep.installed ? dep.version : '未安装' }}</span>
          </div>
        </div>
        <div v-if="Object.values(status.components.dependencies).some((d: any) => !d.installed)" class="hint-box">
          <strong>安装命令：</strong>
          <code>pip install onnxruntime chromadb transformers numpy</code>
        </div>
      </div>

      <!-- 2. 模型文件 -->
      <div class="section">
        <h3>2. BGE-M3 模型文件</h3>
        <div class="model-dir">
          <span class="label">存放目录：</span>
          <code>{{ status.components.model?.model_dir || '-' }}</code>
        </div>

        <table class="file-table">
          <thead>
            <tr>
              <th>文件名</th>
              <th>大小</th>
              <th>状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="f in status.components.model?.files?.found" :key="f.file">
              <td>{{ f.file }}</td>
              <td>{{ f.size_mb }} MB</td>
              <td class="ok">✓ 已就绪</td>
            </tr>
            <tr v-for="f in status.components.model?.files?.missing" :key="f" class="missing">
              <td>{{ f }}</td>
              <td>-</td>
              <td class="missing">✗ 缺失</td>
            </tr>
          </tbody>
        </table>

        <!-- 下载指引 -->
        <div v-if="status.components.model?.files?.missing?.length" class="hint-box">
          <p><strong>缺失文件请手动下载：</strong></p>
          <div class="download-links">
            <a v-for="(url, name) in status.download_guide?.download_urls?.direct_files" :key="name"
               :href="url" target="_blank" class="dl-link">
              {{ name }}
            </a>
          </div>
          <p class="hint-text">下载后放入上述目录，然后刷新状态。</p>
        </div>
      </div>

      <!-- 3. 向量库状态 -->
      <div class="section">
        <h3>3. 向量库</h3>
        <div class="vector-info">
          <div class="info-item">
            <span class="label">存储路径：</span>
            <code>{{ status.components.vector_store?.path || '-' }}</code>
          </div>
          <div class="info-item">
            <span class="label">文档数量：</span>
            <strong>{{ status.components.vector_store?.count ?? '-' }}</strong>
          </div>
          <div class="info-item">
            <span class="label">状态：</span>
            <span :class="status.components.vector_store?.ok ? 'ok' : 'missing'">
              {{ status.components.vector_store?.ok ? '正常' : (status.components.vector_store?.error || '未初始化') }}
            </span>
          </div>
        </div>
        <button v-if="status.components.vector_store?.count > 0"
                class="btn btn-danger btn-sm" @click="resetStore">
          清空向量库
        </button>
      </div>

      <!-- 4. 功能测试 -->
      <div v-if="status.ready" class="section">
        <h3>4. 功能测试</h3>

        <!-- 向量化测试 -->
        <div class="test-group">
          <h4>向量化测试</h4>
          <div class="test-row">
            <input v-model="testText" class="input" placeholder="输入测试文本" />
            <button class="btn btn-primary btn-sm" @click="testEmbed" :disabled="testing">
              {{ testing ? '测试中...' : '测试' }}
            </button>
          </div>
          <div v-if="testResult" class="test-result" :class="testResult.ok ? 'ok' : 'error'">
            <div v-if="testResult.ok">
              <span>维度: {{ testResult.dimension }}</span>
              <span>前5维: [{{ testResult.preview?.map((v: number) => v.toFixed(4)).join(', ') }}]</span>
            </div>
            <span v-else>{{ testResult.error }}</span>
          </div>
        </div>

        <!-- 搜索测试 -->
        <div class="test-group">
          <h4>语义搜索测试</h4>
          <div class="test-row">
            <input v-model="searchQuery" class="input" placeholder="输入搜索内容" />
            <button class="btn btn-primary btn-sm" @click="doSearch" :disabled="searching">
              {{ searching ? '搜索中...' : '搜索' }}
            </button>
          </div>
          <div v-if="searchResults.length" class="search-results">
            <div v-for="(r, i) in searchResults" :key="i" class="search-item">
              <div class="search-meta">
                相似度: {{ r.similarity }} · 来源: {{ r.metadata?.source || '-' }}
              </div>
              <div class="search-text">{{ r.text }}</div>
            </div>
          </div>
          <div v-else-if="searchQuery && !searching" class="no-results">暂无结果</div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.rag-config {
  max-width: 800px;
}

.page-header {
  margin-bottom: 24px;
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

.loading-state {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 40px;
  justify-content: center;
  color: #64748b;
}

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid #e2e8f0;
  border-top-color: #6366f1;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

/* Status Card */
.status-card {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 20px;
  border-radius: 12px;
  margin-bottom: 24px;
}

.status-card.ready {
  background: linear-gradient(135deg, #f0fdf4, #ecfdf5);
  border: 1px solid #bbf7d0;
}

.status-card.not-ready {
  background: linear-gradient(135deg, #fefce8, #fef9c3);
  border: 1px solid #fde68a;
}

.status-icon {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  font-weight: 700;
}

.ready .status-icon {
  background: #22c55e;
  color: #fff;
}

.not-ready .status-icon {
  background: #f59e0b;
  color: #fff;
}

.status-info { flex: 1; }

.status-title {
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.status-desc {
  font-size: 12px;
  color: #64748b;
  margin-top: 2px;
}

/* Section */
.section {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 16px;
}

.section h3 {
  font-size: 14px;
  font-weight: 600;
  color: #334155;
  margin: 0 0 14px;
}

/* Dependencies */
.dep-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}

.dep-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
}

.dep-item.ok { background: #f0fdf4; }
.dep-item.missing { background: #fef2f2; }

.dep-icon { font-weight: 700; }
.dep-item.ok .dep-icon { color: #22c55e; }
.dep-item.missing .dep-icon { color: #ef4444; }

.dep-name { font-weight: 500; color: #334155; flex: 1; }
.dep-version { color: #94a3b8; font-size: 12px; }

/* Hint Box */
.hint-box {
  margin-top: 12px;
  padding: 12px 16px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 13px;
  color: #475569;
}

.hint-box strong { color: #334155; }
.hint-box code {
  display: block;
  margin-top: 6px;
  padding: 8px 12px;
  background: #1e293b;
  color: #e2e8f0;
  border-radius: 6px;
  font-size: 12px;
}

.hint-text { margin-top: 8px; font-size: 12px; color: #94a3b8; }

.download-links {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.dl-link {
  display: inline-block;
  padding: 4px 10px;
  background: #eff6ff;
  color: #3b82f6;
  border-radius: 6px;
  font-size: 12px;
  text-decoration: none;
  transition: all 0.2s;
}

.dl-link:hover { background: #dbeafe; }

/* Model */
.model-dir {
  font-size: 13px;
  margin-bottom: 12px;
}

.model-dir .label { color: #64748b; }
.model-dir code {
  background: #f1f5f9;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 12px;
}

.file-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.file-table th {
  text-align: left;
  padding: 6px 12px;
  color: #64748b;
  font-weight: 500;
  border-bottom: 1px solid #e2e8f0;
}

.file-table td {
  padding: 8px 12px;
  border-bottom: 1px solid #f1f5f9;
}

.file-table .ok { color: #22c55e; font-weight: 500; }
.file-table .missing { color: #ef4444; }

/* Vector Store */
.vector-info {
  display: flex;
  gap: 24px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.info-item { font-size: 13px; }
.info-item .label { color: #64748b; }
.info-item code {
  background: #f1f5f9;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}
.info-item .ok { color: #22c55e; font-weight: 500; }
.info-item .missing { color: #ef4444; }

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border: none;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-sm { padding: 6px 12px; font-size: 12px; }

.btn-primary { background: #6366f1; color: #fff; }
.btn-primary:hover:not(:disabled) { background: #4f46e5; }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-danger { background: #fef2f2; color: #ef4444; border: 1px solid #fecaca; }
.btn-danger:hover { background: #fee2e2; }

/* Test */
.test-group {
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #f1f5f9;
}

.test-group:last-child { border-bottom: none; }

.test-group h4 {
  font-size: 13px;
  font-weight: 500;
  color: #475569;
  margin: 0 0 8px;
}

.test-row {
  display: flex;
  gap: 8px;
}

.input {
  flex: 1;
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-size: 13px;
  outline: none;
  transition: border-color 0.2s;
}

.input:focus { border-color: #6366f1; }

.test-result {
  margin-top: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 12px;
}

.test-result.ok { background: #f0fdf4; color: #166534; }
.test-result.error { background: #fef2f2; color: #991b1b; }

.test-result span + span { margin-left: 16px; }

/* Search Results */
.search-results {
  margin-top: 12px;
}

.search-item {
  padding: 10px 12px;
  background: #f8fafc;
  border-radius: 8px;
  margin-bottom: 8px;
}

.search-meta {
  font-size: 11px;
  color: #94a3b8;
  margin-bottom: 4px;
}

.search-text {
  font-size: 13px;
  color: #334155;
  white-space: pre-wrap;
  max-height: 120px;
  overflow-y: auto;
}

.no-results {
  text-align: center;
  padding: 20px;
  color: #94a3b8;
  font-size: 13px;
}
</style>
