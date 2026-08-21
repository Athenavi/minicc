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

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string>(localStorage.getItem('token') || '')
  const loading = ref(false)

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

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
      token.value = d.token
      user.value = d.user
      localStorage.setItem('token', token.value)
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
      localStorage.setItem('token', token.value)
      return d
    } finally {
      loading.value = false
    }
  }

  async function fetchProfile() {
    if (!token.value) return
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
    } catch (error) {
      // Token 无效，清除
      logout()
    }
  }

  /** SSO 回跳引导：用 httpOnly cookie 换取 Bearer token（SSO 登录后前端建立本地会话） */
  async function bootstrapSession(): Promise<boolean> {
    try {
      const response = await api.get('/v1/auth/session', { withCredentials: true })
      const d = response.data?.data
      if (!d?.token) return false
      token.value = d.token
      user.value = d.user
      localStorage.setItem('token', d.token)
      return true
    } catch {
      return false
    }
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
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
    logout,
  }
})
