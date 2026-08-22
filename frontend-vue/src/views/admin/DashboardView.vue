<script setup lang="ts">
import { ref, onMounted, computed, markRaw, h, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Card, Row, Col, Statistic, Tag, Table, Tooltip, Button, Input, Modal, Space, Popconfirm, Switch, message } from 'ant-design-vue'
import {
  KeyOutlined, OrderedListOutlined, DatabaseOutlined, ThunderboltOutlined, SettingOutlined,
  ArrowRightOutlined, ArrowUpOutlined, ArrowDownOutlined, ClockCircleOutlined,
  ApartmentOutlined, ThunderboltFilled, DatabaseFilled,
  PlusOutlined, ReloadOutlined, PlayCircleOutlined, CopyOutlined, EditOutlined, DeleteOutlined,
} from '@ant-design/icons-vue'
import { api, triggerCronJob, cronWebhookUrl } from '@/api'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { getMetrics, listApiKeys, getQueueStats } from '@/api/admin'
import { useThemeStore } from '@/stores/theme'
import PageSkeleton from '@/components/common/PageSkeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'

use([CanvasRenderer, LineChart, PieChart, GridComponent, TooltipComponent, LegendComponent])

const router = useRouter()
const themeStore = useThemeStore()
const loading = ref(true)
const error = ref('')

// 快捷导航：替换 emoji 为 Ant Design 图标，保持与应用主体图标体系一致
const navItems = [
  { label: 'API Key', path: '/admin/api-keys', icon: markRaw(KeyOutlined), desc: '密钥与限流' },
  { label: '队列监控', path: '/admin/queue', icon: markRaw(OrderedListOutlined), desc: '任务积压' },
  { label: '缓存监控', path: '/admin/cache', icon: markRaw(DatabaseOutlined), desc: '命中率' },
  { label: '性能监控', path: '/admin/performance', icon: markRaw(ThunderboltOutlined), desc: '延迟 P99' },
  { label: '系统设置', path: '/admin/settings', icon: markRaw(SettingOutlined), desc: '全局配置' },
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

// ECharts 主题色：跟随设计系统（亮/暗）
const chartColors = computed(() => {
  const isDark = themeStore.isDark
  return {
    primary: isDark ? '#5686fe' : '#4176e6',
    text: isDark ? '#cfd3d6' : '#61666b',
    textSecondary: isDark ? '#adb2b8' : '#81858c',
    splitLine: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)',
    success: '#22c55e',
    warning: '#f59e0b',
    error: '#ef4444',
    bg: isDark ? '#2c2c2e' : '#f1f3f5',
  }
})

const connectionChartOption = computed(() => ({
  tooltip: {
    trigger: 'axis',
    backgroundColor: chartColors.value.bg,
    borderColor: chartColors.value.splitLine,
    textStyle: { color: chartColors.value.text, fontSize: 12 },
  },
  grid: { left: 40, right: 20, top: 20, bottom: 28 },
  xAxis: {
    type: 'category',
    data: connectionHistory.value.map(h => h.time),
    axisLabel: { color: chartColors.value.textSecondary, fontSize: 11 },
    axisLine: { lineStyle: { color: chartColors.value.splitLine } },
    axisTick: { show: false },
  },
  yAxis: {
    type: 'value',
    name: '连接数',
    nameTextStyle: { color: chartColors.value.textSecondary, fontSize: 11 },
    axisLabel: { color: chartColors.value.textSecondary, fontSize: 11 },
    splitLine: { lineStyle: { color: chartColors.value.splitLine } },
    axisLine: { show: false },
    axisTick: { show: false },
  },
  series: [{
    name: '并发连接',
    type: 'line',
    data: connectionHistory.value.map(h => h.value),
    smooth: true,
    symbol: 'none',
    lineStyle: { color: chartColors.value.primary, width: 2 },
    areaStyle: {
      color: {
        type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
        colorStops: [
          { offset: 0, color: chartColors.value.primary + '4D' },
          { offset: 1, color: chartColors.value.primary + '0D' },
        ],
      },
    },
  }],
}))

const apiKeyStatus = ref({ active: 0, rate_limited: 0, circuit_open: 0 })

