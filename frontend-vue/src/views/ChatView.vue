<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { Button, Input, Modal, Checkbox, Alert, message } from 'ant-design-vue'
import { MenuOutlined, CopyOutlined, LinkOutlined, CloseOutlined } from '@ant-design/icons-vue'
import {
  api, createSSEConnection, submitApproval,
  updateConversation, createShare, getActiveShare, revokeShare,
  getChatSessionMessages, resolveMediaUrl,
} from '../api'
import type { ShareInfo } from '../api'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import { useRoute, useRouter } from 'vue-router'
import ChatSidePanel from '../components/chat/ChatSidePanel.vue'
import MessageList from '../components/chat/MessageList.vue'
import MessageItem from '../components/chat/MessageItem.vue'
import ChatEmptyHero from '../components/chat/ChatEmptyHero.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import CallChainTimeline from '../components/CallChainTimeline.vue'
import { HistoryOutlined, ExportOutlined, BulbOutlined, BulbFilled } from '@ant-design/icons-vue'
import { splitThinking, stripUserInputTag, formatClock, formatSize } from '../components/chat/chat-types'
import type { ChatItem, ChatSession, ChatAttachment } from '../components/chat/chat-types'

const authStore = useAuthStore()
const themeStore = useThemeStore()
const route = useRoute()
const router = useRouter()

// ── 会话状态 ──
const sessions = ref<ChatSession[]>([])
const activeSessionId = ref('')
const activeSession = computed(() => sessions.value.find(s => s.id === activeSessionId.value) || null)
const loading = ref(false)
const items = ref<ChatItem[]>([])
let activeSSE: EventSource | null = null

// ── Trace ID (当前会话的链路追踪标识) ──────────────────────────────
const currentTraceId = ref('')  // SSE done 事件回传的 trace_id

// S 安全修复：待确认工具调用（三态栅栏“确认”态）
interface PendingApproval {
  id: string
  toolName: string
  arguments: string
}
const pendingApprovals = ref<PendingApproval[]>([])

async function resolveApproval(a: PendingApproval, approved: boolean) {
  try {
    await submitApproval({
      session_id: activeSessionId.value || '',
      tool_call_id: a.id,
      approved,
    })
  } catch (e) {
    console.error('approval submit failed', e)
  } finally {
    pendingApprovals.value = pendingApprovals.value.filter(p => p.id !== a.id)
  }
}

// 模式
const modeOptions = [
  { label: '常规', value: 'normal' },
  { label: '极简', value: 'minimal' },
  { label: 'PTC', value: 'ptc' },
  { label: '创造', value: 'creative' },
]
const mode = ref('normal')

// ── 模型路由：会话 llm_config.model（空 = 后端默认路由）──
const llmModel = ref('')

// ── 对话模式预设（mode → temperature/max_tokens；用户显式覆盖优先）──
const MODE_PRESETS: Record<string, { temperature: number; max_tokens: number; note?: string }> = {
  normal: { temperature: 0.6, max_tokens: 4096 },
  minimal: { temperature: 0.2, max_tokens: 1024, note: '简短回复' },
  ptc: { temperature: 0.4, max_tokens: 4096, note: '分步思考' },
  creative: { temperature: 1.0, max_tokens: 8192 },
}

/** 构造 llm_config：mode + 对应预设 temperature/max_tokens + 模型路由 model（base 已显式携带的字段优先保留） */
function buildLlmConfig(base?: Record<string, any>): Record<string, any> {
  const cfg: Record<string, any> = { mode: mode.value, ...(base || {}) }
  // 模型路由：会话选定模型写入 llm_config（空 = 不携带，走后端默认路由）
  if (llmModel.value) cfg.model = llmModel.value
  const preset = MODE_PRESETS[mode.value]
  if (preset) {
    if (cfg.temperature === undefined) cfg.temperature = preset.temperature
    if (cfg.max_tokens === undefined) cfg.max_tokens = preset.max_tokens
  }
  return cfg
}

/** 模型切换：更新 llmModel ref + 会话级持久化（SSE 模式已有会话时立即保存 llm_config） */
function onModelChange(m: string) {
  if (m === llmModel.value) return
  llmModel.value = m
  message.info(m ? `模型已切换：${m}（仅影响后续消息）` : '模型已重置为默认（后端路由）')
  if (!unifiedMode.value && activeSessionId.value) {
    void updateConversation(activeSessionId.value, { llm_config: buildLlmConfig() } as any).catch(() => {})
  }
}

/** 模式切换：更新 mode ref + 提示（仅影响后续消息）+ 会话级持久化（SSE 模式已有会话时立即保存 llm_config） */
function onModeChange(m: string) {
  if (m === mode.value) return
  mode.value = m
  const opt = modeOptions.find(o => o.value === m)
  const preset = MODE_PRESETS[m]
  message.info(`已切换到「${opt?.label || m}」模式${preset?.note ? `（${preset.note}）` : ''}，仅影响后续消息`)
  if (!unifiedMode.value && activeSessionId.value) {
    void updateConversation(activeSessionId.value, { llm_config: buildLlmConfig() } as any).catch(() => {})
  }
}

/** 归一化后端 metadata（可能为 JSON 字符串或对象） */
function normalizeMeta(raw: any): Record<string, any> | undefined {
  if (!raw) return undefined
  if (typeof raw === 'string') {
    try { raw = JSON.parse(raw) } catch { return undefined }
  }
  return raw && typeof raw === 'object' ? raw : undefined
}

// ── 互联互通：统一任务模式 + 上下文芯片（与 SSE 流式并列的新路径）──
// 路由 query 约定（由 WorkstationNav / 各工作台入口发起）：
//   ?task=<sessionId>          统一会话（拉历史 + 继续追问）
//   ?task=&error=xxx           仅错误提示
//   ?kb=<id> / ?agent=<id> / ?skill=<name> / ?workflow=<id|name>   上下文附加
//   ?mode=<auto|agent|workflow> 创建时模式（WorkflowView 传 workflow）
interface ContextChip {
  type: 'kb' | 'agent' | 'skill' | 'workflow'
  label: string
  value: string
}
const contextChips = ref<ContextChip[]>([])
// Agent 配置（从 /v1/agents 尽力取；取不到则只传 agent_id，由后端兼容）
const agentCfg = ref<{ id: string; name?: string; system_prompt?: string; model?: string; max_turns?: number } | null>(null)
const errorBanner = ref('')          // query.error 提示条
const unifiedSessionId = ref('')     // 统一任务会话 id（query.task）
const unifiedSubmitMode = ref('auto') // 会话创建时的 mode（shared_context.mode 优先）
const unifiedMode = computed(() => !!unifiedSessionId.value)
// 纯展示 flag：任务提交成功 → 徽标短暂过渡到“完成”态后复位
const unifiedJustFinished = ref(false)
let unifiedDoneTimer: ReturnType<typeof setTimeout> | null = null
function flashUnifiedDone() {
  unifiedJustFinished.value = true
  if (unifiedDoneTimer) clearTimeout(unifiedDoneTimer)
  unifiedDoneTimer = setTimeout(() => { unifiedJustFinished.value = false }, 1600)
}
let appliedQueryKey = ''

async function applyRouteQuery() {
  const q = route.query
  const key = JSON.stringify(q)
  if (key === appliedQueryKey) return
  appliedQueryKey = key

  const task = typeof q.task === 'string' && q.task.trim() ? q.task.trim() : ''
  // task 变更 / 退出统一模式 → 重置消息区（避免污染普通 SSE 会话）
  if (task !== unifiedSessionId.value) {
    unifiedSessionId.value = task
    items.value = []
    activeSessionId.value = ''
    currentTraceId.value = ''
    loading.value = false
    stopTurnTimer()
    if (activeSSE) { activeSSE.close(); activeSSE = null }
    if (task) await loadUnifiedSession(task)
  }
  errorBanner.value = typeof q.error === 'string' && q.error ? q.error : ''
  await initContextChips(q)
}

async function initContextChips(q: Record<string, any>) {
  const kb = typeof q.kb === 'string' && q.kb ? q.kb : ''
  const agent = typeof q.agent === 'string' && q.agent ? q.agent : ''
  const skill = typeof q.skill === 'string' && q.skill ? q.skill : ''
  const workflow = typeof q.workflow === 'string' && q.workflow ? q.workflow : ''
  const chips: ContextChip[] = []
  if (kb) chips.push({ type: 'kb', label: `知识库 #${kb.slice(0, 8)}`, value: kb })
  if (agent) chips.push({ type: 'agent', label: `Agent #${agent.slice(0, 8)}`, value: agent })
  if (skill) chips.push({ type: 'skill', label: `技能 ${skill}`, value: skill })
  if (workflow) chips.push({ type: 'workflow', label: `工作流 ${workflow}`, value: workflow })
  contextChips.value = chips
  agentCfg.value = null
  // ── 尽力补全展示名 / Agent 配置（失败则保留 id 占位）──
  if (kb) {
    try {
      const res = await api.get(`/v1/kb/${encodeURIComponent(kb)}`)
      const d = res.data?.data || res.data
      if (d?.name) {
        const c = contextChips.value.find(x => x.type === 'kb')
        if (c) c.label = `知识库 ${d.name}`
      }
    } catch { /* 保留 id 占位 */ }
  }
  if (agent) {
    try {
      const res = await api.get('/v1/agents')
      const list = res.data?.data || []
      const a = list.find((x: any) => x.id === agent)
      if (a) {
        agentCfg.value = {
          id: a.id,
          name: a.name,
          system_prompt: a.system_prompt,
          model: a.llm_config?.model,
          max_turns: a.max_turns,
        }
        const c = contextChips.value.find(x => x.type === 'agent')
        if (c && a.name) c.label = `Agent ${a.name}`
      }
    } catch { /* 取不到配置则只传 agent_id，由后端兼容 */ }
  }
  if (workflow) {
    try {
      const res = await api.get('/v1/graphs')
      const list = res.data?.data || []
      const rec = list.find((x: any) => x.id === workflow)
      if (rec?.name) {
        const c = contextChips.value.find(x => x.type === 'workflow')
        if (c) c.label = `工作流 ${rec.name}`
      }
    } catch { /* 无列表时保留原值 */ }
  }
}

