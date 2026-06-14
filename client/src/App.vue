<template>
  <div class="shell">
    <div class="titlebar" :class="{ 'titlebar-dark': !showDashboard }">
      <div class="titlebar-left">
        <div class="app-icon"></div>
        <span class="app-name">AI-OS</span>
      </div>
      <div class="titlebar-actions">
        <button class="tb-btn" @click="windowAiOS.windowMinimize()">
          <svg width="10" height="1" viewBox="0 0 10 1"><rect width="10" height="1" fill="currentColor"/></svg>
        </button>
        <button class="tb-btn" @click="windowAiOS.windowMaximize()">
          <svg width="10" height="10" viewBox="0 0 10 10">
            <rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1"/>
          </svg>
        </button>
        <button class="tb-btn tb-btn-close" @click="windowAiOS.windowClose()">
          <svg width="10" height="10" viewBox="0 0 10 10">
            <line x1="0" y1="0" x2="10" y2="10" stroke="currentColor" stroke-width="1.2"/>
            <line x1="10" y1="0" x2="0" y2="10" stroke="currentColor" stroke-width="1.2"/>
          </svg>
        </button>
      </div>
    </div>

    <div class="content">
      <!-- Splash: matrix rain + lightning, shown for 3s -->
    <div v-if="!showDashboard" class="splash-page">
      <canvas ref="splashCanvas"></canvas>
      <h1 class="splash-title">AI-OS</h1>
    </div>

    <!-- Setup Wizard: shown when Python env not ready -->
    <div v-else-if="needSetup" class="loading-page">
      <SetupWizard
        :current-step="setupStep"
        :step-name="setupStepName"
        :total-steps="5"
        :percent="setupPercent"
        :detail="setupDetail"
        :error="setupError"
        :done="setupDone"
        @retry="runSetup"
      />
    </div>

    <!-- Dashboard -->
    <div v-else class="dashboard">
        <div class="dash-body">
          <router-view />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import SetupWizard from './setup/SetupWizard.vue'

const windowAiOS = window.aiOS

const showDashboard = ref(false)
const splashCanvas = ref<HTMLCanvasElement | null>(null)

// Setup state
const needSetup = ref(false)
const setupStep = ref(0)
const setupStepName = ref('')
const setupPercent = ref(0)
const setupDetail = ref('')
const setupError = ref('')
const setupDone = ref(false)

let splashAnimId = 0

function startSplashAnimation() {
  const canvas = splashCanvas.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  canvas.width = window.innerWidth * dpr
  canvas.height = window.innerHeight * dpr
  ctx.scale(dpr, dpr)
  const w = window.innerWidth
  const h = window.innerHeight

  // Matrix rain
  const fontSize = 16
  const cols = Math.ceil(w / fontSize)
  const drops = Array(cols).fill(0).map(() => Math.random() * -80)
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@#$%&*<>/\\|{}[]01'

  // Lightning
  let bolts: any[] = []
  let nextBolt = performance.now() + 500
  let flashAlpha = 0

  function fractalBolt(x1: number, y1: number, x2: number, y2: number, depth: number, maxDepth: number, jitter: number): {x:number;y:number}[] {
    if (depth >= maxDepth) return [{ x: x1, y: y1 }, { x: x2, y: y2 }]
    const mx = (x1 + x2) / 2 + (Math.random() - 0.5) * jitter
    const my = (y1 + y2) / 2 + (Math.random() - 0.5) * jitter * 0.3
    const left = fractalBolt(x1, y1, mx, my, depth + 1, maxDepth, jitter * 0.55)
    const right = fractalBolt(mx, my, x2, y2, depth + 1, maxDepth, jitter * 0.55)
    return [...left, ...right.slice(1)]
  }

  function makeBolt() {
    const startX = w * 0.15 + Math.random() * w * 0.7
    const endX = startX + (Math.random() - 0.5) * w * 0.3
    const endY = h * (0.5 + Math.random() * 0.5)
    const mainPath = fractalBolt(startX, -20, endX, endY, 0, 6, w * 0.25)
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
    if (path.length < 2 || alpha <= 0) return
    if (glow > 0) {
      ctx!.save()
      ctx!.shadowColor = `rgba(180,180,255,${alpha * 0.8})`
      ctx!.shadowBlur = glow
      ctx!.strokeStyle = `rgba(200,200,255,${alpha * 0.3})`
      ctx!.lineWidth = width * 3
      ctx!.beginPath()
      path.forEach((p, i) => i === 0 ? ctx!.moveTo(p.x, p.y) : ctx!.lineTo(p.x, p.y))
      ctx!.stroke()
      ctx!.restore()
    }
    ctx!.strokeStyle = `rgba(255,255,255,${alpha})`
    ctx!.lineWidth = width
    ctx!.lineCap = 'round'
    ctx!.lineJoin = 'round'
    ctx!.beginPath()
    path.forEach((p, i) => i === 0 ? ctx!.moveTo(p.x, p.y) : ctx!.lineTo(p.x, p.y))
    ctx!.stroke()
  }

  function animate(t: number) {
    ctx!.fillStyle = 'rgba(2,10,2,0.1)'
    ctx!.fillRect(0, 0, w, h)

    if (flashAlpha > 0) {
      ctx!.fillStyle = `rgba(180,180,255,${flashAlpha})`
      ctx!.fillRect(0, 0, w, h)
      flashAlpha *= 0.82
      if (flashAlpha < 0.003) flashAlpha = 0
    }

    // Matrix rain
    ctx!.font = fontSize + 'px Consolas'
    for (let i = 0; i < cols; i++) {
      const ch = chars[Math.floor(Math.random() * chars.length)]
      const x = i * fontSize
      const y = drops[i] * fontSize
      const bright = Math.random() > 0.95
      ctx!.fillStyle = bright ? '#0f0' : `rgba(0,${150 + Math.random()*105},0,${0.4 + Math.random()*0.4})`
      ctx!.fillText(ch, x, y)
      if (y > h && Math.random() > 0.98) drops[i] = 0
      drops[i] += 0.5 + Math.random() * 0.5
    }

    // Lightning
    if (t > nextBolt) {
      bolts.push(makeBolt())
      flashAlpha = 0.05 + Math.random() * 0.05
      nextBolt = t + 300 + Math.random() * 700
      if (Math.random() > 0.5) {
        setTimeout(() => { bolts.push(makeBolt()); flashAlpha = 0.03 + Math.random() * 0.04 }, 100 + Math.random() * 200)
      }
    }

    bolts = bolts.filter(b => t - b.born < 500)
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

    splashAnimId = requestAnimationFrame(animate)
  }

  splashAnimId = requestAnimationFrame(animate)
}

