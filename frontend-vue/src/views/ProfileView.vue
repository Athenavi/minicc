<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Card, Button, Input, Form, FormItem, message, Popconfirm, Tag, Empty, Spin } from 'ant-design-vue'
import { UserOutlined, SafetyOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'
import { api } from '../api'
import {
  listIdentities,
  deleteIdentity,
  setPassword,
  listPublicSsoProviders,
  ssoLoginURL,
  type UserIdentity,
  type SsoPublicProvider,
} from '../api/auth'

const route = useRoute()
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
  // 绑定成功回跳（bindURL=/profile?bind=ok）
  if (route.query.bind === 'ok') {
    message.success('三方账号绑定成功')
  }
  loadBindings()
})

async function handleUpdateProfile() {
  loading.value = true
  try {
    await api.put('/v1/auth/profile', form.value)
    message.success('个人信息已更新')
    await authStore.fetchProfile()
  } catch (error: any) {
    message.error(error.message || '更新失败')
  } finally {
    loading.value = false
  }
}

// ── 三方账号绑定 ──

const identities = ref<UserIdentity[]>([])
const bindable = ref<SsoPublicProvider[]>([])
const bindingsLoading = ref(false)
const unbinding = ref('')

async function loadBindings() {
  bindingsLoading.value = true
  try {
    const [ids, providers] = await Promise.all([
      listIdentities(),
      listPublicSsoProviders().catch(() => [] as SsoPublicProvider[]),
    ])
    identities.value = ids
    // 已绑定的 provider 不再出现在可绑定列表
    const boundNames = new Set(ids.map(i => i.provider_name))
    bindable.value = providers.filter(p => !boundNames.has(p.display_name) && !boundNames.has(p.name))
  } catch (e: any) {
    message.error(e.response?.data?.error || '绑定信息加载失败')
  } finally {
    bindingsLoading.value = false
  }
}

function startBind(providerId: string) {
  // 整页跳转：网关域 cookie 随导航携带，bind state 由后端签发
  window.location.href = ssoLoginURL(providerId, 'bind')
}

async function handleUnbind(id: string) {
  unbinding.value = id
  try {
    await deleteIdentity(id)
    message.success('已解绑')
    await loadBindings()
  } catch (e: any) {
    message.error(e.response?.data?.error || '解绑失败')
  } finally {
    unbinding.value = ''
  }
}

// ── 设置密码（SSO 建号用户首设免旧密码）──

const pwdForm = ref({ current_password: '', new_password: '', confirm: '' })
const pwdLoading = ref(false)

async function handleSetPassword() {
  const { current_password, new_password, confirm } = pwdForm.value
  if (new_password.length < 8 || new_password.length > 128) {
    message.warning('新密码需 8-128 位')
    return
  }
  if (new_password !== confirm) {
    message.warning('两次输入的新密码不一致')
    return
  }
  pwdLoading.value = true
  try {
    await setPassword({ current_password: current_password || undefined, new_password })
    message.success('密码已设置')
    pwdForm.value = { current_password: '', new_password: '', confirm: '' }
  } catch (e: any) {
    const apiErr = e.response?.data?.error
    if (apiErr === 'current_password is required') {
      message.error('该账号已设置密码，请先输入当前密码')
    } else if (apiErr === 'invalid current password') {
      message.error('当前密码不正确')
    } else {
      message.error(apiErr || '设置失败')
    }
  } finally {
    pwdLoading.value = false
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

    <Card class="profile-card" title="三方账号绑定" style="margin-top: 16px">
      <Spin :spinning="bindingsLoading">
        <Empty v-if="!identities.length && !bindable.length" description="暂无可用的三方登录方式" />
        <template v-else>
          <div v-for="item in identities" :key="item.id" class="identity-row">
            <div class="identity-info">
              <Tag color="blue">{{ item.provider_type }}</Tag>
              <span class="identity-name">{{ item.provider_name }}</span>
              <span class="identity-meta">{{ item.email || item.subject }}</span>
            </div>
            <Popconfirm
              title="确定解绑该三方账号？"
              ok-text="解绑"
              cancel-text="取消"
              @confirm="handleUnbind(item.id)"
            >
              <Button danger size="small" :loading="unbinding === item.id">解绑</Button>
            </Popconfirm>
          </div>
          <div v-if="bindable.length" class="bind-section">
            <div class="bind-title">可绑定的三方账号</div>
            <div class="bind-buttons">
              <Button
                v-for="p in bindable"
                :key="p.id"
                size="small"
                @click="startBind(p.id)"
              >
                绑定 {{ p.display_name || p.name }}
              </Button>
            </div>
          </div>
        </template>
      </Spin>
    </Card>

    <Card class="profile-card" title="设置密码" style="margin-top: 16px">
      <template #extra>
        <span class="pwd-hint">
          <SafetyOutlined /> 三方登录账号首次设置密码无需当前密码
        </span>
      </template>
      <Form :model="pwdForm" layout="vertical" style="max-width: 360px">
        <FormItem label="当前密码（首次设置可留空）">
          <Input
            v-model:value="pwdForm.current_password"
            type="password"
            placeholder="已设置过密码的账号必填"
          />
        </FormItem>
        <FormItem label="新密码">
          <Input v-model:value="pwdForm.new_password" type="password" placeholder="8-128 位" />
        </FormItem>
        <FormItem label="确认新密码">
          <Input v-model:value="pwdForm.confirm" type="password" placeholder="再次输入新密码" />
        </FormItem>
        <FormItem>
          <Button type="primary" :loading="pwdLoading" @click="handleSetPassword">保存密码</Button>
        </FormItem>
      </Form>
    </Card>
  </div>
</template>

<style scoped>
.profile-container {
  padding: 24px;
  max-width: 640px;
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

.identity-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 0;
  border-bottom: 1px solid var(--border-card, #f0f0f0);
}

.identity-row:last-of-type {
  border-bottom: none;
}

.identity-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.identity-name {
  font-weight: 500;
  color: var(--text-primary, rgba(0, 0, 0, 0.88));
}

.identity-meta {
  font-size: 12px;
  color: var(--text-tertiary, rgba(0, 0, 0, 0.45));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bind-section {
  margin-top: 16px;
}

.bind-title {
  font-size: 13px;
  color: var(--text-tertiary, rgba(0, 0, 0, 0.45));
  margin-bottom: 8px;
}

.bind-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.pwd-hint {
  font-size: 12px;
  color: var(--text-tertiary, rgba(0, 0, 0, 0.45));
}
</style>
