<!-- WorkflowDAGEditor - 工作流 DAG 可视化编辑器 -->
<template>
  <div class="workflow-dag-editor">
    <!-- 工具栏 -->
    <div class="toolbar">
      <button @click="addNode" class="toolbar-btn">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 1v14M1 8h14" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
        </svg>
        添加节点
      </button>
      <button @click="autoLayout" class="toolbar-btn">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <rect x="2" y="2" width="5" height="5" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <rect x="9" y="2" width="5" height="5" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <rect x="2" y="9" width="5" height="5" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <rect x="9" y="9" width="5" height="5" fill="none" stroke="currentColor" stroke-width="1.5"/>
        </svg>
        自动布局
      </button>
      <button @click="exportJSON" class="toolbar-btn">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path d="M3 3h10v10H3V3z" fill="none" stroke="currentColor" stroke-width="1.5"/>
          <path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
        导出 JSON
      </button>
      <button @click="importJSON" class="toolbar-btn">导入 JSON</button>
      <button @click="validateWorkflow" class="toolbar-btn btn-validate">验证</button>
      <button @click="saveWorkflow" class="toolbar-btn btn-save">保存</button>
    </div>
    
    <!-- SVG 画布 -->
    <div class="canvas-container">
      <svg ref="svgCanvas" class="dag-canvas" @click="handleCanvasClick">
        <defs>
          <!-- 箭头标记 -->
          <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
            <polygon points="0 0, 10 3.5, 0 7" fill="#6b7280"/>
          </marker>
          
          <!-- 渐变定义 -->
          <linearGradient id="nodeGradient" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" style="stop-color:var(--bg-card);stop-opacity:1" />
            <stop offset="100%" style="stop-color:var(--bg-secondary);stop-opacity:1" />
          </linearGradient>
        </defs>
        
        <!-- 连线层 -->
        <g class="edges-layer">
          <path
            v-for="(edge, eIdx) in workflow.edges"
            :key="eIdx"
            :d="getEdgePath(edge.source, edge.target)"
            fill="none"
            stroke-width="2"
            marker-end="url(#arrowhead)"
            class="edge-path"
          />
        </g>
        
        <!-- 节点层 -->
        <g class="nodes-layer">
          <g
            v-for="(node, nIdx) in workflow.nodes"
            :key="nIdx"
            class="dag-node"
            :class="{ selected: selectedNode?.id === node.id, error: node.error }"
            @mousedown="startDrag($event, node)"
            @click.stop="selectNode(node)"
          >
            <!-- 节点矩形 -->
            <rect
              :x="node.x"
              :y="node.y"
              :width="node.width || 180"
              :height="node.height || 80"
              rx="8"
              ry="8"
              fill="url(#nodeGradient)"
              :stroke="getNodeStrokeColor(node.type)"
              stroke-width="2"
            />
            
            <!-- 节点类型图标 -->
            <circle
              :cx="node.x + 16"
              :cy="node.y + 16"
              r="8"
              :fill="getNodeStrokeColor(node.type)"
            />
            <text
              :x="node.x + 16"
              :y="node.y + 20"
              text-anchor="middle"
              fill="white"
              font-size="10"
            >
              {{ getNodeIcon(node.type) }}
            </text>
            
            <!-- 节点标题 -->
            <text
              :x="node.x + 30"
              :y="node.y + 20"
              class="node-title"
              font-size="12"
              font-weight="600"
            >
              {{ node.title }}
            </text>
            
            <!-- 节点描述 -->
            <text
              :x="node.x + 16"
              :y="node.y + 40"
              class="node-desc"
              font-size="10"
            >
              {{ node.description || '暂无描述' }}
            </text>
            
            <!-- 节点状态指示器 -->
            <circle
              v-if="node.status"
              :cx="node.x + (node.width || 180) - 12"
              :cy="node.y + 12"
              r="5"
              :fill="getStatusColor(node.status)"
            />
            
            <!-- 绑定技能标签 -->
            <foreignObject
              v-if="node.boundSkill"
              :x="node.x + 8"
              :y="node.y + (node.height || 80) - 24"
              :width="(node.width || 180) - 16"
              height="20"
            >
              <div class="skill-badge">
                <svg width="10" height="10" viewBox="0 0 10 10" fill="currentColor">
                  <path d="M5 1l3 3h-2v5H4V4H2L5 1z"/>
                </svg>
                {{ node.boundSkill.name }}
              </div>
            </foreignObject>
          </g>
        </g>
      </svg>
      
      <!-- 属性面板 -->
      <div v-if="selectedNode" class="properties-panel">
        <div class="panel-header">
          <span>节点属性</span>
          <button @click="selectedNode = null" class="btn-close" title="关闭属性面板">×</button>
        </div>
        <div class="panel-body">
          <div class="form-group">
            <label>标题</label>
            <input v-model="selectedNode.title" type="text" class="form-input" />
          </div>
          <div class="form-group">
            <label>类型</label>
            <select v-model="selectedNode.type" class="form-select">
              <option value="llm_call">LLM 调用</option>
              <option value="tool_execution">工具执行</option>
              <option value="condition">条件判断</option>
              <option value="loop">循环</option>
              <option value="end">结束</option>
              <option value="agent">Agent</option>
            </select>
          </div>
          <div class="form-group">
            <label>描述</label>
            <textarea v-model="selectedNode.description" class="form-textarea" rows="3"></textarea>
          </div>
          <div class="form-group">
            <label>绑定技能</label>
            <select v-model="selectedNode.boundSkillId" class="form-select">
              <option value="">无</option>
              <option
                v-for="skill in availableSkills"
                :key="skill.capabilityId"
                :value="skill.capabilityId"
              >
                {{ skill.name }}
              </option>
            </select>
          </div>
          <template v-if="selectedNode.type === 'agent'">
            <div class="section-label">Agent 配置</div>
            <div class="form-group">
              <label>Agent</label>
              <select
                v-if="!agentManualMode"
                v-model="editAgentId"
                class="form-select"
                @change="onAgentSelect"
              >
                <option value="">请选择 Agent</option>
                <option v-for="a in agentList" :key="a.id" :value="a.id">{{ a.name }}</option>
              </select>
              <input
                v-else
                v-model="editAgentName"
                type="text"
                class="form-input"
                placeholder="手动输入 Agent 名称"
              />
              <div v-if="agentManualMode && !agentLoading" class="agent-hint">Agent 列表不可用，请手动填写名称</div>
            </div>
            <div class="form-group">
              <label>名称（自动填入，可覆盖）</label>
              <input v-model="editAgentName" type="text" class="form-input" placeholder="Agent 名称" />
            </div>
            <div class="form-group">
              <label>System Prompt（自动填入，可覆盖）</label>
              <textarea v-model="editAgentSystemPrompt" class="form-textarea" rows="3" placeholder="留空则使用 Agent 默认提示词"></textarea>
            </div>
            <div class="form-group">
              <label>模型（自动填入，可覆盖）</label>
              <input v-model="editAgentModel" type="text" class="form-input" placeholder="如 gpt-4o-mini / deepseek-chat" />
            </div>
            <div class="form-group">
              <label>最大轮数（自动填入，可覆盖）</label>
              <input v-model.number="editAgentMaxTurns" type="number" min="1" max="50" class="form-input" />
            </div>
            <div class="form-group">
              <label>任务输入</label>
              <textarea v-model="editAgentTask" class="form-textarea" rows="3" placeholder="子任务描述；支持 $节点ID 引用前置输出（如 $llm_1），留空则使用前置输出"></textarea>
            </div>
          </template>
          <div class="form-actions">
            <button @click="deleteSelectedNode" class="btn-delete">删除节点</button>
            <button @click="duplicateNode" class="btn-duplicate">复制</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import { listAgents } from '../api'

