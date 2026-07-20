<template>
  <div class="performance-monitor">
    <n-spin :show="loading">
      <n-grid :cols="4" :x-gap="16" :y-gap="16">
        <!-- 关键指标 -->
        <n-grid-item>
          <n-card>
            <n-statistic label="并发连接数" :value="metrics.connections">
              <template #prefix>
                <n-icon color="#18a058"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>

        <n-grid-item>
          <n-card>
            <n-statistic label="API 延迟 P99" :value="metrics.latencyP99" suffix="ms">
              <template #prefix>
                <n-icon color="#f0a020"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 17H7v-7h2v7zm4 0h-2V7h2v10zm4 0h-2v-4h2v4z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>

        <n-grid-item>
          <n-card>
            <n-statistic label="错误率" :value="metrics.errorRate" suffix="%">
              <template #prefix>
                <n-icon color="#d03050"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>

        <n-grid-item>
          <n-card>
            <n-statistic label="QPS" :value="metrics.qps">
              <template #prefix>
                <n-icon color="#2080f0"><svg viewBox="0 0 24 24"><path fill="currentColor" d="M13 2.05v2.02c3.95.49 7 3.85 7 7.93 0 3.21-1.92 6-4.72 7.28L13 17v5h5l-1.22-1.22C19.91 18.81 22 15.67 22 12c0-5.18-3.95-9.45-9-9.95zM11 2.06C7.27 2.4 4.12 5.05 3.07 8.58L1.78 7.28C3.09 4.16 5.92 1.82 9.38 1.2L11 2.06z"/></svg></n-icon>
              </template>
            </n-statistic>
          </n-card>
        </n-grid-item>
      </n-grid>

      <!-- 延迟分布 -->
      <n-grid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px">
        <n-grid-item>
          <n-card title="延迟分布">
            <v-chart :option="latencyChartOption" style="height: 300px" autoresize />
          </n-card>
        </n-grid-item>

        <n-grid-item>
          <n-card title="QPS 趋势">
            <v-chart :option="qpsChartOption" style="height: 300px" autoresize />
          </n-card>
        </n-grid-item>
      </n-grid>

      <!-- Go 网关状态 -->
      <n-card title="Go 网关状态" style="margin-top: 16px">
        <n-grid :cols="4" :x-gap="16">
          <n-grid-item>
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="实例数">{{ gatewayStatus.instances }}</n-descriptions-item>
              <n-descriptions-item label="CPU 使用率">{{ gatewayStatus.cpuUsage }}%</n-descriptions-item>
              <n-descriptions-item label="内存使用">{{ gatewayStatus.memoryUsage }}</n-descriptions-item>
              <n-descriptions-item label="Goroutines">{{ gatewayStatus.goroutines }}</n-descriptions-item>
            </n-descriptions>
          </n-grid-item>

          <n-grid-item>
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="连接数">{{ gatewayStatus.connections }}</n-descriptions-item>
              <n-descriptions-item label="Redis 延迟">{{ gatewayStatus.redisLatency }}ms</n-descriptions-item>
              <n-descriptions-item label="DB 延迟">{{ gatewayStatus.dbLatency }}ms</n-descriptions-item>
              <n-descriptions-item label="运行时间">{{ gatewayStatus.uptime }}</n-descriptions-item>
            </n-descriptions>
          </n-grid-item>

          <n-grid-item>
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="版本">{{ gatewayStatus.version }}</n-descriptions-item>
            </n-descriptions>
          </n-grid-item>
        </n-grid>
      </n-card>

      <!-- Python 引擎状态 -->
      <n-card title="Python 引擎状态" style="margin-top: 16px">
        <n-grid :cols="4" :x-gap="16">
          <n-grid-item>
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="Pod 数量">{{ pythonStatus.pods }}</n-descriptions-item>
              <n-descriptions-item label="CPU 使用率">{{ pythonStatus.cpuUsage }}%</n-descriptions-item>
              <n-descriptions-item label="内存使用">{{ pythonStatus.memoryUsage }}</n-descriptions-item>
              <n-descriptions-item label="活跃任务">{{ pythonStatus.activeTasks }}</n-descriptions-item>
            </n-descriptions>
          </n-grid-item>

          <n-grid-item>
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="平均推理时间">{{ pythonStatus.avgInferenceTime }}ms</n-descriptions-item>
              <n-descriptions-item label="Redis 延迟">{{ pythonStatus.redisLatency }}ms</n-descriptions-item>
              <n-descriptions-item label="运行时间">{{ pythonStatus.uptime }}</n-descriptions-item>
              <n-descriptions-item label="版本">{{ pythonStatus.version }}</n-descriptions-item>
            </n-descriptions>
          </n-grid-item>
        </n-grid>
      </n-card>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getPerformance } from '@/api/admin'
