import axios from 'axios'

const API_URL = import.meta.env.VITE_API_URL || ''

// 注意：不要在这里设置默认 Content-Type！
// - 对 JSON 对象 axios 的 transformRequest 会自动设置 application/json
// - 对 FormData，显式 Content-Type 会阻止浏览器自动附加 multipart boundary，
//   导致后端 ParseMultipartForm 报 "invalid form"（上传 400）
export const api = axios.create({
  baseURL: API_URL,
  timeout: 30000,
  // S 安全：鉴权凭 httpOnly cookie（由后端 SetTokenCookie 下发），JS 不可读，
  // 避免 XSS 偷取 localStorage 中的 token。所有请求自动携带同源 cookie。
  withCredentials: true,
})

// 工具确认（S 安全修复：三态栅栏“确认”态 — 前端确认卡片回调）
export async function submitApproval(params: {
  session_id: string
  tool_call_id: string
  approved: boolean
  reason?: string
}): Promise<boolean> {
  const { data } = await api.post('/v1/agent/approval', params)
  return !!(data?.data?.ok ?? data?.ok)
}

// ── 会话操作（重命名 / 置顶）──
export async function updateConversation(id: string, patch: { title?: string; pinned?: boolean }) {
  const { data } = await api.put(`/v1/conversations/${encodeURIComponent(id)}`, patch)
  return data?.data
}

// ── Agents（DB 驱动：CRUD + 运行会话） ──
export interface Agent {
  id: string
  name: string
  description?: string
  system_prompt?: string
  tools?: any[]
  llm_config?: Record<string, any>
  max_turns: number
  timeout_seconds: number
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface AgentSession {
  id: string
  agent_id?: string
  agent_name?: string
  task: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  result?: string
  created_at: string
  updated_at: string
}

export async function listAgents(): Promise<Agent[]> {
  const { data } = await api.get('/v1/agents')
  return data?.data ?? []
}

export async function createAgent(body: Partial<Agent>): Promise<Agent> {
  const { data } = await api.post('/v1/agents', body)
  return data?.data
}

export async function updateAgent(id: string, body: Partial<Agent>): Promise<Agent> {
  const { data } = await api.put(`/v1/agents/${encodeURIComponent(id)}`, body)
  return data?.data
}

export async function deleteAgent(id: string): Promise<void> {
  await api.delete(`/v1/agents/${encodeURIComponent(id)}`)
}

export async function runAgent(id: string, task: string): Promise<AgentSession> {
  const { data } = await api.post(`/v1/agents/${encodeURIComponent(id)}/run`, { task })
  return data?.data
}

export async function listAgentSessions(): Promise<AgentSession[]> {
  const { data } = await api.get('/v1/agents/sessions')
  return data?.data ?? []
}

export async function getAgentSession(id: string): Promise<AgentSession> {
  const { data } = await api.get(`/v1/agents/sessions/${encodeURIComponent(id)}`)
  return data?.data
}

// ── 六大工作台统一入口（quick-execute = chat/submit 的语义别名）──
export interface QuickExecuteResult {
  success: boolean
  session_id?: string
  trace_id?: string
  output?: string
  error?: string
  metadata?: {
    task_id?: string
    duration_ms?: number
    subtasks_completed?: number
  }
}

/** 快捷执行：自然语言任务 → TaskRouter 跨工作台自动编排 */
export async function quickExecute(body: {
  message: string
  session_id?: string
  mode?: 'auto' | 'agent' | 'workflow'
}): Promise<QuickExecuteResult> {
  const { data } = await api.post('/v1/quick-execute', body)
  return data
}

/** 获取统一会话消息历史（含跨工作台共享上下文） */
export async function getChatSessionMessages(sessionId: string) {
  const { data } = await api.get(`/v1/chat/sessions/${encodeURIComponent(sessionId)}/messages`)
  return data
}

// ── 会话分享（chat.deepseek.com/share/{id} 风格）──
export interface ShareInfo {
  share_id: string
  created_at?: string
}

/** 创建分享（body 为选中的消息 id；已有活跃分享时幂等返回） */
export async function createShare(sessionId: string, messageIds: string[]): Promise<ShareInfo> {
  const { data } = await api.post(`/v1/conversations/${encodeURIComponent(sessionId)}/share`, { message_ids: messageIds })
  return data?.data
}

/** 查询会话的活跃分享（无则抛 404） */
export async function getActiveShare(sessionId: string): Promise<ShareInfo> {
  const { data } = await api.get(`/v1/conversations/${encodeURIComponent(sessionId)}/share`)
  return data?.data
}

/** 撤销分享（公开链接随即失效） */
export async function revokeShare(sessionId: string): Promise<void> {
  await api.delete(`/v1/conversations/${encodeURIComponent(sessionId)}/share`)
}

export interface SharedMessage {
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export interface PublicShare {
  id: string
  title: string
  created_at: string
  messages: SharedMessage[]
}

/** 公开分享读取（无鉴权；已撤销返回 410）。走 /v1/share/{id} 与 SPA /share/:id 路由分离（S 修复 路径冲突）。 */
export async function getPublicShare(shareId: string): Promise<PublicShare> {
  const { data } = await api.get(`/v1/share/${encodeURIComponent(shareId)}`)
  return data?.data
}

// 鉴权凭 httpOnly cookie 自动携带（withCredentials），无需请求拦截器设置 Authorization。

// 响应拦截器：处理错误
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // 安装向导端点不触发跳转登录（安装模式下/install端点返回401表示令牌无效）
      const url = error.config?.url || ''
      if (url.startsWith('/v1/install/')) {
        return Promise.reject(error)
      }
      // Cookie 过期/失效：清本地 user 态，跳转登录（后端 cookie 由 /v1/auth/logout 清除）
      localStorage.removeItem('user')
      window.dispatchEvent(new CustomEvent('api:error', {
        detail: { message: '登录已过期，请重新登录' }
      }))
      // 短延迟让 toast 显示后再跳转
      setTimeout(() => { window.location.href = '/login' }, 500)
    } else if (error.response?.status >= 500) {
      console.error('Server error:', error.response.status, error.response.data)
      // 触发全局错误事件，App.vue 中的监听器会显示提示
      window.dispatchEvent(new CustomEvent('api:error', {
        detail: { message: `服务器错误 (${error.response.status})，请稍后重试` }
      }))
    } else if (error.code === 'ECONNABORTED' || !error.response) {
      // 网络超时或无法连接
      window.dispatchEvent(new CustomEvent('api:error', {
        detail: { message: '网络连接失败，请检查网络后重试' }
      }))
    }
    return Promise.reject(error)
  }
)

