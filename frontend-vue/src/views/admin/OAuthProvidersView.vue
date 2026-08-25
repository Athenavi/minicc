<script setup lang="ts">
/**
 * OAuthProvidersView — 管理端：三方登录 Provider 管理 + 人机验证配置 + 短信服务配置
 *
 * - Provider CRUD：/v1/ent/sso/providers（协议 OIDC/OAuth2、模板类型、端点覆盖）
 * - 人机验证：/v1/ent/captcha/config（turnstile/recaptcha/hcaptcha/tencent/custom）
 * - 短信服务：/v1/ent/sms/config（aliyun/tencent/custom，验证码登录）
 */
import { ref, reactive, onMounted } from 'vue'
import {
  Card, Table, Button, Modal, Form, FormItem, Input, Switch, Select, InputNumber,
  message, Popconfirm, Tag, Alert,
} from 'ant-design-vue'
import {
  listSsoProviders, createSsoProvider, updateSsoProvider, deleteSsoProvider,
  getCaptchaAdminConfig, updateCaptchaConfig,
  getSmsAdminConfig, updateSmsConfig,
  type SsoProvider,
} from '../../api/auth'

// ── Provider 列表 ──

const providers = ref<SsoProvider[]>([])
const loading = ref(false)

async function loadProviders() {
  loading.value = true
  try {
    providers.value = await listSsoProviders()
  } catch (e: any) {
    message.error(e.response?.data?.error || '加载失败')
  } finally {
    loading.value = false
  }
}

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '展示名', dataIndex: 'display_name', key: 'display_name' },
  { title: '协议', dataIndex: 'protocol', key: 'protocol' },
  { title: '类型', dataIndex: 'provider_type', key: 'provider_type' },
  { title: 'Client ID', dataIndex: 'client_id', key: 'client_id', ellipsis: true },
  { title: '启用', dataIndex: 'enabled', key: 'enabled' },
  { title: '自动建号', dataIndex: 'auto_provision', key: 'auto_provision' },
  { title: '排序', dataIndex: 'sort_order', key: 'sort_order' },
  { title: '操作', key: 'actions', width: 160 },
]

const providerTypes = [
  { value: 'google', label: 'Google (OIDC)' },
  { value: 'github', label: 'GitHub (OAuth2)' },
  { value: 'wechat', label: '微信 (OAuth2)' },
  { value: 'dingtalk', label: '钉钉 (OAuth2)' },
  { value: 'feishu', label: '飞书 (OAuth2)' },
  { value: 'qq', label: 'QQ (OAuth2)' },
  { value: 'custom', label: '自定义' },
]

// ── 新建 / 编辑表单 ──

const modalVisible = ref(false)
const editingId = ref<string | null>(null)
const saving = ref(false)

const providerForm = reactive({
  name: '',
  issuer: '',
  client_id: '',
  client_secret: '',
  scopes: '',
  enabled: true,
  auto_provision: true,
  protocol: 'oidc',
  provider_type: 'custom',
  display_name: '',
  sort_order: 100,
  auth_url: '',
  token_url: '',
  userinfo_url: '',
})

function openCreate() {
  editingId.value = null
  Object.assign(providerForm, {
    name: '', issuer: '', client_id: '', client_secret: '', scopes: '',
    enabled: true, auto_provision: true, protocol: 'oidc', provider_type: 'custom',
    display_name: '', sort_order: 100, auth_url: '', token_url: '', userinfo_url: '',
  })
  modalVisible.value = true
}

function openEdit(p: SsoProvider) {
  editingId.value = p.id
  Object.assign(providerForm, {
    name: p.name,
    issuer: p.issuer,
    client_id: p.client_id,
    client_secret: '', // 密文不回显；空 = 保留原值
    scopes: (p.scopes || []).join(' '),
    enabled: p.enabled,
    auto_provision: p.auto_provision,
    protocol: p.protocol,
    provider_type: p.provider_type,
    display_name: p.display_name,
    sort_order: p.sort_order,
    auth_url: p.auth_url,
    token_url: p.token_url,
    userinfo_url: p.userinfo_url,
  })
  modalVisible.value = true
}

