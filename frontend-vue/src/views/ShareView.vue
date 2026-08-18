<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import { getPublicShare } from '../api'
import type { PublicShare } from '../api'

const route = useRoute()

const share = ref<PublicShare | null>(null)
const loading = ref(true)
const error = ref('')

// 轻量 markdown 渲染（公开页不引入 KaTeX/mermaid，代码块带复制按钮）
const md = new MarkdownIt({ html: false, linkify: true, breaks: true })
md.renderer.rules.fence = (tokens: any[], idx: number) => {
  const token = tokens[idx]
  const lang = md.utils.escapeHtml((token.info || '').trim().toLowerCase() || 'code')
  const code = md.utils.escapeHtml(token.content)
  const encoded = encodeURIComponent(token.content)
  return `<div class="code-block"><div class="code-block-head"><span class="code-lang">${lang}</span><button class="code-copy" data-code="${encoded}">复制</button></div><pre><code>${code}</code></pre></div>`
}

function renderMarkdown(src: string): string {
  try {
    return DOMPurify.sanitize(md.render(src), {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'code', 'pre', 'ul', 'ol', 'li', 'a', 'blockquote', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'span', 'div', 'table', 'thead', 'tbody', 'tr', 'th', 'td', 'hr', 'del', 'sup', 'sub', 'button'],
      ALLOWED_ATTR: ['href', 'target', 'rel', 'class', 'style', 'data-code'],
      ALLOW_DATA_ATTR: true,
    })
  } catch {
    return md.utils.escapeHtml(src)
  }
}

function handleClick(e: MouseEvent) {
  const btn = (e.target as HTMLElement).closest('.code-copy') as HTMLElement | null
  if (!btn) return
  const code = decodeURIComponent(btn.dataset.code || '')
  if (!code) return
  navigator.clipboard.writeText(code).then(() => {
    btn.textContent = '已复制'
    setTimeout(() => { btn.textContent = '复制' }, 2000)
  }).catch(() => { /* clipboard unavailable */ })
}

onMounted(async () => {
  const id = String(route.params.id || '')
  try {
    share.value = await getPublicShare(id)
  } catch (e: any) {
    const status = e?.response?.status
    if (status === 410) error.value = '此分享已被创建者取消'
    else if (status === 404) error.value = '分享不存在或已失效'
    else error.value = '加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
})

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日 ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}
</script>

<template>
  <div class="share-page">
    <header class="share-header">
      <div class="share-brand">
        <span class="share-brand-mark">MC</span>
        <span>MiniCC · 对话分享</span>
      </div>
    </header>

    <main class="share-main">
      <div v-if="loading" class="share-state">加载中…</div>
      <div v-else-if="error" class="share-state share-error">{{ error }}</div>
      <template v-else-if="share">
        <div class="share-head">
          <h1 class="share-title">{{ share.title || '新对话' }}</h1>
          <div class="share-meta">
            {{ formatDate(share.created_at) }} · {{ share.messages.length }} 条消息
          </div>
        </div>

        <div class="share-thread" @click="handleClick">
          <div v-for="(m, i) in share.messages" :key="i" class="share-msg" :class="m.role">
            <div class="share-msg-bubble">
              <div v-if="m.role === 'assistant'" class="share-msg-text" v-html="renderMarkdown(m.content)"></div>
              <div v-else class="share-msg-text">{{ m.content }}</div>
            </div>
          </div>
        </div>
      </template>
    </main>

    <footer class="share-footer">由 MiniCC 生成 · 本页可被任何获得链接的人查看</footer>
  </div>
</template>