function stopSplashAnimation() {
  if (splashAnimId) { cancelAnimationFrame(splashAnimId); splashAnimId = 0 }
}

onMounted(async () => {
  showDashboard.value = false
  needSetup.value = false

  // 检查是否需要环境配置（health 检测）
  let setupRequired = false
  if (windowAiOS?.checkSetupNeeded) {
    try {
      setupRequired = await windowAiOS.checkSetupNeeded()
    } catch {
      setupRequired = true
    }
  }

  if (setupRequired) {
    // 需要配置：直接进入 SetupWizard，不显示 splash
    showDashboard.value = true
    needSetup.value = true
    runSetup()
  } else {
    // 不需要配置：显示 splash 动画后进入 Dashboard
    await nextTick()
    startSplashAnimation()
    setTimeout(() => {
      stopSplashAnimation()
      showDashboard.value = true
    }, 7000)
  }
})

async function runSetup() {
  setupError.value = ''
  setupDone.value = false

  try {
    const result = await windowAiOS.runSetup((info: any) => {
      setupStep.value = info.stepIndex ?? 0
      setupStepName.value = info.step ?? ''
      setupPercent.value = info.percent ?? 0
      setupDetail.value = info.detail ?? ''
      if (info.error) {
        setupError.value = info.error
      }
    })

    if (result?.ok) {
      setupDone.value = true
      setTimeout(() => {
        needSetup.value = false
      }, 2000)
    } else if (result?.error) {
      setupError.value = result.error
    }
  } catch (e: any) {
    setupError.value = e.message || '环境配置失败'
  }
}

onUnmounted(() => {
  stopSplashAnimation()
})
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: 'Segoe UI', -apple-system, BlinkMacSystemFont, 'PingFang SC', 'Microsoft YaHei', sans-serif;
  overflow: hidden;
  -webkit-font-smoothing: antialiased;
}

.shell {
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.titlebar {
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  -webkit-app-region: drag;
  flex-shrink: 0;
}

.titlebar-dark {
  background: #09090b;
  border-bottom: 1px solid #1a1a1f;
}

.titlebar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 14px;
}

.app-icon {
  width: 16px;
  height: 16px;
  border-radius: 3px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
}

.app-name {
  font-size: 12px;
  font-weight: 600;
  color: #606266;
  letter-spacing: 1.5px;
  text-transform: uppercase;
}

.titlebar-dark .app-name {
  color: #a1a1aa;
}

.titlebar-actions {
  display: flex;
  -webkit-app-region: no-drag;
}

.tb-btn {
  width: 46px;
  height: 38px;
  border: none;
  background: transparent;
  color: #909399;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}

.tb-btn:hover {
  background: #f5f7fa;
  color: #303133;
}

.titlebar-dark .tb-btn {
  color: #71717a;
}

.titlebar-dark .tb-btn:hover {
  background: #18181b;
  color: #e4e4e7;
}

.tb-btn-close:hover {
  background: #dc2626;
  color: #ffffff;
}

.content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.loading-page {
  flex: 1;
  background: #09090b;
}

/* Splash page - matrix rain + lightning */
.splash-page {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: #020a02;
  position: relative;
  overflow: hidden;
}

.splash-page canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}

.splash-title {
  font-size: 80px;
  font-weight: 900;
  letter-spacing: 24px;
  color: #0f0;
  position: relative;
  z-index: 1;
  font-family: 'Consolas', 'Courier New', monospace;
  text-shadow: 0 0 20px rgba(0,255,0,0.6), 0 0 60px rgba(0,255,0,0.2);
}

/* Dashboard */
.dashboard {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}
.dash-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>
