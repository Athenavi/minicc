<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { Button, message } from 'ant-design-vue'
import { MenuOutlined } from '@ant-design/icons-vue'
import { api, createSSEConnection } from '../api'
import { useAuthStore } from '../stores/auth'
import ChatSidebar from '../components/chat/ChatSidebar.vue'
import MessageList from '../components/chat/MessageList.vue'
import ChatEmptyHero from '../components/chat/ChatEmptyHero.vue'
import ChatInput from '../components/chat/ChatInput.vue'
import { submitApproval } from '../api'
import ChatTrajectory from '../components/chat/ChatTrajectory.vue'
import { HistoryOutlined } from '@ant-design/icons-vue'
import { splitThinking, stripUserInputTag, throttleRaf, formatClock } from '../components/chat/chat-types'
import type { ChatItem, ChatSession } from '../components/chat/chat-types'

const authStore = useAuthStore()

// ── 会话状态 ──
const sessions = ref<ChatSession[]>([])
const activeSessionId = ref('')
const loading = ref(false)
const sidebarCollapsed = ref(false)
const mobileSidebarOpen = ref(false)
const items = ref<ChatItem[]>([])
let activeSSE: EventSource | null = null

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

// 轨迹面板（右侧历史跳转）
const trajectoryOpen = ref(false)
const trajectoryFocus = ref<number | null>(null)
const trajectoryToken = ref(0)

function onTrajectoryFocus(index: number) {
  trajectoryFocus.value = index
  trajectoryToken.value += 1
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
  await loadSessions()
  if (sessions.value.length > 0) {
    await switchSession(sessions.value[0].id)
  }
})

onUnmounted(() => {
  stopTurnTimer()
  if (activeSSE) { activeSSE.close(); activeSSE = null }
})

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
}

