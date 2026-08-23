<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Card, Row, Col, Table, Button, Space, Tag, Input, Modal, Alert,
  Statistic, Descriptions, Spin, message,
} from 'ant-design-vue'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons-vue'
import { api } from '../../api'
import { apiErrorMessage } from '../../composables/useCrudResource'
import EmptyState from '../../components/common/EmptyState.vue'

// ── 实例状态 ──
// GET /v1/admin/redis → { status, mode, pool: { hits, misses, timeouts, total_conns, idle_conns, stale_conns } }
const redisData = ref<any>(null)
const loading = ref(false)

async function loadRedis() {
  loading.value = true
  try {
    const resp = await api.get('/v1/admin/redis')
    redisData.value = resp.data?.data || null
  } catch (e: any) {
    message.error(apiErrorMessage(e, '获取 Redis 状态失败'))
  } finally {
    loading.value = false
  }
}

const pool = computed<any>(() => redisData.value?.pool || {})

const hitRate = computed(() => {
  const hits = Number(pool.value.hits) || 0
  const misses = Number(pool.value.misses) || 0
  const total = hits + misses
  return total > 0 ? Number(((hits / total) * 100).toFixed(2)) : 0
})

const poolStats = computed(() => [
  { title: '命中次数', value: Number(pool.value.hits) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '未命中次数', value: Number(pool.value.misses) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '超时次数', value: Number(pool.value.timeouts) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '总连接数', value: Number(pool.value.total_conns) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '空闲连接数', value: Number(pool.value.idle_conns) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '陈旧连接数', value: Number(pool.value.stale_conns) || 0, precision: 0, suffix: undefined as string | undefined },
  { title: '命中率', value: hitRate.value, precision: 2, suffix: '%' },
])

// ── 慢日志 ──
// GET /v1/admin/redis/slow-log → { slow_log: [...], error? }
const slowLog = ref<any[]>([])
const slowError = ref<string | null>(null)
const slowLoading = ref(false)

async function loadSlowLog() {
  slowLoading.value = true
  try {
    const resp = await api.get('/v1/admin/redis/slow-log')
    const d = resp.data?.data || {}
    slowLog.value = Array.isArray(d.slow_log) ? d.slow_log : []
    slowError.value = d.error || null
  } catch (e: any) {
    message.error(apiErrorMessage(e, '获取慢日志失败'))
  } finally {
    slowLoading.value = false
  }
}

const SLOW_LABELS: Record<string, string> = {
  id: '序号',
  timestamp: '时间',
  time: '时间',
  duration: '耗时 (μs)',
  duration_us: '耗时 (μs)',
  command: '命令',
  args: '参数',
  key: '键',
  client: '客户端',
  ip: '来源 IP',
  name: '名称',
}

const slowColumns = computed(() => {
  const first = slowLog.value[0]
  if (!first) return []
  return [
    { title: '#', key: '__idx', width: 64 },
    ...Object.keys(first).map((k) => ({
      title: SLOW_LABELS[k] || k,
      dataIndex: k,
      key: k,
      ellipsis: true,
    })),
  ]
})

