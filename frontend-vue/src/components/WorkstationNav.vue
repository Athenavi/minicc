<template>
  <div class="workstation-nav">
    <div class="nav-header">
      <div class="brand">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2"/>
          <path d="M12 6v6l4 2" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
        <span class="brand-text">MiniCC 工作台</span>
      </div>

      <!-- 快捷命令输入框（子组件 QuickCommandBar：输入 + loading，emit submit） -->
      <QuickCommandBar ref="quickBarRef" @submit="handleQuickCommand" />
    </div>

    <!-- 工作台网格 -->
    <div class="workstation-grid">
      <div
        v-for="ws in workstations"
        :key="ws.id"
        class="workstation-card"
        :class="{ active: activeWorkstation === ws.id }"
        @click="switchWorkstation(ws.id)"
      >
        <div class="ws-icon" :style="{ backgroundColor: ws.color }">
          <component :is="iconMap[ws.icon] || MessageOutlined" />
        </div>
        <div class="ws-info">
          <div class="ws-name">{{ ws.name }}</div>
          <div class="ws-desc">{{ ws.description }}</div>
        </div>
        <div v-if="ws.badge" class="ws-badge">{{ ws.badge }}</div>
      </div>
    </div>

    <!-- 最近活动（子组件 RecentActivities：props activities，emit select） -->
    <RecentActivities
      v-if="showRecentActivity"
      :activities="recentActivities"
      @select="viewActivityDetail"
    />
  </div>
</template>

<!-- 普通 script 块：与 <script setup> 共享模块作用域。
     导出 executeQuickCommand 供全局停靠坞（AppLayout）复用同一套
     快速命令执行逻辑：创建 uni 会话 → /v1/quick-execute → 跳转 /chat?task= -->
<script lang="ts">
import { api } from '../api'
import router from '../router'

export interface QuickCommandRunResult {
  sessionId: string
  title: string
}

/** 快速命令统一执行逻辑（WorkstationNav 与停靠坞弹层共用）：
 *  创建 uni 会话 → 调用 /v1/quick-execute → 跳转聊天页展示结果，可继续追问 */
export async function executeQuickCommand(command: string): Promise<QuickCommandRunResult> {
  const sessionId = `uni_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`
  const title = command.substring(0, 50)
  try {
    const response = await api.post('/v1/quick-execute', {
      user_input: command,
      mode: 'auto',
      session_id: sessionId,
    })
    if (response.data?.success) {
      await router.push({ path: '/chat', query: { task: sessionId } })
      return { sessionId, title }
    }
    await router.push({ path: '/chat', query: { task: sessionId, error: response.data?.error || 'execution failed' } })
    return { sessionId, title }
  } catch (error: any) {
    console.error('Command execution error:', error)
    await router.push({ path: '/chat', query: { task: '', error: error?.message || 'request failed' } })
    throw error
  }
}
</script>

<script setup lang="ts">
import { defineComponent, h, onMounted, onUnmounted, ref, type PropType } from 'vue'
import { MessageOutlined, RobotOutlined, ApartmentOutlined, ThunderboltOutlined, BookOutlined, AppstoreOutlined } from '@ant-design/icons-vue'

// api / router 由上方普通 <script> 块提供（同模块作用域，避免重复 import 造成重名）

// 定义事件
const emit = defineEmits<{
  workstationSwitch: [workstationId: string]
  commandSubmit: [command: string]
}>()

// ── 最近活动数据模型 ──
interface ActivityItem {
  id: string
  title: string
  wsName: string
  wsColor: string
  route?: string
  status: string
  statusText: string
  timestamp: number
}

