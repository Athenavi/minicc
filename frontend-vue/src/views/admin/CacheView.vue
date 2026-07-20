<template>
  <div class="cache-monitor">
    <n-spin :show="loading">
      <n-grid :cols="3" :x-gap="16" :y-gap="16">
        <!-- 缓存命中率 -->
        <n-grid-item>
          <n-card title="缓存命中率">
            <n-progress type="dashboard" :percentage="cacheStats.hitRate" :color="progressColor">
              <template #default>
                <span style="font-size: 24px; font-weight: 600">{{ cacheStats.hitRate }}%</span>
              </template>
            </n-progress>
            <n-descriptions bordered :column="1" style="margin-top: 16px">
              <n-descriptions-item label="L1 命中">{{ cacheStats.l1Hit }}%</n-descriptions-item>
              <n-descriptions-item label="L2 命中">{{ cacheStats.l2Hit }}%</n-descriptions-item>
              <n-descriptions-item label="L3 命中">{{ cacheStats.l3Hit }}%</n-descriptions-item>
            </n-descriptions>
          </n-card>
        </n-grid-item>

        <!-- 缓存统计 -->
        <n-grid-item>
          <n-card title="缓存统计">
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="总请求数">{{ cacheStats.totalRequests }}</n-descriptions-item>
              <n-descriptions-item label="缓存命中">{{ cacheStats.hits }}</n-descriptions-item>
              <n-descriptions-item label="缓存未命中">{{ cacheStats.misses }}</n-descriptions-item>
              <n-descriptions-item label="预取成功">{{ cacheStats.prefetchHits }}</n-descriptions-item>
              <n-descriptions-item label="平均延迟">{{ cacheStats.avgLatency }}ms</n-descriptions-item>
            </n-descriptions>
          </n-card>
        </n-grid-item>

        <!-- 缓存大小 -->
        <n-grid-item>
          <n-card title="缓存大小">
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="L1 容量">{{ cacheStats.l1Size }} / {{ cacheStats.l1Capacity }}</n-descriptions-item>
              <n-descriptions-item label="L2 容量">{{ cacheStats.l2Size }}</n-descriptions-item>
              <n-descriptions-item label="L3 容量">{{ cacheStats.l3Size }}</n-descriptions-item>
              <n-descriptions-item label="总内存">{{ cacheStats.totalMemory }}</n-descriptions-item>
            </n-descriptions>
          </n-card>
        </n-grid-item>
      </n-grid>

      <!-- 缓存命中率趋势 -->
      <n-card title="缓存命中率趋势" style="margin-top: 16px">
        <v-chart :option="hitRateChartOption" style="height: 300px" autoresize />
      </n-card>

      <!-- 热门查询 -->
      <n-card title="热门查询" style="margin-top: 16px">
        <n-data-table :columns="columns" :data="hotQueries" :bordered="false" />
      </n-card>
    </n-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getCacheStats } from '@/api/admin'
import { useMessage } from 'naive-ui'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent])

const message = useMessage()
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
    {
      name: 'L1',
      type: 'line',
      data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l1) : [0],
      smooth: true,
    },
    {
      name: 'L2',
      type: 'line',
      data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l2) : [0],
      smooth: true,
    },
    {
      name: 'L3',
      type: 'line',
      data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.l3) : [0],
      smooth: true,
    },
    {
      name: '总命中率',
      type: 'line',
      data: hitRateHistory.value.length ? hitRateHistory.value.map(h => h.total) : [0],
      smooth: true,
      lineStyle: { width: 3 },
    },
  ],
}))

const hotQueries = ref<any[]>([])

const columns = [
  { title: '查询', key: 'query', ellipsis: { tooltip: true } },
  { title: '命中次数', key: 'hits', width: 120 },
  { title: '命中率', key: 'hit_rate', width: 120, render: (row: any) => `${row.hit_rate}%` },
  { title: '平均延迟', key: 'avg_latency_ms', width: 120, render: (row: any) => `${row.avg_latency_ms}ms` },
]

async function fetchData() {
  loading.value = true
  try {
    const data = await getCacheStats()

    // 计算总内存
    const totalMemoryMB = ((data.l1_size || 0) + parseFloat(String(data.l2_size || 0)) + parseFloat(String(data.l3_size || 0)))
    const formatSize = (mb: number) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(1)} MB`

    cacheStats.value = {
      hitRate: data.total_hit_rate || 0,
      l1Hit: data.l1_hit_rate || 0,
      l2Hit: data.l2_hit_rate || 0,
      l3Hit: data.l3_hit_rate || 0,
      totalRequests: data.total_requests || 0,
      hits: data.total_hits || 0,
      misses: data.total_misses || 0,
      prefetchHits: 0,
      avgLatency: data.avg_latency_ms || 0,
      l1Size: data.l1_size || 0,
      l1Capacity: data.l1_capacity || 0,
      l2Size: formatSize(data.l2_size || 0),
      l3Size: formatSize(data.l3_size || 0),
      totalMemory: formatSize(totalMemoryMB),
    }

    hotQueries.value = data.hot_queries || []

    // 维护历史数据（最近 20 个点）
    hitRateHistory.value.push({
      l1: data.l1_hit_rate || 0,
      l2: data.l2_hit_rate || 0,
      l3: data.l3_hit_rate || 0,
      total: data.total_hit_rate || 0,
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

<style scoped>
.cache-monitor {
  padding: 0;
}
</style>
