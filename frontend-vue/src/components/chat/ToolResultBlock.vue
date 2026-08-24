<script setup lang="ts">
import { ref, computed } from 'vue'
import { CaretRightOutlined } from '@ant-design/icons-vue'
import type { ToolResultItem } from './chat-types'

const props = defineProps<{ item: ToolResultItem }>()
const expanded = ref(false)

// S 修复：仅放行安全光栅格式的 data: URI，拒绝 svg+xml 等可携带外部资源/脚本语义的类型
const isImageData = computed(() => {
  const c = props.item.content
  return /^data:image\/(png|jpe?g|gif|webp);base64,/.test(c)
})

// 分型解析：read_file / 终端 / 搜索 / JSON / 文本
const parsed = computed<{ kind: 'read' | 'terminal' | 'search' | 'json' | 'text'; read?: any; terminal?: any; search?: any; text: string }>(() => {
  const c = props.item.content
  if (isImageData.value) return { kind: 'text', text: '' }
  let obj: any = null
  try { obj = JSON.parse(c) } catch { /* not json */ }

  if (obj && typeof obj === 'object' && 'path' in obj && 'content' in obj && obj.content !== undefined) {
    return { kind: 'read', read: obj, text: '' }
  }
  if (obj && typeof obj === 'object' && ('stdout' in obj || 'exit_code' in obj)) {
    return { kind: 'terminal', terminal: obj, text: '' }
  }
  if (obj && typeof obj === 'object' && Array.isArray(obj.matches)) {
    return { kind: 'search', search: obj, text: '' }
  }
  if (obj && typeof obj === 'object') {
    return { kind: 'json', text: JSON.stringify(obj, null, 2) }
  }
  const text = c.length <= 6000 ? c : `${c.slice(0, 3000)}\n...(truncated ${c.length - 6000} chars)...\n${c.slice(-3000)}`
  return { kind: 'text', text }
})

// read 分型的行号行
const readLines = computed(() => {
  const read = parsed.value.read
  if (!read) return []
  const content = typeof read.content === 'string' ? read.content : String(read.content || '')
  const offset = Number(read.offset || 0)
  return content.split('\n').map((line: string, i: number) => ({ n: offset + i + 1, line }))
})

// 搜索分型：按文件分组（deepseek SearchBlock）
const searchFiles = computed(() => {
  const s = parsed.value.search
  if (!s) return []
  const groups = new Map<string, { path: string; matches: { line: number; text: string }[] }>()
  for (const m of s.matches || []) {
    const path = m.path || ''
    if (!groups.has(path)) groups.set(path, { path, matches: [] })
    groups.get(path)!.matches.push({ line: Number(m.line) || 0, text: String(m.text ?? '') })
  }
  return Array.from(groups.values())
})
</script>

<template>
  <div class="tool-result" :class="{ error: item.isError }">
    <img v-if="isImageData" :src="item.content" class="result-image" alt="tool result" loading="lazy" decoding="async" />

    <!-- read 卡片：banner + 行号 gutter（deepseek ReadBlock） -->
    <div v-else-if="parsed.kind === 'read'" class="read-block">
      <div class="read-banner">
        <span class="read-path">{{ parsed.read.path }}</span>
        <span class="read-count">{{ parsed.read.total_lines }} 行</span>
      </div>
      <div class="read-body">
        <div v-for="row in readLines" :key="row.n" class="read-line">
          <span class="line-no">{{ row.n }}</span>
          <span class="line-text">{{ row.line || ' ' }}</span>
        </div>
      </div>
    </div>

    <!-- 终端卡片（deepseek TerminalBlock 语义） -->
    <div v-else-if="parsed.kind === 'terminal'" class="terminal-block">
      <div class="terminal-banner">
        <span class="terminal-label">终端输出</span>
        <span class="terminal-exit" :class="{ nonzero: parsed.terminal.exit_code }">exit {{ parsed.terminal.exit_code ?? '?' }}</span>
      </div>
      <pre class="terminal-body">{{ parsed.terminal.stdout || parsed.terminal.output || '' }}{{ parsed.terminal.stderr ? '\n[stderr] ' + parsed.terminal.stderr : '' }}</pre>
    </div>

    <!-- 搜索卡片（deepseek SearchBlock：banner + 行号 + pre 水平滚动） -->
    <div v-else-if="parsed.kind === 'search'" class="search-block">
      <div class="search-header">
        <span class="search-summary">{{ parsed.search.count ?? searchFiles.length }} 个匹配 · {{ searchFiles.length }} 个文件</span>
      </div>
      <div class="search-body">
        <template v-for="file in searchFiles" :key="file.path">
          <div class="search-file">{{ file.path }}</div>
          <div v-for="m in file.matches" :key="m.line" class="search-line">
            <span class="search-line-no">{{ m.line }}</span>
            <span class="search-line-text">{{ m.text }}</span>
          </div>
        </template>
      </div>
    </div>

    <!-- JSON / 代码 / 文本：可折叠 -->
    <template v-else>
      <button class="result-head" type="button" @click="expanded = !expanded">
        <CaretRightOutlined class="chevron" :class="{ open: expanded }" />
        <span class="result-label">{{ item.isError ? '结果（失败）' : '结果' }}</span>
      </button>
      <template v-if="expanded">
        <pre v-if="parsed.kind === 'json'" class="result-code">{{ parsed.text }}</pre>
        <pre v-else-if="parsed.kind === 'text' && parsed.text.includes('\n')" class="result-code">{{ parsed.text }}</pre>
        <div v-else class="result-text">{{ parsed.text }}</div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.tool-result { max-width: min(720px, 92%); margin: 2px auto 8px; }
