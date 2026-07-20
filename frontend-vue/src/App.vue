<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { darkTheme, NConfigProvider, NMessageProvider, NDialogProvider, useMessage } from 'naive-ui'
import AppLayout from './components/AppLayout.vue'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const themeStore = useThemeStore()
const showLayout = computed(() => !['Login', 'Register'].includes(route.name as string))

// 全局 API 错误监听
const message = useMessage()
function handleApiError(e: Event) {
  const detail = (e as CustomEvent).detail
  message.error(detail.message || '请求失败')
}
onMounted(() => {
  window.addEventListener('api:error', handleApiError)
})
onUnmounted(() => {
  window.removeEventListener('api:error', handleApiError)
})
</script>

<template>
  <NConfigProvider :theme="themeStore.isDark ? darkTheme : null">
    <NMessageProvider>
      <NDialogProvider>
        <AppLayout v-if="showLayout" />
        <router-view v-else />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  transition: background-color 0.3s, color 0.3s;
}

html.dark body {
  background-color: #1a1a2e;
  color: #e0e0e0;
}

#app {
  width: 100%;
  height: 100vh;
}
</style>