/** 移除单个上下文芯片：本地 context 与路由 query 双源同步清空（侧栏上下文面板触发） */
function removeContextChip(type: ContextChip['type']) {
  contextChips.value = contextChips.value.filter(c => c.type !== type)
  if (type === 'agent') agentCfg.value = null
  const q: Record<string, any> = { ...route.query }
  if (q[type] !== undefined) {
    delete q[type]
    void router.replace({ path: '/chat', query: q })
    appliedQueryKey = JSON.stringify(q)
  }
}

/** 清空全部上下文：本地 context 与路由 query（kb/agent/skill/workflow）一并清空 */
function clearContext() {
  contextChips.value = []
  agentCfg.value = null
  const q: Record<string, any> = { ...route.query }
  let changed = false
  for (const key of ['kb', 'agent', 'skill', 'workflow']) {
    if (q[key] !== undefined) { delete q[key]; changed = true }
  }
  if (changed) {
    void router.replace({ path: '/chat', query: q })
    appliedQueryKey = JSON.stringify(q)
  }
}

/** 统一任务模式：清空当前消息区（保留会话与上下文，可继续追问） */
function clearUnifiedMessages() {
  items.value = []
  currentTraceId.value = ''
  message.info('已清空统一任务消息')
}

/** 统一任务模式：退出（移除 task/error query；路由 watcher 触发 applyRouteQuery 重置消息区） */
async function exitUnifiedMode() {
  const q: Record<string, any> = { ...route.query }
  delete q.task
  delete q.error
  await router.replace({ path: '/chat', query: q })
}

/** kb_hits 标签增强：跳转到引用的知识库详情 */
function openKb(kbId: string) {
  if (kbId) void router.push(`/knowledge/${encodeURIComponent(kbId)}`)
}

/** 组装发送时附带的 context（普通 SSE 模式与统一任务模式共用） */
function buildContext(): Record<string, any> | undefined {
  const ctx: Record<string, any> = {}
  const kb = contextChips.value.find(c => c.type === 'kb')
  if (kb) ctx.kb_id = kb.value
  const agent = contextChips.value.find(c => c.type === 'agent')
  if (agent) {
    ctx.agent_id = agent.value
    if (agentCfg.value) {
      ctx.agent = {
        ...(agentCfg.value.name ? { name: agentCfg.value.name } : {}),
        ...(agentCfg.value.system_prompt ? { system_prompt: agentCfg.value.system_prompt } : {}),
        ...(agentCfg.value.model ? { model: agentCfg.value.model } : {}),
        ...(agentCfg.value.max_turns ? { max_turns: agentCfg.value.max_turns } : {}),
      }
    }
  }
  const skills = contextChips.value.filter(c => c.type === 'skill').map(c => c.value)
  if (skills.length) ctx.skill_names = skills
  const wf = contextChips.value.find(c => c.type === 'workflow')
  if (wf) ctx.workflow_id = wf.value
  return Object.keys(ctx).length ? ctx : undefined
}

/** 安全改造：附件签名 URL 解析 — /media/ 公开路径 → 短时效签名 URL；非 /media/ 前缀原样；失败回退原 url */
async function resolveAttachmentUrls(attachments?: ChatAttachment[]): Promise<ChatAttachment[]> {
  if (!attachments?.length) return []
  return Promise.all(attachments.map(async a => {
    if (!a.url || !a.url.startsWith('/media/')) return a
    const url = await resolveMediaUrl({ id: a.id, file_url: a.url })
    return url && url !== a.url ? { ...a, url } : a
  }))
}

/** 拉取统一会话历史（GET /v1/chat/sessions/{id}/messages） */
async function loadUnifiedSession(sessionId: string) {
  loading.value = true
  try {
    const res = await getChatSessionMessages(sessionId)
    const d = (res?.messages ? res : (res?.data || {})) as any
    const list = Array.isArray(d.messages) ? d.messages : []
    // 会话创建时的 mode（shared_context 优先，其次 query.mode，兜底 auto）
    const sharedMode = d.shared_context?.mode
    unifiedSubmitMode.value =
      (typeof sharedMode === 'string' && sharedMode) ||
      (typeof route.query.mode === 'string' && route.query.mode) ||
      'auto'
    items.value = buildUnifiedItems(list)
  } catch {
    errorBanner.value = errorBanner.value || '统一会话加载失败，可直接发送消息继续';
  } finally {
    loading.value = false
  }
}

/** 统一会话消息 → 现有 ChatItem（user/assistant 映射现有消息组件；metadata 带 kb 时插知识库引用标签） */
function buildUnifiedItems(list: any[]): ChatItem[] {
  const out: ChatItem[] = []
  ;(list || []).forEach((m: any, idx: number) => {
    if (!m || (m.role !== 'user' && m.role !== 'assistant')) return
    const content = typeof m.content === 'string' ? m.content : ''
    if (!content) return
    const time = formatClock(m.timestamp || m.created_at)
    if (m.role === 'user') {
      out.push({ kind: 'text', role: 'user', content: stripUserInputTag(content), time, id: `uni_u_${idx}` })
    } else {
      const { reasoning, body } = splitThinking(content, { loose: true })
      if (reasoning) out.push({ kind: 'reasoning', content: reasoning, time, id: `uni_r_${idx}` })
      if (body) {
        out.push({
          kind: 'text', role: 'assistant', content: body, time, id: `uni_a_${idx}`,
          // 反向定位：透传消息 metadata
          metadata: normalizeMeta(m.metadata),
        } as any)
        const meta = normalizeMeta(m.metadata) || {}
        const n = typeof meta.kb_hits === 'number' ? meta.kb_hits : meta.kb_id ? 1 : 0
        if (meta.kb_id || n > 0) {
          out.push({ kind: 'kb_hits', count: n, kb_id: meta.kb_id || '', id: `uni_k_${idx}` } as unknown as ChatItem)
        }
      }
    }
  })
  return out
}

/** 统一任务模式发送：POST /v1/chat/submit，返回 output 追加为 assistant 消息 */
async function sendUnified(text: string, attachments?: ChatAttachment[]) {
  if (!unifiedSessionId.value) return
  loading.value = true
  startTurnTimer()
  appendUserText(text, attachments)
  const userItemId = items.value[items.value.length - 1]?.id
  currentTraceId.value = ''
  try {
    // 安全改造：附件若为 /media/ 公开路径，先解析为签名 URL 再随消息发送（loading 期间发送已禁用）
    const resolvedAtts = await resolveAttachmentUrls(attachments)
    const res = await api.post('/v1/chat/submit', {
      message: text,
      session_id: unifiedSessionId.value,
      mode: unifiedSubmitMode.value || 'auto',
      context: buildContext(),
      // 模型路由：统一任务发送同样携带 llm_config.model（空 = 后端默认）
      llm_config: llmModel.value ? { model: llmModel.value } : {},
      ...(resolvedAtts.length
        ? { attachments: resolvedAtts.map(a => ({ id: a.id, name: a.name, mime_type: a.mimeType, url: a.url, is_image: a.isImage })) }
        : {}),
    })
    const d = res.data?.data !== undefined ? res.data.data : (res.data || {})
    if (d.success === false) throw new Error(d.error || '请求失败')
    currentTraceId.value = d.trace_id || ''
    appendAssistantWithKb(d.output || '', d.metadata || {})
    flashUnifiedDone()
  } catch (e: any) {
    markMessageFailed(userItemId, e.message || '网络错误')
    message.error('发送失败: ' + (e.message || '网络错误'))
  } finally {
    loading.value = false
    stopTurnTimer()
  }
}

/** 追加 assistant 消息；metadata 有 kb_hits/kb_id 时在其下显示“引用了知识库(×N)”小标签 */
function appendAssistantWithKb(content: string, meta: any) {
  const { reasoning, body } = splitThinking(String(content))
  if (reasoning) items.value.push({ kind: 'reasoning', content: reasoning, id: genItemId() })
  if (body) {
    items.value.push({
      kind: 'text', role: 'assistant', content: body, id: genItemId(),
      // 反向定位：透传 /v1/chat/submit 返回的 metadata
      metadata: normalizeMeta(meta),
    } as any)
    const n = typeof meta.kb_hits === 'number' ? meta.kb_hits : meta.kb_id ? 1 : 0
    if (meta.kb_id || n > 0) {
      items.value.push({ kind: 'kb_hits', count: n, kb_id: meta.kb_id || '', id: genItemId() } as unknown as ChatItem)
    }
  }
}