.result-head { display: flex; align-items: center; gap: 8px; width: 100%; padding: 4px 0; border: none; background: none; color: var(--text-tertiary); cursor: pointer; font-size: 12px; }
.result-head:hover { color: var(--primary); }
.tool-result.error .result-label { color: var(--error); }
.chevron { font-size: 10px; transition: transform 0.2s; }
.chevron.open { transform: rotate(90deg); }
.result-code { margin: 0; padding: 12px; background: var(--bg-code); border-radius: var(--sig-radius-code); font-family: var(--font-mono); font-size: 12px; line-height: 1.6; color: var(--text-code); white-space: pre-wrap; word-break: break-all; }
.result-text { padding: 10px 12px; background: var(--bg-secondary); border-radius: var(--sig-radius-button); font-size: 13px; color: var(--text-secondary); white-space: pre-wrap; word-break: break-all; }
.result-image { max-width: min(320px, 80vw); max-height: 240px; border-radius: var(--sig-radius-card); border: 1px solid var(--border-card); display: block; margin-top: 4px; }

/* read 卡片：12px 圆角 + banner + 48px 行号 gutter + 22px 行高（deepseek ReadBlock） */
.read-block { margin: 8px 0; background: var(--bg-code); border-radius: var(--sig-radius-code); overflow: hidden; }
.read-banner { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 9px 14px; background: var(--bg-secondary); }
.read-path { font-family: var(--font-mono); font-size: 12px; line-height: 18px; color: var(--text-primary); min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.read-count { font-size: 12px; color: var(--text-tertiary); flex-shrink: 0; }
.read-body { max-height: 420px; overflow-y: auto; }
.read-line { display: flex; line-height: 22px; font-family: var(--font-mono); font-size: 12px; }
.line-no { flex: 0 0 48px; padding-right: 12px; text-align: right; color: var(--text-tertiary); user-select: none; }
.line-text { flex: 1; padding-right: 14px; white-space: pre; overflow-x: auto; color: var(--text-primary); }

/* 终端卡片（deepseek TerminalBlock 语义） */
.terminal-block { margin: 8px 0; background: var(--terminal-bg); border-radius: var(--sig-radius-code); overflow: hidden; }
.terminal-banner { display: flex; justify-content: space-between; align-items: center; padding: 9px 14px; background: var(--terminal-header-bg); }
.terminal-label { font-family: var(--font-mono); font-size: 12px; color: var(--terminal-text); }
.terminal-exit { font-family: var(--font-mono); font-size: 12px; color: var(--success); }
.terminal-exit.nonzero { color: var(--error); }
.terminal-body { margin: 0; padding: 14px; font-family: var(--font-mono); font-size: 12px; line-height: 1.6; color: var(--terminal-text); white-space: pre-wrap; word-break: break-all; max-height: 360px; overflow-y: auto; }

/* 搜索卡片（deepseek SearchBlock：12px 圆角 + banner + 22px 行 + pre 不折行） */
.search-block { margin: 8px 0; background: var(--bg-code); border-radius: var(--sig-radius-code); overflow: hidden; }
.search-header { display: flex; align-items: center; gap: 12px; padding: 9px 14px; background: var(--bg-secondary); }
.search-summary { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; color: var(--text-secondary); }
.search-body { padding: 8px 14px 12px 0; overflow-x: auto; font-family: var(--font-mono); font-size: 12px; }
.search-file { padding: 6px 0 2px 14px; color: var(--primary); font-weight: 600; white-space: pre; }
.search-line { min-height: 22px; padding-left: 14px; white-space: pre; color: var(--text-primary); }
.search-line-no { display: inline-block; width: 40px; color: var(--text-tertiary); user-select: none; }
.search-line-text { color: var(--text-primary); }
/* ── 移动端：结果卡片内边距/字号压缩 ── */
@media (max-width: 768px) {
  .result-code { padding: 10px; font-size: 11px; }
  .result-text { padding: 8px 10px; }
  .read-banner, .terminal-banner { padding: 8px 10px; }
  .read-line { line-height: 20px; font-size: 11px; }
  .search-body { font-size: 11px; }
}
</style>
