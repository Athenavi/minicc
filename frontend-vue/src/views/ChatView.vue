<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import {
  Input,
  Button,
  Avatar,
  Popconfirm,
  Select,
  message,
} from 'ant-design-vue'
import {
  SendOutlined,
  PlusOutlined,
  DeleteOutlined,
  MenuOutlined,
} from '@ant-design/icons-vue'
import { api, createSSEConnection } from '../api'
import MarkdownIt from 'markdown-it'
import 'katex/dist/katex.min.css'
import texmath from 'markdown-it-texmath'
import katex from 'katex'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()

// ── Markdown 引擎 ──
const md = new MarkdownIt({ html: false, linkify: true, breaks: true })
md.use(texmath, { engine: katex, delimiters: 'dollars', katexOptions: { throwOnError: false, output: 'html' } })
md.renderer.rules.fence = (tokens: any[], idx: number) => {
  const token = tokens[idx]
  const lang = (token.info || '').trim().toLowerCase()
  const code = token.content
  const escaped = md.utils.escapeHtml(code)
  if (lang === 'mermaid') return `<div class="mermaid">${escaped}</div>`
  const safeLang = md.utils.escapeHtml(lang || 'code')
  const encoded = encodeURIComponent(code)
  return `<div class="code-block-wrapper"><div class="code-block-header"><span class="code-lang">${safeLang}</span><button class="code-copy-btn" data-code="${encoded}">复制</button></div><pre><code class="language-${safeLang}">${escaped}</code></pre></div>`
}

// ── 类型 ──
interface ToolCallEvent {
  id: string; name: string; arguments: string; result?: string
}
interface Message {
  id: string; role: 'user' | 'assistant' | 'system'
  content: string; displayedLength: number; timestamp: number
  toolCalls?: ToolCallEvent[]
}
interface Session {
  id: string; title: string; created_at: string; updated_at: string
}

// ── 状态 ──
const sessions = ref<Session[]>([])
const activeSessionId = ref('')
const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const sidebarCollapsed = ref(false)
const mobileSidebarOpen = ref(false)
let typewriterTimer: ReturnType<typeof setInterval> | null = null
let activeSSE: EventSource | null = null

function toggleSidebar() {
  if (window.innerWidth <= 768) {
    mobileSidebarOpen.value = !mobileSidebarOpen.value
  } else {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }
}

