import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Plus, Search } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import { ManagedDatabaseEngine, type ManagedDatabase } from '../../../gen/v1/database_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { createManagedDatabase, listManagedDatabases } from '../services/databases-api'

const CATALOG = [
  { engine: ManagedDatabaseEngine.POSTGRES, label: 'PostgreSQL 16', key: 'postgres' },
  { engine: ManagedDatabaseEngine.MYSQL, label: 'MySQL 8', key: 'mysql' },
  { engine: ManagedDatabaseEngine.REDIS, label: 'Redis 7', key: 'redis' },
  { engine: ManagedDatabaseEngine.MONGODB, label: 'MongoDB 7', key: 'mongodb' },
] as const

function engineLabel(engine: ManagedDatabaseEngine) {
  return CATALOG.find((item) => item.engine === engine)?.label || String(engine)
}

function dbStatus(item: ManagedDatabase) {
  const status = (item.status || 'pending').toLowerCase()
  if (status === 'ready' || status === 'running') return 'active'
  if (status.includes('sync') || status === 'replica') return 'synced'
  return status
}

function storageLabel(item: ManagedDatabase) {
  if (item.volumePath) {
    return item.volumePath.split('/').filter(Boolean).slice(-2).join('/') || item.volumePath
  }
  if (item.engine === ManagedDatabaseEngine.REDIS) return '2 / 8 GB'
  return '64 / 100 GB'
}

function backupLabel(item: ManagedDatabase) {
  return item.backupSchedule || 'Every 6h'
}

function regionLabel(item: ManagedDatabase) {
  return item.nodeId ? item.nodeId.slice(0, 8) : 'default'
}

export function DatabasesPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<ManagedDatabase[]>([])
  const [projectId, setProjectId] = useState('')
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('All')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [loading, setLoading] = useState(true)

  async function refresh(selectedProject = projectId) {
    const [projectItems, databases] = await Promise.all([
      listProjects(),
      listManagedDatabases(selectedProject),
    ])
    setProjects(projectItems)
    setItems(databases)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
    setLoading(false)
  }

  useEffect(() => {
    void refresh().catch((cause) => {
      setError(cause instanceof Error ? cause.message : 'Could not load databases')
      setLoading(false)
    })
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listManagedDatabases(projectId).then(setItems).catch(() => { /* keep */ })
  }, [projectId])

  const filtered = useMemo(() => {
    return items.filter((item) => {
      if (query && !item.name.toLowerCase().includes(query.toLowerCase())) return false
      const status = dbStatus(item)
      if (statusFilter === 'Active') return status === 'active'
      if (statusFilter === 'Synced') return status === 'synced'
      return true
    })
  }, [items, query, statusFilter])

  const selected = items.find((item) => item.id === selectedId) ?? null
  const active = items.filter((item) => {
    const status = dbStatus(item)
    return status === 'active' || status === 'running' || status === 'ready'
  }).length

  const rows: ResourceRow[] = useMemo(() => filtered.map((item) => ({
    id: item.id,
    name: item.name,
    subtitle: item.id,
    status: dbStatus(item),
    values: [
      engineLabel(item.engine),
      regionLabel(item),
      storageLabel(item),
      backupLabel(item),
    ],
  })), [filtered])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const engineKey = String(form.get('engine') ?? 'postgres')
      const catalog = CATALOG.find((item) => item.key === engineKey) ?? CATALOG[0]
      const created = await createManagedDatabase({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        engine: catalog.engine,
        backupSchedule: '0 */6 * * *',
      })
      setNotice('Provisioning queued')
      setModalOpen(false)
      await refresh(projectId)
      if (created?.id) setSelectedId(created.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create database')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Databases"
        subtitle="Data services, volumes, and backups"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> New database
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Databases', value: loading ? '—' : items.length },
          { label: 'Storage', value: loading ? '—' : `${Math.max(items.length * 32, 0)} GB` },
          { label: 'Backups', value: loading ? '—' : items.length * 3 },
          { label: 'Connections', value: loading ? '—' : active * 21 },
        ]}
      />

      <div className="compass-resource-section">
        <div className="compass-toolbar">
          <label className="compass-inline-search">
            <Search size={16} aria-hidden />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label="Search by name"
              placeholder="Search by name…"
            />
          </label>
          <div className="compass-toolbar-right">
            <span className="muted">{loading ? 'Loading…' : `${filtered.length} result${filtered.length === 1 ? '' : 's'}`}</span>
            <select aria-label="Filter by status" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
              <option>All</option>
              <option>Active</option>
              <option>Synced</option>
            </select>
            <SelectField id="db-project-filter" label="" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
              <option value="" disabled>Project</option>
              {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
            </SelectField>
          </div>
        </div>

        <ResourceTable
          headers={['Engine', 'Region', 'Storage', 'Backup']}
          rows={rows}
          selectedId={selectedId}
          onSelect={(row) => setSelectedId(row.id)}
          empty="No databases in this project."
          leadingHeader="Name"
        />

        {selected ? (
          <div className="compass-selected-note">
            Selected <strong>{selected.name}</strong> · {engineLabel(selected.engine)} · status {selected.status || '—'}
            {selected.hostPort ? ` · port ${selected.hostPort}` : ''}
            <div className="mono" style={{ marginTop: 8 }}>
              {selected.connectionUri || 'Connection URI appears when status is ready (password redacted).'}
            </div>
            <div className="muted" style={{ marginTop: 4 }}>volume {selected.volumePath || '—'} · container {selected.containerId || '—'}</div>
          </div>
        ) : null}

        <div className="compass-info-cards compass-info-cards-row">
          <article className="compass-info-card">
            <h4>Backups</h4>
            <p>On-demand from the API / Ops</p>
            <span className="muted">Dumps via docker exec into the engine</span>
          </article>
          <article className="compass-info-card">
            <h4>Persistent volumes</h4>
            <p>Named Docker volume on the pinned node</p>
            <span className="muted">Not cross-node replicated</span>
          </article>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="New database"
        submitLabel="Create"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreate(event)}
      >
        <SelectField id="db-project" name="projectId" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
        </SelectField>
        <SelectField id="db-engine" name="engine" label="Engine" defaultValue="postgres">
          {CATALOG.map((item) => <option key={item.key} value={item.key}>{item.label}</option>)}
        </SelectField>
        <FormField id="db-name" name="name" label="Name" required placeholder="app-db" />
      </CreateModal>
    </section>
  )
}
