import { useEffect, useMemo, useState } from 'react'
import { DatabaseBackup, Download, RefreshCw } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
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
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')

  async function refresh() {
    try {
      const [backupItems, alertItems] = await Promise.all([listBackups(), listAlerts()])
      setBackups(backupItems)
      setAlerts(alertItems)
      if (selectedId && !backupItems.some((item) => item.ID === selectedId)) setSelectedId('')
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load operations')
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
      setNotice(result.error
        ? `Backup with issues: ${result.error}`
        : `Backup created: ${result.backup.Filename}`)
      await refresh()
      if (result.backup?.ID) setSelectedId(result.backup.ID)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Backup failed')
    } finally {
      setBusy('')
    }
  }

  async function onRestore(backup: BackupItem) {
    if (!window.confirm(`Restore ${backup.Filename}? SQL will be applied to the current database.`)) return
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
  const selected = backups.find((item) => item.ID === selectedId) ?? null
  const canDownloadSelected = selected
    ? selected.Status === 'completed' || selected.Status === 'restored'
    : false

  const rows: ResourceRow[] = useMemo(() => backups.map((backup) => {
    const raw = backup.Status || 'pending'
    const status = raw === 'completed' || raw === 'restored' ? 'active' : raw === 'failed' ? 'failed' : 'pending'
    return {
      id: backup.ID,
      name: backup.Filename,
      subtitle: backup.ID,
      status,
      values: [
        formatBytes(backup.SizeBytes),
        new Date(backup.CreatedAt).toLocaleString(),
      ],
    }
  }), [backups])

  return (
    <section className="compass-page">
      <PageHero
        title="Operations"
        subtitle="PostgreSQL snapshots, alerts, and restore path"
        actions={(
          <>
            <Button variant="secondary" type="button" onClick={() => void refresh()}>
              <RefreshCw size={14} /> Refresh
            </Button>
            <Button type="button" disabled={busy === 'backup'} onClick={() => void onBackup()}>
              <DatabaseBackup size={14} /> {busy === 'backup' ? 'Creating…' : 'Create backup'}
            </Button>
          </>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      {busy === 'backup' ? (
        <div className="callout callout-warn" role="status">
          <div>
            <strong>Creating Postgres backup</strong>
            <p>Running pg_dump in NORIX_BACKUP_DIR.</p>
          </div>
        </div>
      ) : null}

      <MetricStrip
        items={[
          { label: 'Backups', value: loading ? '—' : completed.length },
          { label: 'Last size', value: lastBackup ? formatBytes(lastBackup.SizeBytes) : '—' },
          { label: 'Open alerts', value: loading ? '—' : alerts.length },
          { label: 'Total files', value: loading ? '—' : backups.length },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${backups.length} result${backups.length === 1 ? '' : 's'}`}</strong>
          <span className="muted">Poll every 8s</span>
        </div>
        <div className="compass-split">
          <ResourceTable
            leadingHeader="File"
            headers={['Size', 'Created']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No backups. Create one to verify the restore path."
          />
          <aside className="compass-side-panel">
            {selected ? (
              <>
                <h3>{selected.Filename}</h3>
                <p className="muted">{selected.Status}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>ID</dt><dd className="mono">{selected.ID}</dd></div>
                  <div><dt>Size</dt><dd>{formatBytes(selected.SizeBytes)}</dd></div>
                  <div><dt>Created</dt><dd>{new Date(selected.CreatedAt).toLocaleString()}</dd></div>
                  <div><dt>Message</dt><dd>{selected.Message || '—'}</dd></div>
                </dl>
                <div className="page-actions" style={{ marginTop: 16, flexWrap: 'wrap' }}>
                  {canDownloadSelected ? (
                    <Button
                      type="button"
                      variant="secondary"
                      disabled={busy === `dl-${selected.ID}`}
                      onClick={() => void onDownload(selected)}
                    >
                      <Download size={14} /> {busy === `dl-${selected.ID}` ? '…' : 'Download'}
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    variant="secondary"
                    disabled={busy === selected.ID || selected.Status === 'failed'}
                    onClick={() => void onRestore(selected)}
                  >
                    {busy === selected.ID ? 'Restoring…' : 'Restore'}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <h3>Alerts</h3>
                <p className="muted">Node down and failed deploy. Optional: NORIX_ALERT_WEBHOOK_URL.</p>
                {alerts.length === 0 ? (
                  <p className="muted" style={{ marginTop: 12 }}>No alerts. For a node.down, stop an agent ~45s.</p>
                ) : (
                  <ul className="resource-list" style={{ marginTop: 12 }}>
                    {alerts.map((alert) => (
                      <AlertRow key={alert.ID} alert={alert} />
                    ))}
                  </ul>
                )}
                <p className="muted" style={{ marginTop: 16 }}>Select a backup to download or restore.</p>
              </>
            )}
            {selected && alerts.length > 0 ? (
              <>
                <h4 style={{ marginTop: 20 }}>Recent alerts</h4>
                <ul className="resource-list" style={{ marginTop: 8 }}>
                  {alerts.slice(0, 5).map((alert) => (
                    <AlertRow key={alert.ID} alert={alert} />
                  ))}
                </ul>
              </>
            ) : null}
          </aside>
        </div>
      </div>
    </section>
  )
}

function AlertRow({ alert }: { alert: AlertItem }) {
  return (
    <li>
      <div className="resource-link">
        <strong>{alert.Kind}</strong>
        <div className="resource-meta">
          <span className={`status-badge status-${alert.Severity === 'critical' ? 'failed' : 'pending'}`}>
            {alert.Severity}
          </span>
          <span className="chip">{alert.ResourceType || 'system'}</span>
        </div>
        <p className="hint-line">{alert.Message} · {new Date(alert.CreatedAt).toLocaleString()}</p>
      </div>
    </li>
  )
}
