<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Card, Form, FormItem, Input, Button, Alert, Space } from 'ant-design-vue'
import { MailOutlined, LockOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
import type { Rule } from 'ant-design-vue/es/form'

const router = useRouter()
const authStore = useAuthStore()

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

async function handleLogin() {
  error.value = ''
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  try {
    await authStore.login(form.value.email, form.value.password)
    router.push('/chat')
  } catch (e: any) {
    error.value = e.response?.data?.error || '登录失败'
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
