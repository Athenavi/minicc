<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { Card, Spin, Tabs, TabPane, message, Table, Button, Space, Tag, Input, Modal, Form, Select, DatePicker, Statistic, InputNumber } from 'ant-design-vue'
import { PlusOutlined, EditOutlined, DeleteOutlined, KeyOutlined, ClusterOutlined, ThunderboltOutlined, ClockCircleOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'

// State
const loading = ref(true)
const activeTab = ref('dashboard')
const apiKeys = ref<any[]>([])
const models = ref<any[]>([])
const cronJobs = ref<any[]>([])
const totalKeys = ref(0)
const totalModels = ref(0)
const totalCron = ref(0)

// API Key Modal
const keyModalVisible = ref(false)
const newKeyForm = ref({
  name: '',
  tenant_id: 'default',
  monthly_quota: 0,
  expires_at: null as any,
  allowed_models: [] as string[],
  rate_limit_qps: 10,
  description: ''
})

// Table columns（customRender 用 h()，模板属性里不能写 JSX）
const recentKeyColumns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id' },
  { title: '状态', key: 'status',
    customRender: ({ record }: any) =>
      h(Tag, { color: getStatusColor(record.status) }, () => record.status) },
  { title: '月度配额', dataIndex: 'monthly_quota', key: 'monthly_quota',
    customRender: ({ record }: any) => record.monthly_quota === 0 ? '无限制' : record.monthly_quota },
  { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at',
    customRender: ({ record }: any) => formatDate(record.expires_at) }
]

const apiKeyColumns = [
  { title: 'ID', dataIndex: 'id', key: 'id', width: 80 },
  { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '租户', dataIndex: 'tenant_id', key: 'tenant_id', width: 100 },
  { title: '状态', key: 'status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: getStatusColor(record.status) }, () => record.status) },
  { title: '月度配额', dataIndex: 'monthly_quota', key: 'monthly_quota', width: 120,
    customRender: ({ record }: any) => record.monthly_quota === 0 ? '∞' : record.monthly_quota.toString() },
  { title: '已用量', key: 'usage', width: 100,
    customRender: ({ record }: any) => `${record.used_credits || 0} credits` },
  { title: '速率限制', dataIndex: 'rate_limit_qps', key: 'rate_limit_qps', width: 100,
    customRender: ({ record }: any) => `${record.rate_limit_qps} QPS` },
  { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 160,
    customRender: ({ record }: any) => formatDate(record.expires_at) },
  { title: '操作', key: 'actions', width: 150, fixed: 'right' as const,
    customRender: () => h(Space, () => [
      h(Button, { size: 'small', icon: h(EditOutlined) }),
      h(Button, { size: 'small', danger: true, icon: h(DeleteOutlined) })
    ]) }
]

const modelColumns = [
  { title: '模型 ID', dataIndex: 'model_id', key: 'model_id' },
  { title: '显示名称', dataIndex: 'display_name', key: 'display_name' },
  { title: '提供商', dataIndex: 'provider', key: 'provider', width: 120 },
  { title: '优先级', dataIndex: 'priority', key: 'priority', width: 80 },
  { title: '权重', dataIndex: 'weight', key: 'weight', width: 80 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: getStatusColor(record.status) }, () => record.status) },
  { title: '成本(Input)', key: 'input_cost', width: 120,
    customRender: ({ record }: any) => `${record.input_cost_per_1m} credits/1M tokens` },
  { title: '成本(Output)', key: 'output_cost', width: 130,
    customRender: ({ record }: any) => `${record.output_cost_per_1m} credits/1M tokens` }
]

const cronColumns = [
  { title: '任务 ID', dataIndex: 'job_id', key: 'job_id' },
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '调度表达式', dataIndex: 'schedule', key: 'schedule', width: 150 },
  { title: '上次运行', dataIndex: 'last_run_at', key: 'last_run_at',
    customRender: ({ record }: any) => record.last_run_at ? new Date(record.last_run_at).toLocaleString('zh-CN') : '从未' },
  { title: '上次状态', dataIndex: 'last_run_status', key: 'last_run_status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: record.last_run_status === 'success' ? 'green' : 'red' }, () => (record.last_run_status || '待执行')) },
  { title: '启用状态', dataIndex: 'enabled', key: 'enabled', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: record.enabled ? 'green' : 'default' }, () => (record.enabled ? '已启用' : '已禁用')) }
]

// Create API Key
async function createAPIKey() {
  try {
    const response = await api.post('/admin/api-keys', newKeyForm.value)
    message.success('API Key 创建成功')

    // Show the key (only once!)
    Modal.info({
      title: '🔐 保存您的 API Key',
      content: h('div', [
        h('p', { style: { color: '#ff4d4f', fontWeight: 'bold' } }, '⚠️ 此密钥仅显示一次，请立即复制保存！'),
        h(Input, {
          value: response.data.key,
          readonly: true,
          onClick: (e: Event) => (e.target as HTMLInputElement).select()
        })
      ]),
      width: 600,
    })

    keyModalVisible.value = false
    loadDashboardData()
  } catch (error: any) {
    message.error(error.response?.data?.error || '创建失败')
  }
}

