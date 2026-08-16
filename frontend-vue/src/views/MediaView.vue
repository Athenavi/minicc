<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import {
  Button, Input, Select, Segmented, Pagination, Empty, Spin, Table, message,
  Drawer, Modal, Popconfirm, Upload, Checkbox,
} from 'ant-design-vue'
import {
  SearchOutlined, CloudUploadOutlined, FolderOutlined, FileOutlined,
} from '@ant-design/icons-vue'
import { api } from '../api'
import { FileViewer } from '@file-viewer/vue3'
import allPreset from '@file-viewer/preset-all'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

interface MediaItem {
  id: string
  type: string
  name: string
  size: number
  file_url: string
  mime_type?: string
  created_at: string
}

const loading = ref(true)
const items = ref<MediaItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const searchQuery = ref('')
const typeFilter = ref('')
const viewMode = ref<'grid' | 'list'>(localStorage.getItem('media-view') === 'list' ? 'list' : 'grid')
const breadcrumbs = ref<{ id: string; name: string }[]>([{ id: '', name: '全部文件' }])
const selectedIds = ref<Set<string>>(new Set())

// 详情侧栏 / 上传（T9/T10 填充 UI，此处声明状态）
const detailItem = ref<MediaItem | null>(null)
const showUpload = ref(false)

const currentParentId = computed(() => breadcrumbs.value[breadcrumbs.value.length - 1].id)

const typeOptions = [
  { label: '全部类型', value: '' },
  { label: '图片', value: 'image' },
  { label: '文档', value: 'document' },
  { label: '视频', value: 'video' },
  { label: '音频', value: 'audio' },
  { label: '文件', value: 'file' },
  { label: '文本', value: 'text' },
  { label: '代码', value: 'code' },
]

const listColumns = [
  { title: '名称', dataIndex: 'name', ellipsis: true },
  { title: '类型', dataIndex: 'type', width: 100 },
  { title: '大小', dataIndex: 'size', width: 110, customRender: ({ text }: { text: number }) => formatSize(text) },
  { title: '上传时间', dataIndex: 'created_at', width: 180 },
]

