<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { NInput, NButton, NScrollbar, NAvatar, NSpin, NEmpty, NIcon, NTooltip, NPopconfirm, NSelect, NUpload, NImage } from 'naive-ui'
import { SendOutline, AddOutline, TrashOutline, ChatbubbleEllipsesOutline } from '@vicons/ionicons5'
import { api, createSSEConnection } from '../api'
import MarkdownIt from 'markdown-it'
import 'katex/dist/katex.min.css'
import texmath from 'markdown-it-texmath'
import katex from 'katex'

// ── Markdown 引擎 ──
const md = new MarkdownIt({ html: false, linkify: true, breaks: true })
md.use(texmath, { engine: katex, delimiters: 'dollars', katexOptions: { throwOnError: false, output: 'html' } })
md.renderer.rules.fence = (tokens, idx) => {
  const token = [redacted]
  const lang = (token.info || '').trim().toLowerCase()
  const code = token.content
  const escaped = md.utils.escapeHtml(code)
  if (lang === 'mermaid') return `<div class="mermaid">${code}</div>`
  const langLabel = lang || 'code'
  const encoded = encodeURIComponent(code)
  return `<div class="code-block-wrapper"><div class="code-block-header"><span class="code-lang">${langLabel}</span><button class="code-copy-btn" data-code="${encoded}">复制</button></div><pre><code class="language-${langLabel}">${escaped}</code></pre></div>`
}

// ── 类型 ──
interface Message {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  displayedLength: number
  timestamp: number
}

interface Session {
  id: string
  title: string
  created_at: string
  updated_at: string
}

// ── 状态 ──
const sessions = ref<Session[]>([])
const activeSessionId = ref('')
const messages = ref<Message[]>([])
const input = ref('')
const loading = ref(false)
const streaming = ref(false)
const scrollRef = ref<InstanceType<typeof NScrollbar> | null>(null)
const sidebarCollapsed = ref(false)
let typewriterTimer: ReturnType<typeof setInterval> | null = null

// ── 功能选项 ──
const modeOptions = [
  { label: '常规', value: 'normal' },
  { label: '深度推理', value: 'deep' },
]
const mode = ref('normal')
const uploadedFiles = ref<{ id: string; name: string; url: string; type: string }[]>([])
const uploading = ref(false)

async function handleFileUpload({ file }: { file: File }) {
  if (!file) return
  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const { data } = await api.post('/v1/media/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    if (data?.success && data?.data) {
      uploadedFiles.value.push({
        id: data.data.id || '',
        name: file.name,
        url: data.data.url || data.data.path || '',
        type: file.type,
      })
    }
  } catch (e: any) {
    console.error('Upload failed:', e)
    window.$message?.error?.('文件上传失败: ' + (e.response?.data?.error || e.message))
  } finally {
    uploading.value = false
  }
}

function removeFile(index: number) {
  uploadedFiles.value.splice(index, 1)
}

// ── 初始化 ──
onMounted(async () => {
  await loadSessions()
  if (sessions.value.length > 0) {
    await switchSession(sessions.value[0].id)
  }
})

onUnmounted(() => {
  if (typewriterTimer) { clearInterval(typewriterTimer); typewriterTimer = null }
})

// ── 会话管理 ──
async function loadSessions() {
  try {
    const res = await api.get('/v1/conversations')
    const apiSessions = res.data?.data || res.data || []
    if (apiSessions.length > 0) {
      sessions.value = apiSessions
      persistSessions() // 同步到 localStorage
    } else {
      // API 返回空（如未登录），回退到 localStorage
      const raw = localStorage.getItem('chat_sessions')
      sessions.value = raw ? JSON.parse(raw) : []
    }
  } catch {
    // 无后端时从 localStorage 恢复
    const raw = localStorage.getItem('chat_sessions')
    if (raw) sessions.value = JSON.parse(raw)
  }
}

