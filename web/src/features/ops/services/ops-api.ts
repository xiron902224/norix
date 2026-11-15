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

/** Authenticated download URL for a completed backup SQL dump (opens with session Bearer via fetch helper). */
export function backupDownloadUrl(backupId: string) {
  return `${apiBase}/ops/backups/${backupId}/download`
}

/** Fetch backup bytes with session token (anchors cannot send Authorization). */
export async function downloadBackup(backup: BackupItem): Promise<void> {
  const token = sessionStore.getToken()
  const response = await fetch(backupDownloadUrl(backup.ID), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!response.ok) {
    const text = await response.text()
    throw new Error(text.trim() || 'download failed')
  }
  const blob = await response.blob()
  const objectUrl = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectUrl
  anchor.download = backup.Filename || `norix-backup-${backup.ID}.sql`
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(objectUrl)
}

export async function listAlerts() {
  const response = await opsFetch<{ alerts: AlertItem[] }>('/ops/alerts')
  return response.alerts ?? []
}