interface WorkflowNode {
  id: string
  title: string
  description?: string
  type: string // llm_call | tool_execution | condition | loop | end | agent
  x: number
  y: number
  width?: number
  height?: number
  status?: string // pending | running | completed | error
  boundSkill?: any
  boundSkillId?: string
  config?: Record<string, any> // 节点配置（agent 等类型使用，随工作流 JSON 持久化）
  error?: boolean
}

interface WorkflowEdge {
  source: string
  target: string
}

interface Workflow {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
}

const props = defineProps<{
  workflow: Workflow
  availableSkills?: any[]
}>()

const emit = defineEmits<{
  (e: 'save', workflow: Workflow): void
  (e: 'export', workflow: Workflow): void
  (e: 'update:workflow', workflow: Workflow): void
}>()

const svgCanvas = ref<SVGElement | null>(null)
const selectedNode = ref<WorkflowNode | null>(null)
const draggingNode = ref<WorkflowNode | null>(null)
const dragOffset = reactive({ x: 0, y: 0 })

// 添加节点
function addNode() {
  const newNode: WorkflowNode = {
    id: `node_${Date.now()}`,
    title: '新节点',
    description: '点击编辑描述',
    type: 'tool_execution',
    x: 100 + Math.random() * 200,
    y: 100 + Math.random() * 200,
    width: 180,
    height: 80,
  }
  emit('update:workflow', {
    ...props.workflow,
    nodes: [...props.workflow.nodes, newNode],
  })
}

