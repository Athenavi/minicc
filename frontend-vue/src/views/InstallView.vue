<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import {
  getInstallStatus,
  getInstallStep1,
  postInstallStep2,
  postInstallStep3,
  setupSystem,
  hasInstallToken,
  type InstallDep,
} from '../api/install'

const router = useRouter()
const loading = ref(true)
const error = ref(false)
const submitting = ref(false)

// ── 通用状态 ──
const deps = ref<InstallDep[]>([])
const installed = ref(false)

// ── 手动输入令牌 ──
const manualToken = ref('')

// ── 安装模式（setup）三步向导状态 ──
// wizard: env=环境检测(APP_SECRET) / db=数据库配置 / admin=创建管理员 / done=安装完成 /
//         token=缺少令牌 / legacy=正常模式（DB 就绪但无 owner）单步创建管理员
const wizard = ref<'env' | 'db' | 'admin' | 'done' | 'token' | 'legacy'>('env')
const appSecretSet = ref(false)
const dataWritable = ref(true)
const step2Done = ref(false)

const dbForm = ref({ app_secret: '', postgres_dsn: '', redis_addr: '', redis_password: '', redis_db: 0 })
const adminForm = ref({ email: '', password: '', confirm: '', name: '' })

// ── 正常模式（DB 就绪但无管理员）单步表单 ──
const legacyForm = ref({ email: '', password: '', confirm: '', name: '' })

const depsReady = computed(() => deps.value.length > 0 && deps.value.every((d) => d.ok))
const postgresOk = computed(() => deps.value.find((d) => d.name === 'postgres')?.ok ?? false)

async function fetchStatus() {
  loading.value = true
  error.value = false
  try {
    const s = await getInstallStatus()
    deps.value = s.deps ?? []
    if (deps.value.length === 0) {
      deps.value = [
        { name: 'postgres', ok: s.db, message: s.db ? 'PostgreSQL 连接正常' : 'PostgreSQL 不可用' },
        { name: 'redis', ok: s.redis, message: s.redis ? 'Redis 连接正常' : 'Redis 不可用' },
      ]
    }
    if (!s.needed) {
      installed.value = true
    } else if (!postgresOk.value) {
      // PostgreSQL 不可达 → 安装模式，走三步向导（需要安装令牌）
      await loadWizard()
    } else {
      // PostgreSQL 可达但无 owner → 正常模式单步创建管理员
      wizard.value = 'legacy'
    }
  } catch (e: any) {
    error.value = true
    message.error('无法连接后端服务，请确认服务已启动')
  } finally {
    loading.value = false
  }
}

async function loadWizard() {
  if (!hasInstallToken()) {
    wizard.value = 'token'
    return
  }
  try {
    const s = await getInstallStep1()
    if (s.completed) {
      installed.value = true
      return
    }
    appSecretSet.value = !!s.app_secret_set
    dataWritable.value = !!s.data_writable
    step2Done.value = !!s.step2_done
    if (step2Done.value) {
      wizard.value = 'admin'
    } else if (appSecretSet.value) {
      wizard.value = 'db'
    } else {
      wizard.value = 'env'
    }
  } catch (e: any) {
    if (e?.response?.status === 401) {
      wizard.value = 'token'
    } else {
      error.value = true
      message.error(e?.response?.data?.error || '无法读取安装状态')
    }
  }
}

// ── 手动输入令牌 ──
async function submitToken() {
  const tok = manualToken.value.trim()
  if (!tok) {
    message.warning('请输入安装令牌')
    return
  }
  // 替换 URL 中的查询参数，触发页面重新加载
  const url = new URL(window.location.href)
  url.searchParams.set('token', tok)
  window.location.href = url.toString()
}

// ── Step 1：提交 APP_SECRET（或 APP_SECRET 已配置时直接跳转到数据库配置）──
async function skipStep1() {
  // 如果 APP_SECRET 已通过环境变量配置，直接跳转到数据库配置页
  if (appSecretSet.value) {
    wizard.value = 'db'
    return
  }
  // APP_SECRET 未配置，用户必须输入
  if (!dbForm.value.app_secret.trim()) {
    message.warning('请输入 APP_SECRET（部署主密钥，至少 32 字符）')
    return
  }
  if (dbForm.value.app_secret.trim().length < 32) {
    message.warning('APP_SECRET 长度不足，请使用至少 32 字符的随机字符串')
    return
  }
  // 将 APP_SECRET 保留在 dbForm 中，提交 Step 2 时一并发送
  appSecretSet.value = true
  wizard.value = 'db'
  message.success('APP_SECRET 已设置，请继续配置数据库')
}

