<script setup lang="ts">
import { computed, ref } from 'vue'
import { SearchOutlined, CloseOutlined } from '@ant-design/icons-vue'
import type { ChatItem } from './chat-types'

const props = defineProps<{
  items: ChatItem[]
  selectedIndex: number | null   // 当前聚焦的用户消息索引
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'focus', index: number): void
  (e: 'close'): void
}>()

const searchQuery = ref('')
const hoveredIndex = ref<number | null>(null)

// 仅用户提问作为锚点（deepseek span[data-timeline-span=user]）
const userIndexes = computed(() =>
  props.items
    .map((it, i) => ({ it, i }))
    .filter(x => x.it.kind === 'text' && x.it.role === 'user')
    .map(x => x.i),
)

const filteredIndexes = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return userIndexes.value
  return userIndexes.value.filter(i => {
    const it = props.items[i]
    return it.kind === 'text' && it.content.toLowerCase().includes(q)
  })
})

// 用户提问摘要（截断）
function summary(index: number): string {
  const it = props.items[index]
  if (it.kind !== 'text') return ''
  const s = it.content.replace(/\s+/g, ' ').trim()
  return s.length > 40 ? s.slice(0, 40) + '…' : s
}
</script>

<template>
  <div class="trajectory" :class="{ open }">
    <!-- 工具栏：搜索（deepseek TrajectoryToolbar 32px） -->
    <div class="trajectory-toolbar">
      <span class="toolbar-title">轨迹</span>
      <div class="toolbar-search">
        <SearchOutlined class="search-icon" />
        <input v-model="searchQuery" class="search-input" placeholder="搜索提问" />
        <CloseOutlined v-if="searchQuery" class="search-clear" @click="searchQuery = ''" />
      </div>
    </div>

    <!-- 时间线（deepseek TrajectoryTimeline：44px 标签列 + 50px 轨道 + 8px spans） -->
    <div class="timeline">
      <div class="timeline-labels">
        <span>0</span><span>50</span><span>100</span>
      </div>
      <div class="timeline-track" :data-panning="false">
        <div
          v-for="i in filteredIndexes"
          :key="i"
          class="timeline-span"
          :class="{ selected: i === selectedIndex }"
          :data-selected="i === selectedIndex ? undefined : (selectedIndex === null ? undefined : 'false')"
          :data-hovered="i === hoveredIndex"
          :data-current="i === selectedIndex"
          :style="{
            left: `calc(${((userIndexes.indexOf(i)) / Math.max(userIndexes.length, 1)) * 100}% + 4px)`,
            width: `calc(${100 / Math.max(userIndexes.length, 1)}% - 8px)`,
          }"
          :title="summary(i)"
          @click.stop="emit('focus', i)"
          @mouseenter="hoveredIndex = i"
          @mouseleave="hoveredIndex = null"
        />
        <div v-if="filteredIndexes.length === 0" class="timeline-empty">无提问</div>
      </div>
    </div>

    <!-- 锚点列表（用户提问） -->
    <div class="trajectory-list">
      <div
        v-for="i in filteredIndexes"
        :key="i"
        class="trajectory-row"
        :class="{ active: i === selectedIndex }"
        @click="emit('focus', i)"
      >
        <span class="row-dot" aria-hidden />
        <span class="row-text">{{ summary(i) }}</span>
      </div>
      <div v-if="filteredIndexes.length === 0" class="list-empty">当前会话暂无提问</div>
    </div>
  </div>
</template>

<style scoped>
/* 右侧悬浮面板（deepseek TrajectoryView：bg-layer-1、flex column、100% 高） */
.trajectory {
  position: absolute;
  top: 0; right: 0; bottom: 0;
  width: 280px;
  z-index: 30;
  display: flex; flex-direction: column;
  background: var(--bg-card);
  border-left: 1px solid var(--border);
  box-shadow: var(--shadow-lg);
  transform: translateX(100%);
  transition: transform 0.25s ease;
}
.trajectory.open { transform: translateX(0); }

/* 工具栏（deepseek TrajectoryToolbar：32px） */
.trajectory-toolbar {
  flex: none; display: flex; align-items: center; gap: 8px;
  height: 32px; padding: 0 10px;
  border-bottom: 1px solid var(--border);
}
.toolbar-title { font-size: 13px; font-weight: 600; color: var(--text-primary); }
.toolbar-search { flex: 1; display: flex; align-items: center; gap: 4px; padding: 0 6px; background: var(--bg-secondary); border-radius: 6px; }
.search-icon { font-size: 11px; color: var(--text-tertiary); }
.search-input { flex: 1; border: none; outline: none; background: none; font-size: 12px; color: var(--text-primary); }
.search-clear { font-size: 10px; color: var(--text-tertiary); cursor: pointer; }

/* 时间线（deepseek TrajectoryTimeline：44px 标签 + 50px 轨道） */
.timeline {
  flex: none;
  display: grid; grid-template-columns: 44px minmax(0, 1fr);
  height: 50px; overflow: hidden;
  border-bottom: 1px solid var(--border);
  background: var(--bg-secondary);
  user-select: none;
}
.timeline-labels { position: relative; border-right: 1px solid var(--border); color: var(--text-tertiary); font-size: 10px; line-height: 1; }
.timeline-labels span { position: absolute; right: 3px; height: 8px; display: flex; align-items: center; }
.timeline-labels span:nth-child(1) { top: 7px; }
.timeline-labels span:nth-child(2) { top: 21px; }
.timeline-labels span:nth-child(3) { top: 35px; }
.timeline-track { position: relative; overflow: hidden; cursor: crosshair; }
/* span：8px 高、1px 圆角、品牌蓝（deepseek span[data-timeline-span=user]） */
.timeline-span {
  position: absolute; top: 21px; height: 8px; min-width: 2px;
  border-radius: 1px;
  background: var(--primary);
  opacity: 0.78;
  transition: opacity 0.15s ease;
}
.timeline-span[data-hovered='true']:not([data-current='true']) {
  opacity: 1;
  box-shadow: 0 0 0 1px var(--bg-secondary), 0 0 0 2px color-mix(in srgb, var(--primary) 80%, transparent);
}
.timeline-span[data-selected='false'] { opacity: 0.2; }
.timeline-span[data-current='true'] { opacity: 1; }
.timeline-empty { position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); color: var(--text-tertiary); font-size: 12px; }

/* 锚点列表 */
.trajectory-list { flex: 1; overflow-y: auto; padding: 6px; }
.trajectory-row { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; cursor: pointer; }
.trajectory-row:hover { background: var(--bg-hover); }
.trajectory-row.active { background: var(--primary-bg); }
.row-dot { flex: none; width: 6px; height: 6px; border-radius: 50%; background: var(--primary); }
.row-text { flex: 1; min-width: 0; font-size: 12px; color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.trajectory-row.active .row-text { color: var(--primary); font-weight: 600; }
.list-empty { padding: 20px 8px; text-align: center; color: var(--text-muted); font-size: 12px; }
</style>
