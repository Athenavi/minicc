<!-- SkillMarketCard - 技能市场卡片组件 -->
<template>
  <div class="skill-market">
    <!-- 搜索框 -->
    <div class="search-bar">
      <input
        v-model="searchQuery"
        type="text"
        placeholder="搜索技能..."
        class="search-input"
        @input="handleSearch"
      />
      <select v-model="filterCategory" class="filter-select" @change="handleFilter">
        <option value="">全部分类</option>
        <option value="tool">工具</option>
        <option value="service">服务</option>
        <option value="template">模板</option>
        <option value="composite">组合能力</option>
      </select>
    </div>
    
    <!-- 技能网格 -->
    <div class="skill-grid">
      <div
        v-for="skill in filteredSkills"
        :key="skill.capabilityId"
        class="skill-card"
        :class="{ registered: skill.registered }"
      >
        <!-- 技能图标 -->
        <div class="skill-icon" :style="{ backgroundColor: getSkillColor(skill.category) }">
          <component :is="getSkillIcon(skill.name)" />
        </div>
        
        <!-- 技能信息 -->
        <div class="skill-info">
          <div class="skill-name">{{ skill.name }}</div>
          <div class="skill-desc">{{ skill.description }}</div>
          
          <!-- 标签 -->
          <div class="skill-tags">
            <span
              v-for="tag in skill.tags.slice(0, 3)"
              :key="tag"
              class="skill-tag"
            >
              {{ tag }}
            </span>
          </div>
        </div>
        
        <!-- 状态标识 -->
        <div class="skill-status">
          <span v-if="skill.registered" class="badge-registered">已注册</span>
          <span v-else class="badge-available">可用</span>
        </div>
        
        <!-- 操作按钮 -->
        <div class="skill-actions">
          <button @click="viewDetail(skill)" class="btn-detail">详情</button>
          <button
            v-if="!skill.registered"
            @click="registerSkill(skill)"
            class="btn-register"
          >
            注册
          </button>
          <button
            v-else
            @click="unregisterSkill(skill)"
            class="btn-unregister"
          >
            卸载
          </button>
        </div>
      </div>
    </div>
    
    <!-- 空状态 -->
    <div v-if="filteredSkills.length === 0" class="empty-state">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="#d1d5db">
        <circle cx="24" cy="24" r="20" fill="none" stroke="currentColor" stroke-width="2"/>
        <path d="M18 24h12M24 18v12" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <p>暂无技能</p>
      <p class="empty-hint">尝试调整搜索或筛选条件</p>
    </div>
    
    <!-- 详情弹窗 -->
    <Modal
      v-model:open="showDetailModal"
      :title="selectedSkill?.name"
      width="600px"
    >
      <div v-if="selectedSkill" class="skill-detail">
        <div class="detail-section">
          <h4>描述</h4>
          <p>{{ selectedSkill.description }}</p>
        </div>
        
        <div class="detail-section">
          <h4>输入参数</h4>
          <div v-if="selectedSkill.inputSchema.length" class="param-list">
            <div
              v-for="(param, pIdx) in selectedSkill.inputSchema"
              :key="pIdx"
              class="param-item"
            >
              <span class="param-name">{{ param.name }}</span>
              <span class="param-type">{{ param.type }}</span>
              <span class="param-required" v-if="param.required">必填</span>
            </div>
          </div>
          <p v-else class="empty-params">无</p>
        </div>
        
        <div class="detail-section">
          <h4>输出结果</h4>
          <div v-if="selectedSkill.outputSchema?.length" class="param-list">
            <div
              v-for="(result, rIdx) in selectedSkill.outputSchema"
              :key="rIdx"
              class="param-item"
            >
              <span class="param-name">{{ result.name }}</span>
              <span class="param-type">{{ result.type }}</span>
            </div>
          </div>
          <p v-else class="empty-params">无</p>
        </div>
        
        <div class="detail-section">
          <h4>元数据</h4>
          <pre class="metadata-json">{{ formatMetadata(selectedSkill) }}</pre>
        </div>
      </div>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Modal, message } from 'ant-design-vue'
import { api } from '../api'

interface CapabilityParam {
  name: string
  type: string
  required: boolean
  description?: string
}

interface Capability {
  capabilityId: string
  name: string
  description: string
  category: string // tool/service/template/composite
  tags: string[]
  inputSchema: CapabilityParam[]
  outputSchema?: CapabilityParam[]
  registered: boolean
  tenantId: string
}

const props = defineProps<{
  skills: Capability[]
}>()

const emit = defineEmits<{
  (e: 'skillRegistered', skill: Capability): void
  (e: 'skillUnregistered', skill: Capability): void
}>()

const searchQuery = ref('')
const filterCategory = ref('')
const showDetailModal = ref(false)
const selectedSkill = ref<Capability | null>(null)

const filteredSkills = computed(() => {
  let result = props.skills
  
  // 搜索过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    result = result.filter(s =>
      s.name.toLowerCase().includes(query) ||
      s.description.toLowerCase().includes(query) ||
      s.tags.some(t => t.toLowerCase().includes(query))
    )
  }
  
  // 分类过滤
  if (filterCategory.value) {
    result = result.filter(s => s.category === filterCategory.value)
  }
  
  return result
})

