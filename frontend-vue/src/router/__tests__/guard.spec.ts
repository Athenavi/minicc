import { describe, it, expect, vi, beforeEach } from 'vitest'
import { authGuard } from '../guard'
import type { RouteLocationNormalized, NavigationGuardNext } from 'vue-router'

function fakeRoute(meta: Record<string, unknown>) {
  return { meta, fullPath: '/x' } as unknown as RouteLocationNormalized
}

function fakeNext() {
  const next = vi.fn() as NavigationGuardNext
  return next
}

function seedUser(role: string | null) {
  if (role === null) {
    localStorage.removeItem('user')
  } else {
    localStorage.setItem('user', JSON.stringify({ id: 'u1', email: 'a@b.c', name: 'A', role, tenant_id: 't1' }))
  }
}

describe('authGuard', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('requiresAuth 路由无 token 时重定向 /login', () => {
    localStorage.removeItem('token')
    const next = fakeNext()
    authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith('/login')
  })

  it('requiresAuth 路由有 token 时放行', () => {
    localStorage.setItem('token', 'tok')
    const next = fakeNext()
    authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith()
  })

  it('公开路由无 token 放行', () => {
    localStorage.removeItem('token')
    const next = fakeNext()
    authGuard(fakeRoute({}), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith()
  })

  describe('requiresAdmin', () => {
    it('admin 角色放行', () => {
      localStorage.setItem('token', 'tok')
      seedUser('admin')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith()
    })

    it('普通用户（已登录）重定向首页 /', () => {
      localStorage.setItem('token', 'tok')
      seedUser('user')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/')
    })

    it('未登录（无 token）重定向 /login', () => {
      localStorage.removeItem('token')
      seedUser('admin')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/login')
    })

    it('user localStorage 缺失（仅 token）重定向首页', () => {
      localStorage.setItem('token', 'tok')
      seedUser(null)
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/')
    })

    it('user localStorage 损坏（非法 JSON）重定向首页而非崩溃', () => {
      localStorage.setItem('token', 'tok')
      localStorage.setItem('user', '{broken json')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/')
    })

    it('非 admin 路由不受 user 角色影响', () => {
      localStorage.setItem('token', 'tok')
      seedUser('user')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith()
    })
  })
})
