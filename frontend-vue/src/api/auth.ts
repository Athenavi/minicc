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

// ── 短信验证码登录（公开流程 + 用户自助绑定 + 管理端配置） ──

export interface SmsStatus {
  enabled: boolean
  login_enabled: boolean
}

/** 登录页据此决定是否展示"短信登录"标签页 */
export async function getSmsStatus(): Promise<SmsStatus> {
  const { data } = await api.get('/v1/auth/sms/status')
  return data?.data ?? { enabled: false, login_enabled: false }
}

export interface SendSmsCodeResult {
  status: string
  expire_seconds: number
  interval: number
}

/** 发送验证码（防滥用：人机验证 + 发送冷却 + 每日上限） */
export async function sendSmsCode(body: {
  phone: string
  purpose?: 'login' | 'bind'
  captcha_token?: string
  captcha_randstr?: string
}): Promise<SendSmsCodeResult> {
  const { data } = await api.post('/v1/auth/sms/code', body)
  return data?.data
}

export async function smsLogin(body: {
  phone: string
  code: string
  captcha_token?: string
  captcha_randstr?: string
}): Promise<{ token: string; user: any }> {
  const { data } = await api.post('/v1/auth/sms/login', body)
  return data?.data
}

/** 当前绑定手机号 */
export async function getSmsBind(): Promise<{ phone: string; bound: boolean }> {
  const { data } = await api.get('/v1/auth/sms/bind')
  return data?.data
}

export async function bindPhone(body: { phone: string; code: string }): Promise<void> {
  await api.post('/v1/auth/sms/bind', body)
}

export async function unbindPhone(): Promise<void> {
  await api.delete('/v1/auth/sms/bind')
}

export interface SmsAdminConfig {
  provider: string
  sign_name: string
  template_id: string
  access_key_id: string
  secret: string
  endpoint: string
  code_ttl_seconds: number
  send_interval_seconds: number
  daily_limit: number
  login_enabled: boolean
  auto_register: boolean
  enabled: boolean
  exists?: boolean
}

export async function getSmsAdminConfig(): Promise<SmsAdminConfig> {
  const { data } = await api.get('/v1/ent/sms/config')
  const cfg: SmsAdminConfig = data?.data
  // 前端脱敏处理（纵深防御）：仅展示 secret 首尾各 2 字符，中间用 *** 替代
  // 注意：后端 API 已主动脱敏 secret 字段（maskedSecret），前端脱敏仅作为纵深防御
  if (cfg && cfg.secret && cfg.secret.length > 6) {
    cfg.secret = cfg.secret.slice(0, 2) + '***' + cfg.secret.slice(-2)
  }
  return cfg
}

export async function updateSmsConfig(body: Partial<{
  provider: string
  sign_name: string
  template_id: string
  access_key_id: string
  secret: string
  endpoint: string
  code_ttl_seconds: number
  send_interval_seconds: number
  daily_limit: number
  login_enabled: boolean
  auto_register: boolean
  enabled: boolean
}>): Promise<SmsAdminConfig> {
  const { data } = await api.put('/v1/ent/sms/config', body)
  const cfg: SmsAdminConfig = data?.data
  // 前端脱敏处理（纵深防御）：仅展示 secret 首尾各 2 字符，中间用 *** 替代
  // 注意：后端 API 已主动脱敏 secret 字段（maskedSecret），前端脱敏仅作为纵深防御
  if (cfg && cfg.secret && cfg.secret.length > 6) {
    cfg.secret = cfg.secret.slice(0, 2) + '***' + cfg.secret.slice(-2)
  }
  return cfg
}

/** 手机号校验：可选 + 前缀 + 5-20 位数字（与后端 ValidateSmsPhone 一致） */
export function isValidPhone(phone: string): boolean {
  const p = (phone || '').trim()
  if (!p || p.length > 21) return false
  const digits = p.startsWith('+') ? p.slice(1) : p
  return /^\d{5,20}$/.test(digits)
}