// P1-2 文件上传：调用后端 POST /v1/media/upload（multipart）
// 后端返回 { id, name, type, file_url, size }，这里归一化为 ChatAttachment 所需结构
export async function uploadFile(file: File): Promise<{
  id: string
  url: string
  name: string
  size: number
  mimeType: string
}> {
  const form = new FormData()
  form.append('file', file)
  // S 修复：不要手动设 multipart Content-Type（会丢 boundary 致 "invalid form"），
  // 让浏览器/axios 自动生成带 boundary 的正确头。
  const resp = await api.post('/v1/media/upload', form)
  const d = resp.data?.asset || resp.data || {}
  const id = d.id || d.asset_id || String(Date.now())
  // 后端返回字段 file_url；兜底 /v1/media/{id}/download
  const url = d.file_url || d.url || d.download_url || `/v1/media/${id}/download`
  return {
    id,
    url,
    name: d.name || d.filename || file.name,
    size: Number(d.size) || file.size,
    // 后端不返回 mime_type，用客户端声明的 file.type 兜底
    mimeType: d.mime_type || d.contentType || file.type,
  }
}

// SSE 连接
export function createSSEConnection(sessionId: string, onMessage: (data: any) => void, onError?: () => void) {
  // EventSource 无法设置 header → 用 withCredentials 携带同源 cookie（JWT cookie 由登录接口下发），
  // 避免 JWT 出现在 URL 查询参数（会被浏览器历史/代理日志/Referer 泄露）
  const url = `${API_URL}/events?session_id=${encodeURIComponent(sessionId)}`

  const eventSource = new EventSource(url, { withCredentials: true })

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      onMessage(data)
    } catch (e) {
      console.error('SSE parse error:', e)
    }
  }

  eventSource.onerror = (error) => {
    console.error('SSE error:', error)
    onError?.()
    eventSource.close()
  }

  return eventSource
}

