import * as fs from 'fs'
import * as path from 'path'
import * as https from 'https'
import * as http from 'http'
import { spawn } from 'child_process'

type ProgressCallback = (info: SetupProgress) => void

export interface SetupProgress {
  step: string
  stepIndex: number
  totalSteps: number
  percent: number
  detail: string
  error?: string
  done?: boolean
}

const PYTHON_VERSION = '3.12.10'
const EMBED_URL = 'https://npmmirror.com/mirrors/python/3.12.10/python-3.12.10-embed-amd64.zip'
const NUGET_URL = 'https://registry.npmmirror.com/-/binary/python/3.12.10/python-3.12.10-amd64.zip'
const GET_PIP_URL = 'https://bootstrap.pypa.io/get-pip.py'
const PIP_INDEX = 'https://mirrors.aliyun.com/pypi/simple'
const AGENT_PORT = 18731
const DATA_DIR = path.join(process.env.ProgramData || 'C:\\ProgramData', 'AI-OS')

export class SetupManager {
  private appDir: string
  private pythonDir: string
  private onProgress: ProgressCallback
  private currentStepIndex = 0
  private totalVisibleSteps = 5
  private logFile: string

  constructor(appDir: string, onProgress: ProgressCallback) {
    this.appDir = appDir
    this.pythonDir = path.join(DATA_DIR, 'runtime', 'python')
    this.onProgress = onProgress
    this.logFile = path.join(appDir, 'setup-debug.log')
  }

  private debug(msg: string) {
    const line = `[${new Date().toISOString()}] ${msg}\n`
    try { fs.appendFileSync(this.logFile, line) } catch {}
  }

  private emit(info: Partial<SetupProgress>) {
    this.onProgress({
      step: info.step || '',
      stepIndex: info.stepIndex || 0,
      totalSteps: info.totalSteps || 5,
      percent: info.percent || 0,
      detail: info.detail || '',
      error: info.error,
      done: info.done,
    })
  }

  getPythonExe(): string {
    return path.join(this.pythonDir, 'python.exe')
  }

  isPythonReady(): boolean {
    return fs.existsSync(this.getPythonExe())
  }

  async runFullSetup(): Promise<void> {
    const skipDownload = await this.isPythonVersionOk()
    const skipDeps = skipDownload && await this.allDepsInstalled()

    this.debug('skipDownload:' + skipDownload + ' skipDeps:' + skipDeps)

    const steps: { name: string; fn: () => Promise<void>; skip: boolean }[] = [
      { name: '环境检测', fn: () => this.checkEnvironment(), skip: false },
      { name: '下载 Python', fn: () => this.downloadPython(), skip: skipDownload },
      { name: '安装依赖', fn: () => this.installDeps(), skip: skipDeps },
      { name: '注册服务', fn: () => this.registerService(), skip: false },
      { name: '启动服务', fn: () => this.startService(), skip: false },
    ]

    this.totalVisibleSteps = steps.length

    for (let i = 0; i < steps.length; i++) {
      const step = steps[i]
      this.currentStepIndex = i

      if (step.skip) {
        this.emit({
          step: step.name,
          stepIndex: this.currentStepIndex,
          totalSteps: this.totalVisibleSteps,
          percent: Math.round(((i + 1) / this.totalVisibleSteps) * 100),
          detail: '已就绪',
        })
        await this.sleep(500)
        continue
      }

      try {
        await step.fn()
      } catch (err: any) {
        this.emit({
          step: step.name,
          stepIndex: this.currentStepIndex,
          totalSteps: this.totalVisibleSteps,
          error: `${step.name}失败: ${err.message}`,
          detail: `${err.message}`,
        })
        throw err
      }
    }

    this.debug('All steps done, emitting done')
    this.emit({
      step: '完成',
      stepIndex: this.totalVisibleSteps - 1,
      totalSteps: this.totalVisibleSteps,
      percent: 100,
      detail: 'AI-OS 环境配置完成',
      done: true,
    })
  }

