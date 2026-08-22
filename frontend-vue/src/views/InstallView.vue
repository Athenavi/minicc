<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { getInstallStatus, setupSystem } from '../api/install'

const router = useRouter()
const loading = ref(true)
const needed = ref(false)
const error = ref(false)
const submitting = ref(false)
const form = ref({ email: '', password: '', confirm: '', name: '' })

async function fetchStatus() {
  loading.value = true
  error.value = false
  try {
    const s = await getInstallStatus()
    needed.value = s.needed
  } catch (e: any) {
    error.value = true
    message.error('无法连接后端服务，请确认服务已启动')
  } finally {
    loading.value = false
  }
}

onMounted(fetchStatus)

async function submit() {
  if (!form.value.email || !form.value.name || !form.value.password) {
    message.warning('邮箱、姓名、密码均必填')
    return
  }
  if (form.value.password.length < 8) {
    message.warning('密码至少 8 位')
    return
  }
  if (form.value.password !== form.value.confirm) {
    message.warning('两次密码不一致')
    return
  }
  submitting.value = true
  try {
    await setupSystem({
      email: form.value.email,
      password: form.value.password,
      name: form.value.name,
    })
    message.success('初始化成功，请登录')
    router.replace('/login')
  } catch (e: any) {
    message.error(e?.response?.data?.error || '初始化失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="install-page">
    <div class="install-card">
      <div class="install-brand">
        <span class="brand-mark">MC</span>
        <span>MiniCC · 系统初始化</span>
      </div>

      <a-spin :spinning="loading">
        <!-- 未初始化：显示初始化表单 -->
        <template v-if="needed">
          <p class="install-hint hint-warn">
            检测到系统尚未初始化。请创建首个管理员账户（owner 角色），该账户拥有全部管理权限。
            初始化后此入口将自动关闭。
          </p>
          <a-form layout="vertical" @finish="submit">
            <a-form-item label="邮箱" required>
              <a-input v-model:value="form.email" type="email" placeholder="admin@example.com" />
            </a-form-item>
            <a-form-item label="姓名" required>
              <a-input v-model:value="form.name" placeholder="管理员姓名" />
            </a-form-item>
            <a-form-item label="密码（至少 8 位）" required>
              <a-input-password v-model:value="form.password" placeholder="至少 8 位" />
            </a-form-item>
            <a-form-item label="确认密码" required>
              <a-input-password v-model:value="form.confirm" placeholder="再次输入密码" />
            </a-form-item>
            <a-button type="primary" html-type="submit" :loading="submitting" block>初始化系统</a-button>
          </a-form>
        </template>

        <!-- 已初始化：显示系统状态 -->
        <template v-else-if="!error">
          <div class="installed-state">
            <div class="installed-icon">✓</div>
            <h3 class="installed-title">系统已初始化</h3>
            <p class="installed-desc">
              管理员账户已创建，请使用管理员凭据登录系统。
            </p>
            <a-button type="primary" size="large" block @click="router.push('/login')">前往登录</a-button>
          </div>
        </template>

        <!-- 错误：显示重试 -->
        <template v-else>
          <div class="installed-state">
            <div class="error-icon">⚠</div>
            <h3 class="installed-title">无法检查系统状态</h3>
            <p class="installed-desc">请确认后端服务已启动（默认端口 8080）。</p>
            <a-button type="primary" block @click="fetchStatus">重试</a-button>
          </div>
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
  width: 420px;
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
.install-hint {
  margin: 0 0 20px;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.6;
  padding: 12px;
  border-radius: var(--radius-sm, 6px);
}
.hint-warn {
  background: var(--warning-bg, rgba(255, 197, 23, 0.1));
  border: 1px solid var(--warning-border, rgba(255, 197, 23, 0.3));
}
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

/* 移动端 */
@media (max-width: 480px) {
  .install-page { align-items: flex-start; padding: 16px 12px; }
  .install-card { width: 100%; max-width: 100%; padding: 24px 20px; }
  .install-brand { font-size: 16px; margin-bottom: 18px; }
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
