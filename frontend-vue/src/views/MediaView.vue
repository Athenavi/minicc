<script setup lang="ts">
import { h, ref, computed, onMounted } from 'vue'
import { NCard, NButton, NDataTable, NSpin, NEmpty, NIcon, NUpload, NModal, NSelect, NPagination, NInput, NDropdown, useMessage } from 'naive-ui'
import { ImageOutline, CloudUploadOutline, EyeOutline, BookOutline, SearchOutline } from '@vicons/ionicons5'
import { api } from '../api'
import { FileViewer } from '@file-viewer/vue3'
import allPreset from '@file-viewer/preset-all'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

interface MediaFile {
  id: string
  name: string
  type: string
  size: number
  file_url: string
  created_at: string
}

interface KnowledgeBase {
  id: string
  name: string
  type: string
  visibility: string
}

const message = useMessage()
const loading = ref(true)
const files = ref<MediaFile[]>([])
const showPreview = ref(false)
const previewFile = ref<MediaFile | null>(null)
const selectedRowKeys = ref<string[]>([])

// 分页
const page = ref(1)
const pageSize = ref(20)
const searchQuery = ref('')

// 知识库相关
const knowledgeBases = ref<KnowledgeBase[]>([])
const showKbModal = ref(false)
const selectedKbId = ref<string | null>(null)
const uploadingToKb = ref(false)

const previewUrl = computed(() => {
  if (!previewFile.value) return ''
  const url = previewFile.value.file_url
  if (!url) return ''
  if (url.startsWith('http://') || url.startsWith('https://')) return url
  return `${API_URL}${url.startsWith('/') ? '' : '/'}${url}`
})

const kbOptions = computed(() => {
  return knowledgeBases.value.map(kb => ({
    label: `${kb.name} (${kb.type.toUpperCase()})`,
    value: kb.id,
  }))
})

// 过滤后的文件列表
const filteredFiles = computed(() => {
  if (!searchQuery.value.trim()) return files.value
  const query = searchQuery.value.toLowerCase()
  return files.value.filter(f =>
    f.name.toLowerCase().includes(query) ||
    f.type.toLowerCase().includes(query)
  )
})

// 分页后的文件列表
const paginatedFiles = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredFiles.value.slice(start, start + pageSize.value)
})

const totalItems = computed(() => filteredFiles.value.length)

const columns = [
  { type: 'selection' as const },
  { title: '名称', key: 'name', ellipsis: { tooltip: true } },
  { title: '类型', key: 'type', width: 100 },
  {
    title: '大小', key: 'size', width: 100,
    render(row: MediaFile) {
      return formatSize(row.size)
    },
  },
  { title: '上传时间', key: 'created_at', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render(row: MediaFile) {
      const previewBtn = h('button', {
        style: 'color:#2080f0;cursor:pointer;border:none;background:none;padding:4px 8px;font-size:13px',
        onClick: () => {
          previewFile.value = row
          showPreview.value = true
        },
      }, '预览')
      const moreMenu = h(
        NDropdown,
        {
          options: [
            { label: '添加到知识库', key: 'kb' },
            { label: '删除', key: 'delete' },
          ],
          onSelect: (key: string) => {
            if (key === 'kb') openKbModal([row])
            if (key === 'delete') handleDelete(row.id)
          },
        },
        {
          default: () => h('button', {
            style: 'cursor:pointer;border:none;background:none;padding:4px 8px;font-size:16px;color:#666',
          }, '⋯'),
        }
      )
      return h('div', { style: 'display:flex;gap:4px;align-items:center' }, [previewBtn, moreMenu])
    },
  },
]

onMounted(async () => {
  await Promise.all([loadFiles(), loadKnowledgeBases()])
})

async function loadFiles() {
  try {
    loading.value = true
    const response = await api.get('/v1/media')
    files.value = response.data?.data || []
  } catch (error) {
    // 忽略错误
  } finally {
    loading.value = false
  }
}

async function loadKnowledgeBases() {
  try {
    const res = await api.get('/v1/kb')
    knowledgeBases.value = res.data?.data || []
  } catch (error) {
    console.error('加载知识库失败:', error)
  }
}

function handleSelectionChange(keys: string[]) {
  selectedRowKeys.value = keys
}

function handlePageChange(newPage: number) {
  page.value = newPage
}

function handlePageSizeChange(newSize: number) {
  pageSize.value = newSize
  page.value = 1
}

function openKbModal(fileList?: MediaFile[]) {
  if (fileList && fileList.length > 0) {
    selectedRowKeys.value = fileList.map(f => f.id)
  }
  if (selectedRowKeys.value.length === 0) {
    message.warning('请先选择文件')
    return
  }
  selectedKbId.value = null
  showKbModal.value = true
}

