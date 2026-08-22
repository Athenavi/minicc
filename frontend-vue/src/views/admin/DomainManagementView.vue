<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { Card, Table, Button, Space, Tag, Input, Modal, Form, InputNumber, Switch, Alert, Select, Descriptions } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, CheckCircleOutlined, SyncOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'
import { useCrudResource, apiErrorMessage } from '../../composables/useCrudResource'

// State
const { data: domains, loading, load: loadDomains } = useCrudResource<any[]>(
  [],
  async () => (await api.get('/admin/domains')).data?.data || []
)
const selectedDomain = ref<any>(null)
const modalVisible = ref(false)
const dnsResult = ref<any>(null)
const currentForm = ref({
  domain: '',
  tenant_id: 'default',
  dns_provider: 'cloudflare',
  cname_target: '',
  auto_renew: true
})

// Table columns（customRender 用 h()，模板属性里不能写 JSX）
const columns = [
  { title: '域名', dataIndex: 'domain', key: 'domain', width: 250, ellipsis: true },
  { title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id', width: 150 },
  { title: 'DNS 提供商', dataIndex: 'dns_provider', key: 'dns_provider', width: 120,
    customRender: ({ record }: any) => record.dns_provider || '未设置' },
  { title: 'SSL 状态', key: 'ssl_status', width: 120,
    customRender: ({ record }: any) => {
      const label = record.ssl_status === 'active' ? '有效'
        : record.ssl_status === 'pending' ? '待签发'
        : record.ssl_status === 'expired' ? '已过期' : '失败'
      return h(Tag, { color: getSSLStatusColor(record.ssl_status) }, () => label)
    } },
  { title: 'SSL 过期时间', dataIndex: 'ssl_expires_at', key: 'ssl_expires_at', width: 180,
    customRender: ({ record }: any) => formatDate(record.ssl_expires_at) },
  { title: '自动续期', key: 'auto_renew', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: record.auto_renew ? 'green' : 'default' }, () => (record.auto_renew ? '开启' : '关闭')) },
  { title: '状态', key: 'status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: getDomainStatusColor(record.status) }, () => record.status) },
  { title: '操作', key: 'actions', width: 280, fixed: 'right' as const,
    customRender: ({ record }: any) => h(Space, () => [
      h(Button, { size: 'small', icon: h(CheckCircleOutlined), onClick: () => verifyDNS(record.id) }, () => '验证 DNS'),
      h(Button, { size: 'small', icon: h(SyncOutlined), onClick: () => renewSSL(record.id) }, () => '续期 SSL'),
      h(Button, { size: 'small', icon: h(ReloadOutlined), onClick: () => openEdit(record) }, () => '编辑')
    ]) }
]

// Open edit modal
function openEdit(record: any) {
  selectedDomain.value = record
  currentForm.value = { ...record }
  modalVisible.value = true
}

// Create domain
async function createDomain() {
  try {
    await api.post('/admin/domains', currentForm.value)
    modalVisible.value = false
    loadDomains()
  } catch (error: any) {
    alert(apiErrorMessage(error, '创建失败'))
  }
}

// Update domain
async function updateDomain() {
  try {
    await api.put(`/admin/domains/${selectedDomain.value.id}`, currentForm.value)
    modalVisible.value = false
    loadDomains()
  } catch (error: any) {
    alert(apiErrorMessage(error, '更新失败'))
  }
}

// Verify DNS
async function verifyDNS(id: string) {
  try {
    const response = await api.post(`/admin/domains/${id}/verify`)
    dnsResult.value = response.data
    loadDomains()
  } catch (error: any) {
    alert(apiErrorMessage(error, 'DNS 验证失败'))
  }
}

// Renew SSL
async function renewSSL(id: string) {
  if (!confirm('确定要续期 SSL 证书吗?')) return

  try {
    const response = await api.post(`/admin/domains/${id}/renew-ssl`)
    alert(response.data?.message || 'SSL 证书续期已启动')
    loadDomains()
  } catch (error: any) {
    alert(apiErrorMessage(error, 'SSL 续期失败'))
  }
}

// Helper functions
function getSSLStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'pending': return 'orange'
    case 'expired': return 'red'
    case 'failed': return 'red'
    default: return 'default'
  }
}

function getDomainStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'verifying': return 'blue'
    case 'inactive': return 'gray'
    default: return 'default'
  }
}

function formatDate(date: any): string {
  return date ? new Date(date).toLocaleString('zh-CN') : '未设置'
}

// Initialize
onMounted(() => {
  loadDomains()
})
</script>

<template>
  <div class="domain-management">
    <!-- Header -->
    <div class="page-header">
      <h1>🌐 域名管理</h1>
      <Button type="primary" @click="modalVisible = true">
        <PlusOutlined /> 添加域名
      </Button>
    </div>

    <!-- DNS Verification Notice -->
    <Alert
      message="DNS 验证说明"
      description="添加域名后,需要在 DNS 服务商处配置 CNAME 记录指向系统提供的目标地址,然后点击下方'验证 DNS'按钮进行验证。"
      type="info"
      showIcon
      style="margin-bottom: 16px"
    />

    <!-- Domain List -->
    <Card>
      <Table
        :columns="columns"
        :data-source="domains"
        :loading="loading"
        row-key="id"
        :scroll="{ x: 1400 }"
      >
        <template #emptyText>
          <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
        </template>
      </Table>
    </Card>

    <!-- DNS Result Modal -->
    <Modal
      :open="dnsResult !== null"
      title="DNS 验证结果"
      ok-text="确定"
      cancel-text="取消"
      @ok="dnsResult = null"
      @cancel="dnsResult = null"
    >
      <Alert
        :type="dnsResult?.success ? 'success' : 'error'"
        :message="dnsResult?.success ? '验证成功' : '验证失败'"
        :description="dnsResult?.message || ''"
        showIcon
        style="margin-bottom: 16px"
      />

      <Descriptions v-if="dnsResult?.details" :column="2" bordered>
        <Descriptions.Item label="记录类型">{{ dnsResult.details.record_type }}</Descriptions.Item>
        <Descriptions.Item label="记录名称">{{ dnsResult.details.record_name }}</Descriptions.Item>
        <Descriptions.Item label="记录值">{{ dnsResult.details.record_value }}</Descriptions.Item>
        <Descriptions.Item label="期望值">{{ dnsResult.expected_value }}</Descriptions.Item>
      </Descriptions>
    </Modal>

    <!-- Add/Edit Modal -->
    <Modal
      v-model:open="modalVisible"
      :title="selectedDomain ? '编辑域名' : '添加域名'"
      ok-text="确定"
      cancel-text="取消"
      @ok="selectedDomain ? updateDomain() : createDomain()"
    >
      <Form layout="vertical">
        <Form.Item label="域名" required>
          <Input v-model:value="currentForm.domain" placeholder="example.com" />
        </Form.Item>

        <Form.Item label="租户 ID" required>
          <Input v-model:value="currentForm.tenant_id" placeholder="default" />
        </Form.Item>

        <Form.Item label="DNS 提供商">
          <Select v-model:value="currentForm.dns_provider" placeholder="选择 DNS 提供商">
            <Select.Option value="cloudflare">Cloudflare</Select.Option>
            <Select.Option value="aliyun">阿里云</Select.Option>
            <Select.Option value="tencent">腾讯云</Select.Option>
            <Select.Option value="manual">手动配置</Select.Option>
          </Select>
        </Form.Item>

        <Form.Item label="CNAME 目标地址" required>
          <Input v-model:value="currentForm.cname_target" placeholder="cname.minicc.com" />
          <template #extra>
            <span class="hint">需要将域名 CNAME 记录指向此地址</span>
          </template>
        </Form.Item>

        <Form.Item label="启用自动续期">
          <Switch v-model:checked="currentForm.auto_renew" />
          <template #extra>
            <span class="hint">到期前自动续期 SSL 证书</span>
          </template>
        </Form.Item>

        <Alert
          message="SSL 证书说明"
          description="系统将使用 Let's Encrypt 为域名自动签发和续期免费 SSL 证书。"
          type="info"
          showIcon
        />
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.domain-management {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.hint { color: var(--text-secondary); font-size: 12px; }

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

/* 窄屏:页头换行、触控目标 ≥ 40px */
@media (max-width: 768px) {
  .domain-management .page-header {
    flex-wrap: wrap;
    row-gap: 12px;
  }
  .domain-management .page-header .ant-btn { min-height: 40px; }
}
</style>