// 删除选中节点
function deleteSelectedNode() {
  if (!selectedNode.value) return
  
  const nodeId = selectedNode.value.id
  emit('update:workflow', {
    ...props.workflow,
    nodes: props.workflow.nodes.filter(n => n.id !== nodeId),
    edges: props.workflow.edges.filter(
      e => e.source !== nodeId && e.target !== nodeId
    ),
  })
  selectedNode.value = null
  message.success('节点已删除')
}

// 复制节点
function duplicateNode() {
  if (!selectedNode.value) return
  
  const original = selectedNode.value
  const newNode: WorkflowNode = {
    ...original,
    id: `node_${Date.now()}`,
    x: original.x + 20,
    y: original.y + 20,
  }
  emit('update:workflow', {
    ...props.workflow,
    nodes: [...props.workflow.nodes, newNode],
  })
  message.success('节点已复制')
}

// 拖拽节点
function startDrag(event: MouseEvent, node: WorkflowNode) {
  draggingNode.value = node
  const rect = (event.target as Element).getBoundingClientRect()
  dragOffset.x = event.clientX - rect.left
  dragOffset.y = event.clientY - rect.top
  
  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', stopDrag)
}

function onDrag(event: MouseEvent) {
  if (!draggingNode.value) return
  
  const svg = svgCanvas.value
  if (!svg) return
  
  const rect = svg.getBoundingClientRect()
  draggingNode.value.x = event.clientX - rect.left - dragOffset.x
  draggingNode.value.y = event.clientY - rect.top - dragOffset.y
}

function stopDrag() {
  draggingNode.value = null
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', stopDrag)
}

// 选择节点
function selectNode(node: WorkflowNode) {
  selectedNode.value = node
}

// 点击画布取消选择
function handleCanvasClick() {
  selectedNode.value = null
}

// 获取边路径 (贝塞尔曲线)
function getEdgePath(sourceId: string, targetId: string): string {
  const source = props.workflow.nodes.find(n => n.id === sourceId)
  const target = props.workflow.nodes.find(n => n.id === targetId)
  
  if (!source || !target) return ''
  
  const sourceX = source.x + (source.width || 180)
  const sourceY = source.y + (source.height || 80) / 2
  const targetX = target.x
  const targetY = target.y + (target.height || 80) / 2
  
  const controlOffset = Math.abs(targetX - sourceX) / 2
  
  return `M ${sourceX} ${sourceY} C ${sourceX + controlOffset} ${sourceY}, ${targetX - controlOffset} ${targetY}, ${targetX} ${targetY}`
}

// 获取节点边框颜色
function getNodeStrokeColor(type: string): string {
  const colorMap: Record<string, string> = {
    llm_call: '#8b5cf6',
    tool_execution: '#3b82f6',
    condition: '#f59e0b',
    loop: '#ec4899',
    end: '#ef4444',
    agent: '#6366f1',
  }
  return colorMap[type] || '#6b7280'
}

// 获取节点图标
function getNodeIcon(type: string): string {
  const iconMap: Record<string, string> = {
    llm_call: '🧠',
    tool_execution: '🔧',
    condition: '❓',
    loop: '🔄',
    end: '⏹️',
    agent: '🤖',
  }
  return iconMap[type] || '📦'
}

// 获取状态颜色
function getStatusColor(status: string): string {
  const colorMap: Record<string, string> = {
    pending: '#9ca3af',
    running: '#3b82f6',
    completed: '#10b981',
    error: '#ef4444',
  }
  return colorMap[status] || '#9ca3af'
}

// 自动布局
function autoLayout() {
  message.info('自动布局功能开发中...')
}

// 导出 JSON
function exportJSON() {
  emit('export', props.workflow)
  message.success('工作流已导出')
}

// 导入 JSON
function importJSON() {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = '.json'
  input.onchange = (e) => {
    const file = (e.target as HTMLInputElement).files?.[0]
    if (!file) return
    
    const reader = new FileReader()
    reader.onload = (e) => {
      try {
        const workflow = JSON.parse(e.target?.result as string)
        emit('update:workflow', workflow)
        message.success('工作流已导入')
      } catch (error) {
        message.error('导入失败: 无效的 JSON 文件')
      }
    }
    reader.readAsText(file)
  }
  input.click()
}

