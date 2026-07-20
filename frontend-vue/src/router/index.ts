import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    redirect: '/chat',
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/LoginView.vue'),
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('../views/RegisterView.vue'),
  },
  {
    path: '/chat',
    name: 'Chat',
    component: () => import('../views/ChatView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/agents',
    name: 'Agents',
    component: () => import('../views/AgentsView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/skills',
    name: 'Skills',
    component: () => import('../views/SkillsView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/billing',
    name: 'Billing',
    component: () => import('../views/BillingView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/profile',
    name: 'Profile',
    component: () => import('../views/ProfileView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/media',
    name: 'Media',
    component: () => import('../views/MediaView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/workflow',
    name: 'Workflow',
    component: () => import('../views/WorkflowView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/plugins',
    name: 'Plugins',
    component: () => import('../views/PluginsView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/knowledge',
    name: 'Knowledge',
    component: () => import('../views/KnowledgeView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/knowledge/:id',
    name: 'KnowledgeDetail',
    component: () => import('../views/KnowledgeDetailView.vue'),
    meta: { requiresAuth: true },
  },
  // 管理后台路由
  {
    path: '/admin',
    component: () => import('../views/admin/Layout.vue'),
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      {
        path: '',
        redirect: '/admin/dashboard',
      },
      {
        path: 'dashboard',
        name: 'AdminDashboard',
        component: () => import('../views/admin/DashboardView.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'api-keys',
        name: 'AdminApiKeys',
        component: () => import('../views/admin/ApiKeysView.vue'),
        meta: { title: 'API Key 管理' },
      },
      {
        path: 'queue',
        name: 'AdminQueue',
        component: () => import('../views/admin/QueueView.vue'),
        meta: { title: '队列监控' },
      },
      {
        path: 'cache',
        name: 'AdminCache',
        component: () => import('../views/admin/CacheView.vue'),
        meta: { title: '缓存监控' },
      },
      {
        path: 'performance',
        name: 'AdminPerformance',
        component: () => import('../views/admin/PerformanceView.vue'),
        meta: { title: '性能监控' },
      },
      {
        path: 'settings',
        name: 'AdminSettings',
        component: () => import('../views/admin/SettingsView.vue'),
        meta: { title: '系统设置' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 路由守卫
router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')

  if (to.meta.requiresAuth && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
