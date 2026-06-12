import { contextBridge, ipcRenderer } from 'electron'

contextBridge.exposeInMainWorld('aiOS', {
  windowMinimize: () => ipcRenderer.invoke('window:minimize'),
  windowMaximize: () => ipcRenderer.invoke('window:maximize'),
  windowClose: () => ipcRenderer.invoke('window:close'),
})