// 验证工作流
function validateWorkflow() {
  const hasCycles = detectCycle(props.workflow)
  if (hasCycles) {
    message.error('验证失败: 检测到循环依赖')
    return
  }
  
  const missingEdges = props.workflow.nodes.filter(
    node => !props.workflow.edges.some(e => e.source === node.id || e.target === node.id)
  )
  
  if (missingEdges.length > 0) {
    message.warning(`未连接的节点: ${missingEdges.map(n => n.title).join(', ')}`)
  } else {
    message.success('工作流验证通过')
  }
}

// 检测环
function detectCycle(workflow: Workflow): boolean {
  const visited = new Set<string>()
  const recStack = new Set<string>()
  
  function dfs(nodeId: string): boolean {
    visited.add(nodeId)
    recStack.add(nodeId)
    
    const connected = workflow.edges.filter(e => e.source === nodeId)
    for (const edge of connected) {
      if (!visited.has(edge.target)) {
        if (dfs(edge.target)) return true
      } else if (recStack.has(edge.target)) {
        return true
      }
    }
    
    recStack.delete(nodeId)
    return false
  }
  
  for (const node of workflow.nodes) {
    if (!visited.has(node.id)) {
      if (dfs(node.id)) return true
    }
  }
  
  return false
}

// 保存工作流
function saveWorkflow() {
  validateWorkflow()
  emit('save', props.workflow)
  message.success('工作流已保存')
}

// ── Agent 配置（node_type === 'agent'） ──
const agentList = ref<any[]>([])
const agentLoading = ref(false)
const agentLoadFailed = ref(false)
const editAgentId = ref('')
const editAgentName = ref('')
const editAgentSystemPrompt = ref('')
const editAgentModel = ref('')
const editAgentMaxTurns = ref(5)
const editAgentTask = ref('')
// 取不到 Agent 列表时回退手动输入
const agentManualMode = computed(() =>
  agentLoadFailed.value || (!agentLoading.value && agentList.value.length === 0)
)

async function loadAgentList() {
  agentLoading.value = true
  agentLoadFailed.value = false
  try {
    agentList.value = await listAgents()
  } catch {
    agentList.value = []
    agentLoadFailed.value = true
  } finally {
    agentLoading.value = false
  }
}

// 选中 Agent 后自动填入 name/system_prompt/model/max_turns（可编辑覆盖）
function onAgentSelect() {
  const agent = agentList.value.find(a => a.id === editAgentId.value)
  if (!agent) return
  editAgentId.value = agent.id
  editAgentName.value = agent.name || ''
  editAgentSystemPrompt.value = agent.system_prompt || ''
  editAgentModel.value = String(agent.llm_config?.model || editAgentModel.value)
  editAgentMaxTurns.value = agent.max_turns || 5
}

// 选中节点变化 → 载入表单（type 在面板内可改，监听其变化）
watch([selectedNode, () => selectedNode.value?.type], () => {
  const node = selectedNode.value
  if (!node) return
  const cfg = node.config || {}
  editAgentId.value = cfg.agent_id || ''
  editAgentName.value = cfg.name || ''
  editAgentSystemPrompt.value = cfg.system_prompt || ''
  editAgentModel.value = cfg.model || ''
  editAgentMaxTurns.value = cfg.max_turns || 5
  editAgentTask.value = cfg.task || ''
})

// Agent 表单编辑 → 实时写回 node.config（随工作流 JSON 持久化）
watch([editAgentId, editAgentName, editAgentSystemPrompt, editAgentModel, editAgentMaxTurns, editAgentTask], () => {
  const node = selectedNode.value
  if (!node || node.type !== 'agent') return
  node.config = {
    ...(node.config || {}),
    agent_id: editAgentId.value || undefined,
    name: editAgentName.value || undefined,
    system_prompt: editAgentSystemPrompt.value || undefined,
    model: editAgentModel.value || undefined,
    max_turns: editAgentMaxTurns.value > 0 ? editAgentMaxTurns.value : undefined,
    task: editAgentTask.value || undefined,
  }
})

onMounted(() => {
  loadAgentList()
})
</script>

<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';
@import '@vue-flow/controls/dist/style.css';
@import '@vue-flow/minimap/dist/style.css';
</style>

<style scoped>
.workflow-dag-editor {
  display: flex;
  flex-direction: column;
  height: 600px;
  min-height: 480px;
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  overflow: hidden;
  background: var(--bg-card);
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
}

.toolbar-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 38px;
  padding: 8px 14px;
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-card);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all 0.2s;
}

.toolbar-btn:hover {
  background: var(--bg-hover);
  border-color: var(--text-tertiary);
  color: var(--text-primary);
}

.btn-validate {
  color: var(--warning);
  border-color: var(--warning);
}

