<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Card, Form, FormItem, Input, Button, Alert, Space, Tabs, TabPane, message } from 'ant-design-vue'
import { MailOutlined, LockOutlined, MobileOutlined, SafetyOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
import { getCaptchaPublicConfig, getSmsStatus, sendSmsCode, smsLogin, isValidPhone } from '../api/auth'
import { useSmsCountdown } from '../composables/useSmsCountdown'
import CaptchaWidget from '../components/CaptchaWidget.vue'
import SsoLoginButtons from '../components/SsoLoginButtons.vue'
import type { Rule } from 'ant-design-vue/es/form'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

// ── 密码登录 ──

const formRef = ref()
const form = ref({
  email: '',
  password: '',
})

const rules: Record<string, Rule[]> = {
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email', message: '邮箱格式不正确', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 位', trigger: 'blur' },
  ],
}

const error = ref('')

// ── 人机验证（管理员启用或同 IP 失败升级时要求；两个标签页共用）──

const captchaConfig = ref({ enabled: false, provider: '', site_key: '', verify_url: '' })
const captchaToken = ref('')
const captchaRandstr = ref('')
const captchaRef = ref<InstanceType<typeof CaptchaWidget>>()

const needCaptcha = ref(false)

function markCaptchaDirty() {
  captchaToken.value = ''
  captchaRandstr.value = ''
}

// ── 短信登录 ──

const smsEnabled = ref(false)
const activeTab = ref('password')

const smsFormRef = ref()
const smsForm = ref({ phone: '', code: '' })

const smsRules: Record<string, Rule[]> = {
  phone: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    {
      validator: (_rule: Rule, value: string) =>
        !value || isValidPhone(value) ? Promise.resolve() : Promise.reject('手机号格式不正确'),
      trigger: 'blur',
    },
  ],
  code: [
    { required: true, message: '请输入验证码', trigger: 'blur' },
    { len: 6, message: '验证码为 6 位数字', trigger: 'blur' },
  ],
}

const { remaining, start } = useSmsCountdown(60)
const sending = ref(false)
const smsLoading = ref(false)
const smsError = ref('')