function slowCellText(v: any): string {
  if (v == null) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

// ── FLUSH ALL ──
// POST /v1/admin/redis/flush-all { confirm: true }
const flushVisible = ref(false)
const flushConfirm = ref('')
const flushing = ref(false)

async function flushAll() {
  if (flushConfirm.value !== 'confirm') {
    message.warning('请输入 confirm 以确认执行')
    return
  }
  flushing.value = true
  try {
    await api.post('/v1/admin/redis/flush-all', { confirm: true })
    message.success('FLUSHALL 已执行')
    flushVisible.value = false
    flushConfirm.value = ''
    await loadRedis()
  } catch (e: any) {
    message.error(apiErrorMessage(e, 'FLUSHALL 执行失败'))
  } finally {
    flushing.value = false
  }
}

// ── 工具函数 ──
function statusColor(status: string): string {
  const s = String(status || '').toLowerCase()
  if (['up', 'ok', 'running', 'connected', 'active'].includes(s)) return 'green'
  if (['down', 'error', 'disconnected', 'inactive'].includes(s)) return 'red'
  return 'default'
}

function statusText(status: string): string {
  const s = String(status || '').toLowerCase()
  if (['up', 'ok', 'running', 'connected', 'active'].includes(s)) return '运行中'
  if (['down', 'error', 'disconnected', 'inactive'].includes(s)) return '异常'
  return status || '-'
}

onMounted(() => {
  loadRedis()
  loadSlowLog()
})
</script>

<template>
  <div class="redis-management">
    <div class="page-header">
      <h1>🔴 Redis 管理</h1>
      <Space>
        <Button @click="loadRedis">刷新状态</Button>
        <Button @click="loadSlowLog">
          <template #icon><ReloadOutlined /></template>
          刷新慢日志
        </Button>
        <Button danger type="primary" @click="flushVisible = true">
          <template #icon><ClearOutlined /></template>
          FLUSH ALL
        </Button>
      </Space>
    </div>

    <Spin :spinning="loading && !redisData">
      <!-- 状态卡 -->
      <Card title="实例状态" style="margin-bottom: 16px">
        <Descriptions v-if="redisData" :column="2" bordered size="small">
          <Descriptions.Item label="状态">
            <Tag :color="statusColor(redisData.status)">{{ statusText(redisData.status) }}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="运行模式">{{ redisData.mode || '-' }}</Descriptions.Item>
        </Descriptions>
        <EmptyState v-else description="暂无 Redis 实例信息" />
      </Card>

      <!-- 连接池统计 -->
      <Card title="连接池统计" style="margin-bottom: 16px">
        <Row :gutter="[16, 16]">
          <Col v-for="s in poolStats" :key="s.title" :xs="12" :sm="8" :md="6" :lg="4">
            <Card size="small" class="pool-stat">
              <Statistic :title="s.title" :value="s.value" :precision="s.precision" :suffix="s.suffix" />
            </Card>
          </Col>
        </Row>
      </Card>

      <!-- 慢日志 -->
      <Card title="慢查询日志">
        <Alert
          v-if="slowError"
          type="warning"
          show-icon
          :message="slowError"
          style="margin-bottom: 16px"
        />
        <Table
          :columns="slowColumns"
          :data-source="slowLog"
          :loading="slowLoading"
          :row-key="(_r: any, i?: number) => i ?? 0"
          :pagination="{ pageSize: 10, showSizeChanger: true }"
          :scroll="{ x: 720 }"
          size="small"
        >
          <template #emptyText>
            <EmptyState description="暂无慢查询记录" />
          </template>
          <template #bodyCell="{ column, record, index }">
            <template v-if="column.key === '__idx'">
              {{ index + 1 }}
            </template>
            <template v-else>
              <span class="cell">{{ slowCellText(record[(column as any).dataIndex as string]) }}</span>
            </template>
          </template>
        </Table>
      </Card>
    </Spin>

    <!-- FLUSH ALL 确认 -->
    <Modal
      v-model:open="flushVisible"
      title="⚠️ FLUSH ALL 确认"
      ok-text="执行"
      cancel-text="取消"
      :ok-button-props="{ danger: true }"
      :confirm-loading="flushing"
      @ok="flushAll"
      @cancel="flushConfirm = ''"
    >
      <Alert
        type="error"
        show-icon
        message="此操作将删除 Redis 中的全部数据，不可恢复！"
        style="margin-bottom: 16px"
      />
      <p>请在下方输入 <Tag>confirm</Tag> 以确认执行：</p>
      <Input
        v-model:value="flushConfirm"
        placeholder="输入 confirm"
        @press-enter="flushAll"
      />
    </Modal>
  </div>
</template>

<style scoped>
.redis-management { padding: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 24px; flex-wrap: wrap; }
.page-header h1 { margin: 0; font-size: 24px; font-weight: 600; color: var(--text-primary); }
.pool-stat { text-align: center; }
.cell { word-break: break-all; }

@media (max-width: 768px) {
  .redis-management { padding: 16px 12px; }
  .redis-management .page-header { row-gap: 12px; }
  .redis-management .page-header .ant-btn { min-height: 40px; }
}
@media (max-width: 480px) {
  .redis-management .page-header h1 { font-size: 20px; }
}
</style>