  private async isPythonVersionOk(): Promise<boolean> {
    const exe = this.getPythonExe()
    if (!fs.existsSync(exe)) {
      this.debug('Python not found: ' + exe)
      return false
    }
    try {
      const ver = await this.getPythonVersion()
      this.debug('Python version: ' + ver + ' includes: ' + ver.includes(PYTHON_VERSION))
      return ver.includes(PYTHON_VERSION)
    } catch (e: any) {
      this.debug('getPythonVersion error: ' + e.message)
      return false
    }
  }

  private async allDepsInstalled(): Promise<boolean> {
    const reqFile = path.join(this.appDir, 'resources', 'backend', 'python', 'requirements.txt')
    if (!fs.existsSync(reqFile)) return true

    const reqs = fs.readFileSync(reqFile, 'utf-8')
      .split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))

    for (const req of reqs) {
      const pkgName = req.split(/[><=!~\[]/)[0].trim()
      if (!await this.isPkgInstalled(pkgName)) return false
    }
    return true
  }

  private async checkEnvironment(): Promise<void> {
    this.emit({
      step: '环境检测',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 50,
      detail: '正在检测本地环境...',
    })

    if (await this.isPythonVersionOk()) {
      this.emit({
        step: '环境检测',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: 100,
        detail: `Python ${PYTHON_VERSION} 已就绪`,
      })
    } else {
      this.emit({
        step: '环境检测',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: 100,
        detail: '需要配置 Python 运行环境',
      })
    }
  }

  private async downloadPython(): Promise<void> {
    this.emit({
      step: '下载 Python',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 0,
      detail: '准备下载 Python 运行时...',
    })

    if (fs.existsSync(this.pythonDir)) {
      fs.rmSync(this.pythonDir, { recursive: true, force: true })
    }

    const tempDir = path.join(this.appDir, '.temp-python')
    if (fs.existsSync(tempDir)) fs.rmSync(tempDir, { recursive: true, force: true })
    fs.mkdirSync(tempDir, { recursive: true })

    try {
      const embedZip = path.join(tempDir, 'embed.zip')
      const nugetZip = path.join(tempDir, 'nuget.zip')

      await this.downloadFile(EMBED_URL, embedZip, 'Python 核心', 0, 45)
      await this.downloadFile(NUGET_URL, nugetZip, 'Python 标准库', 45, 90)

      this.emit({
        step: '下载 Python',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: 92,
        detail: '正在解压...',
      })

      const embedDir = path.join(tempDir, 'embed')
      const nugetDir = path.join(tempDir, 'nuget')
      fs.mkdirSync(embedDir, { recursive: true })
      fs.mkdirSync(nugetDir, { recursive: true })

      await this.extractZip(embedZip, embedDir)
      await this.extractZip(nugetZip, nugetDir)

      this.emit({
        step: '下载 Python',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: 96,
        detail: '正在合并标准库...',
      })

      const nugetLib = path.join(nugetDir, 'Lib')
      const embedLib = path.join(embedDir, 'Lib')
      if (fs.existsSync(nugetLib)) {
        this.mergeDirectory(nugetLib, embedLib)
      }

      if (fs.existsSync(this.pythonDir)) fs.rmSync(this.pythonDir, { recursive: true, force: true })
      fs.mkdirSync(this.pythonDir, { recursive: true })
      this.copyDirectory(embedDir, this.pythonDir)

      const pthPath = path.join(this.pythonDir, 'python312._pth')
      fs.writeFileSync(pthPath, 'Lib\nLib\\site-packages\n.\nimport site\n', 'utf-8')

      const sitePackages = path.join(this.pythonDir, 'Lib', 'site-packages')
      if (!fs.existsSync(sitePackages)) fs.mkdirSync(sitePackages, { recursive: true })

      this.emit({
        step: '下载 Python',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: 100,
        detail: `Python ${PYTHON_VERSION} 安装完成`,
      })
    } finally {
      if (fs.existsSync(tempDir)) fs.rmSync(tempDir, { recursive: true, force: true })
    }
  }

  private async installDeps(): Promise<void> {
    this.emit({
      step: '安装依赖',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 0,
      detail: '正在检测 pip...',
    })

    if (!await this.checkPip()) {
      this.emit({ stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps, percent: 5, detail: '正在安装 pip...' })
      await this.installPip()
    }

    this.emit({ stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps, percent: 8, detail: '正在升级 pip...' })
    await this.execAsync(this.getPythonExe(),
      ['-m', 'pip', 'install', '--upgrade', 'pip', '-i', PIP_INDEX, '--no-warn-script-location',
       '--target', path.join(this.pythonDir, 'Lib', 'site-packages')])

    const reqFile = path.join(this.appDir, 'resources', 'backend', 'python', 'requirements.txt')
    if (!fs.existsSync(reqFile)) {
      this.emit({ stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps, percent: 100, detail: '无依赖需要安装' })
      return
    }

    const reqs = fs.readFileSync(reqFile, 'utf-8')
      .split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))

    const total = reqs.length
    let installed = 0
    let skipped = 0

    for (let i = 0; i < total; i++) {
      const req = reqs[i]
      const pkgName = req.split(/[><=!~\[]/)[0].trim()

      if (await this.isPkgInstalled(pkgName)) {
        skipped++
        const pct = 10 + Math.round(((i + 1) / total) * 90)
        this.emit({
          step: '安装依赖',
          stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
          percent: pct,
          detail: `检测依赖 (${i + 1}/${total})...`,
        })
        continue
      }

      installed++
      const pct = 10 + Math.round(((i + 1) / total) * 90)
      this.emit({
        step: '安装依赖',
        stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
        percent: pct,
        detail: `正在安装依赖包 (${i + 1}/${total})...`,
      })

      await this.execAsync(this.getPythonExe(),
        ['-m', 'pip', 'install', req, '-i', PIP_INDEX, '--no-warn-script-location',
         '--target', path.join(this.pythonDir, 'Lib', 'site-packages')])
    }

    this.emit({
      step: '安装依赖',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 100,
      detail: installed > 0 ? `已安装 ${installed} 个，跳过 ${skipped} 个` : `所有 ${total} 个依赖已就绪`,
    })
  }

  private async registerService(): Promise<void> {
    this.emit({
      step: '注册服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 0,
      detail: '正在配置计划任务...',
    })

    const pythonwExe = path.join(this.pythonDir, 'pythonw.exe')
    const pythonExe = path.join(this.pythonDir, 'python.exe')
    const agentScript = path.join(this.appDir, 'resources', 'backend', 'python', 'agent', 'main.py')
    const taskName = 'AI-OS-Agent'

    this.emit({
      step: '注册服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 20,
      detail: '正在注册计划任务...',
    })

    // PowerShell 脚本：注销旧任务 → 注册新计划任务（AtLogOn 自启）
    const scriptPath = path.join(this.appDir, '.temp-svc-setup.ps1')
    const scriptContent = [
      `$ErrorActionPreference = 'Continue'`,
      // Agent：登录时启动
      `$taskName = '${taskName}'`,
      `schtasks /Delete /TN $taskName /F 2>&1 | Out-Null`,
      `schtasks /Create /SC ONLOGON /TN $taskName /TR "'${pythonwExe}' -X utf8 '${agentScript}'" /RL HIGHEST /F`,
      // Watchdog：每 1 分钟检查 main.py 是否存活，不在就启动
      `$wdName = 'AI-OS-Watchdog'`,
      `$wdScript = '${agentScript.replace('main.py', 'watchdog.py')}'`,
      `schtasks /Delete /TN $wdName /F 2>&1 | Out-Null`,
      `schtasks /Create /SC MINUTE /MO 1 /TN $wdName /TR "'${pythonwExe}' -X utf8 '$wdScript'" /F`,
      `schtasks /Run /TN $wdName`,
    ].join('\r\n')

    fs.writeFileSync(scriptPath, scriptContent, 'utf-8')
    this.debug('Svc script: ' + scriptPath)

    try {
      await this.execAsync('powershell.exe', [
        '-NoProfile', '-Command',
        `Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-WindowStyle','Hidden','-File','${scriptPath}'`,
      ])
      this.debug('Svc script done')
    } catch (e: any) {
      this.debug('Svc script error: ' + e.message)
      throw new Error('服务注册失败: ' + e.message)
    } finally {
      try { fs.unlinkSync(scriptPath) } catch {}
    }

    // 从用户进程启动计划任务（不能在 UAC 管理员进程里启动，会跑在 Session 0）
    this.emit({
      step: '注册服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 60,
      detail: '正在启动后端...',
    })
    try {
      await this.execAsync('schtasks.exe', ['/Run', '/TN', taskName], false)
      this.debug('schtasks /Run done')
    } catch (e: any) {
      this.debug('schtasks /Run error: ' + e.message)
    }

    this.emit({
      step: '注册服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 100,
      detail: '服务注册成功',
    })
  }

  private async startService(): Promise<void> {
    this.emit({
      step: '启动服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 0,
      detail: '正在等待服务启动...',
    })

    for (let i = 0; i < 100; i++) {
      await this.sleep(500)
      try {
        const ok = await this.checkHealth()
        if (ok) {
          this.debug('Service health OK, step done')
          this.emit({
            step: '启动服务',
            stepIndex: this.currentStepIndex,
            totalSteps: this.totalVisibleSteps,
            percent: 100,
            detail: '服务已启动',
          })
          return
        }
      } catch {}

      this.emit({
        step: '启动服务',
        stepIndex: this.currentStepIndex,
        totalSteps: this.totalVisibleSteps,
        percent: i + 1,
        detail: '正在等待服务启动...',
      })
    }

    this.debug('Service health timeout')
    this.emit({
      step: '启动服务',
      stepIndex: this.currentStepIndex,
      totalSteps: this.totalVisibleSteps,
      percent: 100,
      detail: '服务启动超时，请尝试手动启动',
    })
  }

  private checkHealth(): Promise<boolean> {
    return new Promise((resolve) => {
      const req = http.get(`http://127.0.0.1:${AGENT_PORT}/health`, { timeout: 2000 }, (res) => {
        let data = ''
        res.on('data', (chunk: string) => { data += chunk })
        res.on('end', () => {
          try { resolve(JSON.parse(data).ok === true) } catch { resolve(false) }
        })
      })
      req.on('error', () => resolve(false))
      req.on('timeout', () => { req.destroy(); resolve(false) })
    })
  }

  private downloadFile(url: string, destPath: string, label: string, pctStart: number, pctEnd: number): Promise<void> {
    return new Promise((resolve, reject) => {
      const file = fs.createWriteStream(destPath)

      const doRequest = (requestUrl: string, redirects: number) => {
        if (redirects > 10) { reject(new Error('Too many redirects')); return }
        const mod = requestUrl.startsWith('https') ? https : http
        const req = mod.get(requestUrl, { headers: { 'User-Agent': 'AI-OS/1.0' } }, (res) => {
          if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            doRequest(res.headers.location, redirects + 1)
            return
          }
          if (res.statusCode !== 200) { reject(new Error(`HTTP ${res.statusCode}`)); return }

          const total = parseInt(res.headers['content-length'] || '0', 10)
          let received = 0
          let lastReportMB = -1

          this.emit({
            stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps,
            percent: pctStart, detail: `${label} 下载中...`,
          })

          res.on('data', (chunk: Buffer) => {
            received += chunk.length
            const currentMB = Math.floor(received / 1048576)
            if (currentMB > lastReportMB || received === total) {
              lastReportMB = currentMB
              const recvMB = (received / 1048576).toFixed(1)
              const totalMB = total ? (total / 1048576).toFixed(1) : '?'
              const pct = total
                ? Math.round(pctStart + (received / total) * (pctEnd - pctStart))
                : pctStart + Math.round((pctEnd - pctStart) * 0.5)
              this.emit({
                stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps,
                percent: pct, detail: `${label}: ${recvMB} / ${totalMB} MB`,
              })
            }
          })

          res.pipe(file)
          file.on('finish', () => {
            file.close()
            this.emit({
              stepIndex: this.currentStepIndex, totalSteps: this.totalVisibleSteps,
              percent: pctEnd, detail: `${label} 下载完成`,
            })
            resolve()
          })
        })
        req.on('error', (err) => {
          file.close()
          if (fs.existsSync(destPath)) fs.unlinkSync(destPath)
          reject(err)
        })
      }

      doRequest(url, 0)
    })
  }

  private extractZip(zipPath: string, destDir: string): Promise<void> {
    return new Promise((resolve, reject) => {
      fs.mkdirSync(destDir, { recursive: true })

      const ps = spawn('powershell.exe', [
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command',
        `Expand-Archive -Path '${zipPath}' -DestinationPath '${destDir}' -Force`,
      ], { stdio: 'pipe', windowsHide: true })

      ps.on('close', (code) => {
        if (code === 0) resolve()
        else reject(new Error(`Expand-Archive exit code: ${code}`))
      })
      ps.on('error', reject)
    })
  }

  private mergeDirectory(src: string, dest: string): void {
    if (!fs.existsSync(dest)) fs.mkdirSync(dest, { recursive: true })
    for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
      const srcPath = path.join(src, entry.name)
      const destPath = path.join(dest, entry.name)
      if (entry.isDirectory()) {
        if (fs.existsSync(destPath)) fs.rmSync(destPath, { recursive: true, force: true })
        fs.cpSync(srcPath, destPath, { recursive: true })
      } else {
        fs.copyFileSync(srcPath, destPath)
      }
    }
  }

  private copyDirectory(src: string, dest: string): void {
    if (!fs.existsSync(dest)) fs.mkdirSync(dest, { recursive: true })
    for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
      const srcPath = path.join(src, entry.name)
      const destPath = path.join(dest, entry.name)
      if (entry.isDirectory()) { this.copyDirectory(srcPath, destPath) }
      else { fs.copyFileSync(srcPath, destPath) }
    }
  }

  private getPythonVersion(): Promise<string> {
    return this.execAsync(this.getPythonExe(), ['--version']).then(out => {
      const m = out.match(/Python\s+(\S+)/)
      return m ? m[1] : ''
    })
  }

  private async checkPip(): Promise<boolean> {
    try {
      await this.execAsync(this.getPythonExe(), ['-m', 'pip', '--version'])
      return true
    } catch { return false }
  }

  private async installPip(): Promise<void> {
    const getPipPath = path.join(this.appDir, '.temp-get-pip.py')
    await this.downloadFile(GET_PIP_URL, getPipPath, 'get-pip.py', 0, 100)
    try { await this.execAsync(this.getPythonExe(), [getPipPath]) }
    finally { if (fs.existsSync(getPipPath)) fs.unlinkSync(getPipPath) }
  }

  private async isPkgInstalled(pkgName: string): Promise<boolean> {
    const sitePackages = path.join(this.pythonDir, 'Lib', 'site-packages')
    const lowerName = pkgName.toLowerCase().replace(/[-_.]/g, '_')
    try {
      const entries = fs.readdirSync(sitePackages)
      return entries.some(e => {
        const lower = e.toLowerCase().replace(/[-_.]/g, '_')
        return lower.startsWith(lowerName) || lower.startsWith(lowerName + '-')
      })
    } catch { return false }
  }

  private execAsync(cmd: string, args: string[], throwOnError = true): Promise<string> {
    return new Promise((resolve, reject) => {
      const proc = spawn(cmd, args, { stdio: 'pipe', windowsHide: true })
      let stdout = ''
      let stderr = ''
      proc.stdout.on('data', (d: Buffer) => { stdout += d.toString() })
      proc.stderr.on('data', (d: Buffer) => { stderr += d.toString() })
      proc.on('close', (code) => {
        if (throwOnError && code !== 0) {
          reject(new Error(`${path.basename(cmd)} exit ${code}: ${(stderr || stdout).substring(0, 200)}`))
        } else { resolve(stdout + stderr) }
      })
      proc.on('error', reject)
    })
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms))
  }
}
