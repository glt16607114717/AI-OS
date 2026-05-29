/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface AiOSApi {
  agentHealth: () => Promise<{ ok: boolean; uptime?: number; version?: string }>
  windowMinimize: () => Promise<void>
  windowMaximize: () => Promise<void>
  windowClose: () => Promise<void>
  windowIsMaximized: () => Promise<boolean>
}

declare global {
  interface Window {
    aiOS: AiOSApi
  }
}
