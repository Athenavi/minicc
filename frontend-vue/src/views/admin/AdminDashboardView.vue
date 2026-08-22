<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import {
  Card, Spin, Tabs, TabPane, message, Table, Button, Space, Tag, Input, Modal,
  Form, Select, DatePicker, Statistic, InputNumber, Switch, Popconfirm,
} from 'ant-design-vue'
import {
  PlusOutlined, EditOutlined, DeleteOutlined, KeyOutlined, ClusterOutlined,
  ThunderboltOutlined, ReloadOutlined,
} from '@ant-design/icons-vue'
import { api } from '../../api'
import { apiErrorMessage } from '../../composables/useCrudResource'
import EmptyState from '../../components/common/EmptyState.vue'

// ── 全局状态 ──
const loading = ref(true)
const activeTab = ref('dashboard')

const apiKeys = ref<any[]>([])
const models = ref<any[]>([])
const cronJobs = ref<any[]>([])
const totalKeys = ref(0)
const totalModels = ref(0)
const totalCron = ref(0)

// ── API Key 新建（真实后端，保持原有实现）──
const keyModalVisible = ref(false)
const newKeyForm = ref({
  name: '',
  tenant_id: 'default',
  monthly_quota: 0,
  expires_at: null as any,
  allowed_models: [] as string[],
  rate_limit_qps: 10,
  description: '',
})

async function createAPIKey() {
  try {
    const response = await api.post('/v1/admin/api-keys', newKeyForm.value)
    message.success('API Key 创建成功')

    // Show the key (only once!)
    Modal.info({
      title: '🔐 保存您的 API Key',
      content: h('div', [
        h('p', { style: { color: '#ff4d4f', fontWeight: 'bold' } }, '⚠️ 此密钥仅显示一次，请立即复制保存！'),
        h(Input, {
          value: response.data?.data?.key ?? response.data?.key ?? '',
          readonly: true,
          onClick: (e: Event) => (e.target as HTMLInputElement).select(),
        }),
      ]),
      width: 600,
    })

    keyModalVisible.value = false
    loadApiKeys()
  } catch (error: any) {
    message.error(error.response?.data?.error || '创建失败')
  }
}

// ── API Key 编辑 / 删除（真实端点：PUT/DELETE /v1/admin/api-keys/{id}）──
const keyEditVisible = ref(false)
const keyEditSubmitting = ref(false)
const keyEditForm = ref({ id: '', status: 'active' })

function openKeyEdit(record: any) {
  keyEditForm.value = { id: record.id, status: record.status || 'active' }
  keyEditVisible.value = true
}

async function submitKeyEdit() {
  keyEditSubmitting.value = true
  try {
    await api.put(`/v1/admin/api-keys/${keyEditForm.value.id}`, { status: keyEditForm.value.status })
    message.success('API Key 状态已更新')
    keyEditVisible.value = false
    await loadApiKeys()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '更新 API Key 失败'))
  } finally {
    keyEditSubmitting.value = false
  }
}

async function deleteApiKeyRow(record: any) {
  try {
    await api.delete(`/v1/admin/api-keys/${record.id}`)
    message.success('API Key 已删除')
    await loadApiKeys()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '删除 API Key 失败'))
  }
}

// ── 模型管理 ──
// GET /v1/admin/models → { models: [{ id, provider, name, display_name, enabled, context_window, created_at }] }
const modelModalVisible = ref(false)
const modelSubmitting = ref(false)
const modelEditingId = ref<string | null>(null)
const modelForm = ref({
  provider: '',
  name: '',
  display_name: '',
  enabled: true,
  context_window: 0,
})

async function loadModels() {
  try {
    const resp = await api.get('/v1/admin/models')
    const d = resp.data?.data || {}
    models.value = d.models || []
    totalModels.value = models.value.length
  } catch (e: any) {
    message.error(apiErrorMessage(e, '加载模型列表失败'))
  }
}

function openModelCreate() {
  modelEditingId.value = null
  modelForm.value = { provider: '', name: '', display_name: '', enabled: true, context_window: 0 }
  modelModalVisible.value = true
}

