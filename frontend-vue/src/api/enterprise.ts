import { api } from './index'

// ── 类型（对齐后端 entRoleResponse / entGroupResponse / entUserItem）──

export interface EntRole {
  id: string
  name: string
  display_name: string
  is_builtin: boolean
  permissions: string[]
  user_count?: number
  created_at: string
  updated_at: string
}

export interface EntGroup {
  id: string
  name: string
  description: string
  member_count: number
  role_ids?: string[]
  created_at: string
}

export interface EntUser {
  id: string
  email: string
  name: string
  role: string
  created_at: string
  roles: unknown
  groups: unknown
}

// ── 角色 ──

export async function listRoles(): Promise<EntRole[]> {
  const { data } = await api.get('/v1/ent/roles')
  return data?.data ?? []
}

export async function getRole(id: string): Promise<EntRole> {
  const { data } = await api.get(`/v1/ent/roles/${encodeURIComponent(id)}`)
  return data?.data
}

export async function createRole(body: { name: string; display_name?: string; permissions?: string[] }): Promise<EntRole> {
  const { data } = await api.post('/v1/ent/roles', body)
  return data?.data
}

export async function updateRole(id: string, body: { name?: string; display_name?: string; permissions?: string[] }): Promise<void> {
  await api.put(`/v1/ent/roles/${encodeURIComponent(id)}`, body)
}

export async function deleteRole(id: string): Promise<void> {
  await api.delete(`/v1/ent/roles/${encodeURIComponent(id)}`)
}

// ── 群组 ──

export async function listGroups(): Promise<EntGroup[]> {
  const { data } = await api.get('/v1/ent/groups')
  return data?.data ?? []
}

export async function getGroup(id: string): Promise<EntGroup> {
  const { data } = await api.get(`/v1/ent/groups/${encodeURIComponent(id)}`)
  return data?.data
}

export async function createGroup(body: { name: string; description?: string }): Promise<EntGroup> {
  const { data } = await api.post('/v1/ent/groups', body)
  return data?.data
}

export async function updateGroup(id: string, body: { name?: string; description?: string }): Promise<void> {
  await api.put(`/v1/ent/groups/${encodeURIComponent(id)}`, body)
}

export async function deleteGroup(id: string): Promise<void> {
  await api.delete(`/v1/ent/groups/${encodeURIComponent(id)}`)
}

export async function setGroupMembers(id: string, userIDs: string[]): Promise<void> {
  await api.put(`/v1/ent/groups/${encodeURIComponent(id)}/members`, { user_ids: userIDs })
}

export async function setGroupRoles(id: string, roleIDs: string[]): Promise<void> {
  await api.put(`/v1/ent/groups/${encodeURIComponent(id)}/roles`, { role_ids: roleIDs })
}

// ── 用户（角色/群组绑定）──

export async function listUsers(search: string, page = 1, pageSize = 50): Promise<{ data: EntUser[]; total: number }> {
  const { data } = await api.get('/v1/ent/users', { params: { search, page, page_size: pageSize } })
  return { data: data?.data ?? [], total: data?.meta?.total ?? 0 }
}

export async function setUserRoles(userID: string, roleIDs: string[]): Promise<void> {
  await api.put(`/v1/ent/users/${encodeURIComponent(userID)}/roles`, { role_ids: roleIDs })
}

export async function setUserGroups(userID: string, groupIDs: string[]): Promise<void> {
  await api.put(`/v1/ent/users/${encodeURIComponent(userID)}/groups`, { group_ids: groupIDs })
}
