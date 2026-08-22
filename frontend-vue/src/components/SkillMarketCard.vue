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
    
    <!-- 空状态：统一 EmptyState 模式 -->
    <EmptyState
      v-if="filteredSkills.length === 0"
      size="list"
      :icon="markRaw(SearchOutlined)"
      description="暂无技能"
      hint="尝试调整搜索或筛选条件"
    />
    
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
import { ref, computed, markRaw } from 'vue'
import { Modal, message } from 'ant-design-vue'
import { SearchOutlined, FileTextOutlined, EditOutlined, CodeOutlined, DatabaseOutlined, ToolOutlined } from '@ant-design/icons-vue'
import { api } from '../api'
import EmptyState from './common/EmptyState.vue'

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
  // 根据技能名称返回真实图标组件（修复：字符串形式无法被 <component :is> 解析）
  const iconMap: Record<string, any> = {
    read_file: FileTextOutlined,
    write_file: EditOutlined,
    shell_exec: CodeOutlined,
    grep_files: SearchOutlined,
    kb_search: DatabaseOutlined,
  }
  return iconMap[name] || ToolOutlined
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
  min-width: 0;
  min-height: 38px;
  padding: 8px 12px;
  font-size: 14px;
  color: var(--text-primary);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.search-input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 2px var(--primary-bg);
}

.search-input::placeholder {
  color: var(--text-muted);
}

.filter-select {
  min-height: 38px;
  padding: 8px 12px;
  font-size: 14px;
  color: var(--text-primary);
  background: var(--bg-input);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  cursor: pointer;
  outline: none;
}

.filter-select:focus {
  border-color: var(--primary);
}

.skill-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.skill-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
  border: 1px solid var(--border-card);
  border-radius: var(--radius-lg);
  background: var(--bg-card);
  box-shadow: var(--shadow-md);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.skill-card:hover {
  transform: translateY(-2px);
  border-color: var(--primary);
  box-shadow: var(--shadow-lg);
}

.skill-card.registered {
  border-color: var(--success);
  background: linear-gradient(to bottom right, rgba(34, 197, 94, 0.08), var(--bg-card));
}

.skill-icon {
  width: 48px;
  height: 48px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  font-size: 24px;
  flex-shrink: 0;
}

.skill-info {
  flex: 1;
  min-width: 0;
}

.skill-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
  margin-bottom: 4px;
  overflow-wrap: anywhere;
}

.skill-desc {
  font-size: 12px;
  color: var(--text-secondary);
  line-height: 1.5;
  margin-bottom: 8px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
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
  background: var(--text-tertiary);
  border-radius: 3px;
}

.skill-status {
  margin-bottom: 2px;
}

.badge-registered {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  background: var(--success);
  border-radius: 3px;
}

.badge-available {
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  background: var(--text-tertiary);
  border-radius: 3px;
}

.skill-actions {
  display: flex;
  gap: 8px;
  margin-top: auto;
}

.btn-detail,
.btn-register,
.btn-unregister {
  flex: 1;
  min-height: 38px;
  padding: 8px 12px;
  font-size: 12px;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid;
}

.btn-detail {
  color: var(--text-secondary);
  background: var(--bg-card);
  border-color: var(--border);
}

.btn-detail:hover {
  background: var(--bg-hover);
  border-color: var(--text-tertiary);
  color: var(--text-primary);
}

.btn-register {
  color: white;
  background: var(--success);
  border-color: var(--success);
}

.btn-register:hover {
  filter: brightness(0.92);
}

.btn-unregister {
  color: white;
  background: var(--error);
  border-color: var(--error);
}

.btn-unregister:hover {
  filter: brightness(0.92);
}

.skill-detail {
  max-height: 400px;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

.detail-section {
  margin-bottom: 16px;
}

.detail-section h4 {
  margin: 0 0 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
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
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
}

.param-name {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

.param-type {
  padding: 2px 6px;
  font-size: 10px;
  color: #ffffff;
  background: var(--text-tertiary);
  border-radius: 3px;
  flex-shrink: 0;
}

.param-required {
  padding: 2px 6px;
  font-size: 10px;
  color: #ffffff;
  background: var(--error);
  border-radius: 3px;
  flex-shrink: 0;
}

.empty-params {
  margin: 0;
  font-size: 12px;
  color: var(--text-tertiary);
  font-style: italic;
}

.metadata-json {
  margin: 0;
  padding: 8px 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.5;
  color: var(--text-secondary);
  white-space: pre-wrap;
  word-break: break-word;
}

/* 移动端：搜索/筛选竖排、网格单列 */
@media (max-width: 576px) {
  .skill-market {
    padding: 12px;
  }
  .search-bar {
    flex-direction: column;
    gap: 8px;
  }
  .skill-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .skill-card,
  .btn-detail,
  .btn-register,
  .btn-unregister {
    transition: none;
  }
}
</style>
