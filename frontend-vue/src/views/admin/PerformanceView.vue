<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Card, Row, Col, Statistic, Descriptions, DescriptionsItem, Spin, message } from 'ant-design-vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getPerformance } from '@/api/admin'

use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent])

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
    xAxis: { type: 'category', data: trend.length ? trend.map(t => t.time) : ['--'] },
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
    const gw = data.gateway || {} as any
    const py = data.python_engine || {} as any

    metrics.value = {
      connections: gw.connections || 0,
      latencyP99: py.avg_inference_ms || 0,
      errorRate: 0,
      qps: data.qps_trend?.length ? data.qps_trend[data.qps_trend.length - 1].qps : 0,
    }

    latencyDistribution.value = data.latency_distribution || {}
    qpsTrend.value = data.qps_trend || []

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

<template>
  <div class="performance-monitor">
    <Spin :spinning="loading">
      <Row :gutter="16">
        <Col :span="6">
          <Card>
            <Statistic title="并发连接数" :value="metrics.connections" />
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="API 延迟 P99" :value="metrics.latencyP99" suffix="ms" />
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="错误率" :value="metrics.errorRate" suffix="%" />
          </Card>
        </Col>
        <Col :span="6">
          <Card>
            <Statistic title="QPS" :value="metrics.qps" />
          </Card>
        </Col>
      </Row>

      <Row :gutter="16" style="margin-top: 16px">
        <Col :span="12">
          <Card title="延迟分布">
            <VChart :option="latencyChartOption" style="height: 300px" autoresize />
          </Card>
        </Col>
        <Col :span="12">
          <Card title="QPS 趋势">
            <VChart :option="qpsChartOption" style="height: 300px" autoresize />
          </Card>
        </Col>
      </Row>

      <Card title="Go 网关状态" style="margin-top: 16px">
        <Row :gutter="16">
          <Col :span="8">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="实例数">{{ gatewayStatus.instances }}</DescriptionsItem>
              <DescriptionsItem label="CPU 使用率">{{ gatewayStatus.cpuUsage }}%</DescriptionsItem>
              <DescriptionsItem label="内存使用">{{ gatewayStatus.memoryUsage }}</DescriptionsItem>
              <DescriptionsItem label="Goroutines">{{ gatewayStatus.goroutines }}</DescriptionsItem>
            </Descriptions>
          </Col>
          <Col :span="8">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="连接数">{{ gatewayStatus.connections }}</DescriptionsItem>
              <DescriptionsItem label="Redis 延迟">{{ gatewayStatus.redisLatency }}ms</DescriptionsItem>
              <DescriptionsItem label="DB 延迟">{{ gatewayStatus.dbLatency }}ms</DescriptionsItem>
              <DescriptionsItem label="运行时间">{{ gatewayStatus.uptime }}</DescriptionsItem>
            </Descriptions>
          </Col>
          <Col :span="8">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="版本">{{ gatewayStatus.version }}</DescriptionsItem>
            </Descriptions>
          </Col>
        </Row>
      </Card>

      <Card title="Python 引擎状态" style="margin-top: 16px">
        <Row :gutter="16">
          <Col :span="8">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="Pod 数量">{{ pythonStatus.pods }}</DescriptionsItem>
              <DescriptionsItem label="CPU 使用率">{{ pythonStatus.cpuUsage }}%</DescriptionsItem>
              <DescriptionsItem label="内存使用">{{ pythonStatus.memoryUsage }}</DescriptionsItem>
              <DescriptionsItem label="活跃任务">{{ pythonStatus.activeTasks }}</DescriptionsItem>
            </Descriptions>
          </Col>
          <Col :span="8">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="平均推理时间">{{ pythonStatus.avgInferenceTime }}ms</DescriptionsItem>
              <DescriptionsItem label="Redis 延迟">{{ pythonStatus.redisLatency }}ms</DescriptionsItem>
              <DescriptionsItem label="运行时间">{{ pythonStatus.uptime }}</DescriptionsItem>
              <DescriptionsItem label="版本">{{ pythonStatus.version }}</DescriptionsItem>
            </Descriptions>
          </Col>
        </Row>
      </Card>
    </Spin>
  </div>
</template>

<style scoped>
.performance-monitor { padding: 0; }
</style>