function openModelEdit(record: any) {
  modelEditingId.value = record.id
  modelForm.value = {
    provider: record.provider || '',
    name: record.name || '',
    display_name: record.display_name || '',
    enabled: !!record.enabled,
    context_window: Number(record.context_window) || 0,
  }
  modelModalVisible.value = true
}

async function submitModel() {
  if (!modelForm.value.provider.trim() || !modelForm.value.name.trim()) {
    message.warning('请填写提供商与模型名称')
    return
  }
  modelSubmitting.value = true
  try {
    if (modelEditingId.value) {
      // PUT 仅支持部分字段
      await api.put(`/v1/admin/models/${modelEditingId.value}`, {
        display_name: modelForm.value.display_name,
        enabled: modelForm.value.enabled,
        context_window: modelForm.value.context_window,
      })
      message.success('模型已更新')
    } else {
      await api.post('/v1/admin/models', {
        provider: modelForm.value.provider.trim(),
        name: modelForm.value.name.trim(),
        display_name: modelForm.value.display_name,
        enabled: modelForm.value.enabled,
        context_window: modelForm.value.context_window,
      })
      message.success('模型已创建')
    }
    modelModalVisible.value = false
    await loadModels()
  } catch (e: any) {
    message.error(apiErrorMessage(e, modelEditingId.value ? '更新模型失败' : '创建模型失败'))
  } finally {
    modelSubmitting.value = false
  }
}

async function deleteModel(record: any) {
  try {
    await api.delete(`/v1/admin/models/${record.id}`)
    message.success('模型已删除')
    await loadModels()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '删除模型失败'))
  }
}

// ── 定时任务管理 ──
// GET /v1/admin/cron-jobs → { jobs: [{ id, name, schedule, task, enabled, last_run_at, last_status, created_at }] }
const cronModalVisible = ref(false)
const cronSubmitting = ref(false)
const cronEditingId = ref<string | null>(null)
const cronForm = ref({
  name: '',
  schedule: '',
  task: '',
  enabled: true,
})

async function loadCronJobs() {
  try {
    const resp = await api.get('/v1/admin/cron-jobs')
    const d = resp.data?.data || {}
    cronJobs.value = d.jobs || []
    totalCron.value = cronJobs.value.length
  } catch (e: any) {
    message.error(apiErrorMessage(e, '加载定时任务失败'))
  }
}

function openCronCreate() {
  cronEditingId.value = null
  cronForm.value = { name: '', schedule: '', task: '', enabled: true }
  cronModalVisible.value = true
}

function openCronEdit(record: any) {
  cronEditingId.value = record.id
  cronForm.value = {
    name: record.name || '',
    schedule: record.schedule || '',
    task: record.task || '',
    enabled: !!record.enabled,
  }
  cronModalVisible.value = true
}

async function submitCron() {
  if (!cronForm.value.name.trim() || !cronForm.value.schedule.trim() || !cronForm.value.task.trim()) {
    message.warning('请填写任务名称、调度表达式与任务内容')
    return
  }
  cronSubmitting.value = true
  try {
    const body = {
      name: cronForm.value.name.trim(),
      schedule: cronForm.value.schedule.trim(),
      task: cronForm.value.task.trim(),
      enabled: cronForm.value.enabled,
    }
    if (cronEditingId.value) {
      await api.put(`/v1/admin/cron-jobs/${cronEditingId.value}`, body)
      message.success('定时任务已更新')
    } else {
      await api.post('/v1/admin/cron-jobs', body)
      message.success('定时任务已创建')
    }
    cronModalVisible.value = false
    await loadCronJobs()
  } catch (e: any) {
    message.error(apiErrorMessage(e, cronEditingId.value ? '更新定时任务失败' : '创建定时任务失败'))
  } finally {
    cronSubmitting.value = false
  }
}

