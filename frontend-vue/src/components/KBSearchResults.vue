<!-- KBSearchResults - 知识库 RAG 检索结果卡片 -->
<template>
  <div class="kb-search-results">
    <div class="results-header">
      <div class="header-title">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path d="M2 2h12v12H2V2zm1 1v10h10V3H3z"/>
          <path d="M5 5h6v1H5V5zm0 2h6v1H5V7zm0 2h4v1H5V9z"/>
        </svg>
        知识库检索结果 ({{ results.length }} 条)
      </div>
      <div class="header-actions">
        <button @click="toggleAll" class="btn-toggle">
          {{ isExpanded ? '收起全部' : '展开全部' }}
        </button>
      </div>
    </div>
    
    <div class="results-list">
      <div
        v-for="(result, index) in results"
        :key="index"
        class="result-card"
        :class="{ expanded: isExpanded || result.expanded }"
      >
        <!-- 排名 + 相似度分数 -->
        <div class="result-rank">
          <span class="rank-badge">#{{ index + 1 }}</span>
          <span class="score-badge" :style="{ backgroundColor: getScoreColor(result.score) }">
            相似度: {{ (result.score * 100).toFixed(1) }}%
          </span>
        </div>
        
        <!-- 文档标题 -->
        <div class="result-title">
          <svg class="doc-icon" width="14" height="14" viewBox="0 0 14 14" fill="currentColor">
            <path d="M2 1h7l3 3v9H2V1zm5 0v3h3L7 1z"/>
          </svg>
          <span>{{ result.documentName || `文档 ${index + 1}` }}</span>
        </div>
        
        <!-- 内容预览 -->
        <div class="result-content">
          <pre class="content-text">{{ getTruncatedContent(result.content) }}</pre>
          
          <!-- 高亮关键词 -->
          <div v-if="result.highlights && result.highlights.length" class="highlights">
            <span class="highlight-label">关键片段:</span>
            <span
              v-for="(highlight, hIdx) in result.highlights"
              :key="hIdx"
              class="highlight-tag"
            >
              {{ highlight }}
            </span>
          </div>
        </div>
        
        <!-- 来源信息 -->
        <div class="result-meta">
          <span class="meta-item">
            <svg width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
              <path d="M6 1a4 4 0 100 8 4 4 0 000-8zm0 6.5a2.5 2.5 0 110-5 2.5 2.5 0 010 5z"/>
            </svg>
            Chunk #{{ result.chunkId }}
          </span>
          <span class="meta-item" v-if="result.tenantId">
            租户: {{ result.tenantId.substring(0, 8) }}
          </span>
          <span class="meta-item" v-if="result.timestamp">
            {{ formatTime(result.timestamp) }}
          </span>
        </div>
        
        <!-- 展开/折叠按钮 -->
        <button
          class="expand-btn"
          @click.stop="toggleResult(result)"
          :class="{ collapsed: !isExpanded && !result.expanded }"
        >
          {{ isExpanded || result.expanded ? '收起' : '展开' }}
        </button>
      </div>
    </div>
    
    <!-- 空状态 -->
    <div v-if="results.length === 0" class="empty-state">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="#d1d5db">
        <circle cx="24" cy="24" r="20" fill="none" stroke="currentColor" stroke-width="2"/>
        <path d="M16 20h16M16 26h12M16 32h8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <p>暂无检索结果</p>
      <p class="empty-hint">尝试其他搜索关键词</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface SearchResult {
  documentId: string
  chunkId: string
  content: string
  score: number
  documentName?: string
  tenantId?: string
  timestamp?: string
  highlights?: string[]
  expanded?: boolean
}

const props = withDefaults(defineProps<{
  results: SearchResult[]
  maxPreviewLength?: number
}>(), {
  maxPreviewLength: 200,
})

const emit = defineEmits<{
  (e: 'select', result: SearchResult): void
  (e: 'reopen'): void
}>()

const isExpanded = ref(false)

function toggleAll() {
  isExpanded.value = !isExpanded.value
}

function toggleResult(result: SearchResult) {
  result.expanded = !result.expanded
}

function getTruncatedContent(content: string): string {
  if (!content) return ''
  const maxLength = props.maxPreviewLength
  return content.length > maxLength
    ? content.substring(0, maxLength) + '...'
    : content
}

function getScoreColor(score: number): string {
  if (score >= 0.9) return '#10b981' // 绿色 - 极高相似
  if (score >= 0.7) return '#3b82f6' // 蓝色 - 高相似
  if (score >= 0.5) return '#f59e0b' // 橙色 - 中等
  return '#ef4444'                    // 红色 - 低相似
}

function formatTime(timestamp: string | number): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>

<style scoped>
.kb-search-results {
  margin-top: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
  background: #ffffff;
}

.results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: linear-gradient(to right, #f3f4f6, #ffffff);
  border-bottom: 1px solid #e5e7eb;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: #1f2937;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.btn-toggle {
  padding: 4px 12px;
  font-size: 12px;
  color: #6b7280;
  background: white;
  border: 1px solid #d1d5db;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-toggle:hover {
  background: #f9fafb;
  border-color: #9ca3af;
}

.results-list {
  max-height: 500px;
  overflow-y: auto;
}

.result-card {
  padding: 16px;
  border-bottom: 1px solid #f3f4f6;
  transition: background 0.2s;
}

.result-card:last-child {
  border-bottom: none;
}

.result-card:hover {
  background: #f9fafb;
}

.result-rank {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.rank-badge {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 700;
  color: #ffffff;
  background: #6b7280;
  border-radius: 3px;
}

.score-badge {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  border-radius: 3px;
}

.result-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
  color: #111827;
}

.doc-icon {
  color: #6b7280;
}

.result-content {
  margin-bottom: 8px;
}

.content-text {
  margin: 0;
  padding: 8px 12px;
  background: #f9fafb;
  border-left: 3px solid #d1d5db;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 12px;
  line-height: 1.6;
  color: #374151;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 80px;
  overflow: hidden;
  transition: max-height 0.3s ease;
}

.result-card.expanded .content-text {
  max-height: 500px;
}

.highlights {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}

.highlight-label {
  font-size: 11px;
  color: #6b7280;
}

.highlight-tag {
  padding: 2px 8px;
  font-size: 11px;
  color: #ffffff;
  background: #3b82f6;
  border-radius: 3px;
}

.result-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: #9ca3af;
}

.expand-btn {
  padding: 4px 12px;
  font-size: 12px;
  color: #3b82f6;
  background: none;
  border: 1px solid #3b82f6;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}

.expand-btn:hover {
  background: #3b82f6;
  color: #ffffff;
}

.expand-btn.collapsed {
  color: #6b7280;
  border-color: #d1d5db;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  text-align: center;
}

.empty-state p {
  margin: 12px 0 4px;
  font-size: 14px;
  color: #9ca3af;
}

.empty-hint {
  font-size: 12px;
  color: #d1d5db;
}
</style>
