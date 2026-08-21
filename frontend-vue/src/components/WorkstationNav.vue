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
      
      <!-- 快捷命令输入框 -->
      <div class="quick-command-bar">
        <svg class="command-icon" width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 0a8 8 0 100 16A8 8 0 008 0zm1 11H7V7h2v4zm0-6H7V3h2v2z"/>
        </svg>
        <input
          v-model="commandInput"
          type="text"
          placeholder="输入自然语言命令,例如: '帮我分析 sales.csv 并生成报告'"
          class="command-input"
          @keydown.enter="handleCommandSubmit"
          @keydown.esc="clearCommand"
        />
        <button
          class="command-submit"
          :disabled="isSubmitting"
          @click="handleCommandSubmit"
        >
          <span v-if="isSubmitting" class="loading-spinner"></span>
          <span v-else>执行</span>
        </button>
      </div>
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
    
    <!-- 最近活动 (可选) -->
    <div v-if="showRecentActivity" class="recent-activity">
      <div class="activity-header">最近活动</div>
      <div
        v-for="activity in recentActivities"
        :key="activity.id"
        class="activity-item"
        @click="viewActivityDetail(activity)"
      >
        <div class="activity-workstation" :style="{ borderColor: activity.wsColor }">
          {{ activity.wsName.charAt(0) }}
        </div>
        <div class="activity-content">
          <div class="activity-title">{{ activity.title }}</div>
          <div class="activity-time">{{ formatTime(activity.timestamp) }}</div>
        </div>
        <div class="activity-status" :class="activity.status">
          {{ activity.statusText }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api, createSSEConnection } from '../api'

// 定义事件
const emit = defineEmits<{
  workstationSwitch: [workstationId: string]
  commandSubmit: [command: string]
}>()

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
    route: '/workflows',
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

const activeWorkstation = ref('dialogue')
const commandInput = ref('')
const isSubmitting = ref(false)
const showRecentActivity = ref(true)

const recentActivities = ref<any[]>([
  // 示例数据,实际应从 API 获取
])

// 切换到工作台
function switchWorkstation(workstationId: string) {
  activeWorkstation.value = workstationId
  emit('workstationSwitch', workstationId)
  
  // 路由跳转
  const ws = workstations.find(w => w.id === workstationId)
  if (ws) {
    window.location.hash = ws.route
  }
}

// 提交快捷命令
async function handleCommandSubmit() {
  const command = commandInput.value.trim()
  if (!command || isSubmitting.value) return
  
  isSubmitting.value = true
  emit('commandSubmit', command)
  
  try {
    // 调用 /v1/quick-execute API
    const response = await api.post('/quick-execute', {
      user_input: command,
      mode: 'auto',
    })
    
    if (response.data?.success) {
      // 显示结果 (可集成到聊天界面)
      console.log('Quick execute result:', response.data)
      
      // 添加到最近活动
      addRecentActivity({
        id: `act_${Date.now()}`,
        title: command.substring(0, 50),
        wsName: '对话',
        wsColor: '#10b981',
        status: 'completed',
        statusText: '完成',
        timestamp: Date.now(),
      })
    } else {
      console.error('Quick execute failed:', response.data)
    }
    
    // 清空输入框
    commandInput.value = ''
    
  } catch (error: any) {
    console.error('Command execution error:', error)
  } finally {
    isSubmitting.value = false
  }
}

// 添加最近活动
function addRecentActivity(activity: any) {
  recentActivities.value.unshift(activity)
  if (recentActivities.value.length > 10) {
    recentActivities.value.pop()
  }
}

// 格式化时间
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

// 清除命令
function clearCommand() {
  commandInput.value = ''
}

// 查看活动详情
function viewActivityDetail(activity: any) {
  console.log('View activity detail:', activity)
  // TODO: 打开详情面板或跳转到相关页面
}

// 注册图标组件（原 ./icons/*..vue 组件不存在，改用 antd 图标映射）
import { MessageOutlined, RobotOutlined, ApartmentOutlined, ThunderboltOutlined, BookOutlined, AppstoreOutlined } from '@ant-design/icons-vue'

const iconMap: Record<string, any> = {
  ChatIcon: MessageOutlined,
  AgentIcon: RobotOutlined,
  WorkflowIcon: ApartmentOutlined,
  SkillIcon: ThunderboltOutlined,
  KnowledgeIcon: BookOutlined,
  PluginIcon: AppstoreOutlined,
}

onMounted(() => {
  // 加载最近活动
  loadRecentActivities()
})

// 从 API 加载最近活动
async function loadRecentActivities() {
  try {
    const response = await api.get('/activities?limit=5')
    if (response.data?.success) {
      recentActivities.value = response.data.data || []
    }
  } catch (error) {
    console.warn('Failed to load recent activities:', error)
  }
}
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

.quick-command-bar {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 600px;
  margin: 0 auto;
}

.command-icon {
  color: var(--text-secondary);
  flex-shrink: 0;
}

.command-input {
  flex: 1;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 14px;
  outline: none;
  transition: all 0.2s ease;
}

.command-input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.1);
}

.command-submit {
  padding: 8px 16px;
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

.command-submit:hover:not(:disabled) {
  background: var(--primary-dark);
}

.command-submit:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.loading-spinner {
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
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.workstation-card.active {
  border-color: var(--primary);
  background: linear-gradient(135deg, rgba(16, 185, 129, 0.1), transparent);
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

.recent-activity {
  border-top: 1px solid var(--border);
  padding: 16px;
}

.activity-header {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 12px;
}

.activity-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s ease;
}

.activity-item:hover {
  background: var(--bg-hover);
}

.activity-workstation {
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

.activity-content {
  flex: 1;
  min-width: 0;
}

.activity-title {
  font-size: 13px;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.activity-time {
  font-size: 11px;
  color: var(--text-secondary);
}

.activity-status {
  font-size: 12px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 4px;
}

.activity-status.completed {
  color: var(--primary);
  background: rgba(16, 185, 129, 0.1);
}

.activity-status.failed {
  color: var(--danger);
  background: rgba(239, 68, 68, 0.1);
}

.activity-status.running {
  color: var(--warning);
  background: rgba(245, 158, 11, 0.1);
}

@media (max-width: 768px) {
  .nav-header {
    flex-direction: column;
    align-items: stretch;
  }
  
  .quick-command-bar {
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
</style>
