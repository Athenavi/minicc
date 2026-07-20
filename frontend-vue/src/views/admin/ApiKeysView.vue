<template>
  <div class="api-keys">
    <n-spin :show="loading">
      <n-card title="API Key 管理">
        <template #header-extra>
          <n-button type="primary" @click="showAddModal = true">
            <template #icon>
              <n-icon><svg viewBox="0 0 24 24"><path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg></n-icon>
            </template>
            添加 Key
          </n-button>
        </template>
        
        <!-- 统计卡片 -->
        <n-grid :cols="4" :x-gap="16" :y-gap="16" style="margin-bottom: 16px">
          <n-grid-item>
            <n-statistic label="总 Key 数" :value="stats.total" />
          </n-grid-item>
          <n-grid-item>
            <n-statistic label="正常" :value="stats.active">
              <template #prefix>
                <n-icon color="#18a058"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-grid-item>
          <n-grid-item>
            <n-statistic label="限流中" :value="stats.rateLimited">
              <template #prefix>
                <n-icon color="#f0a020"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-grid-item>
          <n-grid-item>
            <n-statistic label="熔断" :value="stats.circuitOpen">
              <template #prefix>
                <n-icon color="#d03050"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-grid-item>
        </n-grid>
        
        <!-- API Key 列表 -->
        <n-data-table :columns="columns" :data="apiKeys" :bordered="false" />
      </n-card>
    </n-spin>
    
    <!-- 添加 Key 弹窗 -->
    <n-modal v-model:show="showAddModal" preset="dialog" title="添加 API Key">
      <n-form :model="formData" label-placement="left" label-width="100">
        <n-form-item label="Provider" path="provider">
          <n-select v-model:value="formData.provider" :options="providerOptions" />
        </n-form-item>
        <n-form-item label="API Key" path="key">
          <n-input v-model:value="formData.key" placeholder="sk-..." />
        </n-form-item>
        <n-form-item label="备注" path="remark">
          <n-input v-model:value="formData.remark" placeholder="可选" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-button @click="showAddModal = false">取消</n-button>
        <n-button type="primary" :loading="addLoading" @click="handleAdd">添加</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NTag, NButton, NIcon, NSpace, useMessage } from 'naive-ui'
import { listApiKeys, addApiKey, updateApiKey, deleteApiKey } from '@/api/admin'

const message = useMessage()
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
  { title: 'ID', key: 'id', width: 120 },
  { title: 'Provider', key: 'provider', width: 120 },
  { title: 'Key', key: 'key_preview', width: 180 },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (row: any) => {
      const statusMap: Record<string, { label: string; type: string }> = {
        active: { label: '正常', type: 'success' },
        rate_limited: { label: '限流中', type: 'warning' },
        circuit_open: { label: '熔断', type: 'error' },
      }
      const status = statusMap[row.status] || { label: row.status, type: 'default' }
      return h(NTag, { type: status.type as any, size: 'small' }, { default: () => status.label })
    },
  },
  { title: '权重', key: 'weight', width: 80 },
  { title: '失败次数', key: 'failures', width: 100 },
  { title: '最后使用', key: 'last_used', width: 180 },
  { title: '备注', key: 'remark' },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row: any) => h(NSpace, null, {
      default: () => [
        h(NButton, { size: 'small', type: 'primary', quaternary: true, onClick: () => handleEdit(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', type: 'error', quaternary: true, onClick: () => handleDelete(row) }, { default: () => '删除' }),
      ]
    }),
  },
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
  // 简单编辑：切换状态
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

<style scoped>
.api-keys {
  padding: 0;
}
</style>