async function deleteCron(record: any) {
  try {
    await api.delete(`/v1/admin/cron-jobs/${record.id}`)
    message.success('定时任务已删除')
    await loadCronJobs()
  } catch (e: any) {
    message.error(apiErrorMessage(e, '删除定时任务失败'))
  }
}

// ── 数据加载 ──
async function loadApiKeys() {
  try {
    const resp = await api.get('/v1/admin/api-keys', { params: { page: 1, page_size: 20 } })
    const body = resp.data?.data
    const list = Array.isArray(body) ? body : (body?.keys || [])
    apiKeys.value = list
    totalKeys.value = resp.data?.total ?? body?.total ?? list.length
  } catch (e: any) {
    message.error(apiErrorMessage(e, '加载 API Key 失败'))
  }
}

async function loadDashboardData() {
  loading.value = true
  try {
    await Promise.allSettled([loadApiKeys(), loadModels(), loadCronJobs()])
  } finally {
    loading.value = false
  }
}

// ── 表格列 ──
const recentKeyColumns = [
  { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id', width: 120 },
  { title: '状态', key: 'status', width: 100 },
  { title: '月度配额', key: 'quota', width: 120 },
  { title: '过期时间', key: 'expires', width: 170 },
]

const apiKeyColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '租户', dataIndex: 'tenant_id', key: 'tenant_id', width: 110 },
  { title: '状态', key: 'status', width: 100 },
  { title: '月度配额', key: 'quota', width: 110 },
  { title: '已用量', key: 'usage', width: 110 },
  { title: '速率限制', key: 'qps', width: 110 },
  { title: '过期时间', key: 'expires', width: 170 },
  { title: '操作', key: 'actions', width: 150, fixed: 'right' as const },
]

const modelColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '提供商', dataIndex: 'provider', key: 'provider', width: 130 },
  { title: '模型名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '显示名称', dataIndex: 'display_name', key: 'display_name', ellipsis: true },
  { title: '上下文窗口', key: 'context_window', width: 130 },
  { title: '启用', key: 'enabled', width: 90 },
  { title: '创建时间', key: 'created_at', width: 170 },
  { title: '操作', key: 'actions', width: 140, fixed: 'right' as const },
]

const cronColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 90 },
  { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '调度表达式', dataIndex: 'schedule', key: 'schedule', width: 130 },
  { title: '任务', dataIndex: 'task', key: 'task', ellipsis: true },
  { title: '启用', key: 'enabled', width: 90 },
  { title: '上次运行', key: 'last_run_at', width: 170 },
  { title: '上次状态', key: 'last_status', width: 110 },
  { title: '创建时间', key: 'created_at', width: 170 },
  { title: '操作', key: 'actions', width: 140, fixed: 'right' as const },
]

// ── 工具函数 ──
function formatDate(date: any): string {
  return date ? new Date(date).toLocaleString('zh-CN') : '永久有效'
}

function formatDateOrNever(date: any): string {
  return date ? new Date(date).toLocaleString('zh-CN') : '从未'
}

function getStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'expired': return 'red'
    case 'suspended': return 'orange'
    default: return 'default'
  }
}

function statusText(status: string): string {
  switch (status) {
    case 'active': return '活跃'
    case 'expired': return '已过期'
    case 'suspended': return '已挂起'
    default: return status || '-'
  }
}

function lastStatusColor(status: string): string {
  return status === 'success' ? 'green' : status === 'failed' ? 'red' : 'default'
}

function lastStatusText(status: string): string {
  if (!status) return '待执行'
  return status === 'success' ? '成功' : status === 'failed' ? '失败' : status
}

function quotaText(v: any): string {
  return !v || Number(v) === 0 ? '∞' : String(v)
}

function quotaTextPlain(v: any): string {
  return !v || Number(v) === 0 ? '无限制' : String(v)
}

onMounted(() => {
  loadDashboardData()
})
</script>

