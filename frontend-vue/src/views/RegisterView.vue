<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Card, Form, FormItem, Input, Button, Alert, Space } from 'ant-design-vue'
import { MailOutlined, LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
import { getCaptchaPublicConfig } from '../api/auth'
import CaptchaWidget from '../components/CaptchaWidget.vue'
import type { Rule } from 'ant-design-vue/es/form'

const router = useRouter()
const authStore = useAuthStore()

const formRef = ref()
const form = ref({
  name: '',
  email: '',
  password: '',
  confirmPassword: '',
})

const rules: Record<string, Rule[]> = {
  name: [{ required: true, message: '请输入姓名', trigger: 'blur' }],
  email: [
    { required: true, message: '请输入邮箱', trigger: 'blur' },
    { type: 'email', message: '邮箱格式不正确', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 8, message: '密码至少 8 位', trigger: 'blur' },
  ],
  confirmPassword: [
    { required: true, message: '请确认密码', trigger: 'blur' },
    {
      validator: (_rule: Rule, value: string) => {
        if (value !== form.value.password) {
          return Promise.reject('两次密码输入不一致')
        }
        return Promise.resolve()
      },
      trigger: 'blur',
    },
  ],
}

const error = ref('')

// ── 人机验证（注册接口防刷）──
const captchaConfig = ref({ enabled: false, provider: '', site_key: '', verify_url: '' })
const captchaRequired = ref(false)
const captchaToken = ref('')
const captchaRandstr = ref('')
const captchaRef = ref<InstanceType<typeof CaptchaWidget>>()

function markCaptchaDirty() {
  captchaToken.value = ''
  captchaRandstr.value = ''
}

onMounted(async () => {
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
  captchaRequired.value = captchaConfig.value.enabled
})

async function handleRegister() {
  error.value = ''
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (captchaRequired.value && captchaConfig.value.provider !== 'custom' && !captchaToken.value) {
    error.value = '请先完成人机验证'
    return
  }
  try {
    await authStore.register(form.value.email, form.value.password, form.value.name, {
      token: captchaToken.value,
      randstr: captchaRandstr.value,
    })
    router.push('/chat')
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 428 || apiErr === 'captcha_required') {
      captchaRequired.value = true
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
    error.value = apiErr || '注册失败'
  }
}
</script>

<template>
  <div class="register-container">
    <div class="register-card">
      <div class="register-header">
        <div class="register-logo">MC</div>
        <div class="register-title">创建账号</div>
        <div class="register-subtitle">加入 MiniCC AI Agent 平台</div>
      </div>
      <Card :bordered="false" class="register-form-card">
        <Alert v-if="error" type="error" :message="error" show-icon style="margin-bottom: 16px" />

        <Form
          ref="formRef"
          :model="form"
          :rules="rules"
          @finish="handleRegister"
          layout="vertical"
        >
          <FormItem label="姓名" name="name">
            <Input v-model:value="form.name" placeholder="请输入姓名" size="large">
              <template #prefix><UserOutlined /></template>
            </Input>
          </FormItem>

          <FormItem label="邮箱" name="email">
            <Input v-model:value="form.email" placeholder="请输入邮箱" size="large">
              <template #prefix><MailOutlined /></template>
            </Input>
          </FormItem>

          <FormItem label="密码" name="password">
            <Input
              v-model:value="form.password"
              placeholder="请输入密码（至少8位）"
              type="password"
              size="large"
            >
              <template #prefix><LockOutlined /></template>
            </Input>
          </FormItem>

          <FormItem label="确认密码" name="confirmPassword">
            <Input
              v-model:value="form.confirmPassword"
              placeholder="请再次输入密码"
              type="password"
              size="large"
            >
              <template #prefix><LockOutlined /></template>
            </Input>
          </FormItem>

          <FormItem v-if="captchaRequired" label="人机验证">
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
                注册
              </Button>
              <Button type="link" block @click="router.push('/login')">
                已有账号？登录
              </Button>
            </Space>
          </FormItem>
        </Form>
      </Card>
    </div>
  </div>
</template>

<style scoped>
/* 中性底色 + 顶部微弱 accent 光晕（与登录页一致，去 AI 紫渐变） */
.register-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100dvh;
  background: var(--bg-page);
  position: relative;
  overflow: hidden;
}

.register-container::before {
  content: '';
  position: absolute;
  inset: -45% -20% auto -20%;
  height: 60%;
  background: radial-gradient(ellipse 55% 55% at 50% 0%, var(--primary-bg), transparent 72%);
  pointer-events: none;
}

.register-card {
  width: 420px;
  max-width: calc(100vw - 32px);
  position: relative;
  z-index: 1;
  animation: registerFadeIn 0.5s ease;
}

.register-form-card {
  border-radius: var(--radius-lg) !important;
  border: 1px solid var(--border-card);
  box-shadow: var(--shadow-lg);
  background: var(--bg-card);
}

.register-header {
  text-align: center;
  margin-bottom: 24px;
}

.register-logo {
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

.register-title {
  font-size: 22px;
  font-weight: 650;
  color: var(--text-primary);
  letter-spacing: -0.01em;
}

.register-subtitle {
  font-size: 14px;
  color: var(--text-tertiary);
  margin-top: 4px;
}

@keyframes registerFadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
