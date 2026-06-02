import { contextBridge, ipcRenderer } from 'electron'

contextBridge.exposeInMainWorld('aiOS', {
  windowMinimize: () => ipcRenderer.invoke('window:minimize'),
  windowMaximize: () => ipcRenderer.invoke('window:maximize'),
  windowClose: () => ipcRenderer.invoke('window:close'),
  agentHealth: () => ipcRenderer.invoke('agent:health'),
  agentRequest: (action: string, payload: any) => ipcRenderer.invoke('agent:request', action, payload),
  checkSetupNeeded: () => ipcRenderer.invoke('setup:checkNeeded'),
  runSetup: (onProgress: (info: any) => void) => {
    const channel = 'setup:progress:' + Date.now()
    ipcRenderer.on(channel, (_event, info) => onProgress(info))
    return ipcRenderer.invoke('setup:run', channel).finally(() => {
      ipcRenderer.removeAllListeners(channel)
    })
  },
})
