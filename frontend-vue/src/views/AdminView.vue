<script setup lang="ts">
import { ref, onMounted, h, reactive } from 'vue'
import {
  NCard, NDataTable, NSpin, NTabs, NTabPane, NIcon, NStatistic,
  NGrid, NGi, NButton, NSpace, NTag, NModal, NForm, NFormItem,
  NInput, NSelect, NPopconfirm, useMessage, NProgress, NDescriptions,
  NDescriptionsItem, NDivider, NSwitch, NAlert, NScrollbar,
} from 'naive-ui'
import {
  SettingsOutline, PeopleOutline, ServerOutline, BarChartOutline,
  ShieldCheckmarkOutline, CreateOutline, TrashOutline,
  CloudUploadOutline, HardwareChipOutline, PulseOutline,
  RefreshOutline, DownloadOutline, DocumentTextOutline,
} from '@vicons/ionicons5'
import { api } from '../api'

const message = useMessage()
const loading = ref(true)
const activeTab = ref('overview')

// ── Overview / Metrics ──
const metrics = ref<any>(null)
const health = ref<any>(null)
const systemInfo = ref<any>(null)

// ── Users ──
const users = ref<any[]>([])
const usersLoading = ref(false)
const editingUser = ref<any>(null)
const showUserModal = ref(false)
const userForm = reactive({ email: '', name: '', role: 'user' })

// ── Storage ──
const storageInfo = ref<any>(null)
const storageLoading = ref(false)
const showStorageModal = ref(false)
const storageForm = reactive({
  backend: 'local',
  s3_endpoint: '',
  s3_bucket: '',
  s3_access_key: '',
  s3_secret_key: '',
  s3_use_ssl: false,
})

// ── Redis ──
const redisInfo = ref<any>(null)
const redisLoading = ref(false)
const showRedisModal = ref(false)
const redisForm = reactive({
  mode: 'single',
  addr: '',
  addrs: '',
  master_name: '',
  sentinel_addrs: '',
})

// ── Maintenance ──
const maintenanceLoading = ref<string | null>(null)

// ── Traces ──
const traces = ref<any[]>([])
const tracesLoading = ref(false)

// ── LLM Metrics ──
const llmMetrics = ref<any>(null)
const llmCache = ref<any>(null)

// ── Columns ──
const userColumns = [
  { title: 'ID', key: 'id', width: 100, ellipsis: { tooltip: true } },
  { title: '邮箱', key: 'email', width: 200 },
  { title: '姓名', key: 'name', width: 150 },
  {
    title: '角色',
    key: 'role',
    width: 100,
    render(row: any) {
      const colorMap: Record<string, string> = { owner: 'error', admin: 'warning', user: 'success' }
      return h(NTag, { type: (colorMap[row.role] || 'default') as any, size: 'small' }, { default: () => row.role })
    },
  },
  { title: '创建时间', key: 'created_at', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 130,
    render(row: any) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, { size: 'small', quaternary: true, type: 'info', onClick: () => openEditUser(row) }, {
            icon: () => h(NIcon, null, { default: () => h(CreateOutline) }),
          }),
          h(NPopconfirm, { onPositiveClick: () => deleteUser(row.id) }, {
            trigger: () => h(NButton, { size: 'small', quaternary: true, type: 'error' }, {
              icon: () => h(NIcon, null, { default: () => h(TrashOutline) }),
            }),
            default: () => `确认删除用户 ${row.name || row.email}?`,
          }),
        ],
      })
    },
  },
]

const traceColumns = [
  { title: 'ID', key: 'id', width: 100, ellipsis: { tooltip: true } },
  { title: '工具', key: 'name', width: 160 },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row: any) {
      return h(NTag, { type: row.status === 'ok' ? 'success' : 'error', size: 'small' }, { default: () => row.status })
    },
  },
  { title: '耗时(ms)', key: 'duration_ms', width: 100 },
  { title: '时间', key: 'timestamp', width: 170 },
]

// ── Lifecycle ──
onMounted(async () => {
  await loadAllData()
})

async function loadAllData() {
  loading.value = true
  try {
    await Promise.all([
      loadOverview(),
      loadUsers(),
      loadHealth(),
      loadSystemInfo(),
      loadStorageInfo(),
      loadRedisInfo(),
      loadTraces(),
      loadLLMMetrics(),
    ])
  } finally {
    loading.value = false
  }
}

