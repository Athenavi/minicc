<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import 'katex/dist/katex.min.css'
import texmath from 'markdown-it-texmath'
import katex from 'katex'
import { computed } from 'vue'
import { message } from 'ant-design-vue'
import { CopyOutlined } from '@ant-design/icons-vue'
import ReasoningBlock from './ReasoningBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import ToolResultBlock from './ToolResultBlock.vue'
import type { ChatItem, ToolCallItem, ToolResultItem } from './chat-types'

const props = defineProps<{
  item: ChatItem
  anchorKey?: number
  highlighted?: boolean
}>()

// 消息时间（hover 淡入显示，deepseek MessageIconActions timeEnd）
// S 修复：历史消息用落库 created_at（item.time），实时消息缺省时取当前时间
const timeLabel = computed(() => {
  if (props.item.time) return props.item.time
  const now = new Date()
  return `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`
})

async function copyMessage() {
  if (props.item.kind !== 'text') return
  try {
    await navigator.clipboard.writeText(props.item.content)
    message.success('已复制')
  } catch {
    message.error('复制失败')
  }
}

// ── Markdown 引擎（迁移自原 ChatView） ──
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

// P 性能：图片懒加载（长列表/历史中大量图片不阻塞首屏，滚动到才加载）
md.renderer.rules.image = (tokens: any[], idx: number) => {
  const token = tokens[idx]
  const src = md.utils.escapeHtml(token.attrGet('src') || '')
  const alt = md.utils.escapeHtml(token.content || '')
  return `<img src="${src}" alt="${alt}" loading="lazy" decoding="async" />`
}

function renderMarkdown(src: string): string {
  // P 性能：LRU 缓存渲染结果（同内容消息不重复 markdown + 高亮）
  const hit = mdCache.get(src)
  if (hit !== undefined) return hit
  let out: string
  try {
    out = DOMPurify.sanitize(md.render(src), {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'code', 'pre', 'ul', 'ol', 'li', 'a', 'blockquote', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'span', 'div', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'hr', 'del', 'sup', 'sub', 'img', 'button'],
      ALLOWED_ATTR: ['href', 'target', 'rel', 'class', 'style', 'src', 'alt', 'loading', 'decoding', 'data-code'],
      ALLOW_DATA_ATTR: true,
    })
  } catch {
    out = md.utils.escapeHtml(src)
  }
  if (mdCache.size >= MD_CACHE_MAX) {
    const firstKey = mdCache.keys().next().value
    if (firstKey !== undefined) mdCache.delete(firstKey)
  }
  mdCache.set(src, out)
  return out
}

const mdCache = new Map<string, string>()
const MD_CACHE_MAX = 300

function handleMsgClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement).closest('.code-copy-btn') as HTMLElement | null
  if (!btn) return
  const code = decodeURIComponent(btn.dataset.code || '')
  if (!code) return
  navigator.clipboard.writeText(code).then(() => {
    btn.textContent = '已复制'
    setTimeout(() => { btn.textContent = '复制' }, 2000)
  }).catch(() => { /* clipboard not available */ })
}
</script>

