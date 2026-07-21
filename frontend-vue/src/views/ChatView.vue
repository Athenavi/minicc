<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { NInput, NButton, NScrollbar, NAvatar, NSpin, NEmpty, NIcon, NTooltip, NPopconfirm, NSelect, NUpload, NImage, NTag, useMessage } from 'naive-ui'
import { SendOutline, AddOutline, TrashOutline, ChatbubbleEllipsesOutline } from '@vicons/ionicons5'
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
  if (lang === 'mermaid') return `<div class="mermaid">${code}</div>`
  const langLabel = lang || 'code'
  const encoded = encodeURIComponent(code)
  return `<div class="code-block-wrapper"><div class="code-block-header"><span class="code-lang">${langLabel}</span><button class="code-copy-btn" data-code="${encoded}">复制</button></div><pre><code class="language-${langLabel}">${escaped}</code></pre></div>`
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
const message = useMessage()
const sessions = ref<Session[]>([])
const activeSessionId = ref('')
const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const streaming = ref(false)
const sidebarCollapsed = ref(false)
const mobileSidebarOpen = ref(false)
let typewriterTimer: ReturnType<typeof setInterval> | null = null

function toggleSidebar() {
  if (window.innerWidth <= 768) {
    mobileSidebarOpen.value = !mobileSidebarOpen.value
  } else {
    sidebarCollapsed.value = !sidebarCollapsed.value
  }
}

function getActiveTitle() { return sessions.value.find(s => s.id === activeSessionId.value)?.title || '新对话' }
function formatTime(ts: number) { return new Date(ts).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }
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
    const events = await createSSEConnection(sessionId || `session_${Date.now()}`, (data: any) => {
      if (data.type === 'text') {
        const last = messages.value[messages.value.length - 1]
        if (last?.role === 'assistant') { last.content += data.content }
        else { messages.value.push({ id: `msg_${Date.now()}`, role: 'assistant', content: data.content, displayedLength: 0, timestamp: Date.now() }) }
      }
      if (data.type === 'done') { loading.value = false }
      if (data.type === 'error') { message.error(data.message || '请求失败'); loading.value = false }
    }, () => { loading.value = false })
    if (!sessionId) { activeSessionId.value = events.url?.split('session_id=')[1]?.split('&')[0] || '' }
  } catch (e: any) {
    message.error('发送失败: ' + (e.message || '网络错误'))
    loading.value = false
  }
}

function renderMarkdown(src: string) {
  try { return md.render(src) } catch { return src }
}

function handleMsgClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const code = decodeURIComponent(btn.dataset.code || '')
  if (!code) return
  navigator.clipboard.writeText(code).then(() => { btn.textContent = '已复制'; setTimeout(() => { btn.textContent = '复制' }, 2000) }).catch(() => { /* clipboard not available */ })
}

function quickSend(text: string) { input.value = text; sendMessage() }

function autoResize(e: Event) {
  const el = e.target as HTMLTextAreaElement
  el.style.height = 'auto'
  el.style.height = el.scrollHeight + 'px'
}

const modeOptions = [
  { label: '常规', value: 'normal' },
  { label: '深度推理', value: 'deep' },
]
const mode = ref('normal')
</script>

<template>
  <div class="chat-layout">
    <div :class="['sidebar', { collapsed: sidebarCollapsed, 'mobile-open': mobileSidebarOpen }]">
      <div class="sidebar-header">
        <div class="sidebar-logo">
          <span class="logo-icon">&#9670;</span>
          <span v-if="!sidebarCollapsed" class="logo-text">MiniCC</span>
        </div>
        <button v-if="!sidebarCollapsed" class="new-chat-btn" @click="createSession">+ 新对话</button>
      </div>
      <div class="sidebar-content">
        <div v-if="sessions.length === 0" class="session-empty">暂无对话记录</div>
        <div v-for="s in sessions" :key="s.id" :class="['session-item', { active: s.id === activeSessionId }]" @click="switchSession(s.id)">
          <span class="session-icon">&#128172;</span>
          <div class="session-info">
            <span class="session-title">{{ s.title || '新对话' }}</span>
            <span class="session-time">{{ formatRelativeTime(s.updated_at || s.created_at) }}</span>
          </div>
          <button class="session-delete" @click.stop="deleteSession(s.id)" title="删除">&#10005;</button>
        </div>
      </div>
      <div class="sidebar-footer">
        <span class="user-avatar">{{ authStore.user?.name?.charAt(0) || 'U' }}</span>
        <span v-if="!sidebarCollapsed" class="user-name">{{ authStore.user?.name || '用户' }}</span>
      </div>
    </div>
    <div v-if="mobileSidebarOpen" class="sidebar-overlay" @click="mobileSidebarOpen = false"></div>

    <div class="chat-main">
      <div class="chat-header">
        <button class="sidebar-toggle" @click="toggleSidebar">&#9776;</button>
        <div class="model-pill">
          <span class="model-icon">&#10022;</span>
          <span class="model-name">MiniCC 4.0</span>
          <span class="model-arrow">&#9662;</span>
        </div>
      </div>

      <div class="chat-messages">
        <div v-if="messages.length === 0" class="welcome">
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
          <textarea ref="textareaRef" v-model="input" class="input-field" placeholder="发送消息..." rows="1" @keydown="handleKeydown" @input="autoResize"></textarea>
          <button class="send-btn" :disabled="!input.trim() || loading" @click="sendMessage">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"></line>
              <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
            </svg>
          </button>
        </div>
        <div class="input-hint">Cmd + Enter 发送</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-layout { display: flex; height: 100vh; background: var(--bg-primary); }

