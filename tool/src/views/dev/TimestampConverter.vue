<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const now = ref(Date.now())
const tsInput = ref('')
const tsResult = ref('')
const dateInput = ref('')
const dateResult = ref('')
let timer: ReturnType<typeof setInterval> | null = null

function tsToDate() {
  const ts = Number(tsInput.value.trim())
  if (!tsInput.value.trim() || isNaN(ts)) { tsResult.value = ''; return }
  const ms = ts > 1e12 ? ts : ts * 1000
  const d = new Date(ms)
  if (isNaN(d.getTime())) { tsResult.value = '无效时间戳'; return }
  tsResult.value = d.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
    hour12: false,
  })
}

function dateToTs() {
  if (!dateInput.value) { dateResult.value = ''; return }
  const d = new Date(dateInput.value)
  if (isNaN(d.getTime())) { dateResult.value = '无效日期'; return }
  dateResult.value = String(Math.floor(d.getTime() / 1000))
}

function fillNow() {
  tsInput.value = String(Math.floor(Date.now() / 1000))
  tsToDate()
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {}
}

onMounted(() => {
  timer = setInterval(() => { now.value = Date.now() }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="tool-page">
    <h2 class="page-title">时间戳转换</h2>
    <p class="page-desc">Unix 时间戳与可读日期的相互转换</p>

    <!-- Current timestamp -->
    <div class="now-card">
      <div class="now-label">当前时间戳</div>
      <div class="now-row">
        <span class="now-value">{{ Math.floor(now / 1000) }}</span>
        <span class="now-unit">秒</span>
        <span class="now-sep">|</span>
        <span class="now-value">{{ now }}</span>
        <span class="now-unit">毫秒</span>
        <button class="btn btn-xs" @click="copy(String(Math.floor(now / 1000)))">复制</button>
      </div>
    </div>

    <div class="convert-grid">
      <!-- Timestamp → Date -->
      <div class="convert-card">
        <div class="card-header">
          <span class="card-title">时间戳 → 日期</span>
          <button class="btn btn-xs" @click="fillNow">填入当前</button>
        </div>
        <div class="card-body">
          <input
            v-model="tsInput"
            class="text-input"
            placeholder="输入 Unix 时间戳（秒或毫秒）"
            @input="tsToDate"
          />
          <div v-if="tsResult" class="result-box">
            <span class="result-text">{{ tsResult }}</span>
            <button class="btn btn-xs" @click="copy(tsResult)">复制</button>
          </div>
        </div>
      </div>

      <!-- Date → Timestamp -->
      <div class="convert-card">
        <div class="card-header">
          <span class="card-title">日期 → 时间戳</span>
        </div>
        <div class="card-body">
          <input
            v-model="dateInput"
            class="text-input"
            type="datetime-local"
            @input="dateToTs"
          />
          <div v-if="dateResult" class="result-box">
            <span class="result-text">{{ dateResult }}</span>
            <button class="btn btn-xs" @click="copy(dateResult)">复制</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tool-page {
  max-width: 720px;
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

/* Now card */
.now-card {
  background: linear-gradient(135deg, #0f172a, #1e1b4b);
  border-radius: 12px;
  padding: 18px 20px;
  margin-bottom: 20px;
}

.now-label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 8px;
}

.now-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}

.now-value {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 24px;
  font-weight: 700;
  color: #e0e7ff;
}

.now-unit {
  font-size: 12px;
  color: #64748b;
}

.now-sep {
  color: #334155;
  margin: 0 6px;
}

/* Convert grid */
.convert-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.convert-card {
  background: #fff;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
}

.card-title {
  font-size: 13px;
  font-weight: 600;
  color: #334155;
}

.card-body {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.text-input {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 13px;
  outline: none;
  transition: border-color 0.15s;
  font-family: 'Cascadia Code', 'Consolas', monospace;
}

.text-input:focus {
  border-color: #818cf8;
}

.result-box {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 6px;
}

.result-text {
  font-family: 'Cascadia Code', 'Consolas', monospace;
  font-size: 13px;
  color: #166534;
  font-weight: 500;
}

/* Buttons */
.btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid transparent;
  border-radius: 5px;
  cursor: pointer;
  font-size: 12px;
  font-weight: 500;
  transition: all 0.15s;
  background: rgba(255, 255, 255, 0.08);
  color: #94a3b8;
  padding: 3px 10px;
}

.btn:hover {
  color: #e2e8f0;
  background: rgba(255, 255, 255, 0.14);
}

.btn-xs {
  padding: 2px 8px;
  font-size: 11px;
}
</style>