async function handleSave() {
  if (!providerForm.name || !providerForm.client_id) {
    message.warning('名称与 Client ID 必填')
    return
  }
  if (providerForm.protocol === 'oidc' && !providerForm.issuer) {
    message.warning('OIDC 协议必须填写 Issuer')
    return
  }
  if (!editingId.value && !providerForm.client_secret) {
    message.warning('新建 Provider 必须填写 Client Secret')
    return
  }
  const body: any = {
    name: providerForm.name,
    issuer: providerForm.issuer,
    client_id: providerForm.client_id,
    enabled: providerForm.enabled,
    auto_provision: providerForm.auto_provision,
    protocol: providerForm.protocol,
    provider_type: providerForm.provider_type,
    display_name: providerForm.display_name,
    sort_order: providerForm.sort_order,
    auth_url: providerForm.auth_url,
    token_url: providerForm.token_url,
    userinfo_url: providerForm.userinfo_url,
  }
  if (providerForm.client_secret) body.client_secret = providerForm.client_secret
  if (providerForm.scopes.trim()) {
    body.scopes = providerForm.scopes.trim().split(/[\s,]+/).filter(Boolean)
  }

  saving.value = true
  try {
    if (editingId.value) {
      await updateSsoProvider(editingId.value, body)
      message.success('已更新')
    } else {
      await createSsoProvider(body)
      message.success('已创建')
    }
    modalVisible.value = false
    await loadProviders()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleDelete(p: SsoProvider) {
  try {
    await deleteSsoProvider(p.id)
    message.success('已删除')
    await loadProviders()
  } catch (e: any) {
    message.error(e.response?.data?.error || '删除失败')
  }
}

// ── 人机验证配置 ──

const captcha = reactive({
  provider: 'turnstile',
  site_key: '',
  secret: '',
  verify_url: '',
  enabled: false,
})
const captchaLoading = ref(false)
const captchaSaving = ref(false)

const captchaProviders = [
  { value: 'turnstile', label: 'Cloudflare Turnstile' },
  { value: 'recaptcha', label: 'Google reCAPTCHA' },
  { value: 'hcaptcha', label: 'hCaptcha' },
  { value: 'tencent', label: '腾讯防水墙' },
  { value: 'custom', label: '自定义（HTTP 端点）' },
]

async function loadCaptcha() {
  captchaLoading.value = true
  try {
    const cfg = await getCaptchaAdminConfig()
    captcha.provider = cfg.provider || 'turnstile'
    captcha.site_key = cfg.site_key || ''
    captcha.secret = '' // 安全：后端应主动脱敏 secret 字段，空 = 保留原值
    captcha.verify_url = cfg.verify_url || ''
    captcha.enabled = !!cfg.enabled
  } catch (e: any) {
    message.error(e.response?.data?.error || '验证码配置加载失败')
  } finally {
    captchaLoading.value = false
  }
}

async function handleSaveCaptcha() {
  if (captcha.enabled) {
    if (captcha.provider !== 'custom' && !captcha.site_key) {
      message.warning('启用前必须填写 Site Key')
      return
    }
    if (captcha.provider === 'custom' && !captcha.verify_url) {
      message.warning('custom 类型必须填写验证端点 URL')
      return
    }
  }
  const body: any = {
    provider: captcha.provider,
    site_key: captcha.site_key,
    enabled: captcha.enabled,
    verify_url: captcha.verify_url,
  }
  if (captcha.secret) body.secret = captcha.secret

  captchaSaving.value = true
  try {
    await updateCaptchaConfig(body)
    message.success('验证码配置已保存')
    captcha.secret = ''
    await loadCaptcha()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    captchaSaving.value = false
  }
}

// ── 短信服务配置 ──

const sms = reactive({
  provider: 'aliyun',
  sign_name: '',
  template_id: '',
  access_key_id: '',
  secret: '',
  endpoint: '',
  code_ttl_seconds: 300,
  send_interval_seconds: 60,
  daily_limit: 10,
  login_enabled: false,
  auto_register: false,
  enabled: false,
})
const smsLoading = ref(false)
const smsSaving = ref(false)

const smsProviders = [
  { value: 'aliyun', label: '阿里云短信' },
  { value: 'tencent', label: '腾讯云短信' },
  { value: 'custom', label: '自定义（HTTP 端点）' },
]

async function loadSms() {
  smsLoading.value = true
  try {
    const cfg = await getSmsAdminConfig()
    sms.provider = cfg.provider || 'aliyun'
    sms.sign_name = cfg.sign_name || ''
    sms.template_id = cfg.template_id || ''
    sms.access_key_id = cfg.access_key_id || ''
    sms.secret = '' // 安全：后端应主动脱敏 secret 字段，空 = 保留原值
    sms.endpoint = cfg.endpoint || ''
    sms.code_ttl_seconds = cfg.code_ttl_seconds || 300
    sms.send_interval_seconds = cfg.send_interval_seconds ?? 60
    sms.daily_limit = cfg.daily_limit || 10
    sms.login_enabled = !!cfg.login_enabled
    sms.auto_register = !!cfg.auto_register
    sms.enabled = !!cfg.enabled
  } catch (e: any) {
    message.error(e.response?.data?.error || '短信配置加载失败')
  } finally {
    smsLoading.value = false
  }
}

async function handleSaveSms() {
  if (sms.enabled) {
    if (sms.provider !== 'custom') {
      if (!sms.sign_name) { message.warning('启用前必须填写短信签名'); return }
      if (!sms.template_id) { message.warning('启用前必须填写模板 ID'); return }
    }
    if (sms.provider === 'custom' && !sms.endpoint) {
      message.warning('custom 类型必须填写发送端点 URL')
      return
    }
  }
  if (sms.login_enabled && !sms.enabled) {
    message.warning('短信登录依赖发送能力，请同时启用短信服务')
    return
  }
  const body: any = {
    provider: sms.provider,
    sign_name: sms.sign_name,
    template_id: sms.template_id,
    access_key_id: sms.access_key_id,
    endpoint: sms.endpoint,
    code_ttl_seconds: sms.code_ttl_seconds,
    send_interval_seconds: sms.send_interval_seconds,
    daily_limit: sms.daily_limit,
    login_enabled: sms.login_enabled,
    auto_register: sms.auto_register,
    enabled: sms.enabled,
  }
  if (sms.secret) body.secret = sms.secret

  smsSaving.value = true
  try {
    await updateSmsConfig(body)
    message.success('短信配置已保存')
    sms.secret = ''
    await loadSms()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    smsSaving.value = false
  }
}

onMounted(() => {
  loadProviders()
  loadCaptcha()
  loadSms()
})
</script>

<template>
  <div class="oauth-providers-view">
    <Card title="三方登录 Provider" :loading="loading">
      <template #extra>
        <Button type="primary" @click="openCreate">新建 Provider</Button>
      </template>

      <Alert
        type="info"
        show-icon
        style="margin-bottom: 16px"
        message="选择内置类型（GitHub/微信/钉钉等）时授权端点自动套用模板；自定义端点留空即可。"
      />

      <Table
        :columns="columns"
        :data-source="providers"
        row-key="id"
        :pagination="false"
        :scroll="{ x: 1000 }"
        size="middle"
      >
        <template #emptyText>
          <div class="empty-block"><span class="empty-icon">📭</span><span class="empty-text">暂无数据</span></div>
        </template>
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <Tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</Tag>
          </template>
          <template v-else-if="column.key === 'auto_provision'">
            <Tag :color="record.auto_provision ? 'blue' : 'default'">{{ record.auto_provision ? '是' : '否' }}</Tag>
          </template>
          <template v-else-if="column.key === 'protocol'">
            <Tag :color="record.protocol === 'oauth2' ? 'purple' : 'cyan'">{{ record.protocol }}</Tag>
          </template>
          <template v-else-if="column.key === 'actions'">
            <Button size="small" style="margin-right: 8px" @click="openEdit(record as SsoProvider)">编辑</Button>
            <Popconfirm title="确定删除该 Provider？" ok-text="删除" cancel-text="取消" @confirm="handleDelete(record as SsoProvider)">
              <Button size="small" danger>删除</Button>
            </Popconfirm>
          </template>
        </template>
      </Table>
    </Card>

    <Card title="人机验证（防接口滥用）" style="margin-top: 16px" :loading="captchaLoading">
      <Alert
        type="info"
        show-icon
        style="margin-bottom: 16px"
        message="启用后登录/注册必须携带验证码 token；未启用时同 IP 连续失败 5 次也会自动升级为强制验证码，30 次直接拒绝。"
      />
      <Form layout="vertical" style="max-width: 520px">
        <FormItem label="验证服务商">
          <Select v-model:value="captcha.provider" :options="captchaProviders" />
        </FormItem>
        <FormItem v-if="captcha.provider !== 'custom'" label="Site Key">
          <Input v-model:value="captcha.site_key" placeholder="前端渲染组件用的站点密钥" />
        </FormItem>
        <FormItem label="Secret（留空保留原值）">
          <Input v-model:value="captcha.secret" placeholder="服务端校验密钥（AES-GCM 加密存储）" />
        </FormItem>
        <FormItem v-if="captcha.provider === 'custom'" label="验证端点 URL">
          <Input v-model:value="captcha.verify_url" placeholder="https://your-captcha.example.com/verify" />
        </FormItem>
        <FormItem label="启用">
          <Switch v-model:checked="captcha.enabled" />
        </FormItem>
        <FormItem>
          <Button type="primary" :loading="captchaSaving" @click="handleSaveCaptcha">保存配置</Button>
        </FormItem>
      </Form>
    </Card>

    <Card title="短信服务（验证码登录）" style="margin-top: 16px" :loading="smsLoading">
      <Alert
        type="info"
        show-icon
        style="margin-bottom: 16px"
        message="启用后登录页出现「短信登录」标签页，个人中心可绑定手机号；验证码发送有冷却与每日上限防滥用。AccessKeySecret 加密存储、回显脱敏。"
      />
      <Form layout="vertical" style="max-width: 520px">
        <FormItem label="短信服务商">
          <Select v-model:value="sms.provider" :options="smsProviders" />
        </FormItem>
        <template v-if="sms.provider !== 'custom'">
          <FormItem label="短信签名（SignName）">
            <Input v-model:value="sms.sign_name" placeholder="如 MiniCC" />
          </FormItem>
          <FormItem label="模板 ID（阿里云 TemplateCode / 腾讯云 TemplateId）">
            <Input v-model:value="sms.template_id" placeholder="如 SMS_12345678（模板参数需含 code）" />
          </FormItem>
          <FormItem label="AccessKeyID（腾讯云填 SmsSdkAppId）">
            <Input v-model:value="sms.access_key_id" />
          </FormItem>
        </template>
        <FormItem v-if="sms.provider === 'custom'" label="发送端点 URL">
          <Input v-model:value="sms.endpoint" placeholder="https://your-sms.example.com/send" />
        </FormItem>
        <FormItem label="AccessKeySecret（留空保留原值）">
          <Input v-model:value="sms.secret" type="password" placeholder="AES-GCM 加密存储" />
        </FormItem>
        <div class="form-grid">
          <FormItem label="验证码有效期（秒）">
            <InputNumber v-model:value="sms.code_ttl_seconds" :min="60" :max="900" style="width: 100%" />
          </FormItem>
          <FormItem label="发送冷却（秒）">
            <InputNumber v-model:value="sms.send_interval_seconds" :min="0" :max="3600" style="width: 100%" />
          </FormItem>
        </div>
        <FormItem label="同一手机号每日发送上限">
          <InputNumber v-model:value="sms.daily_limit" :min="1" :max="100" style="width: 100%" />
        </FormItem>
        <div class="switch-row">
          <FormItem label="启用短信服务">
            <Switch v-model:checked="sms.enabled" />
          </FormItem>
          <FormItem label="短信登录入口">
            <Switch v-model:checked="sms.login_enabled" />
          </FormItem>
          <FormItem label="未注册自动建号">
            <Switch v-model:checked="sms.auto_register" />
          </FormItem>
        </div>
        <FormItem>
          <Button type="primary" :loading="smsSaving" @click="handleSaveSms">保存配置</Button>
        </FormItem>
      </Form>
    </Card>

    <Modal
      v-model:open="modalVisible"
      :title="editingId ? '编辑 Provider' : '新建 Provider'"
      :confirm-loading="saving"
      ok-text="保存"
      cancel-text="取消"
      width="640px"
      @ok="handleSave"
    >
      <Form layout="vertical">
        <div class="form-grid">
          <FormItem label="名称（唯一）">
            <Input v-model:value="providerForm.name" placeholder="如 corporate-okta" />
          </FormItem>
          <FormItem label="展示名">
            <Input v-model:value="providerForm.display_name" placeholder="登录按钮文案，默认同名称" />
          </FormItem>
          <FormItem label="协议">
            <Select v-model:value="providerForm.protocol" :options="[
              { value: 'oidc', label: 'OIDC（标准发现）' },
              { value: 'oauth2', label: 'OAuth2（显式端点）' },
            ]" />
          </FormItem>
          <FormItem label="类型模板">
            <Select v-model:value="providerForm.provider_type" :options="providerTypes" />
          </FormItem>
          <FormItem label="Client ID">
            <Input v-model:value="providerForm.client_id" />
          </FormItem>
          <FormItem label="Client Secret（编辑时留空保留原值）">
            <Input v-model:value="providerForm.client_secret" type="password" />
          </FormItem>
          <FormItem v-if="providerForm.protocol === 'oidc'" label="Issuer">
            <Input v-model:value="providerForm.issuer" placeholder="https://accounts.google.com" />
          </FormItem>
          <FormItem label="排序（小者靠前）">
            <InputNumber v-model:value="providerForm.sort_order" :min="0" :max="9999" style="width: 100%" />
          </FormItem>
        </div>
        <FormItem label="Scopes（空格分隔，留空用模板默认）">
          <Input v-model:value="providerForm.scopes" placeholder="openid email profile" />
        </FormItem>
        <div class="form-grid">
          <FormItem label="授权端点覆盖（OAuth2 可留空用模板）">
            <Input v-model:value="providerForm.auth_url" placeholder="留空 = 模板缺省" />
          </FormItem>
          <FormItem label="Token 端点覆盖">
            <Input v-model:value="providerForm.token_url" placeholder="留空 = 模板缺省" />
          </FormItem>
          <FormItem label="Userinfo 端点覆盖">
            <Input v-model:value="providerForm.userinfo_url" placeholder="留空 = 模板缺省" />
          </FormItem>
        </div>
        <div class="switch-row">
          <FormItem label="启用">
            <Switch v-model:checked="providerForm.enabled" />
          </FormItem>
          <FormItem label="自动建号（未绑定用户首次登录自动注册）">
            <Switch v-model:checked="providerForm.auto_provision" />
          </FormItem>
        </div>
      </Form>
    </Modal>
  </div>
</template>

<style scoped>
.oauth-providers-view {
  max-width: 1200px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.switch-row {
  display: flex;
  gap: 32px;
}

/* 空状态统一 */
.empty-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 28px 0;
  color: var(--text-tertiary);
}
.empty-icon { font-size: 26px; line-height: 1; opacity: 0.8; }
.empty-text { font-size: 13px; }

/* 窄屏:表单单列、开关竖排、操作列吸底、触控目标 ≥ 40px */
@media (max-width: 768px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .switch-row {
    flex-direction: column;
    gap: 0;
  }
  .switch-row .ant-form-item {
    margin-bottom: 16px;
  }
  .oauth-providers-view .ant-btn:not(.ant-btn-sm) { min-height: 40px; }
  .oauth-providers-view :deep(.ant-btn-sm) { position: relative; }
  .oauth-providers-view :deep(.ant-btn-sm)::after {
    content: '';
    position: absolute;
    inset: -8px;
    border-radius: inherit;
  }
  /* 操作列固定在右缘,不挤压主列 */
  .oauth-providers-view :deep(.ant-table-thead > tr > th:last-child),
  .oauth-providers-view :deep(.ant-table-tbody > tr > td:last-child) {
    position: sticky;
    right: 0;
    background: var(--bg-card);
    z-index: 2;
    box-shadow: -8px 0 12px -8px rgba(0, 0, 0, 0.12);
  }
  .oauth-providers-view :deep(.ant-table-thead > tr > th:last-child) { z-index: 3; }
}
</style>
