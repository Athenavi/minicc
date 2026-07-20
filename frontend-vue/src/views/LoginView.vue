<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, NSpace, NAlert } from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const form = ref({
  email: '',
  password: '',
})

const error = ref('')

async function handleLogin() {
  error.value = ''

  if (!form.value.email || !form.value.password) {
    error.value = '请输入邮箱和密码'
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
    <NCard title="登录" style="width: 400px">
      <NAlert v-if="error" type="error" style="margin-bottom: 16px">
        {{ error }}
      </NAlert>

      <NForm @submit.prevent="handleLogin">
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
  </div>
</template>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background-color: #f5f5f5;
}
</style>
