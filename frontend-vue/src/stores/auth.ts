import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api } from '../api'

export interface User {
  id: string
  email: string
  name: string
  role: string
  tenant_id: string
}

function loadUserFromStorage(): User | null {
  try {
    const raw = localStorage.getItem('user')
    return raw ? JSON.parse(raw) as User : null
  } catch {
    return null
  }
}

function persistUserToStorage(u: User | null) {
  if (u) {
    localStorage.setItem('user', JSON.stringify(u))
  } else {
    localStorage.removeItem('user')
  }
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(loadUserFromStorage())
  // S 安全：token 不再持久化到 localStorage（XSS 偷不走）。
  // 仅保留内存态用于登录后即时判断；刷新后凭 user 持久态判断登录（isAuthenticated）。
  const token = ref<string>('')
  const loading = ref(false)

  const isAuthenticated = computed(() => !!user.value)
  // S 修复：owner 角色拥有全部 admin 权限，isAdmin 须同时识别 admin + owner
  const isAdmin = computed(() => user.value?.role === 'admin' || user.value?.role === 'owner')

  async function login(email: string, password: string, captcha?: { token: string; randstr?: string }) {
    loading.value = true
    try {
      const response = await api.post('/v1/auth/login', {
        email,
        password,
        captcha_token: captcha?.token || '',
        captcha_randstr: captcha?.randstr || '',
      })
      const d = response.data?.data
      if (!d) throw new Error('invalid login response')
      // S 安全：token 不进 localStorage，鉴权凭 httpOnly cookie（后端 SetTokenCookie 已下发）
      token.value = d.token
      user.value = d.user
      persistUserToStorage(d.user)
      return d
    } finally {
      loading.value = false
    }
  }

  async function register(email: string, password: string, name: string, captcha?: { token: string; randstr?: string }) {
    loading.value = true
    try {
      const response = await api.post('/v1/auth/register', {
        email,
        password,
        name,
        captcha_token: captcha?.token || '',
        captcha_randstr: captcha?.randstr || '',
      })
      const d = response.data?.data
      if (!d) throw new Error('invalid register response')
      token.value = d.token
      user.value = d.user
      persistUserToStorage(d.user)
      return d
    } finally {
      loading.value = false
    }
  }

  async function fetchProfile() {
    if (!user.value) return
    try {
      const response = await api.get('/v1/auth/profile')
      const data = response.data?.data
      if (!data) throw new Error('invalid profile response')
      user.value = {
        id: data.user_id,
        email: data.email,
        name: data.name || data.email,
        role: data.role,
        tenant_id: data.tenant_id || '',
      }
      persistUserToStorage(user.value)
    } catch (error) {
      // Cookie 无效，清除本地态
      await logout()
    }
  }

  /** SSO 回跳引导：凭 httpOnly cookie 换取会话信息（SSO 登录后建立本地 user 态） */
  async function bootstrapSession(): Promise<boolean> {
    try {
      const response = await api.get('/v1/auth/session', { withCredentials: true })
      const d = response.data?.data
      if (!d?.token) return false
      // S 安全：token 不进 localStorage，鉴权持续凭 httpOnly cookie
      token.value = d.token
      user.value = d.user
      persistUserToStorage(d.user)
      return true
    } catch {
      return false
    }
  }

  /** 建立/覆盖本地会话（短信登录等直接拿到 {token, user} 的入口复用） */
  function applySession(t: string, u: User) {
    if (!t) throw new Error('invalid session: empty token')
    token.value = t
    user.value = u
    persistUserToStorage(u)
  }

  // S 安全：logout 调后端清 httpOnly cookie（否则登出后 cookie 仍有效，鉴权不失效）
  async function logout() {
    try {
      await api.post('/v1/auth/logout')
    } catch {
      // 网络失败仍清理本地态
    }
    token.value = ''
    user.value = null
    persistUserToStorage(null)
  }

  return {
    user,
    token,
    loading,
    isAuthenticated,
    isAdmin,
    login,
    register,
    fetchProfile,
    bootstrapSession,
    applySession,
    logout,
  }
})
