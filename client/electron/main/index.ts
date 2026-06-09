import { app, BrowserWindow, ipcMain, Tray, Menu, nativeImage } from 'electron'
import { spawn } from 'child_process'
import * as path from 'path'
import * as fs from 'fs'
import * as http from 'http'
import { SetupManager } from './setup-manager'

let mainWindow: BrowserWindow | null = null
let tray: Tray | null = null

function getAppDir(): string {
  if (app.isPackaged) {
    const exePath = app.getPath('exe')
    const dir = path.dirname(exePath)
    const logLine = `[getAppDir] exe=${exePath} dir=${dir}\n`
    try { fs.appendFileSync(path.join(dir, 'setup-debug.log'), logLine) } catch {}
    return dir
  }
  return path.join(__dirname, '..', '..')
}

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

  mainWindow.on('close', (e) => {
    e.preventDefault()
    mainWindow?.hide()
  })

  mainWindow.on('closed', () => {
    mainWindow = null
  })

  // 默认打开 DevTools 用于调试
  mainWindow.webContents.openDevTools({ mode: 'bottom' })
}

function registerIpcHandlers() {
  ipcMain.handle('window:minimize', () => mainWindow?.minimize())
  ipcMain.handle('window:maximize', () => {
    if (mainWindow?.isMaximized()) {
      mainWindow.unmaximize()
    } else {
      mainWindow?.maximize()
    }
  })
  ipcMain.handle('window:close', () => mainWindow?.close())

  ipcMain.handle('agent:request', async (_event, action: string, payload: any) => {
    // Route to different endpoints based on action prefix
    let apiPath = '/api/voice'
    if (action === 'voice_download_model') apiPath = '/api/voice/download-model'
    else if (action === 'voice_model_status') apiPath = '/api/voice/model-status'
    else if (action.startsWith('llm_') || action.startsWith('chat_') || action.startsWith('skill_')) apiPath = '/api/llm'
    else if (action.startsWith('rag_')) apiPath = '/api/rag'
    else if (!action.startsWith('voice_')) apiPath = '/api'

    const method = (action === 'voice_model_status') ? 'GET' : 'POST'

    if (method === 'GET') {
      return new Promise((resolve) => {
        const req = http.get(`http://127.0.0.1:18731${apiPath}`, { timeout: 5000 }, (res) => {
          let data = ''
          res.on('data', (chunk: string) => { data += chunk })
          res.on('end', () => { try { resolve(JSON.parse(data)) } catch { resolve({ ok: false, error: 'Parse error' }) } })
        })
        req.on('error', () => resolve({ ok: false, error: 'Agent not available' }))
      })
    }

    const isEmptyPost = action === 'voice_download_model'
    const body = isEmptyPost ? '' : JSON.stringify({ action, payload: payload || {} })
    const headers: Record<string, string> = isEmptyPost
      ? { 'Content-Length': '0' }
      : { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) }
    return new Promise((resolve) => {
      const req = http.request(
        { hostname: '127.0.0.1', port: 18731, path: apiPath, method: 'POST', headers },
        (res) => {
          let data = ''
          res.on('data', (chunk: string) => { data += chunk })
          res.on('end', () => { try { resolve(JSON.parse(data)) } catch { resolve({ ok: false, error: 'Parse error' }) } })
        },
      )
      req.on('error', () => resolve({ ok: false, error: 'Agent not available' }))
      if (!isEmptyPost) req.write(body)
      req.end()
    })
  })

  ipcMain.handle('agent:health', async () => {
    try {
      return await new Promise((resolve) => {
        const req = http.get('http://127.0.0.1:18731/health', { timeout: 3000 }, (res) => {
          let data = ''
          res.on('data', (chunk: string) => { data += chunk })
          res.on('end', () => {
            try { resolve(JSON.parse(data)) } catch { resolve({ ok: false }) }
          })
        })
        req.on('error', () => resolve({ ok: false }))
        req.on('timeout', () => { req.destroy(); resolve({ ok: false }) })
      })
    } catch {
      return { ok: false }
    }
  })

  ipcMain.handle('agent:restart', async () => {
    try {
      // 1. 找到并杀掉当前 agent 进程
      const health: any = await new Promise((resolve) => {
        const req = http.get('http://127.0.0.1:18731/health', { timeout: 3000 }, (res) => {
          let data = ''
          res.on('data', (chunk: string) => { data += chunk })
          res.on('end', () => { try { resolve(JSON.parse(data)) } catch { resolve(null) } })
        })
        req.on('error', () => resolve(null))
        req.on('timeout', () => { req.destroy(); resolve(null) })
      })

      if (health?.pid) {
        try {
          process.kill(health.pid)
        } catch (e: any) {
          if (e.code !== 'ESRCH') {
            return { ok: false, error: '无法停止进程（权限不足），请以管理员身份运行' }
          }
        }
      }

      // 2. 等待端口释放
      await new Promise(r => setTimeout(r, 1500))

      // 3. 启动新进程
      const appDir = getAppDir()
      const pythonDir = path.join(process.env.PROGRAMDATA || 'C:\\ProgramData', 'AI-OS', 'runtime', 'python')
      const pythonwExe = path.join(pythonDir, 'pythonw.exe')
      const mainScript = path.join(appDir, 'resources', 'backend', 'python', 'agent', 'main.py')

      if (!fs.existsSync(pythonwExe)) {
        return { ok: false, error: `pythonw.exe 不存在: ${pythonwExe}` }
      }

      spawn(pythonwExe, ['-X', 'utf8', mainScript], {
        cwd: pythonDir,
        detached: true,
        stdio: 'ignore',
      }).unref()

      // 4. 等待新进程启动（最多 15 秒）
      for (let i = 0; i < 15; i++) {
        await new Promise(r => setTimeout(r, 1000))
        const ok = await new Promise<boolean>((resolve) => {
          const req = http.get('http://127.0.0.1:18731/health', { timeout: 2000 }, (res) => {
            let data = ''
            res.on('data', (chunk: string) => { data += chunk })
            res.on('end', () => { try { resolve(!!JSON.parse(data).ok) } catch { resolve(false) } })
          })
          req.on('error', () => resolve(false))
          req.on('timeout', () => { req.destroy(); resolve(false) })
        })
        if (ok) return { ok: true }
      }

      return { ok: false, error: '重启超时' }
    } catch (e: any) {
      return { ok: false, error: e.message }
    }
  })

  ipcMain.handle('setup:checkNeeded', async () => {
    const appDir = getAppDir()
    const manager = new SetupManager(appDir, () => {})
    if (!manager.isPythonReady()) return true

    const healthOk = await new Promise<boolean>((resolve) => {
      const req = http.get('http://127.0.0.1:18731/health', { timeout: 3000 }, (res) => {
        let data = ''
        res.on('data', (chunk: string) => { data += chunk })
        res.on('end', () => {
          try { resolve(!!JSON.parse(data).ok) } catch { resolve(false) }
        })
      })
      req.on('error', () => resolve(false))
      req.on('timeout', () => { req.destroy(); resolve(false) })
    })

    return !healthOk
  })

  ipcMain.handle('setup:run', async (_event, channel: string) => {
    const appDir = getAppDir()
    const manager = new SetupManager(appDir, (info) => {
      mainWindow?.webContents.send(channel, info)
    })
    await manager.runFullSetup()
  })
}

const gotTheLock = app.requestSingleInstanceLock()

if (!gotTheLock) {
  app.quit()
} else {
  app.on('second-instance', () => {
    if (!mainWindow) createWindow()
    mainWindow?.show()
    mainWindow?.focus()
  })

  app.whenReady().then(() => {
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
