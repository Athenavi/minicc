<script setup lang="ts">
import { ref, h, onMounted } from 'vue'
import { Card, Table, Button, Space, Tag, Input, Modal, Form, InputNumber, Statistic, Tabs, TabPane, Alert, Progress } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined, DeleteOutlined, DatabaseOutlined, MonitorOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'
import { useCrudResource, apiErrorMessage } from '../../composables/useCrudResource'
// 后端接口开发中（前端先行）：跳过真实请求，展示降级横幅
const backendMissing = true

// State
const { data: redisConfigs, loading, load: loadRedisConfigs } = useCrudResource<any[]>(
  [],
  async () => (await api.get('/v1/admin/redis')).data || []
)
const selectedRedis = ref<any>(null)
const statusData = ref<any>({})
const poolData = ref<any>({})
const slowLogData = ref<any[]>([])
const modalVisible = ref(false)
const currentForm = ref({
  name: '',
  host: 'localhost',
  port: 6379,
  db_index: 0,
  pool_size: 10,
  min_idle_conns: 5,
  max_conn_age: '300s',
  password_hash: ''
})

// Table columns（customRender 用 h()，模板属性里不能写 JSX）
const columns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 150 },
  { title: '地址', dataIndex: 'host', key: 'host', width: 200,
    customRender: ({ record }: any) => `${record.host}:${record.port}` },
  { title: 'DB', dataIndex: 'db_index', key: 'db_index', width: 60 },
  { title: '状态', key: 'status', width: 100,
    customRender: ({ record }: any) =>
      h(Tag, { color: getStatusColor(record.status) }, () => record.status) },
  { title: '连接池', key: 'pool', width: 100,
    customRender: ({ record }: any) => `${record.pool_size}/${record.min_idle_connections}` },
  { title: '内存使用', dataIndex: 'memory_used_mb', key: 'memory_used_mb', width: 120,
    customRender: ({ record }: any) => `${record.memory_used_mb?.toFixed(2)} MB` },
  { title: '操作', key: 'actions', width: 250, fixed: 'right' as const,
    customRender: ({ record }: any) => h(Space, () => [
      h(Button, { size: 'small', icon: h(MonitorOutlined), onClick: () => getStatus(record.id) }, () => '监控'),
      h(Button, { size: 'small', icon: h(ReloadOutlined), onClick: () => getPoolStats(record.id) }, () => '连接池'),
      h(Button, { size: 'small', danger: true, icon: h(DeleteOutlined), onClick: () => flushCache(record.id) }, () => '清缓存')
    ]) }
]

const poolColumns = [
  { title: '指标', dataIndex: 'metric', key: 'metric' },
  { title: '值', dataIndex: 'value', key: 'value' },
  { title: '说明', dataIndex: 'description', key: 'description' }
]

const poolDataSource = ref<any[]>([])

function refreshPoolRows() {
  poolDataSource.value = [
    { metric: '池大小', value: poolData.value.pool_size || 0, description: '最大连接数' },
    { metric: '空闲连接', value: poolData.value.idle_conns || 0, description: '当前空闲连接数' },
    { metric: '等待请求', value: poolData.value.wait_count || 0, description: '等待获取连接的总次数' },
    { metric: '等待时长', value: poolData.value.wait_duration || '0ms', description: '总等待时间' },
    { metric: '最大空闲连接', value: poolData.value.max_idle_conns || 0, description: '配置的最大空闲连接数' }
  ]
}

const slowLogColumns = [
  { title: '序号', dataIndex: 'id', key: 'id', width: 80 },
  { title: '命令', dataIndex: 'command', key: 'command' },
  { title: '耗时 (μs)', dataIndex: 'duration_us', key: 'duration_us', width: 120 },
  { title: '时间', dataIndex: 'timestamp', key: 'timestamp', width: 200 }
]

// Get status
async function getStatus(id: string) {
  try {
    const response = await api.get(`/admin/redis/${id}/status`)
    statusData.value = response.data
    selectedRedis.value = redisConfigs.value.find(r => r.id === id)
  } catch (error: any) {
    alert(apiErrorMessage(error, '获取状态失败'))
  }
}

// Get pool stats
async function getPoolStats(id: string) {
  try {
    const response = await api.get(`/admin/redis/${id}/pool`)
    poolData.value = response.data
    refreshPoolRows()
    selectedRedis.value = redisConfigs.value.find(r => r.id === id)
  } catch (error: any) {
    console.error('Failed to get pool stats:', error)
  }
}

// Flush cache
async function flushCache(id: string) {
  if (!confirm('确定要清空该 Redis 实例的缓存吗?此操作不可恢复!')) return

  try {
    await api.delete(`/admin/redis/${id}/cache`)
    alert('缓存已清空')
    getStatus(id)
  } catch (error: any) {
    alert(apiErrorMessage(error, '清空缓存失败'))
  }
}

// Flush all
async function flushAll(id: string) {
  if (!confirm('警告: 确定要执行 FLUSHALL 吗?这将删除所有数据!')) return

  try {
    await api.post(`/admin/redis/${id}/flush-all`)
    alert('FLUSHALL 执行成功')
    getStatus(id)
  } catch (error: any) {
    alert(apiErrorMessage(error, '执行失败'))
  }
}

// Get slow log
async function getSlowLog(id: string) {
  try {
    const response = await api.get(`/admin/redis/${id}/slow-log`)
    slowLogData.value = response.data || []
  } catch (error: any) {
    console.error('Failed to get slow log:', error)
  }
}

// Helper functions
function getStatusColor(status: string): string {
  switch (status) {
    case 'active': return 'green'
    case 'inactive': return 'red'
    default: return 'default'
  }
}

