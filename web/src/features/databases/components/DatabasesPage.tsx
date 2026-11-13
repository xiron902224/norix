import { useEffect, useState, type FormEvent } from 'react'
import { Database } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
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

export function DatabasesPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<ManagedDatabase[]>([])
  const [projectId, setProjectId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)

  async function refresh(selectedProject = projectId) {
    const [projectItems, databases] = await Promise.all([
      listProjects(),
      listManagedDatabases(selectedProject),
    ])
    setProjects(projectItems)
    setItems(databases)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
  }

  useEffect(() => {
    void refresh().catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load databases'))
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listManagedDatabases(projectId).then(setItems).catch(() => { /* keep */ })
  }, [projectId])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const engineKey = String(form.get('engine') ?? 'postgres')
      const catalog = CATALOG.find((item) => item.key === engineKey) ?? CATALOG[0]
      await createManagedDatabase({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        engine: catalog.engine,
      })
      setNotice('Database provisioning queued')
      event.currentTarget.reset()
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create database')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Databases</h2>
          <p>1-click managed data services with scheduled S3 backups (S10).</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      <ActivityBanner
        title="Provision records are live"
        detail="Backup/restore are status stubs until volume agents land. Credentials stored encrypted when NORIX_BUILD_SECRETS_KEY is set."
      />

      <div className="stats-grid stats-grid-4">
        {CATALOG.map((item) => (
          <article key={item.key} className="stat-card flux-shadow lift-card">
            <div className="stat-label"><Database /> {item.label}</div>
            <strong className="mono">{item.key}</strong>
          </article>
        ))}
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Create database</h3>
            <p className="muted">Creates a managed_databases row and marks it ready on the next reconcile.</p>
          </div>
        </header>
        <form className="form-grid" onSubmit={(event) => void onCreate(event)}>
          <SelectField
            id="db-project"
            name="projectId"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </SelectField>
          <SelectField id="db-engine" name="engine" label="Engine" defaultValue="postgres">
            {CATALOG.map((item) => (
              <option key={item.key} value={item.key}>{item.label}</option>
            ))}
          </SelectField>
          <FormField id="db-name" name="name" label="Name" required placeholder="app-db" />
          <Button type="submit" disabled={busy || !projectId}>{busy ? 'Creating…' : 'Create database'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Instances</h3>
            <p className="muted">{items.length} in scope</p>
          </div>
        </header>
        {items.length === 0 ? (
          <p className="muted">No managed databases yet.</p>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Engine</th>
                  <th>Status</th>
                  <th>Volume</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <td>
                      <strong>{item.name}</strong>
                      <div className="mono table-sub">{item.id}</div>
                    </td>
                    <td className="mono">{ManagedDatabaseEngine[item.engine]?.toLowerCase() || item.engine}</td>
                    <td><span className="status-chip status-running">{item.status}</span></td>
                    <td className="mono">{item.volumePath || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </section>
  )
}
