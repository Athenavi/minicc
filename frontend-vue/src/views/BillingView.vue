<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import dayjs from 'dayjs'
import QRCode from 'qrcode'
import {
  Card, Button, Spin, Input, Radio, Tabs, TabPane, Table, Tag, Progress,
  Alert, Empty, Modal, message,
} from 'ant-design-vue'
import {
  CreditCardOutlined, WalletOutlined, ThunderboltOutlined, BarChartOutlined,
  ShoppingOutlined, QrcodeOutlined, PayCircleOutlined,
} from '@ant-design/icons-vue'
import { api } from '../api'

// ── 类型 ──

interface CreditTx {
  id: string
  amount: number
  balance: number
  reason: string
  created_at: string
}

interface DailyUsage {
  day: string
  tx_count: number
  credits_spent: number
  credits_added: number
}

interface UsageData {
  daily: DailyUsage[]
  total_spent: number
  total_added: number
  period_days: number
}

interface BalanceData {
  balance: number
  daily_free_limit: number
  daily_free_used: number
  daily_free_remaining: number
  within_free_quota: boolean
}

// ── 状态 ──

const route = useRoute()
const router = useRouter()

const balance = ref<number | null>(null)
const freeInfo = ref<{ limit: number; used: number; remaining: number } | null>(null)
const usage = ref<UsageData | null>(null)
const history = ref<CreditTx[]>([])
const historyLoading = ref(false)

const loading = ref(true)
const checkoutLoading = ref(false)
const activeTab = ref('balance')

const credits = ref(1000)
const customCredits = ref('')
const provider = ref('alipay')

// 支付结果提示（从 URL 参数读取一次后清除）
const payResult = ref<{ type: 'success' | 'info'; text: string } | null>(null)

// 二维码支付弹层状态
const qrVisible = ref(false)
const qrCode = ref('')
const currentOrderId = ref('')
const qrChannel = ref<'alipay' | 'wechat'>('alipay')
const qrCanvas = ref<HTMLCanvasElement | null>(null)
const payStatus = ref<'pending' | 'paid' | 'expired' | 'failed'>('pending')
let pollTimer: number | undefined
const PAY_POLL_INTERVAL = 3000

// ── 常量 ──

const PRESET_CREDITS = [500, 1000, 2000, 5000, 10000]

const providerOptions = [
  { label: '支付宝', value: 'alipay' },
  { label: '微信支付', value: 'wechat' },
  { label: 'PayPal', value: 'paypal' },
]

const REASON_MAP: Record<string, { label: string; color: string }> = {
  llm_token: { label: 'LLM 调用', color: 'blue' },
  llm_call: { label: 'LLM 调用', color: 'blue' },
  image_gen: { label: '图片生成', color: 'purple' },
  stripe_topup: { label: 'Stripe 充值', color: 'green' },
  paypal_topup: { label: 'PayPal 充值', color: 'green' },
  recharge: { label: '管理端充值', color: 'green' },
  admin: { label: '管理员调整', color: 'orange' },
}

// ── 计算属性 ──

const todaySpent = computed(() => {
  if (!usage.value?.daily?.length) return 0
  const today = dayjs().format('YYYY-MM-DD')
  return usage.value.daily.find(d => d.day === today)?.credits_spent ?? 0
})

/** 近 30 天消耗序列（升序，用于柱状图） */
const chartData = computed(() => {
  const daily = [...(usage.value?.daily ?? [])].reverse()
  return daily.map(d => ({
    day: d.day,
    label: dayjs(d.day).format('MM-DD'),
    spent: d.credits_spent,
  }))
})

const chartMax = computed(() => Math.max(1, ...chartData.value.map(d => d.spent)))

const effectiveCredits = computed(() => {
  const n = customCredits.value ? parseInt(customCredits.value, 10) : credits.value
  return Number.isFinite(n) && n > 0 ? n : 0
})