import { useMessage } from 'naive-ui'

use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent])

const message = useMessage()
const loading = ref(false)

const metrics = ref({
  connections: 0,
  latencyP99: 0,
  errorRate: 0,
  qps: 0,
})

const latencyChartOption = computed(() => {
  const dist = latencyDistribution.value
  const labels = Object.keys(dist)
  const values = Object.values(dist)
  return {
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: labels.length ? labels : ['--'] },
    yAxis: { type: 'value', name: '请求数' },
    series: [{
      type: 'bar',
      data: values.length ? values : [0],
      itemStyle: {
        color: (params: any) => {
          const colors = ['#18a058', '#2080f0', '#f0a020', '#d03050', '#802020']
          return colors[params.dataIndex % colors.length]
        },
      },
    }],
  }
})

const qpsChartOption = computed(() => {
  const trend = qpsTrend.value
  return {
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: trend.length ? trend.map(t => t.time) : ['--'],
    },
    yAxis: { type: 'value', name: 'QPS' },
    series: [{
      type: 'line',
      data: trend.length ? trend.map(t => t.qps) : [0],
      smooth: true,
      areaStyle: { opacity: 0.3 },
    }],
  }
})

const latencyDistribution = ref<Record<string, number>>({})
const qpsTrend = ref<{ time: string; qps: number }[]>([])

const gatewayStatus = ref({
  instances: 0,
  cpuUsage: 0,
  memoryUsage: '0 MB',
  goroutines: 0,
  connections: 0,
  redisLatency: 0,
  dbLatency: 0,
  uptime: '--',
  version: '--',
})

const pythonStatus = ref({
  pods: 0,
  cpuUsage: 0,
  memoryUsage: '0 MB',
  activeTasks: 0,
  avgInferenceTime: 0,
  redisLatency: 0,
  uptime: '--',
  version: '--',
})

function formatMemory(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`
  return `${mb} MB`
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return `${days}d ${hours}h ${minutes}m`
}

async function fetchData() {
  loading.value = true
  try {
    const data = await getPerformance()

    // 更新关键指标（从 gateway 和 python_engine 聚合）
    const gw = data.gateway || {} as any
    const py = data.python_engine || {} as any

    metrics.value = {
      connections: gw.connections || 0,
      latencyP99: py.avg_inference_ms || 0,
      errorRate: 0,
      qps: data.qps_trend?.length ? data.qps_trend[data.qps_trend.length - 1].qps : 0,
    }

    // 延迟分布
    latencyDistribution.value = data.latency_distribution || {}

    // QPS 趋势
    qpsTrend.value = data.qps_trend || []

    // 网关状态
    gatewayStatus.value = {
      instances: gw.instances || 0,
      cpuUsage: gw.cpu_percent || 0,
      memoryUsage: formatMemory(gw.memory_mb || 0),
      goroutines: gw.goroutines || 0,
      connections: gw.connections || 0,
      redisLatency: gw.redis_latency_ms || 0,
      dbLatency: gw.db_latency_ms || 0,
      uptime: gw.uptime_seconds ? formatUptime(gw.uptime_seconds) : '--',
      version: gw.version || '--',
    }

    // Python 引擎状态
    pythonStatus.value = {
      pods: py.pods || 0,
      cpuUsage: py.cpu_percent || 0,
      memoryUsage: formatMemory(py.memory_mb || 0),
      activeTasks: py.active_tasks || 0,
      avgInferenceTime: py.avg_inference_ms || 0,
      redisLatency: py.redis_latency_ms || 0,
      uptime: py.uptime_seconds ? formatUptime(py.uptime_seconds) : '--',
      version: py.version || '--',
    }
  } catch (err: any) {
    console.error('Performance fetch error:', err)
    message.error('获取性能数据失败: ' + (err.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.performance-monitor {
  padding: 0;
}
</style>