async function uploadToKnowledgeBase() {
  if (!selectedKbId.value) {
    message.warning('请选择目标知识库')
    return
  }
  if (selectedRowKeys.value.length === 0) {
    message.warning('请先选择文件')
    return
  }

  uploadingToKb.value = true
  let successCount = 0
  let failCount = 0

  for (const fileId of selectedRowKeys.value) {
    const file = files.value.find(f => f.id === fileId)
    if (!file) continue

    try {
      const response = await fetch(`${API_URL}${file.file_url}`)
      const blob = await response.blob()
      const formData = new FormData()
      formData.append('file', blob, file.name)

      await api.post(`/v1/kb/${selectedKbId.value}/documents`, formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      successCount++
    } catch (error) {
      failCount++
      console.error(`上传失败: ${file.name}`, error)
    }
  }

  uploadingToKb.value = false
  showKbModal.value = false
  selectedRowKeys.value = []

  if (successCount > 0) {
    message.success(`成功上传 ${successCount} 个文件到知识库`)
  }
  if (failCount > 0) {
    message.error(`${failCount} 个文件上传失败`)
  }
}

async function handleDelete(id: string) {
  try {
    await api.delete(`/v1/media/${id}`)
    message.success('已删除')
    await loadFiles()
  } catch (error: any) {
    message.error(error.message || '删除失败')
  }
}

function handleUpload({ file }: any) {
  const formData = new FormData()
  formData.append('file', file.file)
  api.post('/v1/media/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
    .then(() => {
      message.success('上传成功')
      loadFiles()
    })
    .catch((error) => {
      message.error(error.message || '上传失败')
    })
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
  <div class="media-container">
    <div class="media-header">
      <NIcon size="24" color="#2080f0">
        <ImageOutline />
      </NIcon>
      <h1>媒体库</h1>
    </div>

    <NCard>
      <template #header>
        <div class="card-header">
          <NIcon><CloudUploadOutline /></NIcon>
          <span>文件管理</span>
          <div class="header-actions">
            <NInput
              v-model:value="searchQuery"
              placeholder="搜索文件..."
              clearable
              style="width: 200px"
              size="small"
            >
              <template #prefix>
                <NIcon><SearchOutline /></NIcon>
              </template>
            </NInput>
            <NButton
              v-if="selectedRowKeys.length > 0"
              type="success"
              size="small"
              @click="openKbModal()"
            >
              <template #icon>
                <NIcon><BookOutline /></NIcon>
              </template>
              添加到知识库 ({{ selectedRowKeys.length }})
            </NButton>
            <NUpload
              :show-file-list="false"
              :custom-request="handleUpload"
            >
              <NButton type="primary" size="small">
                <template #icon>
                  <NIcon><CloudUploadOutline /></NIcon>
                </template>
                上传文件
              </NButton>
            </NUpload>
          </div>
        </div>
      </template>

      <NSpin :show="loading">
        <NEmpty v-if="filteredFiles.length === 0 && !loading" description="暂无文件" />
        <div v-else>
          <NDataTable
            :columns="columns"
            :data="paginatedFiles"
            :bordered="false"
            :single-line="false"
            :row-key="(row: MediaFile) => row.id"
            :checked-row-keys="selectedRowKeys"
            @update:checked-row-keys="handleSelectionChange"
          />
          <div class="pagination-wrapper">
            <NPagination
              v-model:page="page"
              v-model:page-size="pageSize"
              :item-count="totalItems"
              :page-sizes="[10, 20, 50, 100]"
              show-size-picker
              @update:page="handlePageChange"
              @update:page-size="handlePageSizeChange"
            />
          </div>
        </div>
      </NSpin>
    </NCard>

    <!-- 文件预览弹窗 -->
    <NModal
      v-model:show="showPreview"
      title="文件预览"
      preset="card"
      style="width: 90vw; max-width: 1200px"
    >
      <div class="preview-shell" v-if="previewFile">
        <file-viewer
          :url="previewUrl"
          :options="{
            preset: allPreset,
            rendererMode: 'replace',
            theme: 'light',
            toolbar: { position: 'auto' },
          }"
        />
      </div>
      <div v-else class="preview-empty">
        <p>暂无预览文件</p>
      </div>
    </NModal>

    <!-- 添加到知识库弹窗 -->
    <NModal
      v-model:show="showKbModal"
      title="添加到知识库"
      preset="card"
      style="max-width: 500px"
    >
      <div class="kb-modal-content">
        <p>已选择 <strong>{{ selectedRowKeys.length }}</strong> 个文件</p>
        <div class="kb-select">
          <label>选择目标知识库：</label>
          <NSelect
            v-model:value="selectedKbId"
            :options="kbOptions"
            placeholder="请选择知识库"
            filterable
          />
        </div>
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px;">
          <NButton @click="showKbModal = false">取消</NButton>
          <NButton
            type="primary"
            :loading="uploadingToKb"
            :disabled="!selectedKbId"
            @click="uploadToKnowledgeBase"
          >
            开始上传
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.media-container {
  padding: 24px;
}

.media-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.media-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.header-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
  align-items: center;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #f0f0f0;
}

/* 预览弹窗 */
.preview-shell {
  height: 75vh;
  min-height: 400px;
  border-radius: 4px;
  overflow: hidden;
}

.preview-empty {
  height: 300px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #999;
}

.kb-modal-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.kb-select label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
}
</style>
