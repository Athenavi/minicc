<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NForm, NFormItem, NInput, NButton, NSpace, NAlert } from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const form = ref({
  name: '',
  email: '',
  password: '',
  confirmPassword: '',
})

const error = ref('')

async function handleRegister() {
  error.value = ''

  if (!form.value.name || !form.value.email || !form.value.password) {
    error.value = '请填写所有必填字段'
    return
  }

  if (form.value.password !== form.value.confirmPassword) {
    error.value = '两次密码输入不一致'
    return
  }

  if (form.value.password.length < 8) {
    error.value = '密码长度至少8位'
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
    <NCard title="注册" style="width: 400px">
      <NAlert v-if="error" type="error" style="margin-bottom: 16px">
        {{ error }}
      </NAlert>

      <NForm @submit.prevent="handleRegister">
        <NFormItem label="姓名">
          <NInput
            v-model:value="form.name"
            placeholder="请输入姓名"
          />
        </NFormItem>

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
            placeholder="请输入密码（至少8位）"
            type="password"
            show-password-on="click"
          />
        </NFormItem>

        <NFormItem label="确认密码">
          <NInput
            v-model:value="form.confirmPassword"
            placeholder="请再次输入密码"
            type="password"
            show-password-on="click"
          />
        </NFormItem>

        <NSpace vertical style="width: 100%">
          <NButton
            type="primary"
            block
            :loading="authStore.loading"
            @click="handleRegister"
          >
            注册
          </NButton>

          <NButton text @click="router.push('/login')">
            已有账号？登录
          </NButton>
        </NSpace>
      </NForm>
    </NCard>
  </div>
</template>

<style scoped>
.register-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background-color: #f5f5f5;
}
</style>
