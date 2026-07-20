<template>
  <div class="queue-monitor">
    <n-spin :show="loading">
      <n-grid :cols="2" :x-gap="16" :y-gap="16">
        <!-- 队列状态 -->
        <n-grid-item>
          <n-card title="队列状态">
            <n-descriptions bordered :column="1">
              <n-descriptions-item label="任务队列长度">{{ queueStats.taskQueueLength }}</n-descriptions-item>
              <n-descriptions-item label="VIP 队列长度">{{ queueStats.vipQueueLength }}</n-descriptions-item>
              <n-descriptions-item label="消费者数量">{{ queueStats.consumers }}</n-descriptions-item>
              <n-descriptions-item label="吞吐量 (QPS)">{{ queueStats.throughput }}</n-descriptions-item>
              <n-descriptions-item label="平均等待时间">{{ queueStats.avgWaitTime }}ms</n-descriptions-item>
              <n-descriptions-item label="最大等待时间">{{ queueStats.maxWaitTime }}ms</n-descriptions-item>
            </n-descriptions>
          </n-card>
        </n-grid-item>

        <!-- 队列趋势 -->
        <n-grid-item>
          <n-card title="队列长度趋势">
            <v-chart :option="queueChartOption" style="height: 300px" autoresize />
          </n-card>
        </n-grid-item>
      </n-grid>

      <!-- 等待队列 -->
      <n-card title="等待队列" style="margin-top: 16px">
        <template #header-extra>
          <n-space>
            <n-button type="primary" quaternary @click="handleFlushQueue" :loading="flushLoading">清空队列</n-button>
            <n-button type="warning" quaternary @click="handlePauseQueue">
              {{ isPaused ? '恢复消费' : '暂停消费' }}
            </n-button>
          </n-space>
        </template>

        <n-data-table :columns="columns" :data="waitingTasks" :bordered="false" />
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
import { getQueueStats, flushQueue, pauseQueue } from '@/api/admin'
import { useMessage } from 'naive-ui'

use([CanvasRenderer, LineChart, GridComponent, TooltipComponent])

const message = useMessage()
const loading = ref(false)
const flushLoading = ref(false)
const isPaused = ref(false)

const queueStats = ref({
  taskQueueLength: 0,
  vipQueueLength: 0,
  consumers: 0,
  throughput: 0,
  avgWaitTime: 0,
  maxWaitTime: 0,
})

const queueHistory = ref<{ taskLength: number; vipLength: number }[]>([])

const queueChartOption = computed(() => ({
  tooltip: { trigger: 'axis' },
  xAxis: {
    type: 'category',
    data: queueHistory.value.length
      ? queueHistory.value.map((_, i) => `T-${queueHistory.value.length - i}`)
      : ['--'],
  },
  yAxis: { type: 'value', name: '队列长度' },
  series: [
    {
      name: '任务队列',
      type: 'line',
      data: queueHistory.value.length ? queueHistory.value.map(h => h.taskLength) : [0],
      smooth: true,
    },
    {
      name: 'VIP 队列',
      type: 'line',
      data: queueHistory.value.length ? queueHistory.value.map(h => h.vipLength) : [0],
      smooth: true,
    },
  ],
}))

const waitingTasks = ref<any[]>([])

const columns = [
  { title: '任务 ID', key: 'task_id', width: 120 },
  { title: '用户 ID', key: 'user_id', width: 120 },
  { title: '内容', key: 'content', ellipsis: { tooltip: true } },
  { title: '入队时间', key: 'queued_at', width: 120 },
  { title: '位置', key: 'position', width: 80 },
  {
    title: 'VIP',
    key: 'is_vip',
    width: 80,
    render: (row: any) => row.is_vip ? '是' : '否',
  },
]

async function fetchData() {
  loading.value = true
  try {
    const data = await getQueueStats()
    queueStats.value = {
      taskQueueLength: data.task_queue_length || 0,
      vipQueueLength: data.vip_queue_length || 0,
      consumers: data.consumers || 0,
      throughput: data.throughput_qps || 0,
      avgWaitTime: data.avg_wait_ms || 0,
      maxWaitTime: data.max_wait_ms || 0,
    }
    waitingTasks.value = data.waiting_tasks || []

    // 维护历史数据（最近 20 个点）
    queueHistory.value.push({
      taskLength: data.task_queue_length || 0,
      vipLength: data.vip_queue_length || 0,
    })
    if (queueHistory.value.length > 20) {
      queueHistory.value.shift()
    }
  } catch (err: any) {
    console.error('Queue fetch error:', err)
    message.error('获取队列数据失败: ' + (err.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

async function handleFlushQueue() {
  flushLoading.value = true
  try {
    await flushQueue()
    message.success('队列已清空')
    await fetchData()
  } catch (err: any) {
    console.error('Flush queue error:', err)
    message.error('清空队列失败: ' + (err.message || '未知错误'))
  } finally {
    flushLoading.value = false
  }
}

async function handlePauseQueue() {
  try {
    const pause = !isPaused.value
    await pauseQueue(pause)
    isPaused.value = pause
    message.success(pause ? '已暂停消费' : '已恢复消费')
  } catch (err: any) {
    console.error('Pause queue error:', err)
    message.error('操作失败: ' + (err.message || '未知错误'))
  }
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped>
.queue-monitor {
  padding: 0;
}
</style>
