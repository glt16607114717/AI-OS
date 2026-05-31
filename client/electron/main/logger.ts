import fs from 'fs'
import path from 'path'

type LogLevel = 'INFO' | 'WARN' | 'ERROR'

const MAX_SIZE = 5 * 1024 * 1024
const MAX_FILES = 5
const LOG_DIR = path.join(process.env.USERPROFILE || process.env.HOME || '', 'AI-OS', 'logs')
const LOG_FILE = path.join(LOG_DIR, 'client.log')

class Logger {
  private ensureDir(): void {
    if (!fs.existsSync(LOG_DIR)) {
      fs.mkdirSync(LOG_DIR, { recursive: true })
    }
  }

  private rotate(): void {
    const oldest = `${LOG_FILE}.${MAX_FILES}`
    if (fs.existsSync(oldest)) {
      fs.unlinkSync(oldest)
    }
    for (let i = MAX_FILES - 1; i >= 1; i--) {
      const src = `${LOG_FILE}.${i}`
      if (fs.existsSync(src)) {
        fs.renameSync(src, `${LOG_FILE}.${i + 1}`)
      }
    }
    fs.renameSync(LOG_FILE, `${LOG_FILE}.1`)
  }

  private write(level: LogLevel, message: string): void {
    this.ensureDir()
    try {
      const stat = fs.statSync(LOG_FILE)
      if (stat.size >= MAX_SIZE) {
        this.rotate()
      }
    } catch {
      // file not exists yet, skip rotation
    }
    const line = `[${new Date().toISOString()}] [${level}] ${message}\n`
    fs.appendFileSync(LOG_FILE, line, 'utf-8')
    console.log(line.trim())
  }

  info(message: string): void {
    this.write('INFO', message)
  }

  warn(message: string): void {
    this.write('WARN', message)
  }

  error(message: string): void {
    this.write('ERROR', message)
  }
}

export const logger = new Logger()