function formatSize(bytes: number): string {
  if (!bytes) return ''
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function isFolder(item: MediaItem) { return item.type === 'folder' }
function isImage(item: MediaItem) { return (item.mime_type || '').startsWith('image/') || item.type === 'image' }

function absUrl(url: string): string {
  if (!url) return ''
  if (url.startsWith('http://') || url.startsWith('https://')) return url
  return `${API_URL}${url.startsWith('/') ? '' : '/'}${url}`
}

async function fetchItems() {
  loading.value = true
  try {
    const params: Record<string, string | number> = {
      page: page.value,
      page_size: pageSize.value,
      parent_id: currentParentId.value,
    }
    if (searchQuery.value.trim()) params.search = searchQuery.value.trim()
    if (typeFilter.value) params.type = typeFilter.value
    const res = await api.get('/v1/media', { params })
    const data = res.data?.data || { items: [], total: 0 }
    items.value = data.items || []
    total.value = data.total || 0
    selectedIds.value = new Set()
  } catch {
    items.value = []
    total.value = 0
    message.error('加载媒体库失败')
  } finally {
    loading.value = false
  }
}

function enterFolder(id: string, name: string) {
  breadcrumbs.value.push({ id, name })
  page.value = 1
  fetchItems()
}

function goBreadcrumb(idx: number) {
  breadcrumbs.value = breadcrumbs.value.slice(0, idx + 1)
  page.value = 1
  fetchItems()
}

function toggleView(mode: 'grid' | 'list') {
  viewMode.value = mode
  localStorage.setItem('media-view', mode)
}

function onCardClick(item: MediaItem) {
  if (isFolder(item)) {
    enterFolder(item.id, item.name)
  } else {
    detailItem.value = item
  }
}

// ── 详情侧栏操作 ──
const showPreview = ref(false)
const previewItem = ref<MediaItem | null>(null)
const showRename = ref(false)
const renameName = ref('')
const showMove = ref(false)
const moveParentId = ref('')
const folderTree = ref<MediaItem[]>([])
const showShare = ref(false)
const shareUrl = ref('')
const shareExpires = ref('')
const shareLoading = ref(false)

function openPreview(item: MediaItem) { previewItem.value = item; showPreview.value = true }

function downloadItem(item: MediaItem) { window.open(absUrl(item.file_url), '_blank') }

async function copyUrl(item: MediaItem) {
  try {
    await navigator.clipboard.writeText(absUrl(item.file_url))
    message.success('URL 已复制')
  } catch {
    message.error('复制失败')
  }
}

async function shareItem() {
  if (!detailItem.value) return
  shareLoading.value = true
  try {
    const res = await api.post(`/v1/media/${detailItem.value.id}/share`, { expires_in_seconds: 900 })
    const data = res.data?.data || {}
    shareUrl.value = data.url || ''
    shareExpires.value = data.expires_at || ''
    showShare.value = true
    if (!shareUrl.value) message.error('当前存储后端不支持分享')
  } catch (e: any) {
    message.error(e.response?.data?.error || '生成分享链接失败')
  } finally {
    shareLoading.value = false
  }
}

function openRename() {
  if (!detailItem.value) return
  renameName.value = detailItem.value.name
  showRename.value = true
}

async function submitRename() {
  if (!detailItem.value || !renameName.value.trim()) return
  try {
    await api.put(`/v1/media/${detailItem.value.id}`, { name: renameName.value.trim() })
    message.success('已重命名')
    showRename.value = false
    detailItem.value = null
    fetchItems()
  } catch (e: any) {
    message.error(e.response?.data?.error || '重命名失败')
  }
}

async function openMove() {
  moveParentId.value = currentParentId.value
  try {
    const res = await api.get('/v1/media', { params: { page: 1, page_size: 200 } })
    folderTree.value = (res.data?.data?.items || []).filter((i: MediaItem) => i.type === 'folder')
  } catch {
    folderTree.value = []
  }
  showMove.value = true
}

async function submitMove() {
  if (!detailItem.value) return
  try {
    await api.put(`/v1/media/${detailItem.value.id}`, { parent_id: moveParentId.value })
    message.success('已移动')
    showMove.value = false
    detailItem.value = null
    fetchItems()
  } catch (e: any) {
    message.error(e.response?.data?.error || '移动失败')
  }
}

async function deleteItem(id: string) {
  try {
    await api.delete(`/v1/media/${id}`)
    message.success('已删除')
    detailItem.value = null
    fetchItems()
  } catch (e: any) {
    message.error(e.response?.data?.error || '删除失败')
  }
}

async function copyShareUrl() {
  try {
    await navigator.clipboard.writeText(shareUrl.value)
    message.success('分享链接已复制')
  } catch {
    message.error('复制失败')
  }
}

// ── 批量上传（拖拽 + 多文件） ──
const uploadFileList = ref<any[]>([])

function handleUploadRequest(options: any) {
  const { file, onProgress, onSuccess, onError } = options
  const formData = new FormData()
  formData.append('file', file)
  formData.append('parent_id', currentParentId.value)
  api.post('/v1/media/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    onUploadProgress: (e: any) => {
      if (onProgress && e.total) onProgress({ percent: Math.round((e.loaded / e.total) * 100) })
    },
  })
    .then(() => { onSuccess?.(null); message.success(`上传成功: ${file.name}`) })
    .catch((err: any) => { onError?.(err); message.error(`上传失败: ${file.name}`) })
    .finally(() => { fetchItems() })
}

// ── 批量选择 ──
function toggleSelect(item: MediaItem, e: Event) {
  e.stopPropagation()
  const next = new Set(selectedIds.value)
  if (next.has(item.id)) next.delete(item.id)
  else next.add(item.id)
  selectedIds.value = next
}

// ── 批量删除 ──
const batchDeleting = ref(false)

async function batchDelete() {
  const ids = Array.from(selectedIds.value)
  if (ids.length === 0) return
  batchDeleting.value = true
  try {
    const res = await api.post('/v1/media/batch-delete', { ids })
    message.success(`已删除 ${res.data?.data?.deleted || ids.length} 项`)
    fetchItems()
  } catch (e: any) {
    message.error(e.response?.data?.error || '批量删除失败')
  } finally {
    batchDeleting.value = false
  }
}

// ── 添加到知识库（保留既有流程） ──
const showKbModal = ref(false)
const selectedKbId = ref<string | undefined>(undefined)
const knowledgeBases = ref<{ id: string; name: string; type: string }[]>([])
const uploadingToKb = ref(false)
const kbOptions = computed(() => knowledgeBases.value.map(kb => ({ label: `${kb.name} (${kb.type.toUpperCase()})`, value: kb.id })))

