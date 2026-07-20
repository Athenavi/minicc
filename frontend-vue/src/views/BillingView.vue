<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NButton, NBadge, NSpin, NInput, NSelect, NTabs, NTabPane, NIcon, NDataTable, useMessage } from 'naive-ui'
import { CardOutline, WalletOutline, TrendingDownOutline, BarChartOutline } from '@vicons/ionicons5'
import { api } from '../api'

interface CreditTx {
  id: string
  amount: number
  balance: number
  reason: string
  created_at: string
}

const message = useMessage()
const balance = ref<number | null>(null)
const history = ref<CreditTx[]>([])
const usage = ref<any>(null)
const credits = ref('1000')
const provider = ref('stripe')
const loading = ref(true)
const checkoutLoading = ref(false)
const activeTab = ref('balance')

const providerOptions = [
  { label: 'Card / Alipay / WeChat', value: 'stripe' },
  { label: 'PayPal', value: 'paypal' },
]

const historyColumns = [
  { title: '时间', key: 'created_at', width: 180 },
  { title: '原因', key: 'reason', width: 200 },
  { title: '金额', key: 'amount', width: 100 },
  { title: '余额', key: 'balance', width: 100 },
]

onMounted(async () => {
  await loadBillingData()
})

async function loadBillingData() {
  try {
    loading.value = true
    const [balRes, histRes, usageRes] = await Promise.all([
      api.get('/v1/billing/balance'),
      api.get('/v1/billing/history'),
      api.get('/v1/billing/usage'),
    ])
    balance.value = balRes.data?.data?.balance ?? 0
    history.value = histRes.data?.data?.history ?? []
    usage.value = usageRes.data?.data ?? null
  } catch (error: any) {
    message.error(error.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function handlePurchase() {
  checkoutLoading.value = true
  try {
    const response = await api.post('/v1/billing/create-checkout-session', {
      credits: parseInt(credits.value) || 1000,
      provider: provider.value,
    })
    const checkoutUrl = response.data?.data?.checkout_url
    if (checkoutUrl) {
      window.location.href = checkoutUrl
    } else {
      throw new Error('未获取到支付链接')
    }
  } catch (error: any) {
    message.error(error.message || '创建支付会话失败')
  } finally {
    checkoutLoading.value = false
  }
}
</script>

<template>
  <div class="billing-container">
    <div class="billing-header">
      <NIcon size="24" color="#f59e0b">
        <CardOutline />
      </NIcon>
      <h1>计费管理</h1>
    </div>

    <NTabs v-model:value="activeTab" type="line">
      <NTabPane name="balance" tab="余额">
        <NCard class="balance-card">
          <div v-if="loading" class="loading-state">
            <NSpin :show="true" />
          </div>
          <div v-else class="balance-display">
            <div class="balance-amount">{{ balance ?? 0 }}</div>
            <NBadge value="credits" color="#f59e0b" />
          </div>
        </NCard>

        <NCard v-if="usage" class="usage-card" style="margin-top: 16px">
          <template #header>
            <div class="card-header">
              <NIcon><TrendingDownOutline /></NIcon>
              <span>使用统计</span>
            </div>
          </template>
          <div class="usage-stats">
            <div class="stat-item">
              <span class="stat-label">今日使用</span>
              <span class="stat-value">{{ usage.today ?? 0 }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">本月使用</span>
              <span class="stat-value">{{ usage.month ?? 0 }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">总计使用</span>
              <span class="stat-value">{{ usage.total ?? 0 }}</span>
            </div>
          </div>
        </NCard>
      </NTabPane>

      <NTabPane name="purchase" tab="充值">
        <NCard class="purchase-card">
          <template #header>
            <div class="card-header">
              <NIcon><CardOutline /></NIcon>
              <span>充值 Credits</span>
            </div>
          </template>

          <div class="purchase-form">
            <div class="form-item">
              <label>充值数量</label>
              <NInput
                v-model:value="credits"
                placeholder="1000"
              />
            </div>

            <div class="form-item">
              <label>支付方式</label>
              <NSelect
                v-model:value="provider"
                :options="providerOptions"
              />
            </div>

            <NButton
              type="primary"
              size="large"
              block
              :loading="checkoutLoading"
              @click="handlePurchase"
            >
              <template #icon>
                <NIcon><WalletOutline /></NIcon>
              </template>
              立即充值
            </NButton>
          </div>
        </NCard>
      </NTabPane>

      <NTabPane name="history" tab="交易记录">
        <NCard>
          <template #header>
            <div class="card-header">
              <NIcon><BarChartOutline /></NIcon>
              <span>交易历史</span>
            </div>
          </template>

          <NDataTable
            :columns="historyColumns"
            :data="history"
            :bordered="false"
            :single-line="false"
          />
        </NCard>
      </NTabPane>
    </NTabs>
  </div>
</template>

<style scoped>
.billing-container {
  padding: 24px;
}

.billing-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.billing-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}

.loading-state {
  display: flex;
  justify-content: center;
  padding: 40px 0;
}

.balance-display {
  display: flex;
  align-items: center;
  gap: 16px;
}

.balance-amount {
  font-size: 48px;
  font-weight: 700;
  color: #f59e0b;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.usage-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.stat-item {
  text-align: center;
  padding: 16px;
  background-color: #f9fafb;
  border-radius: 8px;
}

.stat-label {
  display: block;
  color: #6b7280;
  font-size: 12px;
  margin-bottom: 8px;
}

.stat-value {
  font-size: 24px;
  font-weight: 600;
}

.purchase-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-item label {
  font-weight: 500;
}
</style>