function getHitRateColor(rate: number): string {
  if (rate >= 95) return 'green'
  if (rate >= 80) return 'orange'
  return 'red'
}

// Initialize
onMounted(() => {
    if (backendMissing) return
  loadRedisConfigs()
})
</script>

<template>
<a-alert v-if="backendMissing" type="warning" show-icon message="该管理能力后端接口开发中（前端界面先行），暂不可用" style="margin: 16px" />
  <div class="redis-management">
    <!-- Header -->
    <div class="page-header">
      <h1>🔴 Redis 管理</h1>
      <Button type="primary" @click="modalVisible = true">
        <PlusOutlined /> 添加 Redis 实例
      </Button>
    </div>

    <!-- Redis Instance List -->
    <Card>
      <Table
        :columns="columns"
        :data-source="redisConfigs"
        :loading="loading"
        row-key="id"
        :scroll="{ x: 1000 }"
      >
        <template #emptyText>
          <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
        </template>
      </Table>
    </Card>

    <!-- Status Detail Modal -->
    <Modal
      :open="selectedRedis !== null"
      :title="`📊 ${selectedRedis?.name ?? ''} - 实时监控`"
      :footer="null"
      :width="900"
      @cancel="selectedRedis = null"
    >
      <Tabs>
        <!-- Overview Tab -->
        <TabPane tab="概览" key="overview">
          <Space direction="vertical" style="width: 100%" size="large">
            <Card>
              <div class="metric-grid">
                <Statistic title="状态" :value="statusData.status" :value-style="{ color: getStatusColor(statusData.status) }" />
                <Statistic title="平均延迟" :value="statusData.avg_latency_ms" suffix="ms" />
                <Statistic title="连接客户端" :value="statusData.clients" />
                <Statistic title="命中率" :value="statusData.hit_rate" :precision="2" suffix="%">
                  <template #prefix>
                    <span :style="{ color: getHitRateColor(statusData.hit_rate), fontWeight: 'bold' }">●</span>
                  </template>
                </Statistic>
              </div>
            </Card>

            <Card title="内存使用情况">
              <Progress
                :percent="Math.round((statusData.memory_usage || 0) / 100 * 100)"
                :status="statusData.memory_usage > 80 ? 'exception' : 'normal'"
              />
              <p style="margin-top: 8px">
                已使用: {{ statusData.memory_usage?.toFixed(2) || 0 }} MB
              </p>
            </Card>
          </Space>
        </TabPane>

        <!-- Pool Stats Tab -->
        <TabPane tab="连接池统计" key="pool">
          <Card>
            <Table
              :columns="poolColumns"
              :data-source="poolDataSource"
              row-key="metric"
              :pagination="false"
            >
              <template #emptyText>
                <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
              </template>
            </Table>
          </Card>
        </TabPane>

        <!-- Slow Log Tab -->
        <TabPane tab="慢查询日志" key="slowlog">
          <Card>
            <Button type="primary" style="margin-bottom: 16px" @click="getSlowLog(selectedRedis.id)">
              <ReloadOutlined /> 刷新
            </Button>
            <Table
              :columns="slowLogColumns"
              :data-source="slowLogData"
              row-key="id"
              :scroll="{ x: 560 }"
              :pagination="{ pageSize: 10 }"
            >
              <template #emptyText>
                <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
              </template>
            </Table>
          </Card>
        </TabPane>
      </Tabs>

      <template #footer>
        <Space>
          <Button danger @click="flushCache(selectedRedis.id)">清空缓存</Button>
          <Button type="primary" danger @click="flushAll(selectedRedis.id)">FLUSHALL</Button>
        </Space>
      </template>
    </Modal>

    <!-- Add/Edit Modal -->
    <Modal
      v-model:open="modalVisible"
      title="添加 Redis 实例"
      ok-text="创建"
      cancel-text="取消"
      @ok="loadRedisConfigs"
    >
      <Form layout="vertical">
        <Form.Item label="实例名称" required>
          <Input v-model:value="currentForm.name" placeholder="例如: Production Redis" />
        </Form.Item>
        <Form.Item label="主机地址" required>
          <Input v-model:value="currentForm.host" placeholder="localhost" />
        </Form.Item>
        <Form.Item label="端口" required>
          <InputNumber v-model:value="currentForm.port" :min="1" :max="65535" style="width: 100%" />
        </Form.Item>
        <Form.Item label="数据库索引">
          <InputNumber v-model:value="currentForm.db_index" :min="0" :max="15" style="width: 100%" />
        </Form.Item>
        <Form.Item label="密码哈希">
          <Input v-model:value="currentForm.password_hash" placeholder="SHA-256 哈希值" />
        </Form.Item>
        <Form.Item label="连接池大小">
          <InputNumber v-model:value="currentForm.pool_size" :min="1" :max="100" style="width: 100%" />
        </Form.Item>
        <Form.Item label="最小空闲连接">
          <InputNumber v-model:value="currentForm.min_idle_conns" :min="0" :max="50" style="width: 100%" />
        </Form.Item>
        <Form.Item label="最大连接生命周期">
          <Input v-model:value="currentForm.max_conn_age" placeholder="300s" />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.redis-management {
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

/* 指标卡网格:auto-fill,窄屏自动降列 */
.metric-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
}

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

/* 窄屏:页头竖排、弹层底部按钮竖排、触控目标 ≥ 40px */
@media (max-width: 768px) {
  .redis-management .page-header {
    flex-wrap: wrap;
    row-gap: 12px;
  }
  .redis-management .page-header .ant-btn { min-height: 40px; }
}
@media (max-width: 576px) {
  .redis-management :deep(.ant-modal-footer) {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .redis-management :deep(.ant-modal-footer .ant-btn) {
    width: 100%;
    min-height: 40px;
    margin: 0;
  }
}
</style>
