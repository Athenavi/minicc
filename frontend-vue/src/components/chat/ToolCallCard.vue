<script setup lang="ts">
import { ref, computed } from 'vue'
import { CaretRightOutlined } from '@ant-design/icons-vue'
import type { ToolCallItem } from './chat-types'

const props = defineProps<{ item: ToolCallItem; depth?: number }>()
const expanded = ref(false)

function prettyArgs(): string {
  try {
    return JSON.stringify(JSON.parse(props.item.arguments || '{}'), null, 2)
  } catch {
    return props.item.arguments || ''
  }
}

// 参数摘要：首行/关键字段（deepseek ToolRow summary 截断）
const summary = computed(() => {
  const a = props.item.arguments || ''
  if (!a) return ''
  try {
    const parsed = JSON.parse(a)
    const keys = Object.keys(parsed)
    if (keys.length === 0) return ''
    // 取前两个短字段值
    const parts = keys.slice(0, 2).map(k => {
      const v = parsed[k]
      const s = typeof v === 'string' ? v : JSON.stringify(v)
      return s.length > 40 ? s.slice(0, 40) + '…' : s
    })
    return parts.join(' · ')
  } catch {
    return a.length > 60 ? a.slice(0, 60) + '…' : a
  }
})

const padLeft = computed(() => (props.depth || 0) * 22)
</script>

<template>
  <div class="tool-row-wrap" :data-state="item.status" :data-tool="item.name">
    <!-- 工具树缩进连接线 -->
    <div v-if="(depth || 0) > 0" class="tree-guide" :style="{ left: `${padLeft - 14}px` }" aria-hidden />

    <div class="tool-row" :style="{ marginLeft: `${padLeft}px` }">
      <button class="tool-main" type="button" @click="expanded = !expanded">
        <CaretRightOutlined class="chevron" :class="{ open: expanded }" />
        <span class="tool-name">{{ item.name }}</span>
        <span class="sep" aria-hidden />
        <span class="state-dot" :class="item.status" aria-hidden />
        <span class="tool-summary">{{ summary }}</span>
      </button>

      <Transition name="expand">
        <div v-if="expanded" class="tool-args"><pre>{{ prettyArgs() }}</pre></div>
      </Transition>
    </div>
  </div>
</template>

<style scoped>
.tool-row-wrap { position: relative; max-width: min(720px, 92%); margin: 2px auto 0; padding: 0 24px; }
.tree-guide { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--border); }

/* 工具行：24px 单行（deepseek ToolRow 视觉） */
.tool-row { overflow: hidden; border-radius: var(--sig-radius-code); }
.tool-row:hover { background: var(--bg-hover); }
.tool-main { display: flex; align-items: center; gap: 6px; width: 100%; height: 28px; padding: 0 8px; border: none; background: none; color: var(--text-secondary); cursor: pointer; font-size: 13px; text-align: left; }
.chevron { font-size: 10px; color: var(--text-muted); transition: transform 0.2s; flex-shrink: 0; }
.chevron.open { transform: rotate(90deg); }
.tool-name { font-family: var(--font-mono); font-size: 13px; color: var(--text-primary); font-weight: 400; white-space: nowrap; }
.sep { flex: none; width: 2px; height: 2px; border-radius: 1px; background: var(--text-muted); margin: 0 6px; }

/* 状态点：实心 + 0.1 光晕双层（StateDot） */
.state-dot { position: relative; flex: none; width: 10px; height: 10px; }
.state-dot::before { content: ''; position: absolute; inset: 0; border-radius: 50%; background: currentColor; opacity: 0.12; }
.state-dot::after { content: ''; position: absolute; inset: 20%; border-radius: 50%; background: currentColor; }
.state-dot.running { color: var(--primary); }
.state-dot.running::after { animation: dotPulse 1.4s ease-in-out infinite; }
.state-dot.done { color: var(--success); }
.state-dot.error { color: var(--error); }
@keyframes dotPulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

.tool-summary { flex: 1; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; color: var(--text-tertiary); font-size: 12px; }

/* running sweep 流光（deepseek ToolRow sweep） */
.tool-row-wrap[data-state='running'] .tool-row { position: relative; }
.tool-row-wrap[data-state='running'] .tool-row::after {
  content: ''; position: absolute; top: 0; bottom: 0; left: 0; width: 300px;
  background: linear-gradient(90deg, transparent 0%, color-mix(in srgb, var(--bg-page) 60%, transparent) 55%, transparent 100%);
  animation: toolRowSweep 2.6s ease-out infinite; pointer-events: none;
}
@keyframes toolRowSweep { 0% { left: -300px; } 90%, 100% { left: 100%; } }
@media (prefers-reduced-motion: reduce) {
  .tool-row-wrap[data-state='running'] .tool-row::after { display: none; }
  .state-dot.running::after { animation: none; }
}

.tool-args { padding: 8px 12px; margin: 2px 8px 6px; background: var(--bg-secondary); border: 1px solid var(--border-card); border-radius: var(--sig-radius-button); }
.tool-args pre { margin: 0; font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary); white-space: pre-wrap; word-break: break-all; }
@media (max-width: 768px) { .tool-row-wrap { padding: 0 16px; } }
/* 轻量展开动画（≤200ms） */
.expand-enter-active, .expand-leave-active { transition: opacity 0.15s ease, transform 0.15s ease; overflow: hidden; }
.expand-enter-from, .expand-leave-to { opacity: 0; transform: translateY(-4px); }
@media (prefers-reduced-motion: reduce) { .expand-enter-active, .expand-leave-active { transition: none; } }
@media (max-width: 768px) { .tool-main { min-height: 36px; } } /* 触控目标放大 */
@media (max-width: 576px) {
  .tool-row-wrap { padding: 0 12px; }
  .tool-args { margin: 2px 4px 6px; }
}
</style>
