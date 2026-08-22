import type { RouteLocationNormalized, NavigationGuardNext } from 'vue-router'

/**
 * 全局路由守卫：
 * - requiresAuth：未登录（无 user）→ 重定向 /login
 * - requiresAdmin：非 admin 角色 → 重定向（登录态回首页，未登录回 /login）
 *
 * S 安全：登录态凭 localStorage 持久化的 user 判断（token 已迁至 httpOnly cookie，
 * JS 不可读）。user 仅含 {id,email,name,role,tenant_id}，无敏感凭据。
 */
export function authGuard(
  to: RouteLocationNormalized,
  _from: RouteLocationNormalized,
  next: NavigationGuardNext,
) {
  let user: { role?: string } | null = null
  try {
    const raw = localStorage.getItem('user')
    user = raw ? JSON.parse(raw) : null
  } catch {
    user = null
  }

  if (to.meta.requiresAuth && !user) {
    next('/login')
    return
  }

  if (to.meta.requiresAdmin) {
    if (user?.role !== 'admin' && user?.role !== 'owner') {
      next(user ? '/' : '/login')
      return
    }
  }

  next()
}