// ── Data Loaders ──

async function loadOverview() {
  try {
    const res = await api.get('/v1/admin/metrics')
    metrics.value = res.data?.data || {}
  } catch { metrics.value = {} }
}

async function loadHealth() {
  try {
    const res = await api.get('/v1/system/health')
    health.value = res.data?.data || {}
  } catch { health.value = {} }
}

async function loadSystemInfo() {
  try {
    const res = await api.get('/v1/admin/system')
    systemInfo.value = res.data?.data || {}
  } catch { systemInfo.value = {} }
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const res = await api.get('/v1/admin/users')
    users.value = res.data?.data || []
  } catch { users.value = [] }
  finally { usersLoading.value = false }
}

async function loadStorageInfo() {
  try {
    const res = await api.get('/v1/admin/storage')
    storageInfo.value = res.data?.data || {}
  } catch { storageInfo.value = {} }
}

async function loadRedisInfo() {
  try {
    const res = await api.get('/v1/admin/redis')
    redisInfo.value = res.data?.data || {}
  } catch { redisInfo.value = {} }
}

async function loadTraces() {
  tracesLoading.value = true
  try {
    const res = await api.get('/v1/system/traces')
    traces.value = res.data?.data?.traces || []
  } catch { traces.value = [] }
  finally { tracesLoading.value = false }
}

async function loadLLMMetrics() {
  try {
    const [mRes, cRes] = await Promise.all([
      api.get('/v1/llm/metrics').catch(() => ({ data: { data: {} } })),
      api.get('/v1/llm/cache').catch(() => ({ data: { data: {} } })),
    ])
    llmMetrics.value = mRes.data?.data || {}
    llmCache.value = cRes.data?.data || {}
  } catch { /* ignore */ }
}

// ── User CRUD ──

function openEditUser(user: any) {
  editingUser.value = user
  userForm.email = user.email
  userForm.name = user.name
  userForm.role = user.role
  showUserModal.value = true
}

async function saveUser() {
  if (!editingUser.value) return
  try {
    await api.put(`/v1/admin/users/${editingUser.value.id}`, {
      email: userForm.email,
      name: userForm.name,
      role: userForm.role,
    })
    message.success('用户已更新')
    showUserModal.value = false
    await loadUsers()
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '更新失败')
  }
}

async function deleteUser(id: string) {
  try {
    await api.delete(`/v1/admin/users/${id}`)
    message.success('用户已删除')
    await loadUsers()
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '删除失败')
  }
}

// ── Storage ──

function openStorageEdit() {
  const info = storageInfo.value?.config || {}
  storageForm.backend = storageInfo.value?.backend || 'local'
  storageForm.s3_endpoint = info.s3_endpoint || ''
  storageForm.s3_bucket = info.s3_bucket || ''
  storageForm.s3_access_key = ''
  storageForm.s3_secret_key = ''
  storageForm.s3_use_ssl = info.s3_use_ssl ?? false
  showStorageModal.value = true
}

async function saveStorage() {
  storageLoading.value = true
  try {
    const res = await api.put('/v1/admin/storage', storageForm)
    const data = res.data?.data || {}
    if (data.warning) message.warning(data.warning)
    else message.success('存储配置已更新')
    showStorageModal.value = false
    await loadStorageInfo()
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '更新失败')
  } finally { storageLoading.value = false }
}

async function testStorage() {
  try {
    const res = await api.post('/v1/admin/storage/test', storageForm)
    const data = res.data?.data || {}
    if (data.status === 'ok') message.success(data.message)
    else message.error(data.message || '测试失败')
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '测试失败')
  }
}

// ── Redis ──

function openRedisEdit() {
  redisForm.mode = redisInfo.value?.mode || 'single'
  redisForm.addr = ''
  redisForm.addrs = ''
  redisForm.master_name = ''
  redisForm.sentinel_addrs = ''
  showRedisModal.value = true
}

