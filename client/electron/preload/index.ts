import { contextBridge, ipcRenderer } from 'electron'

contextBridge.exposeInMainWorld('aiOS', {
  // 窗口控制
  windowMinimize: () => ipcRenderer.invoke('window:minimize'),
  windowMaximize: () => ipcRenderer.invoke('window:maximize'),
  windowClose: () => ipcRenderer.invoke('window:close'),
  windowIsMaximized: () => ipcRenderer.invoke('window:is-maximized'),

  // 本地 Agent（语音等）
  agentHealth: () => ipcRenderer.invoke('agent:health'),
  agentRequest: (action: string, params: Record<string, unknown>) =>
    ipcRenderer.invoke('agent:request', action, params),
  agentRestart: () => ipcRenderer.invoke('agent:restart'),

  // 环境配置
  checkSetupNeeded: () => ipcRenderer.invoke('setup:check'),
  runSetup: (onProgress: (info: any) => void) => {
    const handler = (_event: unknown, info: any) => onProgress(info)
    ipcRenderer.on('setup:progress', handler)
    return ipcRenderer.invoke('setup:run').finally(() => {
      ipcRenderer.removeListener('setup:progress', handler)
    })
  },
})
