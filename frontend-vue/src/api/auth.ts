import api from './index'

// ── 人机验证（公开配置 + 管理端） ──

export interface CaptchaPublicConfig {
  enabled: boolean
  provider?: string
  site_key?: string
  verify_url?: string
}

export interface CaptchaAdminConfig {
  provider: string
  site_key: string
  secret: string
  verify_url?: string
  enabled: boolean
}

/** 登录页拉取验证码组件参数（未启用时 enabled=false） */
export async function getCaptchaPublicConfig(): Promise<CaptchaPublicConfig> {
  const { data } = await api.get('/v1/auth/captcha/config')
  return data?.data ?? { enabled: false }
}

export async function getCaptchaAdminConfig(): Promise<CaptchaAdminConfig> {
  const { data } = await api.get('/v1/ent/captcha/config')
  return data?.data
}

export async function updateCaptchaConfig(body: Partial<{
  provider: string
  site_key: string
  secret: string
  verify_url: string
  enabled: boolean
}>): Promise<CaptchaAdminConfig> {
  const { data } = await api.put('/v1/ent/captcha/config', body)
  return data?.data
}

// ── 三方登录（公开发现 + 用户自助绑定 + 管理端 CRUD） ──

export interface SsoPublicProvider {
  id: string
  name: string
  display_name: string
  provider_type: string
  icon: string
  sort_order: number
  protocol: string
}

export async function listPublicSsoProviders(): Promise<SsoPublicProvider[]> {
  const { data } = await api.get('/v1/auth/sso/providers')
  return data?.data ?? []
}

export interface UserIdentity {
  id: string
  provider_name: string
  provider_type: string
  subject: string
  email: string
  created_at: string
}

export async function listIdentities(): Promise<UserIdentity[]> {
  const { data } = await api.get('/v1/auth/sso/identities')
  return data?.data ?? []
}

export async function deleteIdentity(id: string): Promise<void> {
  await api.delete(`/v1/auth/sso/identities/${encodeURIComponent(id)}`)
}

export async function setPassword(body: { current_password?: string; new_password: string }): Promise<void> {
  await api.post('/v1/auth/password', body)
}

export interface SsoProvider {
  id: string
  name: string
  issuer: string
  client_id: string
  client_secret: string
  scopes: string[]
  enabled: boolean
  auto_provision: boolean
  role_mapping: Record<string, string>
  protocol: string
  provider_type: string
  display_name: string
  icon: string
  sort_order: number
  auth_url: string
  token_url: string
  userinfo_url: string
  extra: Record<string, string>
  created_at?: string
  updated_at?: string
}

export async function listSsoProviders(): Promise<SsoProvider[]> {
  const { data } = await api.get('/v1/ent/sso/providers')
  return data?.data ?? []
}

export async function createSsoProvider(body: Partial<SsoProvider>): Promise<SsoProvider> {
  const { data } = await api.post('/v1/ent/sso/providers', body)
  return data?.data
}

export async function updateSsoProvider(id: string, body: Partial<SsoProvider>): Promise<SsoProvider> {
  const { data } = await api.put(`/v1/ent/sso/providers/${encodeURIComponent(id)}`, body)
  return data?.data
}

export async function deleteSsoProvider(id: string): Promise<void> {
  await api.delete(`/v1/ent/sso/providers/${encodeURIComponent(id)}`)
}

/** 构造三方登录跳转地址（mode=bind 用于个人中心绑定） */
export function ssoLoginURL(providerId: string, mode?: 'bind'): string {
  const base = (api.defaults?.baseURL || '').replace(/\/$/, '')
  const u = `${base}/v1/auth/sso/login/${encodeURIComponent(providerId)}`
  return mode === 'bind' ? `${u}?mode=bind` : u
}
