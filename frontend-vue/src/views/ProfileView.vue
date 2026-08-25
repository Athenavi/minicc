<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Card, Button, Input, Form, FormItem, message, Popconfirm, Tag, Spin } from 'ant-design-vue'
import { UserOutlined, SafetyOutlined, MobileOutlined } from '@ant-design/icons-vue'
import EmptyState from '../components/common/EmptyState.vue'

const themeStore = useThemeStore()
// 自定义换肤：强调色预设（DeepSeek 蓝 / 翡翠绿 / 紫罗兰 / 朱红 / 琥珀）
const ACCENT_PRESETS = ['#4176e6', '#10b981', '#8b5cf6', '#ef4444', '#f59e0b']

import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import { api } from '../api'
import {
  listIdentities,
  deleteIdentity,
  setPassword,
  listPublicSsoProviders,
  ssoLoginURL,
  getSmsBind,
  bindPhone,
  unbindPhone,
  sendSmsCode,
  isValidPhone,
  type UserIdentity,
  type SsoPublicProvider,
} from '../api/auth'
import { useSmsCountdown } from '../composables/useSmsCountdown'

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
  loadPhone()
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

// ── 手机号绑定（短信验证码）──

const phone = ref('')
const phoneBound = ref(false)
const phoneLoading = ref(false)
const phoneForm = ref({ phone: '', code: '' })
const phoneBinding = ref(false)
const unbindingPhone = ref(false)
const { remaining: phoneCountdown, start: startPhoneCountdown } = useSmsCountdown(60)
const sendingCode = ref(false)

async function loadPhone() {
  phoneLoading.value = true
  try {
    const res = await getSmsBind()
    phone.value = res.phone || ''
    phoneBound.value = !!res.bound
  } catch (e) {
    // 短信服务未配置/不可达时静默隐藏绑定表单
    console.warn('loadPhone failed:', e)
    phoneBound.value = false
  } finally {
    phoneLoading.value = false
  }
}

async function handleSendBindCode() {
  const p = phoneForm.value.phone.trim()
  if (!isValidPhone(p)) {
    message.warning('请输入正确的手机号')
    return
  }
  sendingCode.value = true
  try {
    const res = await sendSmsCode({ phone: p, purpose: 'bind' })
    startPhoneCountdown(res.interval || 60)
    message.success('验证码已发送')
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 429) {
      message.error('发送过于频繁，请稍后再试')
    } else if (status === 403) {
      message.error(apiErr || '短信服务未启用')
    } else {
      message.error(apiErr || '验证码发送失败')
    }
  } finally {
    sendingCode.value = false
  }
}

async function handleBindPhone() {
  const { phone: p, code } = phoneForm.value
  if (!isValidPhone(p) || !code.trim()) {
    message.warning('请填写手机号与验证码')
    return
  }
  phoneBinding.value = true
  try {
    await bindPhone({ phone: p.trim(), code: code.trim() })
    message.success('手机号绑定成功')
    phoneForm.value = { phone: '', code: '' }
    await loadPhone()
  } catch (e: any) {
    const status = e.response?.status
    const apiErr = e.response?.data?.error
    if (status === 409) {
      message.error('该手机号已绑定其他账号')
    } else {
      message.error(apiErr || '绑定失败')
    }
  } finally {
    phoneBinding.value = false
  }
}

