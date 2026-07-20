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
    }
    return Promise.reject(error)
  }
)

// SSE 连接
export function createSSEConnection(sessionId: string, onMessage: (data: any) => void) {
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
    eventSource.close()
  }

  return eventSource
}

export default api