.btn-validate:hover {
  background: rgba(245, 158, 11, 0.12);
  color: var(--warning);
  border-color: var(--warning);
}

.btn-save {
  color: #ffffff;
  background: var(--primary);
  border-color: var(--primary);
}

.btn-save:hover {
  background: var(--primary-dark);
  border-color: var(--primary-dark);
  color: #ffffff;
}

.canvas-container {
  flex: 1;
  position: relative;
  overflow: hidden;
  min-height: 400px;
}

.dag-canvas {
  width: 100%;
  height: 100%;
  min-width: 100%;
  background: var(--bg-page);
  cursor: grab;
  display: block;
}

.dag-canvas:active {
  cursor: moving;
}

.edge-path {
  stroke: var(--text-tertiary);
  transition: stroke 0.2s;
}

.edge-path:hover {
  stroke: var(--primary);
  stroke-width: 2.5;
}

/* 箭头标记跟随主题 */
#arrowhead polygon {
  fill: var(--text-tertiary);
}

.dag-node {
  cursor: move;
}

.dag-node.selected rect {
  stroke-width: 3;
  filter: drop-shadow(0 4px 12px rgba(59, 130, 246, 0.3));
}

.dag-node.error rect {
  stroke: var(--error);
  stroke-dasharray: 4 2;
}

/* SVG 节点文本：跟随主题变量 */
.node-title {
  fill: var(--text-primary);
  font-family: var(--font-sans);
}

.node-desc {
  fill: var(--text-tertiary);
  font-family: var(--font-sans);
}

.skill-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  font-size: 9px;
  color: #ffffff;
  background: var(--primary);
  border-radius: 2px;
}

.properties-panel {
  position: absolute;
  top: 0;
  right: 0;
  width: 300px;
  max-width: 85vw;
  height: 100%;
  background: var(--bg-card);
  border-left: 1px solid var(--border);
  box-shadow: var(--shadow-lg);
  z-index: 10;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.btn-close {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  font-size: 16px;
  color: var(--text-tertiary);
  background: none;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
}

.btn-close:hover {
  color: var(--text-primary);
  background: var(--bg-hover);
}

.panel-body {
  padding: 16px;
  overflow-y: auto;
  max-height: calc(100% - 48px);
}

.form-group {
  margin-bottom: 12px;
}

.form-group label {
  display: block;
  margin-bottom: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.form-input,
.form-select,
.form-textarea {
  width: 100%;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--text-primary);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
  font-family: var(--font-sans);
}

.form-input:focus,
.form-select:focus,
.form-textarea:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 2px var(--primary-bg);
}

.form-actions {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}

.btn-delete,
.btn-duplicate {
  flex: 1;
  min-height: 36px;
  padding: 8px 12px;
  font-size: 12px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid;
}

.btn-delete {
  color: var(--error);
  background: var(--bg-card);
  border-color: var(--error);
}

.btn-delete:hover {
  background: rgba(239, 68, 68, 0.1);
}

.btn-duplicate {
  color: var(--text-secondary);
  background: var(--bg-card);
  border-color: var(--border);
}

.btn-duplicate:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.section-label {
  margin: 4px 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}

.agent-hint {
  margin-top: 4px;
  font-size: 11px;
  color: var(--warning);
}

/* 移动端：画布保持可拖拽（min-height + 横向滚动提示），工具栏换行 */
@media (max-width: 768px) {
  .workflow-dag-editor {
    height: auto;
    min-height: 560px;
  }
  .toolbar {
    gap: 6px;
    padding: 10px 12px;
  }
  .canvas-container {
    min-height: 440px;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }
  .dag-canvas {
    min-width: 640px;
  }
  /* 横向滚动提示（纯 CSS，pointer-events none 不挡交互） */
  .canvas-container::after {
    content: '↔ 可横向拖动 · 点击节点编辑';
    position: absolute;
    left: 50%;
    bottom: 10px;
    transform: translateX(-50%);
    z-index: 5;
    pointer-events: none;
    padding: 4px 12px;
    border-radius: var(--radius-full);
    background: var(--bg-card);
    border: 1px solid var(--border);
    color: var(--text-tertiary);
    font-size: 11px;
    white-space: nowrap;
    box-shadow: var(--shadow-md);
    opacity: 0.92;
  }
  .properties-panel {
    width: min(300px, 88vw);
  }
}

@media (max-width: 480px) {
  .toolbar-btn {
    flex: 1 1 auto;
    justify-content: center;
    padding: 8px 10px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .toolbar-btn,
  .btn-delete,
  .btn-duplicate,
  .edge-path {
    transition: none;
  }
}
</style>
