<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { API_BASE } from '../api'

const router = useRouter()

const username = ref('')
const password = ref('')
const rememberMe = ref(true)
const loading = ref(false)
const errorMsg = ref('')

const bgCanvas = ref<HTMLCanvasElement | null>(null)
let animId = 0
let ctx: CanvasRenderingContext2D | null = null
let currentW = 0
let currentH = 0
let fontSize = 16
let cols = 0
let drops: number[] = []
const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@#$%&*<>/\\|{}[]01'
let bolts: any[] = []
let nextBolt = 0
let flashAlpha = 0

function resizeCanvas() {
  const canvas = bgCanvas.value
  if (!canvas) return
  const c = canvas.getContext('2d')
  if (!c) return
  const dpr = window.devicePixelRatio || 1
  const displayWidth = window.innerWidth
  const displayHeight = window.innerHeight
  
  // 设置 canvas 的内部像素尺寸
  canvas.width = displayWidth * dpr
  canvas.height = displayHeight * dpr
  
  // 设置 canvas 的 CSS 显示尺寸
  canvas.style.width = displayWidth + 'px'
  canvas.style.height = displayHeight + 'px'
  
  c.scale(dpr, dpr)
  currentW = displayWidth
  currentH = displayHeight
  fontSize = 16
  cols = Math.ceil(currentW / fontSize)
  drops = Array(cols).fill(0).map(() => Math.random() * -80)
}

function fractalBolt(x1: number, y1: number, x2: number, y2: number, depth: number, maxDepth: number, jitter: number): { x: number; y: number }[] {
  if (depth >= maxDepth) return [{ x: x1, y: y1 }, { x: x2, y: y2 }]
  const mx = (x1 + x2) / 2 + (Math.random() - 0.5) * jitter
  const my = (y1 + y2) / 2 + (Math.random() - 0.5) * jitter * 0.3
  const left = fractalBolt(x1, y1, mx, my, depth + 1, maxDepth, jitter * 0.55)
  const right = fractalBolt(mx, my, x2, y2, depth + 1, maxDepth, jitter * 0.55)
  return [...left, ...right.slice(1)]
}

function makeBolt() {
  const startX = currentW * 0.15 + Math.random() * currentW * 0.7
  const endX = startX + (Math.random() - 0.5) * currentW * 0.3
  const endY = currentH * (0.5 + Math.random() * 0.5)
  const mainPath = fractalBolt(startX, -20, endX, endY, 0, 6, currentW * 0.25)
  const branches: any[] = []
  const branchCount = 4 + Math.floor(Math.random() * 6)
  for (let b = 0; b < branchCount; b++) {
    const idx = Math.floor(Math.random() * (mainPath.length - 2)) + 1
    const origin = mainPath[idx]
    const angle = (Math.random() - 0.5) * Math.PI * 0.8 + Math.PI * 0.15
    const len = 60 + Math.random() * 180
    const bx = origin.x + Math.cos(angle) * len * (Math.random() > 0.5 ? 1 : -1)
    const by = origin.y + Math.abs(Math.sin(angle)) * len
    const branchPath = fractalBolt(origin.x, origin.y, bx, by, 0, 4, 40)
    const subs: any[] = []
    const subCount = Math.floor(Math.random() * 3)
    for (let s = 0; s < subCount; s++) {
      if (branchPath.length < 3) continue
      const si = Math.floor(Math.random() * (branchPath.length - 2)) + 1
      const so = branchPath[si]
      const sa = (Math.random() - 0.5) * Math.PI * 0.6
      const sl = 20 + Math.random() * 60
      const sx = so.x + Math.cos(sa) * sl * (Math.random() > 0.5 ? 1 : -1)
      const sy = so.y + Math.abs(Math.sin(sa)) * sl
      subs.push(fractalBolt(so.x, so.y, sx, sy, 0, 3, 15))
    }
    branches.push({ path: branchPath, subs })
  }
  const strokes: any[] = []
  let strokeTime = 0
  const strokeCount = 2 + Math.floor(Math.random() * 3)
  for (let s = 0; s < strokeCount; s++) {
    strokes.push({ delay: strokeTime, alpha: s === 0 ? 1 : 0.4 + Math.random() * 0.4 })
    strokeTime += 30 + Math.random() * 80
  }
  return { mainPath, branches, strokes, born: performance.now() }
}

function drawPath(path: any[], alpha: number, width: number, glow: number) {
  if (!ctx) return
  if (path.length < 2 || alpha <= 0) return
  if (glow > 0) {
    ctx.save()
    ctx.shadowColor = `rgba(180,180,255,${alpha * 0.8})`
    ctx.shadowBlur = glow
    ctx.strokeStyle = `rgba(200,200,255,${alpha * 0.3})`
    ctx.lineWidth = width * 3
    ctx.beginPath()
    path.forEach((p, i) => (i === 0 ? ctx!.moveTo(p.x, p.y) : ctx!.lineTo(p.x, p.y)))
    ctx.stroke()
    ctx.restore()
  }
  ctx.strokeStyle = `rgba(255,255,255,${alpha})`
  ctx.lineWidth = width
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.beginPath()
  path.forEach((p, i) => (i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y)))
  ctx.stroke()
}

