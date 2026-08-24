<script setup lang="ts">
import { ref, onMounted, computed, markRaw } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Card, Button, Upload, Table, Spin, Empty, Tag, Space,
  Progress, message, Modal, Input, Checkbox, Popconfirm,
} from 'ant-design-vue'
import {
  ArrowLeftOutlined, CloudUploadOutlined,
  PlayCircleOutlined, PictureOutlined, SearchOutlined,
  MessageOutlined,
} from '@ant-design/icons-vue'
import { api, resolveMediaUrl } from '../api'
import { createChunkUpload } from '../utils/uploader'
import EmptyState from '../components/common/EmptyState.vue'

const API_URL = import.meta.env.VITE_API_URL || ''

/** 相对路径补 API_URL 前缀（resolveMediaUrl 返回的签名 URL 为 /media/s/... 相对路径） */
function absUrl(url: string): string {
  if (!url) return ''
  if (url.startsWith('http://') || url.startsWith('https://') || url.startsWith('data:')) return url
  return `${API_URL}${url.startsWith('/') ? '' : '/'}${url}`
}

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const building = ref(false)
const kb = ref<any>(null)
const documents = ref<any[]>([])
const showQueryModal = ref(false)
const queryText = ref('')
const queryResults = ref<any[]>([])
const buildProgress = ref(0)

// 文档列表列定义
const docColumns = [
  { title: '文件名', dataIndex: 'name', ellipsis: true },
  { title: '类型', dataIndex: 'file_type', width: 80 },
  {
    title: '大小', dataIndex: 'file_size_bytes', width: 100,
    customRender: ({ text }: { text: number }) => formatSize(text),
  },
  {
    title: '状态', dataIndex: 'status', width: 100,
  },
  { title: '分块数', dataIndex: 'chunk_count', width: 80 },
  {
    title: '上传时间', dataIndex: 'created_at', width: 160,
    customRender: ({ text }: { text: string }) => {
      if (!text) return ''
      const d = new Date(text)
      return isNaN(d.getTime()) ? '' : d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    },
  },
  {
    title: '操作', key: 'action', width: 90,
  },
]

// 文档搜索 + 批量删除
const docSearch = ref('')
const selectedDocIds = ref<string[]>([])
const deletingDocs = ref(false)

const filteredDocs = computed(() => {
  const q = docSearch.value.trim().toLowerCase()
  if (!q) return documents.value
  return documents.value.filter(d => (d.name || '').toLowerCase().includes(q) || (d.file_type || '').toLowerCase().includes(q))
})

async function deleteDoc(id: string) {
  try {
    await api.delete(`/v1/kb/${kbId}/documents`, { params: { doc_id: id } })
    message.success('已删除')
    await loadDocuments()
    await loadKnowledgeBase()
  } catch (e: any) {
    message.error(e.response?.data?.detail || e.response?.data?.error || '删除失败')
  }
}

async function batchDeleteDocs() {
  const ids = [...selectedDocIds.value]
  if (!ids.length) { message.warning('请先选择文档'); return }
  deletingDocs.value = true
  try {
    for (const id of ids) {
      await api.delete(`/v1/kb/${kbId}/documents`, { params: { doc_id: id } })
    }
    message.success(`已删除 ${ids.length} 个文档`)
    selectedDocIds.value = []
    await loadDocuments()
    await loadKnowledgeBase()
  } catch (e: any) {
    message.error(e.response?.data?.detail || e.response?.data?.error || '删除失败')
  } finally {
    deletingDocs.value = false
  }
}

// 媒体库相关
const showMediaModal = ref(false)
const mediaFiles = ref<any[]>([])
const selectedMediaIds = ref<string[]>([])
const loadingMedia = ref(false)
const importingMedia = ref(false)
const mediaSearchQuery = ref('')

const kbId = route.params.id as string

onMounted(async () => {
  await loadKnowledgeBase()
  await loadDocuments()
})

async function loadKnowledgeBase() {
  try {
    const res = await api.get(`/v1/kb/${kbId}`)
    kb.value = res.data?.data || res.data
  } catch (error) {
    message.error('加载知识库失败')
    router.push('/knowledge')
  } finally {
    loading.value = false
  }
}

