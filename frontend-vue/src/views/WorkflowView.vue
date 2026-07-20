<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { VueFlow, useVueFlow, Handle, Position } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import { NButton, NInput, NSelect, NCard, NIcon, NDrawer, NDrawerContent, NForm, NFormItem, NEmpty, NPopconfirm, NTag, NDivider, NScrollbar, useMessage } from 'naive-ui'
import { AddOutline, SaveOutline, PlayOutline, TrashOutline, CloseOutline, ListOutline, CreateOutline } from '@vicons/ionicons5'
import { api } from '../api'
import type { Node, Edge, Connection } from '@vue-flow/core'

 // 从 JWT token 中解析 user_id（不依赖 auth store 的异步加载）
function getUserIdFromToken(): string | null {
  const token = localStorage.getItem('token')
  if (!token) return null
  try {
    const payload = JSON.parse(atob(token.split('.')[1]))
    return (payload as Record<string, any>).uid || null
  } catch { return null }
}

function formatDate(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return isNaN(d.getTime()) ? '' : d.toLocaleString()
}

// ── Types ──
interface GraphNodeBackend {
  id: string
  label: string
  node_type: string
  config?: Record<string, any>
}

interface GraphEdgeBackend {
  source_id: string
  target_id: string
  condition?: string
  label?: string
}

interface GraphRecord {
  id: string
  name: string
  graph_json: string
  created_at: string
  updated_at: string
}

interface ExecutionEvent {
  graph_id: string
  event?: {
    type: string
    node_id: string
    status?: string
    output?: any
    error?: string
    duration_ms?: number
  }
  status?: string
  error?: string
  summary?: string
  results?: Record<string, any>
}

// ── Node type definitions ──
const nodeTypes = [
  { type: 'input', label: '输入', color: '#22c55e', icon: '📥', description: '接收用户输入' },
  { type: 'llm', label: 'LLM', color: '#8b5cf6', icon: '🧠', description: '调用大语言模型' },
  { type: 'tool', label: '工具', color: '#3b82f6', icon: '🔧', description: '执行注册工具' },
  { type: 'condition', label: '条件', color: '#f59e0b', icon: '🔀', description: '条件分支判断' },
  { type: 'output', label: '输出', color: '#6b7280', icon: '📤', description: '输出结果' },
]

const toolOptions = [
  { value: 'browser_navigate', label: '浏览器导航' },
  { value: 'browser_click', label: '点击元素' },
  { value: 'browser_type', label: '输入文本' },
  { value: 'browser_read', label: '读取页面' },
  { value: 'browser_screenshot', label: '截图' },
  { value: 'browser_scroll', label: '滚动页面' },
  { value: 'browser_get_state', label: '获取页面状态' },
  { value: 'browser_tab_list', label: '列出标签页' },
  { value: 'browser_tab_create', label: '新建标签页' },
  { value: 'browser_tab_switch', label: '切换标签页' },
  { value: 'browser_tab_close', label: '关闭标签页' },
  { value: 'web_search', label: '网页搜索' },
  { value: 'shell_exec', label: '执行命令' },
]

// ── Vue Flow ──
const { findNode, addNodes, addEdges, removeNodes, getNodes, getEdges, toObject, fromObject } = useVueFlow({
  defaultEdgeOptions: {
    type: 'smoothstep',
    animated: true,
  },
})

// ── State ──
const message = useMessage()
const nodes = ref<Node[]>([])
const edges = ref<Edge[]>([])
const workflowName = ref('新建工作流')
const workflowId = ref<string | null>(null)
const savedWorkflows = ref<GraphRecord[]>([])
const showPanel = ref(false)
const selectedNode = ref<Node | null>(null)
const isExecuting = ref(false)
const executionLogs = ref<string[]>([])
let _eventSource: EventSource | null = null
const showDrawer = ref(false)
const dragNodeType = ref<string | null>(null)

// ── Node config form fields ──
const editLabel = ref('')
const editSystemPrompt = ref('')
const editPrompt = ref('')
const editToolName = ref('')
const editCondition = ref('')
const editUserMessage = ref('')
const editInputVariable = ref('user_input')

// ── Helper ──
let nodeCounter = 0

function genNodeId(type: string): string {
  nodeCounter++
  return `${type}_${nodeCounter}`
}

function getNodeColor(type: string): string {
  return nodeTypes.find(n => n.type === type)?.color || '#6b7280'
}

