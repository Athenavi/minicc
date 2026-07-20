<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NSpin, NIcon, NTabs, NTabPane, useMessage } from 'naive-ui'
import { ServerOutline, BarChartOutline } from '@vicons/ionicons5'
import { api } from '../api'

const message = useMessage()
const loading = ref(true)
const activeTab = ref('health')
const health = ref<any>(null)
const metrics = ref<any>(null)

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  return `${h}h ${m}m ${s}s`
}

onMounted(async () => {
  await loadSystemData()
})

async function loadSystemData() {
  try {
    loading.value = true
    const [healthRes, metricsRes] = await Promise.all([
      api.get('/v1/system/health').catch(() => ({ data: { data: {} } })),
      api.get('/v1/metrics').catch(() => ({ data: { data: {} } })),
    ])
    health.value = healthRes.data?.data || {}
    metrics.value = metricsRes.data?.data || {}
  } catch (error: any) {
    message.error(error.message || '加载失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="system-container">
    <div class="system-header">
      <NIcon size="24" color="#22c55e">
        <ServerOutline />
      </NIcon>
      <h1>系统监控</h1>
    </div>

    <NSpin :show="loading">
      <NTabs v-model:value="activeTab" type="line">
        <NTabPane name="health" tab="健康状态">
          <NCard style="margin-top: 16px">
            <template #header>
              <div class="card-header">
                <NIcon><ServerOutline /></NIcon>
                <span>服务状态</span>
              </div>
            </template>
            <div v-if="health && health.scores" class="health-grid">
              <div v-for="(item, index) in health.scores" :key="index" class="health-item">
                <span class="service-name">{{ item.label }}</span>
                <span class="metric-value">{{ item.score }}%</span>
              </div>
              <div v-if="health.uptime" class="health-item">
                <span class="service-name">运行时间</span>
                <span class="metric-value">{{ formatUptime(health.uptime) }}</span>
              </div>
            </div>
            <p v-else style="color: #6b7280">暂无健康数据</p>
          </NCard>
        </NTabPane>

        <NTabPane name="metrics" tab="性能指标">
          <NCard style="margin-top: 16px">
            <template #header>
              <div class="card-header">
                <NIcon><BarChartOutline /></NIcon>
                <span>系统指标</span>
              </div>
            </template>
            <div v-if="metrics" class="metrics-grid">
              <div v-for="(value, metric) in metrics" :key="metric" class="metric-item">
                <span class="metric-name">{{ metric }}</span>
                <span class="metric-value">{{ value }}</span>
              </div>
            </div>
            <p v-else style="color: #6b7280">暂无指标数据</p>
          </NCard>
        </NTabPane>
      </NTabs>
    </NSpin>
  </div>
</template>

<style scoped>
.system-container {
  padding: 24px;
}

.system-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.system-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.health-grid,
.metrics-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
}

.health-item,
.metric-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background-color: #f9fafb;
  border-radius: 8px;
}

.service-name,
.metric-name {
  font-weight: 500;
}

.status-badge {
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
}

.status-ok {
  background-color: #dcfce7;
  color: #16a34a;
}

.status-error {
  background-color: #fef2f2;
  color: #dc2626;
}

.metric-value {
  font-weight: 600;
  color: #2080f0;
}
</style>
