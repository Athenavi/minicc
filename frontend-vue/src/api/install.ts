import { api } from './index'

export interface InstallDep {
  name: string
  ok: boolean
  message?: string
}

export interface InstallStatus {
  needed: boolean
  reason?: string
  db: boolean
  redis: boolean
  deps?: InstallDep[]
}

export interface SetupResult {
  message: string
  user: { id: string; email: string; name: string; role: string }
}

export async function getInstallStatus(): Promise<InstallStatus> {
  const { data } = await api.get('/v1/install/status')
  return data?.data ?? data
}

export async function setupSystem(body: { email: string; password: string; name: string }): Promise<SetupResult> {
  const { data } = await api.post('/v1/install/setup', body)
  return data?.data ?? data
}