function formatRelativeTime(iso: string) {
  if (!iso) return ''
  const d = new Date(iso); const now = new Date()
  const diff = now.getTime() - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

function persistSessions() { localStorage.setItem('chat_sessions', JSON.stringify(sessions.value)) }

onMounted(async () => {
  await loadSessions()
  if (sessions.value.length > 0) {
    await switchSession(sessions.value[0].id)
  }
})
onUnmounted(() => {
  if (typewriterTimer) { clearInterval(typewriterTimer); typewriterTimer = null }
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
  let session: Session | null = null
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
  activeSessionId.value = id; messages.value = []; loading.value = true
  try {
    const res = await api.get(`/v1/conversations/${id}`)
    const data = res.data?.data || res.data
    if (data?.messages) { messages.value = data.messages.map((m: any) => ({ id: m.id, role: m.role, content: m.content, displayedLength: 0, timestamp: Date.parse(m.created_at) || Date.now() })) }
  } catch { /* fallback */ }
  finally { loading.value = false }
}

async function deleteSession(id: string) {
  sessions.value = sessions.value.filter(s => s.id !== id); persistSessions()
  if (activeSessionId.value === id) {
    activeSessionId.value = ''; messages.value = []
    if (sessions.value.length > 0) await switchSession(sessions.value[0].id)
  }
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') { e.preventDefault(); sendMessage() }
}

async function sendMessage() {
  const text = input.value.trim()
  if (!text || loading.value) return
  input.value = ''
  const userMsg: Message = { id: `msg_${Date.now()}`, role: 'user', content: text, displayedLength: 0, timestamp: Date.now() }
  messages.value.push(userMsg)
  loading.value = true
  try {
    const sessionId = activeSessionId.value || ''
    const body: any = { content: text, session_id: sessionId }
    if (mode.value === 'deep') body.llm_config = { deep_reasoning: true, max_turns: 10, temperature: 0.1 }
    if (activeSSE) { activeSSE.close(); activeSSE = null }
    // 先建立 SSE 连接订阅事件，再发送消息触发 AI 处理
    activeSSE = await createSSEConnection(sessionId || `session_${Date.now()}`, (data: any) => {
      if (data.type === 'text') {
        const last = messages.value[messages.value.length - 1]
        if (last?.role === 'assistant') { last.content += data.content }
        else { messages.value.push({ id: `msg_${Date.now()}`, role: 'assistant', content: data.content, displayedLength: 0, timestamp: Date.now() }) }
      }
      if (data.type === 'done') { loading.value = false; activeSSE = null }
      if (data.type === 'error') { message.error(data.message || '请求失败'); loading.value = false }
    }, () => { loading.value = false; activeSSE = null })
    await api.post('/submit', body)
    if (!sessionId) { activeSessionId.value = activeSSE.url?.split('session_id=')[1]?.split('&')[0] || '' }
  } catch (e: any) {
    message.error('发送失败: ' + (e.message || '网络错误'))
    loading.value = false
  }
}

function renderMarkdown(src: string) {
  try { return md.render(src) } catch { return md.utils.escapeHtml(src) }
}

function handleMsgClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const code = decodeURIComponent(btn.dataset.code || '')
  if (!code) return
  navigator.clipboard.writeText(code).then(() => { btn.textContent = '已复制'; setTimeout(() => { btn.textContent = '复制' }, 2000) }).catch(() => { /* clipboard not available */ })
}

function quickSend(text: string) { input.value = text; sendMessage() }

const modeOptions = [
  { label: '常规', value: 'normal' },
  { label: '深度推理', value: 'deep' },
]
const mode = ref('normal')
</script>

<template>
  <div class="chat-layout">
    <!-- 侧边栏 -->
    <div :class="['sidebar', { collapsed: sidebarCollapsed, 'mobile-open': mobileSidebarOpen }]">
      <div class="sidebar-header">
        <div class="sidebar-logo">
          <span class="logo-icon">&#9670;</span>
          <span v-if="!sidebarCollapsed" class="logo-text">MiniCC</span>
        </div>
        <Button v-if="!sidebarCollapsed" block type="dashed" @click="createSession">
          <template #icon><PlusOutlined /></template>
          新对话
        </Button>
      </div>
      <div class="sidebar-content">
        <div v-if="sessions.length === 0" class="session-empty">暂无对话记录</div>
        <div
          v-for="s in sessions"
          :key="s.id"
          :class="['session-item', { active: s.id === activeSessionId }]"
          @click="switchSession(s.id)"
        >
          <span class="session-icon">&#128172;</span>
          <div class="session-info">
            <span class="session-title">{{ s.title || '新对话' }}</span>
            <span class="session-time">{{ formatRelativeTime(s.updated_at || s.created_at) }}</span>
          </div>
          <Popconfirm title="确认删除此对话？" @confirm="deleteSession(s.id)" placement="left">
            <template #icon></template>
            <Button type="text" size="small" class="session-delete-btn" @click.stop>
              <template #icon><DeleteOutlined /></template>
            </Button>
          </Popconfirm>
        </div>
      </div>
      <div class="sidebar-footer">
        <Avatar :size="24" :style="{ backgroundColor: 'var(--primary)' }">
          {{ authStore.user?.name?.charAt(0) || 'U' }}
        </Avatar>
        <span v-if="!sidebarCollapsed" class="user-name">{{ authStore.user?.name || '用户' }}</span>
      </div>
    </div>
    <div v-if="mobileSidebarOpen" class="sidebar-overlay" @click="mobileSidebarOpen = false"></div>

    <!-- 主区域 -->
    <div class="chat-main">
      <div class="chat-header">
        <Button type="text" @click="toggleSidebar">
          <template #icon><MenuOutlined /></template>
        </Button>
        <div class="model-pill">
          <span class="model-icon">&#10022;</span>
          <span class="model-name">MiniCC 4.0</span>
          <span class="model-arrow">&#9662;</span>
        </div>
        <div style="margin-left: auto;">
          <Select
            v-model:value="mode"
            :options="modeOptions"
            size="small"
            style="width: 100px"
          />
        </div>
      </div>

      <div class="chat-messages" @click="handleMsgClick">
        <div v-if="messages.length === 0 && !loading" class="welcome">
          <div class="welcome-content">
            <h1 class="welcome-title">你好，有什么可以帮助你的？</h1>
            <div class="suggestion-grid">
              <div class="suggestion-card" @click="quickSend('写一段 Python 代码实现排序算法')">
                <div class="card-icon">&#128221;</div>
                <div class="card-title">代码生成</div>
                <div class="card-desc">写一段 Python 代码实现排序算法</div>
              </div>
              <div class="suggestion-card" @click="quickSend('帮我写一篇关于 AI 的短文')">
                <div class="card-icon">&#9998;</div>
                <div class="card-title">创意写作</div>
                <div class="card-desc">帮我写一篇关于 AI 的短文</div>
              </div>
              <div class="suggestion-card" @click="quickSend('分析这份数据的趋势')">
                <div class="card-icon">&#128202;</div>
                <div class="card-title">数据分析</div>
                <div class="card-desc">分析这份数据的趋势</div>
              </div>
              <div class="suggestion-card" @click="quickSend('帮我做一个项目计划')">
                <div class="card-icon">&#127919;</div>
                <div class="card-title">方案策划</div>
                <div class="card-desc">帮我做一个项目计划</div>
              </div>
            </div>
          </div>
        </div>

        <div v-for="msg in messages" :key="msg.id" :class="['msg-row', msg.role]">
          <div v-if="msg.role === 'assistant'" class="msg-avatar ai">AI</div>
          <div class="msg-content">
            <div class="msg-text">
              <div v-if="msg.role === 'assistant'" class="rendered-content" v-html="renderMarkdown(msg.content)"></div>
              <div v-else class="rendered-content user-content" v-html="renderMarkdown(msg.content)"></div>
            </div>
          </div>
          <div v-if="msg.role === 'user'" class="msg-avatar user">U</div>
        </div>

        <div v-if="loading" class="loading-indicator">
          <span class="loading-dot"></span>
          <span class="loading-dot"></span>
          <span class="loading-dot"></span>
        </div>
      </div>

      <div class="input-area">
        <div class="input-wrapper">
          <Input.TextArea
            v-model:value="input"
            :rows="1"
            :auto-size="{ minRows: 1, maxRows: 5 }"
            placeholder="发送消息..."
            class="input-field"
            @keydown="handleKeydown"
          />
          <Button
            type="primary"
            shape="circle"
            :disabled="!input.trim() || loading"
            @click="sendMessage"
          >
            <template #icon><SendOutlined /></template>
          </Button>
        </div>
        <div class="input-hint">Cmd + Enter 发送</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-layout { display: flex; height: 100%; background: var(--bg-primary, #0A0A0B); }

.sidebar { width: 260px; flex-shrink: 0; display: flex; flex-direction: column; background: var(--bg-secondary, #111115); border-right: 1px solid var(--border, #1E1E24); transition: transform 0.25s ease; }
.sidebar-header { padding: 16px; display: flex; flex-direction: column; gap: 12px; border-bottom: 1px solid var(--border, #1E1E24); }
.sidebar-logo { display: flex; align-items: center; gap: 8px; }
.logo-icon { font-size: 20px; color: var(--primary); }
.logo-text { font-size: 16px; font-weight: 600; letter-spacing: -0.3px; color: var(--text-primary, #EEEEF0); }
.sidebar-content { flex: 1; overflow-y: auto; padding: 8px; }
.session-empty { padding: 24px 8px; text-align: center; color: var(--text-muted, #606068); font-size: 13px; }
.session-item { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: var(--radius-sm, 6px); cursor: pointer; transition: all 0.15s; margin-bottom: 2px; }
.session-item:hover { background: var(--bg-hover, #1A1A22); }
.session-item.active { background: var(--bg-hover, #1A1A22); }
.session-icon { font-size: 14px; flex-shrink: 0; }
.session-info { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; }
.session-title { font-size: 13px; color: var(--text-secondary, #A0A0A8); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.session-time { font-size: 11px; color: var(--text-muted, #606068); flex-shrink: 0; }
.session-delete-btn { opacity: 0; flex-shrink: 0; color: var(--text-muted, #606068); }
.session-item:hover .session-delete-btn { opacity: 1; }
.sidebar-footer { padding: 12px 16px; display: flex; align-items: center; gap: 8px; border-top: 1px solid var(--border, #1E1E24); }
.user-name { font-size: 13px; color: var(--text-secondary, #A0A0A8); }
.sidebar-overlay { display: none; }

.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.chat-header { height: 48px; display: flex; align-items: center; padding: 0 20px; gap: 12px; border-bottom: 1px solid var(--border, #1E1E24); flex-shrink: 0; }
.model-pill { display: flex; align-items: center; gap: 6px; padding: 4px 12px; background: var(--bg-secondary, #111115); border: 1px solid var(--border, #1E1E24); border-radius: var(--radius-full, 9999px); cursor: pointer; }
.model-pill:hover { border-color: var(--text-muted, #606068); }
.model-icon { font-size: 14px; color: var(--primary); }
.model-name { font-size: 13px; color: var(--text-primary, #EEEEF0); font-weight: 500; }
.model-arrow { font-size: 10px; color: var(--text-muted, #606068); }

.chat-messages { flex: 1; overflow-y: auto; }
.welcome { display: flex; justify-content: center; align-items: center; height: 100%; padding: 0 24px; }
.welcome-content { width: 520px; text-align: center; }
.welcome-title { font-family: 'Inter Tight', var(--font-sans); font-size: 28px; font-weight: 600; letter-spacing: -0.5px; color: var(--text-primary, #EEEEF0); margin-bottom: 32px; line-height: 1.3; }
.suggestion-grid { display: flex; flex-wrap: wrap; gap: 12px; justify-content: center; }
.suggestion-card { width: 248px; padding: 14px; border-radius: var(--radius-lg, 10px); border: 1px solid var(--border-card, rgba(28, 28, 36, 0.8)); background: transparent; cursor: pointer; transition: all 0.2s; text-align: left; }
.suggestion-card:hover { background: var(--bg-hover, #1A1A22); border-color: var(--text-muted, #606068); }
.card-icon { font-size: 20px; margin-bottom: 8px; }
.card-title { font-size: 14px; font-weight: 600; color: var(--text-primary, #EEEEF0); margin-bottom: 4px; }
.card-desc { font-size: 12px; color: var(--text-tertiary, #808090); line-height: 1.4; }

.msg-row { display: flex; gap: 12px; padding: 16px 24px; max-width: 820px; margin: 0 auto; width: 100%; }
.msg-row.user { flex-direction: row-reverse; }
.msg-avatar { width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 600; flex-shrink: 0; }
.msg-avatar.ai { background: var(--bg-hover, #1A1A22); color: var(--primary); border: 1px solid var(--border, #1E1E24); }
.msg-avatar.user { background: var(--primary); color: white; }
.msg-content { flex: 1; min-width: 0; }
.msg-text { font-size: 14px; line-height: 1.7; color: var(--text-primary, #EEEEF0); }
.msg-row.assistant .msg-text { padding: 2px 0; }
.msg-row.user .msg-text { display: inline-block; padding: 10px 16px; background: var(--primary); color: white; border-radius: 16px; border-bottom-right-radius: 4px; max-width: 100%; }

.rendered-content :deep(p) { margin: 6px 0; }
.rendered-content :deep(p:first-child) { margin-top: 0; }
.rendered-content :deep(ul), .rendered-content :deep(ol) { padding-left: 24px; margin: 6px 0; }
.rendered-content :deep(li) { margin: 2px 0; }
.rendered-content :deep(code) { font-family: var(--font-mono); font-size: 0.88em; background: var(--bg-hover, #1A1A22); padding: 2px 6px; border-radius: 4px; }
.rendered-content :deep(pre) { margin: 8px 0; padding: 14px 16px; background: #0d0d12; border-radius: 8px; overflow-x: auto; border: 1px solid var(--border, #1E1E24); }
.rendered-content :deep(pre code) { background: none; padding: 0; font-size: 0.85em; color: #cdd6f4; }
.rendered-content :deep(a) { color: var(--primary-light, #8B7CF7); text-decoration: none; }
.rendered-content :deep(a:hover) { text-decoration: underline; }
.rendered-content :deep(blockquote) { margin: 8px 0; padding: 6px 12px; border-left: 3px solid var(--primary); background: var(--bg-hover, #1A1A22); border-radius: 4px; }
.rendered-content :deep(table) { border-collapse: collapse; margin: 8px 0; width: 100%; }
.rendered-content :deep(th), .rendered-content :deep(td) { border: 1px solid var(--border, #1E1E24); padding: 8px 12px; text-align: left; }
.rendered-content :deep(th) { background: var(--bg-hover, #1A1A22); font-weight: 600; }
.rendered-content :deep(img) { max-width: 100%; border-radius: 8px; }
.user-content :deep(code) { background: rgba(255,255,255,0.15); }

.loading-indicator { display: flex; gap: 4px; padding: 16px 24px; max-width: 820px; margin: 0 auto; }
.loading-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--primary); animation: dotBounce 1.4s infinite ease-in-out both; }
.loading-dot:nth-child(1) { animation-delay: -0.32s; }
.loading-dot:nth-child(2) { animation-delay: -0.16s; }
.loading-dot:nth-child(3) { animation-delay: 0s; }
@keyframes dotBounce { 0%, 80%, 100% { transform: scale(0.6); opacity: 0.4; } 40% { transform: scale(1); opacity: 1; } }

.input-area { padding: 12px 24px 16px; border-top: 1px solid var(--border, #1E1E24); }
.input-wrapper { display: flex; align-items: flex-end; gap: 8px; max-width: 820px; margin: 0 auto; background: var(--bg-input, #111115); border: 1px solid var(--border, #1E1E24); border-radius: var(--radius-xl, 12px); padding: 8px 8px 8px 16px; transition: border-color 0.2s; }
.input-wrapper:focus-within { border-color: var(--primary); box-shadow: 0 0 20px rgba(108, 92, 231, 0.15); }
.input-field { flex: 1; background: transparent !important; }
.input-field :deep(textarea) { color: var(--text-primary, #EEEEF0) !important; }
.input-hint { max-width: 820px; margin: 6px auto 0; text-align: right; font-size: 11px; color: var(--text-disabled, #505058); }

@media (max-width: 768px) {
  .sidebar { position: fixed; top: 0; left: 0; bottom: 0; z-index: 100; transform: translateX(-100%); }
  .sidebar.mobile-open { transform: translateX(0); }
  .sidebar-overlay { display: block; position: fixed; inset: 0; z-index: 99; background: rgba(0,0,0,0.5); }
  .welcome-content { width: 100%; }
  .suggestion-card { width: 100%; }
  .msg-row { padding: 12px 16px; }
  .input-area { padding: 8px 12px 12px; }
}
</style>