const priceHint = computed(() => {
  if (!effectiveCredits.value) return ''
  // 支付宝/微信：1 credit = 1 分人民币；PayPal：1 credit = 1 美分
  if (provider.value === 'paypal') {
    return `≈ $${(effectiveCredits.value / 100).toFixed(2)} USD`
  }
  return `≈ ¥${(effectiveCredits.value / 100).toFixed(2)} CNY`
})

const historyColumns = [
  { title: '时间', key: 'created_at', width: 180 },
  { title: '原因', key: 'reason', width: 180 },
  { title: '金额', key: 'amount', width: 120, align: 'right' as const },
  { title: '余额', key: 'balance', width: 120, align: 'right' as const },
]

// ── 数据加载 ──

onMounted(async () => {
  readPayResultParam()
  await loadBalanceAndUsage()
  if (activeTab.value === 'history') await loadHistory()
})

// 切换 tab 时懒加载历史
watch(activeTab, async (tab) => {
  if (tab === 'history' && history.value.length === 0 && !historyLoading.value) {
    await loadHistory()
  }
})

function readPayResultParam() {
  const success = route.query.success
  const canceled = route.query.canceled
  if (success === '1') {
    payResult.value = { type: 'success', text: '支付成功，Credits 已到账（可能需要几秒同步）' }
  } else if (canceled === '1') {
    payResult.value = { type: 'info', text: '支付已取消，Credits 未变动' }
  }
  if (success === '1' || canceled === '1') {
    // 清除 URL 参数，避免刷新后重复提示
    router.replace({ query: {} })
  }
}