// ── 子组件：QuickCommandBar（props: 无；emit submit；内部管理输入与 loading） ──
const QuickCommandBar = defineComponent({
  name: 'QuickCommandBar',
  emits: ['submit'],
  setup(_, { emit: barEmit, expose }) {
    const commandInput = ref('')
    const isSubmitting = ref(false)

    function submit() {
      const command = commandInput.value.trim()
      if (!command || isSubmitting.value) return
      isSubmitting.value = true
      commandInput.value = ''
      barEmit('submit', command)
    }

    function clear() {
      commandInput.value = ''
    }

    // 供父组件在提交流程（API 请求 → 跳转）结束后复位 loading
    function setSubmitting(v: boolean) {
      isSubmitting.value = v
    }
    expose({ setSubmitting })

    return () => h('div', { class: 'quick-command-bar' }, [
      h('svg', { class: 'command-icon', width: 16, height: 16, viewBox: '0 0 16 16', fill: 'currentColor' }, [
        h('path', { d: 'M8 0a8 8 0 100 16A8 8 0 008 0zm1 11H7V7h2v4zm0-6H7V3h2v2z' }),
      ]),
      h('input', {
        class: 'command-input',
        type: 'text',
        placeholder: "输入自然语言命令,例如: '帮我分析 sales.csv 并生成报告'",
        disabled: isSubmitting.value,
        value: commandInput.value,
        onInput: (e: Event) => { commandInput.value = (e.target as HTMLInputElement).value },
        onKeydown: (e: KeyboardEvent) => {
          if (e.key === 'Enter') submit()
          else if (e.key === 'Escape') clear()
        },
      }),
      h('button', {
        class: 'command-submit',
        type: 'button',
        disabled: isSubmitting.value,
        onClick: submit,
      }, isSubmitting.value
        ? [h('span', { class: 'loading-spinner' })]
        : '执行'),
    ])
  },
})

// ── 子组件：RecentActivities（props: activities；emit select；空态"暂无活动"） ──
const RecentActivities = defineComponent({
  name: 'RecentActivities',
  props: {
    activities: { type: Array as PropType<ActivityItem[]>, required: true },
  },
  emits: ['select'],
  setup(props, { emit: actEmit }) {
    function formatTime(timestamp: number): string {
      const diff = Date.now() - timestamp
      const minutes = Math.floor(diff / 60000)
      if (minutes < 1) return '刚刚'
      if (minutes < 60) return `${minutes} 分钟前`
      const hours = Math.floor(minutes / 60)
      if (hours < 24) return `${hours} 小时前`
      const days = Math.floor(hours / 24)
      return `${days} 天前`
    }

    return () => h('div', { class: 'recent-activity' }, [
      h('div', { class: 'activity-header' }, '最近活动'),
      props.activities.length === 0
        ? h('div', { class: 'activity-empty' }, '暂无活动')
        : props.activities.map((a) => h('div', {
            class: 'activity-item',
            onClick: () => actEmit('select', a),
          }, [
            h('div', { class: 'activity-workstation', style: { borderColor: a.wsColor } }, a.wsName.charAt(0)),
            h('div', { class: 'activity-content' }, [
              h('div', { class: 'activity-title' }, a.title),
              h('div', { class: 'activity-time' }, formatTime(a.timestamp)),
            ]),
            h('div', { class: ['activity-status', a.status] }, a.statusText),
          ])),
    ])
  },
})

// 工作台配置
interface Workstation {
  id: string
  name: string
  description: string
  icon: string
  color: string
  route: string
  badge?: string
}

const workstations: Workstation[] = [
  {
    id: 'dialogue',
    name: '对话',
    description: '智能对话助手',
    icon: 'ChatIcon',
    color: '#10b981',
    route: '/chat',
  },
  {
    id: 'agent',
    name: 'Agent',
    description: '多智能体协同',
    icon: 'AgentIcon',
    color: '#6366f1',
    route: '/agents',
    badge: 'AI',
  },
  {
    id: 'workflow',
    name: '工作流',
    description: 'DAG 流程编排',
    icon: 'WorkflowIcon',
    color: '#f59e0b',
    route: '/workflow',
  },
  {
    id: 'skill',
    name: '技能',
    description: '工具与 MCP',
    icon: 'SkillIcon',
    color: '#ef4444',
    route: '/skills',
  },
  {
    id: 'knowledge',
    name: '知识库',
    description: 'RAG 检索增强',
    icon: 'KnowledgeIcon',
    color: '#8b5cf6',
    route: '/knowledge',
  },
  {
    id: 'plugin',
    name: '插件',
    description: '扩展能力',
    icon: 'PluginIcon',
    color: '#06b6d4',
    route: '/plugins',
  },
]

