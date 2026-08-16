<script setup lang="ts">
import { ref } from 'vue'
import { Button, Input, Popconfirm, Avatar } from 'ant-design-vue'
import { PlusOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { formatRelativeTime } from './chat-types'
import type { ChatSession } from './chat-types'

const props = defineProps<{
  sessions: ChatSession[]
  activeSessionId: string
  collapsed: boolean
  userName?: string
}>()

const emit = defineEmits<{
  (e: 'create'): void
  (e: 'switch', id: string): void
  (e: 'delete', id: string): void
  (e: 'close'): void
}>()

const searchQuery = ref('')

function filtered(): ChatSession[] {
  const q = searchQuery.value.trim().toLowerCase()
  const all = props.sessions || []
  if (!q) return all
  return all.filter(s => (s.title || '').toLowerCase().includes(q))
}
</script>

<template>
  <div class="chat-sidebar">
    <div class="sidebar-header">
      <div class="sidebar-logo">
        <span class="logo-mark">MC</span>
        <span v-if="!collapsed" class="logo-text">MiniCC</span>
      </div>
      <Button v-if="!collapsed" block type="primary" size="small" @click="emit('create')">
        <template #icon><PlusOutlined /></template>
        新对话
      </Button>
      <Input v-if="!collapsed" v-model:value="searchQuery" placeholder="搜索会话" allow-clear size="small">
        <template #prefix><SearchOutlined /></template>
      </Input>
    </div>

    <div class="sidebar-content">
      <div v-if="filtered().length === 0" class="session-empty">暂无对话</div>
      <div
        v-for="s in filtered()"
        :key="s.id"
        class="session-item"
        :class="{ active: s.id === activeSessionId }"
        @click="emit('switch', s.id)"
      >
        <div class="session-info">
          <span class="session-title">{{ s.title || '新对话' }}</span>
          <span class="session-time">{{ formatRelativeTime(s.updated_at || s.created_at) }}</span>
        </div>
        <Popconfirm title="删除此对话？" @confirm="emit('delete', s.id)">
          <Button type="text" size="small" class="session-delete-btn" @click.stop>
            <template #icon><DeleteOutlined /></template>
          </Button>
        </Popconfirm>
      </div>
    </div>

    <div class="sidebar-footer">
      <Avatar :size="24" :style="{ backgroundColor: 'var(--primary)' }">
        {{ (userName || 'U').charAt(0).toUpperCase() }}
      </Avatar>
      <span v-if="!collapsed" class="user-name">{{ userName || '用户' }}</span>
    </div>

    <!-- 移动端关闭遮罩按钮 -->
    <Button v-if="!collapsed" class="sidebar-close" type="text" size="small" @click="emit('close')">✕</Button>
  </div>
</template>

<style scoped>
.chat-sidebar { width: 260px; flex-shrink: 0; display: flex; flex-direction: column; background: var(--bg-card); border-right: 1px solid var(--border); }
.sidebar-header { padding: 14px 12px; display: flex; flex-direction: column; gap: 8px; border-bottom: 1px solid var(--border); }
.sidebar-logo { display: flex; align-items: center; gap: 8px; padding: 2px 4px 4px; }
.logo-mark { width: 24px; height: 24px; border-radius: 7px; background: linear-gradient(135deg, var(--primary), var(--primary-dark)); color: #fff; font-weight: 700; font-size: 11px; display: inline-flex; align-items: center; justify-content: center; }
.logo-text { font-size: 15px; font-weight: 600; color: var(--text-primary); letter-spacing: -0.01em; }
.sidebar-content { flex: 1; overflow-y: auto; padding: 6px; }
.session-empty { padding: 20px 8px; text-align: center; color: var(--text-muted); font-size: 13px; }
.session-item { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-radius: var(--radius-md); cursor: pointer; transition: background 0.15s; margin-bottom: 2px; }
.session-item:hover { background: var(--bg-hover); }
.session-item.active { background: var(--primary-bg); }
.session-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.session-title { font-size: 13px; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.session-item.active .session-title { color: var(--primary); font-weight: 600; }
.session-time { font-size: 11px; color: var(--text-muted); }
.session-delete-btn { opacity: 0; flex-shrink: 0; color: var(--text-muted); }
.session-item:hover .session-delete-btn { opacity: 1; }
.sidebar-footer { padding: 12px 14px; display: flex; align-items: center; gap: 8px; border-top: 1px solid var(--border); }
.user-name { font-size: 13px; color: var(--text-secondary); }
.sidebar-close { display: none; }
@media (max-width: 768px) {
  .sidebar-close { display: inline-flex; position: absolute; top: 8px; right: 8px; }
  .chat-sidebar { position: relative; }
}
</style>