async function loadDocuments() {
  try {
    const res = await api.get(`/v1/kb/${kbId}/documents`)
    documents.value = res.data?.data?.documents || []
  } catch (error) {
    console.error('加载文档失败:', error)
  }
}

// 过滤后的媒体文件
const filteredMediaFiles = computed(() => {
  if (!mediaSearchQuery.value.trim()) return mediaFiles.value
  const query = mediaSearchQuery.value.toLowerCase()
  return mediaFiles.value.filter((f: any) =>
    (f.name || '').toLowerCase().includes(query) ||
    (f.type || '').toLowerCase().includes(query)
  )
})

// 上传文档：全局分片（purpose kb_doc → complete 落 knowledge_documents）
async function handleUpload(info: any) {
  try {
    const handle = await createChunkUpload(info.file, { purpose: 'kb_doc', parentId: kbId })
    await handle.done
    message.success('文档上传成功')
    await loadKnowledgeBase()
    await loadDocuments()
  } catch (error: any) {
    message.error(error.response?.data?.detail || error.response?.data?.error || error.message || '上传失败')
  }
}

async function openMediaModal() {
  loadingMedia.value = true
  showMediaModal.value = true
  selectedMediaIds.value = []
  mediaSearchQuery.value = ''

  try {
    const res = await api.get('/v1/media')
    mediaFiles.value = res.data?.data?.items || []
  } catch (error) {
    message.error('加载媒体库失败')
  } finally {
    loadingMedia.value = false
  }
}

async function importFromMedia() {
  if (selectedMediaIds.value.length === 0) {
    message.warning('请选择要导入的文件')
    return
  }

  importingMedia.value = true
  let successCount = 0
  let failCount = 0

  for (const fileId of selectedMediaIds.value) {
    const file = mediaFiles.value.find((f: any) => f.id === fileId)
    if (!file) continue

    try {
      // 安全改造：/media/ 公开路径先解析为短时效签名 URL 再取字节（带归属校验）
      const signed = await resolveMediaUrl({ id: file.id, file_url: file.file_url })
      const response = await fetch(absUrl(signed || file.file_url))
      const blob = await response.blob()
      const mediaFile = new File([blob], file.name, { type: file.mime_type || '' })
      const handle = await createChunkUpload(mediaFile, { purpose: 'kb_doc', parentId: kbId })
      await handle.done
      successCount++
    } catch (error) {
      failCount++
      console.error(`导入失败: ${file.name}`, error)
    }
  }

  importingMedia.value = false
  showMediaModal.value = false
  selectedMediaIds.value = []

  if (successCount > 0) {
    message.success(`成功导入 ${successCount} 个文件`)
    await loadKnowledgeBase()
    await loadDocuments()
  }
  if (failCount > 0) {
    message.error(`${failCount} 个文件导入失败`)
  }
}

function toggleMediaSelection(id: string) {
  const index = selectedMediaIds.value.indexOf(id)
  if (index === -1) {
    selectedMediaIds.value.push(id)
  } else {
    selectedMediaIds.value.splice(index, 1)
  }
}

function selectAllMedia() {
  selectedMediaIds.value = filteredMediaFiles.value.map((f: any) => f.id)
}

function deselectAllMedia() {
  selectedMediaIds.value = []
}

async function buildKnowledgeBase() {
  try {
    building.value = true
    buildProgress.value = 0

    const res = await api.post(`/v1/kb/${kbId}/build`)
    const data = res.data?.data || res.data

    message.success(`构建已启动，预计消耗 ${data.estimated_cost} credits`)

    // 等待构建完成（轮询状态）
    const checkStatus = async () => {
      for (let i = 0; i < 10; i++) {
        await new Promise(resolve => setTimeout(resolve, 500))
        buildProgress.value = Math.min(90, (i + 1) * 10)
        await loadKnowledgeBase()
        if (kb.value?.status === 'active') {
          buildProgress.value = 100
          message.success('构建完成！')
          await loadDocuments()
          return
        }
      }
      await loadKnowledgeBase()
      await loadDocuments()
    }

    await checkStatus()
  } catch (error: any) {
    message.error(error.response?.data?.error || '构建失败')
  } finally {
    building.value = false
    buildProgress.value = 0
  }
}