function buildRedisConfig() {
  const cfg: any = { mode: redisForm.mode }
  if (redisForm.mode === 'single') cfg.addr = redisForm.addr
  if (redisForm.mode === 'cluster') cfg.addrs = redisForm.addrs.split(',').map(s => s.trim()).filter(Boolean)
  if (redisForm.mode === 'sentinel') {
    cfg.master_name = redisForm.master_name
    cfg.sentinel_addrs = redisForm.sentinel_addrs.split(',').map(s => s.trim()).filter(Boolean)
  }
  return cfg
}

async function saveRedis() {
  redisLoading.value = true
  try {
    const res = await api.put('/v1/admin/redis', buildRedisConfig())
    message.success('Redis 配置已更新')
    showRedisModal.value = false
    await loadRedisInfo()
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '更新失败')
  } finally { redisLoading.value = false }
}

async function testRedis() {
  try {
    const res = await api.post('/v1/admin/redis/test', buildRedisConfig())
    const data = res.data?.data || {}
    if (data.status === 'ok') message.success(data.message)
    else message.error(data.message || '测试失败')
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '测试失败')
  }
}

// ── Maintenance ──

async function triggerMaintenance(action: string) {
  maintenanceLoading.value = action
  try {
    const res = await api.post('/v1/admin/maintenance', { action })
    message.success(`${action} 操作完成`)
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || `${action} 失败`)
  } finally { maintenanceLoading.value = null }
}

async function createBackup() {
  maintenanceLoading.value = 'backup'
  try {
    const res = await api.post('/v1/admin/backup', {}, { responseType: 'blob' })
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const a = document.createElement('a')
    a.href = url
    a.download = `minicc_backup_${new Date().toISOString().slice(0, 10)}.sql`
    a.click()
    window.URL.revokeObjectURL(url)
    message.success('备份已下载')
  } catch (e: any) {
    message.error(e.response?.data?.error || e.message || '备份失败')
  } finally { maintenanceLoading.value = null }
}

// ── Helpers ──

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return `${d}天 ${h}时 ${m}分`
  if (h > 0) return `${h}时 ${m}分 ${s}秒`
  return `${m}分 ${s}秒`
}

function formatTime(t: string): string {
  if (!t) return '-'
  try { return new Date(t).toLocaleString('zh-CN') } catch { return t }
}
</script>

