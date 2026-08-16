<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  Layout,
  LayoutSider,
  LayoutHeader,
  LayoutContent,
  Menu,
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Badge,
  Dropdown,
} from 'ant-design-vue'
import {
  DashboardOutlined,
  KeyOutlined,
  OrderedListOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  SettingOutlined,
  BellOutlined,
  UserOutlined,
  LogoutOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()
const route = useRoute()

const collapsed = ref(false)

const breadcrumbs = computed(() => {
  const matched = route.matched
  return matched.map(item => ({
    path: item.path,
    title: item.meta?.title || item.name || '',
  }))
})

const menuItems: any[] = [
  { key: '/admin/dashboard', label: '仪表盘', icon: () => h(DashboardOutlined) },
  { key: '/admin/api-keys', label: 'API Key 管理', icon: () => h(KeyOutlined) },
  { key: '/admin/queue', label: '队列监控', icon: () => h(OrderedListOutlined) },
  { key: '/admin/cache', label: '缓存监控', icon: () => h(DatabaseOutlined) },
  { key: '/admin/performance', label: '性能监控', icon: () => h(ThunderboltOutlined) },
  { key: '/admin/settings', label: '系统设置', icon: () => h(SettingOutlined) },
]

const userMenuItems: any[] = [
  { key: 'profile', label: '个人设置', icon: () => h(UserOutlined) },
  { key: 'logout', label: '退出登录', icon: () => h(LogoutOutlined) },
]

function handleMenuClick(info: any) {
  router.push(info.key)
}

function handleUserAction(info: any) {
  if (info.key === 'logout') {
    localStorage.removeItem('token')
    router.push('/login')
  } else if (info.key === 'profile') {
    router.push('/profile')
  }
}
</script>

<template>
  <Layout style="height: 100%" class="admin-root-layout">
    <LayoutSider
      v-model:collapsed="collapsed"
      collapsible
      :width="240"
      theme="dark"
    >
      <div class="logo">
        <span v-if="!collapsed" class="logo-text">MiniCC Admin</span>
        <span v-else class="logo-text-mini">MC</span>
      </div>
      <Menu
        theme="dark"
        mode="inline"
        :selectedKeys="[route.path]"
        :items="menuItems"
        @click="handleMenuClick"
      />
    </LayoutSider>
    <Layout>
      <LayoutHeader class="admin-header">
        <Breadcrumb>
          <BreadcrumbItem v-for="item in breadcrumbs" :key="item.path">
            {{ item.title }}
          </BreadcrumbItem>
        </Breadcrumb>
        <div class="header-actions">
          <Badge :count="0" :overflow-count="99">
            <Button type="text" class="header-btn">
              <template #icon><BellOutlined /></template>
            </Button>
          </Badge>
          <Dropdown :menu="{ items: userMenuItems, onClick: handleUserAction }">
            <Button type="text" class="header-btn">
              <template #icon><UserOutlined /></template>
              Admin
            </Button>
          </Dropdown>
        </div>
      </LayoutHeader>
      <LayoutContent :style="{ padding: '24px', overflow: 'auto' }">
        <router-view v-slot="{ Component }">
          <Transition name="fade" mode="out-in">
            <component :is="Component" />
          </Transition>
        </router-view>
      </LayoutContent>
    </Layout>
  </Layout>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.logo {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
  color: #fff;
}

.logo-text-mini {
  font-size: 16px;
  font-weight: 700;
  color: #fff;
}

.admin-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  background: #fff;
  border-bottom: 1px solid #f0f0f0;
}

:root.dark .admin-header {
  background: #1e1e1e;
  border-color: #333;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-btn {
  display: flex;
  align-items: center;
  gap: 4px;
}
</style>
