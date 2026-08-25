import { createRouter, createWebHistory } from 'vue-router'
import { authGuard } from './guard'
import { setupRouteProgress } from './progress'

const routes = [
  {
    path: '/',
    name: 'Home',
    component: () => import('../views/HomeView.vue'),
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
    path: '/install',
    name: 'Install',
    component: () => import('../views/InstallView.vue'),
    // 安装向导：无需登录态，跳过 guard 的 profile 拉取（安装模式下该接口返回 503）
    meta: { public: true },
  },
  {
    path: '/chat',
    name: 'Chat',
    component: () => import('../views/ChatView.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/share/:id',
    name: 'Share',
    component: () => import('../views/ShareView.vue'),
    // 注意：无 requiresAuth — 共享会话可能公开访问，后端应验证 share_id 的权限范围
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
    path: '/memory',
    name: 'Memory',
    component: () => import('../views/MemoryView.vue'),
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
      {
        path: 'tenants',
        name: 'AdminTenants',
        component: () => import('../views/admin/TenantManagementView.vue'),
        meta: { title: '租户管理' },
      },
      {
        path: 'redis',
        name: 'AdminRedis',
        component: () => import('../views/admin/RedisManagementView.vue'),
        meta: { title: 'Redis 管理' },
      },
      {
        path: 'database',
        name: 'AdminDatabase',
        component: () => import('../views/admin/DatabaseManagementView.vue'),
        meta: { title: '数据库管理' },
      },
      {
        path: 'domains',
        name: 'AdminDomains',
        component: () => import('../views/admin/DomainManagementView.vue'),
        meta: { title: '域名管理' },
      },
      {
        path: 'oauth-providers',
        name: 'AdminOAuthProviders',
        component: () => import('../views/admin/OAuthProvidersView.vue'),
        meta: { title: '三方登录与人机验证' },
      },
      {
        path: 'audit',
        name: 'AdminAudit',
        component: () => import('../views/admin/AuditView.vue'),
        meta: { title: '操作审计' },
      },
      {
        path: 'roles',
        name: 'AdminRoles',
        component: () => import('../views/admin/RolesView.vue'),
        meta: { title: '角色管理' },
      },
      {
        path: 'groups',
        name: 'AdminGroups',
        component: () => import('../views/admin/GroupsView.vue'),
        meta: { title: '群组管理' },
      },
      {
        path: 'costcenter',
        name: 'AdminCostCenter',
        component: () => import('../views/admin/CostCenterView.vue'),
        meta: { title: '成本中心' },
      },
      {
        path: 'privacy',
        name: 'AdminPrivacy',
        component: () => import('../views/admin/PrivacyView.vue'),
        meta: { title: '隐私模式管控' },
      },
      {
        path: 'model-policy',
        name: 'AdminModelPolicy',
        component: () => import('../views/admin/ModelPolicyView.vue'),
        meta: { title: '模型策略管控' },
      },
      {
        path: 'market',
        name: 'AdminMarket',
        component: () => import('../views/admin/MarketView.vue'),
        meta: { title: '企业能力市场' },
      },
      {
        path: 'api-docs',
        name: 'AdminApiDocs',
        component: () => import('../views/admin/ApiDocsView.vue'),
        meta: { title: 'API 文档' },
      },
    ],
  },
  // 404 兜底：避免未知地址白屏
  {
    path: '/:pathMatch(.*)*',
    name: 'NotFound',
    component: () => import('../views/NotFoundView.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 路由守卫（逻辑在 guard.ts，独立可测）
router.beforeEach(authGuard)

// 路由进度条
setupRouteProgress(router)

export default router
