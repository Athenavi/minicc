import api from './index'

// ── Types ──

export interface AdminMetrics {
  concurrent_connections: number
  queue_backlog: number
  cache_hit_rate: number
  api_latency_p99: number
  [key: string]: any
}

export interface AdminUser {
  id: string
  email: string
  name: string
  role: string
  created_at: string
  updated_at: string
}

export interface SystemInfo {
  version: string
  uptime_seconds: number
  database: string
  redis: string
  [key: string]: any
}

export interface StorageConfig {
  backend: string
  config: Record<string, any>
}

export interface RedisInfo {
  mode: string
  stats: Record<string, any>
}

export interface QueueStats {
  task_queue_length: number
  vip_queue_length: number
  consumers: number
  throughput_qps: number
  avg_wait_ms: number
  max_wait_ms: number
  waiting_tasks: QueueTask[]
}

export interface QueueTask {
  task_id: string
  user_id: string
  content: string
  queued_at: string
  position: number
  is_vip: boolean
}

export interface CacheStats {
  l1_hit_rate: number
  l2_hit_rate: number
  l3_hit_rate: number
  total_hit_rate: number
  total_requests: number
  total_hits: number
  total_misses: number
  avg_latency_ms: number
  l1_size: number
  l1_capacity: number
  hot_queries: HotQuery[]
}

export interface HotQuery {
  query: string
  hits: number
  hit_rate: number
  avg_latency_ms: number
}

export interface PerformanceStats {
  gateway: {
    instances: number
    cpu_percent: number
    memory_mb: number
    goroutines: number
    connections: number
    redis_latency_ms: number
    db_latency_ms: number
    uptime_seconds: number
    version: string
  }
  python_engine: {
    pods: number
    cpu_percent: number
    memory_mb: number
    active_tasks: number
    avg_inference_ms: number
    redis_latency_ms: number
    uptime_seconds: number
    version: string
  }
  latency_distribution: Record<string, number>
  qps_trend: { time: string; qps: number }[]
}

export interface ApiKey {
  id: string
  provider: string
  key_preview: string
  status: 'active' | 'rate_limited' | 'circuit_open'
  weight: number
  failures: number
  last_used: string
  remark: string
}

// ── Dashboard ──

export async function getMetrics(): Promise<AdminMetrics> {
  const { data } = await api.get('/v1/admin/metrics')
  return data.data
}

// ── Users ──

export async function listUsers(): Promise<AdminUser[]> {
  const { data } = await api.get('/v1/admin/users')
  return data.data?.users || []
}

export async function getUser(id: string): Promise<AdminUser> {
  const { data } = await api.get(`/v1/admin/users/${id}`)
  return data.data?.user
}

export async function updateUser(id: string, updates: Partial<AdminUser>): Promise<void> {
  await api.put(`/v1/admin/users/${id}`, updates)
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/v1/admin/users/${id}`)
}

// ── System ──

export async function getSystemInfo(): Promise<SystemInfo> {
  const { data } = await api.get('/v1/admin/system')
  return data.data
}

export async function triggerMaintenance(action: 'vacuum' | 'reindex' | 'analyze' | 'flush_cache'): Promise<void> {
  await api.post('/v1/admin/maintenance', { action })
}

export async function downloadBackup(): Promise<Blob> {
  const response = await api.post('/v1/admin/backup', null, { responseType: 'blob' })
  return response.data
}

// ── Storage ──

export async function getStorage(): Promise<StorageConfig> {
  const { data } = await api.get('/v1/admin/storage')
  return data.data
}

export async function updateStorage(config: { backend: string; [key: string]: any }): Promise<void> {
  await api.put('/v1/admin/storage', config)
}

export async function testStorage(): Promise<{ success: boolean; message: string }> {
  const { data } = await api.post('/v1/admin/storage/test')
  return data.data
}

// ── Redis ──

export async function getRedis(): Promise<RedisInfo> {
  const { data } = await api.get('/v1/admin/redis')
  return data.data
}

export async function updateRedis(config: { mode: string; [key: string]: any }): Promise<void> {
  await api.put('/v1/admin/redis', config)
}

export async function testRedis(): Promise<{ success: boolean; message: string }> {
  const { data } = await api.post('/v1/admin/redis/test')
  return data.data
}

// ── Queue ──

export async function getQueueStats(): Promise<QueueStats> {
  const { data } = await api.get('/v1/admin/queue')
  return data.data
}

export async function flushQueue(): Promise<void> {
  await api.post('/v1/admin/queue/flush')
}

export async function pauseQueue(pause: boolean): Promise<void> {
  await api.post('/v1/admin/queue/pause', { pause })
}

// ── Cache ──

export async function getCacheStats(): Promise<CacheStats> {
  const { data } = await api.get('/v1/admin/cache/stats')
  return data.data
}

// ── Performance ──

export async function getPerformance(): Promise<PerformanceStats> {
  const { data } = await api.get('/v1/admin/performance')
  return data.data
}

// ── API Keys ──

export async function listApiKeys(): Promise<ApiKey[]> {
  const { data } = await api.get('/v1/admin/api-keys')
  return data.data?.keys || []
}

export async function addApiKey(key: { provider: string; key: string; remark?: string }): Promise<void> {
  await api.post('/v1/admin/api-keys', key)
}

export async function updateApiKey(id: string, updates: Partial<ApiKey>): Promise<void> {
  await api.put(`/v1/admin/api-keys/${id}`, updates)
}

export async function deleteApiKey(id: string): Promise<void> {
  await api.delete(`/v1/admin/api-keys/${id}`)
}

// ── Settings ──

export async function saveSettings(category: string, config: Record<string, any>): Promise<void> {
  await api.put('/v1/admin/settings', { category, config })
}