function animate(t: number) {
  if (!ctx) return
  ctx.fillStyle = 'rgba(2,10,2,0.1)'
  ctx.fillRect(0, 0, currentW, currentH)
  if (flashAlpha > 0) {
    ctx.fillStyle = `rgba(180,180,255,${flashAlpha})`
    ctx.fillRect(0, 0, currentW, currentH)
    flashAlpha *= 0.82
    if (flashAlpha < 0.003) flashAlpha = 0
  }
  ctx.font = fontSize + 'px Consolas'
  for (let i = 0; i < cols; i++) {
    const ch = chars[Math.floor(Math.random() * chars.length)]
    const x = i * fontSize
    const y = drops[i] * fontSize
    const bright = Math.random() > 0.95
    ctx.fillStyle = bright ? '#0f0' : `rgba(0,${150 + Math.random() * 105},0,${0.4 + Math.random() * 0.4})`
    ctx.fillText(ch, x, y)
    if (y > currentH && Math.random() > 0.98) drops[i] = 0
    drops[i] += 0.5 + Math.random() * 0.5
  }
  if (t > nextBolt) {
    bolts.push(makeBolt())
    flashAlpha = 0.05 + Math.random() * 0.05
    nextBolt = t + 300 + Math.random() * 700
    if (Math.random() > 0.5) setTimeout(() => {
      bolts.push(makeBolt())
      flashAlpha = 0.03 + Math.random() * 0.04
    }, 100 + Math.random() * 200)
  }
  bolts = bolts.filter((b) => t - b.born < 500)
  for (const b of bolts) {
    const age = t - b.born
    let a = 0
    for (const stroke of b.strokes) {
      const strokeAge = age - stroke.delay
      if (strokeAge >= 0 && strokeAge < 200) a = Math.max(a, stroke.alpha * (1 - strokeAge / 200))
    }
    if (a <= 0) continue
    drawPath(b.mainPath, a, 2.5, 40)
    for (const br of b.branches) {
      drawPath(br.path, a * 0.6, 1.2, 20)
      for (const sub of br.subs) drawPath(sub, a * 0.35, 0.8, 10)
    }
  }
  animId = requestAnimationFrame(animate)
}

function startBgAnimation() {
  const canvas = bgCanvas.value
  if (!canvas) return
  const c = canvas.getContext('2d')
  if (!c) return
  ctx = c
  
  // 移除可能存在的固定尺寸属性
  canvas.removeAttribute('width')
  canvas.removeAttribute('height')
  
  resizeCanvas()
  bolts = []
  nextBolt = performance.now() + 1000
  flashAlpha = 0
  animId = requestAnimationFrame(animate)
  window.addEventListener('resize', resizeCanvas)
}

function stopBgAnimation() {
  if (animId) cancelAnimationFrame(animId)
  window.removeEventListener('resize', resizeCanvas)
}

onMounted(() => {
  const savedUsername = localStorage.getItem('aios_saved_username')
  const savedPassword = localStorage.getItem('aios_saved_password')
  if (savedUsername) {
    username.value = savedUsername
    password.value = savedPassword || ''
  }
  nextTick(() => {
    startBgAnimation()
    // 确保 canvas 尺寸正确，立即再调用一次 resizeCanvas
    setTimeout(() => {
      resizeCanvas()
    }, 0)
  })
})

onUnmounted(() => stopBgAnimation())

async function handleLogin() {
  errorMsg.value = ''
  if (!username.value.trim()) { errorMsg.value = '请输入用户名'; return }
  if (!password.value) { errorMsg.value = '请输入密码'; return }
  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username.value.trim(), password: password.value })
    })
    const data = await res.json()
    if (data.ok && data.data) {
      const { token, username: name, is_admin } = data.data
      localStorage.setItem('aios_token', token)
      localStorage.setItem('aios_username', name)
      localStorage.setItem('aios_is_admin', String(is_admin))
      if (rememberMe.value) {
        localStorage.setItem('aios_saved_username', username.value.trim())
        localStorage.setItem('aios_saved_password', password.value)
      } else {
        localStorage.removeItem('aios_saved_username')
        localStorage.removeItem('aios_saved_password')
      }
      router.replace('/workspace')
    } else { errorMsg.value = data.msg || data.error || '登录失败，请检查用户名和密码' }
  } catch (e: any) { errorMsg.value = '网络错误，无法连接服务器' } finally { loading.value = false }
}

function onKeydown(e: KeyboardEvent) { if (e.key === 'Enter' && !loading.value) handleLogin() }
</script>

