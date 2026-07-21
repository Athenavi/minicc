<script setup lang="ts">
import { ref, computed, h, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { NLayout, NLayoutSider, NLayoutContent, NMenu, NButton, NAvatar, NDropdown, NIcon, useMessage } from 'naive-ui'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import {
  ChatbubbleOutline,
  PeopleOutline,
  ExtensionPuzzleOutline,
  CardOutline,
  SettingsOutline,
  LogOutOutline,
  PersonOutline,
  ServerOutline,
  ImageOutline,
  PulseOutline,
  BookOutline,
  SunnyOutline,
  MoonOutline,
  GitNetworkOutline,
} from '@vicons/ionicons5'
import type { MenuOption } from 'naive-ui'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const themeStore = useThemeStore()
const collapsed = ref(window.innerWidth <= 768)

// 全局 API 错误监听（AppLayout 在 NMessageProvider 内部渲染）
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

function renderIcon(icon: any) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions = computed<MenuOption[]>(() => [
  {
    label: '对话',
    key: '/chat',
    icon: renderIcon(ChatbubbleOutline),
  },
  {
    label: 'Agent',
    key: '/agents',
    icon: renderIcon(PeopleOutline),
  },
  {
    label: '工作流',
    key: '/workflow',
    icon: renderIcon(GitNetworkOutline),
  },
  {
    label: '技能',
    key: '/skills',
    icon: renderIcon(ExtensionPuzzleOutline),
  },
  {
    label: '媒体库',
    key: '/media',
    icon: renderIcon(ImageOutline),
  },
  {
    label: '知识库',
    key: '/knowledge',
    icon: renderIcon(BookOutline),
  },
  {
    label: '插件',
    key: '/plugins',
    icon: renderIcon(PulseOutline),
  },
  {
    label: '计费',
    key: '/billing',
    icon: renderIcon(CardOutline),
  },
  ...(authStore.isAdmin
    ? [
        {
          label: '管理',
          key: '/admin',
          icon: renderIcon(SettingsOutline),
        },
      ]
    : []),
])

const userMenuOptions = computed(() => [
  {
    label: '个人资料',
    key: 'profile',
    icon: renderIcon(PersonOutline),
  },
  {
    label: themeStore.isDark ? '浅色模式' : '深色模式',
    key: 'toggle-theme',
    icon: renderIcon(themeStore.isDark ? SunnyOutline : MoonOutline),
  },
  {
    label: '退出登录',
    key: 'logout',
    icon: renderIcon(LogOutOutline),
  },
])

function handleMenuUpdate(key: string) {
  router.push(key)
  // 移动端导航后自动关闭侧边栏
  if (window.innerWidth <= 768) {
    collapsed.value = true
  }
}

function handleUserMenu(key: string) {
  if (key === 'logout') {
    authStore.logout()
    router.push('/login')
  } else if (key === 'profile') {
    router.push('/profile')
  } else if (key === 'toggle-theme') {
    themeStore.toggleTheme()
  }
}
</script>

<template>
  <NLayout has-sider style="height: 100vh">
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="0"
      :width="240"
      :collapsed="collapsed"
      show-trigger
      class="nav-sider"
      @collapse="collapsed = true"
      @expand="collapsed = false"
    >
      <div style="padding: 16px; text-align: center">
        <h2 v-if="!collapsed">MiniCC</h2>
        <h2 v-else>MC</h2>
      </div>
      <NMenu
        :collapsed="collapsed"
        :collapsed-width="64"
        :collapsed-icon-size="22"
        :options="menuOptions"
        :value="route.path"
        @update:value="handleMenuUpdate"
      />
      <template #footer>
        <div style="padding: 16px; text-align: center">
          <!-- 已登录用户菜单 -->
          <NDropdown
            v-if="authStore.user"
            :options="userMenuOptions"
            @select="handleUserMenu"
          >
            <NButton quaternary>
              <NAvatar
                round
                size="small"
                :style="{ backgroundColor: '#18a058' }"
              >
                {{ authStore.user.name?.charAt(0)?.toUpperCase() || 'U' }}
              </NAvatar>
              <span v-if="!collapsed" style="margin-left: 8px">
                {{ authStore.user.name || authStore.user.email }}
              </span>
            </NButton>
          </NDropdown>
          <!-- 主题切换按钮（始终显示） -->
          <NButton
            quaternary
            @click="themeStore.toggleTheme()"
            :title="themeStore.isDark ? '切换到浅色模式' : '切换到深色模式'"
            style="margin-top: 8px"
          >
            <template #icon>
              <NIcon :component="themeStore.isDark ? SunnyOutline : MoonOutline" />
            </template>
            <span v-if="!collapsed" style="margin-left: 8px">
              {{ themeStore.isDark ? '浅色模式' : '深色模式' }}
            </span>
          </NButton>
        </div>
      </template>
    </NLayoutSider>
    <!-- 移动端导航菜单按钮 -->
    <button v-if="collapsed" class="nav-menu-btn" @click="collapsed = false" title="打开菜单">☰</button>
    <!-- 移动端导航遮罩 -->
    <div v-if="!collapsed" class="nav-overlay" @click="collapsed = true"></div>
    <NLayoutContent>
      <router-view v-slot="{ Component }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </NLayoutContent>
  </NLayout>
</template>

<style scoped>
/* ── 路由过渡动画 ── */
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
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
    left: auto;
    z-index: 200;
    width: 36px;
    height: 36px;
    border: none;
    background: var(--chat-bg, #fff);
    color: var(--text-color, #333);
    font-size: 20px;
    cursor: pointer;
    border-radius: 8px;
    align-items: center;
    justify-content: center;
    box-shadow: 0 1px 4px rgba(0,0,0,0.1);
  }
  .nav-menu-btn:active {
    background: var(--hover-color, #e8e8ec);
  }
  .nav-sider {
    position: fixed !important;
    top: 0;
    left: 0;
    bottom: 0;
    z-index: 300 !important;
  }
  .nav-sider.n-layout-sider--collapsed {
    transform: translateX(-100%);
  }
  .nav-sider:not(.n-layout-sider--collapsed) {
    transform: translateX(0);
  }
  :deep([class*="toggle-button"]) {
    display: none !important;
  }
  .nav-overlay {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 250;
    background: rgba(0,0,0,0.35);
  }
}
</style>