<template>
  <div class="admin-container">
    <div class="admin-header">
      <NIcon size="28" color="#6366f1">
        <SettingsOutline />
      </NIcon>
      <h1>系统管理</h1>
      <NButton quaternary circle size="small" @click="loadAllData" :loading="loading" style="margin-left: auto">
        <template #icon><NIcon><RefreshOutline /></NIcon></template>
      </NButton>
    </div>

    <NSpin :show="loading">
      <NTabs v-model:value="activeTab" type="line" animated>
        <!-- ═══ 概览 ═══ -->
        <NTabPane name="overview" tab="概览">
          <NGrid :cols="4" :x-gap="16" :y-gap="16" style="margin-top: 16px" responsive="screen" :item-responsive="true">
            <NGi span="4 m:1">
              <NCard size="small">
                <NStatistic label="总请求数" :value="metrics?.requests_total ?? 0">
                  <template #prefix><NIcon><BarChartOutline /></NIcon></template>
                </NStatistic>
              </NCard>
            </NGi>
            <NGi span="4 m:1">
              <NCard size="small">
                <NStatistic label="活跃请求" :value="metrics?.requests_active ?? 0">
                  <template #prefix><NIcon><PulseOutline /></NIcon></template>
                </NStatistic>
              </NCard>
            </NGi>
            <NGi span="4 m:1">
              <NCard size="small">
                <NStatistic label="LLM 调用" :value="metrics?.llm_calls_total ?? 0">
                  <template #prefix><NIcon><BarChartOutline /></NIcon></template>
                </NStatistic>
              </NCard>
            </NGi>
            <NGi span="4 m:1">
              <NCard size="small">
                <NStatistic label="运行时间" :value="formatUptime(metrics?.uptime_seconds ?? 0)">
                  <template #prefix><NIcon><ServerOutline /></NIcon></template>
                </NStatistic>
              </NCard>
            </NGi>
          </NGrid>

          <!-- System Info -->
          <NCard title="系统信息" size="small" style="margin-top: 16px">
            <NDescriptions :column="3" bordered label-placement="left" size="small">
              <NDescriptionsItem label="版本">{{ systemInfo?.version ?? '-' }}</NDescriptionsItem>
              <NDescriptionsItem label="运行时间">{{ systemInfo?.uptime ?? '-' }}</NDescriptionsItem>
              <NDescriptionsItem label="PostgreSQL">
                <NTag :type="systemInfo?.db?.postgres ? 'success' : 'error'" size="small">
                  {{ systemInfo?.db?.postgres ? '已连接' : '未连接' }}
                </NTag>
              </NDescriptionsItem>
              <NDescriptionsItem label="Redis">
                <NTag :type="systemInfo?.db?.redis ? 'success' : 'error'" size="small">
                  {{ systemInfo?.db?.redis ? '已连接' : '未连接' }}
                </NTag>
              </NDescriptionsItem>
              <NDescriptionsItem label="工具错误">{{ metrics?.tool_errors ?? 0 }}</NDescriptionsItem>
              <NDescriptionsItem label="LLM 错误">{{ metrics?.llm_errors ?? 0 }}</NDescriptionsItem>
            </NDescriptions>
          </NCard>

          <!-- LLM Metrics -->
          <NCard title="LLM 服务" size="small" style="margin-top: 16px">
            <NGrid :cols="2" :x-gap="16" :y-gap="12">
              <NGi>
                <NDescriptions :column="1" bordered label-placement="left" size="small">
                  <NDescriptionsItem label="总调用">{{ llmMetrics?.total_calls ?? 0 }}</NDescriptionsItem>
                  <NDescriptionsItem label="总 Token (输入)">{{ llmMetrics?.total_input_tokens ?? 0 }}</NDescriptionsItem>
                  <NDescriptionsItem label="总 Token (输出)">{{ llmMetrics?.total_output_tokens ?? 0 }}</NDescriptionsItem>
                  <NDescriptionsItem label="错误数">{{ llmMetrics?.errors ?? 0 }}</NDescriptionsItem>
                </NDescriptions>
              </NGi>
              <NGi>
                <NDescriptions :column="1" bordered label-placement="left" size="small">
                  <NDescriptionsItem label="缓存命中">{{ llmCache?.hits ?? 0 }}</NDescriptionsItem>
                  <NDescriptionsItem label="缓存未命中">{{ llmCache?.misses ?? 0 }}</NDescriptionsItem>
                  <NDescriptionsItem label="命中率">
                    {{ llmCache?.hits && llmCache?.misses ? ((llmCache.hits / (llmCache.hits + llmCache.misses)) * 100).toFixed(1) + '%' : '-' }}
                  </NDescriptionsItem>
                  <NDescriptionsItem label="缓存大小">{{ llmCache?.size ?? 0 }}</NDescriptionsItem>
                </NDescriptions>
              </NGi>
            </NGrid>
          </NCard>
        </NTabPane>

        <!-- ═══ 系统健康 ═══ -->
        <NTabPane name="health" tab="系统健康">
          <NCard title="健康评分" size="small" style="margin-top: 16px">
            <div v-if="health?.scores?.length" class="health-grid">
              <div v-for="(item, i) in health.scores" :key="i" class="health-item">
                <div class="health-label">{{ item.label }}</div>
                <NProgress
                  type="line"
                  :percentage="item.score"
                  :color="item.score > 80 ? '#22c55e' : item.score > 60 ? '#f59e0b' : '#ef4444'"
                  :show-indicator="true"
                  style="flex: 1"
                />
              </div>
              <div v-if="health?.uptime" class="health-item">
                <span class="health-label">运行时间</span>
                <span class="health-value">{{ formatUptime(health.uptime) }}</span>
              </div>
            </div>
            <p v-else style="color: #6b7280; text-align: center; padding: 24px">暂无健康数据</p>
          </NCard>

          <!-- Traces -->
          <NCard title="最近工具调用" size="small" style="margin-top: 16px">
            <template #header-extra>
              <NButton size="small" quaternary @click="loadTraces" :loading="tracesLoading">
                <template #icon><NIcon><RefreshOutline /></NIcon></template>
                刷新
              </NButton>
            </template>
            <NDataTable
              :columns="traceColumns"
              :data="traces"
              :bordered="false"
              :single-line="false"
              :max-height="400"
              size="small"
            />
          </NCard>
        </NTabPane>

        <!-- ═══ 用户管理 ═══ -->
        <NTabPane name="users" tab="用户管理">
          <NCard size="small" style="margin-top: 16px">
            <template #header>
              <div class="card-header">
                <NIcon><PeopleOutline /></NIcon>
                <span>用户列表</span>
                <NTag :bordered="false" size="small" type="info" style="margin-left: 8px">{{ users.length }}</NTag>
              </div>
            </template>
            <template #header-extra>
              <NButton size="small" quaternary @click="loadUsers" :loading="usersLoading">
                <template #icon><NIcon><RefreshOutline /></NIcon></template>
                刷新
              </NButton>
            </template>
            <NDataTable
              :columns="userColumns"
              :data="users"
              :bordered="false"
              :single-line="false"
              :max-height="500"
              size="small"
            />
          </NCard>
        </NTabPane>

        <!-- ═══ 系统设置 ═══ -->
        <NTabPane name="settings" tab="系统设置">
          <NGrid :cols="2" :x-gap="16" :y-gap="16" style="margin-top: 16px" responsive="screen" :item-responsive="true">
            <!-- Storage -->
            <NGi span="2 m:1">
              <NCard title="存储管理" size="small">
                <template #header-extra>
                  <NButton size="small" quaternary type="primary" @click="openStorageEdit">配置</NButton>
                </template>
                <NDescriptions :column="1" bordered label-placement="left" size="small">
                  <NDescriptionsItem label="后端">
                    <NTag :type="storageInfo?.backend === 's3' ? 'warning' : 'success'" size="small">
                      {{ storageInfo?.backend ?? '未知' }}
                    </NTag>
                  </NDescriptionsItem>
                  <NDescriptionsItem label="状态">
                    <NTag type="success" size="small">正常</NTag>
                  </NDescriptionsItem>
                </NDescriptions>
              </NCard>
            </NGi>

            <!-- Redis -->
            <NGi span="2 m:1">
              <NCard title="Redis 管理" size="small">
                <template #header-extra>
                  <NButton size="small" quaternary type="primary" @click="openRedisEdit">配置</NButton>
                </template>
                <NDescriptions :column="1" bordered label-placement="left" size="small">
                  <NDescriptionsItem label="状态">
                    <NTag :type="redisInfo?.status === 'connected' ? 'success' : 'error'" size="small">
                      {{ redisInfo?.status === 'connected' ? '已连接' : '未连接' }}
                    </NTag>
                  </NDescriptionsItem>
                  <NDescriptionsItem label="模式">{{ redisInfo?.mode ?? '-' }}</NDescriptionsItem>
                  <NDescriptionsItem v-if="redisInfo?.pool" label="连接池">
                    总计 {{ redisInfo.pool.total_conns ?? 0 }} / 空闲 {{ redisInfo.pool.idle_conns ?? 0 }}
                  </NDescriptionsItem>
                </NDescriptions>
              </NCard>
            </NGi>
          </NGrid>

          <!-- Maintenance -->
          <NCard title="维护操作" size="small" style="margin-top: 16px">
            <NSpace>
              <NButton
                :loading="maintenanceLoading === 'vacuum'"
                @click="triggerMaintenance('vacuum')"
                size="small"
              >
                VACUUM ANALYZE
              </NButton>
              <NButton
                :loading="maintenanceLoading === 'reindex'"
                @click="triggerMaintenance('reindex')"
                size="small"
              >
                REINDEX
              </NButton>
              <NButton
                :loading="maintenanceLoading === 'analyze'"
                @click="triggerMaintenance('analyze')"
                size="small"
              >
                ANALYZE
              </NButton>
              <NButton
                :loading="maintenanceLoading === 'flush_cache'"
                @click="triggerMaintenance('flush_cache')"
                size="small"
                type="warning"
              >
                清除缓存
              </NButton>
            </NSpace>
          </NCard>

          <!-- Backup -->
          <NCard title="备份与恢复" size="small" style="margin-top: 16px">
            <NSpace>
              <NButton
                :loading="maintenanceLoading === 'backup'"
                @click="createBackup"
                type="primary"
                size="small"
              >
                <template #icon><NIcon><DownloadOutline /></NIcon></template>
                导出备份
              </NButton>
            </NSpace>
          </NCard>
        </NTabPane>
      </NTabs>
    </NSpin>

    <!-- ═══ User Edit Modal ═══ -->
    <NModal v-model:show="showUserModal" preset="card" title="编辑用户" style="width: 480px">
      <NForm label-placement="left" label-width="80">
        <NFormItem label="邮箱">
          <NInput v-model:value="userForm.email" placeholder="邮箱" />
        </NFormItem>
        <NFormItem label="姓名">
          <NInput v-model:value="userForm.name" placeholder="姓名" />
        </NFormItem>
        <NFormItem label="角色">
          <NSelect
            v-model:value="userForm.role"
            :options="[
              { label: 'user', value: 'user' },
              { label: 'admin', value: 'admin' },
              { label: 'owner', value: 'owner' },
            ]"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showUserModal = false">取消</NButton>
          <NButton type="primary" @click="saveUser">保存</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ═══ Storage Config Modal ═══ -->
    <NModal v-model:show="showStorageModal" preset="card" title="存储配置" style="width: 520px">
      <NForm label-placement="left" label-width="100">
        <NFormItem label="后端类型">
          <NSelect
            v-model:value="storageForm.backend"
            :options="[
              { label: '本地存储 (Local)', value: 'local' },
              { label: '对象存储 (S3)', value: 's3' },
            ]"
          />
        </NFormItem>
        <template v-if="storageForm.backend === 's3'">
          <NFormItem label="S3 Endpoint">
            <NInput v-model:value="storageForm.s3_endpoint" placeholder="e.g. localhost:9000" />
          </NFormItem>
          <NFormItem label="Bucket">
            <NInput v-model:value="storageForm.s3_bucket" placeholder="e.g. minicc" />
          </NFormItem>
          <NFormItem label="Access Key">
            <NInput v-model:value="storageForm.s3_access_key" placeholder="Access Key" />
          </NFormItem>
          <NFormItem label="Secret Key">
            <NInput v-model:value="storageForm.s3_secret_key" type="password" placeholder="Secret Key" show-password-on="click" />
          </NFormItem>
          <NFormItem label="使用 SSL">
            <NSwitch v-model:value="storageForm.s3_use_ssl" />
          </NFormItem>
        </template>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="testStorage">测试连接</NButton>
          <NButton @click="showStorageModal = false">取消</NButton>
          <NButton type="primary" :loading="storageLoading" @click="saveStorage">保存</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ═══ Redis Config Modal ═══ -->
    <NModal v-model:show="showRedisModal" preset="card" title="Redis 配置" style="width: 520px">
      <NForm label-placement="left" label-width="100">
        <NFormItem label="模式">
          <NSelect
            v-model:value="redisForm.mode"
            :options="[
              { label: '单机 (Single)', value: 'single' },
              { label: '集群 (Cluster)', value: 'cluster' },
              { label: '哨兵 (Sentinel)', value: 'sentinel' },
            ]"
          />
        </NFormItem>
        <NFormItem v-if="redisForm.mode === 'single'" label="地址">
          <NInput v-model:value="redisForm.addr" placeholder="e.g. localhost:6379" />
        </NFormItem>
        <template v-if="redisForm.mode === 'cluster'">
          <NFormItem label="节点地址">
            <NInput v-model:value="redisForm.addrs" placeholder="逗号分隔, e.g. host1:6379,host2:6379" />
          </NFormItem>
        </template>
        <template v-if="redisForm.mode === 'sentinel'">
          <NFormItem label="主节点名">
            <NInput v-model:value="redisForm.master_name" placeholder="e.g. mymaster" />
          </NFormItem>
          <NFormItem label="哨兵地址">
            <NInput v-model:value="redisForm.sentinel_addrs" placeholder="逗号分隔, e.g. host1:26379,host2:26379" />
          </NFormItem>
        </template>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="testRedis">测试连接</NButton>
          <NButton @click="showRedisModal = false">取消</NButton>
          <NButton type="primary" :loading="redisLoading" @click="saveRedis">保存</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.admin-container {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.admin-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.admin-header h1 {
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

.health-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.health-item {
  display: flex;
  align-items: center;
  gap: 16px;
}

.health-label {
  font-weight: 500;
  min-width: 100px;
  white-space: nowrap;
}

.health-value {
  font-weight: 600;
  color: #2080f0;
}
</style>