<template>
  <div class="admin-dashboard">
    <div class="dashboard-header">
      <h1>⚙️ 后端管理系统</h1>
      <Button type="primary" size="large" @click="activeTab = 'api-keys'">
        <template #icon><PlusOutlined /></template>
        新建 API Key
      </Button>
    </div>

    <Spin :spinning="loading">
      <Tabs v-model:activeKey="activeTab" type="card">
        <!-- 概览仪表盘 -->
        <TabPane key="dashboard" tab="概览仪表盘">
          <div class="stats-grid">
            <Card class="stat-card">
              <template #title><KeyOutlined /> API Keys</template>
              <Statistic :value="totalKeys" :precision="0" />
            </Card>
            <Card class="stat-card">
              <template #title><ClusterOutlined /> 模型配置</template>
              <Statistic :value="totalModels" :precision="0" />
            </Card>
            <Card class="stat-card">
              <template #title><ThunderboltOutlined /> 定时任务</template>
              <Statistic :value="totalCron" :precision="0" />
            </Card>
          </div>

          <Card title="最近创建的 API Key" class="recent-card">
            <Table :columns="recentKeyColumns" :data-source="apiKeys.slice(0, 5)" :pagination="false" :scroll="{ x: 640 }">
              <template #emptyText>
                <EmptyState description="暂无 API Key" />
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'status'">
                  <Tag :color="getStatusColor(record.status)">{{ statusText(record.status) }}</Tag>
                </template>
                <template v-else-if="column.key === 'quota'">
                  {{ quotaTextPlain(record.monthly_quota) }}
                </template>
                <template v-else-if="column.key === 'expires'">
                  {{ formatDate(record.expires_at) }}
                </template>
              </template>
            </Table>
          </Card>
        </TabPane>

        <!-- API Key 管理 -->
        <TabPane key="api-keys" tab="API Key 管理">
          <Card>
            <template #extra>
              <Space wrap>
                <Button @click="loadApiKeys">
                  <template #icon><ReloadOutlined /></template>
                  刷新
                </Button>
                <Button @click="keyModalVisible = true">
                  <template #icon><PlusOutlined /></template>
                  新建 API Key
                </Button>
              </Space>
            </template>

            <Table
              :columns="apiKeyColumns"
              :data-source="apiKeys"
              row-key="id"
              :pagination="{ pageSize: 20, total: totalKeys }"
              :scroll="{ x: 1100 }"
            >
              <template #emptyText>
                <EmptyState description="暂无 API Key" hint="点击右上角「新建 API Key」创建第一个密钥" />
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'status'">
                  <Tag :color="getStatusColor(record.status)">{{ statusText(record.status) }}</Tag>
                </template>
                <template v-else-if="column.key === 'quota'">
                  {{ quotaText(record.monthly_quota) }}
                </template>
                <template v-else-if="column.key === 'usage'">
                  {{ record.used_credits || 0 }} credits
                </template>
                <template v-else-if="column.key === 'qps'">
                  {{ record.rate_limit_qps }} QPS
                </template>
                <template v-else-if="column.key === 'expires'">
                  {{ formatDate(record.expires_at) }}
                </template>
                <template v-else-if="column.key === 'actions'">
                  <Space :size="4">
                    <Button size="small" @click="openKeyEdit(record)">
                      <template #icon><EditOutlined /></template>
                      编辑
                    </Button>
                    <Popconfirm
                      title="确定删除该 API Key 吗？"
                      ok-text="删除"
                      cancel-text="取消"
                      @confirm="deleteApiKeyRow(record)"
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
        </TabPane>

        <!-- 模型管理 -->
        <TabPane key="models" tab="模型管理">
          <Card>
            <template #extra>
              <Space wrap>
                <Button @click="loadModels">
                  <template #icon><ReloadOutlined /></template>
                  刷新
                </Button>
                <Button type="primary" @click="openModelCreate">
                  <template #icon><PlusOutlined /></template>
                  添加模型
                </Button>
              </Space>
            </template>

            <Table
              :columns="modelColumns"
              :data-source="models"
              row-key="id"
              :pagination="{ pageSize: 20, showSizeChanger: true }"
              :scroll="{ x: 1100 }"
            >
              <template #emptyText>
                <EmptyState description="暂无模型" hint="点击右上角「添加模型」登记第一个模型" />
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'context_window'">
                  {{ record.context_window || '-' }}
                </template>
                <template v-else-if="column.key === 'enabled'">
                  <Tag :color="record.enabled ? 'green' : 'default'">
                    {{ record.enabled ? '已启用' : '已禁用' }}
                  </Tag>
                </template>
                <template v-else-if="column.key === 'created_at'">
                  {{ formatDateOrNever(record.created_at) }}
                </template>
                <template v-else-if="column.key === 'actions'">
                  <Space :size="4">
                    <Button size="small" @click="openModelEdit(record)">
                      <template #icon><EditOutlined /></template>
                      编辑
                    </Button>
                    <Popconfirm
                      title="确定删除该模型吗？"
                      ok-text="删除"
                      cancel-text="取消"
                      @confirm="deleteModel(record)"
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
        </TabPane>

        <!-- 定时任务 -->
        <TabPane key="cron-jobs" tab="定时任务">
          <Card>
            <template #extra>
              <Space wrap>
                <Button @click="loadCronJobs">
                  <template #icon><ReloadOutlined /></template>
                  刷新
                </Button>
                <Button type="primary" @click="openCronCreate">
                  <template #icon><PlusOutlined /></template>
                  新建任务
                </Button>
              </Space>
            </template>

            <Table
              :columns="cronColumns"
              :data-source="cronJobs"
              row-key="id"
              :pagination="{ pageSize: 20, showSizeChanger: true }"
              :scroll="{ x: 1200 }"
            >
              <template #emptyText>
                <EmptyState description="暂无定时任务" hint="点击右上角「新建任务」创建第一个定时任务" />
              </template>
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'enabled'">
                  <Tag :color="record.enabled ? 'green' : 'default'">
                    {{ record.enabled ? '已启用' : '已禁用' }}
                  </Tag>
                </template>
                <template v-else-if="column.key === 'last_run_at'">
                  {{ formatDateOrNever(record.last_run_at) }}
                </template>
                <template v-else-if="column.key === 'last_status'">
                  <Tag :color="lastStatusColor(record.last_status)">{{ lastStatusText(record.last_status) }}</Tag>
                </template>
                <template v-else-if="column.key === 'created_at'">
                  {{ formatDateOrNever(record.created_at) }}
                </template>
                <template v-else-if="column.key === 'actions'">
                  <Space :size="4">
                    <Button size="small" @click="openCronEdit(record)">
                      <template #icon><EditOutlined /></template>
                      编辑
                    </Button>
                    <Popconfirm
                      title="确定删除该定时任务吗？"
                      ok-text="删除"
                      cancel-text="取消"
                      @confirm="deleteCron(record)"
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
        </TabPane>
      </Tabs>
    </Spin>

    <!-- 创建 API Key 弹窗 -->
    <Modal
      v-model:open="keyModalVisible"
      title="创建新的 API Key"
      :width="600"
      @ok="createAPIKey"
    >
      <Form layout="vertical">
        <Form.Item label="Key 名称" required>
          <Input v-model:value="newKeyForm.name" placeholder="例如: 生产环境 Key" />
        </Form.Item>
        <Form.Item label="租户 ID" required>
          <Input v-model:value="newKeyForm.tenant_id" placeholder="默认: default" />
        </Form.Item>
        <Form.Item label="月度配额 (credits, 0=无限制)">
          <InputNumber v-model:value="newKeyForm.monthly_quota" style="width: 100%" placeholder="10000" />
        </Form.Item>
        <Form.Item label="速率限制 (QPS)">
          <InputNumber v-model:value="newKeyForm.rate_limit_qps" style="width: 100%" placeholder="10" />
        </Form.Item>
        <Form.Item label="允许使用的模型">
          <Select
            v-model:value="newKeyForm.allowed_models"
            mode="multiple"
            placeholder="gpt-4, claude-3"
            style="width: 100%"
          >
            <Select.Option value="gpt-4-turbo">GPT-4 Turbo</Select.Option>
            <Select.Option value="gpt-3.5-turbo">GPT-3.5 Turbo</Select.Option>
            <Select.Option value="claude-3-opus">Claude 3 Opus</Select.Option>
            <Select.Option value="claude-3-sonnet">Claude 3 Sonnet</Select.Option>
            <Select.Option value="deepseek-coder">DeepSeek Coder</Select.Option>
          </Select>
        </Form.Item>
        <Form.Item label="过期时间">
          <DatePicker v-model:value="newKeyForm.expires_at" style="width: 100%" show-time placeholder="永久有效" />
        </Form.Item>
        <Form.Item label="描述">
          <Input.TextArea v-model:value="newKeyForm.description" :rows="3" placeholder="可选说明..." />
        </Form.Item>
      </Form>
    </Modal>

    <!-- 编辑 API Key 弹窗 -->
    <Modal
      v-model:open="keyEditVisible"
      title="编辑 API Key 状态"
      ok-text="保存"
      cancel-text="取消"
      :confirm-loading="keyEditSubmitting"
      @ok="submitKeyEdit"
    >
      <Form layout="vertical">
        <Form.Item label="状态">
          <Select v-model:value="keyEditForm.status">
            <Select.Option value="active">活跃</Select.Option>
            <Select.Option value="suspended">已挂起</Select.Option>
            <Select.Option value="expired">已过期</Select.Option>
          </Select>
        </Form.Item>
      </Form>
    </Modal>

    <!-- 模型弹窗 -->
    <Modal
      v-model:open="modelModalVisible"
      :title="modelEditingId ? '编辑模型' : '添加模型'"
      ok-text="保存"
      cancel-text="取消"
      :confirm-loading="modelSubmitting"
      @ok="submitModel"
    >
      <Form layout="vertical">
        <Form.Item label="提供商" required>
          <Input v-model:value="modelForm.provider" placeholder="例如: deepseek" :disabled="!!modelEditingId" />
        </Form.Item>
        <Form.Item label="模型名称" required>
          <Input v-model:value="modelForm.name" placeholder="例如: deepseek-chat" :disabled="!!modelEditingId" />
        </Form.Item>
        <Form.Item label="显示名称">
          <Input v-model:value="modelForm.display_name" placeholder="例如: DeepSeek Chat" />
        </Form.Item>
        <Form.Item label="上下文窗口">
          <InputNumber v-model:value="modelForm.context_window" style="width: 100%" :min="0" placeholder="0" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="modelForm.enabled" />
        </Form.Item>
      </Form>
    </Modal>

    <!-- 定时任务弹窗 -->
    <Modal
      v-model:open="cronModalVisible"
      :title="cronEditingId ? '编辑定时任务' : '新建定时任务'"
      ok-text="保存"
      cancel-text="取消"
      :confirm-loading="cronSubmitting"
      @ok="submitCron"
    >
      <Form layout="vertical">
        <Form.Item label="任务名称" required>
          <Input v-model:value="cronForm.name" placeholder="例如: 每日清理临时数据" />
        </Form.Item>
        <Form.Item label="调度表达式" required>
          <Input v-model:value="cronForm.schedule" placeholder="Cron 表达式，例如 0 3 * * *" />
        </Form.Item>
        <Form.Item label="任务内容" required>
          <Input.TextArea v-model:value="cronForm.task" :rows="3" placeholder="任务描述或命令" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch v-model:checked="cronForm.enabled" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.admin-dashboard { padding: clamp(12px, 2vw, 24px); }
.dashboard-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 24px; flex-wrap: wrap; }
.dashboard-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
.stat-card { text-align: center; min-width: 0; }
.recent-card { margin-top: 16px; }

@media (max-width: 576px) {
  .dashboard-header h1 { font-size: 20px; }
  .admin-dashboard :deep(.ant-btn:not(.ant-btn-sm):not(.ant-btn-link)) { min-height: 40px; }
}
</style>
