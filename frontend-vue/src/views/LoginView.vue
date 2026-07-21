<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, NSpace, NAlert } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const formRef = ref<FormInst | null>(null)
const form = ref({
  email: '',
  password: '',
})

const rules: FormRules = {
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
        <div class="login-logo">🤖</div>
        <div class="login-title">MiniCC</div>
        <div class="login-subtitle">企业级 AI Agent 平台</div>
      </div>
      <NCard :bordered="false">
        <NAlert v-if="error" type="error" style="margin-bottom: 16px">
          {{ error }}
        </NAlert>

      <NForm ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
        <NFormItem label="邮箱">
          <NInput
            v-model:value="form.email"
            placeholder="请输入邮箱"
            type="text"
          />
        </NFormItem>

        <NFormItem label="密码">
          <NInput
            v-model:value="form.password"
            placeholder="请输入密码"
            type="password"
            show-password-on="click"
          />
        </NFormItem>

        <NSpace vertical style="width: 100%">
          <NButton
            type="primary"
            block
            :loading="authStore.loading"
            @click="handleLogin"
          >
            登录
          </NButton>

          <NButton text @click="router.push('/register')">
            没有账号？注册
          </NButton>
        </NSpace>
      </NForm>
    </NCard>
    </div> <!-- login-card -->
  </div> <!-- login-container -->
</template>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  position: relative;
  overflow: hidden;
}

.login-container::before {
  content: '';
  position: absolute;
  width: 600px;
  height: 600px;
  border-radius: 50%;
  background: rgba(255,255,255,0.05);
  top: -200px;
  right: -200px;
}

.login-container::after {
  content: '';
  position: absolute;
  width: 400px;
  height: 400px;
  border-radius: 50%;
  background: rgba(255,255,255,0.05);
  bottom: -100px;
  left: -100px;
}

.login-card {
  width: 400px;
  position: relative;
  z-index: 1;
  animation: loginFadeIn 0.5s ease;
}

.login-card :deep(.n-card) {
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
}

.login-header {
  text-align: center;
  margin-bottom: 24px;
}

.login-logo {
  font-size: 32px;
  margin-bottom: 8px;
}

.login-title {
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--primary), var(--accent));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
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