// ── 三步向导提交 ──
async function submitStep2() {
  if (!dbForm.value.postgres_dsn.trim()) {
    message.warning('PostgreSQL 连接串（DSN）必填')
    return
  }
  submitting.value = true
  try {
    await postInstallStep2({
      app_secret: dbForm.value.app_secret.trim() || undefined,
      postgres_dsn: dbForm.value.postgres_dsn.trim(),
      redis_addr: dbForm.value.redis_addr.trim() || undefined,
      redis_password: dbForm.value.redis_password || undefined,
      redis_db: dbForm.value.redis_db || undefined,
    })
    step2Done.value = true
    wizard.value = 'admin'
    message.success('数据库配置已保存并验证通过')
  } catch (e: any) {
    message.error(e?.response?.data?.error || '数据库配置失败')
  } finally {
    submitting.value = false
  }
}

async function submitStep3() {
  if (!adminForm.value.email || !adminForm.value.name || !adminForm.value.password) {
    message.warning('邮箱、姓名、密码均必填')
    return
  }
  if (adminForm.value.password.length < 8) {
    message.warning('密码至少 8 位')
    return
  }
  if (adminForm.value.password !== adminForm.value.confirm) {
    message.warning('两次密码不一致')
    return
  }
  submitting.value = true
  try {
    await postInstallStep3({
      email: adminForm.value.email,
      password: adminForm.value.password,
      name: adminForm.value.name,
    })
    wizard.value = 'done'
    message.success('安装完成')
  } catch (e: any) {
    message.error(e?.response?.data?.error || '创建管理员失败')
  } finally {
    submitting.value = false
  }
}

// ── 正常模式提交（DB 就绪、无 owner）──
async function submitLegacy() {
  if (!legacyForm.value.email || !legacyForm.value.name || !legacyForm.value.password) {
    message.warning('邮箱、姓名、密码均必填')
    return
  }
  if (legacyForm.value.password.length < 8) {
    message.warning('密码至少 8 位')
    return
  }
  if (legacyForm.value.password !== legacyForm.value.confirm) {
    message.warning('两次密码不一致')
    return
  }
  submitting.value = true
  try {
    await setupSystem({
      email: legacyForm.value.email,
      password: legacyForm.value.password,
      name: legacyForm.value.name,
    })
    message.success('初始化成功，请登录')
    router.replace('/login')
  } catch (e: any) {
    message.error(e?.response?.data?.error || '初始化失败')
  } finally {
    submitting.value = false
  }
}

onMounted(fetchStatus)
</script>

