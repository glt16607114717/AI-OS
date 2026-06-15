<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { Chart, registerables } from 'chart.js'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

Chart.register(...registerables)

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

interface VendorStat {
  name: string
  requests: number
  tokens: number
  avg_latency_ms: number
  success_count: number
  errors: number
}

interface ModelStat {
  requests: number
  tokens: number
  avg_latency_ms: number
  success_count: number
  errors: number
}

interface UserStat {
  username: string
  requests: number
  tokens: number
  prompt_tokens: number
  completion_tokens: number
  avg_latency_ms: number
  success_count: number
  errors: number
}

interface DailyStat {
  requests: number
  tokens: number
  prompt_tokens: number
  completion_tokens: number
  success_count: number
  errors: number
}

interface Stats {
  total_requests: number
  total_tokens: number
  total_prompt_tokens: number
  total_completion_tokens: number
  avg_latency_ms: number
  success_rate: number
  by_vendor: Record<string, VendorStat>
  by_model: Record<string, ModelStat>
  by_user: Record<string, UserStat>
  daily: Record<string, DailyStat>
}

const stats = ref<Stats | null>(null)
const loading = ref(false)
const days = ref(30)
const currentUsername = ref('')

// Chart refs
const trendChartRef = ref<HTMLCanvasElement | null>(null)
const tokenChartRef = ref<HTMLCanvasElement | null>(null)
const modelPieRef = ref<HTMLCanvasElement | null>(null)
const userRankRef = ref<HTMLCanvasElement | null>(null)

let trendChart: Chart | null = null
let tokenChart: Chart | null = null
let modelPie: Chart | null = null
let userRank: Chart | null = null

// 获取当前用户名
async function fetchCurrentUser() {
  try {
    const token = localStorage.getItem('aios_token')
    if (!token) return
    const res = await fetch(`${API_BASE}/api/system/me`, {
      headers: { Authorization: `Bearer ${token}` }
    })
    const data = await res.json()
    if (data?.ok && data?.data) {
      currentUsername.value = data.data.username || ''
    }
  } catch (e) {
    console.error('[StatsDashboard] fetchCurrentUser error:', e)
    ElMessage.error('获取用户信息失败: ' + (e as Error).message)
  }
}

const sortedDaily = computed(() => {
  if (!stats.value?.daily) return []
  return Object.entries(stats.value.daily)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, d]) => ({ date, ...d }))
})

const modelEntries = computed(() => {
  if (!stats.value?.by_model) return []
  return Object.entries(stats.value.by_model)
    .map(([id, m]) => ({ id, ...m }))
    .sort((a, b) => b.requests - a.requests)
})

const userEntries = computed(() => {
  if (!stats.value?.by_user) return []
  const entries = Object.entries(stats.value.by_user)
    .map(([id, u]) => ({ id, ...u }))
    .sort((a, b) => b.tokens - a.tokens)

  // 当前用户置顶
  if (currentUsername.value) {
    const idx = entries.findIndex(e => e.username === currentUsername.value)
    if (idx > 0) {
      const [me] = entries.splice(idx, 1)
      entries.unshift(me)
    }
  }
  return entries
})

const myStats = computed(() => {
  if (!stats.value?.by_user || !currentUsername.value) return null
  for (const u of Object.values(stats.value.by_user)) {
    if (u.username === currentUsername.value) return u
  }
  return null
})

function formatNumber(n: number) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}

function formatLatency(ms: number) {
  if (ms >= 1000) return (ms / 1000).toFixed(1) + 's'
  return Math.round(ms) + 'ms'
}

