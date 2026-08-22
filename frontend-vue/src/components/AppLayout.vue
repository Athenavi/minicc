<script setup lang="ts">
import { computed, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
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
  HistoryOutlined,
  RobotOutlined,
  AppstoreOutlined,
  ConsoleSqlOutlined,
} from '@ant-design/icons-vue'
// 全局快速命令：复用 WorkstationNav 的统一执行逻辑（创建 uni 会话 → /chat?task=）
import { executeQuickCommand } from './WorkstationNav.vue'
// 全局命令面板（Ctrl/Cmd+K）：六大工作台切换 / 主题 / 快速命令 / 全局搜索 / 最近活动
import CommandPalette from './CommandPalette.vue'

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
  // 刷新用户信息（角色可能已变更；fetchProfile 失败时 token 无效会自动登出）
  if (authStore.token) {
    authStore.fetchProfile()
  }
})
onUnmounted(() => {
  window.removeEventListener('api:error', handleApiError)
  document.removeEventListener('click', onQuickDocClick)
  window.removeEventListener('keydown', onQuickKeydown)
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
    { key: '/memory', label: '记忆', icon: () => h(HistoryOutlined) },
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

async function handleUserMenuClick(info: any) {
  if (info.key === 'logout') {
    // S 安全：调 authStore.logout 清后端 httpOnly cookie + 本地 user 态
    await authStore.logout()
    router.push('/login')
  } else if (info.key === 'profile') {
    router.push('/profile')
  }
}

// ── 工作台停靠坞：六大工作台全局一键切换（互联互通核心 UX） ──
interface DockItem {
  key: string
  label: string
  /** 一句描述（tooltip 副行，与 WorkstationNav workstations.description 对齐） */
  desc: string
  icon: any
}

const dockItems: DockItem[] = [
  { key: '/chat', label: '对话', desc: '智能对话助手', icon: MessageOutlined },
  { key: '/agents', label: 'Agent', desc: '多智能体协同', icon: RobotOutlined },
  { key: '/workflow', label: '工作流', desc: 'DAG 流程编排', icon: ApartmentOutlined },
  { key: '/skills', label: '技能', desc: '工具与 MCP', icon: ThunderboltOutlined },
  { key: '/knowledge', label: '知识库', desc: 'RAG 检索增强', icon: BookOutlined },
  { key: '/plugins', label: '插件', desc: '扩展能力', icon: AppstoreOutlined },
]

// 仅登录用户可见；首页（/）已有完整工作台总览（WorkstationNav），停靠坞不重复展示
const showDock = computed(() => !!authStore.token && route.path !== '/')

function isDockActive(key: string) {
  return route.path === key || route.path.startsWith(key + '/')
}

function goDock(key: string) {
  if (route.path !== key) router.push(key)
}

// ── 停靠坞快速命令弹层（复用 WorkstationNav 的 executeQuickCommand 逻辑） ──
const quickOpen = ref(false)
const quickInput = ref('')
const quickLoading = ref(false)
const quickPanelEl = ref<HTMLElement | null>(null)
const quickBtnEl = ref<HTMLElement | null>(null)

function onQuickDocClick(e: MouseEvent) {
  const t = e.target as Node
  if (quickPanelEl.value?.contains(t) || quickBtnEl.value?.contains(t)) return
  quickOpen.value = false
}

function onQuickKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') closeQuickCommand()
}

function toggleQuickCommand() {
  quickOpen.value = !quickOpen.value
}

function closeQuickCommand() {
  quickOpen.value = false
  quickInput.value = ''
}

watch(quickOpen, (open) => {
  if (open) {
    document.addEventListener('click', onQuickDocClick)
    window.addEventListener('keydown', onQuickKeydown)
    // 打开后聚焦输入框
    nextTick(() => {
      const input = quickPanelEl.value?.querySelector('input') as HTMLInputElement | null
      input?.focus()
    })
  } else {
    document.removeEventListener('click', onQuickDocClick)
    window.removeEventListener('keydown', onQuickKeydown)
  }
})

// 路由变化时收起弹层（浏览器前进后退、停靠坞切换等）
watch(() => route.path, () => {
  if (quickOpen.value) quickOpen.value = false
})

async function runQuickCommand() {
  const command = quickInput.value.trim()
  if (!command || quickLoading.value) return
  quickLoading.value = true
  try {
    // 与 WorkstationNav 同一套逻辑：创建 uni 会话 → /v1/quick-execute → 跳转 /chat?task=
    await executeQuickCommand(command)
    message.success('任务已提交，正在对话页展示结果')
    closeQuickCommand()
  } catch {
    // executeQuickCommand 已携带 error query 跳转 /chat，无需额外处理
  } finally {
    quickLoading.value = false
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

    <!-- 工作台停靠坞：六大工作台一键切换 + 全局快速命令（玻璃拟态；z-index 低于顶栏） -->
    <nav v-if="showDock" class="dock" aria-label="工作台停靠坞">
      <div class="dock-items">
        <button
          v-for="item in dockItems"
          :key="item.key"
          type="button"
          class="dock-item"
          :class="{ active: isDockActive(item.key) }"
          :aria-label="item.label"
          @click="goDock(item.key)"
        >
          <component :is="item.icon" class="dock-icon" />
          <span class="dock-tip" role="tooltip">
            <span class="dock-tip-name">{{ item.label }}</span>
            <span class="dock-tip-desc">{{ item.desc }}</span>
          </span>
        </button>
      </div>

      <!-- 快速命令入口：点击展开弹层（复用 WorkstationNav 的 executeQuickCommand 逻辑） -->
      <div class="dock-command">
        <button
          ref="quickBtnEl"
          type="button"
          class="dock-command-btn"
          :class="{ open: quickOpen }"
          :aria-label="quickOpen ? '关闭快速命令' : '打开快速命令'"
          :aria-expanded="quickOpen"
          @click="toggleQuickCommand"
        >
          <ConsoleSqlOutlined />
        </button>
        <Transition name="dock-pop">
          <div v-if="quickOpen" ref="quickPanelEl" class="dock-popover" role="dialog" aria-label="快速命令">
            <div class="dock-popover-head">
              <span class="dock-popover-title">快速命令</span>
              <span class="dock-popover-hint">自然语言任务 → 自动编排六大工作台，结果进入统一会话</span>
            </div>
            <div class="dock-command-row">
              <input
                v-model="quickInput"
                type="text"
                class="dock-command-input"
                placeholder="例如：'帮我分析 sales.csv 并生成报告'"
                @keydown.enter="runQuickCommand"
                @keydown.esc="closeQuickCommand"
              />
              <button type="button" class="dock-command-go" :disabled="quickLoading" @click="runQuickCommand">
                <span v-if="quickLoading" class="dock-spinner"></span>
                <span v-else>执行</span>
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </nav>

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

    <!-- 全宽内容区（浮动胶囊悬浮其上，不占位；停靠坞可见时桌面 margin-left / 移动端 padding-bottom 避让） -->
    <main class="app-content" :class="{ 'app-content--docked': showDock }">
      <router-view v-slot="{ Component }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>

    <!-- 全局命令面板（Ctrl/Cmd+K；Teleport 到 body，覆盖所有页面） -->
    <CommandPalette />
  </div>
</template>

<style scoped>
.app-shell {
  position: relative;
  height: 100vh;
  background: var(--bg-page);
  /* 停靠坞尺寸令牌（桌面侧栏 60px / 移动端底部横条 56px） */
  --dock-w: 60px;
  --dock-h: 56px;
  --dock-gap: 12px;
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

/* ── 工作台停靠坞：玻璃拟态侧栏（互联互通核心 UX） ── */
.dock {
  position: fixed;
  top: 64px;            /* 避开左上角品牌胶囊（顶部 12 + 高 40） */
  left: 10px;
  bottom: 10px;
  z-index: 20;          /* 低于顶栏(30)：快速命令弹层、tooltip 在其上 */
  width: var(--dock-w);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 10px 8px;
  border-radius: 16px;
  background: var(--menu-bg);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-md);
  overflow-y: auto;
  scrollbar-width: none;
}
.dock::-webkit-scrollbar { display: none; }

.dock-items {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

/* 触控目标 44px（≥40px 要求）；active 用 --primary 色底 */
.dock-item {
  position: relative;
  width: 44px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 10px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease, transform 0.08s ease;
}
.dock-item:hover { background: var(--bg-hover); color: var(--text-primary); }
.dock-item:active { transform: scale(0.94); }
.dock-item:focus-visible,
.dock-command-btn:focus-visible,
.brand-btn:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}
.dock-item.active {
  background: var(--primary);
  color: #fff;
  box-shadow: 0 4px 14px var(--primary-bg), inset 0 1px 1px hsla(0, 0%, 100%, 0.25);
}
/* 激活项 2px 指示条（桌面：左侧竖向；scaleY 入场动画） */
.dock-item.active::before {
  content: '';
  position: absolute;
  left: -9px;
  top: 50%;
  width: 2px;
  height: 20px;
  border-radius: 1px;
  background: var(--primary);
  transform: translateY(-50%) scaleY(0);
  transform-origin: center;
  animation: dockBarIn 0.22s ease-out forwards;
}
@keyframes dockBarIn {
  to { transform: translateY(-50%) scaleY(1); }
}
.dock-icon { font-size: 18px; }

/* hover 名称 tooltip（仅指针设备；触屏无 hover 语义，保留 aria-label） */
.dock-tip {
  position: absolute;
  left: calc(100% + 10px);
  top: 50%;
  transform: translateY(-50%) translateX(-4px);
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 5px 10px;
  border-radius: 6px;
  background: var(--menu-bg);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-md);
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.15s ease, transform 0.15s ease;
  z-index: 25;
}
.dock-tip-name { line-height: 16px; }
.dock-tip-desc { font-size: 11px; font-weight: 400; color: var(--text-tertiary); line-height: 15px; }
.dock-item:hover .dock-tip { opacity: 1; transform: translateY(-50%) translateX(0); }
@media (hover: none) { .dock-tip { display: none; } }

/* ── 快速命令入口（停靠坞底部）+ 弹层 ── */
.dock-command {
  position: relative;
  margin-top: auto;
}
.dock-command-btn {
  width: 44px;
  height: 44px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--border);
  border-radius: 10px;
  background: var(--bg-hover);
  color: var(--text-secondary);
  font-size: 17px;
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}
.dock-command-btn:hover,
.dock-command-btn.open {
  color: var(--primary);
  border-color: var(--primary);
  background: var(--primary-bg);
}

.dock-popover {
  position: absolute;
  left: calc(100% + 10px);
  bottom: 0;
  width: 340px;
  padding: 12px;
  border-radius: 12px;
  background: var(--menu-bg);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid var(--border);
  box-shadow: var(--shadow-lg);
  z-index: 25;          /* 高于停靠坞(20)、低于顶栏(30) */
}
.dock-popover-head { margin-bottom: 10px; }
.dock-popover-title {
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
}
.dock-popover-hint {
  display: block;
  margin-top: 2px;
  font-size: 11px;
  line-height: 16px;
  color: var(--text-tertiary);
}
.dock-command-row { display: flex; gap: 8px; }
.dock-command-input {
  flex: 1;
  min-width: 0;
  min-height: 40px;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-primary);
  font-size: 13px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.dock-command-input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px var(--primary-bg);
}
.dock-command-input::placeholder { color: var(--text-muted); }
.dock-command-go {
  flex: none;
  min-height: 40px;
  min-width: 64px;
  padding: 0 14px;
  border: none;
  border-radius: 8px;
  background: var(--primary);
  color: #fff;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.2s ease, opacity 0.2s ease;
}
.dock-command-go:hover:not(:disabled) { background: var(--primary-dark); }
.dock-command-go:disabled { opacity: 0.6; cursor: not-allowed; }
.dock-spinner {
  display: inline-block;
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.35);
  border-top-color: #fff;
  border-radius: 50%;
  animation: dockSpin 0.6s linear infinite;
}
@keyframes dockSpin { to { transform: rotate(360deg); } }

