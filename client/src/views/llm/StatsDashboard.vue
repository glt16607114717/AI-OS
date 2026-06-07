<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

const agentRequest = window.aiOS.agentRequest

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

interface DailyStat {
  requests: number
  tokens: number
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
  daily: Record<string, DailyStat>
}

const stats = ref<Stats | null>(null)
const loading = ref(false)
const days = ref(30)

const sortedDaily = computed(() => {
  if (!stats.value?.daily) return []
  return Object.entries(stats.value.daily)
    .sort(([a], [b]) => b.localeCompare(a))
    .map(([date, d]) => ({ date, ...d }))
})

const vendorEntries = computed(() => {
  if (!stats.value?.by_vendor) return []
  return Object.entries(stats.value.by_vendor).map(([id, v]) => ({ id, ...v }))
})

const modelEntries = computed(() => {
  if (!stats.value?.by_model) return []
  return Object.entries(stats.value.by_model).map(([id, m]) => ({ id, ...m }))
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

function successRate(success: number, errors: number) {
  const total = success + errors
  if (total === 0) return '—'
  return (success / total * 100).toFixed(1) + '%'
}

async function fetchStats() {
  loading.value = true
  try {
    const res = await agentRequest('llm_get_stats', { days: days.value })
    if (res.ok) {
      stats.value = res.stats || null
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

onMounted(() => {
  fetchStats()
  refreshTimer = setInterval(fetchStats, 5000)
})

onUnmounted(() => {
  if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null }
})
</script>

<template>
  <div class="stats-dashboard">
    <!-- Header -->
    <div class="page-header">
      <h2 class="page-title">统计仪表</h2>
      <div class="header-actions">
        <div class="day-tabs">
          <button
            class="day-tab"
            :class="{ active: days === 7 }"
            @click="days = 7; fetchStats()"
          >近 7 天</button>
          <button
            class="day-tab"
            :class="{ active: days === 30 }"
            @click="days = 30; fetchStats()"
          >近 30 天</button>
        </div>
        <button class="refresh-toggle" :class="{ active: autoRefresh }" @click="toggleAutoRefresh">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="23 4 23 10 17 10" />
            <polyline points="1 20 1 14 7 14" />
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
          </svg>
          {{ autoRefresh ? '自动刷新' : '已暂停' }}
        </button>
      </div>
    </div>

    <!-- Loading -->
    <div v-if="loading && !stats" class="empty-state">
      <p class="empty-text">加载中...</p>
    </div>

    <!-- Empty State -->
    <div v-else-if="!stats" class="empty-state">
      <p class="empty-text">暂无统计数据</p>
    </div>

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
      </div>

      <!-- Vendor Table -->
      <div class="section-card">
        <div class="section-title">按厂商统计</div>
        <div v-if="vendorEntries.length === 0" class="table-empty">暂无数据</div>
        <table v-else class="stats-table">
          <thead>
            <tr>
              <th>厂商</th>
              <th>请求数</th>
              <th>Token 数</th>
              <th>平均延迟</th>
              <th>成功率</th>
              <th>错误数</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="v in vendorEntries" :key="v.id">
              <td class="cell-name">{{ v.name || v.id }}</td>
              <td>{{ v.requests }}</td>
              <td>{{ formatNumber(v.tokens) }}</td>
              <td>{{ formatLatency(v.avg_latency_ms) }}</td>
              <td>{{ successRate(v.success_count, v.errors) }}</td>
              <td>
                <span v-if="v.errors > 0" class="error-badge">{{ v.errors }}</span>
                <span v-else class="zero-text">0</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Model Table -->
      <div class="section-card">
        <div class="section-title">按模型统计</div>
        <div v-if="modelEntries.length === 0" class="table-empty">暂无数据</div>
        <table v-else class="stats-table">
          <thead>
            <tr>
              <th>模型</th>
              <th>请求数</th>
              <th>Token 数</th>
              <th>平均延迟</th>
              <th>成功率</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in modelEntries" :key="m.id">
              <td class="cell-model">{{ m.id }}</td>
              <td>{{ m.requests }}</td>
              <td>{{ formatNumber(m.tokens) }}</td>
              <td>{{ formatLatency(m.avg_latency_ms) }}</td>
              <td>{{ successRate(m.success_count, m.errors) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Daily Trend Table -->
      <div class="section-card">
        <div class="section-title">每日趋势</div>
        <div v-if="sortedDaily.length === 0" class="table-empty">暂无数据</div>
        <table v-else class="stats-table">
          <thead>
            <tr>
              <th>日期</th>
              <th>请求数</th>
              <th>Token 数</th>
              <th>成功</th>
              <th>错误</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="d in sortedDaily" :key="d.date">
              <td class="cell-date">{{ d.date }}</td>
              <td>{{ d.requests }}</td>
              <td>{{ formatNumber(d.tokens) }}</td>
              <td class="cell-success">{{ d.success_count }}</td>
              <td>
                <span v-if="d.errors > 0" class="error-badge">{{ d.errors }}</span>
                <span v-else class="zero-text">0</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.stats-dashboard {
  max-width: 960px;
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

/* Day Tabs */
.header-actions {
  display: flex;
  align-items: center;
}

.day-tabs {
  display: flex;
  gap: 0;
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

.day-tab:hover {
  color: #303133;
}

.day-tab.active {
  background: #fff;
  color: #409eff;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

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

/* Empty State */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 60px 0;
  color: #c0c4cc;
}

.empty-text {
  font-size: 15px;
  font-weight: 500;
  color: #909399;
}

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

.overview-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.card-value {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
  line-height: 1.2;
}

.value-good {
  color: #67c23a;
}

.value-warn {
  color: #e6a23c;
}

.value-bad {
  color: #f56c6c;
}

.card-label {
  font-size: 13px;
  font-weight: 500;
  color: #909399;
}

.card-sub {
  font-size: 11px;
  color: #c0c4cc;
  margin-top: 2px;
}

/* Section Card */
.section-card {
  background: #fff;
  border-radius: 12px;
  border: 1px solid #e4e7ed;
  padding: 20px;
  transition: box-shadow 0.2s;
}

.section-card:hover {
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 14px;
}

/* Table */
.table-empty {
  text-align: center;
  padding: 24px 0;
  font-size: 13px;
  color: #c0c4cc;
}

.stats-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.stats-table th {
  text-align: left;
  padding: 10px 12px;
  font-weight: 600;
  color: #909399;
  font-size: 12px;
  border-bottom: 1px solid #f0f0f0;
  white-space: nowrap;
}

.stats-table td {
  padding: 10px 12px;
  color: #606266;
  border-bottom: 1px solid #fafafa;
  white-space: nowrap;
}

.stats-table tbody tr:hover {
  background: #fafbfc;
}

.stats-table tbody tr:last-child td {
  border-bottom: none;
}

.cell-name {
  font-weight: 500;
  color: #303133;
}

.cell-model {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
  color: #409eff;
}

.cell-date {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 12px;
  color: #303133;
}

.cell-success {
  color: #67c23a;
}

.error-badge {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 500;
  color: #f56c6c;
  background: #fef0f0;
}

.zero-text {
  color: #c0c4cc;
}
</style>