function handleSearch() {
  // 搜索逻辑已在 computed 中处理
}

function handleFilter() {
  // 过滤逻辑已在 computed 中处理
}

function viewDetail(skill: Capability) {
  selectedSkill.value = skill
  showDetailModal.value = true
}

async function registerSkill(skill: Capability) {
  try {
    // 调用注册 API
    await api.post(`/v1/skills/${skill.capabilityId}/register`)
    message.success(`技能 "${skill.name}" 注册成功`)
    skill.registered = true
    emit('skillRegistered', skill)
  } catch (error) {
    message.error(`注册失败: ${(error as any).message}`)
  }
}

async function unregisterSkill(skill: Capability) {
  try {
    await api.delete(`/v1/skills/${skill.capabilityId}`)
    message.success(`技能 "${skill.name}" 已卸载`)
    skill.registered = false
    emit('skillUnregistered', skill)
  } catch (error) {
    message.error(`卸载失败: ${(error as any).message}`)
  }
}

function getSkillColor(category: string): string {
  const colorMap: Record<string, string> = {
    tool: '#10b981',
    service: '#3b82f6',
    template: '#f59e0b',
    composite: '#8b5cf6',
  }
  return colorMap[category] || '#6b7280'
}

function getSkillIcon(name: string): any {
  // 根据技能名称返回不同的图标组件
  const iconMap: Record<string, any> = {
    read_file: 'DocumentOutlined',
    write_file: 'EditOutlined',
    shell_exec: 'CodeOutlined',
    grep_files: 'SearchOutlined',
    kb_search: 'DatabaseOutlined',
  }
  return iconMap[name] || 'ToolOutlined'
}

function formatMetadata(skill: Capability): string {
  return JSON.stringify({
    capabilityId: skill.capabilityId,
    tenantId: skill.tenantId,
    category: skill.category,
    tags: skill.tags,
  }, null, 2)
}
</script>

<style scoped>
.skill-market {
  padding: 16px;
}

.search-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.search-input {
  flex: 1;
  padding: 8px 12px;
  font-size: 14px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  outline: none;
  transition: border-color 0.2s;
}

.search-input:focus {
  border-color: #3b82f6;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
}

.filter-select {
  padding: 8px 12px;
  font-size: 14px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: white;
  cursor: pointer;
}

.skill-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.skill-card {
  position: relative;
  padding: 16px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: white;
  transition: all 0.2s;
}

.skill-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.skill-card.registered {
  border-color: #10b981;
  background: linear-gradient(to bottom right, #ecfdf5, #ffffff);
}

.skill-icon {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 24px;
  margin-bottom: 12px;
}

.skill-info {
  margin-bottom: 12px;
}

.skill-name {
  font-size: 15px;
  font-weight: 600;
  color: #111827;
  margin-bottom: 4px;
}

.skill-desc {
  font-size: 12px;
  color: #6b7280;
  line-height: 1.5;
  margin-bottom: 8px;
}

.skill-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.skill-tag {
  padding: 2px 6px;
  font-size: 10px;
  color: #ffffff;
  background: #9ca3af;
  border-radius: 2px;
}

.skill-status {
  margin-bottom: 8px;
}

.badge-registered {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  background: #10b981;
  border-radius: 3px;
}

.badge-available {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  background: #6b7280;
  border-radius: 3px;
}

.skill-actions {
  display: flex;
  gap: 8px;
}

.btn-detail,
.btn-register,
.btn-unregister {
  flex: 1;
  padding: 6px 12px;
  font-size: 12px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid;
}

.btn-detail {
  color: #6b7280;
  background: white;
  border-color: #d1d5db;
}

.btn-detail:hover {
  background: #f9fafb;
  border-color: #9ca3af;
}

.btn-register {
  color: white;
  background: #10b981;
  border-color: #10b981;
}

.btn-register:hover {
  background: #059669;
}

.btn-unregister {
  color: white;
  background: #ef4444;
  border-color: #ef4444;
}

.btn-unregister:hover {
  background: #dc2626;
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

.skill-detail {
  max-height: 400px;
  overflow-y: auto;
}

.detail-section {
  margin-bottom: 16px;
}

.detail-section h4 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: #374151;
}

.param-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.param-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background: #f9fafb;
  border-radius: 4px;
}

.param-name {
  flex: 1;
  font-size: 12px;
  font-weight: 600;
  color: #111827;
  font-family: 'Fira Code', monospace;
}

.param-type {
  padding: 2px 6px;
  font-size: 10px;
  color: #ffffff;
  background: #6b7280;
  border-radius: 2px;
}

.param-required {
  padding: 2px 6px;
  font-size: 10px;
  color: #ffffff;
  background: #ef4444;
  border-radius: 2px;
}

.empty-params {
  margin: 0;
  font-size: 12px;
  color: #9ca3af;
  font-style: italic;
}

.metadata-json {
  margin: 0;
  padding: 8px 12px;
  background: #f9fafb;
  border-radius: 4px;
  font-family: 'Fira Code', monospace;
  font-size: 11px;
  line-height: 1.5;
  color: #374151;
}
</style>
