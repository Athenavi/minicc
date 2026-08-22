<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Spin, Tabs, TabPane, message } from 'ant-design-vue'
import { CloudServerOutlined, BarChartOutlined } from '@ant-design/icons-vue'
import { api } from '../api'

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
      <CloudServerOutlined class="system-header-icon" />
      <h1>系统监控</h1>
    </div>

    <Spin :spinning="loading">
      <Tabs v-model:activeKey="activeTab">
        <TabPane key="health" tab="健康状态">
          <Card class="system-card">
            <template #title>
              <CloudServerOutlined /> 服务状态
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
            <p v-else class="empty-hint">暂无健康数据</p>
          </Card>
        </TabPane>

        <TabPane key="metrics" tab="性能指标">
          <Card class="system-card">
            <template #title>
              <BarChartOutlined /> 系统指标
            </template>
            <div v-if="metrics" class="metrics-grid">
              <div v-for="(value, metric) in metrics" :key="metric" class="metric-item">
                <span class="metric-name">{{ metric }}</span>
                <span class="metric-value">{{ value }}</span>
              </div>
            </div>
            <p v-else class="empty-hint">暂无指标数据</p>
          </Card>
        </TabPane>
      </Tabs>
    </Spin>
  </div>
</template>

<style scoped>
.system-container { padding: 24px; max-width: 1080px; margin: 0 auto; }
.system-header { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.system-header h1 { margin: 0; font-size: 24px; font-weight: 600; color: var(--text-primary); }
.system-header-icon { font-size: 24px; color: var(--colorSuccess, #22c55e); }
.system-card { margin-top: 16px; }
.health-grid, .metrics-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; }
.health-item, .metric-item { display: flex; justify-content: space-between; align-items: center; padding: 12px; background: var(--bg-secondary, rgba(0,0,0,0.02)); border-radius: 8px; }
.service-name, .metric-name { font-weight: 500; color: var(--text-primary); }
.metric-value { font-weight: 600; color: var(--primary, #2080f0); }
.empty-hint { color: var(--text-tertiary); margin: 0; }

/* 移动端 */
@media (max-width: 640px) {
  .system-container { padding: 16px 12px; }
  .system-header h1 { font-size: 20px; }
  .health-grid, .metrics-grid { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .system-container { transition: none; }
}
</style>
