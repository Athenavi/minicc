<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  Card, Table, Button, Space, Tag, Input, Modal, Form, Alert,
  Descriptions, Popconfirm, Spin, message,
} from 'ant-design-vue'
import {
  PlusOutlined, EditOutlined, DeleteOutlined, CheckCircleOutlined,
  SyncOutlined, ReloadOutlined,
} from '@ant-design/icons-vue'
import { api } from '../../api'
import { apiErrorMessage } from '../../composables/useCrudResource'
import EmptyState from '../../components/common/EmptyState.vue'

// ── 域名列表 ──
// GET /v1/admin/domains → { domains: [{ id, domain, ssl_status, verified, created_at }] }
const domains = ref<any[]>([])
const loading = ref(false)

async function loadDomains() {
  loading.value = true
  try {
    const resp = await api.get('/v1/admin/domains')
    domains.value = resp.data?.data?.domains || []
  } catch (e: any) {
    message.error(apiErrorMessage(e, '加载域名列表失败'))
  } finally {
    loading.value = false
  }
}

// ── 新建 / 编辑 ──
// POST /v1/admin/domains { domain }; PUT /v1/admin/domains/{id} { domain }
const modalVisible = ref(false)
const submitting = ref(false)
const editingId = ref<string | null>(null)
const form = ref({ domain: '' })

function openCreate() {
  editingId.value = null
  form.value = { domain: '' }
  modalVisible.value = true
}

function openEdit(record: any) {
  editingId.value = record.id
  form.value = { domain: record.domain || '' }
  modalVisible.value = true
}

async function submitForm() {
  if (!form.value.domain.trim()) {
    message.warning('请输入域名')
    return
  }
  submitting.value = true
  try {
    if (editingId.value) {
      await api.put(`/v1/admin/domains/${editingId.value}`, { domain: form.value.domain.trim() })
      message.success('域名已更新')
    } else {
      await api.post('/v1/admin/domains', { domain: form.value.domain.trim() })
      message.success('域名已添加')
    }
    modalVisible.value = false
    await loadDomains()
  } catch (e: any) {
    message.error(apiErrorMessage(e, editingId.value ? '更新域名失败' : '添加域名失败'))
  } finally {
    submitting.value = false
  }
}

// ── 验证 ──
// POST /v1/admin/domains/{id}/verify → { verified, addresses?, reason? }
const verifyVisible = ref(false)
const verifyResult = ref<any>(null)
const verifyingId = ref<string | null>(null)

async function verifyDomain(record: any) {
  verifyingId.value = record.id
  try {
    const resp = await api.post(`/v1/admin/domains/${record.id}/verify`)
    verifyResult.value = resp.data?.data || {}
    verifyVisible.value = true
    await loadDomains()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '域名验证失败'))
  } finally {
    verifyingId.value = null
  }
}

// ── SSL 续期 ──
// POST /v1/admin/domains/{id}/renew-ssl → { ssl_status, note? }
const renewingId = ref<string | null>(null)

async function renewSSL(record: any) {
  renewingId.value = record.id
  try {
    const resp = await api.post(`/v1/admin/domains/${record.id}/renew-ssl`)
    const d = resp.data?.data || {}
    message.success(d.note ? `SSL 续期完成（${d.ssl_status || 'ok'}）：${d.note}` : `SSL 续期完成（${d.ssl_status || 'ok'}）`)
    await loadDomains()
  } catch (e: any) {
    message.error(apiErrorMessage(e, 'SSL 续期失败'))
  } finally {
    renewingId.value = null
  }
}

// ── 删除 ──
// DELETE /v1/admin/domains/{id}
async function removeDomain(record: any) {
  try {
    await api.delete(`/v1/admin/domains/${record.id}`)
    message.success('域名已删除')
    await loadDomains()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '删除域名失败'))
  }
}

// ── 表格列 ──
const columns = [
  { title: '域名', dataIndex: 'domain', key: 'domain', ellipsis: true },
  { title: '验证状态', key: 'verified', width: 110 },
  { title: 'SSL 状态', key: 'ssl_status', width: 120 },
  { title: '创建时间', key: 'created_at', width: 180 },
  { title: '操作', key: 'actions', width: 280, fixed: 'right' as const },
]

// ── 工具函数 ──
function sslStatusText(status: string): string {
  switch (status) {
    case 'active': return '有效'
    case 'pending': return '签发中'
    case 'expired': return '已过期'
    case 'failed': return '失败'
    default: return status || '-'
  }
}

function sslStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'pending': return 'orange'
    case 'expired': return 'red'
    case 'failed': return 'red'
    default: return 'default'
  }
}

function formatDate(d: any): string {
  return d ? new Date(d).toLocaleString('zh-CN') : '-'
}

