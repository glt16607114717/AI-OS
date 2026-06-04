import { app, BrowserWindow, ipcMain, Tray, Menu, nativeImage } from 'electron'
import * as path from 'path'
import * as fs from 'fs'
import * as http from 'http'
import { execFile } from 'child_process'
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
    width: 800,
    height: 600,
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

/**
 * 检查并注册 Voice Worker 计划任务。
 * 计划任务让 voice_worker 跑在用户 Session 1，解决 WinSW（Session 0）无法读取键鼠输入的问题。
 * 首次注册会弹一次 UAC 确认框，之后不再弹。
 */
function ensureVoiceWorkerTask() {
  if (!app.isPackaged) return // 开发模式不注册

  const taskName = 'AI-OS-Voice-Worker'

  // 先检查计划任务是否已注册
  execFile('schtasks', ['/Query', '/TN', taskName], (error) => {
    if (!error) {
      console.log(`[VoiceWorker] 计划任务 ${taskName} 已注册，跳过`)
      return
    }

    // 任务未注册，需要用 elevate.exe 提权注册
    const resourcesDir = process.resourcesPath
    const elevateExe = path.join(resourcesDir, 'elevate.exe')
    const ps1Script = path.join(resourcesDir, 'scripts', 'register-voice-task.ps1')

    if (!fs.existsSync(elevateExe)) {
      console.warn(`[VoiceWorker] elevate.exe 不存在: ${elevateExe}`)
      return
    }
    if (!fs.existsSync(ps1Script)) {
      console.warn(`[VoiceWorker] 注册脚本不存在: ${ps1Script}`)
      return
    }

    console.log(`[VoiceWorker] 计划任务未注册，正在通过 UAC 提权注册...`)

    // elevate.exe 弹 UAC 框，然后以管理员权限执行 PowerShell 脚本
    execFile(elevateExe, [
      'powershell.exe',
      '-WindowStyle', 'Hidden',
      '-ExecutionPolicy', 'Bypass',
      '-NonInteractive',
      '-NoProfile',
      '-File', ps1Script,
    ], (err, stdout, stderr) => {
      if (err) {
        console.warn(`[VoiceWorker] 注册失败（用户可能取消了 UAC）: ${err.message}`)
      } else {
        console.log(`[VoiceWorker] 注册完成: ${stdout}`)
      }
    })
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
    ensureVoiceWorkerTask()

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