async function openKbModal() {
  if (selectedIds.value.size === 0) { message.warning('请先选择文件'); return }
  try {
    const res = await api.get('/v1/kb')
    knowledgeBases.value = res.data?.data?.knowledge_bases || []
  } catch {
    knowledgeBases.value = []
  }
  selectedKbId.value = undefined
  showKbModal.value = true
}

async function uploadToKnowledgeBase() {
  if (!selectedKbId.value || selectedIds.value.size === 0) return
  uploadingToKb.value = true
  let ok = 0, fail = 0
  const files = items.value.filter(i => selectedIds.value.has(i.id) && !isFolder(i))
  for (const file of files) {
    try {
      const resp = await fetch(absUrl(file.file_url))
      const blob = await resp.blob()
      const formData = new FormData()
      formData.append('file', blob, file.name)
      await api.post(`/v1/kb/${selectedKbId.value}/documents`, formData, { headers: { 'Content-Type': 'multipart/form-data' } })
      ok++
    } catch {
      fail++
    }
  }
  uploadingToKb.value = false
  showKbModal.value = false
  if (ok > 0) message.success(`成功上传 ${ok} 个文件到知识库`)
  if (fail > 0) message.error(`${fail} 个文件上传失败`)
}

watch([searchQuery, typeFilter, page, pageSize], () => { fetchItems() })

onMounted(fetchItems)
</script>