// 统一任务模式：新消息后自动滚到底部
watch(() => items.value.length, async () => {
  if (!unifiedMode.value) return
  await nextTick()
  const el = document.querySelector<HTMLElement>('.unified-list')
  if (el) el.scrollTop = el.scrollHeight
})

// 侧面板（主从时间线：轨迹 / 会话历史）；上下文面板：桌面端（≥1025px）默认展开常驻，≤1024px 折叠为抽屉
const panelOpen = ref(window.matchMedia('(min-width: 1025px)').matches)
const panelView = ref<'trajectory' | 'sessions'>('trajectory')
const trajectoryFocus = ref<number | null>(null)
const trajectoryToken = ref(0)

function onTrajectoryFocus(index: number) {
  trajectoryFocus.value = index
  trajectoryToken.value += 1
}

// 打开面板并直达指定视图；点击已激活的入口则收起
function openPanel(view: 'trajectory' | 'sessions') {
  if (panelOpen.value && panelView.value === view) {
    panelOpen.value = false
    return
  }
  panelView.value = view
  panelOpen.value = true
}

/** ChatInput「上下文」快捷按钮：确保上下文面板展开（抽屉模式下亦然）并直达轨迹视图 */
function openContextPanel() {
  panelView.value = 'trajectory'
  panelOpen.value = true
}

// turn 计时（deepseek turnStatusClock）
const turnElapsed = ref(0)
const connectionLost = ref(false)  // SSE 断线横幅（deepseek ConnectionBanner）
let turnTimer: ReturnType<typeof setInterval> | null = null

function startTurnTimer() {
  turnElapsed.value = 0
  if (turnTimer) clearInterval(turnTimer)
  turnTimer = setInterval(() => { turnElapsed.value += 1 }, 1000)
}

function stopTurnTimer() {
  if (turnTimer) { clearInterval(turnTimer); turnTimer = null }
}

function persistSessions() { localStorage.setItem('chat_sessions', JSON.stringify(sessions.value)) }

// ── 会话 CRUD（保留原逻辑） ──
onMounted(async () => {
  // 互联互通：解析 /chat query（task / error / kb / agent / skill / workflow）
  await applyRouteQuery()
  await loadSessions()
  // 统一任务模式不自动切换普通会话；其余保持原有行为
  if (!unifiedMode.value && sessions.value.length > 0) {
    await switchSession(sessions.value[0].id)
  }
  // 互联互通：同一路由内 query 变化（如 WorkstationNav 再次跳转）
  watch(() => route.query, () => applyRouteQuery())
  // P2-G: 监听网络在线/离线状态
  window.addEventListener('online', onOnline)
  window.addEventListener('offline', onOffline)
  // P2-H: 全局键盘快捷键
  window.addEventListener('keydown', onGlobalKeydown)
})

onUnmounted(() => {
  stopTurnTimer()
  if (activeSSE) { activeSSE.close(); activeSSE = null }
  if (unifiedDoneTimer) { clearTimeout(unifiedDoneTimer); unifiedDoneTimer = null }
  window.removeEventListener('online', onOnline)
  window.removeEventListener('offline', onOffline)
  window.removeEventListener('keydown', onGlobalKeydown)
})

// P2-G: 离线监听 + 自动重连
const isOnline = ref(navigator.onLine)
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let reconnectAttempts = 0

function onOffline() {
  isOnline.value = false
  connectionLost.value = true
  // 离线时停止 SSE 重试
  if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null }
}

function onOnline() {
  isOnline.value = true
  // 上线后指数退避重连，恢复会话
  if (reconnectTimer) clearTimeout(reconnectTimer)
  reconnectAttempts = 0
  attemptReconnect()
}

async function attemptReconnect() {
  if (!isOnline.value) return
  reconnectAttempts++
  // 指数退避：1s, 2s, 4s, 8s, 16s（最多 16s）
  const delay = Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 16000)
  if (reconnectAttempts > 1) {
    // 非首次重连，先等待
    await new Promise(r => setTimeout(r, delay))
  }
  if (!isOnline.value) return
  try {
    // 探测后端是否可达
    await api.get('/health', { timeout: 5000 })
    connectionLost.value = false
    reconnectAttempts = 0
    // 重连成功后重新加载当前会话（可能错过 SSE 事件）
    if (activeSessionId.value) {
      await switchSession(activeSessionId.value)
    }
  } catch {
    // 仍未可达，安排下一次重试（最多 5 次）
    if (reconnectAttempts < 5) {
      reconnectTimer = setTimeout(attemptReconnect, delay)
    }
  }
}

