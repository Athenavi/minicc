<script setup lang="ts">
import { ref, computed } from 'vue'
import { CaretRightOutlined } from '@ant-design/icons-vue'

const props = defineProps<{ content: string; streaming?: boolean }>()
const expanded = ref(false)

// 折叠摘要：running 跟随最新行，完成显示首行（deepseek ReasoningRow 语义）
const summary = computed(() => {
  const text = props.content.trimEnd()
  if (props.streaming) {
    const nl = text.lastIndexOf('\n')
    return nl === -1 ? text : text.slice(nl + 1)
  }
  const nl = text.indexOf('\n')
  return nl === -1 ? text : text.slice(0, nl)
})
</script>

<template>
  <div class="reasoning-row" :data-state="streaming ? 'running' : 'ok'">
    <button class="reasoning-main" type="button" @click="expanded = !expanded">
      <CaretRightOutlined class="chevron" :class="{ open: expanded }" />
      <span class="think-icon" aria-hidden>💭</span>
      <span class="think-label">Think</span>
      <span class="sep" aria-hidden />
      <span class="state-dot" :class="streaming ? 'running' : 'done'" aria-hidden />
      <span class="think-summary">{{ summary }}</span>
    </button>
    <Transition name="expand">
      <div v-if="expanded" class="think-body">{{ content }}</div>
    </Transition>
  </div>
</template>

<style scoped>
.reasoning-row { max-width: min(720px, 92%); margin: 4px auto 0; padding: 0 24px; }
.reasoning-main { display: flex; align-items: center; gap: 6px; width: 100%; height: 28px; padding: 0 8px; border: none; background: none; color: var(--text-secondary); cursor: pointer; font-size: 13px; text-align: left; border-radius: var(--sig-radius-button); }
.reasoning-main:hover { background: var(--bg-hover); }
.chevron { font-size: 10px; color: var(--text-muted); transition: transform 0.2s; flex-shrink: 0; }
.chevron.open { transform: rotate(90deg); }
.think-icon { font-size: 12px; flex-shrink: 0; }
.think-label { font-weight: 600; color: var(--text-primary); font-size: 13px; }
.sep { flex: none; width: 2px; height: 2px; border-radius: 1px; background: var(--text-muted); margin: 0 6px; }
.state-dot { position: relative; flex: none; width: 10px; height: 10px; }
.state-dot::before { content: ''; position: absolute; inset: 0; border-radius: 50%; background: currentColor; opacity: 0.12; }
.state-dot::after { content: ''; position: absolute; inset: 20%; border-radius: 50%; background: currentColor; }
.state-dot.running { color: var(--primary); }
.state-dot.running::after { animation: dotPulse 1.4s ease-in-out infinite; }
.state-dot.done { color: var(--success); }
@keyframes dotPulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }
.think-summary { flex: 1; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; color: var(--text-tertiary); font-size: 12px; text-align: left; }
.reasoning-row[data-state='running'] .think-summary { color: var(--primary); }
.think-body { padding: 10px 12px; font-size: 13px; line-height: 1.7; color: var(--text-secondary); background: var(--bg-secondary); border: 1px solid var(--border-card); border-radius: var(--sig-radius-button); margin-top: 2px; white-space: pre-wrap; }
@media (prefers-reduced-motion: reduce) { .state-dot.running::after { animation: none; } }
/* 轻量展开动画（≤200ms） */
.expand-enter-active, .expand-leave-active { transition: opacity 0.15s ease, transform 0.15s ease; overflow: hidden; }
.expand-enter-from, .expand-leave-to { opacity: 0; transform: translateY(-4px); }
@media (prefers-reduced-motion: reduce) { .expand-enter-active, .expand-leave-active { transition: none; } }
@media (max-width: 768px) {
  .reasoning-row { padding: 0 16px; }
  .reasoning-main { min-height: 36px; } /* 触控目标放大 */
}
@media (max-width: 576px) { .reasoning-row { padding: 0 12px; } }
</style>
