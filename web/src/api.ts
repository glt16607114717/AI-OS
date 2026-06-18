// 开发环境连本地 Go 后端，生产环境连云端
const isDev = import.meta.env.DEV
export const API_BASE = isDev ? 'http://localhost:18731' : 'http://8.163.127.182:18731'