function formatAddresses(a: any): string {
  if (a == null) return '-'
  if (Array.isArray(a)) return a.join('；')
  if (typeof a === 'object') return Object.entries(a).map(([k, v]) => `${k}: ${v}`).join('；')
  return String(a)
}

onMounted(loadDomains)
</script>

<template>
  <div class="domain-management">
    <div class="page-header">
      <h1>🌐 域名管理</h1>
      <Space>
        <Button @click="loadDomains">
          <template #icon><ReloadOutlined /></template>
          刷新
        </Button>
        <Button type="primary" @click="openCreate">
          <template #icon><PlusOutlined /></template>
          添加域名
        </Button>
      </Space>
    </div>

    <Alert
      message="域名接入说明"
      description="添加域名后，先点击「验证」获取需要配置的解析地址；在 DNS 服务商处完成解析后再次点击「验证」即可确认接入。证书到期前可点击「续期」刷新 SSL 证书。"
      type="info"
      show-icon
      style="margin-bottom: 16px"
    />

    <Spin :spinning="loading">
      <Card>
        <Table
          :columns="columns"
          :data-source="domains"
          row-key="id"
          :pagination="{ pageSize: 20, showSizeChanger: true }"
          :scroll="{ x: 900 }"
        >
          <template #emptyText>
            <EmptyState description="暂无域名" hint="点击右上角「添加域名」接入第一个域名" />
          </template>

          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'verified'">
              <Tag :color="record.verified ? 'green' : 'orange'">
                {{ record.verified ? '已验证' : '未验证' }}
              </Tag>
            </template>

            <template v-else-if="column.key === 'ssl_status'">
              <Tag :color="sslStatusColor(record.ssl_status)">{{ sslStatusText(record.ssl_status) }}</Tag>
            </template>

            <template v-else-if="column.key === 'created_at'">
              {{ formatDate(record.created_at) }}
            </template>

            <template v-else-if="column.key === 'actions'">
              <Space :size="4" wrap>
                <Button size="small" :loading="verifyingId === record.id" @click="verifyDomain(record)">
                  <template #icon><CheckCircleOutlined /></template>
                  验证
                </Button>
                <Button size="small" :loading="renewingId === record.id" @click="renewSSL(record)">
                  <template #icon><SyncOutlined /></template>
                  续期 SSL
                </Button>
                <Button size="small" @click="openEdit(record)">
                  <template #icon><EditOutlined /></template>
                  编辑
                </Button>
                <Popconfirm
                  title="确定删除该域名吗？删除后其 SSL 证书与接入配置将一并移除。"
                  ok-text="删除"
                  cancel-text="取消"
                  @confirm="removeDomain(record)"
                >
                  <Button size="small" danger>
                    <template #icon><DeleteOutlined /></template>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            </template>
          </template>
        </Table>
      </Card>
    </Spin>

    <!-- 验证结果 -->
    <Modal
      v-model:open="verifyVisible"
      title="域名验证结果"
      :footer="null"
      :width="560"
      @cancel="verifyResult = null"
    >
      <template v-if="verifyResult">
        <Alert
          :type="verifyResult.verified ? 'success' : 'error'"
          :message="verifyResult.verified ? '验证成功，域名已接入' : '验证失败'"
          show-icon
          style="margin-bottom: 16px"
        />
        <Descriptions v-if="verifyResult.verified" :column="1" bordered size="small">
          <Descriptions.Item label="解析地址">
            {{ formatAddresses(verifyResult.addresses) }}
          </Descriptions.Item>
        </Descriptions>
        <Alert
          v-else-if="verifyResult.reason"
          type="warning"
          :message="String(verifyResult.reason)"
          show-icon
        />
      </template>
    </Modal>

    <!-- 新建 / 编辑域名 -->
    <Modal
      v-model:open="modalVisible"
      :title="editingId ? '编辑域名' : '添加域名'"
      ok-text="保存"
      cancel-text="取消"
      :confirm-loading="submitting"
      @ok="submitForm"
    >
      <Form layout="vertical">
        <Form.Item label="域名" required>
          <Input v-model:value="form.domain" placeholder="example.com" @press-enter="submitForm" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.domain-management { padding: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 24px; flex-wrap: wrap; }
.page-header h1 { margin: 0; font-size: 24px; font-weight: 600; color: var(--text-primary); }

@media (max-width: 768px) {
  .domain-management { padding: 16px 12px; }
  .domain-management .page-header { row-gap: 12px; }
  .domain-management .page-header .ant-btn { min-height: 40px; }
}
@media (max-width: 480px) {
  .domain-management .page-header h1 { font-size: 20px; }
}
</style>
