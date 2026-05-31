import { app } from 'electron'
import http from 'http'
import { execFile } from 'child_process'
import fs from 'fs'
import path from 'path'
import { logger } from './logger'

const PORT = 18731
const SERVICE_NAME = 'AI-OS-Agent'

interface HealthResult {
  ok: boolean
  uptime?: number
  version?: string
}

interface ServiceResult {
  ok: boolean
  error?: string
}

function healthCheck(): Promise<HealthResult> {
  return new Promise((resolve) => {
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port: PORT,
        path: '/health',
        method: 'GET',
        timeout: 3000,
      },
      (res) => {
        let body = ''
        res.on('data', (chunk: Buffer) => {
          body += chunk.toString()
        })
        res.on('end', () => {
          try {
            resolve({ ok: true, ...JSON.parse(body) })
          } catch {
            resolve({ ok: true })
          }
        })
      },
    )
    req.on('error', () => resolve({ ok: false }))
    req.on('timeout', () => {
      req.destroy()
      resolve({ ok: false })
    })
    req.end()
  })
}

function getResourcesPath(...segments: string[]): string {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, ...segments)
  }
  return path.join(app.getAppPath(), ...segments)
}

async function ensureService(): Promise<ServiceResult> {
  const health = await healthCheck()
  if (health.ok) {
    logger.info('Agent service already running')
    return { ok: true }
  }

  if (process.platform !== 'win32') {
    logger.warn('Service start skipped: unsupported platform')
    return { ok: false, error: 'Unsupported platform' }
  }

  logger.info('Agent service not running, starting via WinSW...')

  const winswExe = getResourcesPath('winsw', 'ai-os-agent.exe')
  if (!fs.existsSync(winswExe)) {
    logger.error(`WinSW not found: ${winswExe}`)
    return { ok: false, error: 'WinSW executable not found' }
  }

  return new Promise((resolve) => {
    execFile(
      winswExe,
      ['start'],
      { timeout: 15000, windowsHide: true },
      (err, stdout, stderr) => {
        if (err) {
          logger.error(`WinSW start failed: ${err.message}`)
          resolve({ ok: false, error: err.message })
          return
        }
        logger.info(`WinSW start output: ${stdout || stderr}`)
        resolve({ ok: true })
      },
    )
  })
}

export { PORT, SERVICE_NAME, healthCheck, getResourcesPath, ensureService }
export type { HealthResult, ServiceResult }
