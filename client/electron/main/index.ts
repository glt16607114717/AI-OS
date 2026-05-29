import { app, BrowserWindow, ipcMain } from 'electron'
import path from 'path'
import http from 'http'
import { execFile } from 'child_process'
import fs from 'fs'

let mainWindow: BrowserWindow | null = null

const IS_WIN = process.platform === 'win32'
const PORT = 18731
const SERVICE_NAME = 'AI-OS-Agent'

const USER_DATA_DIR = IS_WIN
  ? path.join(process.env.USERPROFILE || '', 'AI-OS')
  : path.join(process.env.HOME || '', 'AI-OS')
const LOG_FILE = path.join(USER_DATA_DIR, 'client.log')

function log(msg: string) {
  const ts = new Date().toISOString()
  const line = `[${ts}] ${msg}\n`
  try {
    if (!fs.existsSync(USER_DATA_DIR)) fs.mkdirSync(USER_DATA_DIR, { recursive: true })
    fs.appendFileSync(LOG_FILE, line)
  } catch {}
  console.log(line.trim())
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1000,
    height: 700,
    frame: false,
    webPreferences: {
      preload: path.join(__dirname, '../preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  if (process.env.VITE_DEV_SERVER_URL) {
    mainWindow.loadURL(process.env.VITE_DEV_SERVER_URL)
    mainWindow.webContents.openDevTools()
  } else {
    mainWindow.loadFile(path.join(__dirname, '../dist/index.html'))
  }

  mainWindow.on('closed', () => { mainWindow = null })
}

function checkAgentHealth(): Promise<{ ok: boolean; uptime?: number; version?: string }> {
  return new Promise((resolve) => {
    const req = http.request({
      hostname: '127.0.0.1',
      port: PORT,
      path: '/health',
      method: 'GET',
      timeout: 3000,
    }, (res) => {
      let body = ''
      res.on('data', (chunk: Buffer) => { body += chunk.toString() })
      res.on('end', () => {
        try {
          resolve({ ok: true, ...JSON.parse(body) })
        } catch {
          resolve({ ok: true })
        }
      })
    })
    req.on('error', () => resolve({ ok: false }))
    req.on('timeout', () => { req.destroy(); resolve({ ok: false }) })
    req.end()
  })
}

function getResourcesPath(...segments: string[]): string {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, ...segments)
  }
  return path.join(app.getAppPath(), ...segments)
}

async function ensureService(): Promise<{ ok: boolean; error?: string }> {
  const health = await checkAgentHealth()
  if (health.ok) {
    log('Agent service already running')
    return { ok: true }
  }

  if (!IS_WIN) {
    return { ok: false, error: 'Unsupported platform' }
  }

  log('Agent service not running, trying to start via WinSW...')
  const winswExe = getResourcesPath('winsw', 'ai-os-agent.exe')

  if (!fs.existsSync(winswExe)) {
    log(`WinSW not found: ${winswExe}`)
    return { ok: false, error: 'WinSW executable not found' }
  }

  return new Promise((resolve) => {
    execFile(winswExe, ['start'], { timeout: 15000, windowsHide: true }, (err, stdout, stderr) => {
      if (err) {
        log(`WinSW start failed: ${err.message}`)
        resolve({ ok: false, error: err.message })
        return
      }
      log(`WinSW start output: ${stdout || stderr}`)
      resolve({ ok: true })
    })
  })
}

app.whenReady().then(async () => {
  log('========== AI-OS Client Starting ==========')
  log(`Platform: ${process.platform}`)
  log(`Packaged: ${app.isPackaged}`)

  const svcResult = await ensureService()
  log(`Service ensure result: ok=${svcResult.ok}, error=${svcResult.error || 'none'}`)

  createWindow()

  ipcMain.handle('agent:health', () => checkAgentHealth())
  ipcMain.handle('window:minimize', () => mainWindow?.minimize())
  ipcMain.handle('window:maximize', () => {
    if (mainWindow?.isMaximized()) {
      mainWindow?.unmaximize()
    } else {
      mainWindow?.maximize()
    }
  })
  ipcMain.handle('window:close', () => mainWindow?.close())
  ipcMain.handle('window:isMaximized', () => mainWindow?.isMaximized() ?? false)
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