// P2-I: 导出当前会话为 Markdown 文件
function exportMarkdown() {
  if (!items.value.length) {
    message.warning('当前没有可导出的消息')
    return
  }
  const session = sessions.value.find(s => s.id === activeSessionId.value)
  const title = session?.title || '对话导出'
  const lines: string[] = [`# ${title}`, '']
  for (const it of items.value) {
    if (it.kind !== 'text') continue
    const role = it.role === 'user' ? '🧑 用户' : '🤖 助手'
    lines.push(`## ${role}`, '')
    lines.push(it.content || '(空消息)')
    if (it.attachments?.length) {
      lines.push('')
      for (const a of it.attachments) {
        if (a.isImage) lines.push(`![${a.name}](${a.url})`)
        else lines.push(`- 📎 [${a.name}](${a.url}) (${formatSize(a.size)})`)
      }
    }
    lines.push('')
  }
  // 工具调用也导出（便于审计）
  const toolCalls = items.value.filter(i => i.kind === 'tool_call')
  if (toolCalls.length) {
    lines.push('---', '', '## 工具调用记录', '')
    for (const tc of toolCalls) {
      if (tc.kind !== 'tool_call') continue
      lines.push(`### ${tc.name || 'tool'}`, '```json', tc.arguments || '{}', '```', '')
    }
  }
  const md = lines.join('\n')
  const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${title.replace(/[\\/:*?"<>|]/g, '_')}.md`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  message.success('已导出 Markdown')
}

// P3-C: 斜杠命令处理
function onSlashCommand(cmd: string) {
  switch (cmd) {
    case '/clear':
      items.value = []
      activeSessionId.value = ''
      message.info('已清空当前对话')
      break
    case '/export':
      exportMarkdown()
      break
    case '/new':
      items.value = []
      activeSessionId.value = ''
      panelOpen.value = false
      message.info('已新建会话')
      break
    case '/theme':
      themeStore.toggleTheme()
      message.success(themeStore.isDark ? '已切换到暗色模式' : '已切换到亮色模式')
      break
    case '/stop':
      stopGeneration()
      break
    default:
      message.warning(`未知命令: ${cmd}`)
  }
}

// P2-H: 全局键盘快捷键
// Ctrl/Cmd+K: 打开侧边栏 + 切到会话历史视图
// Esc: 关闭侧边栏（若打开）
function onGlobalKeydown(e: KeyboardEvent) {
  // Ctrl/Cmd + K
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    panelOpen.value = true
    panelView.value = 'sessions'
    nextTick(() => {
      const searchInput = document.querySelector('.panel-search .search-input') as HTMLInputElement | null
      searchInput?.focus()
    })
  }
  // Esc 关闭侧边栏
  if (e.key === 'Escape' && panelOpen.value) {
    panelOpen.value = false
  }
}

async function loadSessions() {
  try {
    const res = await api.get('/v1/conversations')
    const apiSessions = res.data?.data || res.data || []
    if (apiSessions.length > 0) { sessions.value = apiSessions; persistSessions() }
    else { const raw = localStorage.getItem('chat_sessions'); sessions.value = raw ? JSON.parse(raw) : [] }
  } catch {
    const raw = localStorage.getItem('chat_sessions')
    if (raw) sessions.value = JSON.parse(raw)
  }
  sortSessions()
}

async function createSession() {
  let session: ChatSession | null = null
  try {
    const res = await api.post('/v1/conversations', { title: '新对话', llm_config: buildLlmConfig() })
    const data = res.data?.data || res.data
    if (data?.id) session = { id: data.id, title: data.title || '新对话', created_at: data.created_at, updated_at: data.updated_at }
  } catch { /* fallback */ }
  if (!session) {
    const id = crypto.randomUUID()
    session = { id, title: '新对话', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
  }
  sessions.value.unshift(session); persistSessions()
  panelView.value = 'trajectory' // 新建会话后回到轨迹视图
  try { await switchSession(session.id) } catch { /* ignore */ }
}

async function switchSession(id: string) {
  // 互联互通：从统一任务模式切到普通会话 → 移除 task/error（保留 kb/agent/skill/workflow 上下文）
  if (unifiedMode.value && unifiedSessionId.value) {
    const q = { ...route.query }
    delete q.task
    delete q.error
    await router.replace({ path: '/chat', query: q })
    appliedQueryKey = JSON.stringify(q)
    unifiedSessionId.value = ''
  }
  if (id === activeSessionId.value) return
  activeSessionId.value = id; items.value = []; loading.value = true
  hasMore.value = false; earliestCursor.value = ''; loadingEarlier.value = false
  initialLoading.value = true  // P2-E: 显示骨架屏
  try {
    const res = await api.get(`/v1/conversations/${id}?limit=${HISTORY_PAGE_SIZE}`)
    const data = res.data?.data || res.data
    if (data?.messages) {
      items.value = mergeHistory(data.messages, data.tool_calls || [])
      earliestCursor.value = data.cursor || ''
      hasMore.value = !!data.has_more
    }
    // ── 会话级模式恢复：历史 llm_config.mode 有效则恢复 mode ref（切换器与 toolbar 同步高亮）──
    let cfg: any = data?.llm_config
    if (typeof cfg === 'string') { try { cfg = JSON.parse(cfg) } catch { cfg = undefined } }
    const savedMode = cfg?.mode
    if (typeof savedMode === 'string' && modeOptions.some(o => o.value === savedMode)) {
      mode.value = savedMode
    }
    // ── 会话级模型恢复：llm_config.model 回填模型下拉（空 = 后端默认）──
    llmModel.value = typeof cfg?.model === 'string' ? cfg.model : ''
  } catch { /* fallback */ } finally {
    loading.value = false
    initialLoading.value = false  // P2-E: 隐藏骨架屏
  }
}

// P 性能：cursor 分页 — 触顶加载更早的消息（首屏只加载最近 HISTORY_PAGE_SIZE 条）
const HISTORY_PAGE_SIZE = 50
const hasMore = ref(false)
const earliestCursor = ref('')
const loadingEarlier = ref(false)
const initialLoading = ref(false)  // P2-E: 首次加载会话历史的骨架屏

function mergeHistory(messages: any[], toolCalls: any[]): ChatItem[] {
  // 重构：工具调用双源渲染 — tool_calls 表为主源，messages 内联 tool_calls 列兜底
  // （旧数据/中断场景 tool_calls 表可能为空，从 assistant 消息内联列还原工具链）
  interface TimelineEntry { t: number; items: ChatItem[] }
  const timeline: TimelineEntry[] = (messages || [])
    .filter((m: any) => (m.role === 'user' || m.role === 'assistant') && m.content)
    .map((m: any) => {
      const clock = formatClock(m.created_at) // S 修复：历史消息日期还原
      const items: ChatItem[] = []
      if (m.role === 'user') {
        // 剥离后端安全净化添加的 <user_input> 包装（Go InputSanitizer）
        items.push({ kind: 'text', role: 'user', content: stripUserInputTag(m.content), time: clock, id: m.id })
      } else {
        // assistant 历史：流式保存的原始文本（含 [thinking] 碎片）→ 宽松拆分
        const { reasoning, body } = splitThinking(m.content, { loose: true })
        if (reasoning) items.push({ kind: 'reasoning', content: reasoning, time: clock, id: `${m.id}:r` })
        if (body) items.push({
          kind: 'text', role: 'assistant', content: body, time: clock, id: m.id,
          // 反向定位：透传消息 metadata（kb_id/kb_hits/workflow_id/agent_id/trace_id）
          metadata: normalizeMeta((m as any)?.metadata),
        } as any)
      }
      return { t: new Date(m.created_at).getTime(), items }
    })

  // 1) 主源：tool_calls 表记录
  const callsById = new Map<string, any>((toolCalls || []).map((tc: any) => [tc.id, tc]))
  // 2) 兜底源：messages 内联 tool_calls 列（存 id 集合 ["c1","c2"]；兼容旧 OpenAI 格式对象数组）
  ;(messages || []).forEach((m: any) => {
    if (m.role !== 'assistant' || !m.tool_calls || m.tool_calls === '[]') return
    let inline: any[]
    try { inline = typeof m.tool_calls === 'string' ? JSON.parse(m.tool_calls) : m.tool_calls } catch { return }
    for (const tc of inline || []) {
      if (!tc) continue
      // id 数组：tool_calls 表已无此 id（旧数据）→ 渲染占位卡片（仅 id 可定位）
      if (typeof tc === 'string') {
        if (!callsById.has(tc)) {
          callsById.set(tc, { id: tc, tool_name: 'tool', input: '', output: '', is_error: false, created_at: m.created_at })
        }
        continue
      }
      if (!tc.id || callsById.has(tc.id)) continue
      callsById.set(tc.id, {
        id: tc.id,
        tool_name: tc.function?.name ?? tc.name,
        input: tc.function?.arguments ?? tc.arguments ?? '',
        output: '',
        is_error: false,
        created_at: m.created_at,
      })
    }
  })

  // 工具调用：tool_call 卡片 + 结果（output 非空时），按发起时间插入
  Array.from(callsById.values()).forEach((tc: any) => {
    const callItems: ChatItem[] = [{
      kind: 'tool_call', id: tc.id, name: tc.tool_name,
      arguments: tc.input || '', status: 'done',
    }]
    if (tc.output) {
      callItems.push({
        kind: 'tool_result', toolCallId: tc.id, id: `${tc.id}:res`,
        content: tc.output, isError: !!tc.is_error,
      })
    }
    timeline.push({ t: new Date(tc.created_at).getTime(), items: callItems })
  })
  timeline.sort((a, b) => a.t - b.t)
  const flat = timeline.flatMap(e => e.items)
  // UI/UX：跨天消息插入日期分隔线（deepseek DateDivider）；单条或多条同天则无需分隔
  const merged: ChatItem[] = []
  let prevDay = ''
  timeline.forEach((e, i) => {
    const d = new Date(e.t)
    const dayKey = `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`
    if (i > 0 && prevDay !== dayKey) {
      merged.push({ kind: 'date_divider', content: `${d.getMonth() + 1}月${d.getDate()}日`, id: `date-${dayKey}-${i}` })
    }
    merged.push(...e.items)
    prevDay = dayKey
  })
  return merged.length > flat.length ? merged : flat
}

async function loadEarlier() {
  if (loadingEarlier.value || !hasMore.value || !activeSessionId.value || !earliestCursor.value) return
  loadingEarlier.value = true
  const el = document.querySelector<HTMLElement>('.message-list')
  const prevHeight = el ? el.scrollHeight : 0
  try {
    const res = await api.get(
      `/v1/conversations/${activeSessionId.value}?limit=${HISTORY_PAGE_SIZE}&before=${encodeURIComponent(earliestCursor.value)}`,
    )
    const data = res.data?.data || res.data
    if (data?.messages?.length) {
      const earlier = mergeHistory(data.messages, data.tool_calls || [])
      items.value = [...earlier, ...items.value]
      earliestCursor.value = data.cursor || ''
      hasMore.value = !!data.has_more
    } else {
      hasMore.value = false
    }
  } catch {
    hasMore.value = false
  } finally {
    loadingEarlier.value = false
    // 保持滚动位置（顶部插入内容后补偿）
    await nextTick()
    if (el) el.scrollTop = el.scrollHeight - prevHeight
  }
}

async function deleteSession(id: string) {
  try { await api.delete(`/v1/conversations/${id}`) } catch { /* 保留本地删除 */ }
  sessions.value = sessions.value.filter(s => s.id !== id); persistSessions()
  if (activeSessionId.value === id) {
    activeSessionId.value = ''; items.value = []
    if (sessions.value.length > 0) await switchSession(sessions.value[0].id)
  }
}

// 删除前确认（菜单 danger 项 → Modal.confirm）
function requestDelete(id: string) {
  const s = sessions.value.find(x => x.id === id)
  Modal.confirm({
    title: '删除对话',
    content: `确定删除「${s?.title || '新对话'}」？此操作不可恢复。`,
    okText: '删除',
    okButtonProps: { danger: true },
    cancelText: '取消',
    onOk: () => deleteSession(id),
  })
}

// ── 重命名（deepseek session rename dialog：Modal + 行内输入） ──
const renameTarget = ref<ChatSession | null>(null)
const renameDraft = ref('')
const renaming = ref(false)

function openRename(id: string, currentTitle: string) {
  const s = sessions.value.find(x => x.id === id)
  if (!s) return
  renameTarget.value = s
  renameDraft.value = currentTitle
}

async function confirmRename() {
  const target = renameTarget.value
  const title = renameDraft.value.trim()
  if (!title || !target) return
  renaming.value = true
  try {
    // 会话级持久化：重命名时一并保存 llm_config（含 mode）
    await updateConversation(target.id, { title, llm_config: buildLlmConfig() } as any)
    const s = sessions.value.find(x => x.id === target.id)
    if (s) s.title = title
    persistSessions()
    message.success('已重命名')
    renameTarget.value = null
  } catch (e: any) {
    message.error('重命名失败: ' + (e?.response?.data?.error || e?.message || '网络错误'))
  } finally {
    renaming.value = false
  }
}

// ── 置顶（列表排序：pinned DESC → updated_at DESC） ──
function sortSessions() {
  sessions.value = [...sessions.value].sort((a, b) => {
    if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1
    return new Date(b.updated_at || 0).getTime() - new Date(a.updated_at || 0).getTime()
  })
}

async function togglePin(id: string, pinned: boolean) {
  const s = sessions.value.find(x => x.id === id)
  if (!s) return
  const prev = s.pinned
  s.pinned = pinned
  sortSessions(); persistSessions()
  try {
    // 会话级持久化：置顶时一并保存 llm_config（含 mode）
    await updateConversation(id, { pinned, llm_config: buildLlmConfig() } as any)
  } catch {
    s.pinned = prev // 失败回滚
    sortSessions(); persistSessions()
    message.error('置顶操作失败')
  }
}

// P3-D: 设置会话标签（前端 localStorage 持久化，无需后端支持）
function setSessionTag(id: string, tag: string) {
  const s = sessions.value.find(x => x.id === id)
  if (!s) return
  s.tag = tag || undefined
  persistSessions()
  message.success(tag ? `已设置标签：${tag}` : '已清除标签')
}

// ── 分享（chat.deepseek.com/share/{id} 风格：选消息 → 生成链接 → 可取消） ──
const shareOpen = ref(false)
const shareTarget = ref<ChatSession | null>(null)
const shareInfo = ref<ShareInfo | null>(null)
const shareLoading = ref(false)
const shareRevoking = ref(false)
const shareError = ref('')
const shareMessageIds = ref<string[]>([])

const isGuest = computed(() => !authStore.user)

// 分享候选：会话中所有文本消息（用户可勾选；工具调用/思考块不分享）
const shareCandidates = computed(() => items.value
  .filter((it): it is Extract<ChatItem, { kind: 'text' }> =>
    it.kind === 'text' && (it.role === 'user' || it.role === 'assistant') && !!it.id)
  .map(it => ({
    id: it.id as string,
    role: it.role,
    preview: (it.content || '').replace(/\s+/g, ' ').trim().slice(0, 56),
  })))

async function openShare(id: string) {
  const s = sessions.value.find(x => x.id === id)
  if (!s) return
  // 分享的可能不是当前会话：先切换加载其消息，保证消息选择列表正确
  if (id !== activeSessionId.value) {
    await switchSession(id)
  }
  shareTarget.value = s
  shareInfo.value = null
  shareError.value = ''
  shareMessageIds.value = shareCandidates.value.map(c => c.id) // 默认全选
  if (!isGuest.value) {
    try { shareInfo.value = await getActiveShare(s.id) } catch { /* 无活跃分享 */ }
  }
  shareOpen.value = true
}

function toggleShareMessage(id: string) {
  const i = shareMessageIds.value.indexOf(id)
  if (i >= 0) shareMessageIds.value.splice(i, 1)
  else shareMessageIds.value.push(id)
}

async function generateShare() {
  if (!shareTarget.value) return
  if (shareMessageIds.value.length === 0) { message.warning('请至少选择一条要分享的消息'); return }
  shareLoading.value = true
  shareError.value = ''
  try {
    shareInfo.value = await createShare(shareTarget.value.id, shareMessageIds.value)
  } catch (e: any) {
    shareError.value = e?.response?.data?.error || '生成分享链接失败'
  } finally {
    shareLoading.value = false
  }
}

async function revokeCurrentShare() {
  if (!shareTarget.value || !shareInfo.value) return
  shareRevoking.value = true
  try {
    await revokeShare(shareTarget.value.id)
    shareInfo.value = null
    message.success('分享已取消，链接已失效')
  } catch {
    message.error('取消分享失败')
  } finally {
    shareRevoking.value = false
  }
}

function shareUrl(): string {
  return `${window.location.origin}/share/${shareInfo.value?.share_id || ''}`
}

async function copyShareLink() {
  try {
    await navigator.clipboard.writeText(shareUrl())
    message.success('链接已复制')
  } catch {
    message.error('复制失败')
  }
}

// ── SSE 编排：事件 → ChatItem ──
// 流式缓冲：累积 assistant 原始文本后整体重算（跨 chunk 的 [thinking] 标签正确配对）
let streamBuf = ''
let streamTextId = ''
let streamReasonId = ''

function resetStreamState() {
  streamBuf = ''
  streamTextId = ''
  streamReasonId = ''
}

function appendUserText(text: string, attachments?: ChatAttachment[]) {
  items.value.push({ kind: 'text', role: 'user', content: text, id: genItemId(), attachments })
}

// P 性能/正确性：稳定 id（虚拟列表 key + 流式定位，loadEarlier 头部插入不错位）
let itemIdSeq = 0
function genItemId() {
  return `msg_${Date.now().toString(36)}_${itemIdSeq++}`
}

function onTextChunk(text: string) {
  streamBuf += text
  const { reasoning, body } = splitThinking(streamBuf)
  if (reasoning) {
    const existing = items.value.find(it => it.id === streamReasonId)
    if (existing?.kind === 'reasoning') {
      existing.content = reasoning
    } else {
      const id = genItemId()
      streamReasonId = id
      items.value.push({ kind: 'reasoning', content: reasoning, streaming: true, id })
    }
  }
  if (body) {
    const existing = items.value.find(it => it.id === streamTextId)
    if (existing?.kind === 'text' && existing.role === 'assistant') {
      existing.content = body
    } else {
      const id = genItemId()
      streamTextId = id
      items.value.push({ kind: 'text', role: 'assistant', content: body, streaming: true, id })
    }
  }
}

function flushStreamingFlags() {
  for (const it of items.value) {
    if (it.kind === 'text' && it.streaming) it.streaming = false
    if (it.kind === 'reasoning' && it.streaming) it.streaming = false
    if (it.kind === 'tool_call' && it.status === 'running') it.status = 'done'
  }
  resetStreamState()
}

function onSSEMessage(raw: any) {
  const type = raw?.type
  const d = raw?.data || {}
  if (type === 'text') {
    const text = d?.content ?? raw?.content ?? ''
    if (!text) return
    onTextChunk(text)
  } else if (type === 'tool_call') {
    items.value.push({
      kind: 'tool_call', id: d?.id ?? String(Date.now()), name: d?.name ?? 'tool',
      arguments: d?.arguments ?? '', status: 'running',
    })
  } else if (type === 'tool_result') {
    const callId = d?.tool_call_id ?? d?.id ?? ''
    const call = items.value.find(it => it.kind === 'tool_call' && it.id === callId)
    if (call && call.kind === 'tool_call') call.status = 'done'
    const content = d?.content ?? d?.result ?? ''
    if (content) {
      items.value.push({
        kind: 'tool_result', toolCallId: callId, id: `${callId}:res`,
        content: typeof content === 'string' ? content : JSON.stringify(content),
        isError: !!d?.error,
      })
    }
  } else if (type === 'done') {
    flushStreamingFlags()
    loading.value = false
    stopTurnTimer()
    activeSSE?.close(); activeSSE = null
    // ── 捕获 trace_id (用于 Timeline 查询) ────────────────────
    currentTraceId.value = d?.trace_id || ''
    // ── 反向定位：done 事件携带 metadata（kb_id/kb_hits/workflow_id/agent_id）→ 挂到流式 assistant 消息 ──
    const doneMeta = normalizeMeta(d?.metadata)
    if (doneMeta && Object.keys(doneMeta).length && streamTextId) {
      const streamItem = items.value.find(x => x.id === streamTextId)
      if (streamItem?.kind === 'text' && streamItem.role === 'assistant') {
        ;(streamItem as any).metadata = { ...((streamItem as any).metadata || {}), ...doneMeta }
      }
    }
    const it = d?.input_tokens ?? 0
    const ot = d?.output_tokens ?? 0
    if (it || ot) {
      items.value.push({
        kind: 'turn_stats', inputTokens: it, outputTokens: ot,
        durationSec: turnElapsed.value,
      })
    }
  } else if (type === 'approval') {
    // S 安全修复：工具确认请求 — 展示确认卡片，等待用户允许/拒绝
    const callId = d?.id ?? d?.tool_call_id ?? String(Date.now())
    pendingApprovals.value.push({
      id: callId,
      toolName: d?.name ?? 'tool',
      arguments: d?.arguments ?? '',
    })
  } else if (type === 'guardrail_blocked') {
    // S 安全修复：输入/输出栅栏拦截 — 提示并停止
    flushStreamingFlags()
    loading.value = false
    stopTurnTimer()
    activeSSE?.close(); activeSSE = null
    message.warning(d?.content || '请求被安全策略拦截')
  } else if (type === 'error') {
    flushStreamingFlags()
    loading.value = false
    stopTurnTimer()
    activeSSE?.close(); activeSSE = null
    message.error(d?.content || d?.error || '请求失败')
  }
}

async function sendMessage(text: string, attachments?: ChatAttachment[]) {
  // 互联互通：统一任务模式 → POST /v1/chat/submit（与 SSE 流式并列的新路径）
  if (unifiedMode.value && unifiedSessionId.value) {
    await sendUnified(text, attachments)
    return
  }
  loading.value = true
  startTurnTimer()
  resetStreamState()
  connectionLost.value = false
  appendUserText(text, attachments)
  const userItemId = items.value[items.value.length - 1]?.id
  const sessionId = activeSessionId.value || crypto.randomUUID()
  currentTraceId.value = ''  // 清空上一次 trace_id
  try {
    if (activeSSE) { activeSSE.close(); activeSSE = null }
    activeSSE = await createSSEConnection(
      sessionId,
      onSSEMessage,
      () => {
        loading.value = false
        stopTurnTimer()
        connectionLost.value = true   // SSE 断线 → 顶部横幅（deepseek ConnectionBanner）
        activeSSE?.close(); activeSSE = null
        // P1-3: 标记用户消息为失败，展示重试按钮
        markMessageFailed(userItemId, '连接已断开')
      },
    )
    const body: any = { content: text, session_id: sessionId, llm_config: buildLlmConfig() }
    // 互联互通：上下文附加（知识库/Agent/技能/工作流，普通 SSE 模式同样携带）
    const ctx = buildContext()
    if (ctx) body.context = ctx
    // 安全改造：附件若为 /media/ 公开路径，先解析为短时效签名 URL 再发送（loading 期间发送已禁用）
    const resolvedAtts = await resolveAttachmentUrls(attachments)
    if (resolvedAtts.length) {
      body.attachments = resolvedAtts.map(a => ({ id: a.id, name: a.name, mime_type: a.mimeType, url: a.url, is_image: a.isImage }))
    }
    await api.post('/submit', body)
    activeSessionId.value = sessionId
  } catch (e: any) {
    loading.value = false
    stopTurnTimer()
    flushStreamingFlags()
    // P1-3: 标记用户消息为失败，展示重试按钮（而非仅 toast）
    markMessageFailed(userItemId, e.message || '网络错误')
    message.error('发送失败: ' + (e.message || '网络错误'))
  }
}

// ── P1-3 失败消息标记 ──
function markMessageFailed(itemId: string | undefined, errorMsg: string) {
  if (!itemId) return
  const it = items.value.find(i => i.id === itemId)
  if (it && it.kind === 'text') {
    it.error = true
    it.errorMsg = errorMsg
  }
}

// ── P1-1 消息重试/重新生成 ──
/** 删除指定 itemId 及其后所有消息，返回被删除的用户消息文本（如有） */
function truncateFrom(itemId: string): { text?: string; attachments?: ChatAttachment[] } {
  const idx = items.value.findIndex(i => i.id === itemId)
  if (idx < 0) return {}
  const removed = items.value.slice(idx)
  items.value = items.value.slice(0, idx)
  // 找到被删除的用户消息文本（用于 regenerate：取上一条用户消息）
  const userMsg = removed.find(i => i.kind === 'text' && i.role === 'user') as any
  return userMsg ? { text: userMsg.content, attachments: userMsg.attachments } : {}
}

/** 用户消息编辑后重发：删除该消息及之后所有，用新文本重发 */
function retryFromUserMessage(itemId: string, newText: string) {
  truncateFrom(itemId)
  sendMessage(newText)
}

/** 助手消息重新生成：删除该消息及之后所有，取上一条用户消息重发 */
function regenerateAssistant(itemId: string) {
  // 找到这条助手消息之前的最后一条用户消息
  const idx = items.value.findIndex(i => i.id === itemId)
  if (idx < 0) return
  // 向前找最近一条用户消息
  let userMsg: any = null
  for (let i = idx - 1; i >= 0; i--) {
    const it = items.value[i]
    if (it.kind === 'text' && it.role === 'user') { userMsg = it; break }
  }
  truncateFrom(itemId)
  if (userMsg) {
    // 连同附件一起重发（注意：这里也删掉了之前的用户消息，所以要带回去）
    sendMessage(userMsg.content, userMsg.attachments)
  } else {
    message.warning('未找到对应的用户消息，无法重新生成')
  }
}

/** P1-3 失败消息重试：清除错误状态，用原文本重发 */
function retryFailedMessage(itemId: string) {
  const idx = items.value.findIndex(i => i.id === itemId)
  if (idx < 0) return
  const it = items.value[idx]
  if (it.kind !== 'text') return
  const text = it.content
  const attachments = it.attachments
  // 删除该失败消息及之后所有，重发
  truncateFrom(itemId)
  sendMessage(text, attachments)
}

function stopGeneration() {
  stopTurnTimer()
  if (activeSSE) { activeSSE.close(); activeSSE = null }
  loading.value = false
  flushStreamingFlags()
  // P2-F: 标记最后一条助手消息为"已停止"，在下方显示"继续生成"提示
  const last = items.value[items.value.length - 1]
  if (last && last.kind === 'text' && last.role === 'assistant') {
    last.stopped = true
  }
}

// P2-F: 继续生成（停止后）— 等价于重新生成最后一条助手消息
function continueGeneration() {
  const last = items.value[items.value.length - 1]
  if (last && last.kind === 'text' && last.role === 'assistant' && last.stopped) {
    // 用 regenerate 逻辑：删除该消息及之后所有，用上一条用户消息重发
    regenerateAssistant(last.id!)
  }
}
</script>

<template>
  <div class="chat-layout">
    <div class="chat-main">
      <div v-if="connectionLost" class="connection-banner">
        {{ isOnline ? '与服务器的连接已断开，正在尝试重连…' : '网络已断开，请检查网络连接' }}
      </div>
      <div class="chat-body">
        <!-- 内容区工具条：对话名称居中（避开左上角品牌胶囊），右侧会话/轨迹入口 -->
        <div class="chat-toolbar">
          <div class="toolbar-side" aria-hidden="true" />
          <div class="toolbar-center">
            <span class="toolbar-title">{{ unifiedMode ? '统一任务' : (activeSession?.title || 'MiniCC') }}</span>
            <span class="toolbar-mode">{{ unifiedMode ? (unifiedSubmitMode || 'auto') : (modeOptions.find(o => o.value === mode)?.label || '常规') }}</span>
          </div>
          <div class="toolbar-side toolbar-actions">
            <Button
              type="text" size="small" class="toolbar-btn"
              :class="{ active: panelOpen && panelView === 'sessions' }"
              :title="panelOpen && panelView === 'sessions' ? '收起会话列表' : '会话历史'"
              @click="openPanel('sessions')"
            >
              <template #icon><MenuOutlined /></template>
              <span class="toolbar-label">会话</span>
            </Button>
            <Button
              type="text" size="small" class="toolbar-btn"
              :class="{ active: panelOpen && panelView === 'trajectory' }"
              :title="panelOpen && panelView === 'trajectory' ? '收起轨迹' : '查看历史提问'"
              @click="openPanel('trajectory')"
            >
              <template #icon><HistoryOutlined /></template>
              <span class="toolbar-label">轨迹</span>
            </Button>
            <!-- P2-I: 导出当前会话为 Markdown -->
            <Button
              type="text" size="small" class="toolbar-btn"
              title="导出为 Markdown"
              :disabled="!items.length"
              @click="exportMarkdown"
            >
              <template #icon><ExportOutlined /></template>
              <span class="toolbar-label">导出</span>
            </Button>
            <!-- P3-A: 暗色模式切换 -->
            <Button
              type="text" size="small" class="toolbar-btn"
              :title="themeStore.isDark ? '切换到亮色模式' : '切换到暗色模式'"
              @click="themeStore.toggleTheme()"
            >
              <template #icon>
                <BulbFilled v-if="themeStore.isDark" />
                <BulbOutlined v-else />
              </template>
            </Button>
          </div>
        </div>

        <!-- 互联互通：错误提示条（query.error） -->
        <div v-if="errorBanner" class="unified-error-banner">
          <span class="ueb-text">{{ errorBanner }}</span>
          <CloseOutlined class="ueb-close" title="关闭" @click="errorBanner = ''" />
        </div>

        <!-- 互联互通：统一任务模式（WorkstationNav 发起；POST /v1/chat/submit，不走 SSE） -->
        <template v-if="unifiedMode">
          <!-- 统一任务标识 + 会话 mode 标签 + 清空/退出（编排中脉冲 + 完成态过渡） -->
          <div class="unified-bar">
            <span class="ub-badge" :class="{ running: loading, done: unifiedJustFinished }">
              {{ loading ? '编排中' : unifiedJustFinished ? '完成' : '统一任务' }}
            </span>
            <span class="ub-mode">{{ unifiedSubmitMode || 'auto' }}</span>
            <span class="ub-spacer" />
            <button type="button" class="ub-btn" :disabled="!items.length" @click="clearUnifiedMessages">清空</button>
            <button type="button" class="ub-btn exit" title="退出统一任务模式" @click="exitUnifiedMode">退出</button>
          </div>
          <!-- 执行中：正在编排/执行子任务的轻量提示（CallChainTimeline 在 trace 就绪后展示） -->
          <div v-if="loading" class="unified-exec-hint">
            <span class="ueh-dot" />正在编排/执行子任务…<template v-if="turnElapsed >= 2">&nbsp;·&nbsp;{{ turnElapsed }}s</template>
          </div>
          <div v-if="!items.length && !loading" class="unified-empty">统一任务会话已就绪，直接发送消息即可继续追问</div>
          <div class="unified-list">
            <template v-for="(it, i) in items" :key="it.id ?? i">
              <MessageItem
                v-if="it.kind === 'text' || it.kind === 'reasoning'"
                :item="it"
                @retry-from="retryFromUserMessage"
                @regenerate="regenerateAssistant"
                @continue="continueGeneration"
                @retry-failed="retryFailedMessage"
              />
              <div v-else-if="(it as any).kind === 'kb_hits'" class="kb-hits-tag">
                <span class="kb-hits-text">引用了知识库（×{{ (it as any).count || 1 }}）</span>
                <a
                  v-if="(it as any).kb_id"
                  class="kb-hits-link"
                  href="#"
                  title="查看引用的知识库"
                  @click.prevent="openKb((it as any).kb_id)"
                >查看知识库</a>
              </div>
            </template>
          </div>
          <CallChainTimeline
            v-if="currentTraceId && !loading"
            :trace-id="currentTraceId"
            :tenant-id="authStore.user?.tenant_id || ''"
          />
        </template>

        <!-- 原有 SSE 流式模式 -->
        <ChatEmptyHero v-else-if="items.length === 0 && !loading" @suggest="sendMessage" />
        <template v-else>
          <div v-if="loading" class="turn-status">思考中<template v-if="turnElapsed >= 2">&nbsp;·&nbsp;{{ turnElapsed }}s</template></div>
          <MessageList
            :items="items"
            :loading="loading"
            :initial-loading="initialLoading"
            :focus-index="trajectoryFocus"
            :focus-token="trajectoryToken"
            :has-more="hasMore"
            :loading-earlier="loadingEarlier"
            @load-earlier="loadEarlier"
            @retry-from="retryFromUserMessage"
            @regenerate="regenerateAssistant"
            @continue="continueGeneration"
            @retry-failed="retryFailedMessage"
          />
          <!-- ── Trace 调用链时间线 (仅在有 trace_id 且非 loading 时显示) ── -->
          <CallChainTimeline
            v-if="currentTraceId && !loading"
            :trace-id="currentTraceId"
            :tenant-id="authStore.user?.tenant_id || ''"
          />
        </template>
      </div>

      <div v-if="pendingApprovals.length" class="approval-zone">
        <div v-for="a in pendingApprovals" :key="a.id" class="approval-card">
          <div class="approval-info">
            <span class="approval-tag">工具确认</span>
            <span class="approval-name">{{ a.toolName }}</span>
          </div>
          <div class="approval-args">{{ a.arguments }}</div>
          <div class="approval-actions">
            <button class="approval-btn danger" type="button" @click="resolveApproval(a, false)">拒绝</button>
            <button class="approval-btn allow" type="button" @click="resolveApproval(a, true)">允许执行</button>
          </div>
        </div>
      </div>

      <ChatInput
        :loading="loading"
        :mode="mode"
        :mode-options="modeOptions"
        :model="llmModel"
        :session-id="unifiedMode ? unifiedSessionId : activeSessionId"
        @send="sendMessage"
        @stop="stopGeneration"
        @update:mode="onModeChange"
        @model-change="onModelChange"
        @command="onSlashCommand"
        @open-panel="openContextPanel"
      />
    </div>

    <!-- 侧面板自由浮动抽屉（轨迹 ⇄ 会话历史主从导航）+ 点击遮罩关闭 -->
    <Transition name="overlay-fade">
      <div v-if="panelOpen" class="panel-overlay" @click="panelOpen = false"></div>
    </Transition>
    <ChatSidePanel
      :open="panelOpen"
      :view="panelView"
      :items="items"
      :selected-index="trajectoryFocus"
      :sessions="sessions"
      :active-session-id="activeSessionId"
      :user-name="authStore.user?.name"
      :context-chips="contextChips"
      @update:view="(v: 'trajectory' | 'sessions') => (panelView = v)"
      @focus="onTrajectoryFocus"
      @close="panelOpen = false"
      @create="createSession"
      @switch="switchSession"
      @delete="requestDelete"
      @rename="openRename"
      @pin="togglePin"
      @share="openShare"
      @tag="setSessionTag"
      @remove-context="removeContextChip"
      @clear-context="clearContext"
    />

    <!-- 重命名对话框 -->
    <Modal
      :open="!!renameTarget"
      title="重命名对话"
      :confirm-loading="renaming"
      ok-text="保存"
      cancel-text="取消"
      @ok="confirmRename"
      @cancel="renameTarget = null"
    >
      <Input
        v-model:value="renameDraft"
        placeholder="输入新的对话名称"
        :maxlength="120"
        @press-enter="confirmRename"
      />
    </Modal>

    <!-- 分享对话框（选消息 → 生成链接 → 可随时取消） -->
    <Modal
      :open="shareOpen"
      :title="`分享「${shareTarget?.title || '新对话'}」`"
      :footer="null"
      width="560px"
      @cancel="shareOpen = false"
    >
      <Alert
        type="warning"
        show-icon
        class="share-risk"
        message="分享链接对任何获得链接的人可见"
        description="请勿分享包含敏感或隐私信息的内容。你可以随时取消分享，取消后链接立即失效。"
      />

      <template v-if="isGuest">
        <div class="share-guest-tip">登录后即可生成分享链接。</div>
      </template>

      <template v-else-if="shareInfo">
        <div class="share-link-row">
          <Input :model-value="shareUrl()" readonly class="share-link-input">
            <template #prefix><LinkOutlined /></template>
          </Input>
          <Button type="primary" @click="copyShareLink">
            <template #icon><CopyOutlined /></template>
            复制链接
          </Button>
        </div>
        <div class="share-manage">
          <span class="share-manage-hint">链接已公开，任何获得链接的人均可查看。</span>
          <Button danger :loading="shareRevoking" @click="revokeCurrentShare">取消分享</Button>
        </div>
      </template>

      <template v-else>
        <div class="share-select-title">选择要分享的消息（{{ shareMessageIds.length }}/{{ shareCandidates.length }}）</div>
        <div class="share-select-list">
          <label
            v-for="c in shareCandidates"
            :key="c.id"
            class="share-select-item"
            @click.prevent="toggleShareMessage(c.id)"
          >
            <Checkbox :checked="shareMessageIds.includes(c.id)" @click.stop />
            <span class="share-select-role" :class="c.role">{{ c.role === 'user' ? '我' : 'AI' }}</span>
            <span class="share-select-preview">{{ c.preview || '（空消息）' }}</span>
          </label>
        </div>
        <div v-if="shareError" class="share-error">{{ shareError }}</div>
        <div class="share-actions">
          <Button type="primary" :loading="shareLoading" @click="generateShare">生成分享链接</Button>
        </div>
      </template>
    </Modal>
  </div>
</template>

<style scoped>
.chat-layout { position: relative; display: flex; height: 100%; background: var(--bg-page); overflow: hidden; }
.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.chat-body { position: relative; flex: 1; display: flex; flex-direction: column; min-height: 0; }
/* 内容区工具条：三列网格，对话名称绝对居中（避开左上角品牌胶囊） */
.chat-toolbar {
  flex: none;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  height: 40px;
  padding: 0 16px;
  border-bottom: 1px solid var(--border);
}
.toolbar-side { display: flex; align-items: center; gap: 8px; min-width: 0; }
.toolbar-center {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  justify-content: center;
}
.toolbar-title {
  max-width: 40vw;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.toolbar-mode { flex: none; font-size: 11px; color: var(--text-tertiary); background: var(--bg-secondary); padding: 2px 8px; border-radius: var(--radius-full); }
.toolbar-actions { justify-content: flex-end; gap: 2px; }
.toolbar-btn { color: var(--text-secondary); border-radius: var(--radius-md); }
.toolbar-btn:hover { color: var(--text-primary) !important; background: var(--bg-hover) !important; }
.toolbar-btn.active { color: var(--primary); background: var(--primary-bg); }
.toolbar-btn:not(:disabled):active { transform: scale(0.94); }
/* ── 响应式：≤1024px 侧栏抽屉化；≤768px 工具栏图标化；≤576px 极窄适配 ── */
@media (max-width: 1024px) {
  .chat-toolbar { padding: 0 10px; }
  .toolbar-title { max-width: 32vw; }
}
@media (max-width: 768px) {
  .chat-toolbar { padding: 0 8px; }
  .toolbar-actions { gap: 0; }
  .toolbar-btn.ant-btn { height: 36px; min-width: 36px; padding: 0 8px; }
  .toolbar-label { display: none; } /* 图标常驻，语义由 title 承担 */
}
@media (max-width: 576px) {
  .chat-toolbar { padding: 0 6px; }
  .toolbar-title { max-width: 24vw; font-size: 12px; }
  .toolbar-mode { display: none; }
  .approval-zone { padding: 0 12px 8px; }
  .turn-status { margin: 8px auto 0; padding: 0 12px; }
}
/* S 安全修复：工具确认卡片 */
.approval-zone { padding: 0 20px 8px; display: flex; flex-direction: column; gap: 8px; }
.approval-card { background: var(--bg-card); border: 1px solid var(--border); border-left: 3px solid var(--primary); border-radius: 10px; padding: 10px 14px; }
.approval-info { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.approval-tag { font-size: 11px; color: var(--primary); background: var(--primary-bg); padding: 2px 8px; border-radius: 10px; }
.approval-name { font-weight: 600; font-size: 13px; color: var(--text-primary); }
.approval-args { font-family: var(--font-mono); font-size: 12px; color: var(--text-muted); word-break: break-all; margin-bottom: 8px; }
.approval-actions { display: flex; gap: 8px; }
.approval-btn { border: none; border-radius: 8px; padding: 6px 16px; font-size: 13px; cursor: pointer; transition: transform 0.1s ease, opacity 0.15s ease, background 0.15s ease; }
.approval-btn:active { transform: scale(0.97); }
.approval-btn.allow { background: var(--primary); color: #fff; }
.approval-btn.allow:hover { opacity: 0.9; }
.approval-btn.danger { background: var(--bg-hover); color: var(--text-primary); }
.approval-btn.danger:hover { background: var(--danger-bg, rgba(239,68,68,.12)); color: var(--danger, #ef4444); }
/* 连接断线横幅（deepseek ConnectionBanner：fixed 顶部全宽红底白字） */
.connection-banner {
  position: fixed; top: 0; left: 0; right: 0; z-index: 100;
  padding: 4px 12px; text-align: center;
  font-size: 12px; line-height: 18px;
  background: var(--error); color: #fff;
}
/* 分享对话框 */
.share-risk { margin-bottom: 14px; }
.share-guest-tip { padding: 20px 0; text-align: center; color: var(--text-secondary); font-size: 14px; }
.share-link-row { display: flex; gap: 10px; margin-top: 14px; }
.share-link-input { flex: 1; }
.share-manage { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 14px; }
.share-manage-hint { font-size: 12px; color: var(--text-tertiary); }
.share-select-title { font-size: 13px; font-weight: 600; color: var(--text-primary); margin: 14px 0 8px; }
.share-select-list { display: flex; flex-direction: column; gap: 2px; max-height: 260px; overflow-y: auto; width: 100%; }
.share-select-item {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 8px; border-radius: 8px; cursor: pointer;
  transition: background 0.15s ease;
}
.share-select-item:hover { background: var(--bg-hover); }
.share-select-role { flex: none; font-size: 11px; font-weight: 600; padding: 1px 7px; border-radius: 10px; }
.share-select-role.user { color: var(--primary); background: var(--primary-bg); }
.share-select-role.assistant { color: var(--text-secondary); background: var(--bg-secondary); }
.share-select-preview { flex: 1; min-width: 0; font-size: 13px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.share-error { margin-top: 10px; font-size: 12px; color: var(--error); }
.share-actions { display: flex; justify-content: flex-end; margin-top: 14px; }
/* turn 状态条：品牌蓝文字流光（deepseek turnStatus） */
.turn-status {
  align-self: flex-start; margin: 10px auto 0; max-width: 748px; padding: 0 24px;
  height: 26px; display: inline-flex; align-items: center;
  font-size: 13px; font-weight: 600; white-space: nowrap;
  background: linear-gradient(90deg, var(--primary) 0%, var(--primary) 40%, var(--accent) 50%, var(--primary) 60%, var(--primary) 100%);
  background-position: 100% 0; background-size: 250% 100%; background-clip: text; -webkit-background-clip: text;
  color: transparent; -webkit-text-fill-color: transparent;
  animation: turnStatusShimmer 1.8s linear infinite;
  font-variant-numeric: tabular-nums;
}
@keyframes turnStatusShimmer { to { background-position: 0 0; } }
@media (prefers-reduced-motion: reduce) {
  .turn-status { background-position: 0 0; background-size: 100% 100%; animation: none; }
}
/* 侧面板遮罩：点击关闭（z 低于面板 120） */
.panel-overlay {
  position: fixed; inset: 0; z-index: 110;
  background: rgba(10, 10, 12, 0.35);
}
/* 桌面端（≥1025px）：上下文面板常驻展开，无需遮罩 */
@media (min-width: 1025px) { .panel-overlay { display: none; } }
.overlay-fade-enter-active, .overlay-fade-leave-active { transition: opacity 0.2s ease; }
.overlay-fade-enter-from, .overlay-fade-leave-to { opacity: 0; }

/* ── 互联互通：错误提示条（query.error）── */
.unified-error-banner {
  flex: none; display: flex; align-items: center; gap: 8px;
  margin: 8px 20px 0; padding: 8px 12px;
  background: rgba(239, 68, 68, 0.08); border: 1px solid rgba(239, 68, 68, 0.25);
  border-radius: 10px; color: var(--danger, #ef4444); font-size: 13px;
}
.ueb-text { flex: 1; min-width: 0; line-height: 1.5; }
.ueb-close { flex: none; cursor: pointer; opacity: 0.7; font-size: 12px; }
.ueb-close:hover { opacity: 1; }

/* ── 互联互通：统一任务标识栏（标识 + mode 标签 + 清空/退出）── */
.unified-bar {
  flex: none; display: flex; align-items: center; gap: 8px;
  margin: 8px 20px 0; padding: 6px 10px;
  border: 1px solid var(--border); border-radius: 10px;
  background: var(--bg-card);
}
.ub-badge {
  flex: none; display: inline-flex; align-items: center; gap: 6px;
  padding: 1px 10px; border-radius: 10px;
  background: var(--primary); color: #fff;
  font-size: 11px; font-weight: 600; line-height: 18px;
  transition: background 0.3s ease;
}
.ub-badge::before { content: ''; width: 6px; height: 6px; border-radius: 50%; background: rgba(255, 255, 255, 0.85); }
/* 编排中：圆点脉冲 + 徽标呼吸（与 .unified-exec-hint 同一语言） */
.ub-badge.running::before {
  animation: uehPulse 1.1s ease-in-out infinite;
  box-shadow: 0 0 0 0 rgba(255, 255, 255, 0.5);
}
.ub-badge.running { animation: uehPulse 1.1s ease-in-out infinite; }
/* 完成态：短促过渡到成功色 */
.ub-badge.done {
  background: var(--success);
}
.ub-badge.done::before { animation: none; }
.ub-mode {
  flex: none; font-size: 11px; color: var(--text-secondary);
  background: var(--bg-secondary); padding: 1px 8px; border-radius: 10px; line-height: 18px;
}
.ub-spacer { flex: 1; }
.ub-btn {
  flex: none; border: 1px solid var(--border); border-radius: 8px;
  background: var(--bg-card); color: var(--text-secondary);
  font-size: 11px; line-height: 18px; padding: 1px 10px; cursor: pointer;
  transition: all 0.15s ease;
}
.ub-btn:focus-visible,
.ueb-close:focus-visible,
.approval-btn:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
.ueb-close:focus-visible { border-radius: 4px; }
.ub-btn:hover:not(:disabled) { border-color: var(--primary); color: var(--primary); }
.ub-btn.exit:hover:not(:disabled) { border-color: var(--danger, #ef4444); color: var(--danger, #ef4444); }
.ub-btn:disabled { opacity: 0.4; cursor: not-allowed; }

/* ── 互联互通：统一任务执行中轻量提示（编排/子任务进行中）── */
.unified-exec-hint {
  flex: none; display: inline-flex; align-items: center; gap: 6px;
  align-self: center; margin: 10px auto 0;
  font-size: 12px; color: var(--text-tertiary);
  font-variant-numeric: tabular-nums;
}
.ueh-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--primary);
  animation: uehPulse 1.2s ease-in-out infinite;
}
@keyframes uehPulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.25; } }
@media (prefers-reduced-motion: reduce) { .ueh-dot { animation: none; } }
@media (prefers-reduced-motion: reduce) {
  .ub-badge.running,
  .ub-badge.running::before { animation: none; }
  .ub-badge { transition: none; }
}

/* ── 互联互通：统一任务消息列表（复用 MessageItem 渲染 user/assistant）── */
.unified-list { flex: 1; overflow-y: auto; padding: 12px 0 24px; scrollbar-width: thin; scrollbar-color: var(--text-disabled) transparent; }
.unified-empty { padding: 40px 20px; text-align: center; color: var(--text-muted); font-size: 13px; }
/* 知识库引用小标签（assistant 消息下方；kb_id 已知时展示“查看知识库”链接） */
.kb-hits-tag {
  display: flex; align-items: center;
  max-width: min(720px, 92%); margin: 2px auto 6px;
  padding: 2px 10px; border-radius: 10px;
  background: var(--primary-bg); color: var(--primary);
  font-size: 11px; line-height: 18px;
}
.kb-hits-text { flex: 1; min-width: 0; }
.kb-hits-link {
  flex: none; margin-left: auto; padding-left: 12px;
  color: var(--primary); font-weight: 600; white-space: nowrap;
  text-decoration: none;
}
.kb-hits-link:hover { text-decoration: underline; }
@media (max-width: 576px) {
  .unified-error-banner { margin: 6px 12px 0; }
  .unified-bar { margin: 6px 12px 0; }
  .kb-hits-tag { max-width: 88%; }
}
</style>