// Load Dashboard Data
async function loadDashboardData() {
  try {
    loading.value = true

    const [keysRes, modelsRes, jobsRes] = await Promise.all([
      api.get('/admin/api-keys?page=1&page_size=10').catch(() => ({ data: { data: [] } })),
      api.get('/admin/models').catch(() => ({ data: { data: [] } })),
      api.get('/admin/cron-jobs').catch(() => ({ data: { data: [] } }))
    ])

    apiKeys.value = keysRes.data?.data || []
    models.value = modelsRes.data?.data || []
    cronJobs.value = jobsRes.data?.data || []
    totalKeys.value = keysRes.data?.total || 0
    totalModels.value = modelsRes.data?.total || 0
    totalCron.value = cronJobs.value.length

  } catch (error: any) {
    message.error(error.response?.data?.error || '加载数据失败')
  } finally {
    loading.value = false
  }
}

// Helper functions
function formatDate(date: any): string {
  return date ? new Date(date).toLocaleString('zh-CN') : '永久有效'
}

function getStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'expired': return 'red'
    case 'suspended': return 'orange'
    default: return 'default'
  }
}

// Initialize
onMounted(() => {
  loadDashboardData()
})
</script>

<template>
  <div class="admin-dashboard">
    <!-- Header -->
    <div class="dashboard-header">
      <h1>⚙️ 后端管理系统</h1>
      <Button type="primary" @click="activeTab = 'api-keys'">
        <PlusOutlined /> 新建 API Key
      </Button>
    </div>

    <Spin :spinning="loading">
      <Tabs v-model:activeKey="activeTab" type="card">

        <!-- Dashboard Overview -->
        <TabPane key="dashboard" tab="概览仪表盘">
          <div class="stats-grid">
            <Card class="stat-card">
              <template #title>
                <KeyOutlined /> API Keys
              </template>
              <Statistic :value="totalKeys" :precision="0" />
            </Card>

            <Card class="stat-card">
              <template #title>
                <ClusterOutlined /> 模型配置
              </template>
              <Statistic :value="totalModels" :precision="0" />
            </Card>

            <Card class="stat-card">
              <template #title>
                <ThunderboltOutlined /> 定时任务
              </template>
              <Statistic :value="totalCron" :precision="0" />
            </Card>
          </div>

          <!-- Recent API Keys -->
          <Card title="最近创建的 API Key" style="margin-top: 24px">
            <Table :columns="recentKeyColumns" :data-source="apiKeys.slice(0, 5)" :pagination="false" />
          </Card>
        </TabPane>

        <!-- API Key Management -->
        <TabPane key="api-keys" tab="API Key 管理">
          <Card>
            <template #extra>
              <Space>
                <Input.Search
                  placeholder="搜索名称..."
                  style="width: 200px"
                />
                <Button @click="keyModalVisible = true">
                  <PlusOutlined /> 新建 API Key
                </Button>
              </Space>
            </template>

            <Table
              :columns="apiKeyColumns"
              :data-source="apiKeys"
              :pagination="{ pageSize: 20, total: totalKeys }"
            />
          </Card>
        </TabPane>

        <!-- Model Configuration -->
        <TabPane key="models" tab="模型编排">
          <Card>
            <template #extra>
              <Button type="primary">
                <PlusOutlined /> 添加模型
              </Button>
            </template>

            <Table
              :columns="modelColumns"
              :data-source="models"
              :pagination="false"
            />
          </Card>
        </TabPane>

        <!-- Cron Jobs -->
        <TabPane key="cron-jobs" tab="定时任务">
          <Card>
            <Table
              :columns="cronColumns"
              :data-source="cronJobs"
              :pagination="false"
            />
          </Card>
        </TabPane>
      </Tabs>
    </Spin>

    <!-- Create API Key Modal -->
    <Modal
      v-model:open="keyModalVisible"
      title="创建新的 API Key"
      :width="600"
      @ok="createAPIKey"
    >
      <Form layout="vertical">
        <Form.Item label="Key 名称" required>
          <Input
            v-model:value="newKeyForm.name"
            placeholder="例如: 生产环境 Key"
          />
        </Form.Item>

        <Form.Item label="租户 ID" required>
          <Input
            v-model:value="newKeyForm.tenant_id"
            placeholder="默认: default"
          />
        </Form.Item>

        <Form.Item label="月度配额 (credits, 0=无限制)">
          <InputNumber
            v-model:value="newKeyForm.monthly_quota"
            style="width: 100%"
            placeholder="10000"
          />
        </Form.Item>

        <Form.Item label="速率限制 (QPS)">
          <InputNumber
            v-model:value="newKeyForm.rate_limit_qps"
            style="width: 100%"
            placeholder="10"
          />
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
          <DatePicker
            v-model:value="newKeyForm.expires_at"
            style="width: 100%"
            show-time
            placeholder="永久有效"
          />
        </Form.Item>

        <Form.Item label="描述">
          <Input.TextArea
            v-model:value="newKeyForm.description"
            :rows="3"
            placeholder="可选说明..."
          />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.admin-dashboard { padding: 24px; }
.dashboard-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
.dashboard-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.stats-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 16px; }
.stat-card { text-align: center; }
</style>