/* 弹层过渡 */
.dock-pop-enter-active,
.dock-pop-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}
.dock-pop-enter-from,
.dock-pop-leave-to {
  opacity: 0;
  transform: translateY(4px);
}

/* 内容区自动避让：桌面 margin-left（悬浮侧栏 10 + 60 + 间距 12） */
.app-content--docked {
  margin-left: calc(10px + var(--dock-w) + var(--dock-gap));
}

/* 移动端（≤768px）：停靠坞改为底部悬浮横条；内容区 padding-bottom 避让 */
@media (max-width: 768px) {
  .dock {
    top: auto;
    left: 10px;
    right: 10px;
    bottom: calc(10px + env(safe-area-inset-bottom, 0px));
    width: auto;
    height: var(--dock-h);
    flex-direction: row;
    align-items: center;
    gap: 4px;
    padding: 8px 10px;
    border-radius: 16px;
  }
  .dock-items {
    flex: 1;
    flex-direction: row;
    justify-content: space-around;
    gap: 2px;
    min-width: 0;
  }
  .dock-item { width: 40px; height: 40px; }
  /* 移动端：指示条改为顶部横向（scaleX 入场） */
  .dock-item.active::before {
    left: 50%;
    top: -7px;
    width: 20px;
    height: 2px;
    transform: translateX(-50%) scaleX(0);
    animation-name: dockBarInX;
  }
  @keyframes dockBarInX {
    to { transform: translateX(-50%) scaleX(1); }
  }
  .dock-tip { display: none; }
  .dock-command { margin-top: 0; }
  .dock-command-btn { width: 40px; height: 40px; }
  .dock-popover {
    position: absolute;
    left: auto;
    right: 0;
    bottom: calc(100% + 10px);
    width: min(340px, calc(100vw - 44px));
  }
  .app-content--docked {
    margin-left: 0;
    padding-bottom: calc(10px + var(--dock-h) + var(--dock-gap) + env(safe-area-inset-bottom, 0px));
  }
}

@media (prefers-reduced-motion: reduce) {
  .dock-item,
  .dock-command-btn,
  .dock-command-input,
  .dock-command-go,
  .dock-tip {
    transition: none;
  }
  .dock-item.active::before { animation: none; transform: translateY(-50%) scaleY(1); }
  @media (max-width: 768px) {
    .dock-item.active::before { animation: none; transform: translateX(-50%) scaleX(1); }
  }
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