// 注册图标组件（原 ./icons/*.vue 组件不存在，改用 antd 图标映射）
const iconMap: Record<string, any> = {
  ChatIcon: MessageOutlined,
  AgentIcon: RobotOutlined,
  WorkflowIcon: ApartmentOutlined,
  SkillIcon: ThunderboltOutlined,
  KnowledgeIcon: BookOutlined,
  PluginIcon: AppstoreOutlined,
}

const activeWorkstation = ref('dialogue')
const showRecentActivity = ref(true)
const recentActivities = ref<ActivityItem[]>([])
const quickBarRef = ref<{ setSubmitting(v: boolean): void } | null>(null)

// 切换到工作台
function switchWorkstation(workstationId: string) {
  activeWorkstation.value = workstationId
  emit('workstationSwitch', workstationId)

  // 路由跳转
  const ws = workstations.find(w => w.id === workstationId)
  if (ws) {
    // 修复：createWebHistory 模式下 location.hash 不会触发路由，改用 router.push
    router.push(ws.route)
  }
}

// 快速命令提交：复用 executeQuickCommand（创建 uni 会话 → 跳转 /chat?task=）
async function handleQuickCommand(command: string) {
  emit('commandSubmit', command)
  try {
    const result = await executeQuickCommand(command)
    // 添加到最近活动
    addRecentActivity({
      id: `act_${Date.now()}`,
      title: result.title,
      wsName: '对话',
      wsColor: '#10b981',
      route: '/chat',
      status: 'completed',
      statusText: '完成',
      timestamp: Date.now(),
    })
  } catch {
    // executeQuickCommand 已携带 error query 跳转 /chat，无需额外处理
  } finally {
    // 提交后跳转前的 loading 态：由父组件在流程结束后复位
    quickBarRef.value?.setSubmitting(false)
  }
}

// 添加最近活动
function addRecentActivity(activity: ActivityItem) {
  recentActivities.value.unshift(activity)
  if (recentActivities.value.length > 10) {
    recentActivities.value.pop()
  }
}

// 查看活动详情：跳转到活动关联的工作台路由
// 活动带 route（如快捷命令产生的活动）直接跳；否则按 wsName 匹配工作台；兜底 /chat
function viewActivityDetail(activity: ActivityItem) {
  const route =
    activity?.route ||
    workstations.find(w => w.name === activity?.wsName)?.route ||
    '/chat'
  router.push(route)
}

// 从 API 加载最近活动（/v1/activities；30s 轮询，卸载时停止）
let activityTimer: number | undefined
let loadingActivities = false

async function loadRecentActivities() {
  if (loadingActivities) return
  loadingActivities = true
  try {
    const response = await api.get('/v1/activities?limit=10')
    const list = response.data?.activities || []
    recentActivities.value = list.map((a: any): ActivityItem => {
      const ws = workstations.find(w => w.id === a.workstation)
      return {
        id: `${a.workstation}_${a.timestamp}`,
        title: a.title || '暂无标题',
        wsName: ws?.name || '对话',
        wsColor: ws?.color || '#10b981',
        route: a.route || ws?.route || '/chat',
        status: a.status || '',
        statusText: a.status_text || '',
        timestamp: a.timestamp || Date.now(),
      }
    })
  } catch (error) {
    console.warn('Failed to load recent activities:', error)
  } finally {
    loadingActivities = false
  }
}

onMounted(() => {
  loadRecentActivities()
  activityTimer = window.setInterval(loadRecentActivities, 30_000)
})

onUnmounted(() => {
  if (activityTimer !== undefined) window.clearInterval(activityTimer)
  activityTimer = undefined
})
</script>

