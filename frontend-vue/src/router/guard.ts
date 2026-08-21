import type { RouteLocationNormalized, NavigationGuardNext } from 'vue-router'

/**
 * 全局路由守卫：
 * - requiresAuth：未登录（无 token）→ 重定向 /login
 * - requiresAdmin：非 admin 角色 → 重定向（登录态回首页，未登录回 /login）
 *
 * 角色从 localStorage 持久化的 user 读取（auth store 登录时写入，
 * 页面刷新后 Pinia store 由其初始化，此处直接读 localStorage 避免时序依赖）。
 */
export function authGuard(
  to: RouteLocationNormalized,
  _from: RouteLocationNormalized,
  next: NavigationGuardNext,
) {
  const token = localStorage.getItem('token')

  if (to.meta.requiresAuth && !token) {
    next('/login')
    return
  }

  if (to.meta.requiresAdmin) {
    let role = ''
    try {
      const raw = localStorage.getItem('user')
      role = raw ? (JSON.parse(raw) as { role?: string }).role || '' : ''
    } catch {
      role = ''
    }
    if (role !== 'admin') {
      next(token ? '/' : '/login')
      return
    }
  }

  next()
}
