<!-- Unified CallChain Timeline — 聊天界面底部折叠面板 -->
<template>
  <div v-if="isVisible" class="callchain-timeline">
    <CollapseTransition>
      <div class="timeline-header" @click="toggle">
        <span class="header-title">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M8 1v14M1 8h14" stroke="currentColor" stroke-width="2"/>
          </svg>
          工具调用链 ({{ spanCount }} 个工具, 总耗时 {{ totalDurationMs }}ms)
        </span>
        <span class="header-icon">{{ isExpanded ? '▼' : '▶' }}</span>
      </div>

      <div v-if="isExpanded" class="timeline-body">
        <!-- 时间线渲染 -->
        <div v-for="(span, idx) in displaySpans" :key="idx" class="timeline-node">
          <div class="node-line" :class="getNodeClass(span)">
            <span class="node-icon">{{ getNodeIcon(span) }}</span>
            <div class="node-content">
              <div class="node-header">
                <span class="node-name">{{ getSpanDisplayName(span) }}</span>
                <span class="node-duration">{{ span.duration_ms }}ms</span>
              </div>
              
              <!-- 展开详情 -->
              <div v-if="span.showDetails" class="node-details">
                <pre class="metadata-json">{{ formatMetadata(span.metadata) }}</pre>
              </div>
            </div>
            
            <button class="details-toggle" @click.stop="span.showDetails = !span.showDetails">
              {{ span.showDetails ? '收起' : '详情' }}
            </button>
          </div>
        </div>

        <!-- 完成事件 -->
        <div class="timeline-node complete">
          <CheckCircleOutlined style="color: #52c41a" />
          <span>推理完成 (总耗时 {{ totalDurationMs }}ms)</span>
        </div>
      </div>
    </CollapseTransition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { CheckCircleOutlined } from '@ant-design/icons-vue'
import CollapseTransition from '@/components/CollapseTransition.vue'
import { api } from '../api'

interface TraceSpan {
  trace_id: string
  span_name: string
  duration_ms: number
  timestamp: string
  tenant_id: string
  metadata?: Record<string, any>
  showDetails?: boolean
}

const props = defineProps<{
  /** 兼容旧用法：直接传入 spans */
  visible?: boolean
  spans?: TraceSpan[]
  /** 新用法：传 traceId 由组件自行拉取 GET /v1/traces/{trace_id} */
  traceId?: string
  tenantId?: string
}>()

const emit = defineEmits<{
  (e: 'select', span: TraceSpan): void
}>()

const fetchedSpans = ref<TraceSpan[]>([])
const fetching = ref(false)

// traceId 模式：拉取该 trace 的全部 span
watch(() => props.traceId, async (id) => {
  if (!id) {
    fetchedSpans.value = []
    return
  }
  fetching.value = true
  try {
    const response = await api.get(`/v1/traces/${encodeURIComponent(id)}`)
    const payload = response.data?.data ?? response.data
    fetchedSpans.value = (payload?.spans || []).map((s: any) => ({ ...s, showDetails: false }))
  } catch (error) {
    console.warn('CallChainTimeline: 获取 trace 失败:', error)
    fetchedSpans.value = []
  } finally {
    fetching.value = false
  }
}, { immediate: true })

// 展示用的 spans：traceId 模式用拉取结果，否则用传入的 spans
const displaySpans = computed<TraceSpan[]>(() =>
  props.traceId ? fetchedSpans.value : (props.spans || [])
)

const isVisible = computed(() =>
  props.traceId ? displaySpans.value.length > 0 : !!props.visible
)

const isExpanded = ref(false)

function toggle() {
  isExpanded.value = !isExpanded.value
}

const spanCount = computed(() => displaySpans.value.filter(s => s.span_name.startsWith('tool:')).length)
const totalDurationMs = computed(() => displaySpans.value.reduce((sum, s) => sum + s.duration_ms, 0))

function getNodeClass(span: TraceSpan): string {
  if (span.span_name === 'llm_call') return 'type-llm'
  if (span.span_name.startsWith('tool:')) return 'type-tool'
  if (span.span_name.includes('workflow')) return 'type-workflow'
  return 'type-default'
}

function getNodeIcon(span: TraceSpan): string {
  if (span.span_name === 'llm_call') return '🧠'
  if (span.span_name.startsWith('tool:')) {
    const toolName = span.span_name.replace('tool:', '')
    const iconMap: Record<string, string> = {
      read_file: '📄',
      write_file: '✏️',
      shell_exec: '💻',
      grep_files: '🔍',
      workflow_run: '⚙️',
    }
    return iconMap[toolName] || '🔧'
  }
  return '📊'
}

function getSpanDisplayName(span: TraceSpan): string {
  if (span.span_name === 'llm_call') {
    const model = span.metadata?.model || 'unknown'
    const inputTokens = span.metadata?.input_tokens || 0
    return `LLM 调用 (${model}, ${inputTokens} tokens)`
  }
  if (span.span_name.startsWith('tool:')) {
    const toolName = span.span_name.replace('tool:', '')
    return capitalize(toolName.replace(/_/g, ' '))
  }
  return span.span_name
}

function formatMetadata(metadata?: Record<string, any>): string {
  if (!metadata) return '{}'
  return JSON.stringify(metadata, null, 2)
}

function capitalize(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1)
}
</script>

<style scoped>
.callchain-timeline {
  margin-top: 12px;
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  overflow: hidden;
}

.timeline-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: #fafafa;
  cursor: pointer;
  user-select: none;
  transition: background 0.2s;
}

.timeline-header:hover {
  background: #f0f0f0;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 500;
  color: #333;
}

.header-icon {
  font-size: 12px;
  color: #999;
}

.timeline-body {
  padding: 12px;
  max-height: 400px;
  overflow-y: auto;
}

.timeline-node {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
}

.complete {
  color: #52c41a;
  font-weight: 500;
}

.node-line {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  background: #fff;
  border: 1px solid #e8e8e8;
  border-left: 3px solid #d9d9d9;
  border-radius: 4px;
  transition: all 0.2s;
}

.node-line:hover {
  background: #f9f9f9;
}

.node-line.type-llm {
  border-left-color: #8b5cf6;
}

.node-line.type-tool {
  border-left-color: #3b82f6;
}

.node-line.type-workflow {
  border-left-color: #ec4899;
}

.node-icon {
  font-size: 18px;
}

.node-content {
  flex: 1;
}

.node-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.node-name {
  font-size: 13px;
  font-weight: 500;
  color: #333;
}

.node-duration {
  font-size: 12px;
  color: #999;
}

.node-details {
  margin-top: 8px;
}

.metadata-json {
  background: #f5f5f5;
  padding: 8px 12px;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  line-height: 1.5;
  overflow-x: auto;
  max-height: 200px;
  overflow-y: auto;
}

.details-toggle {
  padding: 4px 8px;
  font-size: 12px;
  color: #1890ff;
  background: none;
  border: 1px solid #1890ff;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}

.details-toggle:hover {
  background: #1890ff;
  color: #fff;
}
</style>