async function handleUnbindPhone() {
  unbindingPhone.value = true
  try {
    await unbindPhone()
    message.success('已解绑手机号')
    await loadPhone()
  } catch (e: any) {
    message.error(e.response?.data?.error || '解绑失败')
  } finally {
    unbindingPhone.value = false
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

        <Card class="profile-card" title="外观与主题" style="margin-top: 16px">
      <div class="appearance-row">
        <div class="appearance-label">深浅模式</div>
        <a-radio-group :value="themeStore.preference" @change="(e: any) => themeStore.toggleTheme()">
          <a-radio-button :value="'dark'">深色</a-radio-button>
          <a-radio-button :value="'light'">浅色</a-radio-button>
        </a-radio-group>
      </div>
      <div class="appearance-row">
        <div class="appearance-label">强调色</div>
        <div class="accent-picker">
          <button
            v-for="c in ACCENT_PRESETS"
            :key="c"
            type="button"
            class="accent-swatch"
            :class="{ active: themeStore.accent === c }"
            :style="{ backgroundColor: c }"
            :title="c"
            @click="themeStore.setAccent(c)"
          />
          <label class="accent-custom" :style="{ backgroundColor: themeStore.accent || '#0a0a0a' }" title="自定义颜色">
            <input
              type="color"
              :value="themeStore.accent"
              @input="(e: any) => themeStore.setAccent(e.target.value)"
            />
          </label>
        </div>
      </div>
    </Card>

<Card class="profile-card" title="三方账号绑定" style="margin-top: 16px">
      <Spin :spinning="bindingsLoading">
        <EmptyState v-if="!identities.length && !bindable.length" description="暂无可用的三方登录方式" />
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
              <Button danger :loading="unbinding === item.id">解绑</Button>
            </Popconfirm>
          </div>
          <div v-if="bindable.length" class="bind-section">
            <div class="bind-title">可绑定的三方账号</div>
            <div class="bind-buttons">
              <Button
                v-for="p in bindable"
                :key="p.id"
                @click="startBind(p.id)"
              >
                绑定 {{ p.display_name || p.name }}
              </Button>
            </div>
          </div>
        </template>
      </Spin>
    </Card>

    <Card class="profile-card" title="手机号" style="margin-top: 16px" :loading="phoneLoading">
      <template v-if="phoneBound">
        <div class="identity-row">
          <div class="identity-info">
            <Tag color="green"><MobileOutlined /></Tag>
            <span class="identity-name">{{ phone }}</span>
            <span class="identity-meta">可用于短信验证码登录</span>
          </div>
          <Popconfirm
            title="确定解绑该手机号？"
            ok-text="解绑"
            cancel-text="取消"
            @confirm="handleUnbindPhone"
          >
            <Button danger :loading="unbindingPhone">解绑</Button>
          </Popconfirm>
        </div>
      </template>
      <template v-else>
        <Form :model="phoneForm" layout="vertical" class="profile-form">
          <FormItem label="手机号">
            <Input
              v-model:value="phoneForm.phone"
              placeholder="请输入手机号"
              :maxlength="21"
            >
              <template #prefix><MobileOutlined /></template>
            </Input>
          </FormItem>
          <FormItem label="验证码">
            <Input v-model:value="phoneForm.code" placeholder="短信验证码" :maxlength="6">
              <template #suffix>
                <Button
                  size="small"
                  type="link"
                  :disabled="phoneCountdown > 0 || sendingCode"
                  :loading="sendingCode"
                  @click="handleSendBindCode"
                >
                  {{ phoneCountdown > 0 ? `${phoneCountdown}s 后重发` : '获取验证码' }}
                </Button>
              </template>
            </Input>
          </FormItem>
          <FormItem>
            <Button type="primary" :loading="phoneBinding" @click="handleBindPhone">
              绑定手机号
            </Button>
          </FormItem>
        </Form>
      </template>
    </Card>

    <Card class="profile-card" title="设置密码" style="margin-top: 16px">
      <template #extra>
        <span class="pwd-hint">
          <SafetyOutlined /> 三方登录账号首次设置密码无需当前密码
        </span>
      </template>
      <Form :model="pwdForm" layout="vertical" class="profile-form">
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
  margin: 0 auto;
}

/* 内嵌表单（手机号/设置密码）：桌面限定宽度，窄屏自动全宽 */
.profile-form {
  max-width: 360px;
}

/* 移动端 */
@media (max-width: 640px) {
  .profile-container { padding: 16px 12px; }
  .profile-header { flex-direction: column; align-items: flex-start; gap: 8px; margin-bottom: 18px; }
  .profile-header h1 { font-size: 20px; }
  .identity-row { flex-direction: column; align-items: flex-start; gap: 6px; }
  .identity-info { width: 100%; }
}

/* 窄屏（≤576px，与 .u-hide-sm 断点一致）：表单/列表竖排、卡片贴边、iOS 防缩放 */
@media (max-width: 576px) {
  .profile-container { padding: 12px; }
  .profile-form { max-width: 100%; }
  .profile-card :deep(.ant-card-body) { padding: 16px; }
  .profile-card :deep(.ant-input) { font-size: 16px; }
  .profile-card :deep(.ant-btn:not(.ant-btn-sm)) { min-height: 40px; }
  /* 卡片头（标题+右侧提示）允许换行，避免长提示挤压 */
  .profile-card :deep(.ant-card-head-wrapper) { flex-wrap: wrap; gap: 4px 8px; }
  .pwd-hint { display: block; }
}

@media (prefers-reduced-motion: reduce) {
  .profile-container { animation: none; transition: none; }
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
  font-variant-numeric: tabular-nums; /* 手机号等数字等宽对齐 */
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

<style scoped>
.appearance-row { display: flex; align-items: center; gap: 16px; padding: 6px 0; flex-wrap: wrap; }
.appearance-label { font-size: 13px; color: var(--text-secondary); min-width: 72px; }
.accent-picker { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.accent-swatch {
  width: 26px; height: 26px; border-radius: 50%; border: 2px solid transparent;
  cursor: pointer; padding: 0; transition: transform 0.12s ease, border-color 0.12s ease;
}
.accent-swatch:hover { transform: scale(1.12); }
.accent-swatch.active { border-color: var(--text-primary); }
.accent-custom {
  position: relative; width: 30px; height: 30px; border-radius: 50%; overflow: hidden;
  border: 2px solid var(--border); display: inline-flex; align-items: center; justify-content: center;
}
.accent-custom input { position: absolute; inset: -6px; width: 42px; height: 42px; opacity: 0; cursor: pointer; }
</style>
