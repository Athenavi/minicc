<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Card, Form, FormItem, Input, Button, Alert, Space } from 'ant-design-vue'
import { MailOutlined, LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
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

async function handleRegister() {
  error.value = ''
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  try {
    await authStore.register(form.value.email, form.value.password, form.value.name)
    router.push('/chat')
  } catch (e: any) {
    error.value = e.response?.data?.error || '注册失败'
  }
}
</script>

<template>
  <div class="register-container">
    <div class="register-card">
      <div class="register-header">
        <div class="register-logo">🤖</div>
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
.register-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  position: relative;
  overflow: hidden;
}

.register-container::before {
  content: '';
  position: absolute;
  width: 500px;
  height: 500px;
  border-radius: 50%;
  background: rgba(255,255,255,0.05);
  top: -150px;
  left: -150px;
}

.register-container::after {
  content: '';
  position: absolute;
  width: 350px;
  height: 350px;
  border-radius: 50%;
  background: rgba(255,255,255,0.05);
  bottom: -80px;
  right: -80px;
}

.register-card {
  width: 420px;
  position: relative;
  z-index: 1;
  animation: registerFadeIn 0.5s ease;
}

.register-form-card {
  border-radius: var(--radius-lg) !important;
  box-shadow: var(--shadow-lg, 0 8px 24px rgba(0, 0, 0, 0.5));
}

.register-header {
  text-align: center;
  margin-bottom: 24px;
}

.register-logo {
  font-size: 32px;
  margin-bottom: 8px;
}

.register-title {
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--primary), var(--accent));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.register-subtitle {
  font-size: 14px;
  color: var(--text-tertiary, #808090);
  margin-top: 4px;
}

@keyframes registerFadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
