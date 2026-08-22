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

  it('requiresAuth 路由无 user 时重定向 /login', () => {
    seedUser(null)
    const next = fakeNext()
    authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith('/login')
  })

  it('requiresAuth 路由有 user 时放行', () => {
    seedUser('user')
    const next = fakeNext()
    authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith()
  })

  it('公开路由无 user 放行', () => {
    seedUser(null)
    const next = fakeNext()
    authGuard(fakeRoute({}), {} as RouteLocationNormalized, next)
    expect(next).toHaveBeenCalledWith()
  })

  describe('requiresAdmin', () => {
    it('admin 角色放行', () => {
      seedUser('admin')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith()
    })

    it('普通用户（已登录）重定向首页 /', () => {
      seedUser('user')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/')
    })

    it('未登录（无 user）重定向 /login', () => {
      seedUser(null)
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/login')
    })

    it('user localStorage 损坏（非法 JSON）重定向 /login 而非崩溃', () => {
      localStorage.setItem('user', '{broken json')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true, requiresAdmin: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith('/login')
    })

    it('非 admin 路由不受 user 角色影响', () => {
      seedUser('user')
      const next = fakeNext()
      authGuard(fakeRoute({ requiresAuth: true }), {} as RouteLocationNormalized, next)
      expect(next).toHaveBeenCalledWith()
    })
  })
})
