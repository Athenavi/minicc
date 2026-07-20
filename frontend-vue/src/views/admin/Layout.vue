<template>
  <n-layout has-sider style="height: 100vh">
    <!-- 侧边栏 -->
    <n-layout-sider
      bordered
      collapse-mode="width"
      :collapsed-width="64"
      :width="240"
      :collapsed="collapsed"
      show-trigger
      @collapse="collapsed = true"
      @expand="collapsed = false"
    >
      <div class="logo">
        <n-icon size="32" color="#18a058">
          <svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
        </n-icon>
        <span v-if="!collapsed" class="logo-text">MiniCC Admin</span>
      </div>
      
      <n-menu
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="22"
        :options="menuOptions"
        :value="activeKey"
        @update:value="handleMenuChange"
      />
    </n-layout-sider>
    
    <!-- 主内容区 -->
    <n-layout>
      <!-- 顶部栏 -->
      <n-layout-header bordered style="height: 64px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between">
        <n-breadcrumb>
          <n-breadcrumb-item v-for="item in breadcrumbs" :key="item.path">
            {{ item.title }}
          </n-breadcrumb-item>
        </n-breadcrumb>
        
        <n-space>
          <n-badge :value="alerts" :max="99">
            <n-button quaternary circle>
              <template #icon>
                <n-icon><svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"/></svg></n-icon>
              </template>
            </n-button>
          </n-badge>
          
          <n-dropdown :options="userOptions" @select="handleUserAction">
            <n-button quaternary>
              <template #icon>
                <n-icon><svg viewBox="0 0 24 24"><path fill="currentColor" d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg></n-icon>
              </template>
              Admin
            </n-button>
          </n-dropdown>
        </n-space>
      </n-layout-header>
      
      <!-- 内容区 -->
      <n-layout-content content-style="padding: 24px;" :native-scrollbar="false">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NIcon } from 'naive-ui'
import type { MenuOption, DropdownOption } from 'naive-ui'

const router = useRouter()
const route = useRoute()

const collapsed = ref(false)
const alerts = ref(3)

const activeKey = computed(() => route.path)

const breadcrumbs = computed(() => {
  const matched = route.matched
  return matched.map(item => ({
    path: item.path,
    title: item.meta?.title || item.name || ''
  }))
})

const menuOptions: MenuOption[] = [
  {
    label: '仪表盘',
    key: '/admin/dashboard',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z' })]) })
  },
  {
    label: 'API Key 管理',
    key: '/admin/api-keys',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M12.65 10C11.83 7.67 9.61 6 7 6c-3.31 0-6 2.69-6 6s2.69 6 6 6c2.61 0 4.83-1.67 5.65-4H17v4h4v-4h2v-4H12.65zM7 14c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z' })]) })
  },
  {
    label: '队列监控',
    key: '/admin/queue',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M4 18h16v2H4v-2zm0-5h16v2H4v-2zm0-5h16v2H4V8z' })]) })
  },
  {
    label: '缓存监控',
    key: '/admin/cache',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M20 13H4c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h16c.55 0 1-.45 1-1v-6c0-.55-.45-1-1-1zM7 19c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2zM20 3H4c-.55 0-1 .45-1 1v6c0 .55.45 1 1 1h16c.55 0 1-.45 1-1V4c0-.55-.45-1-1-1zM7 9c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z' })]) })
  },
  {
    label: '性能监控',
    key: '/admin/performance',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M19 3H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zM9 17H7v-7h2v7zm4 0h-2V7h2v10zm4 0h-2v-4h2v4z' })]) })
  },
  {
    label: '系统设置',
    key: '/admin/settings',
    icon: () => h(NIcon, null, { default: () => h('svg', { viewBox: '0 0 24 24' }, [h('path', { fill: 'currentColor', d: 'M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.07.62-.07.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z' })]) })
  }
]

const userOptions: DropdownOption[] = [
  { label: '个人设置', key: 'profile' },
  { label: '退出登录', key: 'logout' }
]

const handleMenuChange = (key: string) => {
  router.push(key)
}

const handleUserAction = (key: string) => {
  if (key === 'logout') {
    router.push('/login')
  }
}
</script>

<style scoped>
.logo {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-bottom: 1px solid var(--n-border-color);
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  color: var(--n-text-color);
}
</style>
