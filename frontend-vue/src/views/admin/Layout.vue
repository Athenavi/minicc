<script setup lang="ts">
import { ref, computed, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../../stores/auth'
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
  TeamOutlined,
  GlobalOutlined,
  SafetyOutlined,
  FileSearchOutlined,
  IdcardOutlined,
  ClusterOutlined,
  WalletOutlined,
  ControlOutlined,
  ShopOutlined,
  FileTextOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

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
  { key: '/admin/tenants', label: '租户管理', icon: () => h(TeamOutlined) },
  { key: '/admin/api-keys', label: 'API Key 管理', icon: () => h(KeyOutlined) },
  { key: '/admin/domains', label: '域名管理', icon: () => h(GlobalOutlined) },
  { key: '/admin/oauth-providers', label: '三方登录', icon: () => h(SafetyOutlined) },
  { key: '/admin/redis', label: 'Redis 管理', icon: () => h(DatabaseOutlined) },
  { key: '/admin/database', label: '数据库管理', icon: () => h(DatabaseOutlined) },
  { key: '/admin/queue', label: '队列监控', icon: () => h(OrderedListOutlined) },
  { key: '/admin/cache', label: '缓存监控', icon: () => h(DatabaseOutlined) },
  { key: '/admin/performance', label: '性能监控', icon: () => h(ThunderboltOutlined) },
  { key: '/admin/settings', label: '系统设置', icon: () => h(SettingOutlined) },
  { key: '/admin/audit', label: '操作审计', icon: () => h(FileSearchOutlined) },
  { key: '/admin/roles', label: '角色管理', icon: () => h(IdcardOutlined) },
  { key: '/admin/groups', label: '群组管理', icon: () => h(ClusterOutlined) },
  { key: '/admin/costcenter', label: '成本中心', icon: () => h(WalletOutlined) },
  { key: '/admin/privacy', label: '隐私模式管控', icon: () => h(SafetyOutlined) },
  { key: '/admin/model-policy', label: '模型策略管控', icon: () => h(ControlOutlined) },
  { key: '/admin/market', label: '企业能力市场', icon: () => h(ShopOutlined) },
  { key: '/admin/api-docs', label: 'API 文档', icon: () => h(FileTextOutlined) },
]

const userMenuItems: any[] = [
  { key: 'profile', label: '个人设置', icon: () => h(UserOutlined) },
  { key: 'logout', label: '退出登录', icon: () => h(LogoutOutlined) },
]

function handleMenuClick(info: any) {
  router.push(info.key)
}

async function handleUserAction(info: any) {
  if (info.key === 'logout') {
    // S 安全：调 authStore.logout 清后端 httpOnly cookie + 本地 user 态
    await authStore.logout()
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
