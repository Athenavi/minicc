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
    { required: true, message: '请输入邮�?, trigger: 'blur' },
    { type: 'email', message: '邮箱格式不正�?, trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密�?, trigger: 'blur' },
    { min: 6, message: '密码至少 6 �?, trigger: 'blur' },
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
    { len: 6, message: '验证码为 6 位数�?, trigger: 'blur' },
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
    smsError.value = '请输入正确的手机�?
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
    message.success('验证码已发�?)
    // 一次性凭据：发送后重置，登录时按需重新验证
    markCaptchaDirty()
    captchaRef.value?.reset()
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      needCaptcha.value = true
      smsError.value = '操作过于频繁，请完成人机验证后重�?
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
      smsError.value = '发送过于频繁，请稍后再�?
      return
    }
    smsError.value = apiErr || '验证码发送失�?
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
      smsError.value = '操作过于频繁，请完成人机验证后重�?
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
  // SSO 登录回跳（successURL �??sso=ok）：cookie �?Bearer token 建立本地会话
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
    // 配置接口不可达时按无验证码处理（后端仍会兜底校验�?
  }
  needCaptcha.value = captchaConfig.value.enabled
  try {
    const st = await getSmsStatus()
    smsEnabled.value = !!(st.enabled && st.login_enabled)
  } catch {
    // 短信服务状态不可达时隐藏短信登录入口（后端仍会兜底拒绝�?
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
    // owner/admin 登录后进入管理后台，普�?user 进入对话
    const role = authStore.user?.role
    router.push(role === 'admin' || role === 'owner' ? '/admin' : '/chat')
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      // 后端要求人机验证（同 IP 失败升级）→ 强制展示验证码组�?
      needCaptcha.value = true
      error.value = '操作过于频繁，请完成人机验证后重�?
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
    <!-- 左侧品牌展示（≥960px 可见�?-->
    <aside class="login-brand">
      <div class="login-brand-badge">Chiron · 企业�?AI Agent 平台</div>
      <h1 class="login-brand-title">
        �?AI Agent<br /><span>持续工作</span>
      </h1>
      <p class="login-brand-desc">
        自托管、多租户、全栈可控的 AI Agent 平台。对话、Agent、工作流、技能、知识库与插件一体化�?
      </p>
      <div class="login-brand-features">
        <div class="login-brand-feature">多租户数据隔�?/div>
        <div class="login-brand-feature">端到端轨迹追�?/div>
        <div class="login-brand-feature">MCP 插件生�?/div>
        <div class="login-brand-feature">HTTPOnly 安全会话</div>
      </div>
    </aside>

    <!-- 右侧登录表单 -->
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">MC</div>
        <div class="login-title">欢迎回来</div>
        <div class="login-subtitle">登录进入你的 AI 工作�?/div>
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
                placeholder="请输入邮�?
                size="large"
                aria-label="邮箱"
                autocomplete="email"
              >
                <template #prefix><MailOutlined /></template>
              </Input>
            </FormItem>

            <FormItem label="密码" name="password">
              <Input
                v-model:value="form.password"
                placeholder="请输入密�?
                type="password"
                size="large"
                aria-label="密码"
                autocomplete="current-password"
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
                  没有账号？注�?
                </Button>
                <Button type="link" block @click="router.push('/install')">
                  首次部署？初始化系统
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
            <FormItem label="手机�? name="phone">
              <Input
                v-model:value="smsForm.phone"
                placeholder="请输入手机号"
                size="large"
                :maxlength="21"
              >
                <template #prefix><MobileOutlined /></template>
              </Input>
            </FormItem>

            <FormItem label="验证�? name="code">
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
                    {{ remaining > 0 ? `${remaining}s 后重发` : '获取验证�? }}
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
.login-container {
  display: grid;
  grid-template-columns: 1fr;
  min-height: 100dvh;
  background: var(--bg-page);
  position: relative;
  overflow: hidden;
}

.login-container::before {
  content: '';
  position: absolute;
  inset: -20%;
  background:
    radial-gradient(ellipse 60% 50% at 20% 20%, var(--primary-bg), transparent 60%),
    radial-gradient(ellipse 50% 45% at 85% 15%, var(--accent-bg), transparent 60%),
    radial-gradient(ellipse 55% 50% at 50% 95%, var(--info-bg), transparent 65%);
  pointer-events: none;
  filter: blur(40px);
  opacity: 0.7;
}

.login-container::after {
  content: '';
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--border-subtle) 1px, transparent 1px),
    linear-gradient(90deg, var(--border-subtle) 1px, transparent 1px);
  background-size: 32px 32px;
  mask-image: radial-gradient(ellipse 70% 70% at 50% 40%, black 15%, transparent 75%);
  -webkit-mask-image: radial-gradient(ellipse 70% 70% at 50% 40%, black 15%, transparent 75%);
  pointer-events: none;
  opacity: 0.5;
}

.login-card {
  width: 420px;
  max-width: calc(100vw - 32px);
  margin: auto;
  position: relative;
  z-index: 1;
  padding: 32px 36px;
  border-radius: var(--radius-2xl);
  background: var(--bg-surface);
  border: 1px solid var(--border-default);
  box-shadow: var(--shadow-lg), 0 0 0 1px var(--border-subtle) inset;
  animation: loginFadeIn 0.5s ease;
}

.login-header {
  text-align: center;
  margin-bottom: 28px;
}

.login-logo {
  width: 52px;
  height: 52px;
  margin: 0 auto 16px;
  border-radius: 14px;
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #fff;
  font-weight: 700;
  font-size: 18px;
  letter-spacing: 0.02em;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-md), 0 6px 20px var(--primary-bg);
}

.login-title {
  font-size: 24px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.login-subtitle {
  font-size: 13px;
  color: var(--text-tertiary);
  margin-top: 6px;
}

.login-form-card {
  background: transparent !important;
  border: none !important;
  box-shadow: none !important;
  padding: 0 !important;
}
.login-form-card :deep(.ant-card-body) { padding: 0; }

.login-form-card :deep(.ant-form-item-label > label) {
  font-weight: 500;
  color: var(--text-secondary);
  font-size: 13px;
  height: 24px;
}

.login-form-card :deep(.ant-input),
.login-form-card :deep(.ant-input-affix-wrapper) {
  font-size: 14px;
  transition: border-color var(--dur-fast) var(--ease-out),
              box-shadow var(--dur-fast) var(--ease-out);
}

.login-form-card :deep(.ant-btn-primary) {
  height: 44px;
  font-weight: 600;
  font-size: 14px;
  letter-spacing: 0.01em;
}

.login-form-card :deep(.ant-tabs-tab) {
  font-size: 13px;
  font-weight: 500;
}
.login-form-card :deep(.ant-tabs-tab-active .ant-tabs-tab-btn) {
  font-weight: 600;
}

@keyframes loginFadeIn {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 平板以上：左右分栏品牌展�?*/
@media (min-width: 960px) {
  .login-container {
    grid-template-columns: 1.1fr 1fr;
  }
  .login-brand {
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: 64px 56px;
    position: relative;
    z-index: 1;
  }
  .login-brand-badge {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 5px 12px;
    border-radius: var(--radius-full);
    background: var(--primary-bg);
    color: var(--primary);
    font-size: 12px;
    font-weight: 600;
    width: fit-content;
    margin-bottom: 24px;
  }
  .login-brand-badge::before {
    content: '';
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--primary);
    box-shadow: 0 0 8px var(--primary);
  }
  .login-brand-title {
    font-size: clamp(36px, 4.5vw, 52px);
    font-weight: 700;
    line-height: 1.1;
    letter-spacing: -0.02em;
    color: var(--text-primary);
    margin-bottom: 16px;
  }
  .login-brand-title span {
    background: linear-gradient(100deg, var(--primary), var(--accent));
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
  }
  .login-brand-desc {
    font-size: 15px;
    line-height: 1.7;
    color: var(--text-secondary);
    max-width: 460px;
  }
  .login-brand-features {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 14px 28px;
    margin-top: 32px;
    max-width: 460px;
  }
  .login-brand-feature {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    font-size: 13px;
    color: var(--text-secondary);
  }
  .login-brand-feature::before {
    content: '�?;
    color: var(--success);
    font-weight: 700;
    flex-shrink: 0;
  }
}

@media (max-width: 959px) {
  .login-brand { display: none; }
}

/* 移动�?*/
@media (max-width: 576px) {
  .login-card {
    padding: 24px 20px;
    border-radius: var(--radius-xl);
  }
  .login-logo { width: 44px; height: 44px; font-size: 16px; }
  .login-title { font-size: 20px; }
  .login-subtitle { font-size: 12px; }
  .login-header { margin-bottom: 20px; }
  .login-form-card :deep(.ant-input) { font-size: 16px; }
  .login-form-card :deep(.ant-btn:not(.ant-btn-sm)) { min-height: 44px; }
}

/* 焦点可见�?*/
.login-form-card :deep(.ant-input:focus),
.login-form-card :deep(.ant-input-affix-wrapper-focused),
.login-form-card :deep(.ant-btn:focus-visible) {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .login-card { animation: none; }
}
</style>
