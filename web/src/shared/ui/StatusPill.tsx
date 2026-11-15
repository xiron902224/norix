const TONE: Record<string, string> = {
  running: 'status-running',
  active: 'status-running',
  online: 'status-running',
  ready: 'status-running',
  synced: 'status-running',
  building: 'status-building',
  deploying: 'status-building',
  pending: 'status-pending',
  provisioning: 'status-pending',
  degraded: 'status-pending',
  warning: 'status-pending',
  failed: 'status-failed',
  critical: 'status-failed',
  error: 'status-failed',
  stopped: 'status-stopped',
  idle: 'status-stopped',
  empty: 'status-stopped',
  offline: 'status-stopped',
}

const LABEL: Record<string, string> = {
  running: 'Active',
  active: 'Active',
  online: 'Active',
  ready: 'Active',
  synced: 'Synced',
  building: 'Building',
  deploying: 'Deploying',
  failed: 'Failed',
  stopped: 'Idle',
  idle: 'Idle',
  pending: 'Pending',
  provisioning: 'Provisioning',
  deleting: 'Deleting',
  degraded: 'Degraded',
  offline: 'Offline',
  critical: 'Critical',
  warning: 'Warning',
}

export function statusTone(status: string) {
  const key = (status || '').toLowerCase()
  return TONE[key] || 'status-pending'
}

export function statusLabel(status: string) {
  const key = (status || '').toLowerCase()
  return LABEL[key] || status || 'Pending'
}

export function StatusPill({ status, label }: { status: string; label?: string }) {
  return (
    <span className={`status-chip ${statusTone(status)}`}>
      <span className="status-dot" aria-hidden />
      {label || statusLabel(status)}
    </span>
  )
}