function getNodeIcon(type: string): string {
  return nodeTypes.find(n => n.type === type)?.icon || '📦'
}

// ── Drag & Drop ──
function onDragStart(event: DragEvent, type: string) {
  if (event.dataTransfer) {
    event.dataTransfer.setData('application/vueflow', type)
    event.dataTransfer.effectAllowed = 'move'
  }
  dragNodeType.value = type
}

function onDragOver(event: DragEvent) {
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
}

function onDrop(event: DragEvent) {
  const type = event.dataTransfer?.getData('application/vueflow')
  if (!type) return

  // Get drop position relative to flow canvas
  const flowEl = document.querySelector('.vue-flow')
  if (!flowEl) return
  const rect = flowEl.getBoundingClientRect()
  const position = {
    x: event.clientX - rect.left - 80,
    y: event.clientY - rect.top - 30,
  }

  const nodeDef = nodeTypes.find(n => n.type === type)
  const newNode: Node = {
    id: genNodeId(type),
    type: type,
    position,
    data: {
      label: nodeDef?.label || type,
      nodeType: type,
      color: nodeDef?.color || '#6b7280',
      icon: nodeDef?.icon || '📦',
    },
  }

  addNodes([newNode])
}

// ── Node selection ──
function onNodeClick(_event: any) {
  const node = _event.node
  selectedNode.value = node
  showPanel.value = true

  // Populate edit fields
  editLabel.value = node.data?.label || ''
  const cfg = node.data?.config || {}
  editSystemPrompt.value = cfg.system_prompt || ''
  editPrompt.value = cfg.prompt || ''
  editToolName.value = cfg.tool_name || ''
  editCondition.value = cfg.condition || ''
  editUserMessage.value = cfg.user_message || ''
  editInputVariable.value = cfg.variable || 'user_input'
}

function onPaneClick() {
  selectedNode.value = null
  showPanel.value = false
}

// ── Save node config from panel ──
function applyNodeConfig() {
  if (!selectedNode.value) return
  const node = findNode(selectedNode.value.id)
  if (!node) return

  node.data = {
    ...node.data,
    label: editLabel.value,
    config: {
      system_prompt: editSystemPrompt.value || undefined,
      prompt: editPrompt.value || undefined,
      tool_name: editToolName.value || undefined,
      condition: editCondition.value || undefined,
      user_message: editUserMessage.value || undefined,
      variable: editInputVariable.value || undefined,
    },
  }
}

// Watch config changes
watch([editLabel, editSystemPrompt, editPrompt, editToolName, editCondition, editUserMessage, editInputVariable], () => {
  applyNodeConfig()
})

// ── Delete selected node ──
function deleteSelectedNode() {
  if (!selectedNode.value) return
  removeNodes([selectedNode.value.id])
  selectedNode.value = null
  showPanel.value = false
}

// ── Edge connection ──
function onConnect(params: Connection) {
  const newEdge: Edge = {
    id: `e-${params.source}-${params.target}`,
    source: params.source as string,
    target: params.target as string,
    sourceHandle: params.sourceHandle || undefined,
    targetHandle: params.targetHandle || undefined,
    type: 'smoothstep',
    animated: true,
  }
  addEdges([newEdge])
}

// ── Convert between VueFlow ↔ Backend format ──
function toBackendFormat(): { nodes: GraphNodeBackend[]; edges: GraphEdgeBackend[]; entry_point: string } {
  const allNodes = getNodes.value
  const allEdges = getEdges.value

  const backendNodes: GraphNodeBackend[] = allNodes.map((n) => {
    const config: Record<string, any> = n.data?.config || {}
    // Store position in config for round-tripping
    config.position = { x: Math.round(n.position.x), y: Math.round(n.position.y) }
    return {
      id: n.id,
      label: n.data?.label || n.id,
      node_type: n.data?.nodeType || n.type || 'tool',
      config,
    }
  })

  const backendEdges: GraphEdgeBackend[] = allEdges.map((e) => ({
    source_id: e.source,
    target_id: e.target,
    condition: e.data?.condition || '',
    label: typeof e.label === 'string' ? e.label : '',
  }))

  // Entry point: first input node, or first node
  const inputNode = allNodes.find(n => n.data?.nodeType === 'input')
  const entryPoint = inputNode?.id || allNodes[0]?.id || ''

  return { nodes: backendNodes, edges: backendEdges, entry_point: entryPoint }
}