<style scoped>
.share-page { min-height: 100vh; display: flex; flex-direction: column; background: var(--bg-page); color: var(--text-primary); }
.share-header { height: 52px; display: flex; align-items: center; padding: 0 24px; border-bottom: 1px solid var(--border); background: var(--bg-card); }
.share-brand { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600; color: var(--text-primary); }
.share-brand-mark { width: 22px; height: 22px; border-radius: 6px; background: linear-gradient(135deg, var(--primary), var(--primary-dark)); color: #fff; font-weight: 700; font-size: 10px; display: inline-flex; align-items: center; justify-content: center; }
.share-main { flex: 1; width: 100%; max-width: 748px; margin: 0 auto; padding: 32px 24px 60px; }
.share-state { padding: 80px 0; text-align: center; color: var(--text-muted); font-size: 14px; }
.share-state.share-error { color: var(--text-secondary); }
.share-head { margin-bottom: 24px; }
.share-title { font-size: 22px; line-height: 30px; font-weight: 700; margin: 0 0 6px; word-break: break-word; }
.share-meta { font-size: 12px; color: var(--text-tertiary); }
.share-thread { display: flex; flex-direction: column; gap: 18px; }
.share-msg { display: flex; }
.share-msg.user { justify-content: flex-end; }
.share-msg-bubble { min-width: 0; max-width: 100%; }
.share-msg.user .share-msg-bubble {
  max-width: min(525px, 100%);
  padding: 10px 16px;
  background: var(--bubble-user); color: var(--bubble-user-text);
  border-radius: 22px; border-bottom-right-radius: 6px;
  font-size: 15px; line-height: 24px; white-space: pre-wrap; word-break: break-word;
}
.share-msg-text { font-size: 16px; line-height: 28px; color: var(--text-primary); word-break: break-word; }
.share-msg-text :deep(p) { margin: 16px 0; }
.share-msg-text :deep(p:first-child) { margin-top: 0; }
.share-msg-text :deep(h1) { font-size: 24px; line-height: 34px; font-weight: 700; margin: 32px 0 16px; }
.share-msg-text :deep(h2) { font-size: 22px; line-height: 32px; font-weight: 700; margin: 32px 0 16px; }
.share-msg-text :deep(h3) { font-size: 20px; line-height: 30px; font-weight: 700; margin: 32px 0 16px; }
.share-msg-text :deep(ul), .share-msg-text :deep(ol) { padding-left: 24px; margin: 16px 0; }
.share-msg-text :deep(li) { margin: 4px 0; }
.share-msg-text :deep(blockquote) { margin: 16px 0; padding: 6px 12px; border-left: 3px solid var(--primary); background: var(--bg-hover); border-radius: 6px; }
.share-msg-text :deep(a) { color: var(--primary); text-decoration: none; }
.share-msg-text :deep(a:hover) { text-decoration: underline; }
.share-msg-text :deep(code) { font-family: var(--font-mono); font-size: 0.9em; background: var(--bg-secondary); padding: 2px 6px; border-radius: 6px; }
.share-msg-text :deep(.code-block) { margin: 16px 0; background: var(--bg-code); border-radius: 12px; overflow: hidden; }
.share-msg-text :deep(.code-block-head) { display: flex; justify-content: space-between; align-items: center; padding: 8px 14px; background: var(--bg-secondary); }
.share-msg-text :deep(.code-lang) { font-family: var(--font-mono); font-size: 12px; color: var(--text-primary); }
.share-msg-text :deep(.code-copy) { background: none; border: none; color: var(--text-tertiary); cursor: pointer; font-size: 12px; padding: 0; }
.share-msg-text :deep(.code-copy:hover) { color: var(--primary); }
.share-msg-text :deep(pre) { margin: 0 !important; padding: 16px; overflow-x: auto; white-space: pre-wrap; word-break: break-all; }
.share-msg-text :deep(pre code) { background: none; padding: 0; font-size: 0.9em; color: var(--text-code); }
.share-msg-text :deep(table) { border-collapse: collapse; margin: 16px 0; width: 100%; }
.share-msg-text :deep(th), .share-msg-text :deep(td) { border: 1px solid var(--border); padding: 8px 12px; text-align: left; }
.share-msg-text :deep(th) { background: var(--bg-secondary); font-weight: 600; }
.share-footer { padding: 18px 24px; text-align: center; font-size: 12px; color: var(--text-muted); border-top: 1px solid var(--border); }
</style>