const apiKeyChartOption = computed(() => ({
  tooltip: {
    trigger: 'item',
    backgroundColor: chartColors.value.bg,
    borderColor: chartColors.value.splitLine,
    textStyle: { color: chartColors.value.text, fontSize: 12 },
  },
  legend: {
    bottom: 0,
    icon: 'circle',
    itemWidth: 8, itemHeight: 8,
    textStyle: { color: chartColors.value.textSecondary, fontSize: 12 },
  },
  series: [{
    name: 'API Key 状态',
    type: 'pie',
    radius: ['52%', '72%'],
    center: ['50%', '44%'],
    avoidLabelOverlap: true,
    itemStyle: { borderColor: chartColors.value.bg, borderWidth: 2 },
    label: { show: false },
    data: [
      { value: apiKeyStatus.value.active || 1, name: '正常', itemStyle: { color: chartColors.value.success } },
      { value: apiKeyStatus.value.rate_limited || 0, name: '限流中', itemStyle: { color: chartColors.value.warning } },
      { value: apiKeyStatus.value.circuit_open || 0, name: '熔断', itemStyle: { color: chartColors.value.error } },
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
  error.value = ''
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
    error.value = err?.message || '数据加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchDashboardData()
  fetchCronJobs()
  // 自动刷新：每 30s 静默刷新定时任务列表（last_status / last_run_at 会随执行更新）
  cronAutoTimer = setInterval(() => { fetchCronJobs(true) }, 30_000)
})

onUnmounted(() => {
  if (cronAutoTimer) { clearInterval(cronAutoTimer); cronAutoTimer = null }
})

// ── 定时任务（Cron Jobs）：列表 + 手动触发 + Webhook + 自动刷新 ──
interface CronJob {
  id: string
  name: string
  schedule: string
  task: string
  enabled: boolean
  last_run_at: string | null
  last_status: string
  webhook_token?: string
  created_at: string
}

const cronJobs = ref<CronJob[]>([])
const cronLoading = ref(false)
const cronRefreshing = ref(false)
let cronAutoTimer: ReturnType<typeof setInterval> | null = null

/** 拉取定时任务列表（silent：静默刷新，不闪 loading / 不弹错） */
async function fetchCronJobs(silent = false) {
  if (cronRefreshing.value) return
  cronRefreshing.value = true
  if (!silent) cronLoading.value = true
  try {
    const res = await api.get('/v1/admin/cron-jobs')
    const d = res.data?.data || res.data
    cronJobs.value = Array.isArray(d?.jobs) ? d.jobs : []
  } catch {
    if (!silent) message.error('定时任务列表加载失败')
  } finally {
    cronLoading.value = false
    cronRefreshing.value = false
  }
}

/** 手动触发：POST /v1/admin/cron-jobs/{id}/trigger → 成功提示 + 刷新列表 */
async function triggerCronJobById(job: CronJob) {
  try {
    await triggerCronJob(job.id)
    message.success(`已触发「${job.name}」，任务将异步执行`)
    await fetchCronJobs(true)
  } catch (e: any) {
    message.error('触发失败: ' + (e?.response?.data?.error || e?.message || '网络错误'))
  }
}

/** 复制文本：优先 Clipboard API，失败降级 execCommand */
async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      return ok
    } catch {
      return false
    }
  }
}

/** 复制 Webhook 触发地址（仅列表含 webhook_token 时展示） */
async function copyCronWebhook(job: CronJob) {
  if (!job.webhook_token) return
  const ok = await copyText(cronWebhookUrl(job.id, job.webhook_token))
  if (ok) message.success('Webhook 地址已复制')
  else message.error('复制失败，请手动复制')
}

/** last_status → Tag 颜色：成功绿 / 失败红 / 其余灰（pending/未运行） */
function cronStatusColor(s: string): string {
  if (s === 'success') return 'success'
  if (s === 'failed' || s === 'error') return 'error'
  return 'default'
}
function cronStatusLabel(s: string): string {
  if (!s) return '未运行'
  if (s === 'success') return '成功'
  if (s === 'failed' || s === 'error') return '失败'
  return s
}

function formatCronTime(ts: string | null): string {
  if (!ts) return '—'
  const d = new Date(ts)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

// ── 创建 / 编辑 ──
const cronModalOpen = ref(false)
const cronEditing = ref<CronJob | null>(null)
const cronSaving = ref(false)
const cronForm = ref({ name: '', schedule: '', task: '', enabled: true })

function openCronCreate() {
  cronEditing.value = null
  cronForm.value = { name: '', schedule: '0 9 * * *', task: '', enabled: true }
  cronModalOpen.value = true
}

function openCronEdit(job: CronJob) {
  cronEditing.value = job
  cronForm.value = { name: job.name, schedule: job.schedule, task: job.task, enabled: job.enabled }
  cronModalOpen.value = true
}

async function saveCronJob() {
  const f = cronForm.value
  if (!f.name.trim() || !f.schedule.trim() || !f.task.trim()) {
    message.warning('名称、Cron 表达式与任务 JSON 均为必填')
    return
  }
  try {
    JSON.parse(f.task)
  } catch {
    message.error('任务 JSON 格式不正确，请检查后重试')
    return
  }
  cronSaving.value = true
  try {
    if (cronEditing.value) {
      await api.put(`/v1/admin/cron-jobs/${cronEditing.value.id}`, f)
      message.success('已更新定时任务')
    } else {
      const res = await api.post('/v1/admin/cron-jobs', f)
      const d = res.data?.data || res.data
      message.success('已创建定时任务')
      // 创建响应含 webhook_token：顺手复制一次触发地址（列表刷新后行内仍可复制）
      if (d?.id && d?.webhook_token) {
        const ok = await copyText(cronWebhookUrl(d.id, d.webhook_token))
        if (ok) message.success('Webhook 触发地址已复制到剪贴板')
      }
    }
    cronModalOpen.value = false
    await fetchCronJobs()
  } catch (e: any) {
    message.error('保存失败: ' + (e?.response?.data?.error || e?.message || '网络错误'))
  } finally {
    cronSaving.value = false
  }
}

async function deleteCronJob(job: CronJob) {
  try {
    await api.delete(`/v1/admin/cron-jobs/${job.id}`)
    message.success('已删除定时任务')
    await fetchCronJobs(true)
  } catch (e: any) {
    message.error('删除失败: ' + (e?.response?.data?.error || e?.message || '网络错误'))
  }
}

const cronColumns = [
  { title: '名称', dataIndex: 'name', key: 'name', width: 170, ellipsis: true },
  { title: '计划', dataIndex: 'schedule', key: 'schedule', width: 120 },
  { title: '任务', dataIndex: 'task', key: 'task', ellipsis: true },
  { title: '启用', dataIndex: 'enabled', key: 'enabled', width: 64 },
  { title: '上次运行', dataIndex: 'last_run_at', key: 'last_run_at', width: 130 },
  { title: '状态', dataIndex: 'last_status', key: 'last_status', width: 84 },
  { title: '操作', key: 'actions', width: 300 },
]
</script>

<template>
  <div class="dashboard">
    <!-- 骨架屏（加载态） -->
    <PageSkeleton v-if="loading" variant="cards" :columns="2" header />

    <!-- 错误态 -->
    <EmptyState
      v-else-if="error"
      size="page"
      :description="error"
      hint="请检查后端服务是否正常，或稍后重试"
    >
      <a-button type="primary" size="large" @click="fetchDashboardData">重试</a-button>
    </EmptyState>

    <template v-else>
      <!-- 快捷导航卡片 -->
      <div class="nav-grid">
        <div
          v-for="nav in navItems"
          :key="nav.path"
          class="nav-card"
          role="link"
          tabindex="0"
          @click="router.push(nav.path)"
          @keydown.enter="router.push(nav.path)"
        >
          <div class="nav-card-icon">
            <component :is="nav.icon" />
          </div>
          <div class="nav-card-body">
            <div class="nav-card-label">{{ nav.label }}</div>
            <div class="nav-card-desc">{{ nav.desc }}</div>
          </div>
          <ArrowRightOutlined class="nav-card-arrow" />
        </div>
      </div>

      <!-- 统计卡片 -->
      <div class="stats-grid">
        <Card class="stat-card" :bordered="false">
          <Statistic
            title="并发连接"
            :value="stats.connections"
            :value-style="{ color: 'var(--text-primary)', fontSize: '28px', fontFamily: 'var(--font-sans)', fontVariantNumeric: 'tabular-nums' }"
          >
            <template #prefix><ApartmentOutlined class="stat-icon" /></template>
            <template #suffix>
              <Tag v-if="stats.connectionsTrend !== 0" :color="stats.connectionsTrend > 0 ? 'success' : 'error'" class="stat-trend">
                <component :is="stats.connectionsTrend > 0 ? ArrowUpOutlined : ArrowDownOutlined" />
                {{ Math.abs(stats.connectionsTrend) }}%
              </Tag>
            </template>
          </Statistic>
        </Card>
        <Card class="stat-card" :bordered="false">
          <Statistic
            title="队列积压"
            :value="stats.queueBacklog"
            :value-style="{ color: 'var(--text-primary)', fontSize: '28px', fontVariantNumeric: 'tabular-nums' }"
          >
            <template #prefix><OrderedListOutlined class="stat-icon" /></template>
            <template #suffix>
              <Tag :color="stats.queueBacklog > 1000 ? 'error' : 'success'" class="stat-trend">
                {{ stats.queueBacklog > 1000 ? '告警' : '正常' }}
              </Tag>
            </template>
          </Statistic>
        </Card>
        <Card class="stat-card" :bordered="false">
          <Statistic
            title="缓存命中率"
            :value="stats.cacheHitRate"
            suffix="%"
            :value-style="{ color: 'var(--text-primary)', fontSize: '28px', fontVariantNumeric: 'tabular-nums' }"
          >
            <template #prefix><DatabaseFilled class="stat-icon" /></template>
          </Statistic>
        </Card>
        <Card class="stat-card" :bordered="false">
          <Statistic
            title="API 延迟 P99"
            :value="stats.latencyP99"
            suffix="ms"
            :value-style="{ color: 'var(--text-primary)', fontSize: '28px', fontVariantNumeric: 'tabular-nums' }"
          >
            <template #prefix><ThunderboltFilled class="stat-icon" /></template>
          </Statistic>
        </Card>
      </div>

      <!-- 图表区域 -->
      <Row :gutter="16" class="charts-row">
        <Col :xs="24" :lg="12">
          <Card class="chart-card" :bordered="false">
            <template #title>
              <span class="chart-title">
                <ApartmentOutlined class="chart-title-icon" /> 并发连接趋势
              </span>
            </template>
            <VChart :option="connectionChartOption" class="chart-canvas" autoresize />
          </Card>
        </Col>
        <Col :xs="24" :lg="12">
          <Card class="chart-card" :bordered="false">
            <template #title>
              <span class="chart-title">
                <KeyOutlined class="chart-title-icon" /> API Key 状态
              </span>
            </template>
            <VChart :option="apiKeyChartOption" class="chart-canvas" autoresize />
          </Card>
        </Col>
      </Row>

      <!-- 告警列表 -->
      <Card class="alert-card" :bordered="false">
        <template #title>
          <span class="chart-title">
            <ClockCircleOutlined class="chart-title-icon" /> 最近告警
          </span>
        </template>
        <EmptyState
          v-if="alerts.length === 0"
          size="list"
          description="暂无告警"
          hint="系统运行正常"
        />
        <Table v-else :columns="alertColumns" :data-source="alerts" :pagination="false" size="small" />
      </Card>
    </template>

    <!-- 定时任务：列表 + 手动触发 + Webhook + 自动刷新（独立于指标加载态，30s 自动刷新） -->
    <Card class="cron-card" :bordered="false">
      <template #title>
        <span class="chart-title">
          <ClockCircleOutlined class="chart-title-icon" /> 定时任务
        </span>
      </template>
      <template #extra>
        <Space :size="8">
          <Button size="small" :icon="h(ReloadOutlined)" :loading="cronRefreshing" @click="fetchCronJobs()">立即刷新</Button>
          <Button size="small" type="primary" :icon="h(PlusOutlined)" @click="openCronCreate">新建任务</Button>
        </Space>
      </template>
      <EmptyState
        v-if="!cronLoading && cronJobs.length === 0"
        size="list"
        description="暂无定时任务"
        hint="点击「新建任务」创建定时自动化任务"
      />
      <Table
        v-else
        :columns="cronColumns"
        :data-source="cronJobs"
        :loading="cronLoading"
        :pagination="{ pageSize: 8, showSizeChanger: false, showTotal: (t: number) => `共 ${t} 条` }"
        size="small"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'schedule'">
            <code class="cron-schedule">{{ record.schedule }}</code>
          </template>
          <template v-else-if="column.key === 'task'">
            <Tooltip :title="record.task" placement="topLeft">
              <span class="cron-task">{{ record.task }}</span>
            </Tooltip>
          </template>
          <template v-else-if="column.key === 'enabled'">
            <Switch :checked="record.enabled" size="small" disabled />
          </template>
          <template v-else-if="column.key === 'last_run_at'">
            {{ formatCronTime(record.last_run_at) }}
          </template>
          <template v-else-if="column.key === 'last_status'">
            <Tag :color="cronStatusColor(record.last_status)">{{ cronStatusLabel(record.last_status) }}</Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Space :size="0" wrap>
              <Button size="small" type="link" :icon="h(PlayCircleOutlined)" @click="triggerCronJobById(record as CronJob)">手动触发</Button>
              <Button v-if="record.webhook_token" size="small" type="link" :icon="h(CopyOutlined)" @click="copyCronWebhook(record as CronJob)">复制 Webhook</Button>
              <Button size="small" type="link" :icon="h(EditOutlined)" @click="openCronEdit(record as CronJob)">编辑</Button>
              <Popconfirm
                title="确定删除该定时任务？"
                ok-text="删除"
                cancel-text="取消"
                @confirm="deleteCronJob(record as CronJob)"
              >
                <Button size="small" type="link" danger :icon="h(DeleteOutlined)">删除</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <!-- 新建 / 编辑定时任务对话框 -->
    <Modal
      :open="cronModalOpen"
      :title="cronEditing ? '编辑定时任务' : '新建定时任务'"
      :confirm-loading="cronSaving"
      ok-text="保存"
      cancel-text="取消"
      @ok="saveCronJob"
      @cancel="cronModalOpen = false"
    >
      <div class="cron-form">
        <div class="cron-field">
          <label class="cron-label">任务名称</label>
          <Input v-model:value="cronForm.name" placeholder="例如：每日早报生成" :maxlength="120" />
        </div>
        <div class="cron-field">
          <label class="cron-label">Cron 表达式</label>
          <Input v-model:value="cronForm.schedule" placeholder="例如：0 9 * * *（每天 09:00）" />
          <div class="cron-hint">标准 5 段 Cron：分 时 日 月 周</div>
        </div>
        <div class="cron-field">
          <label class="cron-label">任务 JSON</label>
          <Input.TextArea v-model:value="cronForm.task" :rows="4" class="cron-task-input" placeholder='{"type":"agent","agent_id":"...","prompt":"..."}' />
          <div class="cron-hint">
            任务类型示例：<br />
            <code>{"type":"agent","agent_id":"...","prompt":"..."}</code>（Agent 任务）<br />
            <code>{"type":"quick","user_input":"...","mode":"auto"}</code>（统一任务）
          </div>
        </div>
        <div class="cron-field cron-field-row">
          <label class="cron-label">启用</label>
          <Switch v-model:checked="cronForm.enabled" />
        </div>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.dashboard { display: flex; flex-direction: column; gap: 16px; }

/* ── 快捷导航卡片网格 ── */
.nav-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}
.nav-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--border-card);
  border-radius: 10px;
  background: var(--bg-card);
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, transform 0.15s ease;
}
.nav-card:hover {
  border-color: var(--primary);
  box-shadow: var(--shadow-md);
  transform: translateY(-1px);
}
.nav-card:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.nav-card:active { transform: translateY(0); }
.nav-card-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 8px;
  background: var(--primary-bg);
  color: var(--primary);
  font-size: 16px;
  flex-shrink: 0;
}
.nav-card-body { flex: 1; min-width: 0; }
.nav-card-label {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}
.nav-card-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 1px;
}
.nav-card-arrow {
  font-size: 12px;
  color: var(--text-muted);
  transition: transform 0.15s ease, color 0.15s ease;
}
.nav-card:hover .nav-card-arrow { color: var(--primary); transform: translateX(2px); }