function renderCharts() {
  if (!stats.value) return

  const dailyData = sortedDaily.value
  const labels = dailyData.map(d => d.date.slice(5)) // MM-DD

  // ── 折线图：每日请求趋势 ──
  if (trendChartRef.value) {
    if (trendChart) trendChart.destroy()

    const totalRequests = dailyData.map(d => d.requests)
    const myRequests = dailyData.map(d => {
      if (!currentUsername.value || !stats.value) return 0
      // by_user 里没有按天分的数据，用全局的近似
      return 0 // 暂时只有全局线
    })

    trendChart = new Chart(trendChartRef.value, {
      type: 'line',
      data: {
        labels,
        datasets: [{
          label: '总请求数',
          data: totalRequests,
          borderColor: '#409eff',
          backgroundColor: 'rgba(64,158,255,0.1)',
          fill: true,
          tension: 0.3,
          pointRadius: 3,
          pointHoverRadius: 5,
        }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: true, position: 'top', labels: { font: { size: 12 } } } },
        scales: {
          y: { beginAtZero: true, grid: { color: '#f0f0f0' }, ticks: { font: { size: 11 } } },
          x: { grid: { display: false }, ticks: { font: { size: 11 } } },
        },
      },
    })
  }

  // ── 柱状图：每日 Token 消耗（堆叠） ──
  if (tokenChartRef.value) {
    if (tokenChart) tokenChart.destroy()

    tokenChart = new Chart(tokenChartRef.value, {
      type: 'bar',
      data: {
        labels,
        datasets: [
          {
            label: 'Prompt Tokens',
            data: dailyData.map(d => d.prompt_tokens || 0),
            backgroundColor: '#409eff',
            borderRadius: 3,
          },
          {
            label: 'Completion Tokens',
            data: dailyData.map(d => d.completion_tokens || 0),
            backgroundColor: '#67c23a',
            borderRadius: 3,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: true, position: 'top', labels: { font: { size: 12 } } } },
        scales: {
          x: { stacked: true, grid: { display: false }, ticks: { font: { size: 11 } } },
          y: { stacked: true, beginAtZero: true, grid: { color: '#f0f0f0' }, ticks: { font: { size: 11 } } },
        },
      },
    })
  }

  // ── 饼图：模型请求占比 ──
  if (modelPieRef.value) {
    if (modelPie) modelPie.destroy()

    const models = modelEntries.value.slice(0, 8) // 最多显示 8 个
    const colors = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#9b59b6', '#1abc9c', '#e74c3c']

    modelPie = new Chart(modelPieRef.value, {
      type: 'doughnut',
      data: {
        labels: models.map(m => m.id),
        datasets: [{
          data: models.map(m => m.requests),
          backgroundColor: colors.slice(0, models.length),
          borderWidth: 2,
          borderColor: '#fff',
        }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: true, position: 'right', labels: { font: { size: 11 }, boxWidth: 12, padding: 8 } },
        },
      },
    })
  }

  // ── 柱状图：用户用量排行 ──
  if (userRankRef.value) {
    if (userRank) userRank.destroy()

    const users = userEntries.value.slice(0, 10)
    const bgColors = users.map(u =>
      u.username === currentUsername.value ? '#f56c6c' : '#409eff'
    )

    userRank = new Chart(userRankRef.value, {
      type: 'bar',
      data: {
        labels: users.map(u => u.username || u.id),
        datasets: [{
          label: 'Token 用量',
          data: users.map(u => u.tokens),
          backgroundColor: bgColors,
          borderRadius: 4,
        }],
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { beginAtZero: true, grid: { color: '#f0f0f0' }, ticks: { font: { size: 11 } } },
          y: { grid: { display: false }, ticks: {
            font: { size: 12 },
            color: (ctx: any) => {
              const label = ctx.tick?.label as string
              return label === currentUsername.value ? '#f56c6c' : '#606266'
            },
          }},
        },
      },
    })
  }
}

