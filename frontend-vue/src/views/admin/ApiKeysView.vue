<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Card, Row, Col, Statistic, Table, Modal, Form, FormItem, Input, Select, Tag, Button, Spin, message } from 'ant-design-vue'
import { PlusOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined } from '@ant-design/icons-vue'
import { listApiKeys, addApiKey, updateApiKey, deleteApiKey } from '@/api/admin'

const loading = ref(false)
const addLoading = ref(false)
const showAddModal = ref(false)

const formData = ref({
  provider: 'anthropic',
  key: '',
  remark: '',
})

const providerOptions = [
  { label: 'Anthropic', value: 'anthropic' },
  { label: 'OpenAI', value: 'openai' },
  { label: 'DeepSeek', value: 'deepseek' },
]

const apiKeys = ref<any[]>([])

const stats = computed(() => {
  const keys = apiKeys.value
  return {
    total: keys.length,
    active: keys.filter(k => k.status === 'active').length,
    rateLimited: keys.filter(k => k.status === 'rate_limited').length,
    circuitOpen: keys.filter(k => k.status === 'circuit_open').length,
  }
})

const columns = [
  { title: 'ID', dataIndex: 'id', width: 120 },
  { title: 'Provider', dataIndex: 'provider', width: 120 },
  { title: 'Key', dataIndex: 'key_preview', width: 180 },
  { title: '状态', dataIndex: 'status', width: 100 },
  { title: '权重', dataIndex: 'weight', width: 80 },
  { title: '失败次数', dataIndex: 'failures', width: 100 },
  { title: '最后使用', dataIndex: 'last_used', width: 180 },
  { title: '备注', dataIndex: 'remark' },
  { title: '操作', dataIndex: 'actions', width: 150 },
]

async function fetchApiKeys() {
  loading.value = true
  try {
    const keys = await listApiKeys()
    apiKeys.value = keys
  } catch (err: any) {
    console.error('Failed to fetch API keys:', err)
  } finally {
    loading.value = false
  }
}

async function handleAdd() {
  if (!formData.value.key) {
    message.warning('请输入 API Key')
    return
  }

  addLoading.value = true
  try {
    await addApiKey({
      provider: formData.value.provider,
      key: formData.value.key,
      remark: formData.value.remark,
    })
    message.success('API Key 已添加')
    showAddModal.value = false
    formData.value = { provider: 'anthropic', key: '', remark: '' }
    await fetchApiKeys()
  } catch (err: any) {
    message.error('添加失败: ' + (err.message || '未知错误'))
  } finally {
    addLoading.value = false
  }
}

async function handleEdit(row: any) {
  const newStatus = row.status === 'active' ? 'rate_limited' : 'active'
  try {
    await updateApiKey(row.id, { status: newStatus })
    message.success('状态已更新')
    await fetchApiKeys()
  } catch (err: any) {
    message.error('更新失败: ' + (err.message || '未知错误'))
  }
}

async function handleDelete(row: any) {
  try {
    await deleteApiKey(row.id)
    message.success('API Key 已删除')
    await fetchApiKeys()
  } catch (err: any) {
    message.error('删除失败: ' + (err.message || '未知错误'))
  }
}

onMounted(() => {
  fetchApiKeys()
})
</script>

<template>
  <div class="api-keys">
    <Spin :spinning="loading">
      <Card title="API Key 管理">
        <template #extra>
          <Button type="primary" @click="showAddModal = true">
            <template #icon><PlusOutlined /></template>
            添加 Key
          </Button>
        </template>

        <!-- 统计卡片 -->
        <Row :gutter="16" style="margin-bottom: 16px">
          <Col :span="6">
            <Statistic title="总 Key 数" :value="stats.total" />
          </Col>
          <Col :span="6">
            <Statistic title="正常" :value="stats.active">
              <template #prefix><CheckCircleOutlined style="color: #18a058" /></template>
            </Statistic>
          </Col>
          <Col :span="6">
            <Statistic title="限流中" :value="stats.rateLimited">
              <template #prefix><WarningOutlined style="color: #f0a020" /></template>
            </Statistic>
          </Col>
          <Col :span="6">
            <Statistic title="熔断" :value="stats.circuitOpen">
              <template #prefix><CloseCircleOutlined style="color: #d03050" /></template>
            </Statistic>
          </Col>
        </Row>

        <Table :columns="columns" :dataSource="apiKeys" :pagination="false">
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'status'">
              <Tag :color="record.status === 'active' ? 'success' : record.status === 'rate_limited' ? 'warning' : 'error'">
                {{ record.status === 'active' ? '正常' : record.status === 'rate_limited' ? '限流中' : '熔断' }}
              </Tag>
            </template>
            <template v-else-if="column.dataIndex === 'actions'">
              <Button type="link" size="small" @click="handleEdit(record)">编辑</Button>
              <Button type="link" danger size="small" @click="handleDelete(record)">删除</Button>
            </template>
          </template>
        </Table>
      </Card>
    </Spin>

    <!-- 添加 Key 弹窗 -->
    <Modal v-model:visible="showAddModal" title="添加 API Key" :footer="null" destroyOnClose>
      <Form :model="formData" layout="vertical">
        <FormItem label="Provider">
          <Select v-model:value="formData.provider" :options="providerOptions" style="width: 100%" />
        </FormItem>
        <FormItem label="API Key">
          <Input v-model:value="formData.key" placeholder="sk-..." />
        </FormItem>
        <FormItem label="备注">
          <Input v-model:value="formData.remark" placeholder="可选" />
        </FormItem>
      </Form>
      <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px">
        <Button @click="showAddModal = false">取消</Button>
        <Button type="primary" :loading="addLoading" @click="handleAdd">添加</Button>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.api-keys { padding: 0; }
</style>