/* ── 统计卡片 ── */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 12px;
}
.stat-card {
  border-radius: 10px !important;
  background: var(--bg-card) !important;
  box-shadow: var(--shadow-md);
  min-width: 0;
}
.stat-card :deep(.ant-card-body) { padding: 18px 20px; }
.stat-card :deep(.ant-statistic-title) {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-bottom: 6px;
}
.stat-icon { font-size: 18px; color: var(--primary); margin-right: 4px; }
.stat-trend { font-size: 11px; padding: 0 6px; border-radius: 4px; margin-left: 4px; }

/* ── 图表卡片 ── */
.charts-row { margin-bottom: 0 !important; }
.chart-card {
  border-radius: 10px !important;
  background: var(--bg-card) !important;
  box-shadow: var(--shadow-md);
}
.chart-card :deep(.ant-card-head) {
  min-height: 44px;
  border-bottom: 1px solid var(--border-card);
  padding: 0 16px;
}
.chart-card :deep(.ant-card-body) { padding: 12px 16px 16px; }
.chart-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}
.chart-title-icon { color: var(--primary); font-size: 15px; }
.chart-canvas { height: 280px; }

/* ── 告警卡片 ── */
.alert-card {
  border-radius: 10px !important;
  background: var(--bg-card) !important;
  box-shadow: var(--shadow-md);
}
.alert-card :deep(.ant-card-body) { padding: 0 16px 16px; }
.alert-card :deep(.ant-table) {
  background: transparent;
}
.alert-card :deep(.ant-table-thead > tr > th) {
  background: var(--bg-secondary);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-card);
}
.alert-card :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--border-card);
  font-size: 13px;
}
.alert-card :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--bg-hover) !important;
}

