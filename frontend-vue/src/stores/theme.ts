import { defineStore } from 'pinia'
import { ref, watch, computed } from 'vue'

export type ThemeId = 'linear' | 'supabase' | 'notion' | 'futuristic'
export type ThemeMode = 'light' | 'dark'
export type ThemePreference = ThemeMode | 'system'

export interface ThemePreset {
  id: ThemeId
  name: string
  description: string
  icon: string
  lightTokens: Record<string, string | number>
  darkTokens: Record<string, string | number>
  lightLabel: string
  darkLabel: string
}

const THEME_REGISTRY: Record<ThemeId, ThemePreset> = {
  linear: {
    id: 'linear',
    name: '极简专业',
    description: 'Linear 风格，黑白灰主色，极简专业',
    icon: '◼',
    lightLabel: 'Linear 浅色',
    darkLabel: 'Linear 深色',
    lightTokens: {
      colorPrimary: '#0a0a0a',
      colorInfo: '#0a0a0a',
      borderRadius: 6,
      colorBgLayout: '#fafafa',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorBorder: '#e5e5e5',
      colorBorderSecondary: '#f0f0f0',
      colorText: '#0a0a0a',
      colorTextSecondary: '#525252',
      colorTextTertiary: '#737373',
      controlOutline: 'rgba(10, 10, 10, 0.1)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
    darkTokens: {
      colorPrimary: '#f5f5f5',
      colorInfo: '#f5f5f5',
      borderRadius: 6,
      colorBgLayout: '#0a0a0a',
      colorBgContainer: '#171717',
      colorBgElevated: '#171717',
      colorBorder: '#262626',
      colorBorderSecondary: '#1f1f1f',
      colorText: '#fafafa',
      colorTextSecondary: '#a3a3a3',
      colorTextTertiary: '#737373',
      controlOutline: 'rgba(255, 255, 255, 0.1)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
  },
  supabase: {
    id: 'supabase',
    name: '精致现代',
    description: 'Supabase 风格，柔和渐变，琥珀主色',
    icon: '◆',
    lightLabel: 'Supabase 浅色',
    darkLabel: 'Supabase 深色',
    lightTokens: {
      colorPrimary: '#eab308',
      colorInfo: '#eab308',
      borderRadius: 12,
      colorBgLayout: '#fefef8',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorBorder: '#e7e5e4',
      colorBorderSecondary: '#f5f5f4',
      colorText: '#1c1917',
      colorTextSecondary: '#57534e',
      colorTextTertiary: '#78716c',
      controlOutline: 'rgba(234, 179, 8, 0.1)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
    darkTokens: {
      colorPrimary: '#facc15',
      colorInfo: '#facc15',
      borderRadius: 12,
      colorBgLayout: '#1c1917',
      colorBgContainer: '#292524',
      colorBgElevated: '#292524',
      colorBorder: '#44403c',
      colorBorderSecondary: '#292524',
      colorText: '#fef9c3',
      colorTextSecondary: '#d6d3d1',
      colorTextTertiary: '#a8a29e',
      controlOutline: 'rgba(250, 204, 21, 0.15)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
  },
  notion: {
    id: 'notion',
    name: '极简功能',
    description: 'Notion 风格，高信息密度，清晰排版',
    icon: '■',
    lightLabel: 'Notion 浅色',
    darkLabel: 'Notion 深色',
    lightTokens: {
      colorPrimary: '#0f172a',
      colorInfo: '#0f172a',
      borderRadius: 4,
      colorBgLayout: '#ffffff',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorBorder: '#e2e8f0',
      colorBorderSecondary: '#f1f5f9',
      colorText: '#0f172a',
      colorTextSecondary: '#475569',
      colorTextTertiary: '#64748b',
      controlOutline: 'rgba(15, 23, 42, 0.08)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
    darkTokens: {
      colorPrimary: '#f1f5f9',
      colorInfo: '#f1f5f9',
      borderRadius: 4,
      colorBgLayout: '#0f172a',
      colorBgContainer: '#1e293b',
      colorBgElevated: '#1e293b',
      colorBorder: '#334155',
      colorBorderSecondary: '#1e293b',
      colorText: '#f8fafc',
      colorTextSecondary: '#cbd5e1',
      colorTextTertiary: '#94a3b8',
      controlOutline: 'rgba(241, 245, 249, 0.1)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
  },
  futuristic: {
    id: 'futuristic',
    name: '未来科技',
    description: '霓虹渐变，玻璃拟态，科技感十足',
    icon: '◈',
    lightLabel: '科技浅色',
    darkLabel: '科技深色',
    lightTokens: {
      colorPrimary: '#0d9488',
      colorInfo: '#0d9488',
      borderRadius: 16,
      colorBgLayout: '#f0fdfa',
      colorBgContainer: '#ffffff',
      colorBgElevated: '#ffffff',
      colorBorder: '#ccfbf1',
      colorBorderSecondary: '#f0fdfa',
      colorText: '#042f2e',
      colorTextSecondary: '#115e59',
      colorTextTertiary: '#0d9488',
      controlOutline: 'rgba(13, 148, 136, 0.12)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
    darkTokens: {
      colorPrimary: '#5eead4',
      colorInfo: '#5eead4',
      borderRadius: 16,
      colorBgLayout: '#040f0f',
      colorBgContainer: '#061a19',
      colorBgElevated: '#061a19',
      colorBorder: 'rgba(94, 234, 212, 0.18)',
      colorBorderSecondary: 'rgba(94, 234, 212, 0.08)',
      colorText: '#f0fdfa',
      colorTextSecondary: '#5eead4',
      colorTextTertiary: '#2dd4bf',
      controlOutline: 'rgba(94, 234, 212, 0.15)',
      fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif",
    },
  },
}

const systemDark = window.matchMedia('(prefers-color-scheme: dark)')

export const useThemeStore = defineStore('theme', () => {
  const activeThemeId = ref<ThemeId>('linear')
  const mode = ref<ThemeMode>('light')
  const preference = ref<ThemePreference>('system')

  const accent = ref<string | null>(null)

  const systemDarkMql = window.matchMedia('(prefers-color-scheme: dark)')

  const resolvedMode = computed<ThemeMode>(() => {
    if (preference.value === 'system') {
      return systemDarkMql.matches ? 'dark' : 'light'
    }
    return preference.value
  })

  const preset = computed<ThemePreset>(() => THEME_REGISTRY[activeThemeId.value])

  const isDark = computed<boolean>(() => resolvedMode.value === 'dark')

  const antdTokens = computed(() => {
    const tokens = isDark.value ? preset.value.darkTokens : preset.value.lightTokens
    return {
      ...tokens,
      colorPrimary: String(accent.value ?? (isDark.value ? preset.value.darkTokens.colorPrimary : preset.value.lightTokens.colorPrimary)),
      colorInfo: String(accent.value ?? (isDark.value ? preset.value.darkTokens.colorInfo : preset.value.lightTokens.colorInfo)),
    }
  })

  const cssThemeId = computed<string>(() => `${activeThemeId.value}-${resolvedMode.value}`)

  function applyTheme() {
    const root = document.documentElement
    root.setAttribute('data-theme', cssThemeId.value)
    if (isDark.value) {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
  }

  function setTheme(themeId: ThemeId) {
    activeThemeId.value = themeId
    localStorage.setItem('minicc-theme', themeId)
    applyTheme()
  }

  function setMode(newMode: ThemePreference) {
    preference.value = newMode
    localStorage.setItem('minicc-theme-pref', newMode)
    applyTheme()
  }

  function toggleMode() {
    setMode(isDark.value ? 'light' : 'dark')
  }

  function toggleTheme() {
    toggleMode()
  }

  function setAccent(color: string | null) {
    if (color && /^#[0-9a-fA-F]{6}$/.test(color)) {
      accent.value = color
      localStorage.setItem('minicc-accent', color)
    } else if (color === null) {
      accent.value = null
      localStorage.removeItem('minicc-accent')
    }
  }

  function getAvailableThemes(): ThemePreset[] {
    return Object.values(THEME_REGISTRY)
  }

  // Initialization
  function init() {
    const savedTheme = localStorage.getItem('minicc-theme') as ThemeId | null
    if (savedTheme && savedTheme in THEME_REGISTRY) {
      activeThemeId.value = savedTheme
    }

    const savedPref = localStorage.getItem('minicc-theme-pref') as ThemePreference | null
    if (savedPref) {
      preference.value = savedPref
    }

    const savedAccent = localStorage.getItem('minicc-accent')
    if (savedAccent && /^#[0-9a-fA-F]{6}$/.test(savedAccent)) {
      accent.value = savedAccent
    }

    applyTheme()
  }

  // Watchers
  watch([activeThemeId, preference, accent], () => {
    applyTheme()
  })

  systemDarkMql.addEventListener('change', (e) => {
    if (preference.value === 'system') {
      applyTheme()
    }
  })

  return {
    activeThemeId,
    mode,
    preference,
    preset,
    isDark,
    antdTokens,
    cssThemeId,
    accent,
    resolvedMode,
    setTheme,
    setMode,
    toggleMode,
    toggleTheme,
    setAccent,
    getAvailableThemes,
    init,
  }
})
