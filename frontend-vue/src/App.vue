<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { darkTheme, NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import AppLayout from './components/AppLayout.vue'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const themeStore = useThemeStore()
const showLayout = computed(() => !['Login', 'Register'].includes(route.name as string))
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
