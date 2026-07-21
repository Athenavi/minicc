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
#app {
  width: 100%;
  height: 100vh;
}
</style>
