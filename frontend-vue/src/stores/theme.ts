import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { api } from '../api'

export const useThemeStore = defineStore('theme', () => {
  const isDark = ref(false)
  const themePreference = ref<'light' | 'dark' | 'system'>('system')
  // 自定义强调色（自定义换肤）：默认 DeepSeek 蓝
  const accent = ref('#4176e6')
  const savedAccent = localStorage.getItem('minicc-accent')
  if (savedAccent && /^#[0-9a-fA-F]{6}$/.test(savedAccent)) accent.value = savedAccent

  
  // 从 localStorage 读取
  const savedTheme = localStorage.getItem('minicc-theme') as 'light' | 'dark' | 'system' | null
  if (savedTheme) {
    themePreference.value = savedTheme
  }
  
  // 检测系统偏好
  const systemDark = window.matchMedia('(prefers-color-scheme: dark)')
  
  // 初始化 isDark
  if (themePreference.value === 'system') {
    isDark.value = systemDark.matches
  } else {
    isDark.value = themePreference.value === 'dark'
  }
  
  // 同步 body class
  function syncBodyClass() {
    if (isDark.value) {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
  }
  syncBodyClass()
  watch(isDark, syncBodyClass)
  
  // 监听系统偏好变化
  systemDark.addEventListener('change', (e) => {
    if (themePreference.value === 'system') {
      isDark.value = e.matches
    }
  })
  
  // 切换主题
  function toggleTheme() {
    if (isDark.value) {
      themePreference.value = 'light'
      isDark.value = false
    } else {
      themePreference.value = 'dark'
      isDark.value = true
    }
    localStorage.setItem('minicc-theme', themePreference.value)
  }
  
  // 生成强调色派生变量（简单明暗/透明推导）
  function shade(hex: string, amt: number): string {
    const n = parseInt(hex.slice(1), 16)
    const r = Math.min(255, Math.max(0, (n >> 16) + amt))
    const g = Math.min(255, Math.max(0, ((n >> 8) & 0xff) + amt))
    const b = Math.min(255, Math.max(0, (n & 0xff) + amt))
    return `#${((r << 16) | (g << 8) | b).toString(16).padStart(6, '0')}`
  }
  function applyAccent() {
    const root = document.documentElement.style
    root.setProperty('--primary', accent.value)
    root.setProperty('--primary-light', shade(accent.value, 28))
    root.setProperty('--primary-dark', shade(accent.value, -42))
    root.setProperty('--accent', shade(accent.value, 46))
  }
  applyAccent()
  watch(accent, applyAccent)

  // 服务端持久化（节流）与加载
  let persistTimer: number | undefined
  function persistAccent() {
    localStorage.setItem('minicc-accent', accent.value)
    if (persistTimer) window.clearTimeout(persistTimer)
    persistTimer = window.setTimeout(() => {
      api.put('/v1/auth/profile', { settings: { theme: { accent: accent.value } } }).catch(() => {})
    }, 600)
  }
  function setAccent(color: string) {
    if (!/^#[0-9a-fA-F]{6}$/.test(color)) return
    accent.value = color
    persistAccent()
  }
  // 应用启动时从服务端恢复（优先于本地缓存）
  api.get('/v1/auth/profile').then((r: any) => {
    const t = r.data?.settings?.theme
    if (t?.accent && /^#[0-9a-fA-F]{6}$/.test(t.accent)) {
      accent.value = t.accent
      applyAccent()
    }
  }).catch(() => {})

  return { isDark, themePreference, accent, toggleTheme, setAccent }
})