<template>
  <div class="media-page">
    <div class="media-toolbar">
      <div class="breadcrumb">
        <span
          v-for="(cr, i) in breadcrumbs"
          :key="cr.id || 'root'"
          class="crumb"
          :class="{ active: i === breadcrumbs.length - 1 }"
          @click="goBreadcrumb(i)"
        >{{ cr.name }}<span v-if="i < breadcrumbs.length - 1" class="crumb-sep">/</span></span>
      </div>
      <div class="toolbar-actions">
        <Input v-model:value="searchQuery" placeholder="搜索文件" allow-clear style="width: 180px" size="small">
          <template #prefix><SearchOutlined /></template>
        </Input>
        <Select v-model:value="typeFilter" :options="typeOptions" size="small" style="width: 110px" />
        <Segmented
          :value="viewMode"
          :options="[{ label: '网格', value: 'grid' }, { label: '列表', value: 'list' }]"
          size="small"
          @change="toggleView(($event as any) as 'grid' | 'list')"
        />
        <Button type="primary" size="small" @click="showUpload = true">
          <template #icon><CloudUploadOutlined /></template>上传
        </Button>
      </div>
    </div>

    <div v-if="selectedIds.size > 0" class="batch-bar">
      <span class="batch-count">已选择 {{ selectedIds.size }} 项</span>
      <Button size="small" danger :loading="batchDeleting" @click="batchDelete">批量删除</Button>
      <Button size="small" @click="openKbModal">添加到知识库</Button>
      <Button size="small" type="text" @click="selectedIds = new Set()">取消选择</Button>
    </div>

    <Spin :spinning="loading">
      <Empty v-if="!loading && items.length === 0" description="暂无文件，拖拽或点击上传" />
      <div v-else-if="viewMode === 'grid'" class="file-grid">
        <div
          v-for="item in items"
          :key="item.id"
          class="file-card"
          :class="{ selected: selectedIds.has(item.id) }"
          @click="onCardClick(item)"
        >
          <Checkbox
            class="card-check"
            :checked="selectedIds.has(item.id)"
            @click="toggleSelect(item, $event)"
          />
          <div class="card-thumb">
            <img
              v-if="isImage(item) && !isFolder(item)"
              :src="absUrl(item.file_url)"
              loading="lazy"
              @error="(e: any) => (e.target.style.display = 'none')"
            />
            <FolderOutlined v-else-if="isFolder(item)" class="thumb-icon folder" />
            <FileOutlined v-else class="thumb-icon" />
          </div>
          <div class="card-name" :title="item.name">{{ item.name }}</div>
          <div v-if="!isFolder(item)" class="card-meta">{{ formatSize(item.size) }}</div>
        </div>
      </div>
      <Table
        v-else
        :columns="listColumns"
        :data-source="items"
        :row-key="'id'"
        :pagination="false"
        size="small"
        :row-class-name="(record: MediaItem) => (selectedIds.has(record.id) ? 'row-selected' : '')"
        @row-click="(record: MediaItem) => onCardClick(record)"
      />
    </Spin>

    <div v-if="total > pageSize" class="pagination-bar">
      <Pagination
        v-model:current="page"
        v-model:pageSize="pageSize"
        :total="total"
        :pageSizeOptions="['20', '50', '100', '200']"
        show-size-changer
      />
    </div>

    <!-- 详情侧栏 -->
    <Drawer
      :open="!!detailItem"
      :width="360"
      :title="detailItem?.name || '文件详情'"
      @close="detailItem = null"
    >
      <template v-if="detailItem">
        <div class="detail-thumb">
          <img
            v-if="isImage(detailItem) && !isFolder(detailItem)"
            :src="absUrl(detailItem.file_url)"
          />
          <FolderOutlined v-else-if="isFolder(detailItem)" class="thumb-icon folder" />
          <FileOutlined v-else class="thumb-icon" />
        </div>
        <div class="detail-info">
          <div class="detail-row"><span class="label">类型</span><span>{{ detailItem.type }}</span></div>
          <div class="detail-row"><span class="label">大小</span><span>{{ formatSize(detailItem.size) }}</span></div>
          <div class="detail-row"><span class="label">MIME</span><span>{{ detailItem.mime_type || '—' }}</span></div>
          <div class="detail-row"><span class="label">上传时间</span><span>{{ detailItem.created_at }}</span></div>
          <div class="detail-row"><span class="label">URL</span><span class="url-text">{{ absUrl(detailItem.file_url) }}</span></div>
        </div>
        <div class="detail-actions">
          <Button v-if="!isFolder(detailItem)" block @click="openPreview(detailItem)">预览</Button>
          <Button v-if="!isFolder(detailItem)" block @click="downloadItem(detailItem)">下载</Button>
          <Button v-if="!isFolder(detailItem)" block @click="copyUrl(detailItem)">复制 URL</Button>
          <Button v-if="!isFolder(detailItem)" block :loading="shareLoading" @click="shareItem">生成分享链接</Button>
          <Button block @click="openRename">重命名</Button>
          <Button block @click="openMove">移动到</Button>
          <Popconfirm title="确认删除？文件夹将连同子项一并删除" @confirm="detailItem && deleteItem(detailItem.id)">
            <Button block danger>删除</Button>
          </Popconfirm>
        </div>
      </template>
    </Drawer>

    <!-- 重命名 -->
    <Modal v-model:open="showRename" title="重命名" :width="360" @ok="submitRename">
      <Input v-model:value="renameName" @press-enter="submitRename" />
    </Modal>

    <!-- 移动 -->
    <Modal v-model:open="showMove" title="移动到" :width="400" @ok="submitMove">
      <Select v-model:value="moveParentId" style="width: 100%">
        <Select.Option value="">根目录</Select.Option>
        <Select.Option v-for="f in folderTree" :key="f.id" :value="f.id">{{ f.name }}</Select.Option>
      </Select>
    </Modal>

    <!-- 分享 -->
    <Modal v-model:open="showShare" title="分享链接" :width="480" :footer="null">
      <div class="share-row">
        <Input :value="shareUrl" read-only />
        <Button @click="copyShareUrl">复制</Button>
      </div>
      <div v-if="shareExpires" class="share-expires">有效期至 {{ shareExpires }}</div>
    </Modal>

    <!-- 预览 -->
    <Modal
      v-model:open="showPreview"
      :title="previewItem?.name"
      :width="'90vw'"
      :style="{ maxWidth: '1200px' }"
      :footer="null"
      @cancel="showPreview = false"
    >
      <div class="preview-shell" v-if="previewItem">
        <file-viewer
          :url="absUrl(previewItem.file_url)"
          :options="{ preset: allPreset, rendererMode: 'replace', theme: 'light', toolbar: { position: 'auto' } }"
        />
      </div>
    </Modal>
    <!-- 上传（拖拽 + 多文件 + 进度） -->
    <Modal v-model:open="showUpload" title="上传文件" :width="520" :footer="null" destroy-on-close>
      <Upload
        :file-list="uploadFileList"
        :before-upload="() => false"
        :multiple="true"
        :custom-request="handleUploadRequest"
        drag
        style="width: 100%"
      >
        <p class="ant-upload-drag-icon"><CloudUploadOutlined /></p>
        <p class="ant-upload-text">拖拽文件到此处，或点击选择</p>
        <p class="ant-upload-hint">支持多文件上传，单文件不超过 50MB</p>
      </Upload>
    </Modal>

    <!-- 添加到知识库 -->
    <Modal v-model:open="showKbModal" title="添加到知识库" :width="420" :footer="null">
      <p>已选择 <strong>{{ selectedIds.size }}</strong> 个文件</p>
      <Select
        v-model:value="selectedKbId"
        :options="kbOptions"
        placeholder="请选择知识库"
        show-search
        style="width: 100%; margin-top: 12px"
      />
      <div class="modal-footer">
        <Button @click="showKbModal = false">取消</Button>
        <Button type="primary" :loading="uploadingToKb" :disabled="!selectedKbId" @click="uploadToKnowledgeBase">
          开始上传
        </Button>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.media-page { padding: 16px 24px; display: flex; flex-direction: column; gap: 12px; height: 100%; overflow: hidden; }