<template>
  <div class="login-page" @keydown="onKeydown">
    <canvas ref="bgCanvas" class="bg-canvas"></canvas>
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" fill="url(#login-bolt)" />
            <defs>
              <linearGradient id="login-bolt" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
                <stop offset="0%" stop-color="#06b6d4" />
                <stop offset="100%" stop-color="#a78bfa" />
              </linearGradient>
            </defs>
          </svg>
        </div>
        <h1 class="login-title">AI-OS</h1>
        <p class="login-subtitle">智能操作系统</p>
      </div>
      <div class="login-form">
        <div class="form-group">
          <label class="form-label">用户名</label>
          <input v-model="username" type="text" class="form-input" placeholder="请输入用户名" autocomplete="username" :disabled="loading" />
        </div>
        <div class="form-group">
          <label class="form-label">密码</label>
          <input v-model="password" type="password" class="form-input" placeholder="请输入密码" autocomplete="current-password" :disabled="loading" />
        </div>
        <label class="remember-row">
          <input v-model="rememberMe" type="checkbox" class="remember-checkbox" />
          <span class="remember-text">记住密码</span>
        </label>
        <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>
        <button class="login-btn" :disabled="loading" @click="handleLogin">
          <span v-if="loading" class="btn-loader"></span>
          <span v-else>登 录</span>
        </button>
      </div>
    </div>
    <div class="login-footer">AI-OS v1.0.0</div>
  </div>
</template>

<style scoped>
.login-page {
  height: 100vh;
  width: 100vw;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: #020a02;
  position: relative;
  overflow: hidden;
}

.bg-canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100% !important;
  height: 100% !important;
  z-index: 0;
  display: block;
}

.login-card {
  width: 380px;
  padding: 40px 36px 32px;
  border-radius: 16px;
  background: rgba(30, 41, 59, 0.65);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 8px 32px rgba(0,0,0,0.3), 0 0 0 1px rgba(255,255,255,0.03) inset;
  position: relative;
  z-index: 1;
}

.login-header { text-align: center; margin-bottom: 32px; }
.login-logo {
  width: 52px; height: 52px;
  border-radius: 14px;
  background: linear-gradient(135deg, rgba(6,182,212,0.15), rgba(167,139,250,0.15));
  display: flex; align-items: center; justify-content: center;
  margin: 0 auto 16px;
}
.login-title { font-size: 22px; font-weight: 700; color: #e2e8f0; letter-spacing: 4px; margin-bottom: 6px; }
.login-subtitle { font-size: 13px; color: #64748b; letter-spacing: 1px; }

.login-form { display: flex; flex-direction: column; gap: 18px; }

.form-group { display: flex; flex-direction: column; gap: 6px; }
.form-label { font-size: 12px; font-weight: 500; color: #94a3b8; letter-spacing: 0.5px; }
.form-input {
  height: 42px; padding: 0 14px; border-radius: 8px;
  border: 1px solid rgba(255,255,255,0.08);
  background: rgba(15,23,42,0.6);
  color: #e2e8f0; font-size: 14px; outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.form-input::placeholder { color: #47548b; }
.form-input:focus { border-color: rgba(99,102,241,0.5); box-shadow: 0 0 0 3px rgba(99,102,241,0.1); }
.form-input:disabled { opacity: 0.6; cursor: not-allowed; }

.remember-row { display: flex; align-items: center; gap: 8px; cursor: pointer; user-select: none; }
.remember-checkbox { width: 15px; height: 15px; border-radius: 4px; accent-color: #6366f1; cursor: pointer; }
.remember-text { font-size: 13px; color: #94a3b8; }

.error-msg {
  font-size: 13px; color: #f87171;
  padding: 8px 12px; border-radius: 6px;
  background: rgba(248,113,113,0.08);
  border: 1px solid rgba(248,113,113,0.15);
}

.login-btn {
  height: 42px; border-radius: 8px; border: none;
  background: linear-gradient(135deg, #6366f1, #a78bfa);
  color: #fff; font-size: 15px; font-weight: 600; letter-spacing: 2px;
  cursor: pointer; transition: opacity 0.2s, transform 0.15s;
  display: flex; align-items: center; justify-content: center;
}
.login-btn:hover:not(:disabled) { opacity: 0.92; transform: translateY(-1px); }
.login-btn:active:not(:disabled) { transform: translateY(0); }
.login-btn:disabled { opacity: 0.6; cursor: not-allowed; }

.btn-loader {
  width: 18px; height: 18px;
  border: 2px solid rgba(255,255,255,0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: login-spin 0.7s linear infinite;
}
@keyframes login-spin { to { transform: rotate(360deg); } }

.login-footer {
  position: absolute; bottom: 24px;
  font-size: 11px; color: #334155;
  letter-spacing: 1px; z-index: 1;
}
</style>
