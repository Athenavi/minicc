<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { Card, Table, Button, Space, Tag, Input, Modal, Form, InputNumber, DatePicker, Select, Statistic, Tabs, TabPane } from 'ant-design-vue'
import { PlusOutlined, EditOutlined, DeleteOutlined, StopOutlined, BarChartOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'
import { useCrudResource, apiErrorMessage } from '../../composables/useCrudResource'
// 后端接口开发中（前端先行）：跳过真实请求，展示降级横幅
const backendMissing = true

// State
const totalTenants = ref(0)
const selectedTenant = ref<any>(null)
const usageData = ref<any[]>([])

// Filters
const searchText = ref('')
const statusFilter = ref<string | undefined>(undefined)

// Load tenants（loader 闭包读取筛选状态，reload 时自动生效）
const { data: tenants, loading, load: loadTenants } = useCrudResource<any[]>([], async () => {
  const params: any = { page: 1, page_size: 20 }
  if (searchText.value) params.search = searchText.value
  if (statusFilter.value) params.status = statusFilter.value

  const response = await api.get('/admin/tenants', { params })
  totalTenants.value = response.data?.total || 0
  return response.data?.data || []
})

// CRUD state
const modalVisible = ref(false)
const editModalVisible = ref(false)
const currentForm = ref({
  tenant_id: '',
  name: '',
  company_name: '',
  contact_email: '',
  contact_phone: '',
  max_api_keys: 10,
  max_models: 5,
  monthly_quota: 0,
  max_concurrent_sessions: 10,
  expires_at: null as any,
  features: {} as any
})

// Table columns（customRender 用 h()，模板属性里不能写 JSX）
const columns = [
  { title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id', width: 150, fixed: 'left' as const },
  { title: '名称', dataIndex: 'name', key: 'name', ellipsis: true },
  { title: '公司', dataIndex: 'company_name', key: 'company_name', ellipsis: true },
  { title: '状态', key: 'status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: getStatusColor(record.status) }, () => record.status) },
  { title: 'API Keys', key: 'api_keys', width: 100,
    customRender: ({ record }: any) => `${Object.keys(record.features || {}).length}/${record.max_api_keys}` },
  { title: '月度配额', dataIndex: 'monthly_quota', key: 'monthly_quota', width: 120,
    customRender: ({ record }: any) => record.monthly_quota === 0 ? '无限制' : record.monthly_quota.toLocaleString() },
  { title: '并发会话', dataIndex: 'max_concurrent_sessions', key: 'max_concurrent_sessions', width: 100 },
  { title: '过期时间', dataIndex: 'expires_at', key: 'expires_at', width: 160,
    customRender: ({ record }: any) => formatDate(record.expires_at) },
  { title: '操作', key: 'actions', width: 200, fixed: 'right' as const,
    customRender: ({ record }: any) => h(Space, () => [
      h(Button, { size: 'small', icon: h(BarChartOutlined), onClick: () => { selectedTenant.value = record; getUsage(record.tenant_id) } }, () => '用量'),
      h(Button, { size: 'small', icon: h(EditOutlined), onClick: () => openEdit(record) }, () => '编辑'),
      h(Button, { size: 'small', danger: true, icon: h(StopOutlined), onClick: () => suspendTenant(record.tenant_id) }, () => '暂停')
    ]) }
]

const usageColumns = [
  { title: '日期', dataIndex: 'stat_date', key: 'stat_date' },
  { title: 'API 调用', dataIndex: 'api_calls', key: 'api_calls' },
  { title: 'Token 使用', dataIndex: 'tokens_used', key: 'tokens_used' },
  { title: 'Credits 消耗', dataIndex: 'credits_consumed', key: 'credits_consumed' },
  { title: '存储空间 (MB)', dataIndex: 'storage_mb', key: 'storage_mb' }
]

// Open edit modal
function openEdit(record: any) {
  selectedTenant.value = record
  currentForm.value = { ...currentForm.value, ...record }
  editModalVisible.value = true
}

// Create tenant
async function createTenant() {
  try {
    await api.post('/admin/tenants', currentForm.value)
    modalVisible.value = false
    loadTenants()
  } catch (error: any) {
    alert(apiErrorMessage(error, '创建失败'))
  }
}

// Update tenant
async function updateTenant() {
  try {
    await api.put(`/admin/tenants/${selectedTenant.value.tenant_id}`, currentForm.value)
    editModalVisible.value = false
    loadTenants()
  } catch (error: any) {
    alert(apiErrorMessage(error, '更新失败'))
  }
}

// Suspend tenant
async function suspendTenant(tenantId: string) {
  if (!confirm('确定要暂停该租户吗?')) return

  try {
    await api.post(`/admin/tenants/${tenantId}/suspend`)
    loadTenants()
  } catch (error: any) {
    alert(apiErrorMessage(error, '操作失败'))
  }
}

// Get tenant usage
async function getUsage(tenantId: string) {
  try {
    const response = await api.get(`/admin/tenants/${tenantId}/usage`)
    usageData.value = response.data?.data || []
  } catch (error: any) {
    console.error('Failed to get usage:', error)
  }
}

// Helper functions
function getStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'suspended': return 'orange'
    case 'expired': return 'red'
    default: return 'default'
  }
}

function formatDate(date: any): string {
  return date ? new Date(date).toLocaleString('zh-CN') : '永久有效'
}

// Initialize
onMounted(() => {
    if (backendMissing) return
  loadTenants()
})
</script>

<template>
<a-alert v-if="backendMissing" type="warning" show-icon message="该管理能力后端接口开发中（前端界面先行），暂不可用" style="margin: 16px" />
  <div class="tenant-management">
    <!-- Header -->
    <div class="page-header">
      <h1>🏢 租户管理</h1>
      <Button type="primary" @click="modalVisible = true">
        <PlusOutlined /> 新建租户
      </Button>
    </div>

    <!-- Filters -->
    <Card style="margin-bottom: 16px">
      <Space class="filter-bar">
        <Input.Search
          v-model:value="searchText"
          placeholder="搜索租户名称或 ID..."
          style="width: 300px"
          @search="loadTenants"
        />
        <Select
          v-model:value="statusFilter"
          placeholder="筛选状态"
          style="width: 150px"
          allowClear
          @change="loadTenants"
        >
          <Select.Option value="active">活跃</Select.Option>
          <Select.Option value="suspended">已暂停</Select.Option>
          <Select.Option value="expired">已过期</Select.Option>
        </Select>
        <Button @click="loadTenants">刷新</Button>
      </Space>
    </Card>

    <!-- Tenant List -->
    <Card>
      <Table
        :columns="columns"
        :data-source="tenants"
        :loading="loading"
        :pagination="{ pageSize: 20, total: totalTenants, showSizeChanger: true }"
        :scroll="{ x: 1200 }"
      >
        <template #emptyText>
          <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
        </template>
      </Table>
    </Card>

    <!-- Usage Statistics Modal -->
    <Modal
      :open="selectedTenant !== null"
      :title="`📊 ${selectedTenant?.name ?? ''} - 用量统计`"
      :footer="null"
      :width="800"
      @cancel="selectedTenant = null"
    >
      <Tabs>
        <TabPane tab="日用量统计" key="daily">
          <Table
            :columns="usageColumns"
            :data-source="usageData"
            :pagination="false"
            :scroll="{ x: 640 }"
          >
            <template #emptyText>
              <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
            </template>
          </Table>
        </TabPane>
      </Tabs>
    </Modal>

    <!-- Create Tenant Modal -->
    <Modal
      v-model:open="modalVisible"
      title="创建新租户"
      :width="600"
      @ok="createTenant"
    >
      <Form layout="vertical">
        <Form.Item label="租户 ID" required>
          <Input v-model:value="currentForm.tenant_id" placeholder="例如: acme_corp" />
        </Form.Item>

        <Form.Item label="租户名称" required>
          <Input v-model:value="currentForm.name" placeholder="例如: ACME Corporation" />
        </Form.Item>

        <Form.Item label="公司名称">
          <Input v-model:value="currentForm.company_name" placeholder="可选" />
        </Form.Item>

        <Form.Item label="联系邮箱">
          <Input v-model:value="currentForm.contact_email" type="email" placeholder="admin@acme.com" />
        </Form.Item>

        <Form.Item label="联系电话">
          <Input v-model:value="currentForm.contact_phone" placeholder="+86-xxx-xxxx-xxxx" />
        </Form.Item>

        <Form.Item label="最大 API Keys">
          <InputNumber v-model:value="currentForm.max_api_keys" style="width: 100%" :min="1" />
        </Form.Item>

        <Form.Item label="最大模型数">
          <InputNumber v-model:value="currentForm.max_models" style="width: 100%" :min="1" />
        </Form.Item>

        <Form.Item label="月度 Credits 配额 (0=无限制)">
          <InputNumber v-model:value="currentForm.monthly_quota" style="width: 100%" :min="0" placeholder="100000" />
        </Form.Item>

        <Form.Item label="最大并发会话">
          <InputNumber v-model:value="currentForm.max_concurrent_sessions" style="width: 100%" :min="1" />
        </Form.Item>

        <Form.Item label="过期时间">
          <DatePicker
            v-model:value="currentForm.expires_at"
            style="width: 100%"
            show-time
            placeholder="永久有效"
          />
        </Form.Item>
      </Form>
    </Modal>

    <!-- Edit Tenant Modal -->
    <Modal
      v-model:open="editModalVisible"
      title="编辑租户"
      :width="600"
      @ok="updateTenant"
    >
      <Form layout="vertical">
        <Form.Item label="租户名称">
          <Input v-model:value="currentForm.name" />
        </Form.Item>

        <Form.Item label="月度 Credits 配额">
          <InputNumber v-model:value="currentForm.monthly_quota" style="width: 100%" :min="0" />
        </Form.Item>

        <Form.Item label="最大并发会话">
          <InputNumber v-model:value="currentForm.max_concurrent_sessions" style="width: 100%" :min="1" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.tenant-management { padding: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
.page-header h1 { margin: 0; font-size: 24px; font-weight: 600; }

/* 空状态统一 */
.empty-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 28px 0;
  color: var(--text-tertiary);
}
.empty-icon { font-size: 26px; line-height: 1; opacity: 0.8; }
.empty-text { font-size: 13px; }

/* 窄屏:页头换行、筛选表单竖排、触控目标 ≥ 40px */
@media (max-width: 768px) {
  .tenant-management .page-header { flex-wrap: wrap; row-gap: 12px; }
  .tenant-management .page-header .ant-btn { min-height: 40px; }
  .tenant-management .filter-bar { width: 100%; }
  .tenant-management .filter-bar :deep(.ant-space-item) { flex: 1 1 100%; min-width: 0; }
  .tenant-management .filter-bar :deep(.ant-input-group-wrapper),
  .tenant-management .filter-bar :deep(.ant-select) { width: 100% !important; }
  .tenant-management .filter-bar :deep(.ant-btn) { min-height: 40px; }
}
</style>