async function createSession() {
  let session: Session | null = null
  try {
    const res = await api.post('/v1/conversations', { title: '新对话' })
    const data = res.data?.data || res.data
    if (data?.id) {
      session = { id: data.id, title: data.title || '新对话', created_at: data.created_at, updated_at: data.updated_at }
    }
  } catch { /* fallback */ }

  if (!session) {
    // localStorage fallback
    const id = `session_${Date.now()}_${Math.random().toString(36).slice(2)}`
    session = { id, title: '新对话', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
  }

  sessions.value.unshift(session)
  persistSessions() // 始终同步到 localStorage
  try { await switchSession(session.id) } catch { /* ignore */ }
}

async function switchSession(id: string) {
  if (activeSessionId.value === id) return
  // 保存当前会话消息
  if (activeSessionId.value) persistMessages()
  activeSessionId.value = id
  messages.value = []
  await loadMessages()
  nextTick(() => scrollToBottom())
}

async function deleteSession(id: string) {
  try { await api.delete(`/v1/conversations/${id}`) } catch { /* ignore */ }
  sessions.value = sessions.value.filter(s => s.id !== id)
  localStorage.removeItem(`chat_msgs_${id}`)
  if (activeSessionId.value === id) {
    if (sessions.value.length > 0) {
      try { await switchSession(sessions.value[0].id) } catch { /* ignore */ }
    } else {
      activeSessionId.value = ''
      messages.value = []
    }
  }
  persistSessions()
}

function getActiveTitle(): string {
  const s = sessions.value.find(s => s.id === activeSessionId.value)
  return s?.title || '新对话'
}

function updateSessionTitle(title: string) {
  const s = sessions.value.find(s => s.id === activeSessionId.value)
  if (s) {
    s.title = title
    persistSessions()
    try { api.put(`/v1/conversations/${s.id}`, { title }) } catch { /* ignore */ }
  }
}

// ── 消息持久化（per-session） ──
function persistMessages() {
  if (!activeSessionId.value) return
  const data = messages.value.map(m => ({
    id: m.id, role: m.role, content: m.content,
    displayedLength: m.content.length, timestamp: m.timestamp,
  }))
  localStorage.setItem(`chat_msgs_${activeSessionId.value}`, JSON.stringify(data))
}

async function loadMessages() {
  // 优先从 API 获取消息（后端持久化）
  try {
    const res = await api.get(`/v1/conversations/${activeSessionId.value}`)
    const conv = res.data?.data || res.data
    const apiMsgs = conv?.messages
    if (apiMsgs && apiMsgs.length > 0) {
      messages.value = apiMsgs.map((m: any) => ({
        id: m.id, role: m.role, content: m.content,
        displayedLength: m.content.length, timestamp: new Date(m.created_at).getTime(),
      }))
      persistMessages() // 同步到 localStorage 作为缓存
      return
    }
  } catch { /* API 不可用，回退 localStorage */ }

  // 回退到 localStorage
  try {
    const raw = localStorage.getItem(`chat_msgs_${activeSessionId.value}`)
    if (!raw) return
    const saved = JSON.parse(raw) as Message[]
    if (saved.length > 0) {
      messages.value = saved.map(m => ({ ...m, displayedLength: m.content.length }))
    }
  } catch { /* ignore */ }
}

function persistSessions() {
  localStorage.setItem('chat_sessions', JSON.stringify(sessions.value))
}

// ── 发送消息 ──
async function sendMessage() {
  if (!input.value.trim() || loading.value) return
  if (!activeSessionId.value) await createSession()

  const userMessage: Message = {
    id: `msg_${Date.now()}`, role: 'user', content: input.value,
    displayedLength: input.value.length, timestamp: Date.now(),
  }
  messages.value.push(userMessage)
  persistMessages()
  const content = input.value
  input.value = ''
  loading.value = true
  await nextTick()
  scrollToBottom()

  // 首条消息时更新标题
  if (messages.value.length === 1) {
    const title = content.slice(0, 30) + (content.length > 30 ? '...' : '')
    updateSessionTitle(title)
  }

  const assistantMessage: Message = {
    id: `msg_${Date.now()}_assistant`, role: 'assistant',
    content: '', displayedLength: 0, timestamp: Date.now(),
  }
  messages.value.push(assistantMessage)
  streaming.value = true

  // 打字机定时器
  typewriterTimer = setInterval(() => {
    const msg = messages.value.find(m => m.id === assistantMessage.id)
    if (!msg || !streaming.value || msg.displayedLength >= msg.content.length) return
    const remaining = msg.content.length - msg.displayedLength
    const step = Math.max(1, Math.floor(remaining / 10))
    msg.displayedLength = Math.min(msg.content.length, msg.displayedLength + Math.min(step, 4))
    nextTick(() => scrollToBottom())
  }, 30)

  // SSE
  const eventSource = createSSEConnection(activeSessionId.value, (data) => {
    if (data.type === 'text') {
      assistantMessage.content += data.data?.content || data.content || ''
      if (assistantMessage.content.length - assistantMessage.displayedLength > 80) {
        assistantMessage.displayedLength = assistantMessage.content.length - 40
      }
    } else if (data.type === 'turn_done' || data.type === 'error') {
      if (data.type === 'error') assistantMessage.content += `\n\n错误: ${data.data?.content || data.content || ''}`
      assistantMessage.displayedLength = assistantMessage.content.length
      streaming.value = false
      loading.value = false
      eventSource.close()
      if (typewriterTimer) { clearInterval(typewriterTimer); typewriterTimer = null }
      nextTick(() => renderMermaid())
      persistMessages()
    }
  })

  try {
    // 构建请求体：文件引用 + 文本 + 模式
    let finalContent = content
    if (uploadedFiles.value.length > 0) {
      const fileRefs = uploadedFiles.value.map(f => {
        if (f.type.startsWith('image/')) return `![${f.name}](${f.url})`
        return `[${f.name}](${f.url})`
      }).join('\n')
      finalContent = fileRefs + '\n' + content
      uploadedFiles.value = []  // 发送后清空文件列表
    }
    const payload: Record<string, any> = {
      content: finalContent,
      session_id: activeSessionId.value,
    }
    if (mode.value === 'deep') {
      payload.llm_config = { deep_reasoning: true, max_turns: 10, temperature: 0.1 }
    }
    await api.post('/submit', payload)
  } catch (error: any) {
    assistantMessage.content = `发送失败: ${error.message}`
    assistantMessage.displayedLength = assistantMessage.content.length
    streaming.value = false
    loading.value = false
    eventSource.close()
    if (typewriterTimer) { clearInterval(typewriterTimer); typewriterTimer = null }
  }
}

// ── 辅助函数 ──
function scrollToBottom() {
  scrollRef.value?.scrollTo({ top: 999999, behavior: 'smooth' })
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() }
}

