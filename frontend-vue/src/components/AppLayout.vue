<script setup lang="ts">
import { computed, h, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useThemeStore } from '../stores/theme'
import { useAuthStore } from '../stores/auth'

// Ant Design Vue 组件
import {
  Avatar,
  Dropdown,
  Menu,
  message,
} from 'ant-design-vue'
// Ant Design 图标
import {
  HomeOutlined,
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
  DownOutlined,
} from '@ant-design/icons-vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const themeStore = useThemeStore()

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

// 导航菜单（品牌下拉）
interface MenuItem {
  key: string
  label: string
  icon?: any
}

const menuItems = computed<MenuItem[]>(() => {
  const items: MenuItem[] = [
    { key: '/', label: '首页', icon: () => h(HomeOutlined) },
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
    // 主题切换并入品牌菜单（顶栏不再有独立按钮；未登录用户也可用）
    { key: 'toggle-theme', label: themeStore.isDark ? '浅色模式' : '深色模式', icon: () => h(BulbOutlined) },
  ]
  return items
})

// 当前路由对应的菜单项文案（品牌胶囊 title 提示）
const currentLabel = computed(() => {
  const hit = menuItems.value.find(m => route.path === m.key || route.path.startsWith(m.key + '/'))
  return hit?.label || ''
})

// 首页（品牌页）强制深色：整个应用外壳（含顶栏及其后方背景）统一深色，
// 避免深色内容 + 亮色顶栏后区的割裂（半透明白叠亮底 = 视觉不透明）
const isHome = computed(() => route.path === '/')

const selectedKeys = computed(() => {
  const exact = menuItems.value.find(m => route.path === m.key)
  return [exact?.key ?? route.path]
})

function handleMenuClick(info: any) {
  if (info.key === 'toggle-theme') {
    themeStore.toggleTheme()
    return
  }
  router.push(info.key)
}

// 用户下拉（主题切换已并入品牌菜单，单一入口）
const userMenuItems = computed<any[]>(() => [
  { key: 'profile', label: '个人资料', icon: () => h(UserSwitchOutlined) },
  { key: 'logout', label: '退出登录', icon: () => h(LogoutOutlined) },
])

function handleUserMenuClick(info: any) {
  if (info.key === 'logout') {
    authStore.logout()
    router.push('/login')
  } else if (info.key === 'profile') {
    router.push('/profile')
  }
}
</script>

<template>
  <div class="app-shell" :class="{ 'app-shell--dark': isHome }">
    <!-- 左上角浮动品牌胶囊（导航入口；不占布局，悬浮于内容之上） -->
    <header class="topbar" :title="currentLabel || '导航菜单'">
      <Dropdown trigger="click" placement="bottomLeft">
        <button type="button" class="brand-btn" title="导航菜单">
          <span class="brand-logo">MC</span>
          <span class="brand-name">MiniCC</span>
          <DownOutlined class="brand-caret" />
        </button>
        <template #overlay>
          <Menu
            class="nav-menu"
            :selectedKeys="selectedKeys"
            :items="menuItems"
            @click="handleMenuClick"
          />
        </template>
      </Dropdown>
    </header>

    <!-- 右上角用户胶囊（登录时；不占布局） -->
    <div v-if="authStore.user" class="user-fab">
      <Dropdown :menu="{ items: userMenuItems, onClick: handleUserMenuClick }">
        <Avatar
          :size="30"
          class="user-fab-avatar"
          :style="{ backgroundColor: 'var(--primary)' }"
        >
          {{ authStore.user.name?.charAt(0)?.toUpperCase() || 'U' }}
        </Avatar>
      </Dropdown>
    </div>

    <!-- 全宽内容区（浮动胶囊悬浮其上，不占位） -->
    <main class="app-content">
      <router-view v-slot="{ Component }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>
  </div>
</template>

<style scoped>
.app-shell {
  position: relative;
  height: 100vh;
  background: var(--bg-page);
}

/* 首页（品牌页）统一深色：顶栏及后方背景跟随深色变量（rgba 半透明才显现） */
.app-shell--dark {
  --bg-page: #151517;
  --bg-card: #232324;
  --bg-secondary: #2c2c2e;
  --bg-hover: rgba(255, 255, 255, 0.08);
  --border: rgba(255, 255, 255, 0.12);
  --topbar-bg: rgba(35, 35, 36, 0.4);
  --menu-bg: rgba(44, 44, 46, 0.72);
  --text-primary: #f9fafb;
  --text-secondary: #cfd3d6;
  --text-tertiary: #adb2b8;
  --primary: #5686fe;
  --primary-bg: rgba(103, 158, 254, 0.14);
}

/* ── 左上角浮动品牌胶囊：不占布局、悬浮于内容上（沉浸导航） ── */
.topbar {
  position: fixed;
  top: 12px;
  left: 12px;
  z-index: 30;
  height: 40px;
  display: flex;
  align-items: center;
  padding: 0 6px 0 4px;
  border-radius: 999px;
  background: var(--topbar-bg);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-md);
}
.brand-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 32px;
  padding: 0 8px 0 4px;
  border: none;
  border-radius: 999px;
  background: transparent;
  cursor: pointer;
  transition: background 0.15s ease;
}
.brand-btn:hover { background: var(--bg-hover); }
.brand-logo {
  width: 24px;
  height: 24px;
  border-radius: 7px;
  background: linear-gradient(135deg, var(--primary), var(--primary-dark));
  color: #fff;
  font-weight: 700;
  font-size: 11px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--shadow-md);
  flex-shrink: 0;
}
.brand-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
  letter-spacing: -0.01em;
  white-space: nowrap;
}
.brand-caret { font-size: 10px; color: var(--text-tertiary); }
@media (max-width: 480px) {
  .brand-name { display: none; }
}

/* ── 右上角用户胶囊（登录时） ── */
.user-fab {
  position: fixed;
  top: 14px;
  right: 14px;
  z-index: 30;
  padding: 2px;
  border-radius: 50%;
  background: var(--topbar-bg);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-md);
  cursor: pointer;
}
.user-fab-avatar { cursor: pointer; display: block; }
.user-fab-avatar:hover { opacity: 0.9; }

/* 导航下拉菜单（半透明玻璃：var(--menu-bg)，透出滚动内容） */
.nav-menu {
  min-width: 200px;
  border-radius: 10px;
  padding: 4px;
  box-shadow: var(--shadow-lg);
  background: var(--menu-bg) !important;
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid var(--border);
}
.nav-menu :deep(.ant-dropdown-menu-item) {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  border-radius: 6px;
  padding: 8px 12px !important;
}
.nav-menu :deep(.ant-dropdown-menu-item-selected) {
  background: var(--primary-bg) !important;
  color: var(--primary) !important;
  font-weight: 600;
}

/* ── 全宽内容区（浮动胶囊悬浮其上，内容从顶部开始、无占位） ── */
.app-content {
  height: 100vh;
  overflow-y: auto;
  overflow-x: hidden;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
