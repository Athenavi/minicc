<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Button, Spin, Empty, message, Tag } from 'ant-design-vue'
import { UserOutlined, PlayCircleOutlined, ThunderboltOutlined } from '@ant-design/icons-vue'
import { api } from '../api'

interface AgentItem {
  type: string
  name: string
  description: string
}

const agents = ref<AgentItem[]>([])
const loading = ref(true)

const agentColors: Record<string, string> = {
  code: '#8b5cf6',
  knowledge: '#22c55e',
  rpa: '#f59e0b',
  browser: '#3b82f6',
  tool: '#6b7280',
}

const agentIcons: Record<string, string> = {
  code: '💻',
  knowledge: '📚',
  rpa: '🤖',
  browser: '🌐',
  tool: '🔧',
}

onMounted(async () => {
  try {
    const response = await api.get('/v1/agents')
    if (Array.isArray(response.data?.data)) {
      agents.value = response.data.data.filter((a: AgentItem) => a.type !== 'tool' && a.type !== 'knowledge')
    }
  } catch (error) {
    message.error('获取 Agent 列表失败，请检查网络连接')
  } finally {
    loading.value = false
  }
})

async function runAgent(agent: AgentItem) {
  const task = window.prompt(`请输入 ${agent.name} 的任务:`)
  if (!task) return

  try {
    await api.post('/v1/agents/dispatch', {
      agent_type: agent.type,
      task: task,
      session_id: `agent_${Date.now()}`,
    })
    message.success(`任务已派发给 ${agent.name}`)
  } catch (error: any) {
    message.error(error.message || '派发失败')
  }
}
</script>

<template>
  <div class="agents-container">
    <div class="agents-header">
      <div class="header-icon">
        <UserOutlined style="font-size: 24px; color: #8b5cf6" />
      </div>
      <div>
        <h1>Agents</h1>
        <p class="subtitle">可用的 AI Agent 及其能力</p>
      </div>
    </div>

    <Spin v-if="loading" class="loading-spinner" />

    <Empty v-else-if="agents.length === 0" description="暂无 Agent 配置">
      <template #image>
        <ThunderboltOutlined style="font-size: 48px; color: #8b5cf6" />
      </template>
    </Empty>

    <div v-else class="agents-grid">
      <Card v-for="agent in agents" :key="agent.type" class="agent-card" :bordered="true">
        <div class="agent-header">
          <span class="agent-icon">{{ agentIcons[agent.type] || '🤖' }}</span>
          <span class="agent-name">{{ agent.name }}</span>
          <Tag :color="agentColors[agent.type] || '#6b7280'">{{ agent.type }}</Tag>
        </div>
        <p class="agent-description">{{ agent.description }}</p>
        <div class="agent-actions">
          <Button type="primary" size="small" @click="runAgent(agent)">
            <template #icon><PlayCircleOutlined /></template>
            运行
          </Button>
        </div>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.agents-container { padding: 24px; }
.agents-header { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.header-icon { width: 48px; height: 48px; border-radius: 12px; background-color: #f3e8ff; display: flex; align-items: center; justify-content: center; }
.agents-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.subtitle { margin: 4px 0 0; color: #6b7280; font-size: 14px; }
.loading-spinner { display: flex; justify-content: center; padding: 80px 0; }
.agents-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }
.agent-card { transition: box-shadow 0.2s; }
.agent-card:hover { box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1); }
.agent-header { display: flex; align-items: center; gap: 8px; margin-bottom: 12px; }
.agent-icon { font-size: 20px; }
.agent-name { font-weight: 600; font-size: 16px; }
.agent-description { color: #6b7280; font-size: 14px; margin: 0 0 16px; line-height: 1.5; }
.agent-actions { display: flex; align-items: center; gap: 12px; }
</style>
