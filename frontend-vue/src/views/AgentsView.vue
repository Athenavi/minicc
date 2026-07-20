<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NButton, NBadge, NSpin, NEmpty, NIcon, useMessage } from 'naive-ui'
import { GameControllerOutline, PlayOutline, SparklesOutline } from '@vicons/ionicons5'
import { api } from '../api'

interface AgentItem {
  type: string
  name: string
  description: string
}

const message = useMessage()
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
      // 过滤掉 tool 和 knowledge agent
      agents.value = response.data.data.filter((a: AgentItem) => a.type !== 'tool' && a.type !== 'knowledge')
    }
  } catch (error) {
    // 忽略错误
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
        <NIcon size="24" color="#8b5cf6">
          <GameControllerOutline />
        </NIcon>
      </div>
      <div>
        <h1>Agents</h1>
        <p class="subtitle">可用的 AI Agent 及其能力</p>
      </div>
    </div>

    <NSpin v-if="loading" :show="loading" class="loading-spinner">
      <div style="height: 200px"></div>
    </NSpin>

    <NEmpty v-else-if="agents.length === 0" description="暂无 Agent 配置">
      <template #icon>
        <NIcon size="48" color="#8b5cf6">
          <SparklesOutline />
        </NIcon>
      </template>
    </NEmpty>

    <div v-else class="agents-grid">
      <NCard v-for="agent in agents" :key="agent.type" class="agent-card">
        <div class="agent-header">
          <span class="agent-icon">{{ agentIcons[agent.type] || '🤖' }}</span>
          <span class="agent-name">{{ agent.name }}</span>
          <span
            class="agent-status"
            :style="{ backgroundColor: agentColors[agent.type] || '#6b7280' }"
          ></span>
        </div>
        <p class="agent-description">{{ agent.description }}</p>
        <div class="agent-actions">
          <NButton
            type="primary"
            size="small"
            @click="runAgent(agent)"
          >
            <template #icon>
              <NIcon><PlayOutline /></NIcon>
            </template>
            运行
          </NButton>
          <NBadge :value="agent.type" :color="agentColors[agent.type] || '#6b7280'" />
        </div>
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.agents-container {
  padding: 24px;
}

.agents-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.header-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background-color: #f3e8ff;
  display: flex;
  align-items: center;
  justify-content: center;
}

.agents-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.subtitle {
  margin: 4px 0 0;
  color: #6b7280;
  font-size: 14px;
}

.loading-spinner {
  display: flex;
  justify-content: center;
  padding: 80px 0;
}

.agents-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.agent-card {
  transition: box-shadow 0.2s;
}

.agent-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.agent-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.agent-icon {
  font-size: 20px;
}

.agent-name {
  font-weight: 600;
  font-size: 16px;
}

.agent-status {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-left: auto;
}

.agent-description {
  color: #6b7280;
  font-size: 14px;
  margin: 0 0 16px;
  line-height: 1.5;
}

.agent-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
</style>
