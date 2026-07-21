<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Card, Row, Col, Progress, Descriptions, DescriptionsItem, Table, Spin, message } from 'ant-design-vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getCacheStats } from '@/api/admin'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent])

const loading = ref(false)

const cacheStats = ref({
  hitRate: 0,
  l1Hit: 0,
  l2Hit: 0,
  l3Hit: 0,
  totalRequests: 0,
  hits: 0,
  misses: 0,
  prefetchHits: 0,
  avgLatency: 0,
  l1Size: 0,
  l1Capacity: 0,
  l2Size: '0 MB',
  l3Size: '0 MB',
  totalMemory: '0 MB',
})

const progressColor = computed(() => {
  const rate = cacheStats.value.hitRate
  if (rate >= 80) return '#18a058'
  if (rate >= 60) return '#f0a020'
  return '#d03050'
})

const hitRateHistory = ref<{ l1: number; l2: number; l3: number; total: number }[]>([])

const hitRateChartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: hitRateHistory.value.length
      ? hitRateHistory.value.map((_, i) => `T-${hitRateHistory.value.length - i}`)
      : ['--'],
  },
  yAxis: { type: 'value', name: '命中率 (%)', max: 100 },
  series: [
    { name: 'L1', type: 'line', data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l1) : [0], smooth: true },
    { name: 'L2', type: 'line', data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l2) : [0], smooth: true },
    { name: 'L3', type: 'line', data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l3) : [0], smooth: true },
    { name: '总命中率', type: 'line', data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.total) : [0], smooth: true, lineStyle: { width: 3 } },
  ],
}))

const hotQueries = ref<any[]>([])

const columns = [
  { title: '查询', dataIndex: 'query', ellipsis: true },
  { title: '命中次数', dataIndex: 'hits', width: 120 },
  { title: '命中率', dataIndex: 'hit_rate', width: 120, customRender: ({ text }: { text: number }) => `${text}%` },
  { title: '平均延迟', dataIndex: 'avg_latency_ms', width: 120, customRender: ({ text }: { text: number }) => `${text}ms` },
]

async function fetchData() {
  loading.value = true
  try {
    const d: any = await getCacheStats()

    const totalMemoryMB = ((d.l1_size || 0) + parseFloat(String(d.l2_size || 0)) + parseFloat(String(d.l3_size || 0)))
    const formatSize = (mb: number) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(1)} MB`

    cacheStats.value = {
      hitRate: d.total_hit_rate || 0,
      l1Hit: d.l1_hit_rate || 0,
      l2Hit: d.l2_hit_rate || 0,
      l3Hit: d.l3_hit_rate || 0,
      totalRequests: d.total_requests || 0,
      hits: d.total_hits || 0,
      misses: d.total_misses || 0,
      prefetchHits: 0,
      avgLatency: d.avg_latency_ms || 0,
      l1Size: d.l1_size || 0,
      l1Capacity: d.l1_capacity || 0,
      l2Size: formatSize(d.l2_size || 0),
      l3Size: formatSize(d.l3_size || 0),
      totalMemory: formatSize(totalMemoryMB),
    }

    hotQueries.value = d.hot_queries || []

    hitRateHistory.value.push({
      l1: d.l1_hit_rate || 0,
      l2: d.l2_hit_rate || 0,
      l3: d.l3_hit_rate || 0,
      total: d.total_hit_rate || 0,
    })
    if (hitRateHistory.value.length > 20) {
      hitRateHistory.value.shift()
    }
  } catch (err: any) {
    console.error('Cache fetch error:', err)
    message.error('获取缓存数据失败: ' + (err.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
})
</script>

<template>
  <div class="cache-monitor">
    <Spin :spinning="loading">
      <Row :gutter="16">
        <Col :span="8">
          <Card title="缓存命中率">
            <Progress type="dashboard" :percent="cacheStats.hitRate" :strokeColor="progressColor" size="small">
              <template #format><span style="font-size: 24px; font-weight: 600">{{ cacheStats.hitRate }}%</span></template>
            </Progress>
            <Descriptions bordered :column="1" style="margin-top: 16px">
              <DescriptionsItem label="L1 命中">{{ cacheStats.l1Hit }}%</DescriptionsItem>
              <DescriptionsItem label="L2 命中">{{ cacheStats.l2Hit }}%</DescriptionsItem>
              <DescriptionsItem label="L3 命中">{{ cacheStats.l3Hit }}%</DescriptionsItem>
            </Descriptions>
          </Card>
        </Col>
        <Col :span="8">
          <Card title="缓存统计">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="总请求数">{{ cacheStats.totalRequests }}</DescriptionsItem>
              <DescriptionsItem label="缓存命中">{{ cacheStats.hits }}</DescriptionsItem>
              <DescriptionsItem label="缓存未命中">{{ cacheStats.misses }}</DescriptionsItem>
              <DescriptionsItem label="平均延迟">{{ cacheStats.avgLatency }}ms</DescriptionsItem>
            </Descriptions>
          </Card>
        </Col>
        <Col :span="8">
          <Card title="缓存大小">
            <Descriptions bordered :column="1">
              <DescriptionsItem label="L1 容量">{{ cacheStats.l1Size }} / {{ cacheStats.l1Capacity }}</DescriptionsItem>
              <DescriptionsItem label="L2 容量">{{ cacheStats.l2Size }}</DescriptionsItem>
              <DescriptionsItem label="L3 容量">{{ cacheStats.l3Size }}</DescriptionsItem>
              <DescriptionsItem label="总内存">{{ cacheStats.totalMemory }}</DescriptionsItem>
            </Descriptions>
          </Card>
        </Col>
      </Row>

      <Card title="缓存命中率趋势" style="margin-top: 16px">
        <VChart :option="hitRateChartOption" style="height: 300px" autoresize />
      </Card>

      <Card title="热门查询" style="margin-top: 16px">
        <Table :columns="columns" :dataSource="hotQueries" :pagination="false" />
      </Card>
    </Spin>
  </div>
</template>

<style scoped>
.cache-monitor { padding: 0; }
</style>