.sidebar { width: 260px; flex-shrink: 0; display: flex; flex-direction: column; background: var(--bg-secondary); border-right: 1px solid var(--border); transition: transform 0.25s ease; }
.sidebar-header { padding: 16px; display: flex; flex-direction: column; gap: 12px; border-bottom: 1px solid var(--border); }
.sidebar-logo { display: flex; align-items: center; gap: 8px; }
.logo-icon { font-size: 20px; color: var(--primary); }
.logo-text { font-size: 16px; font-weight: 600; letter-spacing: -0.3px; color: var(--text-primary); }
.new-chat-btn { height: 34px; padding: 0 14px; border: 1px solid var(--border); border-radius: var(--radius-md); background: transparent; color: var(--text-secondary); font-size: 13px; cursor: pointer; transition: all 0.15s; text-align: left; }
.new-chat-btn:hover { background: var(--bg-hover); color: var(--text-primary); border-color: var(--text-muted); }
.sidebar-content { flex: 1; overflow-y: auto; padding: 8px; }
.session-empty { padding: 24px 8px; text-align: center; color: var(--text-muted); font-size: 13px; }
.session-item { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: var(--radius-sm); cursor: pointer; transition: all 0.15s; margin-bottom: 2px; height: 36px; }
.session-item:hover { background: var(--bg-hover); }
.session-item.active { background: var(--bg-hover); }
.session-item.active .session-title { color: var(--text-primary); font-weight: 500; }
.session-icon { font-size: 14px; flex-shrink: 0; }
.session-info { flex: 1; min-width: 0; display: flex; align-items: center; gap: 8px; }
.session-title { font-size: 13px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.session-time { font-size: 11px; color: var(--text-muted); flex-shrink: 0; }
.session-delete { opacity: 0; border: none; background: none; color: var(--text-muted); font-size: 12px; cursor: pointer; padding: 2px; border-radius: 3px; flex-shrink: 0; }
.session-item:hover .session-delete { opacity: 1; }
.session-delete:hover { color: var(--text-primary); }
.sidebar-footer { padding: 12px 16px; display: flex; align-items: center; gap: 8px; border-top: 1px solid var(--border); }
.user-avatar { width: 28px; height: 28px; border-radius: 50%; background: var(--primary); color: white; font-size: 12px; font-weight: 600; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
.user-name { font-size: 13px; color: var(--text-secondary); }
.sidebar-overlay { display: none; }

.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.chat-header { height: 48px; display: flex; align-items: center; padding: 0 20px; gap: 12px; border-bottom: 1px solid var(--border); flex-shrink: 0; }
.sidebar-toggle { width: 28px; height: 28px; border: none; background: transparent; color: var(--text-muted); font-size: 16px; cursor: pointer; border-radius: var(--radius-sm); display: flex; align-items: center; justify-content: center; }
.sidebar-toggle:hover { background: var(--bg-hover); color: var(--text-primary); }
.model-pill { display: flex; align-items: center; gap: 6px; padding: 4px 12px; background: var(--bg-secondary); border: 1px solid var(--border); border-radius: var(--radius-full); cursor: pointer; }
.model-pill:hover { border-color: var(--text-muted); }
.model-icon { font-size: 14px; color: var(--primary); }
.model-name { font-size: 13px; color: var(--text-primary); font-weight: 500; }
.model-arrow { font-size: 10px; color: var(--text-muted); }

.chat-messages { flex: 1; overflow-y: auto; }
.welcome { display: flex; justify-content: center; align-items: center; height: 100%; padding: 0 24px; }
.welcome-content { width: 520px; text-align: center; }
.welcome-title { font-family: 'Inter Tight', var(--font-sans); font-size: 28px; font-weight: 600; letter-spacing: -0.5px; color: var(--text-primary); margin-bottom: 32px; line-height: 1.3; }
.suggestion-grid { display: flex; flex-wrap: wrap; gap: 12px; justify-content: center; }
.suggestion-card { width: 248px; padding: 14px; border-radius: var(--radius-lg); border: 1px solid var(--border-card); background: transparent; cursor: pointer; transition: all 0.2s; text-align: left; }
.suggestion-card:hover { background: var(--bg-hover); border-color: var(--text-muted); }
.card-icon { font-size: 20px; margin-bottom: 8px; }
.card-title { font-size: 14px; font-weight: 600; color: var(--text-primary); margin-bottom: 4px; }
.card-desc { font-size: 12px; color: var(--text-tertiary); line-height: 1.4; }

.msg-row { display: flex; gap: 12px; padding: 16px 24px; max-width: 820px; margin: 0 auto; width: 100%; }
.msg-row.user { flex-direction: row-reverse; }
.msg-avatar { width: 28px; height: 28px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 600; flex-shrink: 0; }
.msg-avatar.ai { background: var(--bg-hover); color: var(--primary); border: 1px solid var(--border); }
.msg-avatar.user { background: var(--primary); color: white; }
.msg-content { flex: 1; min-width: 0; }
.msg-text { font-size: 14px; line-height: 1.7; color: var(--text-primary); }
.msg-row.assistant .msg-text { padding: 2px 0; }
.msg-row.user .msg-text { display: inline-block; padding: 10px 16px; background: var(--primary); color: white; border-radius: 16px; border-bottom-right-radius: 4px; max-width: 100%; }

.rendered-content :deep(p) { margin: 6px 0; }
.rendered-content :deep(p:first-child) { margin-top: 0; }
.rendered-content :deep(ul), .rendered-content :deep(ol) { padding-left: 24px; margin: 6px 0; }
.rendered-content :deep(li) { margin: 2px 0; }
.rendered-content :deep(code) { font-family: var(--font-mono); font-size: 0.88em; background: var(--bg-hover); padding: 2px 6px; border-radius: 4px; }
.rendered-content :deep(pre) { margin: 8px 0; padding: 14px 16px; background: #0d0d12; border-radius: 8px; overflow-x: auto; border: 1px solid var(--border); }
.rendered-content :deep(pre code) { background: none; padding: 0; font-size: 0.85em; color: #cdd6f4; }
.rendered-content :deep(a) { color: var(--primary-light); text-decoration: none; }
.rendered-content :deep(a:hover) { text-decoration: underline; }
.rendered-content :deep(blockquote) { margin: 8px 0; padding: 6px 12px; border-left: 3px solid var(--primary); background: var(--bg-hover); border-radius: 4px; }
.rendered-content :deep(table) { border-collapse: collapse; margin: 8px 0; width: 100%; }
.rendered-content :deep(th), .rendered-content :deep(td) { border: 1px solid var(--border); padding: 8px 12px; text-align: left; }
.rendered-content :deep(th) { background: var(--bg-hover); font-weight: 600; }
.rendered-content :deep(img) { max-width: 100%; border-radius: 8px; }
.user-content :deep(code) { background: rgba(255,255,255,0.15); }

.loading-indicator { display: flex; gap: 4px; padding: 16px 24px; max-width: 820px; margin: 0 auto; }
.loading-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--primary); animation: dotBounce 1.4s infinite ease-in-out both; }
.loading-dot:nth-child(1) { animation-delay: -0.32s; }
.loading-dot:nth-child(2) { animation-delay: -0.16s; }
.loading-dot:nth-child(3) { animation-delay: 0s; }
@keyframes dotBounce { 0%, 80%, 100% { transform: scale(0.6); opacity: 0.4; } 40% { transform: scale(1); opacity: 1; } }

.input-area { padding: 12px 24px 16px; border-top: 1px solid var(--border); }
.input-wrapper { display: flex; align-items: flex-end; gap: 8px; max-width: 820px; margin: 0 auto; background: var(--bg-input); border: 1px solid var(--border); border-radius: var(--radius-xl); padding: 8px 8px 8px 16px; transition: border-color 0.2s; }
.input-wrapper:focus-within { border-color: var(--primary); box-shadow: var(--shadow-glow); }
.input-field { flex: 1; border: none; background: transparent; color: var(--text-primary); font-family: var(--font-sans); font-size: 14px; line-height: 1.5; resize: none; outline: none; padding: 4px 0; max-height: 120px; }
.input-field::placeholder { color: var(--text-muted); }
.send-btn { flex-shrink: 0; width: 36px; height: 36px; border-radius: 50%; border: none; background: var(--primary); color: white; cursor: pointer; display: flex; align-items: center; justify-content: center; transition: all 0.15s; }
.send-btn:hover:not(:disabled) { background: var(--primary-light); box-shadow: var(--shadow-glow); }
.send-btn:disabled { opacity: 0.3; cursor: not-allowed; }
.input-hint { max-width: 820px; margin: 6px auto 0; text-align: right; font-size: 11px; color: var(--text-disabled); }

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
