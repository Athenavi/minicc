<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { ArrowDownOutlined } from '@ant-design/icons-vue'
import MessageItem from './MessageItem.vue'
import type { ChatItem } from './chat-types'

const props = defineProps<{
  items: ChatItem[]
  loading: boolean
  focusIndex?: number | null   // 跳转目标（用户消息索引）
  focusToken?: number          // 递增触发跳转
  hasMore?: boolean            // P 性能：是否还有更早的消息
  loadingEarlier?: boolean     // P 性能：正在加载更早消息
  /** P2-E: 首次加载会话历史时的骨架屏 */
  initialLoading?: boolean
}>()

const emit = defineEmits<{
  (e: 'load-earlier'): void
  /** P1-1 用户消息编辑后重发 */
  (e: 'retry-from', itemId: string, text: string): void
  /** P1-1 助手消息重新生成 */
  (e: 'regenerate', itemId: string): void
  /** P2-F 停止后继续生成 */
  (e: 'continue', itemId: string): void
  /** P1-3 失败消息重试 */
  (e: 'retry-failed', itemId: string): void
}>()

const scrollRef = ref<HTMLDivElement | null>(null)
const stickToBottom = ref(true)
const highlightIndex = ref<number | null>(null)
const showBackToBottom = ref(false)
const unseenCount = ref(0)

// deepseek bottom-follow：离开底部 120px 视为停止跟随
const SCROLL_THRESHOLD = 120
// P 性能：距顶部 60px 内触发加载更早
const TOP_LOAD_THRESHOLD = 60

// ── 虚拟滚动（P 性能：只渲染可视区 ± overscan，千条消息 DOM 从数千节点 → 几十节点）──
// 注意：count 必须是数字（virtual-core 直接参与运算，ComputedRef 对象会算出 NaN 崩溃）；
// options 包 computed → items 变化时重算 count 并触发 setOptions（响应式正确用法）。
const virtualizer = useVirtualizer(computed(() => ({
  count: props.items.length,
  getScrollElement: () => scrollRef.value,
  estimateSize: () => 72,          // 估算行高，measureElement 动态修正
  overscan: 8,
  // P 正确性：稳定 id 作为 key（loadEarlier 头部插入时旧项 key 不变，不错位不重挂载）
  getItemKey: (index) => props.items[index]?.id ?? index,
})))

function isUserAnchor(item: ChatItem | undefined): boolean {
  return !!item && item.kind === 'text' && item.role === 'user'
}

function onScroll() {
  const el = scrollRef.value
  if (!el) return
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < SCROLL_THRESHOLD
  stickToBottom.value = atBottom
  if (atBottom) {
    showBackToBottom.value = false
    unseenCount.value = 0
  } else {
    showBackToBottom.value = true
  }
  // P 性能：触顶加载更早消息（防重复触发）
  if (props.hasMore && !props.loadingEarlier && el.scrollTop <= TOP_LOAD_THRESHOLD) {
    emit('load-earlier')
  }
}

// 新消息到达：跟随底部时自动滚到底；否则累积未读数
watch(() => props.items.length, async (n, prev) => {
  if (!stickToBottom.value) {
    if (typeof prev === 'number' && n > prev) unseenCount.value += n - prev
    return
  }
  await nextTick()
  const el = scrollRef.value
  if (el) {
    virtualizer.value.scrollToOffset(el.scrollHeight, { align: 'end' })
  }
})

function scrollToBottom() {
  const el = scrollRef.value
  if (!el) return
  virtualizer.value.scrollToOffset(el.scrollHeight, { align: 'end' })
  stickToBottom.value = true
  showBackToBottom.value = false
  unseenCount.value = 0
}

// 轨迹跳转：滚动到对应用户消息 + 高亮闪烁（虚拟列表用 scrollToIndex）
watch(() => props.focusToken, async () => {
  if (props.focusIndex == null) return
  stickToBottom.value = false
  virtualizer.value.scrollToIndex(props.focusIndex, { align: 'start' })
  highlightIndex.value = props.focusIndex
  setTimeout(() => { if (highlightIndex.value === props.focusIndex) highlightIndex.value = null }, 2000)
})

const badgeText = computed(() => (unseenCount.value > 99 ? '99+' : String(unseenCount.value)))
</script>