<template>
  <div
    v-if="item.kind === 'text'"
    v-bind="$attrs"
    class="msg-row"
    :class="[item.role, { streaming: item.streaming, highlighted: highlighted }]"
    :data-chat-anchor-key="anchorKey"
    @click="handleMsgClick"
    data-time-hover-root
  >
    <div class="msg-content">
      <!-- P 性能：流式期间纯文本渲染（零 markdown 开销），完成后一次性 markdown + 缓存 -->
      <div v-if="item.streaming" class="msg-text streaming-text">{{ item.content }}</div>
      <div v-else class="msg-text" v-html="renderMarkdown(item.content)"></div>
      <div class="msg-actions" :class="item.role">
        <span class="msg-time">{{ timeLabel }}</span>
        <button class="msg-action" type="button" title="复制" @click.stop="copyMessage">
          <CopyOutlined />
        </button>
      </div>
    </div>
  </div>

  <ReasoningBlock v-else-if="item.kind === 'reasoning'" :content="item.content" :streaming="item.streaming" />
  <ToolCallCard v-else-if="item.kind === 'tool_call'" :item="item as ToolCallItem" />
  <ToolResultBlock v-else-if="item.kind === 'tool_result'" :item="item as ToolResultItem" />
  <!-- UI/UX：日期分隔线（deepseek DateDivider） -->
  <div v-else-if="item.kind === 'date_divider'" v-bind="$attrs" class="date-divider">
    <span class="date-divider-line" /><span class="date-divider-text">{{ item.content }}</span><span class="date-divider-line" />
  </div>

  <div v-else-if="item.kind === 'turn_stats'" v-bind="$attrs" class="turn-stats">
    <span v-if="item.inputTokens || item.outputTokens || item.durationSec">
      <template v-if="item.durationSec">耗时 {{ item.durationSec }}s · </template>
      tokens: {{ item.inputTokens }} in / {{ item.outputTokens }} out
    </span>
  </div>
</template>

<style scoped>
/* 消息列：748px 内容宽（deepseek --dsh-chat-content-width），16px 节奏。
   注意：虚拟列表 item 为 absolute（left:0 right:0），此处不能设 width:100%，
   否则 left+width+right 超约束会让 right 失效、margin auto 退化为 0（消息列贴左） */
