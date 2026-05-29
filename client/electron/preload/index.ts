import { contextBridge, ipcRenderer } from 'electron'

contextBridge.exposeInMainWorld('aiOS', {
  agentHealth: () => ipcRenderer.invoke('agent:health'),
  windowMinimize: () => ipcRenderer.invoke('window:minimize'),
  windowMaximize: () => ipcRenderer.invoke('window:maximize'),
  windowClose: () => ipcRenderer.invoke('window:close'),
  windowIsMaximized: () => ipcRenderer.invoke('window:isMaximized'),
})