<template>
  <div ref="scrollRef" class="message-list" @scroll.passive="onScroll">
    <!-- P2-E: 首次加载骨架屏 -->
    <div v-if="initialLoading" class="skeleton-list">
      <div v-for="n in 4" :key="n" class="skeleton-msg" :class="n % 2 === 0 ? 'user' : 'assistant'">
        <div class="skeleton-avatar"></div>
        <div class="skeleton-lines">
          <div class="skeleton-line" :style="{ width: 60 + (n * 7) % 30 + '%' }"></div>
          <div class="skeleton-line" :style="{ width: 80 + (n * 11) % 15 + '%' }"></div>
        </div>
      </div>
    </div>
    <div v-else-if="items.length === 0" class="list-empty-placeholder" />

    <!-- P 性能：触顶加载更早（infinite scroll） -->
    <div v-if="props.hasMore || props.loadingEarlier" class="earlier-loader">
      <template v-if="props.loadingEarlier">
        <span class="loading-dot"></span><span class="loading-dot"></span><span class="loading-dot"></span>
      </template>
      <span v-else>加载更早的消息</span>
    </div>

    <!-- 虚拟滚动窗口 -->
    <div class="virtual-window" :style="{ height: virtualizer.getTotalSize() + 'px', position: 'relative' }">
      <MessageItem
        v-for="vi in virtualizer.getVirtualItems()"
        :key="String(vi.key)"
        :item="items[vi.index]"
        :anchor-key="isUserAnchor(items[vi.index]) ? vi.index : undefined"
        :highlighted="highlightIndex === vi.index"
        :data-index="vi.index"
        :style="{
          position: 'absolute',
          top: '0',
          left: '0',
          right: '0',
          transform: `translateY(${vi.start}px)`,
        }"
        :ref="(el: any) => el && virtualizer.measureElement(el.$el ?? el)"
        @retry-from="(id: string, text: string) => emit('retry-from', id, text)"
        @regenerate="(id: string) => emit('regenerate', id)"
        @continue="(id: string) => emit('continue', id)"
        @retry-failed="(id: string) => emit('retry-failed', id)"
      />
    </div>

    <div v-if="loading" class="loading-indicator">
      <span class="loading-dot"></span><span class="loading-dot"></span><span class="loading-dot"></span>
    </div>

    <!-- 回到底部按钮：离开底部时显示 + 新消息未读徽标（deepseek bottom-follow 语义补充） -->
    <Transition name="back-fade">
      <button
        v-if="showBackToBottom"
        class="back-to-bottom"
        type="button"
        :title="'回到底部' + (unseenCount ? `（${unseenCount} 条新消息）` : '')"
        @click="scrollToBottom"
      >
        <ArrowDownOutlined />
        <span v-if="unseenCount" class="back-badge">{{ badgeText }}</span>
      </button>
    </Transition>
  </div>
</template>

<style scoped>
.message-list { flex: 1; overflow-y: auto; position: relative; }
.list-empty-placeholder { height: 24px; }

/* P2-E: 首屏骨架屏 */
.skeleton-list { padding: 16px 24px; }
.skeleton-msg { display: flex; gap: 12px; margin-bottom: 24px; }
.skeleton-msg.user { flex-direction: row-reverse; }
.skeleton-avatar { width: 28px; height: 28px; border-radius: 50%; background: var(--bg-hover); flex-shrink: 0; animation: skeleton-pulse 1.4s ease-in-out infinite; }
.skeleton-lines { flex: 1; display: flex; flex-direction: column; gap: 8px; max-width: 70%; }
.skeleton-msg.user .skeleton-lines { align-items: flex-end; }
.skeleton-line { height: 14px; border-radius: 4px; background: var(--bg-hover); animation: skeleton-pulse 1.4s ease-in-out infinite; }
.skeleton-line:nth-child(2) { animation-delay: 0.2s; }
@keyframes skeleton-pulse { 0%, 100% { opacity: 0.5; } 50% { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .skeleton-avatar, .skeleton-line { animation: none; } }
/* ── 移动端：骨架屏间距/头像压缩 ── */
@media (max-width: 768px) {
  .skeleton-list { padding: 12px 16px; }
  .skeleton-msg { gap: 10px; margin-bottom: 18px; }
  .skeleton-avatar { width: 24px; height: 24px; }
  .skeleton-lines { max-width: 85%; }
}
@media (max-width: 576px) { .skeleton-list { padding: 10px 12px; } }
.virtual-window { width: 100%; }
.earlier-loader { display: flex; align-items: center; justify-content: center; gap: 6px; height: 36px; font-size: 12px; color: var(--text-tertiary); }
.loading-indicator { display: flex; justify-content: center; gap: 6px; padding: 14px 0; }
.loading-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--text-tertiary); animation: dotPulse 1.4s ease-in-out infinite; }
.loading-dot:nth-child(2) { animation-delay: 0.2s; }
.loading-dot:nth-child(3) { animation-delay: 0.4s; }
@keyframes dotPulse { 0%, 100% { opacity: 0.3; } 50% { opacity: 1; } }
.back-to-bottom { position: sticky; bottom: 16px; left: calc(50% - 22px); width: 44px; height: 44px; border-radius: 50%; border: 1px solid var(--border); background: var(--bg-card); color: var(--text-secondary); cursor: pointer; box-shadow: var(--sig-shadow-card); display: flex; align-items: center; justify-content: center; z-index: 10; }
.back-to-bottom:hover { color: var(--primary); border-color: var(--primary); }
.back-badge { position: absolute; top: -4px; right: -4px; min-width: 18px; height: 18px; padding: 0 4px; border-radius: 9px; background: var(--primary); color: #fff; font-size: 11px; line-height: 18px; text-align: center; }
.back-fade-enter-active, .back-fade-leave-active { transition: opacity 0.2s, transform 0.2s; }
.back-fade-enter-from, .back-fade-leave-to { opacity: 0; transform: translateY(8px); }
</style>