function fromBackendFormat(data: any) {
  if (!data) return
  const graphDef = typeof data === 'string' ? JSON.parse(data) : data

  const flowNodes: Node[] = (graphDef.nodes || []).map((n: GraphNodeBackend) => {
    const cfg = n.config || {}
    const pos = cfg.position || { x: 0, y: 0 }
    return {
      id: n.id,
      type: n.node_type,
      position: { x: pos.x || 0, y: pos.y || 0 },
      data: {
        label: n.label,
        nodeType: n.node_type,
        color: getNodeColor(n.node_type),
        icon: getNodeIcon(n.node_type),
        config: cfg,
      },
    }
  })

  const flowEdges: Edge[] = (graphDef.edges || []).map((e: GraphEdgeBackend, i: number) => ({
    id: `e-${e.source_id}-${e.target_id}-${i}`,
    source: e.source_id,
    target: e.target_id,
    label: e.label || '',
    type: 'smoothstep',
    animated: true,
  }))

  nodes.value = flowNodes
  edges.value = flowEdges
  workflowName.value = graphDef.name || '未命名工作流'

  // Update counter to avoid ID collision
  nodeCounter = flowNodes.length + 10
}

// ── API: Save ──
async function saveWorkflow() {
  const graphData = toBackendFormat()
  const payload: Record<string, any> = {
    id: workflowId.value || undefined,
    name: workflowName.value,
    graph_json: JSON.stringify({
      name: workflowName.value,
      ...graphData,
    }),
    user_id: getUserIdFromToken() || undefined,
  }

  try {
    const resp = await api.post('/v1/graphs', payload)
    workflowId.value = resp.data?.data?.id || resp.data?.id
    message.success('工作流已保存')
    await loadWorkflows()
  } catch (err: any) {
    message.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

// ── API: Load list ──
async function loadWorkflows() {
  try {
    const resp = await api.get('/v1/graphs')
    savedWorkflows.value = resp.data?.data || []
  } catch {
    savedWorkflows.value = []
  }
}

// ── API: Load one ──
function loadWorkflow(record: GraphRecord) {
  workflowId.value = record.id
  workflowName.value = record.name
  fromBackendFormat(record.graph_json)
  message.success(`已加载: ${record.name}`)
}

// ── API: Delete ──
async function deleteWorkflow(id: string) {
  try {
    await api.delete(`/v1/graphs/${id}`)
    message.success('已删除')
    await loadWorkflows()
    if (workflowId.value === id) {
      workflowId.value = null
      resetCanvas()
    }
  } catch (err: any) {
    message.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

// ── API: Execute ──
async function executeWorkflow() {
  if (!workflowId.value) {
    message.warning('请先保存工作流')
    return
  }

  isExecuting.value = true
  executionLogs.value = ['⏳ 正在执行...']

  try {
    // Execute fires SSE events; we listen on the broadcast channel
    await api.post(`/v1/graphs/${workflowId.value}/execute`, {
      initial_state: {},
    })
    message.info('工作流已提交执行')

    // Listen for SSE events
    const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'
    _eventSource = new EventSource(`${API_URL}/events?session_id=graph_${workflowId.value}`)
    const eventSource = _eventSource

    eventSource.onmessage = (event) => {
      try {
        const data: ExecutionEvent = JSON.parse(event.data)

        if (data.event) {
          const evt = data.event
          const icon = evt.status === 'completed' ? '✅' : evt.status === 'error' ? '❌' : '⏳'
          executionLogs.value.push(`${icon} [${evt.type}] ${evt.node_id} ${evt.error || ''}`)
        }

        if (data.status === 'completed' || data.event?.type === 'done') {
          executionLogs.value.push('✅ 执行完成')
          if (data.summary) {
            executionLogs.value.push(data.summary)
          }
          isExecuting.value = false
          eventSource.close()
        }

        if (data.error) {
          executionLogs.value.push(`❌ 错误: ${data.error}`)
          isExecuting.value = false
          eventSource.close()
        }
      } catch {
        // ignore parse errors
      }
    }

    eventSource.onerror = () => {
      executionLogs.value.push('⚠️ SSE 连接已断开')
      isExecuting.value = false
      eventSource.close()
    }
  } catch (err: any) {
    executionLogs.value.push(`❌ 启动失败: ${err.response?.data?.error || err.message}`)
    isExecuting.value = false
  }
}

// ── Reset ──
function resetCanvas() {
  nodes.value = []
  edges.value = []
  workflowName.value = '新建工作流'
  workflowId.value = null
  selectedNode.value = null
  showPanel.value = false
  nodeCounter = 0
}

// ── Mount ──
onMounted(() => {
  loadWorkflows()
})

// ── Unmount — cleanup SSE connection ──
onUnmounted(() => {
  if (_eventSource) {
    _eventSource.close()
    _eventSource = null
  }
})
</script>

<template>
  <div class="workflow-container">
    <!-- Top Toolbar -->
    <div class="toolbar">
      <div class="toolbar-left">
        <NInput
          v-model:value="workflowName"
          placeholder="工作流名称"
          size="small"
          style="width: 200px"
        />
        <NButton size="small" type="primary" @click="saveWorkflow" :loading="false">
          <template #icon><NIcon><SaveOutline /></NIcon></template>
          保存
        </NButton>
        <NButton size="small" type="success" @click="executeWorkflow" :loading="isExecuting" :disabled="!workflowId">
          <template #icon><NIcon><PlayOutline /></NIcon></template>
          执行
        </NButton>
        <NButton size="small" quaternary @click="resetCanvas">
          新建
        </NButton>
      </div>
      <div class="toolbar-right">
        <NButton size="small" quaternary @click="showDrawer = true">
          <template #icon><NIcon><ListOutline /></NIcon></template>
          工作流列表
        </NButton>
        <NTag v-if="workflowId" type="success" size="small">已保存</NTag>
        <NTag v-else type="warning" size="small">未保存</NTag>
      </div>
    </div>

    <div class="main-area">
      <!-- Left: Node Palette -->
      <div class="node-palette">
        <div class="palette-title">节点</div>
        <div
          v-for="nt in nodeTypes"
          :key="nt.type"
          class="palette-item"
          :draggable="true"
          @dragstart="(e: DragEvent) => onDragStart(e, nt.type)"
        >
          <span class="palette-icon">{{ nt.icon }}</span>
          <span class="palette-label">{{ nt.label }}</span>
        </div>
      </div>

      <!-- Center: Canvas -->
      <div
        class="canvas-wrapper"
        @drop="onDrop"
        @dragover="onDragOver"
      >
        <VueFlow
          v-model:nodes="nodes"
          v-model:edges="edges"
          :default-edge-options="{ type: 'smoothstep', animated: true }"
          :snap-to-grid="true"
          :snap-grid="[15, 15]"
          fit-view-on-init
          @node-click="onNodeClick"
          @pane-click="onPaneClick"
          @connect="onConnect"
        >
          <Background :gap="15" :size="1" />
          <Controls />
          <MiniMap />

          <!-- Custom node template -->
          <template #node-input="nodeProps">
            <div class="custom-node" :style="{ borderColor: '#22c55e' }">
              <div class="node-header" style="background: #22c55e20;">
                <span>📥 {{ nodeProps.data?.label || '输入' }}</span>
              </div>
              <div class="node-body">
                <span class="node-type-tag">input</span>
              </div>
              <Handle type="source" :position="Position.Bottom" />
            </div>
          </template>

          <template #node-llm="nodeProps">
            <div class="custom-node" :style="{ borderColor: '#8b5cf6' }">
              <Handle type="target" :position="Position.Top" />
              <div class="node-header" style="background: #8b5cf620;">
                <span>🧠 {{ nodeProps.data?.label || 'LLM' }}</span>
              </div>
              <div class="node-body">
                <span class="node-type-tag">llm</span>
              </div>
              <Handle type="source" :position="Position.Bottom" />
            </div>
          </template>

          <template #node-tool="nodeProps">
            <div class="custom-node" :style="{ borderColor: '#3b82f6' }">
              <Handle type="target" :position="Position.Top" />
              <div class="node-header" style="background: #3b82f620;">
                <span>🔧 {{ nodeProps.data?.label || '工具' }}</span>
              </div>
              <div class="node-body">
                <span class="node-type-tag">tool</span>
                <span v-if="nodeProps.data?.config?.tool_name" class="node-detail">
                  {{ nodeProps.data.config.tool_name }}
                </span>
              </div>
              <Handle type="source" :position="Position.Bottom" />
            </div>
          </template>

          <template #node-condition="nodeProps">
            <div class="custom-node" :style="{ borderColor: '#f59e0b' }">
              <Handle type="target" :position="Position.Top" />
              <div class="node-header" style="background: #f59e0b20;">
                <span>🔀 {{ nodeProps.data?.label || '条件' }}</span>
              </div>
              <div class="node-body">
                <span class="node-type-tag">condition</span>
              </div>
              <Handle id="true" type="source" :position="Position.Bottom" style="left: 30%" />
              <Handle id="false" type="source" :position="Position.Bottom" style="left: 70%" />
            </div>
          </template>

          <template #node-output="nodeProps">
            <div class="custom-node" :style="{ borderColor: '#6b7280' }">
              <Handle type="target" :position="Position.Top" />
              <div class="node-header" style="background: #6b728020;">
                <span>📤 {{ nodeProps.data?.label || '输出' }}</span>
              </div>
              <div class="node-body">
                <span class="node-type-tag">output</span>
              </div>
            </div>
          </template>
        </VueFlow>
      </div>

      <!-- Right: Property Panel -->
      <div v-if="showPanel && selectedNode" class="property-panel">
        <div class="panel-header">
          <span>节点属性</span>
          <NButton text size="small" @click="showPanel = false">
            <template #icon><NIcon><CloseOutline /></NIcon></template>
          </NButton>
        </div>
        <NScrollbar style="max-height: calc(100vh - 180px)">
          <NForm label-placement="top" size="small" style="padding: 12px">
            <NFormItem label="节点 ID">
              <NInput :value="selectedNode.id" disabled />
            </NFormItem>
            <NFormItem label="类型">
              <NInput :value="selectedNode.data?.nodeType" disabled />
            </NFormItem>
            <NFormItem label="标签">
              <NInput v-model:value="editLabel" placeholder="节点标签" />
            </NFormItem>

            <!-- LLM config -->
            <template v-if="selectedNode.data?.nodeType === 'llm'">
              <NDivider />
              <NFormItem label="System Prompt">
                <NInput
                  v-model:value="editSystemPrompt"
                  type="textarea"
                  :rows="4"
                  placeholder="系统提示词"
                />
              </NFormItem>
              <NFormItem label="用户消息模板">
                <NInput
                  v-model:value="editUserMessage"
                  type="textarea"
                  :rows="3"
                  placeholder="使用 {{变量名}} 引用状态变量"
                />
              </NFormItem>
            </template>

            <!-- Tool config -->
            <template v-if="selectedNode.data?.nodeType === 'tool'">
              <NDivider />
              <NFormItem label="工具名称">
                <NSelect
                  v-model:value="editToolName"
                  :options="toolOptions"
                  placeholder="选择工具"
                  filterable
                />
              </NFormItem>
            </template>

            <!-- Condition config -->
            <template v-if="selectedNode.data?.nodeType === 'condition'">
              <NDivider />
              <NFormItem label="条件表达式">
                <NInput
                  v-model:value="editCondition"
                  type="textarea"
                  :rows="3"
                  placeholder="如: state.status == 'ok'"
                />
              </NFormItem>
            </template>

            <!-- Input config -->
            <template v-if="selectedNode.data?.nodeType === 'input'">
              <NDivider />
              <NFormItem label="输入变量名">
                <NInput v-model:value="editInputVariable" placeholder="user_input" />
              </NFormItem>
            </template>

            <NDivider />
            <NButton type="error" size="small" @click="deleteSelectedNode" block>
              <template #icon><NIcon><TrashOutline /></NIcon></template>
              删除节点
            </NButton>
          </NForm>
        </NScrollbar>
      </div>
    </div>

    <!-- Execution logs bar -->
    <div v-if="executionLogs.length > 0" class="execution-bar">
      <div class="execution-header">
        <span>执行日志</span>
        <NButton text size="tiny" @click="executionLogs = []">清除</NButton>
      </div>
      <NScrollbar style="max-height: 150px">
        <div v-for="(log, i) in executionLogs" :key="i" class="log-line">{{ log }}</div>
      </NScrollbar>
    </div>

    <!-- Workflow List Drawer -->
    <NDrawer v-model:show="showDrawer" :width="380" placement="right">
      <NDrawerContent title="已保存的工作流">
        <NEmpty v-if="savedWorkflows.length === 0" description="暂无工作流" />
        <div v-else class="workflow-list">
          <NCard
            v-for="wf in savedWorkflows"
            :key="wf.id"
            size="small"
            class="workflow-item"
            hoverable
          >
            <div class="wf-item-row">
              <div class="wf-item-info" @click="loadWorkflow(wf); showDrawer = false">
                <div class="wf-item-name">{{ wf.name || '未命名工作流' }}</div>
                <div class="wf-item-time">{{ formatDate(wf.updated_at) || formatDate(wf.created_at) }}</div>
              </div>
              <NPopconfirm @positive-click="deleteWorkflow(wf.id)">
                <template #trigger>
                  <NButton text size="small" type="error">
                    <template #icon><NIcon><TrashOutline /></NIcon></template>
                  </NButton>
                </template>
                确认删除 {{ wf.name }}？
              </NPopconfirm>
            </div>
          </NCard>
        </div>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<!-- Import vue-flow styles -->
<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';
@import '@vue-flow/controls/dist/style.css';
@import '@vue-flow/minimap/dist/style.css';
</style>

<style scoped>
.workflow-container {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  border-bottom: 1px solid #e0e0e0;
  background: #fff;
  gap: 8px;
  flex-shrink: 0;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.main-area {
  display: flex;
  flex: 1;
  overflow: hidden;
}

/* Left Palette */
.node-palette {
  width: 160px;
  padding: 12px;
  border-right: 1px solid #e0e0e0;
  background: #fafafa;
  flex-shrink: 0;
}

.palette-title {
  font-weight: 600;
  font-size: 13px;
  color: #666;
  margin-bottom: 12px;
  text-transform: uppercase;
  letter-spacing: 1px;
}

.palette-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  margin-bottom: 6px;
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 6px;
  cursor: grab;
  font-size: 13px;
  transition: box-shadow 0.15s;
  user-select: none;
}

.palette-item:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.palette-item:active {
  cursor: grabbing;
}

.palette-icon {
  font-size: 16px;
}

.palette-label {
  font-weight: 500;
}

/* Canvas */
.canvas-wrapper {
  flex: 1;
  position: relative;
}

/* Custom Nodes */
.custom-node {
  background: #fff;
  border: 2px solid;
  border-radius: 8px;
  min-width: 140px;
  font-size: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.node-header {
  padding: 6px 10px;
  border-radius: 6px 6px 0 0;
  font-weight: 600;
  font-size: 13px;
  white-space: nowrap;
}

.node-body {
  padding: 6px 10px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.node-type-tag {
  background: #f3f4f6;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 10px;
  color: #6b7280;
}

.node-detail {
  font-size: 10px;
  color: #9ca3af;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100px;
}

/* Property Panel */
.property-panel {
  width: 300px;
  border-left: 1px solid #e0e0e0;
  background: #fff;
  flex-shrink: 0;
  overflow: hidden;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid #e0e0e0;
  font-weight: 600;
  font-size: 14px;
}

/* Execution logs */
.execution-bar {
  border-top: 1px solid #e0e0e0;
  background: #1e1e1e;
  color: #d4d4d4;
  font-family: monospace;
  font-size: 12px;
  padding: 8px 16px;
  flex-shrink: 0;
}

.execution-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
  color: #9ca3af;
  font-size: 11px;
}

.log-line {
  padding: 2px 0;
  white-space: pre-wrap;
  word-break: break-all;
}

/* Workflow list */
.workflow-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.workflow-item {
  cursor: pointer;
  transition: box-shadow 0.15s;
}

.workflow-item:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.wf-item-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.wf-item-info {
  flex: 1;
  cursor: pointer;
}

.wf-item-name {
  font-weight: 600;
  font-size: 14px;
}

.wf-item-time {
  font-size: 12px;
  color: #9ca3af;
  margin-top: 2px;
}

/* Dark mode support */
:root.dark .toolbar {
  background: #1e1e1e;
  border-color: #333;
}

:root.dark .node-palette {
  background: #252525;
  border-color: #333;
}

:root.dark .palette-item {
  background: #2d2d2d;
  border-color: #444;
  color: #d4d4d4;
}

:root.dark .custom-node {
  background: #2d2d2d;
  color: #d4d4d4;
}

:root.dark .property-panel {
  background: #1e1e1e;
  border-color: #333;
}
</style>
