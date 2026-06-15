import { app, BrowserWindow, ipcMain, Tray, Menu, nativeImage } from 'electron'
import * as path from 'path'
import * as http from 'http'
import { SetupManager, SetupProgress } from './setup-manager'

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null
let setupManager: SetupManager | null = null

const LOCAL_AGENT_PORT = 18732

// ─── 本地 Agent 通信 ────────────────────────────────

function getSetupManager(): SetupManager {
  if (!setupManager) {
    const appDir = app.isPackaged
      ? path.dirname(app.getPath('exe'))
      : path.join(__dirname, '..', '..')
    setupManager = new SetupManager(appDir, (info: SetupProgress) => {
      // 转发进度到渲染进程
      mainWindow?.webContents.send('setup:progress', info)
    })
  }
  return setupManager
}

function agentHealthCheck(): Promise<{ ok: boolean; uptime?: number; version?: string; pid?: number }> {
  return new Promise((resolve) => {
    const req = http.get(`http://127.0.0.1:${LOCAL_AGENT_PORT}/health`, { timeout: 2000 }, (res) => {
      let data = ''
      res.on('data', (chunk: string) => { data += chunk })
      res.on('end', () => {
        try {
          const json = JSON.parse(data)
          resolve({ ok: json.ok === true, uptime: json.uptime, version: json.version, pid: json.pid })
        } catch {
          resolve({ ok: false })
        }
      })
    })
    req.on('error', () => resolve({ ok: false }))
    req.on('timeout', () => { req.destroy(); resolve({ ok: false }) })
  })
}

/**
 * 调用本地 Python Agent 的 /api/voice 接口
 */
function agentRequest(action: string, params: Record<string, unknown>): Promise<any> {
  return new Promise((resolve, reject) => {
    const body = JSON.stringify({ action, payload: params })
    const req = http.request(
      `http://127.0.0.1:${LOCAL_AGENT_PORT}/api/voice`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': Buffer.byteLength(body),
        },
        timeout: 10000,
      },
      (res) => {
        let data = ''
        res.on('data', (chunk: string) => { data += chunk })
        res.on('end', () => {
          try {
            resolve(JSON.parse(data))
          } catch {
            resolve({ ok: false, error: 'Invalid response' })
          }
        })
      },
    )
    req.on('error', (e) => resolve({ ok: false, error: e.message }))
    req.on('timeout', () => { req.destroy(); resolve({ ok: false, error: 'timeout' }) })
    req.write(body)
    req.end()
  })
}

async function agentRestart(): Promise<{ ok: boolean; error?: string }> {
  try {
    // 先尝试优雅关闭
    await new Promise<void>((resolve) => {
      const body = JSON.stringify({})
      const req = http.request(
        `http://127.0.0.1:${LOCAL_AGENT_PORT}/shutdown`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) },
          timeout: 3000,
        },
        () => resolve(),
      )
      req.on('error', () => resolve())
      req.on('timeout', () => { req.destroy(); resolve() })
      req.write(body)
      req.end()
    })

    // 等待 3 秒让进程退出
    await new Promise(r => setTimeout(r, 3000))

    // 通过计划任务重新启动
    const { exec } = require('child_process')
    exec('schtasks.exe /Run /TN "AI-OS-Agent"', (err: Error | null) => {
      if (err) {
        // 计划任务失败，尝试直接 spawn
        const mgr = getSetupManager()
        // spawn 会在 startService 里处理
      }
    })

    // 等待健康检查通过
    for (let i = 0; i < 15; i++) {
      await new Promise(r => setTimeout(r, 1000))
      const health = await agentHealthCheck()
      if (health.ok) return { ok: true }
    }
    return { ok: false, error: 'Agent 启动超时' }
  } catch (e: any) {
    return { ok: false, error: e.message }
  }
}

// ─── 窗口管理 ────────────────────────────────────────

function createTray() {
  const iconPath = app.isPackaged
    ? path.join(process.resourcesPath, 'icon.ico')
    : path.join(__dirname, '..', '..', 'public', 'icon.ico')
  const icon = nativeImage.createFromPath(iconPath)
  tray = new Tray(icon.resize({ width: 16, height: 16 }))
  tray.setToolTip('AI-OS')

  const contextMenu = Menu.buildFromTemplate([
    {
      label: '显示 AI-OS',
      click: () => {
        if (!mainWindow) createWindow()
        mainWindow?.show()
        mainWindow?.focus()
      },
    },
    { type: 'separator' },
    {
      label: '退出',
      click: () => {
        tray?.destroy()
        tray = null
        app.quit()
      },
    },
  ])

  tray.setContextMenu(contextMenu)

  tray.on('double-click', () => {
    if (!mainWindow) createWindow()
    mainWindow?.show()
    mainWindow?.focus()
  })
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1100,
    height: 700,
    frame: false,
    resizable: true,
    minWidth: 640,
    minHeight: 480,
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  if (app.isPackaged) {
    mainWindow.loadFile(path.join(__dirname, '..', '..', 'dist', 'index.html'))
  } else {
    mainWindow.loadURL('http://localhost:5173')
  }

  // 绕过系统代理，让本地请求直连
  mainWindow.webContents.session.setProxy({ proxyRules: 'direct://', pacScript: '', proxyBypassRules: '<-loopback>' })

  mainWindow.on('close', (e) => {
    e.preventDefault()
    mainWindow?.hide()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })

  // 开发模式打开 DevTools
  if (!app.isPackaged) {
    mainWindow.webContents.openDevTools({ mode: 'bottom' })
  }
}

// ─── IPC 注册 ────────────────────────────────────────

function registerIpcHandlers() {
  // 窗口控制
  ipcMain.handle('window:minimize', () => mainWindow?.minimize())
  ipcMain.handle('window:maximize', () => {
    if (mainWindow?.isMaximized()) {
      mainWindow.unmaximize()
    } else {
      mainWindow?.maximize()
    }
  })
  ipcMain.handle('window:close', () => mainWindow?.close())
  ipcMain.handle('window:is-maximized', () => mainWindow?.isMaximized() ?? false)

  // 本地 Agent 健康
  ipcMain.handle('agent:health', async () => {
    return await agentHealthCheck()
  })

  // 本地 Agent 请求（语音等）
  ipcMain.handle('agent:request', async (_event, action: string, params: Record<string, unknown>) => {
    return await agentRequest(action, params)
  })

  // 本地 Agent 重启
  ipcMain.handle('agent:restart', async () => {
    return await agentRestart()
  })

  // Setup 检查（只检测 health 接口，返回 true = 需要配置）
  ipcMain.handle('setup:check', async () => {
    const health = await agentHealthCheck()
    return !health.ok
  })

  // Setup 执行
  ipcMain.handle('setup:run', async () => {
    const mgr = getSetupManager()
    try {
      await mgr.runFullSetup()
      return { ok: true }
    } catch (e: any) {
      return { ok: false, error: e.message }
    }
  })
}

// ─── 应用启动 ────────────────────────────────────────

const gotTheLock = app.requestSingleInstanceLock()

if (!gotTheLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    if (!mainWindow) createWindow()
    mainWindow?.show()
    mainWindow?.focus()
  })

  app.whenReady().then(async () => {
    registerIpcHandlers()
    createTray()
    createWindow()

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        createWindow()
      }
    })
  })
}

app.on('before-quit', () => {
  mainWindow?.removeAllListeners('close')
})
