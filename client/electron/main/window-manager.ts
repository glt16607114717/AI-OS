import { BrowserWindow } from 'electron'
import path from 'path'
import { logger } from './logger'

class WindowManager {
  private win: BrowserWindow | null = null

  create(): BrowserWindow {
    this.win = new BrowserWindow({
      width: 1000,
      height: 700,
      minWidth: 800,
      minHeight: 500,
      frame: false,
      title: 'AI-OS',
      webPreferences: {
        preload: path.join(__dirname, '../preload/index.js'),
        contextIsolation: true,
        nodeIntegration: false,
      },
    })

    if (process.env.VITE_DEV_SERVER_URL) {
      this.win.loadURL(process.env.VITE_DEV_SERVER_URL)
      this.win.webContents.openDevTools()
    } else {
      this.win.loadFile(path.join(__dirname, '../dist/index.html'))
    }

    this.win.on('closed', () => {
      this.win = null
      logger.info('Window closed')
    })

    logger.info('Window created')
    return this.win
  }

  getWindow(): BrowserWindow | null {
    return this.win
  }

  isMaximized(): boolean {
    return this.win?.isMaximized() ?? false
  }

  minimize(): void {
    this.win?.minimize()
  }

  maximize(): void {
    if (!this.win) return
    if (this.win.isMaximized()) {
      this.win.unmaximize()
    } else {
      this.win.maximize()
    }
  }

  close(): void {
    this.win?.close()
  }
}

export const windowManager = new WindowManager()
