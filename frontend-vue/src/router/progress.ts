import { ref } from 'vue'
import type { Router } from 'vue-router'

/**
 * 路由进度条状态：beforeEach 启动，afterEach 结束。
 * 轻量自实现，无外部依赖（nprogress 替代）。
 */
export const routeLoading = ref(false)

let timer: ReturnType<typeof setTimeout> | null = null

export function startProgress() {
  if (timer) clearTimeout(timer)
  // 延迟 50ms 启动，避免快速导航闪烁
  timer = setTimeout(() => { routeLoading.value = true }, 50)
}

export function stopProgress() {
  if (timer) { clearTimeout(timer); timer = null }
  routeLoading.value = false
}

/** 在 router 上挂载进度钩子 */
export function setupRouteProgress(router: Router) {
  router.beforeEach((_to, from, next) => {
    startProgress()
    next()
  })
  router.afterEach(() => {
    stopProgress()
  })
  router.onError(() => {
    stopProgress()
  })
}
