import type { RouteLocationNormalized, NavigationGuardNext } from 'vue-router'
import { useAuthStore } from '../stores/auth'

/**
 * 全局路由守卫：
 * - requiresAuth：未登录（无 user）→ 重定向 /login
 * - requiresAdmin：非 admin/owner 角色 → 重定向（登录态回首页，未登录回 /login）
 *
 * S 安全：token 已迁至 httpOnly cookie，JS 不可读。admin 权限**不得**信任
 * localStorage.role（可被用户篡改为 owner）。每次进入 admin 页都向后端
 * /v1/auth/profile 拉取权威角色，防止伪 admin 借守卫放行敏感管理接口。
 */
export async function authGuard(
  to: RouteLocationNormalized,
  _from: RouteLocationNormalized,
  next: NavigationGuardNext,
) {
  const auth = useAuthStore()

  // admin 访问（或本地无会话态）时，以 httpOnly cookie 向后端拉取权威 profile。
  // fetchProfile 内部失败会登出；此处不抛错，继续按现有本地态降级判定。
  // meta.public 的页面（如安装向导）不需要登录态，跳过 profile 拉取
  // （安装模式下 /v1/auth/profile 返回 503，避免触发全局错误提示）。
  if (to.meta.requiresAdmin || (!auth.user && !to.meta.public)) {
    try {
      await auth.fetchProfile()
    } catch {
      /* fetchProfile 已内部登出 */
    }
  }

  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    next('/login')
    return
  }

  if (to.meta.requiresAdmin) {
    const role = auth.user?.role
    if (role !== 'admin' && role !== 'owner') {
      next(auth.isAuthenticated ? '/' : '/login')
      return
    }
  }

  next()
}