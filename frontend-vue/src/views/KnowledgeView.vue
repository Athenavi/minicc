<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import {
  NCard, NButton, NModal, NForm, NFormItem, NInput, NSelect,
  NRadioGroup, NRadioButton, NSpin, NEmpty, NIcon, NTag,
  NSpace, NPopconfirm, useMessage
} from 'naive-ui'
import { BookOutline, AddOutline, TrashOutline } from '@vicons/ionicons5'
import { api } from '../api'

interface KnowledgeBase {
  id: string
  name: string
  description: string
  type: 'wiki' | 'rag'
  visibility: 'public' | 'private'
  status: string
  document_count: number
  total_size_bytes: number
  credits_consumed: number
  created_at: string
  updated_at: string
}

const router = useRouter()
const message = useMessage()
const loading = ref(true)
const knowledgeBases = ref<KnowledgeBase[]>([])
const showCreateModal = ref(false)

// 创建表单
const createForm = ref({
  name: '',
  description: '',
  type: 'wiki' as 'wiki' | 'rag',
  visibility: 'private' as 'public' | 'private',
})

onMounted(() => {
  loadKnowledgeBases()
})

async function loadKnowledgeBases() {
  try {
    loading.value = true
    const res = await api.get('/v1/kb')
    knowledgeBases.value = res.data?.data || []
  } catch (error) {
    console.error('加载知识库失败:', error)
  } finally {
    loading.value = false
  }
}

async function createKnowledgeBase() {
  if (!createForm.value.name.trim()) {
    message.warning('请输入知识库名称')
    return
  }

  try {
    await api.post('/v1/kb', createForm.value)
    message.success('知识库创建成功')
    showCreateModal.value = false
    createForm.value = { name: '', description: '', type: 'wiki', visibility: 'private' }
    await loadKnowledgeBases()
  } catch (error: any) {
    message.error(error.response?.data?.error || '创建失败')
  }
}

async function deleteKnowledgeBase(id: string) {
  try {
    await api.delete(`/v1/kb/${id}`)
    message.success('已删除')
    await loadKnowledgeBases()
  } catch (error: any) {
    message.error(error.response?.data?.error || '删除失败')
  }
}

function openKnowledgeBase(kb: KnowledgeBase) {
  router.push(`/knowledge/${kb.id}`)
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatDate(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

const publicKbs = computed(() => knowledgeBases.value.filter(kb => kb.visibility === 'public'))
const privateKbs = computed(() => knowledgeBases.value.filter(kb => kb.visibility === 'private'))
</script>

<template>
  <div class="kb-container">
    <div class="kb-header">
      <div class="header-left">
        <NIcon size="24" color="#2080f0">
          <BookOutline />
        </NIcon>
        <h1>知识库</h1>
      </div>
      <NButton type="primary" @click="showCreateModal = true">
        <template #icon>
          <NIcon><AddOutline /></NIcon>
        </template>
        创建知识库
      </NButton>
    </div>

    <NSpin :show="loading">
      <div v-if="!loading && knowledgeBases.length === 0" class="empty-state">
        <NEmpty description="暂无知识库">
          <template #extra>
            <NButton type="primary" @click="showCreateModal = true">创建第一个知识库</NButton>
          </template>
        </NEmpty>
      </div>

      <div v-else class="kb-sections">
        <!-- 私人知识库 -->
        <div v-if="privateKbs.length > 0" class="kb-section">
          <h2>📁 我的知识库</h2>
          <div class="kb-grid">
            <NCard
              v-for="kb in privateKbs"
              :key="kb.id"
              class="kb-card"
              hoverable
              @click="openKnowledgeBase(kb)"
            >
              <div class="kb-card-header">
                <span class="kb-name">{{ kb.name }}</span>
                <NTag :type="kb.type === 'rag' ? 'success' : 'info'" size="small">
                  {{ kb.type.toUpperCase() }}
                </NTag>
              </div>
              <p class="kb-desc">{{ kb.description || '暂无描述' }}</p>
              <div class="kb-stats">
                <span>📄 {{ kb.document_count }} 文档</span>
                <span>💾 {{ formatSize(kb.total_size_bytes) }}</span>
                <span>💰 {{ kb.credits_consumed }} credits</span>
              </div>
              <div class="kb-footer">
                <span class="kb-time">{{ formatDate(kb.updated_at) }}</span>
                <NPopconfirm @positive-click.stop="deleteKnowledgeBase(kb.id)">
                  <template #trigger>
                    <NButton size="tiny" type="error" quaternary @click.stop>
                      <NIcon><TrashOutline /></NIcon>
                    </NButton>
                  </template>
                  确认删除此知识库？
                </NPopconfirm>
              </div>
            </NCard>
          </div>
        </div>

        <!-- 公共知识库 -->
        <div v-if="publicKbs.length > 0" class="kb-section">
          <h2>🌐 公共知识库</h2>
          <div class="kb-grid">
            <NCard
              v-for="kb in publicKbs"
              :key="kb.id"
              class="kb-card public"
              hoverable
              @click="openKnowledgeBase(kb)"
            >
              <div class="kb-card-header">
                <span class="kb-name">{{ kb.name }}</span>
                <NTag type="warning" size="small">公共</NTag>
              </div>
              <p class="kb-desc">{{ kb.description || '暂无描述' }}</p>
              <div class="kb-stats">
                <span>📄 {{ kb.document_count }} 文档</span>
                <span>💾 {{ formatSize(kb.total_size_bytes) }}</span>
              </div>
            </NCard>
          </div>
        </div>
      </div>
    </NSpin>

    <!-- 创建知识库弹窗 -->
    <NModal v-model:show="showCreateModal" preset="card" title="创建知识库" style="max-width: 500px">
      <NForm>
        <NFormItem label="名称" required>
          <NInput v-model:value="createForm.name" placeholder="输入知识库名称" />
        </NFormItem>
        <NFormItem label="描述">
          <NInput v-model:value="createForm.description" type="textarea" placeholder="输入描述（可选）" />
        </NFormItem>
        <NFormItem label="索引方式">
          <NRadioGroup v-model:value="createForm.type">
            <NRadioButton value="wiki" label="Wiki（全文搜索）" />
            <NRadioButton value="rag" label="RAG（语义检索）" />
          </NRadioGroup>
        </NFormItem>
        <NFormItem label="可见性">
          <NRadioGroup v-model:value="createForm.visibility">
            <NRadioButton value="private" label="私人" />
            <NRadioButton value="public" label="公共" />
          </NRadioGroup>
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreateModal = false">取消</NButton>
          <NButton type="primary" @click="createKnowledgeBase">创建</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.kb-container {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.kb-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.empty-state {
  display: flex;
  justify-content: center;
  padding: 80px 0;
}

.kb-section {
  margin-bottom: 32px;
}

.kb-section h2 {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 16px;
  color: #333;
}

.kb-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.kb-card {
  cursor: pointer;
  transition: all 0.2s;
}

.kb-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.kb-card.public {
  border-left: 3px solid #f59e0b;
}

.kb-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.kb-name {
  font-weight: 600;
  font-size: 16px;
}

.kb-desc {
  color: #666;
  font-size: 14px;
  margin: 0 0 12px;
  line-height: 1.5;
}

.kb-stats {
  display: flex;
  gap: 16px;
  font-size: 13px;
  color: #888;
}

.kb-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid #f0f0f0;
}

.kb-time {
  font-size: 12px;
  color: #999;
}
</style>