async function createSession() {
  let session: ChatSession | null = null
  try {
    const res = await api.post('/v1/conversations', { title: '新对话' })
    const data = res.data?.data || res.data
    if (data?.id) session = { id: data.id, title: data.title || '新对话', created_at: data.created_at, updated_at: data.updated_at }
  } catch { /* fallback */ }
  if (!session) {
    const id = `session_${Date.now()}_${Math.random().toString(36).slice(2)}`
    session = { id, title: '新对话', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
  }
  sessions.value.unshift(session); persistSessions()
  try { await switchSession(session.id) } catch { /* ignore */ }
}

async function switchSession(id: string) {
  if (id === activeSessionId.value) return
  activeSessionId.value = id; items.value = []; loading.value = true
  hasMore.value = false; earliestCursor.value = ''; loadingEarlier.value = false
  try {
    const res = await api.get(`/v1/conversations/${id}?limit=${HISTORY_PAGE_SIZE}`)
    const data = res.data?.data || res.data
    if (data?.messages) {
      items.value = mergeHistory(data.messages, data.tool_calls || [])
      earliestCursor.value = data.cursor || ''
      hasMore.value = !!data.has_more
    }
  } catch { /* fallback */ } finally {
    loading.value = false
  }
}

// P 性能：cursor 分页 — 触顶加载更早的消息（首屏只加载最近 HISTORY_PAGE_SIZE 条）
const HISTORY_PAGE_SIZE = 50
const hasMore = ref(false)
const earliestCursor = ref('')
const loadingEarlier = ref(false)

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
        if (body) items.push({ kind: 'text', role: 'assistant', content: body, time: clock, id: m.id })
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

function toggleSidebar() {
  if (window.innerWidth <= 768) mobileSidebarOpen.value = !mobileSidebarOpen.value
  else sidebarCollapsed.value = !sidebarCollapsed.value
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

function appendUserText(text: string) {
  items.value.push({ kind: 'text', role: 'user', content: text, id: genItemId() })
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

const scheduleTextFlush = throttleRaf(() => { /* rAF 已合并，Vue 响应式自动批量 */ })

function onSSEMessage(raw: any) {
  const type = raw?.type
  const d = raw?.data || {}
  if (type === 'text') {
    const text = d?.content ?? raw?.content ?? ''
    if (!text) return
    onTextChunk(text)
    scheduleTextFlush()
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

async function sendMessage(text: string) {
  loading.value = true
  startTurnTimer()
  resetStreamState()
  connectionLost.value = false
  appendUserText(text)
  const sessionId = activeSessionId.value || `session_${Date.now()}_${Math.random().toString(36).slice(2)}`
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
      },
    )
    const body: any = { content: text, session_id: sessionId, llm_config: { mode: mode.value } }
    await api.post('/submit', body)
    activeSessionId.value = sessionId
  } catch (e: any) {
    loading.value = false
    stopTurnTimer()
    flushStreamingFlags()
    message.error('发送失败: ' + (e.message || '网络错误'))
  }
}

function stopGeneration() {
  stopTurnTimer()
  if (activeSSE) { activeSSE.close(); activeSSE = null }
  loading.value = false
  flushStreamingFlags()
}
</script>

<template>
  <div class="chat-layout">
    <ChatSidebar
      :class="{ collapsed: sidebarCollapsed || !mobileSidebarOpen }"
      :sessions="sessions"
      :active-session-id="activeSessionId"
      :collapsed="false"
      :user-name="authStore.user?.name"
      @create="createSession"
      @switch="switchSession"
      @delete="deleteSession"
      @close="mobileSidebarOpen = false"
    />
    <div v-if="mobileSidebarOpen" class="sidebar-overlay" @click="mobileSidebarOpen = false"></div>

    <div class="chat-main">
      <div class="chat-header">
        <Button type="text" size="small" @click="toggleSidebar">
          <template #icon><MenuOutlined /></template>
        </Button>
        <span class="header-title">MiniCC</span>
        <span class="header-sub">{{ modeOptions.find(o => o.value === mode)?.label || '常规' }}</span>
        <Button
          type="text" size="small" class="trajectory-toggle"
          :class="{ active: trajectoryOpen }"
          :title="trajectoryOpen ? '收起轨迹' : '查看历史提问'"
          @click="trajectoryOpen = !trajectoryOpen"
        >
          <template #icon><HistoryOutlined /></template>
        </Button>
      </div>

      <div v-if="connectionLost" class="connection-banner">连接已断开，请重试发送消息</div>
      <div class="chat-body">
        <ChatEmptyHero v-if="items.length === 0 && !loading" @suggest="sendMessage" />
        <template v-else>
          <div v-if="loading" class="turn-status">思考中<template v-if="turnElapsed >= 2">&nbsp;·&nbsp;{{ turnElapsed }}s</template></div>
          <MessageList
            :items="items"
            :loading="loading"
            :focus-index="trajectoryFocus"
            :focus-token="trajectoryToken"
            :has-more="hasMore"
            :loading-earlier="loadingEarlier"
            @load-earlier="loadEarlier"
          />
        </template>

        <ChatTrajectory
          :items="items"
          :selected-index="trajectoryFocus"
          :open="trajectoryOpen"
          @focus="onTrajectoryFocus"
          @close="trajectoryOpen = false"
        />
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
        @send="sendMessage"
        @stop="stopGeneration"
        @update:mode="(m: string) => (mode = m)"
      />
    </div>
  </div>
</template>

<style scoped>
.chat-layout { display: flex; height: 100%; background: var(--bg-page); }
.chat-sidebar { display: flex; }
.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.chat-body { position: relative; flex: 1; display: flex; flex-direction: column; min-height: 0; }
.trajectory-toggle.active { color: var(--primary); background: var(--primary-bg); }
/* S 安全修复：工具确认卡片 */
.approval-zone { padding: 0 20px 8px; display: flex; flex-direction: column; gap: 8px; }
.approval-card { background: var(--bg-card); border: 1px solid var(--border); border-left: 3px solid var(--primary); border-radius: 10px; padding: 10px 14px; }
.approval-info { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.approval-tag { font-size: 11px; color: var(--primary); background: var(--primary-bg); padding: 2px 8px; border-radius: 10px; }
.approval-name { font-weight: 600; font-size: 13px; color: var(--text-primary); }
.approval-args { font-family: var(--font-mono); font-size: 12px; color: var(--text-muted); word-break: break-all; margin-bottom: 8px; }
.approval-actions { display: flex; gap: 8px; }
.approval-btn { border: none; border-radius: 8px; padding: 6px 16px; font-size: 13px; cursor: pointer; }
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
.chat-header { height: 44px; display: flex; align-items: center; gap: 10px; padding: 0 16px; border-bottom: 1px solid var(--border); flex-shrink: 0; }
.header-title { font-size: 14px; font-weight: 600; color: var(--text-primary); }
.header-sub { font-size: 12px; color: var(--text-muted); background: var(--bg-secondary); padding: 2px 8px; border-radius: var(--radius-full); }
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
.sidebar-overlay { display: none; }
@media (max-width: 768px) {
  .chat-sidebar { position: fixed; top: 0; left: 0; bottom: 0; z-index: 100; transform: translateX(-100%); transition: transform 0.25s ease; }
  .chat-sidebar.collapsed { transform: translateX(-100%); }
  .chat-sidebar:not(.collapsed) { transform: translateX(0); }
  .sidebar-overlay { display: block; position: fixed; inset: 0; z-index: 99; background: rgba(10, 10, 12, 0.45); }
}
</style>