async function queryKnowledgeBase() {
  if (!queryText.value.trim()) {
    message.warning('请输入查询内容')
    return
  }

  try {
    const res = await api.post(`/v1/kb/${kbId}/query`, {
      query: queryText.value,
      top_k: 5,
    })
    queryResults.value = res.data?.data?.results || []
    if (queryResults.value.length === 0) {
      message.info('未找到相关内容')
    }
  } catch (error: any) {
    message.error(error.response?.data?.error || '查询失败')
  }
}

// 互联互通：携带知识库上下文跳转到对话工作台
function askInChat() {
  router.push({ path: '/chat', query: { kb: kbId } })
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}
</script>

<template>
  <div class="kb-detail-container">
    <div class="kb-detail-header">
      <Button type="text" @click="router.push('/knowledge')">
        <template #icon><ArrowLeftOutlined /></template>
        返回
      </Button>
      <h1>{{ kb?.name || '知识库' }}</h1>
      <Space>
        <Button @click="askInChat" title="在对话中基于该知识库提问">
          <template #icon><MessageOutlined /></template>
          就此提问
        </Button>
        <Button @click="showQueryModal = true">查询知识库</Button>
        <Button
          v-if="kb?.status !== 'building'"
          type="primary"
          :loading="building"
          :disabled="building"
          @click="buildKnowledgeBase"
        >
          <template #icon><PlayCircleOutlined /></template>
          构建索引
        </Button>
        <Button
          v-else
          disabled
        >
          构建中...
        </Button>
      </Space>
    </div>

    <Spin :spinning="loading">
      <div v-if="kb" class="kb-info">
        <Card>
          <div class="info-grid">
            <div class="info-item">
              <span class="label">类型</span>
              <Tag :color="kb.type === 'rag' ? 'success' : 'blue'">
                {{ kb.type.toUpperCase() }}
              </Tag>
            </div>
            <div class="info-item">
              <span class="label">可见性</span>
              <Tag :color="kb.visibility === 'public' ? 'warning' : 'default'">
                {{ kb.visibility === 'public' ? '公共' : '私人' }}
              </Tag>
            </div>
            <div class="info-item">
              <span class="label">状态</span>
              <Tag :color="kb.status === 'active' ? 'success' : kb.status === 'building' ? 'processing' : 'error'">
                {{ kb.status }}
              </Tag>
            </div>
            <div class="info-item">
              <span class="label">文档数</span>
              <span>{{ kb.document_count }}</span>
            </div>
            <div class="info-item">
              <span class="label">总大小</span>
              <span>{{ formatSize(kb.total_size_bytes) }}</span>
            </div>
            <div class="info-item">
              <span class="label">已消耗</span>
              <span>{{ kb.credits_consumed }} credits</span>
            </div>
          </div>
          <p v-if="kb.description" class="kb-description">{{ kb.description }}</p>
        </Card>

        <!-- 构建进度 -->
        <Card v-if="building" title="构建进度" style="margin-top: 16px">
          <Progress :percent="buildProgress" status="active" />
        </Card>

        <!-- 文档列表 -->
        <Card title="文档管理" style="margin-top: 16px">
          <template #extra>
            <Space>
              <Input
                v-model:value="docSearch"
                placeholder="搜索文档"
                allow-clear
                size="small"
                style="width: 180px"
              >
                <template #prefix><SearchOutlined /></template>
              </Input>
              <Button
                v-if="selectedDocIds.length > 0"
                size="small"
                danger
                :loading="deletingDocs"
                @click="batchDeleteDocs"
              >
                <template #icon><DeleteOutlined /></template>
                批量删除（{{ selectedDocIds.length }}）
              </Button>
              <Button size="small" @click="openMediaModal">
                <template #icon><PictureOutlined /></template>
                从媒体库选取
              </Button>
              <Upload
                :show-upload-list="false"
                :custom-request="handleUpload"
                accept=".pdf,.md,.txt,.csv,.docx"
              >
                <Button type="primary" size="small">
                  <template #icon><CloudUploadOutlined /></template>
                  上传文档
                </Button>
              </Upload>
            </Space>
          </template>

          <EmptyState
            v-if="filteredDocs.length === 0"
            size="list"
            :icon="markRaw(CloudUploadOutlined)"
            description="暂无文档"
            hint="上传文档或从媒体库选取，构建索引后即可检索"
          />
          <Table
            v-else
            :columns="docColumns"
            :dataSource="filteredDocs"
            :pagination="false"
            :row-selection="{ selectedRowKeys: selectedDocIds, onChange: (keys: any[]) => (selectedDocIds = keys) }"
            row-key="id"
          >
            <template #bodyCell="{ column, text, record }">
              <template v-if="column.dataIndex === 'file_size_bytes'">
                {{ formatSize(text) }}
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="text === 'completed' ? 'success' : text === 'processing' ? 'processing' : text === 'error' ? 'error' : 'default'">
                  {{ text === 'pending' ? '待处理' : text === 'processing' ? '处理中' : text === 'completed' ? '已完成' : '失败' }}
                </Tag>
              </template>
              <template v-else-if="column.dataIndex === 'created_at'">
                {{ text ? new Date(text).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '' }}
              </template>
              <template v-else-if="column.key === 'action'">
                <Popconfirm title="确认删除此文档？" @confirm="deleteDoc(record.id)">
                  <Button type="text" danger size="small" title="删除文档">
                    <template #icon><DeleteOutlined /></template>
                  </Button>
                </Popconfirm>
              </template>
            </template>
          </Table>
        </Card>
      </div>
    </Spin>

    <!-- 查询弹窗 -->
    <Modal v-model:visible="showQueryModal" title="查询知识库" :footer="null" :style="{ maxWidth: '600px' }">
      <Input.TextArea
        v-model:value="queryText"
        placeholder="输入查询内容..."
        :rows="3"
      />
      <div class="modal-footer">
        <Button @click="showQueryModal = false">关闭</Button>
        <Button type="primary" @click="queryKnowledgeBase">查询</Button>
      </div>

      <!-- 查询结果 -->
      <div v-if="queryResults.length > 0" class="query-results">
        <h3>查询结果</h3>
        <div v-for="(result, index) in queryResults" :key="index" class="query-result-item">
          <div class="result-header">
            <Tag>相关度: {{ (result.score * 100).toFixed(1) }}%</Tag>
            <span v-if="result.name || result.document_name" class="result-source">📄 {{ result.name || result.document_name }}</span>
          </div>
          <p class="result-content">{{ result.content }}</p>
        </div>
      </div>
    </Modal>

    <!-- 从媒体库选取弹窗 -->
    <Modal v-model:visible="showMediaModal" title="从媒体库选取" :footer="null" :style="{ maxWidth: '700px' }">
      <!-- 搜索栏 -->
      <div class="media-search-bar">
        <Input
          v-model:value="mediaSearchQuery"
          placeholder="搜索文件..."
          allow-clear
        >
          <template #prefix><SearchOutlined /></template>
        </Input>
        <div class="media-actions">
          <span class="selected-count">已选择 {{ selectedMediaIds.length }} / {{ filteredMediaFiles.length }}</span>
          <Button size="small" @click="selectAllMedia">全选</Button>
          <Button size="small" @click="deselectAllMedia">取消全选</Button>
        </div>
      </div>

      <Spin :spinning="loadingMedia">
        <div v-if="filteredMediaFiles.length === 0 && !loadingMedia" class="media-empty">
          <Empty :description="mediaSearchQuery ? '没有匹配的文件' : '媒体库暂无文件'" />
        </div>
        <div v-else class="media-list">
          <div
            v-for="item in filteredMediaFiles"
            :key="item.id"
            :class="['media-item', { selected: selectedMediaIds.includes(item.id) }]"
            @click="toggleMediaSelection(item.id)"
          >
            <Checkbox :checked="selectedMediaIds.includes(item.id)" />
            <div class="media-info">
              <span class="media-name">{{ item.name }}</span>
              <span class="media-meta">
                <span class="media-type">{{ item.type }}</span>
                <span class="media-size">{{ formatSize(item.size) }}</span>
              </span>
            </div>
          </div>
        </div>
      </Spin>

      <div class="modal-footer">
        <Button @click="showMediaModal = false">取消</Button>
        <Button
          type="primary"
          :loading="importingMedia"
          :disabled="selectedMediaIds.length === 0"
          @click="importFromMedia"
        >
          导入选中文件 ({{ selectedMediaIds.length }})
        </Button>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.kb-detail-container { padding: 24px; max-width: 1200px; margin: 0 auto; }
