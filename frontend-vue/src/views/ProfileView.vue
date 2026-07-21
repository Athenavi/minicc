<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Card, Button, Input, Form, FormItem, message } from 'ant-design-vue'
import { UserOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
import { api } from '../api'

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
      <UserOutlined style="font-size: 24px; color: var(--primary)" />
      <h1>个人资料</h1>
    </div>

    <Card class="profile-card">
      <Form :model="form" layout="vertical">
        <FormItem label="姓名">
          <Input v-model:value="form.name" placeholder="请输入姓名" />
        </FormItem>
        <FormItem label="邮箱">
          <Input v-model:value="form.email" placeholder="请输入邮箱" disabled />
        </FormItem>
        <FormItem>
          <Button type="primary" :loading="loading" @click="handleUpdateProfile">
            保存修改
          </Button>
        </FormItem>
      </Form>
    </Card>
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
