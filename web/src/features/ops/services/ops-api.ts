import { sessionStore } from '../../auth/services/session'

const apiBase = (import.meta.env.VITE_NORIX_API_URL as string | undefined)?.replace(/\/$/, '') || '/api'

async function opsFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const token = sessionStore.getToken()
  const response = await fetch(`${apiBase}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers ?? {}),
    },
  })
  const text = await response.text()
  let body: Record<string, unknown> = {}
  if (text) {
    try {
      body = JSON.parse(text) as Record<string, unknown>
    } catch {
      body = { message: text.trim() }
    }
  }
  if (!response.ok) {
    const message =
      (typeof body.error === 'string' && body.error) ||
      (typeof body.message === 'string' && body.message) ||
      response.statusText ||
      'request failed'
    throw new Error(message)
  }
  return body as T
}

export interface BackupItem {
  ID: string
  Filename: string
  SizeBytes: number
  Status: string
  Message: string
  CreatedBy: string
  CreatedAt: string
}

export interface AlertItem {
  ID: number
  Kind: string
  Severity: string
  Message: string
  ResourceType: string
  ResourceID: string
  Delivered: boolean
  CreatedAt: string
}

export async function listBackups() {
  const response = await opsFetch<{ backups: BackupItem[] }>('/ops/backups')
  return response.backups ?? []
}

export async function createBackup() {
  return opsFetch<{ backup: BackupItem; error?: string }>('/ops/backups', { method: 'POST' })
}

export async function restoreBackup(backupId: string) {
  return opsFetch<{ backup: BackupItem; error?: string }>(`/ops/backups/${backupId}/restore`, { method: 'POST' })
}

export async function listAlerts() {
  const response = await opsFetch<{ alerts: AlertItem[] }>('/ops/alerts')
  return response.alerts ?? []
}
