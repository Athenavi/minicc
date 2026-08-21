// 六大工作台统一状态管理 (Pinia Store)
// 整合对话、Agent、工作流、技能、知识库、插件的上下文

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { quickExecute as quickExecuteApi } from '@/api'

// ── 类型定义 ──

export type WorkstationType = 'dialogue' | 'agent' | 'workflow' | 'skill' | 'knowledge' | 'plugin'

export interface TraceSpan {
  trace_id: string
  span_name: string
  duration_ms: number
  timestamp: string
  tenant_id: string
  metadata?: Record<string, any>
  showDetails?: boolean
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  timestamp: number
  metadata?: Record<string, any>
}

export interface AgentStatus {
  id: string
  name: string
  role: string
  status: 'pending' | 'running' | 'completed' | 'error'
  output?: string
  logs: Array<{
    timestamp: number
    level: 'info' | 'warn' | 'error'
    message: string
  }>
}

export interface WorkflowNodeState {
  id: string
  title: string
  status: 'pending' | 'running' | 'completed' | 'error'
  progress: number // 0-100
}

export interface CapabilityInfo {
  capabilityId: string
  name: string
  description: string
  category: string // tool/service/template/composite
  tags: string[]
  registered: boolean
}

// ── Store ──

export const useGlobalStore = defineStore('global', () => {
  // ── State: 当前工作台 ──
  const currentWorkstation = ref<WorkstationType>('dialogue')
  
  // ── State: 对话上下文 ──
  const chatMessages = ref<ChatMessage[]>([])
  const activeSessionId = ref('')
  const currentTraceId = ref('')
  
  // ── State: Agent 协同 ──
  const agentStatuses = ref<AgentStatus[]>([])
  const isAgentCollaborating = ref(false)
  
  // ── State: 工作流 ──
  const workflowNodes = ref<WorkflowNodeState[]>([])
  const isWorkflowRunning = ref(false)
  
  // ── State: 技能市场 ──
  const availableCapabilities = ref<CapabilityInfo[]>([])
  const registeredCapabilities = ref<CapabilityInfo[]>([])
  
  // ── State: 知识库检索结果 ──
  const kbSearchResults = ref<any[]>([])
  const isKbSearching = ref(false)
  
  // ── State: 跨工作台调用链 ──
  const callChainSpans = ref<TraceSpan[]>([])
  const isCallChainVisible = ref(false)
  
  // ── State: 全局通知 ──
  const notifications = ref<Array<{
    id: string
    type: 'success' | 'warning' | 'error' | 'info'
    message: string
    timestamp: number
  }>>([])
  
  // ── Getters ──
  const activeWorkstationName = computed(() => {
    const names: Record<WorkstationType, string> = {
      dialogue: '对话',
      agent: 'Agent 协同',
      workflow: '工作流',
      skill: '技能市场',
      knowledge: '知识库',
      plugin: '插件中心',
    }
    return names[currentWorkstation.value]
  })
  
  const hasActiveTasks = computed(() => {
    return (
      isAgentCollaborating.value ||
      isWorkflowRunning.value ||
      isKbSearching.value
    )
  })
  
  const totalSpansCount = computed(() => {
    return callChainSpans.value.filter(s => s.span_name.startsWith('tool:')).length
  })
  
  const totalDurationMs = computed(() => {
    return callChainSpans.value.reduce((sum, s) => sum + s.duration_ms, 0)
  })
  
  // ── Actions: 工作台切换 ──
  function switchWorkstation(workstation: WorkstationType) {
    currentWorkstation.value = workstation
    console.log(`[GlobalStore] 切换到工作台: ${workstation}`)
  }
  
  // ── Actions: 对话操作 ──
  function addChatMessage(message: ChatMessage) {
    chatMessages.value.push(message)
  }
  
  function clearChatMessages() {
    chatMessages.value = []
  }
  
  function setActiveSession(sessionId: string) {
    activeSessionId.value = sessionId
  }
  
  function setCurrentTrace(traceId: string) {
    currentTraceId.value = traceId
  }
  
  // ── Actions: Agent 操作 ──
  function setAgentStatuses(agents: AgentStatus[]) {
    agentStatuses.value = agents
    isAgentCollaborating.value = agents.some(a => a.status === 'running')
  }
  
  function updateAgentLog(agentId: string, log: { timestamp: number; level: string; message: string }) {
    const agent = agentStatuses.value.find(a => a.id === agentId)
    if (agent) {
      agent.logs.push(log)
    }
  }
  
  function updateAgentOutput(agentId: string, output: string) {
    const agent = agentStatuses.value.find(a => a.id === agentId)
    if (agent) {
      agent.output = output
      agent.status = 'completed'
    }
  }
  
  function resetAgentStatuses() {
    agentStatuses.value = []
    isAgentCollaborating.value = false
  }
  
  // ── Actions: 工作流操作 ──
  function setWorkflowNodes(nodes: WorkflowNodeState[]) {
    workflowNodes.value = nodes
    isWorkflowRunning.value = nodes.some(n => n.status === 'running')
  }
  
  function updateNodeProgress(nodeId: string, progress: number) {
    const node = workflowNodes.value.find(n => n.id === nodeId)
    if (node) {
      node.progress = progress
      if (progress === 100) {
        node.status = 'completed'
      }
    }
  }
  
  function resetWorkflowNodes() {
    workflowNodes.value = []
    isWorkflowRunning.value = false
  }
  
  // ── Actions: 能力注册 ──
  function registerCapability(capability: CapabilityInfo) {
    registeredCapabilities.value.push(capability)
    const available = availableCapabilities.value.find(c => c.capabilityId === capability.capabilityId)
    if (available) {
      available.registered = true
    }
  }
  
  function unregisterCapability(capabilityId: string) {
    registeredCapabilities.value = registeredCapabilities.value.filter(c => c.capabilityId !== capabilityId)
    const available = availableCapabilities.value.find(c => c.capabilityId === capabilityId)
    if (available) {
      available.registered = false
    }
  }
  
  function loadCapabilities(capabilities: CapabilityInfo[]) {
    availableCapabilities.value = capabilities
  }
  
  // ── Actions: 知识库搜索 ──
  function setKbSearchResults(results: any[]) {
    kbSearchResults.value = results
    isKbSearching.value = false
  }
  
  function startKbSearch() {
    isKbSearching.value = true
    kbSearchResults.value = []
  }
  
  function clearKbSearchResults() {
    kbSearchResults.value = []
  }
  
  // ── Actions: 调用链更新 ──
  function appendCallChainSpan(span: TraceSpan) {
    callChainSpans.value.push(span)
    isCallChainVisible.value = true
  }
  
  function resetCallChain() {
    callChainSpans.value = []
    isCallChainVisible.value = false
  }
  
  // ── Actions: 通知 ──
  function addNotification(notification: {
    type: 'success' | 'warning' | 'error' | 'info'
    message: string
  }) {
    const id = `notif_${Date.now()}`
    notifications.value.push({
      ...notification,
      id,
      timestamp: Date.now(),
    })
    
    // 30 秒后自动删除
    setTimeout(() => {
      removeNotification(id)
    }, 30000)
  }
  
  function removeNotification(id: string) {
    notifications.value = notifications.value.filter(n => n.id !== id)
  }
  
  // ── Actions: 快捷执行 (跨工作台编排) ──
  async function quickExecute(userInput: string) {
    try {
      // 切换到对话工作台
      switchWorkstation('dialogue')

      // 显示调用链
      isCallChainVisible.value = true

      // 添加用户消息
      addChatMessage({
        role: 'user',
        content: userInput,
        timestamp: Date.now(),
      })

      addNotification({
        type: 'success',
        message: '任务已提交,正在执行...',
      })

      // 调用六大工作台统一入口（Go 网关 → Python TaskRouter 自动编排）
      const result = await quickExecuteApi({ message: userInput, mode: 'auto' })

      if (result.trace_id) {
        setCurrentTrace(result.trace_id)
      }

      if (result.success) {
        // 将跨工作台子任务渲染到调用链
        const subtasks = (result.metadata as any)?.subtasks ?? []
        for (const st of subtasks) {
          appendCallChainSpan({
            trace_id: result.trace_id ?? '',
            span_name: `task:${st.capability_id}`,
            duration_ms: st.duration_ms ?? 0,
            timestamp: new Date().toISOString(),
            tenant_id: '',
            metadata: {
              subtask_id: st.subtask_id,
              status: st.status,
            },
          })
        }

        addChatMessage({
          role: 'assistant',
          content: result.output || '(无输出)',
          timestamp: Date.now(),
          metadata: {
            task_id: result.metadata?.task_id,
            duration_ms: result.metadata?.duration_ms,
          },
        })

        addNotification({
          type: 'success',
          message: `任务完成 (${result.metadata?.duration_ms ?? 0}ms)`,
        })
      } else {
        addChatMessage({
          role: 'assistant',
          content: `执行失败: ${result.error ?? '未知错误'}`,
          timestamp: Date.now(),
        })
        addNotification({
          type: 'error',
          message: `执行失败: ${result.error ?? '未知错误'}`,
        })
      }

      return result
    } catch (error) {
      console.error('[GlobalStore] 快捷执行失败:', error)
      addChatMessage({
        role: 'assistant',
        content: `执行失败: ${(error as Error).message}`,
        timestamp: Date.now(),
      })
      addNotification({
        type: 'error',
        message: `执行失败: ${(error as Error).message}`,
      })
      return { success: false, error }
    }
  }
  
  // ── Actions: 持久化 ──
  function persistToLocalStorage() {
    const data = {
      currentWorkstation: currentWorkstation.value,
      chatMessages: chatMessages.value,
      activeSessionId: activeSessionId.value,
    }
    localStorage.setItem('minicc_global_state', JSON.stringify(data))
  }
  
  function loadFromLocalStorage() {
    const raw = localStorage.getItem('minicc_global_state')
    if (!raw) return
    
    try {
      const data = JSON.parse(raw)
      currentWorkstation.value = data.currentWorkstation || 'dialogue'
      chatMessages.value = data.chatMessages || []
      activeSessionId.value = data.activeSessionId || ''
    } catch (error) {
      console.error('[GlobalStore] 加载本地存储失败:', error)
    }
  }
  
  // ── 初始化 ──
  loadFromLocalStorage()
  
  // 定期持久化 (每 5 秒)
  setInterval(persistToLocalStorage, 5000)
  
  return {
    // State
    currentWorkstation,
    chatMessages,
    activeSessionId,
    currentTraceId,
    agentStatuses,
    isAgentCollaborating,
    workflowNodes,
    isWorkflowRunning,
    availableCapabilities,
    registeredCapabilities,
    kbSearchResults,
    isKbSearching,
    callChainSpans,
    isCallChainVisible,
    notifications,
    
    // Getters
    activeWorkstationName,
    hasActiveTasks,
    totalSpansCount,
    totalDurationMs,
    
    // Actions
    switchWorkstation,
    addChatMessage,
    clearChatMessages,
    setActiveSession,
    setCurrentTrace,
    setAgentStatuses,
    updateAgentLog,
    updateAgentOutput,
    resetAgentStatuses,
    setWorkflowNodes,
    updateNodeProgress,
    resetWorkflowNodes,
    registerCapability,
    unregisterCapability,
    loadCapabilities,
    setKbSearchResults,
    startKbSearch,
    clearKbSearchResults,
    appendCallChainSpan,
    resetCallChain,
    addNotification,
    removeNotification,
    quickExecute,
  }
})
