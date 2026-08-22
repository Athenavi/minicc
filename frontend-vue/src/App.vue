<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ConfigProvider, theme } from 'ant-design-vue'
import AppLayout from './components/AppLayout.vue'
import RouteProgressBar from './components/common/RouteProgressBar.vue'
import ErrorBoundary from './components/common/ErrorBoundary.vue'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const themeStore = useThemeStore()
const showLayout = computed(() => !['Login', 'Register'].includes(route.name as string))

// 与 style.css 语义令牌同步的 antd token（亮/暗双套）
// 强调色：DeepSeek 品牌蓝（复刻 design-platform）；输入胶囊圆角 22
const fontStack = "'Geist Variable', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Helvetica Neue', Helvetica, Arial, sans-serif"

const lightTokens = {
  colorPrimary: '#4176E6',
  colorInfo: '#4176E6',
  colorSuccess: '#22C55E',
  colorWarning: '#F59E0B',
  colorError: '#EF4444',
  borderRadius: 6,
  fontFamily: fontStack,
  colorBgLayout: '#F9FAFB',
  colorBgContainer: '#FFFFFF',
  colorBgElevated: '#FFFFFF',
  colorBorder: 'rgba(0, 0, 0, 0.1)',
  colorBorderSecondary: 'rgba(0, 0, 0, 0.04)',
  colorText: '#0F1115',
  colorTextSecondary: '#61666B',
  colorTextTertiary: '#81858C',
  colorTextQuaternary: '#ADB2B8',
  controlOutline: 'rgba(65, 118, 230, 0.15)',
}

const darkTokens = {
  colorPrimary: '#5686FE',
  colorInfo: '#5686FE',
  colorSuccess: '#22C55E',
  colorWarning: '#F59E0B',
  colorError: '#EF4444',
  borderRadius: 6,
  fontFamily: fontStack,
  colorBgLayout: '#151517',
  colorBgContainer: '#232324',
  colorBgElevated: '#2C2C2E',
  colorBorder: 'rgba(255, 255, 255, 0.12)',
  colorBorderSecondary: 'rgba(255, 255, 255, 0.06)',
  colorText: '#F9FAFB',
  colorTextSecondary: '#CFD3D6',
  colorTextTertiary: '#ADB2B8',
  colorTextQuaternary: '#81858C',
  controlOutline: 'rgba(103, 158, 254, 0.25)',
}

const themeConfig = computed(() => ({
  algorithm: themeStore.isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: themeStore.isDark ? darkTokens : lightTokens,
}))
</script>

<template>
  <ConfigProvider :theme="themeConfig">
    <RouteProgressBar />
    <ErrorBoundary>
      <AppLayout v-if="showLayout" />
      <router-view v-else />
    </ErrorBoundary>
  </ConfigProvider>
</template>

<style>
#app {
  width: 100%;
  height: 100vh;
}
</style>
