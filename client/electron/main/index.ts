import { app, ipcMain } from 'electron'
import { logger } from './logger'
import { windowManager } from './window-manager'
import { healthCheck, ensureService } from './service-manager'

app.whenReady().then(async () => {
  logger.info('========== AI-OS Client Starting ==========')
  logger.info(`Platform: ${process.platform}`)
  logger.info(`Packaged: ${app.isPackaged}`)

  const result = await ensureService()
  logger.info(`Service ensure result: ok=${result.ok}, error=${result.error || 'none'}`)

  windowManager.create()

  ipcMain.handle('agent:health', () => healthCheck())
  ipcMain.handle('window:minimize', () => windowManager.minimize())
  ipcMain.handle('window:maximize', () => windowManager.maximize())
  ipcMain.handle('window:close', () => windowManager.close())
  ipcMain.handle('window:isMaximized', () => windowManager.isMaximized())
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