function renderMarkdown(text: string): string {
  if (!text) return ''
  try { return md.render(text) } catch { return md.utils?.escapeHtml(text) || text }
}

// ── 深度推理思考过程解析 ──
interface SplitContent {
  thinking: string
  answer: string
}
function splitThinking(content: string): SplitContent {
  // 提取所有 [thinking] 块内容
  const thinking: string[] = []
  const regex = /\[thinking\]([\s\S]*?)\[\/thinking\]/g
  let match
  while ((match = regex.exec(content)) !== null) {
    thinking.push(match[1].trim())
  }
  // 移除所有 [thinking]...[/thinking] 标签得到最终答案
  const answer = content.replace(/\[thinking\][\s\S]*?\[\/thinking\]/g, '').trim()
  return { thinking: thinking.join('\n\n'), answer }
}

const thinkingExpanded = ref<Record<string, boolean>>({})

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function formatRelativeTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const now = Date.now()
  const diff = now - d.getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}小时前`
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

// ── Mermaid ──
let mermaidInit = false
async function renderMermaid() {
  const els = document.querySelectorAll('.mermaid')
  if (!els.length) return
  try {
    const mod = await import('mermaid')
    if (!mermaidInit) { mod.default.initialize({ startOnLoad: false, theme: 'default', securityLevel: 'loose' }); mermaidInit = true }
    await mod.default.run({ nodes: Array.from(els) })
  } catch (e) { console.warn('mermaid error:', e) }
}

// ── 代码复制 ──
function handleMsgClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const code = decodeURIComponent(btn.dataset.code || '')
  if (!code) return
  navigator.clipboard.writeText(code).then(() => { btn.textContent = '已复制'; setTimeout(() => { btn.textContent = '复制' }, 2000) }).catch(() => { /* clipboard not available */ })
}

// ── 消息折叠 ──
const expandedMap = ref<Record<string, boolean>>({})
const overflowMap = ref<Record<string, boolean>>({})
const LONG_HEIGHT = 400

function toggleExpand(id: string) { expandedMap.value[id] = !expandedMap.value[id] }
function isOverflow(el: HTMLElement | null): boolean { return el ? el.scrollHeight > LONG_HEIGHT : false }
function checkOverflow(id: string, el: HTMLElement | null) { if (el && !overflowMap.value[id]) overflowMap.value[id] = isOverflow(el) }
function collapsedStyle(id: string) {
  if (expandedMap.value[id]) return {}
  if (overflowMap.value[id]) return { maxHeight: LONG_HEIGHT + 'px', overflow: 'hidden' }
  return {}
}
</script>

<template>
  <div class="chat-layout">
    <!-- 侧边栏 -->
    <div :class="['sidebar', { collapsed: sidebarCollapsed }]">
      <div class="sidebar-header">
        <NButton quaternary size="small" @click="sidebarCollapsed = !sidebarCollapsed">
          <template #icon><NIcon><ChatbubbleEllipsesOutline /></NIcon></template>
        </NButton>
        <span v-if="!sidebarCollapsed" class="sidebar-title">对话</span>
        <NButton v-if="!sidebarCollapsed" type="primary" size="small" @click="createSession">
          <template #icon><NIcon><AddOutline /></NIcon></template>
        </NButton>
      </div>

      <NScrollbar v-if="!sidebarCollapsed" class="session-list">
        <div v-if="sessions.length === 0" class="session-empty">
          <NEmpty size="small" description="暂无对话" />
        </div>
        <div
          v-for="s in sessions"
          :key="s.id"
          :class="['session-item', { active: s.id === activeSessionId }]"
          @click="switchSession(s.id)"
        >
          <div class="session-info">
            <div class="session-title">{{ s.title || '新对话' }}</div>
            <div class="session-time">{{ formatRelativeTime(s.updated_at || s.created_at) }}</div>
          </div>
          <NPopconfirm @positive-click="deleteSession(s.id)">
            <template #trigger>
              <NButton quaternary size="tiny" class="session-delete" @click.stop>
                <template #icon><NIcon><TrashOutline /></NIcon></template>
              </NButton>
            </template>
            确认删除此对话？
          </NPopconfirm>
        </div>
      </NScrollbar>
    </div>

    <!-- 主聊天区 -->
    <div class="chat-main">
      <div class="chat-header">
        <h3>{{ getActiveTitle() }}</h3>
      </div>

      <NScrollbar ref="scrollRef" class="chat-messages" @click="handleMsgClick">
        <div v-if="messages.length === 0" class="empty-state">
          <NEmpty description="开始新的对话" />
        </div>

        <div v-for="msg in messages" :key="msg.id" :class="['message', msg.role]">
          <div class="avatar-col">
            <NAvatar round size="small" :style="{ backgroundColor: msg.role === 'user' ? '#2080f0' : '#18a058' }">
              {{ msg.role === 'user' ? 'U' : 'A' }}
            </NAvatar>
          </div>
          <div class="message-body">
            <div class="message-meta">
              <span class="message-role">{{ msg.role === 'user' ? '你' : 'AI' }}</span>
              <span class="message-time">{{ formatTime(msg.timestamp) }}</span>
            </div>
            <div
              :ref="(el) => checkOverflow(msg.id, el as HTMLElement | null)"
              :class="['message-text', msg.role === 'assistant' ? 'markdown-body' : '']"
              :style="collapsedStyle(msg.id)"
            >
              <template v-if="msg.role === 'assistant' && (msg.displayedLength ?? 0) < (msg.content?.length ?? 0)">
                <span class="typewriter-text">{{ msg.content.slice(0, msg.displayedLength ?? 0) }}</span><span class="cursor-blink">▌</span>
              </template>
              <!-- 深度推理：思考过程与最终回答分开渲染 -->
              <div v-else-if="msg.role === 'assistant'" class="thinking-wrapper">
                <template v-if="splitThinking(msg.content).thinking">
                  <div class="thinking-section">
                    <button class="thinking-toggle" @click="thinkingExpanded[msg.id] = !thinkingExpanded[msg.id]">
                      <span class="thinking-icon">🧠</span>
                      <span>{{ thinkingExpanded[msg.id] ? '收起思考过程' : '展开思考过程' }}</span>
                      <span class="thinking-arrow">{{ thinkingExpanded[msg.id] ? '▼' : '▶' }}</span>
                    </button>
                    <div v-show="thinkingExpanded[msg.id]" class="thinking-content" v-html="renderMarkdown(splitThinking(msg.content).thinking)"></div>
                  </div>
                </template>
                <div v-if="splitThinking(msg.content).answer" class="rendered-content" v-html="renderMarkdown(splitThinking(msg.content).answer)"></div>
                <div v-else-if="!splitThinking(msg.content).thinking" class="rendered-content" v-html="renderMarkdown(msg.content)"></div>
              </div>
              <div v-else class="rendered-content" v-html="renderMarkdown(msg.content)"></div>
            </div>
            <button v-if="msg.content && overflowMap[msg.id] && !expandedMap[msg.id]" class="expand-btn" @click="toggleExpand(msg.id)">展开全文 ▼</button>
            <button v-else-if="msg.content && overflowMap[msg.id] && expandedMap[msg.id]" class="expand-btn" @click="toggleExpand(msg.id)">收起 ▲</button>
          </div>
        </div>

        <div v-if="loading" class="loading-indicator">
          <NSpin size="small" /><span>思考中...</span>
        </div>
      </NScrollbar>

      <div class="chat-input">
        <div class="chat-input-toolbar">
          <NSelect v-model:value="mode" :options="modeOptions" size="tiny" class="mode-select" />
          <NUpload
            :default-upload="false"
            :multiple="false"
            accept="image/*,.pdf,.doc,.docx,.txt,.csv,.json,.py,.js,.ts,.go,.md"
            @change="handleFileUpload"
          >
            <NButton size="tiny" :loading="uploading" :disabled="loading">
              <template #icon><span>📎</span></template>
            </NButton>
          </NUpload>
        </div>
        <div v-if="uploadedFiles.length > 0" class="file-preview-list">
          <div v-for="(f, i) in uploadedFiles" :key="i" class="file-preview-item">
            <NImage v-if="f.type.startsWith('image/') && f.url" :src="f.url" :alt="f.name" width="60" height="60" object-fit="cover" preview-disabled />
            <span class="file-name">{{ f.name }}</span>
            <button class="file-remove" @click="removeFile(i)">✕</button>
          </div>
        </div>
        <NInput v-model:value="input" type="textarea" placeholder="输入消息..." :autosize="{ minRows: 1, maxRows: 4 }" @keydown="handleKeydown" />
        <NButton type="primary" :disabled="(!input.trim() && uploadedFiles.length === 0) || loading" @click="sendMessage">
          <template #icon><SendOutline /></template>
        </NButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-layout {
  display: flex;
  height: 100vh;
}

/* ── 侧边栏 ── */
.sidebar {
  width: 260px;
  border-right: 1px solid var(--border-color, #eee);
  display: flex;
  flex-direction: column;
  background: var(--sidebar-bg, #fafafa);
  transition: width 0.2s;
}
.sidebar.collapsed { width: 48px; }

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  border-bottom: 1px solid var(--border-color, #eee);
}
.sidebar-title { flex: 1; font-weight: 600; font-size: 15px; }

.session-list { flex: 1; padding: 8px; }
.session-empty { padding: 40px 0; }

.session-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.15s;
  margin-bottom: 2px;
}
.session-item:hover { background: var(--hover-color, #e8e8ec); }
.session-item.active { background: var(--active-color, #d0e0ff); }

.session-info { flex: 1; min-width: 0; }
.session-title {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--text-color, #333);
}
.session-time { font-size: 11px; color: var(--text-color-3, #999); margin-top: 2px; }
.session-delete { opacity: 0; transition: opacity 0.15s; }
.session-item:hover .session-delete { opacity: 1; }

/* ── 主聊天区 ── */
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.chat-header { padding: 16px 24px; border-bottom: 1px solid var(--border-color, #eee); }
.chat-header h3 { margin: 0; font-size: 18px; }
.chat-messages { flex: 1; padding: 24px; }
.empty-state { display: flex; justify-content: center; align-items: center; height: 100%; }

/* ── 消息 ── */
.message { display: flex; gap: 12px; margin-bottom: 20px; }
.message.user { flex-direction: row-reverse; }
.avatar-col { flex-shrink: 0; }
.message-body { max-width: 70%; min-width: 0; }
.message.user .message-body { align-items: flex-end; }
.message-meta { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; padding: 0 4px; }
.message-role { font-size: 13px; font-weight: 600; color: var(--text-color, #333); }
.message-time { font-size: 11px; color: var(--text-color-3, #999); }
.message.user .message-meta { flex-direction: row-reverse; }
.message-text { padding: 12px 16px; border-radius: 12px; line-height: 1.6; word-break: break-word; }
.message.user .message-text { background-color: #2080f0; color: white; }
.message.assistant .message-text { background-color: var(--assistant-bg, #f4f4f5); color: var(--text-color, #333); }

/* ── Markdown ── */
.message-text :deep(h1), .message-text :deep(h2), .message-text :deep(h3), .message-text :deep(h4) { margin-top: 16px; margin-bottom: 8px; font-weight: 600; line-height: 1.3; }
.message-text :deep(h1) { font-size: 1.4em; }
.message-text :deep(h2) { font-size: 1.25em; }
.message-text :deep(h3) { font-size: 1.1em; }
.message-text :deep(p) { margin: 6px 0; }
.message-text :deep(p:first-child) { margin-top: 0; }
.message-text :deep(ul), .message-text :deep(ol) { padding-left: 24px; margin: 6px 0; }
.message-text :deep(li) { margin: 3px 0; }
.message-text :deep(blockquote) { margin: 8px 0; padding: 6px 12px; border-left: 3px solid #2080f0; background: rgba(32,128,240,0.06); border-radius: 4px; color: var(--text-color-2, #555); }
.message-text :deep(code) { font-family: 'Cascadia Code','JetBrains Mono','Fira Code',monospace; font-size: 0.88em; background: rgba(0,0,0,0.12); padding: 2px 6px; border-radius: 4px; }
.message-text :deep(pre) { margin: 0; padding: 14px 16px; background: #1e1e2e; border-radius: 0 0 8px 8px; overflow-x: auto; }
.message-text :deep(pre code) { background: none; padding: 0; font-size: 0.85em; color: #cdd6f4; }
.message-text :deep(.code-block-wrapper) { margin: 10px 0; border-radius: 8px; overflow: hidden; border: 1px solid #444; }
.message-text :deep(.code-block-header) { display: flex; justify-content: space-between; align-items: center; padding: 6px 12px; background: #2d2d3f; border-bottom: 1px solid #444; }
.message-text :deep(.code-lang) { font-size: 12px; font-weight: 600; color: #a0a0c0; text-transform: lowercase; }
.message-text :deep(.code-copy-btn) { padding: 2px 10px; border: 1px solid #555; border-radius: 4px; background: transparent; color: #bbb; font-size: 12px; cursor: pointer; transition: all 0.15s; }
.message-text :deep(.code-copy-btn:hover) { background: rgba(255,255,255,0.1); color: #fff; border-color: #888; }
.message-text :deep(.mermaid) { margin: 10px 0; padding: 12px; background: #fff; border-radius: 8px; overflow-x: auto; }
.message-text :deep(.katex) { font-size: 1.05em; }
.message-text :deep(.katex-display) { margin: 10px 0; overflow-x: auto; overflow-y: hidden; }
.message-text :deep(a) { color: #2080f0; text-decoration: none; }
.message-text :deep(a:hover) { text-decoration: underline; }
.message-text :deep(table) { border-collapse: collapse; margin: 10px 0; width: 100%; font-size: 0.92em; }
.message-text :deep(th), .message-text :deep(td) { border: 1px solid var(--border-color, #ddd); padding: 8px 12px; text-align: left; }
.message-text :deep(th) { background: var(--th-bg, #eef2f7); font-weight: 600; }
.message-text :deep(hr) { border: none; border-top: 1px solid var(--border-color, #ddd); margin: 12px 0; }
.message-text :deep(img) { max-width: 100%; border-radius: 8px; margin: 8px 0; }
.user .message-text :deep(code) { background: rgba(255,255,255,0.18); color: #fff; }
.user .message-text :deep(a) { color: #b3d9ff; }

.expand-btn { display: block; width: 100%; margin-top: 4px; padding: 6px; border: none; border-radius: 8px; background: transparent; color: #2080f0; font-size: 13px; cursor: pointer; text-align: center; transition: background 0.15s; }
.expand-btn:hover { background: rgba(32,128,240,0.08); }
.loading-indicator { display: flex; align-items: center; gap: 8px; padding: 8px 0; color: var(--text-color-3, #999); }
.chat-input-toolbar { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.chat-input-toolbar .mode-select { width: 120px; }
.file-preview-list { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
.file-preview-item { display: flex; align-items: center; gap: 6px; padding: 4px 8px; background: var(--hover-color, #f5f5f5); border-radius: 6px; font-size: 12px; }
.file-name { max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-remove { cursor: pointer; border: none; background: none; color: var(--error-color, #f00); font-size: 14px; padding: 0 2px; }

/* ── 深度推理思考过程样式 ── */
.thinking-wrapper { width: 100%; }
.thinking-section { margin-bottom: 12px; border-left: 3px solid #e8a317; padding-left: 12px; }
.thinking-toggle { display: inline-flex; align-items: center; gap: 6px; cursor: pointer; border: none; background: none; color: var(--text-color-2, #666); font-size: 13px; padding: 4px 8px; border-radius: 4px; }
.thinking-toggle:hover { background: var(--hover-color, #f0f0f0); }
.thinking-icon { font-size: 14px; }
.thinking-arrow { font-size: 10px; transition: transform 0.2s; }
.thinking-content { margin-top: 8px; padding: 12px 16px; background: #f8f6f0; border-radius: 8px; font-size: 13px; line-height: 1.6; color: #555; }
.dark .thinking-content { background: #2a2824; color: #bbb; }
.thinking-content p { margin: 4px 0; }
.thinking-content code { font-size: 12px; background: rgba(0,0,0,0.06); padding: 1px 4px; border-radius: 3px; }
.dark .thinking-content code { background: rgba(255,255,255,0.08); }
.cursor-blink { animation: blink 0.8s step-end infinite; font-size: 1em; line-height: 1; }
@keyframes blink { 50% { opacity: 0; } }
.typewriter-text { white-space: pre-wrap; }
.rendered-content { display: block; min-height: 1em; }

.chat-input { display: flex; gap: 12px; padding: 16px 24px; border-top: 1px solid var(--border-color, #eee); }

/* ── 深色模式 ── */
:global(html.dark) {
  --sidebar-bg: #1a1a2e;
  --border-color: #333;
  --hover-color: #2a2a3e;
  --active-color: #1a3a5c;
  --text-color: #e0e0e0;
  --text-color-2: #b0b0b0;
  --text-color-3: #888;
  --assistant-bg: #2a2a3e;
  --th-bg: #2a2a3e;
}

:global(html.dark) .sidebar {
  background: var(--sidebar-bg);
  border-right-color: var(--border-color);
}

:global(html.dark) .sidebar-header {
  border-bottom-color: var(--border-color);
}

:global(html.dark) .session-item:hover {
  background: var(--hover-color);
}

:global(html.dark) .session-item.active {
  background: var(--active-color);
}

:global(html.dark) .chat-header {
  border-bottom-color: var(--border-color);
}

:global(html.dark) .chat-input {
  border-top-color: var(--border-color);
}

:global(html.dark) .message.assistant .message-text {
  background-color: var(--assistant-bg);
  color: var(--text-color);
}

:global(html.dark) .message-text :deep(.mermaid) {
  background: #2a2a3e;
}
</style>