async function loadBalanceAndUsage() {
  try {
    loading.value = true
    const [balRes, usageRes] = await Promise.all([
      api.get('/v1/billing/balance'),
      api.get('/v1/billing/usage'),
    ])
    const bal = balRes.data?.data as BalanceData | undefined
    balance.value = bal?.balance ?? 0
    if (bal && (bal.daily_free_limit || bal.daily_free_used)) {
      freeInfo.value = {
        limit: bal.daily_free_limit,
        used: bal.daily_free_used,
        remaining: bal.daily_free_remaining,
      }
    }
    usage.value = (usageRes.data?.data as UsageData | undefined) ?? null
  } catch (error: any) {
    message.error(error.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadHistory() {
  historyLoading.value = true
  try {
    const resp = await api.get('/v1/billing/history')
    history.value = (resp.data?.data?.history as CreditTx[] | undefined) ?? []
  } catch (error: any) {
    message.error(error.message || '加载交易记录失败')
  } finally {
    historyLoading.value = false
  }
}

// ── 充值 ──

function selectPreset(n: number) {
  credits.value = n
  customCredits.value = ''
}

async function handlePurchase() {
  const amount = effectiveCredits.value
  if (amount <= 0) {
    message.warning('请输入有效的充值数量')
    return
  }
  checkoutLoading.value = true
  try {
    const response = await api.post('/v1/billing/pay', {
      credits: amount,
      channel: provider.value,
    })
    const data = response.data?.data
    if (!data) throw new Error('创建订单失败')

    if (provider.value === 'paypal') {
      // PayPal：跳转授权页
      if (data.checkout_url) {
        // S 安全：校验协议为 http/https，防 javascript:/data: 等 XSS 向量
        if (!/^https?:\/\//i.test(data.checkout_url)) {
          throw new Error('非法的支付链接')
        }
        window.location.href = data.checkout_url
      } else {
        throw new Error('未获取到 PayPal 支付链接')
      }
      return
    }

    // 支付宝/微信：展示二维码并轮询订单状态
    if (!data.qr_code) throw new Error('未获取到支付二维码')
    qrCode.value = data.qr_code
    currentOrderId.value = data.id
    qrChannel.value = provider.value as 'alipay' | 'wechat'
    payStatus.value = 'pending'
    qrVisible.value = true
    await nextTick()
    renderQRCode()
    startPolling()
  } catch (error: any) {
    message.error(error.message || '创建支付订单失败')
  } finally {
    checkoutLoading.value = false
  }
}

async function renderQRCode() {
  if (!qrCanvas.value || !qrCode.value) return
  try {
    await QRCode.toCanvas(qrCanvas.value, qrCode.value, {
      width: 220, margin: 1, errorCorrectionLevel: 'M',
    })
  } catch (e) {
    message.error('二维码生成失败')
  }
}

function startPolling() {
  stopPolling()
  pollTimer = window.setInterval(async () => {
    if (!currentOrderId.value) return
    try {
      const resp = await api.get(`/v1/billing/orders/${encodeURIComponent(currentOrderId.value)}`)
      const order = resp.data?.data
      if (!order) return
      if (order.status === 'paid') {
        payStatus.value = 'paid'
        stopPolling()
        qrVisible.value = false
        message.success('充值成功，Credits 已到账')
        await loadBalanceAndUsage()
        if (activeTab.value === 'history') await loadHistory()
      } else if (order.status === 'expired' || order.status === 'failed') {
        payStatus.value = order.status
        stopPolling()
      }
    } catch (error: any) {
      // 轮询出错静默，等待下一次
      console.error('payment poll error:', error)
    }
  }, PAY_POLL_INTERVAL)
}

function stopPolling() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function closeQR() {
  stopPolling()
  qrVisible.value = false
}

onUnmounted(stopPolling)

// ── 展示辅助 ──

function formatTime(v: string): string {
  return v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-'
}

function reasonInfo(reason: string) {
  return REASON_MAP[reason] ?? { label: reason || '未知', color: 'default' }
}

function amountText(amount: number): string {
  return amount > 0 ? `+${amount}` : `${amount}`
}
</script>

<template>
  <div class="billing-container">
    <div class="billing-header">
      <CreditCardOutlined style="font-size: 26px; color: #f59e0b" />
      <h1>计费管理</h1>
    </div>

    <Alert
      v-if="payResult"
      :type="payResult.type"
      :message="payResult.text"
      style="margin-bottom: 16px"
      show-icon
      :closable="true"
      @close="payResult = null"
    />

    <div v-if="loading" class="loading-state"><Spin size="large" /></div>

    <template v-else>
      <!-- 概览卡片 -->
      <div class="overview-grid">
        <Card class="balance-card">
          <template #title><WalletOutlined /> 当前余额</template>
          <div class="balance-display">
            <span class="balance-amount">{{ balance ?? 0 }}</span>
            <Tag color="#f59e0b">credits</Tag>
          </div>
          <div v-if="freeInfo" class="free-quota">
            <div class="free-quota-label">
              今日免费对话额度
              <span class="free-quota-count">{{ freeInfo.used }} / {{ freeInfo.limit }}</span>
            </div>
            <Progress
              :percent="freeInfo.limit > 0 ? Math.round((freeInfo.used / freeInfo.limit) * 100) : 0"
              :status="freeInfo.remaining > 0 ? 'active' : 'exception'"
              size="small"
            />
          </div>
        </Card>

        <Card class="usage-card">
          <template #title><BarChartOutlined /> 近 {{ usage?.period_days ?? 30 }} 天使用</template>
          <div class="usage-stats">
            <div class="stat-item">
              <span class="stat-label">今日消耗</span>
              <span class="stat-value stat-spent">{{ todaySpent }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">累计消耗</span>
              <span class="stat-value stat-spent">{{ usage?.total_spent ?? 0 }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">累计充值</span>
              <span class="stat-value stat-added">{{ usage?.total_added ?? 0 }}</span>
            </div>
          </div>
        </Card>
      </div>

      <!-- 消耗趋势 -->
      <Card v-if="chartData.length" class="chart-card" style="margin-top: 16px">
        <template #title><ThunderboltOutlined /> 每日消耗趋势</template>
        <div class="bar-chart">
          <div v-for="d in chartData" :key="d.day" class="bar-col" :title="`${d.day}: ${d.spent} credits`">
            <div class="bar-value">{{ d.spent > 0 ? d.spent : '' }}</div>
            <div
              class="bar"
              :style="{ height: Math.max(2, Math.round((d.spent / chartMax) * 120)) + 'px' }"
              :class="{ 'bar-zero': d.spent === 0 }"
            />
            <div class="bar-label">{{ d.label }}</div>
          </div>
        </div>
      </Card>
    </template>

    <Tabs v-model:activeKey="activeTab" style="margin-top: 20px">
      <!-- 充值 -->
      <TabPane key="purchase" tab="充值">
        <Card>
          <template #title><ShoppingOutlined /> 充值 Credits</template>

          <div class="purchase-form">
            <div class="form-item">
              <label>快捷选择</label>
              <div class="preset-row">
                <Button
                  v-for="n in PRESET_CREDITS"
                  :key="n"
                  :type="credits === n && !customCredits ? 'primary' : 'default'"
                  @click="selectPreset(n)"
                >
                  {{ n >= 1000 ? `${n / 1000}k` : n }}
                </Button>
              </div>
            </div>

            <div class="form-item">
              <label>自定义数量（credits）</label>
              <Input
                v-model:value="customCredits"
                type="number"
                min="1"
                placeholder="输入任意数量，如 3000"
                style="max-width: 240px"
              />
            </div>

            <div class="form-item">
              <label>支付方式</label>
              <Radio.Group v-model:value="provider" :options="providerOptions" option-type="button" button-style="solid" />
            </div>

            <div class="price-hint">
              {{ effectiveCredits ? `本次充值 ${effectiveCredits} credits ${priceHint}` : '请选择或输入充值数量' }}
            </div>

            <Button
              type="primary"
              size="large"
              :loading="checkoutLoading"
              :disabled="effectiveCredits <= 0"
              @click="handlePurchase"
              style="max-width: 240px"
            >
              <template #icon><WalletOutlined /></template>
              立即充值
            </Button>

            <div class="purchase-note">
              {{ provider === 'paypal'
                ? '跳转 PayPal 完成付款，1 credit = 1 美分（USD）。'
                : '扫码完成付款，1 credit = 1 分（CNY）。支付成功后 Credits 自动到账。' }}
              充值不可退款，请确认数量。
            </div>
          </div>
        </Card>
      </TabPane>

      <!-- 交易历史 -->
      <TabPane key="history" tab="交易记录">
        <Card>
          <template #title><BarChartOutlined /> 交易历史</template>
          <Spin :spinning="historyLoading">
            <Table
              v-if="history.length"
              :columns="historyColumns"
              :dataSource="history"
              row-key="id"
              :pagination="{ pageSize: 10, showSizeChanger: false, showTotal: (t: number) => `共 ${t} 条` }"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'created_at'">
                  {{ formatTime(record.created_at) }}
                </template>
                <template v-else-if="column.key === 'reason'">
                  <Tag :color="reasonInfo(record.reason).color">{{ reasonInfo(record.reason).label }}</Tag>
                </template>
                <template v-else-if="column.key === 'amount'">
                  <span :class="record.amount >= 0 ? 'amount-add' : 'amount-deduct'">
                    {{ amountText(record.amount) }}
                  </span>
                </template>
                <template v-else-if="column.key === 'balance'">
                  {{ record.balance }}
                </template>
              </template>
            </Table>
            <Empty v-else-if="!historyLoading" description="暂无交易记录" />
          </Spin>
        </Card>
      </TabPane>
    </Tabs>

    <!-- 扫码支付弹层 -->
    <Modal
      :open="qrVisible"
      :footer="null"
      :closable="true"
      :maskClosable="false"
      width="340px"
      title="扫码支付"
      @cancel="closeQR"
    >
      <div class="qr-body">
        <div class="qr-channel">
          {{ qrChannel === 'alipay' ? '支付宝' : '微信支付' }}
          <Tag color="#f59e0b">{{ effectiveCredits }} credits</Tag>
        </div>

        <Spin :spinning="payStatus === 'pending' && !qrCode" tip="生成二维码中...">
          <canvas v-show="qrCode" ref="qrCanvas" class="qr-canvas" />
        </Spin>

        <div v-if="payStatus === 'pending'" class="qr-tip">
          <QrcodeOutlined /> 请使用{{ qrChannel === 'alipay' ? '支付宝' : '微信' }}扫码完成支付
          <br />
          <span class="qr-sub">页面将自动检测支付结果，无需手动刷新</span>
        </div>
        <div v-else-if="payStatus === 'paid'" class="qr-success">
          <PayCircleOutlined /> 支付成功，Credits 已到账
        </div>
        <div v-else class="qr-expired">
          <Alert type="warning" message="订单已{{ payStatus === 'expired' ? '超时' : '失败' }}" description="请关闭后重新发起充值" />
        </div>
      </div>
    </Modal>
  </div>
</template>

<style scoped>
.billing-container { padding: 24px; max-width: 1080px; margin: 0 auto; }
.billing-header { display: flex; align-items: center; gap: 12px; margin-bottom: 20px; }
.billing-header h1 { margin: 0; font-size: 24px; font-weight: 600; }
.loading-state { display: flex; justify-content: center; padding: 60px 0; }

.overview-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
@media (max-width: 768px) { .overview-grid { grid-template-columns: 1fr; } }

.balance-display { display: flex; align-items: baseline; gap: 12px; }
.balance-amount { font-size: 44px; font-weight: 700; color: #f59e0b; line-height: 1.2; }
.free-quota { margin-top: 16px; }
.free-quota-label { display: flex; justify-content: space-between; color: #6b7280; font-size: 13px; margin-bottom: 4px; }
.free-quota-count { font-weight: 600; color: #374151; }

.usage-stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
.stat-item { text-align: center; padding: 14px 8px; background-color: #f9fafb; border-radius: 8px; }
.stat-label { display: block; color: #6b7280; font-size: 12px; margin-bottom: 6px; }
.stat-value { font-size: 22px; font-weight: 600; }
.stat-spent { color: #ef4444; }
.stat-added { color: #10b981; }

.bar-chart { display: flex; align-items: flex-end; gap: 6px; height: 170px; overflow-x: auto; padding-top: 8px; }
.bar-col { flex: 1; min-width: 26px; display: flex; flex-direction: column; align-items: center; justify-content: flex-end; height: 100%; }
.bar-value { font-size: 11px; color: #6b7280; margin-bottom: 2px; white-space: nowrap; }
.bar { width: 70%; border-radius: 3px 3px 0 0; background: linear-gradient(180deg, #f59e0b, #fbbf24); }
.bar-zero { background: #e5e7eb; }
.bar-label { font-size: 10px; color: #9ca3af; margin-top: 4px; white-space: nowrap; transform: rotate(-30deg); transform-origin: top left; }

.purchase-form { display: flex; flex-direction: column; gap: 18px; max-width: 480px; }
.form-item { display: flex; flex-direction: column; gap: 8px; }
.form-item label { font-weight: 500; color: #374151; }
.preset-row { display: flex; flex-wrap: wrap; gap: 8px; }
.price-hint { font-size: 14px; color: #6b7280; }
.purchase-note { font-size: 12px; color: #9ca3af; }

.amount-add { color: #10b981; font-weight: 600; }
.amount-deduct { color: #ef4444; font-weight: 600; }

.qr-body { text-align: center; padding: 8px 0; }
.qr-channel { margin-bottom: 14px; font-size: 15px; font-weight: 600; display: flex; align-items: center; justify-content: center; gap: 8px; }
.qr-canvas { width: 220px; height: 220px; border: 1px solid #e5e7eb; border-radius: 8px; }
.qr-tip { margin-top: 14px; color: #374151; font-size: 14px; }
.qr-sub { color: #9ca3af; font-size: 12px; }
.qr-success { margin-top: 16px; color: #10b981; font-size: 16px; font-weight: 600; }
.qr-expired { margin-top: 16px; text-align: left; }
</style>
