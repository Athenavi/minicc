<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { message } from 'ant-design-vue'
import type { TableColumnsType } from 'ant-design-vue'
import { queryAuditLogs } from '../../api/audit'
import type { AuditLog } from '../../api/audit'

const loading = ref(false)
const logs = ref<AuditLog[]>([])
const total = ref(0)
const detailVisible = ref(false)
const currentLog = ref<AuditLog | null>(null)

// 过滤条件（时间默认最近 7 天，对齐后端强制范围）
const filters = reactive({
  user_id: '',
  action: '',
  resource_type: '',
  dateRange: [] as [string, string] | undefined,
})

const pagination = reactive({
  page: 1,
  page_size: 50,
})

function defaultRange(): [string, string] {
  const to = new Date()
  const from = new Date(to.getTime() - 6 * 24 * 60 * 60 * 1000)
  return [fmt(from), fmt(to)]
}

function fmt(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

async function fetchLogs() {
  loading.value = true
  try {
    const range = filters.dateRange?.length === 2 ? filters.dateRange : defaultRange()
    const res = await queryAuditLogs({
      user_id: filters.user_id || undefined,
      action: filters.action || undefined,
      resource_type: filters.resource_type || undefined,
      from: range[0],
      to: range[1],
      page: pagination.page,
      page_size: pagination.page_size,
    })
    logs.value = res.data
    total.value = res.total
  } catch (e: any) {
    message.error(e?.response?.data?.error || '查询失败')
  } finally {
    loading.value = false
  }
}

function onSearch() {
  pagination.page = 1
  fetchLogs()
}

function onReset() {
  filters.user_id = ''
  filters.action = ''
  filters.resource_type = ''
  filters.dateRange = defaultRange()
  pagination.page = 1
  fetchLogs()
}

function onPageChange(page: number, pageSize: number) {
  pagination.page = page
  pagination.page_size = pageSize
  fetchLogs()
}

function showDetail(log: AuditLog) {
  currentLog.value = log
  detailVisible.value = true
}

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { hour12: false })
}

function formatDetails(d: unknown): string {
  if (d === null || d === undefined) return ''
  if (typeof d === 'string') return d
  try { return JSON.stringify(d, null, 2) } catch { return String(d) }
}

const columns: TableColumnsType = [
  { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 180, customRender: ({ text }) => formatTime(text) },
  { title: '用户', dataIndex: 'user_id', key: 'user_id', width: 140, ellipsis: true },
  { title: '动作', dataIndex: 'action', key: 'action', width: 140 },
  { title: '资源类型', dataIndex: 'resource_type', key: 'resource_type', width: 140 },
  { title: '资源 ID', dataIndex: 'resource_id', key: 'resource_id', width: 180, ellipsis: true },
  { title: 'IP', dataIndex: 'ip_address', key: 'ip_address', width: 140 },
  { title: '操作', key: 'action_btn', width: 80, fixed: 'right' },
]

onMounted(() => {
  filters.dateRange = defaultRange()
  fetchLogs()
})
</script>

<template>
  <div class="audit-view">
    <div class="audit-header">
      <h2 class="audit-title">操作审计</h2>
      <p class="audit-desc">查询范围限制为 7 天内，确保命中索引性能。</p>
    </div>

    <div class="audit-filters">
      <a-range-picker
        v-model:value="filters.dateRange"
        :format="'YYYY-MM-DD'"
        style="width: 260px"
      />
      <a-input
        v-model:value="filters.user_id"
        placeholder="用户 ID"
        allow-clear
        style="width: 180px"
        @press-enter="onSearch"
      />
      <a-input
        v-model:value="filters.action"
        placeholder="动作（如 POST /v1/ent/privacy）"
        allow-clear
        style="width: 280px"
        @press-enter="onSearch"
      />
      <a-input
        v-model:value="filters.resource_type"
        placeholder="资源类型"
        allow-clear
        style="width: 160px"
        @press-enter="onSearch"
      />
      <a-button type="primary" @click="onSearch">查询</a-button>
      <a-button @click="onReset">重置</a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="logs"
      :loading="loading"
      :row-key="(r: AuditLog) => r.id"
      :pagination="{
        current: pagination.page,
        pageSize: pagination.page_size,
        total,
        showSizeChanger: true,
        pageSizeOptions: ['20', '50', '100'],
        showTotal: (t: number) => `共 ${t} 条`,
      }"
      :scroll="{ x: 1100 }"
      size="small"
      @change="(p: any) => onPageChange(p.current, p.pageSize)"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'action_btn'">
          <a-button type="link" size="small" @click="showDetail(record)">详情</a-button>
        </template>
      </template>
    </a-table>

    <a-drawer
      v-model:open="detailVisible"
      title="审计日志详情"
      width="560"
      placement="right"
    >
      <template v-if="currentLog">
        <a-descriptions :column="1" size="small" bordered>
          <a-descriptions-item label="ID">{{ currentLog.id }}</a-descriptions-item>
          <a-descriptions-item label="时间">{{ formatTime(currentLog.created_at) }}</a-descriptions-item>
          <a-descriptions-item label="租户">{{ currentLog.tenant_id }}</a-descriptions-item>
          <a-descriptions-item label="用户">{{ currentLog.user_id || '-' }}</a-descriptions-item>
          <a-descriptions-item label="动作">{{ currentLog.action }}</a-descriptions-item>
          <a-descriptions-item label="资源类型">{{ currentLog.resource_type }}</a-descriptions-item>
          <a-descriptions-item label="资源 ID">{{ currentLog.resource_id || '-' }}</a-descriptions-item>
          <a-descriptions-item label="IP">{{ currentLog.ip_address || '-' }}</a-descriptions-item>
        </a-descriptions>
        <div class="audit-detail-block">
          <div class="audit-detail-label">详情（details）</div>
          <pre class="audit-detail-json">{{ formatDetails(currentLog.details) || '-' }}</pre>
        </div>
      </template>
    </a-drawer>
  </div>
</template>

<style scoped>
.audit-view {
  padding: 16px 24px;
}
.audit-header {
  margin-bottom: 16px;
}
.audit-title {
  margin: 0;
  font-size: 20px;
}
.audit-desc {
  margin: 4px 0 0;
  color: rgba(0, 0, 0, 0.45);
  font-size: 13px;
}
.audit-filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.audit-detail-block {
  margin-top: 16px;
}
.audit-detail-label {
  margin-bottom: 8px;
  font-weight: 500;
}
.audit-detail-json {
  margin: 0;
  padding: 12px;
  background: rgba(0, 0, 0, 0.04);
  border-radius: 4px;
  font-size: 12px;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
