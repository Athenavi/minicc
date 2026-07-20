import axios from 'axios'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

export const api = axios.create({
  baseURL: API_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// 请求拦截器：添加 Token
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器：处理错误
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Token 过期，清除并跳转登录
      localStorage.removeItem('token')
      window.location.href = '/login'
    } else if (error.response?.status >= 500) {
      console.error('Server error:', error.response.status, error.response.data)
      // 触发全局错误事件，App.vue 中的监听器会显示提示
      window.dispatchEvent(new CustomEvent('api:error', {
        detail: { message: `服务器错误 (${error.response.status})，请稍后重试` }
      }))
    } else if (error.code === 'ECONNABORTED' || !error.response) {
      // 网络超时或无法连接
      window.dispatchEvent(new CustomEvent('api:error', {
        detail: { message: '网络连接失败，请检查网络后重试' }
      }))
    }
    return Promise.reject(error)
  }
)

// SSE 连接
export function createSSEConnection(sessionId: string, onMessage: (data: any) => void, onError?: () => void) {
  const url = `${API_URL}/events?session_id=${encodeURIComponent(sessionId)}`

  const eventSource = new EventSource(url)

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      onMessage(data)
    } catch (e) {
      console.error('SSE parse error:', e)
    }
  }

  eventSource.onerror = (error) => {
    console.error('SSE error:', error)
    onError?.()
    eventSource.close()
  }

  return eventSource
}

export default api