/* 移动端：导航卡片两列 → 单列；统计/图表高度压缩 */
@media (max-width: 768px) {
  .nav-grid { grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); }
  .chart-canvas { height: 220px; }
}
@media (max-width: 576px) {
  .nav-grid { grid-template-columns: 1fr; }
  .chart-canvas { height: 200px; }
  .stat-card :deep(.ant-card-body) { padding: 14px 16px; }
  .stat-card :deep(.ant-statistic-content-value) { font-size: 22px !important; }
}

/* ── 定时任务卡片 ── */
.cron-card {
  border-radius: 10px !important;
  background: var(--bg-card) !important;
  box-shadow: var(--shadow-md);
}
.cron-card :deep(.ant-card-head) {
  min-height: 44px;
  border-bottom: 1px solid var(--border-card);
  padding: 0 16px;
}
.cron-card :deep(.ant-card-body) { padding: 4px 16px 16px; }
.cron-card :deep(.ant-table) { background: transparent; }
.cron-card :deep(.ant-table-thead > tr > th) {
  background: var(--bg-secondary);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-tertiary);
  border-bottom: 1px solid var(--border-card);
}
.cron-card :deep(.ant-table-tbody > tr > td) {
  border-bottom: 1px solid var(--border-card);
  font-size: 13px;
}
.cron-card :deep(.ant-table-tbody > tr:hover > td) {
  background: var(--bg-hover) !important;
}
.cron-schedule {
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  padding: 1px 6px;
  border-radius: 4px;
}
.cron-task {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-secondary);
}

/* ── 新建/编辑对话框 ── */
.cron-form { display: flex; flex-direction: column; gap: 12px; }
.cron-field { display: flex; flex-direction: column; gap: 4px; }
.cron-field-row { flex-direction: row; align-items: center; gap: 8px; }
.cron-label { font-size: 12px; font-weight: 600; color: var(--text-secondary); }
.cron-hint { font-size: 12px; color: var(--text-tertiary); line-height: 1.7; }
.cron-hint code {
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--bg-secondary);
  padding: 1px 5px;
  border-radius: 4px;
  color: var(--text-secondary);
  word-break: break-all;
}
.cron-task-input { font-family: var(--font-mono); font-size: 12px; }
</style>