.msg-row { padding: 6px 0; max-width: 748px; margin: 0 auto; }
.msg-row.user { display: flex; justify-content: flex-end; }
/* 轨迹跳转高亮闪烁（deepseek data-current 聚焦反馈） */
.msg-row.highlighted { background: var(--primary-bg); border-radius: 12px; animation: trajectoryFlash 2s ease-out; }
@keyframes trajectoryFlash { 0% { background: var(--primary-bg); } 100% { background: transparent; } }
.msg-content { min-width: 0; }
.msg-text { font-size: 16px; line-height: 28px; color: var(--text-primary); }
.streaming-text { white-space: pre-wrap; word-break: break-word; }
/* UI/UX：流式生成光标（deepseek 打字效果） */
.streaming-text::after {
  content: '▍';
  display: inline-block;
  margin-left: 2px;
  color: var(--primary);
  animation: streamCursor 0.9s ease-in-out infinite;
}
@keyframes streamCursor { 0%, 100% { opacity: 1; } 50% { opacity: 0.2; } }
/* UI/UX：日期分隔线（deepseek DateDivider） */
.date-divider { display: flex; align-items: center; gap: 12px; max-width: 748px; margin: 18px auto; padding: 0 20px; }
.date-divider-line { flex: 1; height: 1px; background: var(--border); opacity: 0.6; }
.date-divider-text { font-size: 12px; color: var(--text-tertiary); white-space: nowrap; }
/* 流式光标：assistant 正在输出时末尾闪烁（打字感） */
.msg-row.streaming.assistant .msg-text::after {
  content: ''; display: inline-block; width: 2px; height: 1em;
  margin-left: 2px; vertical-align: -0.15em;
  background: var(--primary);
  animation: streamCursor 1s step-end infinite;
}
@keyframes streamCursor { 0%, 100% { opacity: 1; } 50% { opacity: 0; } }
@media (prefers-reduced-motion: reduce) { .msg-row.streaming.assistant .msg-text::after { animation: none; } }
.msg-row.assistant .msg-text { padding: 2px 0; }
/* 用户气泡：deepseek User_Bubble — deepseek-50 淡蓝底 + 深色文字，22px 圆角，525px 上限 */
.msg-row.user .msg-text {
  display: inline-block; padding: 10px 16px;
  background: var(--bubble-user); color: var(--bubble-user-text);
  border-radius: 22px; border-bottom-right-radius: 6px; max-width: min(525px, 100%);
  line-height: 24px;
  transition: box-shadow 0.2s ease;
}
.msg-row.user .msg-text:hover { box-shadow: var(--shadow-md); }
.turn-stats { max-width: 748px; margin: 0 auto; padding: 4px 0 10px; font-size: 11px; color: var(--text-muted); text-align: right; }
/* markdown 正文（deepseek MarkdownText：16/28、标题层级、块 gap 16） */
.msg-text :deep(p) { margin: 16px 0; }
.msg-text :deep(p:first-child) { margin-top: 0; }
.msg-text :deep(h1) { font-size: 24px; line-height: 34px; font-weight: 700; margin: 32px 0 16px; }
.msg-text :deep(h2) { font-size: 22px; line-height: 32px; font-weight: 700; margin: 32px 0 16px; }
.msg-text :deep(h3) { font-size: 20px; line-height: 30px; font-weight: 700; margin: 32px 0 16px; }
.msg-text :deep(h4) { font-size: 16px; line-height: 28px; font-weight: 600; margin: 16px 0; }
.msg-text :deep(strong) { font-weight: 600; }
.msg-text :deep(ul), .msg-text :deep(ol) { padding-left: 24px; margin: 16px 0; }
.msg-text :deep(li) { margin: 4px 0; }
.msg-text :deep(blockquote) { margin: 16px 0; padding: 6px 12px; border-left: 3px solid var(--primary); background: var(--bg-hover); border-radius: 6px; }
.msg-text :deep(a) { color: var(--primary); text-decoration: none; }
.msg-text :deep(a:hover) { text-decoration: underline; }
.msg-text :deep(table) { border-collapse: collapse; margin: 16px 0; width: 100%; }
.msg-text :deep(th), .msg-text :deep(td) { border: 1px solid var(--border); padding: 8px 12px; text-align: left; }
.msg-text :deep(th) { background: var(--bg-secondary); font-weight: 600; }
.msg-text :deep(img) { max-width: 100%; border-radius: 12px; }/* 行内代码（deepseek markdown-inline-code：bluish-100 底） */
.msg-text :deep(code) { font-family: var(--font-mono); font-size: 0.9em; background: var(--bg-secondary); padding: 2px 6px; border-radius: 6px; }
/* 代码块：12px 圆角 + banner（deepseek CodeBlock） */
.msg-text :deep(.code-block-wrapper) { margin: 16px 0; background: var(--bg-code); border-radius: 12px; overflow: hidden; }
.msg-text :deep(.code-block-header) { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 9px 14px; background: var(--bg-secondary); }
.msg-text :deep(.code-lang) { font-family: var(--font-mono); font-size: 12px; line-height: 18px; color: var(--text-primary); }
.msg-text :deep(.code-copy-btn) { background: none; border: none; color: var(--text-tertiary); cursor: pointer; font-size: 12px; padding: 0; }
.msg-text :deep(.code-copy-btn:hover) { color: var(--primary); }
.msg-text :deep(pre) { margin: 0 !important; padding: 16px; overflow-x: auto; white-space: pre-wrap; word-break: break-all; }
.msg-text :deep(pre code) { background: none; padding: 0; font-size: 0.9em; color: var(--text-code); }
/* 消息操作行（deepseek MessageIconActions：28px 高、hover 淡入、80ms） */
.msg-actions { display: flex; align-items: center; gap: 10px; height: 28px; margin-top: 4px; }
.msg-actions.user { justify-content: flex-end; }
.msg-time { padding-right: 6px; font-size: 12px; color: var(--text-tertiary); opacity: 0; transition: opacity 80ms ease; }
[data-time-hover-root]:hover .msg-time, [data-time-hover-root]:focus-within .msg-time { opacity: 1; }
.msg-action { display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: none; border-radius: 50%; background: transparent; color: var(--text-tertiary); cursor: pointer; opacity: 0; transition: opacity 80ms ease, background 0.15s ease; }
.msg-action:hover { background: var(--bg-hover); color: var(--text-primary); }
[data-time-hover-root]:hover .msg-action, [data-time-hover-root]:focus-within .msg-action { opacity: 1; }
@media (hover: none) { .msg-time, .msg-action { opacity: 1; } }
@media (max-width: 768px) { .msg-row { padding: 6px 0; } }
</style>