<style scoped>
.workstation-nav {
  background: var(--bg-page);
  border-bottom: 1px solid var(--border);
}

.nav-header {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border);
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
}

/* ── QuickCommandBar 子组件样式（:deep 穿透运行时子组件） ── */
:deep(.quick-command-bar) {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 600px;
  margin: 0 auto;
}

:deep(.command-icon) {
  color: var(--text-secondary);
  flex-shrink: 0;
}

:deep(.command-input) {
  flex: 1;
  min-width: 0;
  min-height: 40px;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 14px;
  outline: none;
  transition: all 0.2s ease;
}

:deep(.command-input:focus) {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px var(--primary-bg);
}

:deep(.command-input::placeholder) {
  color: var(--text-muted);
}

:deep(.command-submit) {
  min-height: 40px;
  padding: 8px 18px;
  border: none;
  border-radius: 8px;
  background: var(--primary);
  color: #fff;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  white-space: nowrap;
}

:deep(.command-submit:hover:not(:disabled)) {
  background: var(--primary-dark);
}

:deep(.command-submit:disabled) {
  opacity: 0.6;
  cursor: not-allowed;
}

:deep(.loading-spinner) {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.6s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.workstation-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
  padding: 16px;
}

.workstation-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--bg-card);
  cursor: pointer;
  transition: all 0.2s ease;
  position: relative;
}

.workstation-card:hover {
  border-color: var(--primary);
  background: var(--bg-hover);
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
}

.workstation-card.active {
  border-color: var(--primary);
  background: linear-gradient(135deg, var(--primary-bg), transparent);
}

.ws-icon {
  width: 40px;
  height: 40px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  flex-shrink: 0;
}

.ws-info {
  flex: 1;
  min-width: 0;
}

.ws-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 2px;
}

.ws-desc {
  font-size: 12px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ws-badge {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--primary);
  color: #fff;
  font-size: 10px;
  font-weight: 600;
}

/* ── RecentActivities 子组件样式（:deep 穿透运行时子组件；状态色走语义令牌） ── */
:deep(.recent-activity) {
  border-top: 1px solid var(--border);
  padding: 16px;
}

:deep(.activity-header) {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
}

:deep(.activity-empty) {
  padding: 16px 8px;
  text-align: center;
  font-size: 13px;
  color: var(--text-muted);
}

:deep(.activity-item) {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s ease;
}

:deep(.activity-item:hover) {
  background: var(--bg-hover);
}

:deep(.activity-workstation) {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: 2px solid;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  flex-shrink: 0;
}

:deep(.activity-content) {
  flex: 1;
  min-width: 0;
}

:deep(.activity-title) {
  font-size: 13px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.activity-time) {
  font-size: 11px;
  color: var(--text-secondary);
}

:deep(.activity-status) {
  font-size: 12px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 4px;
}

:deep(.activity-status.completed) {
  color: var(--success);
  background: rgba(34, 197, 94, 0.12);
}

:deep(.activity-status.failed) {
  color: var(--error);
  background: rgba(239, 68, 68, 0.12);
}

:deep(.activity-status.running) {
  color: var(--warning);
  background: rgba(245, 158, 11, 0.12);
}

@media (max-width: 768px) {
  .nav-header {
    flex-direction: column;
    align-items: stretch;
  }

  :deep(.quick-command-bar) {
    order: -1;
  }

  .workstation-grid {
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 8px;
  }

  .workstation-card {
    padding: 8px;
  }
}

@media (max-width: 576px) {
  .nav-header {
    padding: 10px 12px;
    gap: 10px;
  }

  .workstation-grid {
    grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
    padding: 12px;
  }

  :deep(.command-input) {
    font-size: 13px;
  }

  :deep(.activity-item) {
    padding: 10px 4px;
  }
}

@media (prefers-reduced-motion: reduce) {
  :deep(.command-input),
  :deep(.command-submit),
  .workstation-card,
  :deep(.activity-item) {
    transition: none;
  }
}
</style>