async function handleSendCode() {
  smsError.value = ''
  const phone = smsForm.value.phone.trim()
  if (!isValidPhone(phone)) {
    smsError.value = '请输入正确的手机号'
    return
  }
  if (needCaptcha.value && captchaConfig.value.provider !== 'custom' && !captchaToken.value) {
    smsError.value = '请先完成人机验证'
    return
  }
  sending.value = true
  try {
    const res = await sendSmsCode({
      phone,
      purpose: 'login',
      captcha_token: captchaToken.value,
      captcha_randstr: captchaRandstr.value,
    })
    start(res.interval || 60)
    message.success('验证码已发送')
    // 一次性凭据：发送后重置，登录时按需重新验证
    markCaptchaDirty()
    captchaRef.value?.reset()
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      needCaptcha.value = true
      smsError.value = '操作过于频繁，请完成人机验证后重试'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    if (status === 403 && String(apiErr).includes('captcha')) {
      smsError.value = '人机验证未通过，请重新验证'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    if (status === 429) {
      smsError.value = '发送过于频繁，请稍后再试'
      return
    }
    smsError.value = apiErr || '验证码发送失败'
  } finally {
    sending.value = false
  }
}

async function handleSmsLogin() {
  smsError.value = ''
  try {
    await smsFormRef.value?.validate()
  } catch {
    return
  }
  if (needCaptcha.value && captchaConfig.value.provider !== 'custom' && !captchaToken.value) {
    smsError.value = '请先完成人机验证'
    return
  }
  smsLoading.value = true
  try {
    const { token, user } = await smsLogin({
      phone: smsForm.value.phone.trim(),
      code: smsForm.value.code.trim(),
      captcha_token: captchaToken.value,
      captcha_randstr: captchaRandstr.value,
    })
    authStore.applySession(token, user)
    router.push('/chat')
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      needCaptcha.value = true
      smsError.value = '操作过于频繁，请完成人机验证后重试'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    if (status === 403 && String(apiErr).includes('captcha')) {
      smsError.value = '人机验证未通过，请重新验证'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    smsError.value = apiErr || '登录失败'
  } finally {
    smsLoading.value = false
  }
}

onMounted(async () => {
  // SSO 登录回跳（successURL 带 ?sso=ok）：cookie 换 Bearer token 建立本地会话
  if (route.query.sso === 'ok' && !authStore.token) {
    const ok = await authStore.bootstrapSession()
    if (ok) {
      router.push('/chat')
      return
    }
  }
  try {
    const cfg = await getCaptchaPublicConfig()
    captchaConfig.value = {
      enabled: !!cfg.enabled,
      provider: cfg.provider || '',
      site_key: cfg.site_key || '',
      verify_url: cfg.verify_url || '',
    }
  } catch {
    // 配置接口不可达时按无验证码处理（后端仍会兜底校验）
  }
  needCaptcha.value = captchaConfig.value.enabled
  try {
    const st = await getSmsStatus()
    smsEnabled.value = !!(st.enabled && st.login_enabled)
  } catch {
    // 短信服务状态不可达时隐藏短信登录入口（后端仍会兜底拒绝）
  }
})

async function handleLogin() {
  error.value = ''
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (needCaptcha.value && captchaConfig.value.provider !== 'custom' && !captchaToken.value) {
    error.value = '请先完成人机验证'
    return
  }
  try {
    await authStore.login(form.value.email, form.value.password, {
      token: captchaToken.value,
      randstr: captchaRandstr.value,
    })
    router.push('/chat')
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      // 后端要求人机验证（同 IP 失败升级）→ 强制展示验证码组件
      needCaptcha.value = true
      error.value = '操作过于频繁，请完成人机验证后重试'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    if (status === 403 && String(apiErr).includes('captcha')) {
      error.value = '人机验证未通过，请重新验证'
      captchaRef.value?.reset()
      markCaptchaDirty()
      return
    }
    error.value = apiErr || '登录失败'
  }
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">MC</div>
        <div class="login-title">MiniCC</div>
        <div class="login-subtitle">企业级 AI Agent 平台</div>
      </div>
      <Card :bordered="false" class="login-form-card">
        <Tabs v-model:activeKey="activeTab" centered>
          <TabPane key="password" tab="密码登录" />

          <TabPane v-if="smsEnabled" key="sms" tab="短信登录" />
        </Tabs>

        <!-- 密码登录 -->
        <template v-if="activeTab === 'password'">
          <Alert v-if="error" type="error" :message="error" show-icon style="margin-bottom: 16px" />

          <Form
            ref="formRef"
            :model="form"
            :rules="rules"
            @finish="handleLogin"
            layout="vertical"
          >
            <FormItem label="邮箱" name="email">
              <Input
                v-model:value="form.email"
                placeholder="请输入邮箱"
                size="large"
              >
                <template #prefix><MailOutlined /></template>
              </Input>
            </FormItem>

            <FormItem label="密码" name="password">
              <Input
                v-model:value="form.password"
                placeholder="请输入密码"
                type="password"
                size="large"
              >
                <template #prefix><LockOutlined /></template>
              </Input>
            </FormItem>

            <FormItem v-if="needCaptcha" label="人机验证">
              <CaptchaWidget
                ref="captchaRef"
                :provider="captchaConfig.provider"
                :site-key="captchaConfig.site_key"
                :verify-url="captchaConfig.verify_url"
                @verified="(p: any) => { captchaToken = p.token; captchaRandstr = p.randstr || '' }"
                @expired="markCaptchaDirty"
              />
            </FormItem>

            <FormItem>
              <Space direction="vertical" style="width: 100%">
                <Button type="primary" html-type="submit" block :loading="authStore.loading" size="large">
                  登录
                </Button>
                <Button type="link" block @click="router.push('/register')">
                  没有账号？注册
                </Button>
              </Space>
            </FormItem>
          </Form>
        </template>

        <!-- 短信登录 -->
        <template v-else>
          <Alert v-if="smsError" type="error" :message="smsError" show-icon style="margin-bottom: 16px" />

          <Form
            ref="smsFormRef"
            :model="smsForm"
            :rules="smsRules"
            layout="vertical"
          >
            <FormItem label="手机号" name="phone">
              <Input
                v-model:value="smsForm.phone"
                placeholder="请输入手机号"
                size="large"
                :maxlength="21"
              >
                <template #prefix><MobileOutlined /></template>
              </Input>
            </FormItem>

            <FormItem label="验证码" name="code">
              <Input
                v-model:value="smsForm.code"
                placeholder="6 位数字验证码"
                size="large"
                :maxlength="6"
              >
                <template #prefix><SafetyOutlined /></template>
                <template #suffix>
                  <Button
                    size="small"
                    type="link"
                    :disabled="remaining > 0 || sending"
                    :loading="sending"
                    @click="handleSendCode"
                  >
                    {{ remaining > 0 ? `${remaining}s 后重发` : '获取验证码' }}
                  </Button>
                </template>
              </Input>
            </FormItem>

            <FormItem v-if="needCaptcha" label="人机验证">
              <CaptchaWidget
                ref="captchaRef"
                :provider="captchaConfig.provider"
                :site-key="captchaConfig.site_key"
                :verify-url="captchaConfig.verify_url"
                @verified="(p: any) => { captchaToken = p.token; captchaRandstr = p.randstr || '' }"
                @expired="markCaptchaDirty"
              />
            </FormItem>

            <FormItem>
              <Button
                type="primary"
                block
                size="large"
                :loading="smsLoading"
                @click="handleSmsLogin"
              >
                登录
              </Button>
            </FormItem>
          </Form>
        </template>

        <SsoLoginButtons />
      </Card>
    </div>
  </div>
</template>

<style scoped>
/* 中性底色 + 顶部微弱 accent 光晕（克制，去 AI 紫渐变） */
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100dvh;
  background: var(--bg-page);
  position: relative;
  overflow: hidden;
}

.login-container::before {
  content: '';
  position: absolute;
  inset: -45% -20% auto -20%;
  height: 60%;
  background: radial-gradient(ellipse 55% 55% at 50% 0%, var(--primary-bg), transparent 72%);
  pointer-events: none;
}

.login-card {
  width: 400px;
  max-width: calc(100vw - 32px);
  position: relative;
  z-index: 1;
  animation: loginFadeIn 0.5s ease;
}

.login-form-card {
  border-radius: var(--radius-lg) !important;
  border: 1px solid var(--border-card);
  box-shadow: var(--shadow-lg);
  background: var(--bg-card);
}

.login-header {
  text-align: center;
  margin-bottom: 24px;
}

.login-logo {
  width: 44px;
  height: 44px;
  margin: 0 auto 14px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #fff;
  font-weight: 700;
  font-size: 16px;
  letter-spacing: 0.02em;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-md);
}

.login-title {
  font-size: 22px;
  font-weight: 650;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.login-subtitle {
  font-size: 14px;
  color: var(--text-tertiary);
  margin-top: 4px;
}

@keyframes loginFadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
