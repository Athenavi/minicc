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
            <stop offset="0%" style="stop-color:#f9fafb;stop-opacity:1" />
            <stop offset="100%" style="stop-color:#f3f4f6;stop-opacity:1" />
          </linearGradient>
        </defs>
        
        <!-- 连线层 -->
        <g class="edges-layer">
          <path
            v-for="(edge, eIdx) in workflow.edges"
            :key="eIdx"
            :d="getEdgePath(edge.source, edge.target)"
            fill="none"
            stroke="#6b7280"
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
              fill="#111827"
              font-size="12"
              font-weight="600"
            >
              {{ node.title }}
            </text>
            
            <!-- 节点描述 -->
            <text
              :x="node.x + 16"
              :y="node.y + 40"
              fill="#6b7280"
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
          <button @click="selectedNode = null" class="btn-close">×</button>
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
import { ref, reactive } from 'vue'
import { message } from 'ant-design-vue'

interface WorkflowNode {
  id: string
  title: string
  description?: string
  type: string // llm_call | tool_execution | condition | loop | end
  x: number
  y: number
  width?: number
  height?: number
  status?: string // pending | running | completed | error
  boundSkill?: any
  boundSkillId?: string
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
</script>

<style scoped>
.workflow-dag-editor {
  display: flex;
  flex-direction: column;
  height: 600px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
}

.toolbar {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
}

.toolbar-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  font-size: 12px;
  color: #374151;
  background: white;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
}

.toolbar-btn:hover {
  background: #f3f4f6;
  border-color: #9ca3af;
}

.btn-validate {
  color: #f59e0b;
  border-color: #f59e0b;
}

.btn-validate:hover {
  background: #fef3c7;
}

.btn-save {
  color: #ffffff;
  background: #10b981;
  border-color: #10b981;
}

.btn-save:hover {
  background: #059669;
}

.canvas-container {
  flex: 1;
  position: relative;
  overflow: hidden;
}

.dag-canvas {
  width: 100%;
  height: 100%;
  background: #ffffff;
  cursor: grab;
}

.dag-canvas:active {
  cursor: moving;
}

.edge-path {
  transition: stroke 0.2s;
}

.edge-path:hover {
  stroke: #3b82f6;
  stroke-width: 2.5;
}

.dag-node {
  cursor: move;
}

.dag-node.selected rect {
  stroke-width: 3;
  filter: drop-shadow(0 4px 12px rgba(59, 130, 246, 0.3));
}

.dag-node.error rect {
  stroke: #ef4444;
  stroke-dasharray: 4 2;
}

.skill-badge {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  font-size: 9px;
  color: #ffffff;
  background: #3b82f6;
  border-radius: 2px;
}

.properties-panel {
  position: absolute;
  top: 0;
  right: 0;
  width: 300px;
  height: 100%;
  background: white;
  border-left: 1px solid #e5e7eb;
  box-shadow: -4px 0 12px rgba(0, 0, 0, 0.1);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
  font-size: 13px;
  font-weight: 600;
}

.btn-close {
  padding: 4px 8px;
  font-size: 16px;
  color: #9ca3af;
  background: none;
  border: none;
  cursor: pointer;
}

.btn-close:hover {
  color: #374151;
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
  color: #374151;
}

.form-input,
.form-select,
.form-textarea {
  width: 100%;
  padding: 8px 12px;
  font-size: 12px;
  border: 1px solid #d1d5db;
  border-radius: 4px;
  outline: none;
  transition: border-color 0.2s;
}

.form-input:focus,
.form-select:focus,
.form-textarea:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
}

.form-actions {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}

.btn-delete,
.btn-duplicate {
  flex: 1;
  padding: 8px 12px;
  font-size: 12px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid;
}

.btn-delete {
  color: #ef4444;
  background: white;
  border-color: #ef4444;
}

.btn-delete:hover {
  background: #fef2f2;
}

.btn-duplicate {
  color: #374151;
  background: white;
  border-color: #d1d5db;
}

.btn-duplicate:hover {
  background: #f3f4f6;
}
</style>
