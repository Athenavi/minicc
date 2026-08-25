import { api } from './index'

export interface InstallDep {
  name: string
  ok: boolean
  message?: string
}

export interface InstallStatus {
  needed: boolean
  reason?: string
  db: boolean
  redis: boolean
  deps?: InstallDep[]
}

export interface InstallStep1 {
  completed: boolean
  app_secret_set?: boolean
  data_writable?: boolean
  step2_done?: boolean
  step3_done?: boolean
  message?: string
}

export interface InstallStep2Body {
  app_secret?: string
  postgres_dsn: string
  redis_addr?: string
  redis_password?: string
  redis_db?: number
}

export interface InstallStep3Body {
  email: string
  password: string
  name: string
}

export interface InstallResult {
  message: string
  completed?: boolean
  restart?: boolean
  user?: { id: string; email: string; name: string; role: string }
}

export interface SetupResult {
  message: string
  user: { id: string; email: string; name: string; role: string }
}

// 安装令牌（Jenkins 模式）：安装模式时由部署者从启动日志获取，URL 携带 ?token=xxx；
// API 调用统一放入 X-Install-Token header（不依赖 URL 参数传递凭据）。
// 支持从 location.search、location.hash 和 localStorage 中提取（SPA 路由可能吞掉查询参数）。
// 注意：每次调用 _extractToken 重新读取，以便手动输入令牌后能立即生效。
const _extractToken = (): string => {
  const fromSearch = new URLSearchParams(window.location.search).get('token')
  if (fromSearch) return fromSearch
  // 兼容 hash 模式路由（如 /#/install?token=xxx）
  const hashIdx = window.location.href.indexOf('?token=')
  if (hashIdx >= 0) {
    const params = new URLSearchParams(window.location.href.slice(hashIdx))
    return params.get('token') || ''
  }
  // 从 localStorage 读取（用户首次输入后保存）
  const fromStorage = localStorage.getItem('install_token')
  if (fromStorage) return fromStorage
  return ''
}

export function saveInstallToken(token: string): void {
  localStorage.setItem('install_token', token)
}

/** 动态获取当前安装令牌（每次调用都重新提取，确保手动输入后立即生效） */
function getInstallHeaders(): Record<string, string> | undefined {
  const token = _extractToken()
  return token ? { 'X-Install-Token': token } : undefined
}

export function getInstallToken(): string {
  return _extractToken()
}

export function hasInstallToken(): boolean {
  return _extractToken() !== ''
}

// ── 安装模式三步向导（setup mode：数据库/主密钥未配置时）──

export async function getInstallStep1(): Promise<InstallStep1> {
  const { data } = await api.get('/v1/install/step1', { headers: getInstallHeaders() })
  return data?.data ?? data
}

export async function postInstallStep2(body: InstallStep2Body): Promise<InstallResult> {
  const { data } = await api.post('/v1/install/step2', body, { headers: getInstallHeaders() })
  return data?.data ?? data
}

export async function postInstallStep3(body: InstallStep3Body): Promise<InstallResult> {
  const { data } = await api.post('/v1/install/step3', body, { headers: getInstallHeaders() })
  return data?.data ?? data
}

// ── 正常模式（数据库已就绪、尚未创建管理员时）──

export async function getInstallStatus(): Promise<InstallStatus> {
  const { data } = await api.get('/v1/install/status')
  return data?.data ?? data
}

export async function setupSystem(body: InstallStep3Body): Promise<SetupResult> {
  const { data } = await api.post('/v1/install/setup', body)
  return data?.data ?? data
}
