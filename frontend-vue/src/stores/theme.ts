import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

export const useThemeStore = defineStore('theme', () => {
  const isDark = ref(false)
  const themePreference = ref<'light' | 'dark' | 'system'>('system')
  
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
  
  return { isDark, themePreference, toggleTheme }
})
