<template>
  <div class="dashboard">
    <n-spin :show="loading">
      <!-- 快捷导航 -->
      <n-grid :cols="5" :x-gap="12" :y-gap="12" style="margin-bottom: 16px">
        <n-grid-item v-for="nav in navItems" :key="nav.path">
          <n-card hoverable style="cursor: pointer; text-align: center" @click="$router.push(nav.path)">
            <div style="font-size: 24px; margin-bottom: 8px;">{{ nav.emoji }}</div>
            <div style="font-weight: bold">{{ nav.label }}</div>
          </n-card>
        </n-grid-item>
      </n-grid>

      <!-- 统计卡片 -->
      <n-grid :cols="4" :x-gap="16" :y-gap="16">
        <n-grid-item>
          <n-card>
            <n-statistic label="并发连接数" :value="stats.connections">
              <template #suffix>
                <n-tag :type="stats.connectionsTrend > 0 ? 'success' : 'error'" size="small">
                  {{ stats.connectionsTrend > 0 ? '+' : '' }}{{ stats.connectionsTrend }}%
                </n-tag>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>
        
        <n-grid-item>
          <n-card>
            <n-statistic label="队列积压" :value="stats.queueBacklog">
              <template #suffix>
                <n-tag :type="stats.queueBacklog > 1000 ? 'error' : 'success'" size="small">
                  {{ stats.queueBacklog > 1000 ? '警告' : '正常' }}
                </n-tag>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>
        
        <n-grid-item>
          <n-card>
            <n-statistic label="缓存命中率" :value="stats.cacheHitRate" suffix="%" />
          </n-card>
        </n-grid-item>
        
        <n-grid-item>
          <n-card>
            <n-statistic label="API 延迟 P99" :value="stats.latencyP99" suffix="ms" />
          </n-card>
        </n-grid-item>
      </n-grid>
      
      <!-- 图表区域 -->
      <n-grid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px">
        <n-grid-item>
          <n-card title="并发连接趋势">
            <v-chart :option="connectionChartOption" style="height: 300px" autoresize />
          </n-card>
        </n-grid-item>
        
        <n-grid-item>
          <n-card title="API Key 状态">
            <v-chart :option="apiKeyChartOption" style="height: 300px" autoresize />
          </n-card>
        </n-grid-item>
      </n-grid>
      
      <!-- 告警列表 -->
      <n-card title="最近告警" style="margin-top: 16px">
        <n-data-table :columns="alertColumns" :data="alerts" :bordered="false" />
      </n-card>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { 
  NGrid, NGridItem, NCard, NStatistic, NTag, NDataTable, NSpin 
} from 'naive-ui'
import { getMetrics, listApiKeys, getQueueStats } from '@/api/admin'

use([CanvasRenderer, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent])

const router = useRouter()
const loading = ref(false)

// 快捷导航
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
  xAxis: { 
    type: 'category', 
    data: connectionHistory.value.map(h => h.time)
  },
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
  { title: '时间', key: 'time', width: 180 },
  { title: '级别', key: 'level', width: 100 },
  { title: '消息', key: 'message' },
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
        active: keys.filter(k => k.status === 'active').length,
        rate_limited: keys.filter(k => k.status === 'rate_limited').length,
        circuit_open: keys.filter(k => k.status === 'circuit_open').length,
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

<style scoped>
.dashboard {
  padding: 0;
}
</style>
