<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NButton, NInput, NForm, NFormItem, NIcon, useMessage } from 'naive-ui'
import { PersonOutline } from '@vicons/ionicons5'
import { useAuthStore } from '../stores/auth'
import { api } from '../api'

const message = useMessage()
const authStore = useAuthStore()
const loading = ref(false)
const form = ref({
  name: '',
  email: '',
})

onMounted(async () => {
  if (authStore.user) {
    form.value.name = authStore.user.name || ''
    form.value.email = authStore.user.email || ''
  }
})

async function handleUpdateProfile() {
  loading.value = true
  try {
    await api.put('/v1/profile', form.value)
    message.success('个人信息已更新')
    await authStore.fetchProfile()
  } catch (error: any) {
    message.error(error.message || '更新失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="profile-container">
    <div class="profile-header">
      <NIcon size="24" color="#2080f0">
        <PersonOutline />
      </NIcon>
      <h1>个人资料</h1>
    </div>

    <NCard class="profile-card">
      <NForm label-placement="left" label-width="80">
        <NFormItem label="姓名">
          <NInput v-model:value="form.name" placeholder="请输入姓名" />
        </NFormItem>
        <NFormItem label="邮箱">
          <NInput v-model:value="form.email" placeholder="请输入邮箱" disabled />
        </NFormItem>
        <NFormItem>
          <NButton type="primary" :loading="loading" @click="handleUpdateProfile">
            保存修改
          </NButton>
        </NFormItem>
      </NForm>
    </NCard>
  </div>
</template>

<style scoped>
.profile-container {
  padding: 24px;
  max-width: 600px;
}

.profile-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
}

.profile-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}
</style>