.media-toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.breadcrumb { display: flex; align-items: center; gap: 4px; min-width: 0; }
.crumb { font-size: 14px; color: var(--text-secondary); cursor: pointer; white-space: nowrap; }
.crumb:hover { color: var(--primary); }
.crumb.active { color: var(--text-primary); font-weight: 600; }
.crumb-sep { margin-left: 4px; color: var(--text-muted); }
.toolbar-actions { margin-left: auto; display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.file-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; overflow-y: auto; padding-bottom: 24px; }
.file-card { border: 1px solid var(--border-card); border-radius: var(--radius-lg); background: var(--bg-card); padding: 10px; cursor: pointer; transition: all 0.15s; display: flex; flex-direction: column; gap: 6px; }
.file-card:hover { border-color: var(--primary); box-shadow: var(--shadow-md); }
.file-card.selected { border-color: var(--primary); background: var(--primary-bg); }
.card-thumb { height: 84px; display: flex; align-items: center; justify-content: center; border-radius: var(--radius-md); background: var(--bg-secondary); overflow: hidden; }
.card-thumb img { max-width: 100%; max-height: 100%; object-fit: cover; }
.thumb-icon { font-size: 32px; color: var(--text-tertiary); }
.thumb-icon.folder { color: var(--primary); }
.card-name { font-size: 13px; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-meta { font-size: 11px; color: var(--text-muted); }
.pagination-bar { display: flex; justify-content: flex-end; padding-top: 8px; border-top: 1px solid var(--border); }
:deep(.row-selected) { background: var(--primary-bg) !important; }
.batch-bar { position: sticky; top: 0; z-index: 10; display: flex; align-items: center; gap: 12px; padding: 8px 12px; background: var(--bg-card); border: 1px solid var(--primary); border-radius: var(--radius-md); box-shadow: var(--shadow-md); }
.batch-count { font-size: 13px; color: var(--text-primary); }
.card-check { position: absolute; top: 6px; left: 6px; }
.file-card { position: relative; }
.detail-thumb { height: 160px; display: flex; align-items: center; justify-content: center; background: var(--bg-secondary); border-radius: var(--radius-lg); margin-bottom: 16px; overflow: hidden; }
.detail-thumb img { max-width: 100%; max-height: 100%; object-fit: contain; }
.detail-info { display: flex; flex-direction: column; gap: 8px; margin-bottom: 16px; }
.detail-row { display: flex; justify-content: space-between; gap: 12px; font-size: 13px; }
.detail-row .label { color: var(--text-muted); flex-shrink: 0; }
.url-text { word-break: break-all; color: var(--text-secondary); }
.detail-actions { display: flex; flex-direction: column; gap: 8px; }
.share-row { display: flex; gap: 8px; }
.share-expires { margin-top: 8px; font-size: 12px; color: var(--text-muted); }
.preview-shell { height: 75vh; min-height: 400px; border-radius: 4px; overflow: hidden; }
@media (max-width: 768px) {
  .media-page { padding: 12px; }
  .file-grid { grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 8px; }
  :deep(.ant-drawer-content-wrapper) { width: 100vw !important; }
  .toolbar-actions { margin-left: 0; width: 100%; }
  .batch-bar { position: fixed; left: 12px; right: 12px; bottom: 12px; top: auto; }
}
.modal-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
</style>
