<script setup lang="ts">
import { ref, computed, h, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useThemeStore } from '../stores/theme'
import { useAuthStore } from '../stores/auth'

// Ant Design Vue 组件
import {
  Layout,
  LayoutSider,
  LayoutContent,
  Menu,
  Button,
  Avatar,
  Dropdown,
  message,
} from 'ant-design-vue'
// Ant Design 图标
import {
  MessageOutlined,
  UserOutlined,
  ApartmentOutlined,
  BlockOutlined,
  PictureOutlined,
  BookOutlined,
  ThunderboltOutlined,
  CreditCardOutlined,
  SettingOutlined,
  LogoutOutlined,
  UserSwitchOutlined,
  BulbOutlined,
  MenuOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const themeStore = useThemeStore()
const collapsed = ref(window.innerWidth <= 768)

// 监听 API 错误
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

// 菜单配置
interface MenuItem {
  key: string
  label: string
  icon?: any
  children?: MenuItem[]
}

const menuItems = computed<MenuItem[]>(() => {
  const items: MenuItem[] = [
    { key: '/chat', label: '对话', icon: () => h(MessageOutlined) },
    { key: '/agents', label: 'Agent', icon: () => h(UserOutlined) },
    { key: '/workflow', label: '工作流', icon: () => h(ApartmentOutlined) },
    { key: '/skills', label: '技能', icon: () => h(BlockOutlined) },
    { key: '/media', label: '媒体库', icon: () => h(PictureOutlined) },
    { key: '/knowledge', label: '知识库', icon: () => h(BookOutlined) },
    { key: '/plugins', label: '插件', icon: () => h(ThunderboltOutlined) },
    { key: '/billing', label: '计费', icon: () => h(CreditCardOutlined) },
    ...(authStore.isAdmin
      ? [{ key: '/admin', label: '管理', icon: () => h(SettingOutlined) }]
      : []),
  ]
  return items
})

const selectedKeys = computed(() => [route.path])

const userMenuItems = computed<any[]>(() => [
  { key: 'profile', label: '个人资料', icon: () => h(UserSwitchOutlined) },
  { key: 'toggle-theme', label: themeStore.isDark ? '浅色模式' : '深色模式', icon: () => h(BulbOutlined) },
  { key: 'logout', label: '退出登录', icon: () => h(LogoutOutlined) },
])

function handleMenuClick(info: any) {
  router.push(info.key)
  if (window.innerWidth <= 768) {
    collapsed.value = true
  }
}

function handleUserMenuClick(info: any) {
  if (info.key === 'logout') {
    authStore.logout()
    router.push('/login')
  } else if (info.key === 'profile') {
    router.push('/profile')
  } else if (info.key === 'toggle-theme') {
    themeStore.toggleTheme()
  }
}
</script>

<template>
  <Layout style="height: 100vh">
    <LayoutSider
      v-model:collapsed="collapsed"
      :trigger="null"
      collapsible
      :width="240"
      :collapsed-width="0"
      :class="['nav-sider', { 'nav-sider-mobile': collapsed }]"
      :style="{ position: 'relative', zIndex: 300, height: '100vh', overflow: 'auto' }"
    >
      <div class="sidebar-header" :class="{ collapsed }">
        <span class="sidebar-logo">MC</span>
        <span v-if="!collapsed" class="sidebar-title">MiniCC</span>
      </div>
      <Menu
        :theme="themeStore.isDark ? 'dark' : 'light'"
        mode="inline"
        :selectedKeys="selectedKeys"
        :items="menuItems"
        @click="handleMenuClick"
        :style="{ borderRight: 0, flex: 1, background: 'transparent' }"
      />
      <div class="sidebar-footer">
        <Dropdown
          v-if="authStore.user"
          :menu="{ items: userMenuItems, onClick: handleUserMenuClick }"
        >
          <Button type="text" size="small" class="sidebar-user-btn">
            <Avatar
              :size="24"
              :style="{ backgroundColor: 'var(--primary)', verticalAlign: 'middle' }"
            >
              {{ authStore.user.name?.charAt(0)?.toUpperCase() || 'U' }}
            </Avatar>
            <span v-if="!collapsed" style="margin-left: 8px">
              {{ authStore.user.name || authStore.user.email }}
            </span>
          </Button>
        </Dropdown>
        <Button
          type="text"
          size="small"
          class="sidebar-user-btn"
          @click="themeStore.toggleTheme()"
          :title="themeStore.isDark ? '切换到浅色模式' : '切换到深色模式'"
        >
          <template #icon>
            <BulbOutlined />
          </template>
          <span v-if="!collapsed" style="margin-left: 8px">
            {{ themeStore.isDark ? '浅色模式' : '深色模式' }}
          </span>
        </Button>
      </div>
    </LayoutSider>

    <!-- 移动端菜单按钮 -->
    <Button
      v-if="collapsed"
      class="nav-menu-btn"
      type="text"
      @click="collapsed = false"
      title="打开菜单"
    >
      <template #icon><MenuOutlined /></template>
    </Button>

    <!-- 移动端遮罩 -->
    <div v-if="!collapsed" class="nav-overlay" @click="collapsed = true"></div>

    <LayoutContent :style="{ margin: 0, overflow: 'auto' }">
      <router-view v-slot="{ Component }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </LayoutContent>
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

/* ── 侧边栏：跟随主题，边框分隔（Linear/Vercel 式） ── */
.nav-sider {
  background: var(--bg-card) !important;
  border-right: 1px solid var(--border);
}

/* ── 品牌区：accent 小色块 + 品牌名，去紫色渐变 ── */
.sidebar-header {
  padding: 18px 16px 14px;
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 4px 0;
}
.sidebar-header.collapsed {
  padding: 18px 0 14px;
  justify-content: center;
}

.sidebar-logo {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-weight: 700;
  font-size: 12px;
  letter-spacing: 0.02em;
  flex-shrink: 0;
  box-shadow: var(--shadow-md);
}

.sidebar-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.01em;
  white-space: nowrap;
}