export default api


// ─────────────────────────────────────────────────────────────
// 六大工作台互联互通 · 市场与媒体签名（2026-08-22）
// ─────────────────────────────────────────────────────────────

// ── 市场：技能 / Agent / MCP 浏览与一键安装 ──
export type MarketType = 'skill' | 'agent' | 'mcp'

export interface MarketItem {
  id: string
  type: MarketType
  name: string
  version: string
  status: string
  manifest: Record<string, any>
  installed?: boolean
}

export async function listMarket(type: MarketType): Promise<MarketItem[]> {
  const resp = await api.get('/v1/market', { params: { type } })
  return resp.data?.items || []
}

export async function installMarket(type: MarketType, itemID: string): Promise<any> {
  const resp = await api.post(`/v1/market/${type}/${itemID}/install`)
  return resp.data
}

// ── 媒体签名 URL（短时效，防裸公开猜测）──
const signedMediaCache = new Map<string, { url: string; exp: number }>()

export async function signMediaUrl(id: string): Promise<string> {
  const resp = await api.post(`/v1/media/${id}/sign`)
  return resp.data?.url || ''
}

/**
 * 解析媒体资源为可访问 URL：非 /media/ 前缀(绝对/签名/data:)直接返回；
 * /media/ 公开路径统一改走签名 URL（带 12 分钟本地缓存）。
 */
export async function resolveMediaUrl(asset: { id?: string; file_url?: string }): Promise<string> {
  const f = asset?.file_url || ''
  if (f && !f.startsWith('/media/')) return f
  if (!asset?.id) return f
  const cached = signedMediaCache.get(asset.id)
  if (cached && cached.exp > Date.now()) return cached.url
  try {
    const url = await signMediaUrl(asset.id)
    signedMediaCache.set(asset.id, { url, exp: Date.now() + 12 * 60 * 1000 })
    return url
  } catch {
    return f
  }
}


// ── 模型路由 / 定时自动化（2026-08-23）──
export interface LlmModel {
  provider: string
  name: string
  display_name: string
  context_window: number
}

export async function listModels(): Promise<LlmModel[]> {
  const resp = await api.get('/v1/models')
  return resp.data?.models || []
}

export async function triggerCronJob(id: string): Promise<any> {
  const resp = await api.post(`/v1/admin/cron-jobs/${id}/trigger`)
  return resp.data
}

/** 生成 cron job 的 Webhook 触发 URL（POST，token 为鉴权） */
export function cronWebhookUrl(id: string, token: string): string {
  return `${location.origin}/v1/hooks/${id}?token=${encodeURIComponent(token)}`
}


// ── 模板市场 / 共享可见性（2026-08-23 批次2）──
export interface TemplateItem {
  id: string
  type: 'workflow' | 'agent' | 'skill'
  name: string
  description: string
  payload: Record<string, any>
}

export async function listTemplates(type?: 'workflow' | 'agent' | 'skill'): Promise<TemplateItem[]> {
  const resp = await api.get('/v1/templates', { params: type ? { type } : {} })
  return resp.data?.templates || []
}

export async function useTemplate(id: string): Promise<any> {
  const resp = await api.post(`/v1/templates/${id}/use`)
  return resp.data
}

export async function setAgentVisibility(id: string, visibility: 'private' | 'tenant' | 'public'): Promise<any> {
  const resp = await api.put(`/v1/agents/${id}/visibility`, { visibility })
  return resp.data
}

export async function setKBVisibility(id: string, visibility: 'private' | 'tenant' | 'public'): Promise<any> {
  const resp = await api.put(`/v1/kb/${id}/visibility`, { visibility })
  return resp.data
}

