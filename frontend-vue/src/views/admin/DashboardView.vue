<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Card, Row, Col, Statistic, Tag, Table, Spin } from 'ant-design-vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getMetrics, listApiKeys, getQueueStats } from '@/api/admin'

use([CanvasRenderer, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent])

const router = useRouter()
const loading = ref(false)

const navItems = [
  { label: 'API Keys', path: '/admin/api-keys', emoji: '🔑' },
  { label: '队列监控', path: '/admin/queue', emoji: '📊' },
  { label: '缓存监控', path: '/admin/cache', emoji: '💾' },
  { label: '性能监控', path: '/admin/performance', emoji: '⚡' },
  { label: '系统设置', path: '/admin/settings', emoji: '⚙️' },
]

const stats = ref({
  connections: 0,
  connectionsTrend: 0,
  queueBacklog: 0,
  cacheHitRate: 0,
  latencyP99: 0,
})

const connectionHistory = ref<{ time: string; value: number }[]>([
  { time: '00:00', value: 0 },
  { time: '04:00', value: 0 },
  { time: '08:00', value: 0 },
  { time: '12:00', value: 0 },
  { time: '16:00', value: 0 },
  { time: '20:00', value: 0 },
  { time: '24:00', value: 0 },
])

const connectionChartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: { type: 'category', data: connectionHistory.value.map(h => h.time) },
  yAxis: { type: 'value', name: '连接数' },
  series: [{
    name: '并发连接',
    type: 'line',
    data: connectionHistory.value.map(h => h.value),
    smooth: true,
    areaStyle: { opacity: 0.3 },
  }],
}))

const apiKeyStatus = ref({ active: 0, rate_limited: 0, circuit_open: 0 })

const apiKeyChartOption = computed(() => ({
  tooltip: { trigger: 'item' },
  series: [{
    name: 'API Key 状态',
    type: 'pie',
    radius: ['40%', '70%'],
    data: [
      { value: apiKeyStatus.value.active || 1, name: '正常', itemStyle: { color: '#18a058' } },
      { value: apiKeyStatus.value.rate_limited || 0, name: '限流中', itemStyle: { color: '#f0a020' } },
      { value: apiKeyStatus.value.circuit_open || 0, name: '熔断', itemStyle: { color: '#d03050' } },
    ],
  }],
}))

const alertColumns = [
  { title: '时间', dataIndex: 'time', width: 180 },
  { title: '级别', dataIndex: 'level', width: 100 },
  { title: '消息', dataIndex: 'message' },
]

const alerts = ref<{ time: string; level: string; message: string }[]>([])

async function fetchDashboardData() {
  loading.value = true
  try {
    const [metricsRes, keysRes, queueRes] = await Promise.allSettled([
      getMetrics(),
      listApiKeys(),
      getQueueStats(),
    ])

    if (metricsRes.status === 'fulfilled') {
      const m = metricsRes.value
      stats.value.connections = m.concurrent_connections || 0
      stats.value.queueBacklog = m.queue_backlog || 0
      stats.value.cacheHitRate = m.cache_hit_rate || 0
      stats.value.latencyP99 = m.api_latency_p99 || 0
    }

    if (keysRes.status === 'fulfilled') {
      const keys = keysRes.value
      apiKeyStatus.value = {
        active: keys.filter((k: any) => k.status === 'active').length,
        rate_limited: keys.filter((k: any) => k.status === 'rate_limited').length,
        circuit_open: keys.filter((k: any) => k.status === 'circuit_open').length,
      }
    }

    if (queueRes.status === 'fulfilled') {
      stats.value.queueBacklog = queueRes.value.task_queue_length || 0
    }
  } catch (err: any) {
    console.error('Dashboard fetch error:', err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchDashboardData()
})
</script>

<template>
  <div class="dashboard">
    <Spin :spinning="loading">
      <!-- 快捷导航 -->
      <Row :gutter="[12, 12]" style="margin-bottom: 16px">
        <Col v-for="nav in navItems" :key="nav.path" :span="Math.floor(24 / navItems.length)">
          <Card hoverable style="cursor: pointer; text-align: center" @click="router.push(nav.path)">
            <div style="font-size: 24px; margin-bottom: 8px;">{{ nav.emoji }}</div>
            <div style="font-weight: bold">{{ nav.label }}</div>
          </Card>
        </Col>
      </Row>

      <!-- 统计卡片 -->
      <Row :gutter="16">
        <Col :span="6">
          <Card>
            <Statistic title="并发连接数" :value="stats.connections">
              <template #suffix>
                <Tag :color="stats.connectionsTrend > 0 ? 'success' : 'error'">
                  {{ stats.connectionsTrend > 0 ? '+' : '' }}{{ stats.connectionsTrend }}%
                </Tag>
              </template>
            </Statistic>
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="队列积压" :value="stats.queueBacklog">
              <template #suffix>
                <Tag :color="stats.queueBacklog > 1000 ? 'error' : 'success'">
                  {{ stats.queueBacklog > 1000 ? '警告' : '正常' }}
                </Tag>
              </template>
            </Statistic>
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="缓存命中率" :value="stats.cacheHitRate" suffix="%" />
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="API 延迟 P99" :value="stats.latencyP99" suffix="ms" />
          </Card>
        </Col>
      </Row>

      <!-- 图表区域 -->
      <Row :gutter="16" style="margin-top: 16px">
        <Col :span="12">
          <Card title="并发连接趋势">
            <VChart :option="connectionChartOption" style="height: 300px" autoresize />
          </Card>
        </Col>
        <Col :span="12">
          <Card title="API Key 状态">
            <VChart :option="apiKeyChartOption" style="height: 300px" autoresize />
          </Card>
        </Col>
      </Row>

      <!-- 告警列表 -->
      <Card title="最近告警" style="margin-top: 16px">
        <Table :columns="alertColumns" :dataSource="alerts" :pagination="false" />
      </Card>
    </Spin>
  </div>
</template>

<style scoped>
.dashboard { padding: 0; }
</style>