/* ── 菜单：透明背景跟随侧边栏，选中态 accent-soft ── */
.nav-sider :deep(.ant-menu) {
  background: transparent !important;
  border-inline-end: 0 !important;
}
.nav-sider :deep(.ant-menu-item) {
  margin: 2px 8px;
  border-radius: var(--radius-md) !important;
  color: var(--text-secondary);
}
.nav-sider :deep(.ant-menu-item:hover) {
  color: var(--text-primary) !important;
  background: var(--bg-hover) !important;
}
.nav-sider :deep(.ant-menu-item-selected) {
  background: var(--primary-bg) !important;
  color: var(--primary) !important;
  font-weight: 600;
}
.nav-sider :deep(.ant-menu-item .anticon) {
  font-size: 15px;
}

/* ── 底部用户区 ── */
.sidebar-footer {
  padding: 12px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
  border-top: 1px solid var(--border);
  margin-top: auto;
}

.sidebar-user-btn {
  width: 100%;
  display: flex;
  justify-content: flex-start;
  align-items: center;
  color: var(--text-secondary) !important;
  height: 40px;
  border-radius: var(--radius-md);
}

.sidebar-user-btn:hover {
  color: var(--text-primary) !important;
  background: var(--bg-hover) !important;
}

/* 移动端导航按钮 */
.nav-menu-btn {
  display: none;
}
.nav-overlay {
  display: none;
}

@media (max-width: 768px) {
  .nav-menu-btn {
    display: flex;
    position: fixed;
    top: 12px;
    right: 12px;
    z-index: 200;
    width: 36px;
    height: 36px;
    align-items: center;
    justify-content: center;
    background: var(--bg-card);
    box-shadow: var(--shadow-md);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-primary);
  }
  .nav-sider-mobile {
    transition: transform 0.25s ease !important;
  }
  .nav-sider-mobile.ant-layout-sider-collapsed {
    transform: translateX(-100%);
  }
  .nav-overlay {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 250;
    background: rgba(10, 10, 12, 0.45);
  }
}
</style>
