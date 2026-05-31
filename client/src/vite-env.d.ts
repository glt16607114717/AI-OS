/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface AgentHealthResult {
  ok: boolean
  uptime?: number
  version?: string
  pid?: number
  started_at?: string
}

interface AiOSApi {
  agentHealth: () => Promise<AgentHealthResult>
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

export {}