.kb-detail-header { display: flex; align-items: center; gap: 16px; margin-bottom: 24px; }
.kb-detail-header h1 { flex: 1; margin: 0; font-size: 24px; font-weight: 600; color: var(--text-primary); }
.info-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 16px; }
.info-item { display: flex; flex-direction: column; gap: 4px; }
.info-item .label { font-size: 13px; color: var(--text-tertiary); }
.info-item .value { color: var(--text-primary); }
.kb-description { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--border-card); color: var(--text-secondary); line-height: 1.6; }
.modal-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }

.query-results { margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--border-card); }
.query-results h3 { margin: 0 0 12px; font-size: 16px; color: var(--text-primary); }
.query-result-item { padding: 12px; background: var(--bg-secondary, rgba(0,0,0,0.02)); border-radius: 8px; margin-bottom: 8px; }
.result-source { font-size: 12px; color: var(--text-tertiary); }
.result-header { margin-bottom: 8px; }
.result-content { margin: 0; font-size: 14px; line-height: 1.6; color: var(--text-primary); }

.media-search-bar { display: flex; gap: 12px; align-items: center; margin-bottom: 16px; }
.media-actions { display: flex; align-items: center; gap: 8px; white-space: nowrap; }
.selected-count { font-size: 13px; color: var(--text-secondary); }
.media-empty { padding: 40px 0; }
.media-list { border: 1px solid var(--border-card); border-radius: 8px; overflow: hidden; max-height: 400px; overflow-y: auto; -webkit-overflow-scrolling: touch; }
.media-item { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-card); cursor: pointer; transition: background 0.2s; }
.media-item:last-child { border-bottom: none; }
.media-item:hover { background: var(--bg-hover, rgba(0,0,0,0.03)); }
.media-item.selected { background: var(--primary-bg, #e6f7ff); }
.media-info { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; }
.media-name { font-weight: 500; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); }
.media-meta { display: flex; gap: 12px; font-size: 12px; color: var(--text-tertiary); }
.media-type { text-transform: uppercase; }

/* 移动端/平板：搜索栏堆叠、header 纵向、卡片头部换行、表格横向滚动 */
@media (max-width: 768px) {
  .kb-detail-container { padding: 16px 12px; }
  .kb-detail-header { flex-direction: column; align-items: flex-start; gap: 10px; }
  .kb-detail-header h1 { font-size: 20px; }
  .media-search-bar { flex-direction: column; align-items: stretch; gap: 8px; }
  .media-actions { justify-content: flex-end; }
  .info-grid { grid-template-columns: repeat(2, 1fr); }
  .media-item { padding: 10px 12px; }
  /* 卡片头部（标题 + 搜索/按钮）允许换行 */
  :deep(.ant-card-head) { flex-wrap: wrap; row-gap: 8px; }
  :deep(.ant-card-head-wrapper) { flex-wrap: wrap; row-gap: 8px; }
  :deep(.ant-card-extra) { margin-left: 0; width: 100%; }
  :deep(.ant-card-extra .ant-space) { flex-wrap: wrap; row-gap: 8px; }
  /* 文档表格窄屏横向滚动 */
  :deep(.ant-table-wrapper) { overflow-x: auto; -webkit-overflow-scrolling: touch; }
  :deep(.ant-table-wrapper .ant-table) { min-width: 640px; }
}

@media (prefers-reduced-motion: reduce) {
  .media-item { transition: none; }
}
</style>
