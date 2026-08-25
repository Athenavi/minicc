/* Chiron Service Worker —�?离线�?+ 资源缓存 */
const CACHE = 'chiron-v1'

self.addEventListener('install', (event) => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  )
})

self.addEventListener('fetch', (event) => {
  const { request } = event
  if (request.method !== 'GET') return
  const url = new URL(request.url)

  // 仅缓存同源资源；API/SSE/WS 直连网络
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/v1/') || url.pathname.startsWith('/events') ||
      url.pathname.startsWith('/ws') || url.pathname.startsWith('/media/s/')) return

  // 导航请求：网络优先，离线回退缓存�?  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request).catch(() => caches.match('/index.html'))
    )
    return
  }

  // 静态资源：缓存优先 + 后台更新
  event.respondWith(
    caches.match(request).then((cached) => {
      const network = fetch(request).then((resp) => {
        if (resp && resp.ok) {
          const clone = resp.clone()
          caches.open(CACHE).then((c) => c.put(request, clone))
        }
        return resp
      }).catch(() => cached)
      return cached || network
    })
  )
})
