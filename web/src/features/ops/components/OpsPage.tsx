import { useEffect, useState } from 'react'
import { Bell, DatabaseBackup, Download, HardDriveDownload, RefreshCw } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import {
  createBackup,
  downloadBackup,
  listAlerts,
  listBackups,
  restoreBackup,
  type AlertItem,
  type BackupItem,
} from '../services/ops-api'

function formatBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let idx = 0
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx += 1
  }
  return `${size.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`
}

export function OpsPage() {
  const [backups, setBackups] = useState<BackupItem[]>([])
  const [alerts, setAlerts] = useState<AlertItem[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')

  async function refresh() {
    try {
      const [backupItems, alertItems] = await Promise.all([listBackups(), listAlerts()])
      setBackups(backupItems)
      setAlerts(alertItems)
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load ops data')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 8000)
    return () => window.clearInterval(timer)
  }, [])

  async function onBackup() {
    setBusy('backup')
    setNotice('')
    setError('')
    try {
      const result = await createBackup()
      setNotice(result.error ? `Backup finished with issues: ${result.error}` : `Backup created: ${result.backup.Filename}`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Backup failed')
    } finally {
      setBusy('')
    }
  }

  async function onRestore(backup: BackupItem) {
    if (!window.confirm(`Restore ${backup.Filename}? This applies SQL onto the current database.`)) return
    setBusy(backup.ID)
    setNotice('')
    setError('')
    try {
      const result = await restoreBackup(backup.ID)
      setNotice(result.error ? `Restore issues: ${result.error}` : `Restored ${backup.Filename}`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Restore failed')
    } finally {
      setBusy('')
    }
  }

  async function onDownload(backup: BackupItem) {
    setBusy(`dl-${backup.ID}`)
    setError('')
    try {
      await downloadBackup(backup)
      setNotice(`Downloading ${backup.Filename}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Download failed')
    } finally {
      setBusy('')
    }
  }

  const completed = backups.filter((item) => item.Status === 'completed' || item.Status === 'restored')
  const lastBackup = completed[0]

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Operations</h2>
          <p>PostgreSQL snapshots, alert dispatch, and Alpha restore path (Stitch Domains &amp; Ops).</p>
        </div>
        <div className="page-actions">
          <Button variant="secondary" type="button" onClick={() => void refresh()}>
            <RefreshCw size={14} /> Refresh
          </Button>
          <Button type="button" disabled={busy === 'backup'} onClick={() => void onBackup()}>
            <DatabaseBackup size={14} /> {busy === 'backup' ? 'Backing up…' : 'Trigger Backup'}
          </Button>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}
      {busy === 'backup' ? (
        <ActivityBanner title="Creating Postgres backup" detail="Running pg_dump into NORIX_BACKUP_DIR." />
      ) : null}

      <div className="metric-grid">
        <article className="metric-card">
          <span className="label-sm">PostgreSQL backups</span>
          <strong className="mono">{completed.length}</strong>
          <p className="muted">completed archives</p>
        </article>
        <article className="metric-card">
          <span className="label-sm">Last snapshot</span>
          <strong className="mono">{lastBackup ? formatBytes(lastBackup.SizeBytes) : '—'}</strong>
          <p className="muted">{lastBackup ? new Date(lastBackup.CreatedAt).toLocaleString() : 'none yet'}</p>
        </article>
        <article className="metric-card">
          <span className="label-sm">Open alerts</span>
          <strong className="mono">{alerts.length}</strong>
          <p className="muted">node / deploy events</p>
        </article>
      </div>

      <div className="ops-grid">
        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>PostgreSQL automated snapshots</h3>
              <p className="muted">Download SQL dumps or restore into the current database.</p>
            </div>
            <HardDriveDownload size={16} className="muted-icon" aria-hidden />
          </header>
          {loading ? <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading backups…</span></div> : null}
          {!loading && backups.length === 0 ? (
            <div className="empty-state compact">
              <DatabaseBackup />
              <strong>No backups yet</strong>
              <p className="muted">Trigger a backup to verify the Alpha restore path.</p>
            </div>
          ) : null}
          <div className="table-wrap">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Archive</th>
                  <th>Status</th>
                  <th>Size</th>
                  <th>Created</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {backups.map((backup) => {
                  const canDownload = backup.Status === 'completed' || backup.Status === 'restored'
                  return (
                    <tr key={backup.ID}>
                      <td className="mono">{backup.Filename}</td>
                      <td>
                        <span className={`status-badge status-${canDownload ? 'running' : backup.Status === 'failed' ? 'failed' : 'pending'}`}>
                          {backup.Status}
                        </span>
                      </td>
                      <td className="mono">{formatBytes(backup.SizeBytes)}</td>
                      <td className="muted">{new Date(backup.CreatedAt).toLocaleString()}</td>
                      <td>
                        <div className="row-actions">
                          {canDownload ? (
                            <Button
                              type="button"
                              variant="secondary"
                              disabled={busy === `dl-${backup.ID}`}
                              onClick={() => void onDownload(backup)}
                            >
                              <Download size={14} /> {busy === `dl-${backup.ID}` ? '…' : 'Download'}
                            </Button>
                          ) : (
                            <span className="muted">—</span>
                          )}
                          <Button
                            type="button"
                            variant="secondary"
                            disabled={busy === backup.ID || backup.Status === 'failed'}
                            onClick={() => void onRestore(backup)}
                          >
                            {busy === backup.ID ? 'Restoring…' : 'Restore'}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </section>

        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Emergency alert webhooks</h3>
              <p className="muted">Node down and deploy failed. Optional: <code>NORIX_ALERT_WEBHOOK_URL</code>.</p>
            </div>
            <Bell size={16} className="muted-icon" aria-hidden />
          </header>
          {loading ? <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading alerts…</span></div> : null}
          {!loading && alerts.length === 0 ? (
            <div className="empty-state compact">
              <Bell />
              <strong>No alerts</strong>
              <p className="muted">Stop an agent for ~45s to generate a node.down alert.</p>
            </div>
          ) : null}
          <ul className="resource-list">
            {alerts.map((alert) => (
              <li key={alert.ID}>
                <div className="resource-link">
                  <strong>{alert.Kind}</strong>
                  <div className="resource-meta">
                    <span className={`status-badge status-${alert.Severity === 'critical' ? 'failed' : 'pending'}`}>{alert.Severity}</span>
                    <span className="chip">{alert.ResourceType || 'system'}</span>
                  </div>
                  <p className="hint-line">{alert.Message} · {new Date(alert.CreatedAt).toLocaleString()}</p>
                </div>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </section>
  )
}
