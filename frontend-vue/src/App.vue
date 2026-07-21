<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { ConfigProvider, theme } from 'ant-design-vue'
import AppLayout from './components/AppLayout.vue'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const themeStore = useThemeStore()
const showLayout = computed(() => !['Login', 'Register'].includes(route.name as string))

const themeConfig = computed(() => ({
  algorithm: themeStore.isDark ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: {
    colorPrimary: '#6C5CE7',
    borderRadius: 8,
  },
}))
</script>

<template>
  <ConfigProvider :theme="themeConfig">
    <AppLayout v-if="showLayout" />
    <router-view v-else />
  </ConfigProvider>
</template>

<style>
#app {
  width: 100%;
  height: 100vh;
}
</style>