<template>
  <div class="install-page">
    <div class="install-card">
      <div class="install-brand">
        <span class="brand-mark">MC</span>
        <span>MiniCC · 系统初始化</span>
      </div>

      <a-spin :spinning="loading">
        <!-- 依赖探测（始终展示，便于排查连接问题） -->
        <div v-if="!error && deps.length" class="dep-list" aria-label="依赖就绪状态">
          <div v-for="d in deps" :key="d.name" class="dep-item">
            <span class="dep-icon" :class="d.ok ? 'ok' : 'fail'">{{ d.ok ? '✓' : '✕' }}</span>
            <span class="dep-name">{{ d.name }}</span>
            <span class="dep-msg">{{ d.message }}</span>
          </div>
          <a-button v-if="!depsReady" size="small" type="link" @click="fetchStatus">重新检测</a-button>
        </div>

        <!-- 部署模型说明 -->
        <div v-if="!error && !installed" class="install-hint hint-info">
          本部署仅需在 .env 配置 <b>APP_SECRET</b>（唯一主密钥）。PostgreSQL / Redis / CORS / 存储 / 模型 / 支付等配置
          初始化后可在后台「系统设置」统一管理。若数据库/Redis 不在本机默认地址，可在安装向导中填写连接信息，
          保存后<b>重启服务</b>生效。
        </div>

        <!-- 错误（无法连接后端，优先显示） -->
        <template v-if="error">
          <div class="installed-state">
            <div class="error-icon">⚠</div>
            <h3 class="installed-title">无法检查系统状态</h3>
            <p class="installed-desc">请确认后端服务已启动（默认端口 8080）。</p>
            <a-button type="primary" block @click="fetchStatus">重试</a-button>
          </div>
        </template>

        <!-- 已初始化（优先于向导） -->
        <template v-else-if="installed">
          <div class="installed-state">
            <div class="installed-icon">✓</div>
            <h3 class="installed-title">系统已初始化</h3>
            <p class="installed-desc">
              管理员账户已创建，请使用管理员凭据登录系统。
            </p>
            <a-button type="primary" size="large" block @click="router.push('/login')">前往登录</a-button>
          </div>
        </template>

        <!-- ══ 安装模式：缺少令牌 ══ -->
        <template v-else-if="wizard === 'token'">
          <p class="install-hint hint-warn">
            当前处于<b>安装模式</b>（系统未配置数据库/主密钥）。安装页面受令牌保护：
          </p>
          <ol class="token-steps">
            <li>查看服务启动日志中的 <code>install_url</code>（形如 <code>/install?token=xxx</code>）；</li>
            <li>使用日志中的完整地址（含令牌）重新访问本页面，或<b>在下方输入令牌</b>。</li>
          </ol>
          <p class="install-hint hint-info">
            提示：未配置 APP_SECRET 时令牌为随机生成（重启后变化）；配置 APP_SECRET 后令牌由其确定性派生。
          </p>
          <a-form layout="vertical" @finish="submitToken">
            <a-form-item label="安装令牌">
              <a-input v-model:value="manualToken" placeholder="在此粘贴启动日志中的 token" />
            </a-form-item>
            <a-button type="primary" html-type="submit" block>提交令牌</a-button>
          </a-form>
          <a-button type="link" block @click="router.replace('/install')">重新访问安装页</a-button>
        </template>

        <!-- ══ 安装模式 Step 1：环境检测（APP_SECRET）══ -->
        <template v-else-if="wizard === 'env'">
          <a-steps :current="0" size="small" class="wizard-steps">
            <a-step title="环境检测" />
            <a-step title="数据库配置" />
            <a-step title="创建管理员" />
          </a-steps>
          <p class="install-hint hint-warn">
            系统未检测到有效的 <b>APP_SECRET</b>（部署级主密钥，≥32 字符）。它是 JWT / 配置加密的唯一密钥来源。
            您可以<b>在下方输入 APP_SECRET</b>，或在 <code>.env</code> 中配置后重启服务。
          </p>
          <div class="env-check">
            <div class="dep-item">
              <span class="dep-icon" :class="appSecretSet ? 'ok' : 'fail'">{{ appSecretSet ? '✓' : '✕' }}</span>
              <span class="dep-name">APP_SECRET</span>
              <span class="dep-msg">{{ appSecretSet ? '已配置' : '未配置或为弱值/占位符' }}</span>
            </div>
            <div class="dep-item">
              <span class="dep-icon" :class="dataWritable ? 'ok' : 'fail'">{{ dataWritable ? '✓' : '✕' }}</span>
              <span class="dep-name">数据目录</span>
              <span class="dep-msg">{{ dataWritable ? '可写（install.lock 可落盘）' : '不可写：请检查 data/ 目录权限' }}</span>
            </div>
          </div>
          <a-form layout="vertical" @finish="skipStep1">
            <a-form-item label="APP_SECRET（部署主密钥，至少 32 字符）">
              <a-input-password v-model:value="dbForm.app_secret" placeholder="在此输入 APP_SECRET（或先在 .env 中配置后重启服务）" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block>
              {{ appSecretSet ? '继续配置数据库' : '提交 APP_SECRET 并继续' }}
            </a-button>
          </a-form>
        </template>

        <!-- ══ 安装模式 Step 2：数据库配置 ══ -->
        <template v-else-if="wizard === 'db'">
          <a-steps :current="1" size="small" class="wizard-steps">
            <a-step title="环境检测" />
            <a-step title="数据库配置" />
            <a-step title="创建管理员" />
          </a-steps>
          <p class="install-hint hint-warn">
            填写 PostgreSQL 连接信息（必填）与 Redis（选填，留空则按环境变量并降级运行）。
            后端将<b>尝试连接验证</b>，通过后加密保存到 <code>data/install.lock</code>；重启服务后全面生效。
          </p>
          <a-form layout="vertical" @finish="submitStep2">
            <a-form-item label="PostgreSQL 连接串（DSN）" required>
              <a-input v-model:value="dbForm.postgres_dsn" placeholder="postgres://user:pass@host:5432/minicc?sslmode=disable" />
            </a-form-item>
            <a-form-item label="Redis 地址（选填）">
              <a-input v-model:value="dbForm.redis_addr" placeholder="localhost:6379" />
            </a-form-item>
            <a-form-item label="Redis 密码（选填）">
              <a-input-password v-model:value="dbForm.redis_password" placeholder="无密码可留空" />
            </a-form-item>
            <a-form-item label="Redis DB（选填）">
              <a-input-number v-model:value="dbForm.redis_db" :min="0" :max="15" style="width: 100%" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block>保存并验证连接</a-button>
          </a-form>
        </template>

        <!-- ══ 安装模式 Step 3：创建管理员 ══ -->
        <template v-else-if="wizard === 'admin'">
          <a-steps :current="2" size="small" class="wizard-steps">
            <a-step title="环境检测" />
            <a-step title="数据库配置" />
            <a-step title="创建管理员" />
          </a-steps>
          <p class="install-hint hint-warn">
            数据库配置已保存并验证通过。请创建首个管理员账户（owner 角色），该账户拥有全部管理权限。
            完成后安装入口将关闭。
          </p>
          <a-form layout="vertical" @finish="submitStep3">
            <a-form-item label="邮箱" required>
              <a-input v-model:value="adminForm.email" type="email" placeholder="admin@example.com" />
            </a-form-item>
            <a-form-item label="姓名" required>
              <a-input v-model:value="adminForm.name" placeholder="管理员姓名" />
            </a-form-item>
            <a-form-item label="密码（至少 8 位）" required>
              <a-input-password v-model:value="adminForm.password" placeholder="至少 8 位" />
            </a-form-item>
            <a-form-item label="确认密码" required>
              <a-input-password v-model:value="adminForm.confirm" placeholder="再次输入密码" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block>完成安装</a-button>
          </a-form>
        </template>

        <!-- ══ 安装模式：完成 ══ -->
        <template v-else-if="wizard === 'done'">
          <div class="installed-state">
            <div class="installed-icon">✓</div>
            <h3 class="installed-title">安装完成</h3>
            <p class="installed-desc">
              管理员账户已创建，数据库配置已保存。请<b>重启服务</b>使全部功能生效，然后使用管理员凭据登录。
            </p>
            <a-button type="primary" size="large" block @click="router.replace('/login')">前往登录</a-button>
          </div>
        </template>

        <!-- ══ 正常模式：DB 就绪、无管理员（创建首个 owner；Redis 可选降级）══ -->
        <template v-else-if="wizard === 'legacy'">
          <p class="install-hint hint-warn">
            <template v-if="depsReady">
              检测到系统尚未初始化。请创建首个管理员账户（owner 角色），该账户拥有全部管理权限。
              初始化后此入口将自动关闭。
            </template>
            <template v-else>
              PostgreSQL 连接正常，但 Redis 不可用（服务以降级模式运行）。仍可直接创建管理员账户完成初始化。
            </template>
          </p>
          <a-form layout="vertical" @finish="submitLegacy">
            <a-form-item label="邮箱" required>
              <a-input v-model:value="legacyForm.email" type="email" placeholder="admin@example.com" />
            </a-form-item>
            <a-form-item label="姓名" required>
              <a-input v-model:value="legacyForm.name" placeholder="管理员姓名" />
            </a-form-item>
            <a-form-item label="密码（至少 8 位）" required>
              <a-input-password v-model:value="legacyForm.password" placeholder="至少 8 位" />
            </a-form-item>
            <a-form-item label="确认密码" required>
              <a-input-password v-model:value="legacyForm.confirm" placeholder="再次输入密码" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block>初始化系统</a-button>
          </a-form>
        </template>
      </a-spin>

      <div class="install-footer">
        <a-button type="link" @click="router.push('/login')">返回登录</a-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.install-page {
  min-height: 100dvh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 16px;
  background: var(--bg-page);
  position: relative;
  overflow: hidden;
}
.install-page::before {
  content: '';
  position: absolute;
  inset: -45% -20% auto -20%;
  height: 60%;
  background: radial-gradient(ellipse 55% 55% at 50% 0%, var(--primary-bg), transparent 72%);
  pointer-events: none;
}
.install-card {
  width: 440px;
  max-width: calc(100vw - 32px);
  padding: 32px;
  background: var(--bg-card);
  border: 1px solid var(--border-card);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  position: relative;
  z-index: 1;
  animation: installFadeIn 0.5s ease;
}
.install-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 24px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #fff;
  border-radius: 8px;
  font-size: 14px;
  box-shadow: var(--shadow-md);
}
.wizard-steps {
  margin: 0 0 20px;
}
.install-hint {
  margin: 0 0 20px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
  padding: 12px;
  border-radius: var(--radius-sm, 6px);
}
.dep-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0 0 20px;
  padding: 4px 0;
}
.env-check {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0 0 16px;
}
.dep-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
  border: 1px solid var(--border-card);
  border-radius: var(--radius-sm, 6px);
  padding: 8px 10px;
}
.dep-icon.ok { color: var(--colorSuccess, #52c41a); }
.dep-icon.fail { color: var(--colorError, #ff4d4f); }
.dep-name {
  font-weight: 600;
  color: var(--text-primary);
  min-width: 90px;
}
.dep-msg { font-size: 12px; }
.token-steps {
  margin: 0 0 20px;
  padding-left: 18px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.8;
}
.token-steps code {
  background: var(--bg-hover, rgba(128, 128, 128, 0.12));
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 12px;
  word-break: break-all;
}
.hint-warn {
  background: var(--warning-bg, rgba(255, 197, 23, 0.1));
  border: 1px solid var(--warning-border, rgba(255, 197, 23, 0.3));
}
.hint-info {
  background: var(--info-bg, rgba(22, 119, 255, 0.08));
  border: 1px solid var(--info-border, rgba(22, 119, 255, 0.25));
}
.hint-info b { color: var(--text-primary); }
.installed-state {
  text-align: center;
  padding: 24px 0;
}
.installed-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  border-radius: 50%;
  background: var(--success-bg, rgba(82, 196, 26, 0.12));
  color: var(--colorSuccess, #52c41a);
  font-size: 32px;
  line-height: 64px;
  font-weight: bold;
}
.error-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  border-radius: 50%;
  background: var(--error-bg, rgba(255, 77, 79, 0.1));
  color: var(--colorError, #ff4d4f);
  font-size: 32px;
  line-height: 64px;
  font-weight: bold;
}
.installed-title {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: var(--text-primary);
}
.installed-desc {
  margin: 0 0 24px;
  color: var(--text-tertiary);
  font-size: 13px;
}
.install-footer {
  margin-top: 16px;
  text-align: center;
}
@keyframes installFadeIn {
  from { opacity: 0; transform: translateY(20px); }
  to { opacity: 1; transform: translateY(0); }
}

/* 移动端（≤576px，与 .u-hide-sm 断点一致）：小屏顶部对齐，便于长表单滚动 */
@media (max-width: 576px) {
  .install-page { align-items: flex-start; padding: 16px 12px; }
  .install-card { width: 100%; max-width: 100%; padding: 24px 16px; }
  .install-brand { font-size: 16px; margin-bottom: 18px; }
  /* iOS 聚焦防缩放：输入字号 ≥16px（含密码框内层 input） */
  .install-card :deep(.ant-input) { font-size: 16px; }
  /* 触控目标 ≥ 40px（排除 small 按钮） */
  .install-card :deep(.ant-btn:not(.ant-btn-sm)) { min-height: 40px; }
}

/* 焦点增强 */
.install-card :deep(.ant-input:focus),
.install-card :deep(.ant-btn:focus-visible) {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

@media (prefers-reduced-motion: reduce) {
  .install-card { animation: none; }
}
</style>
