import api from './index'

// ── 长期记忆（记忆四层架构 L2 档案卡：跨会话留存） ──

export type MemorySlot = 'identity' | 'preference' | 'decision' | 'fact'
export type MemorySource = 'user_confirmed' | 'derived' | 'tool_written'

export interface MemoryEntry {
  id: string
  slot: string
  slot_label: string
  key: string
  value: string
  confidence: number
  source: string
  source_label: string
  has_embedding: boolean
  access_count: number
  last_accessed_at: string | null
  status: string
  created_at: string
  updated_at: string
}

export interface MemorySearchHit extends MemoryEntry {
  similarity: number
  score: number
}

export interface OrganizeResult {
  backfilled: number
  merged: number
  archived: number
  evicted: number
  errors: string[]
}

export interface OrganizeStatus {
  running: boolean
  started_at: number | null
  finished_at: number | null
  result: OrganizeResult | null
  error: string | null
}

export interface ProfileListResponse {
  entries: MemoryEntry[]
  counts: Record<string, number>
  total: number
  slots: { slot: string; label: string }[]
  organize: OrganizeStatus
}

/** 整卡列表（按槽位分组统计） */
export async function listMemory(includeArchived = false): Promise<ProfileListResponse> {
  const { data } = await api.get('/v1/memory/profile', {
    params: includeArchived ? { archived: 'true' } : {},
  })
  return data
}

/** 新建 / 更新一条记忆（同 slot+key 自动更新） */
export async function upsertMemory(body: {
  slot: MemorySlot
  key: string
  value: string
  confidence?: number
  source?: MemorySource
}): Promise<{ entry: MemoryEntry; created: boolean; duplicate_of?: MemoryEntry; evicted?: number }> {
  const { data } = await api.post('/v1/memory/profile', body)
  return data
}

/** 编辑已有记忆（按 id） */
export async function updateMemory(body: {
  id: string
  key?: string
  value?: string
  confidence?: number
  source?: MemorySource
}): Promise<{ entry: MemoryEntry }> {
  const { data } = await api.put('/v1/memory/profile', body)
  return data
}

/** 删除单条记忆 */
export async function deleteMemory(id: string): Promise<void> {
  await api.delete(`/v1/memory/profile/${encodeURIComponent(id)}`)
}

/** 清空全部记忆（需 confirm=true） */
export async function clearMemory(): Promise<{ deleted: number }> {
  const { data } = await api.post('/v1/memory/profile/clear', { confirm: true })
  return data
}

/** 语义检索（相似度 + 重排序；embedding 不可用时降级关键词） */
export async function searchMemory(
  query: string,
  opts?: { top_k?: number; slot?: MemorySlot },
): Promise<{ query: string; mode: 'semantic' | 'keyword'; count: number; results: MemorySearchHit[] }> {
  const { data } = await api.post('/v1/memory/search', {
    query,
    top_k: opts?.top_k ?? 10,
    slot: opts?.slot,
  })
  return data
}

/** 触发异步智能整理（去重 / 归档 / 补嵌入） */
export async function startOrganize(): Promise<{ started: boolean; status: OrganizeStatus }> {
  const { data } = await api.post('/v1/memory/organize')
  return data
}

/** 整理任务状态 */
export async function getOrganizeStatus(): Promise<OrganizeStatus> {
  const { data } = await api.get('/v1/memory/organize/status')
  return data?.status
}

export const MEMORY_SLOTS: { slot: MemorySlot; label: string }[] = [
  { slot: 'identity', label: '身份' },
  { slot: 'preference', label: '偏好' },
  { slot: 'decision', label: '关键决策' },
  { slot: 'fact', label: '长期事实' },
]
