import { useEffect, useState, type FormEvent } from 'react'
import { Globe, Trash2 } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import type { DomainRoute, Project, Workload } from '../../../gen/v1/workload_pb'
import { listProjects, listWorkloads } from '../../projects/services/projects-api'
import { assignDomain, listDomains, unassignDomain } from '../services/domains-api'

export function DomainsPage() {
  const [routes, setRoutes] = useState<DomainRoute[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [projectId, setProjectId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  async function refresh() {
    setError('')
    try {
      const [domainRoutes, projectItems] = await Promise.all([listDomains(), listProjects()])
      setRoutes(domainRoutes)
      setProjects(projectItems)
      if (!projectId && projectItems[0]) {
        setProjectId(projectItems[0].id)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load domains')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => {
      void listDomains().then(setRoutes).catch(() => { /* keep last */ })
    }, 5000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!projectId) {
      setWorkloads([])
      return
    }
    void listWorkloads(projectId)
      .then(setWorkloads)
      .catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load workloads'))
  }, [projectId])

  async function onAssign(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const route = await assignDomain({
        workloadId: String(form.get('workloadId') ?? ''),
        hostname: String(form.get('hostname') ?? ''),
        tlsMode: String(form.get('tlsMode') ?? ''),
      })
      setNotice(`Domain ${route.hostname} queued (tls=${route.tlsMode}). Edge applies on the next reconcile.`)
      event.currentTarget.reset()
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not assign domain')
    } finally {
      setBusy(false)
    }
  }

  async function onRemove(route: DomainRoute) {
    setBusy(true)
    setError('')
    try {
      await unassignDomain(route.id)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not remove domain')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Domains</h2>
          <p>Map hostnames to running workloads through the Caddy edge (HTTPS).</p>
        </div>
      </header>

      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="notice" role="status">{notice}</div>}

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Assign domain</h3>
            <p className="muted">Local tip: use <code>app.localhost</code> with tls <code>internal</code>, then open <code>https://app.localhost:8443</code>.</p>
          </div>
        </header>
        <form className="form-grid form-grid-4" onSubmit={(event) => void onAssign(event)}>
          <label className="form-field">
            <span>Project</span>
            <select value={projectId} onChange={(event) => setProjectId(event.target.value)} required>
              <option value="" disabled>Select project</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>{project.name}</option>
              ))}
            </select>
          </label>
          <label className="form-field">
            <span>Workload</span>
            <select name="workloadId" required defaultValue="">
              <option value="" disabled>Select workload</option>
              {workloads.map((workload) => (
                <option key={workload.id} value={workload.id}>
                  {workload.name} ({workload.status || 'pending'})
                </option>
              ))}
            </select>
          </label>
          <FormField id="hostname" name="hostname" label="Hostname" required placeholder="app.localhost" />
          <label className="form-field">
            <span>TLS mode</span>
            <select name="tlsMode" defaultValue="">
              <option value="">auto</option>
              <option value="internal">internal</option>
              <option value="acme">acme (Pebble/LE)</option>
            </select>
          </label>
          <Button type="submit" disabled={busy || !workloads.length}>{busy ? 'Saving…' : 'Assign domain'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Routes</h3>
            <p className="muted">Polled every 5s. Status becomes active after Caddy load + upstream health check.</p>
          </div>
        </header>
        {loading ? (
          <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading domains…</span></div>
        ) : null}
        {!loading && routes.length === 0 ? (
          <div className="empty-state">
            <Globe />
            <strong>No domains yet</strong>
            <p className="muted">Deploy a workload first, then assign a hostname.</p>
          </div>
        ) : null}
        <ul className="resource-list">
          {routes.map((route) => (
            <li key={route.id} className="workload-row">
              <div className="resource-link">
                <strong>{route.hostname}</strong>
                <div className="resource-meta">
                  <span className={`status-badge status-${route.status || 'pending'}`}>{route.status}</span>
                  <span className="chip">tls {route.tlsMode}</span>
                  <span className="mono">gen {String(route.edgeGeneration)}</span>
                </div>
                <p className="hint-line">
                  {route.httpsUrl ? (
                    <a className="endpoint-link" href={route.httpsUrl.replace('https://', 'https://').replace(/^(https:\/\/[^/]+)$/, '$1:8443')} target="_blank" rel="noreferrer">
                      {route.httpsUrl}:8443
                    </a>
                  ) : null}
                  {route.lastError ? ` · ${route.lastError}` : ''}
                </p>
              </div>
              <div className="row-actions">
                <Button variant="danger" type="button" disabled={busy} onClick={() => void onRemove(route)}>
                  <Trash2 size={14} /> Remove
                </Button>
              </div>
            </li>
          ))}
        </ul>
      </section>
    </section>
  )
}