async function fetchStats() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/stats/summary?days=${days.value}`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      stats.value = result.data || null
      await nextTick()
      renderCharts()
    }
  } catch (e) {
    console.error('[StatsDashboard] fetchStats error:', e)
  }
  loading.value = false
}

let refreshTimer: ReturnType<typeof setInterval> | null = null
const autoRefresh = ref(true)

function toggleAutoRefresh() {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    refreshTimer = setInterval(fetchStats, 5000)
  } else {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null }
  }
}

onMounted(async () => {
  await fetchCurrentUser()
  await fetchStats()
  refreshTimer = setInterval(fetchStats, 30000)
})

onUnmounted(() => {
  if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null }
  trendChart?.destroy()
  tokenChart?.destroy()
  modelPie?.destroy()
  userRank?.destroy()
})
</script>

<template>
  <div class="stats-dashboard">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">统计仪表</h2>
      <div class="header-actions">
        <div class="day-tabs">
          <button class="day-tab" :class="{ active: days === 7 }" @click="days = 7; fetchStats()">近 7 天</button>
          <button class="day-tab" :class="{ active: days === 30 }" @click="days = 30; fetchStats()">近 30 天</button>
        </div>
        <button class="refresh-toggle" :class="{ active: autoRefresh }" @click="toggleAutoRefresh">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          {{ autoRefresh ? '自动刷新' : '已暂停' }}
        </button>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading && !stats" class="empty-state"><p class="empty-text">加载中...</p></div>
    <div v-else-if="!stats" class="empty-state"><p class="empty-text">暂无统计数据</p></div>

    <template v-else>
      <!-- Overview Cards -->
      <div class="overview-cards">
        <div class="overview-card">
          <div class="card-value">{{ formatNumber(stats.total_requests) }}</div>
          <div class="card-label">总请求数</div>
        </div>
        <div class="overview-card">
          <div class="card-value">{{ formatNumber(stats.total_tokens) }}</div>
          <div class="card-label">总 Token 数</div>
          <div class="card-sub">Prompt: {{ formatNumber(stats.total_prompt_tokens) }} / Completion: {{ formatNumber(stats.total_completion_tokens) }}</div>
        </div>
        <div class="overview-card">
          <div class="card-value">{{ formatLatency(stats.avg_latency_ms) }}</div>
          <div class="card-label">平均延迟</div>
        </div>
        <div class="overview-card">
          <div class="card-value" :class="stats.success_rate >= 0.95 ? 'value-good' : stats.success_rate >= 0.8 ? 'value-warn' : 'value-bad'">
            {{ (stats.success_rate * 100).toFixed(1) }}%
          </div>
          <div class="card-label">成功率</div>
        </div>
        <!-- My Stats -->
        <div v-if="myStats" class="overview-card my-card">
          <div class="card-value value-red">{{ formatNumber(myStats.requests) }}</div>
          <div class="card-label card-label-red">我的请求数</div>
        </div>
        <div v-if="myStats" class="overview-card my-card">
          <div class="card-value value-red">{{ formatNumber(myStats.tokens) }}</div>
          <div class="card-label card-label-red">我的 Token 数</div>
        </div>
      </div>

      <!-- Charts Grid -->
      <div class="charts-grid">
        <!-- 折线图：每日请求趋势 -->
        <div class="chart-card">
          <div class="section-title">每日请求趋势</div>
          <div class="chart-container"><canvas ref="trendChartRef"></canvas></div>
        </div>

        <!-- 柱状图：每日 Token 消耗 -->
        <div class="chart-card">
          <div class="section-title">每日 Token 消耗</div>
          <div class="chart-container"><canvas ref="tokenChartRef"></canvas></div>
        </div>

        <!-- 饼图：模型请求占比 -->
        <div class="chart-card">
          <div class="section-title">模型请求占比</div>
          <div class="chart-container"><canvas ref="modelPieRef"></canvas></div>
        </div>

        <!-- 柱状图：用户用量排行 -->
        <div class="chart-card">
          <div class="section-title">用户用量排行</div>
          <div class="chart-container chart-container-tall"><canvas ref="userRankRef"></canvas></div>
        </div>
      </div>

      <!-- Model Detail Table -->
      <div class="section-card">
        <div class="section-title">模型详情</div>
        <div v-if="modelEntries.length === 0" class="table-empty">暂无数据</div>
        <table v-else class="stats-table">
          <thead>
            <tr><th>模型</th><th>请求数</th><th>Token 数</th><th>平均延迟</th><th>成功率</th><th>错误数</th></tr>
          </thead>
          <tbody>
            <tr v-for="m in modelEntries" :key="m.id">
              <td class="cell-model">{{ m.id }}</td>
              <td>{{ m.requests }}</td>
              <td>{{ formatNumber(m.tokens) }}</td>
              <td>{{ formatLatency(m.avg_latency_ms) }}</td>
              <td>{{ m.success_count + m.errors > 0 ? (m.success_count / (m.success_count + m.errors) * 100).toFixed(1) + '%' : '—' }}</td>
              <td><span v-if="m.errors > 0" class="error-badge">{{ m.errors }}</span><span v-else class="zero-text">0</span></td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- User Detail Table -->
      <div class="section-card">
        <div class="section-title">用户详情</div>
        <div v-if="userEntries.length === 0" class="table-empty">暂无数据</div>
        <table v-else class="stats-table">
          <thead>
            <tr><th>用户</th><th>请求数</th><th>Token 数</th><th>Prompt</th><th>Completion</th><th>平均延迟</th><th>成功率</th></tr>
          </thead>
          <tbody>
            <tr v-for="u in userEntries" :key="u.id" :class="{ 'my-row': u.username === currentUsername }">
              <td class="cell-name" :class="{ 'cell-name-red': u.username === currentUsername }">{{ u.username || u.id }}</td>
              <td>{{ u.requests }}</td>
              <td>{{ formatNumber(u.tokens) }}</td>
              <td>{{ formatNumber(u.prompt_tokens) }}</td>
              <td>{{ formatNumber(u.completion_tokens) }}</td>
              <td>{{ formatLatency(u.avg_latency_ms) }}</td>
              <td>{{ u.success_count + u.errors > 0 ? (u.success_count / (u.success_count + u.errors) * 100).toFixed(1) + '%' : '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.stats-dashboard {
  max-width: 1080px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.day-tabs {
  display: flex;
  background: #f4f4f5;
  border-radius: 8px;
  padding: 3px;
}

.day-tab {
  padding: 5px 16px;
  font-size: 13px;
  font-weight: 500;
  color: #606266;
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.day-tab:hover { color: #303133; }
.day-tab.active { background: #fff; color: #409eff; box-shadow: 0 1px 4px rgba(0,0,0,0.08); }

.refresh-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 12px;
  font-size: 12px;
  color: #909399;
  background: #f4f4f5;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}
.refresh-toggle:hover { color: #606266; }
.refresh-toggle.active { color: #67c23a; background: #f0f9eb; }

.empty-state { display: flex; flex-direction: column; align-items: center; padding: 60px 0; }
.empty-text { font-size: 15px; font-weight: 500; color: #909399; }

/* Overview Cards */
.overview-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 12px;
}

.overview-card {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  transition: box-shadow 0.2s;
}
.overview-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,0.06); }

.my-card {
  border-color: #fde2e2;
  background: linear-gradient(135deg, #fff5f5, #fff);
}

.card-value { font-size: 24px; font-weight: 700; color: #303133; line-height: 1.2; }
.value-good { color: #67c23a; }
.value-warn { color: #e6a23c; }
.value-bad { color: #f56c6c; }
.value-red { color: #f56c6c; }

.card-label { font-size: 13px; font-weight: 500; color: #909399; }
.card-label-red { color: #f56c6c; }
.card-sub { font-size: 11px; color: #c0c4cc; margin-top: 2px; }

/* Charts Grid */
.charts-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.chart-card {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 20px;
  transition: box-shadow 0.2s;
}
.chart-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,0.06); }

.chart-container {
  height: 240px;
  position: relative;
}

.chart-container-tall {
  height: 300px;
}

/* Section Card */
.section-card {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 20px;
  transition: box-shadow 0.2s;
}
.section-card:hover { box-shadow: 0 2px 12px rgba(0,0,0,0.06); }

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 14px;
}

/* Table */
.table-empty { text-align: center; padding: 24px 0; font-size: 13px; color: #c0c4cc; }

.stats-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.stats-table th { text-align: left; padding: 10px 12px; font-weight: 600; color: #909399; font-size: 12px; border-bottom: 1px solid #f0f0f0; white-space: nowrap; }
.stats-table td { padding: 10px 12px; color: #606266; border-bottom: 1px solid #fafafa; white-space: nowrap; }
.stats-table tbody tr:hover { background: #fafbfc; }
.stats-table tbody tr:last-child td { border-bottom: none; }

.my-row { background: #fff5f5; }
.my-row:hover { background: #fef0f0; }

.cell-name { font-weight: 500; color: #303133; }
.cell-name-red { color: #f56c6c; font-weight: 600; }
.cell-model { font-family: 'Cascadia Code', 'Consolas', monospace; font-size: 12px; color: #409eff; }

.error-badge { display: inline-block; padding: 1px 8px; border-radius: 10px; font-size: 12px; font-weight: 500; color: #f56c6c; background: #fef0f0; }
.zero-text { color: #c0c4cc; }
</style>
