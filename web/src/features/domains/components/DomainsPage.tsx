import { useEffect, useState, type FormEvent } from 'react'
import { ExternalLink, Globe, Link2, Trash2, TriangleAlert } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import { domainPublicUrl } from '../../../shared/lib/edge'
import type { DomainRoute, Project, Workload } from '../../../gen/v1/workload_pb'
import { listProjects, listWorkloads } from '../../projects/services/projects-api'
import { assignDomain, listDomains, unassignDomain } from '../services/domains-api'

function busyStatus(status: string) {
  return status === 'pending' || status === 'provisioning' || status === 'deleting'
}

export function DomainsPage() {
  const [routes, setRoutes] = useState<DomainRoute[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [projectId, setProjectId] = useState('')
  const [workloadId, setWorkloadId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [removingId, setRemovingId] = useState('')

  async function refresh() {
    try {
      const [domainRoutes, projectItems] = await Promise.all([listDomains(), listProjects()])
      setRoutes(domainRoutes)
      setProjects(projectItems)
      if (!projectId && projectItems[0]) setProjectId(projectItems[0].id)
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
    }, 4000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!projectId) {
      setWorkloads([])
      setWorkloadId('')
      return
    }
    void listWorkloads(projectId)
      .then((items) => {
        setWorkloads(items)
        setWorkloadId((current) => current && items.some((item) => item.id === current) ? current : (items[0]?.id ?? ''))
      })
      .catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load workloads'))
  }, [projectId])

  async function onAssign(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    const hostname = String(form.get('hostname') ?? '').trim()
    try {
      const route = await assignDomain({
        workloadId: String(form.get('workloadId') ?? workloadId),
        hostname,
        tlsMode: String(form.get('tlsMode') ?? ''),
      })
      const url = domainPublicUrl(route.hostname, route.httpsUrl)
      setNotice(`Domain ${route.hostname} queued (${route.tlsMode}). When status is active, open ${url}`)
      event.currentTarget.reset()
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not assign domain')
    } finally {
      setBusy(false)
    }
  }

  async function onRemove(route: DomainRoute) {
    setRemovingId(route.id)
    setError('')
    try {
      await unassignDomain(route.id)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not remove domain')
    } finally {
      setRemovingId('')
    }
  }

  const provisioning = routes.filter((route) => busyStatus(route.status)).length
  const active = routes.filter((route) => route.status === 'active').length
  const selectedWorkload = workloads.find((item) => item.id === workloadId)

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Domains</h2>
          <p>Publish a running workload on a hostname through the Caddy edge.</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      {provisioning > 0 ? (
        <ActivityBanner
          title={`Provisioning ${provisioning} domain route${provisioning === 1 ? '' : 's'}`}
          detail="Caddy reloads config, checks the upstream, then marks the route active (or rolls back)."
        />
      ) : null}

      {!selectedWorkload?.hostPort && workloadId ? (
        <div className="callout callout-warn" role="status">
          <TriangleAlert size={18} />
          <div>
            <strong>Selected workload has no host port yet</strong>
            <p>Deploy the workload first so the edge can proxy to it. Domains stay pending until a port exists.</p>
          </div>
        </div>
      ) : null}

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Globe /> Routes</div>
          <strong>{loading ? '—' : routes.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Active</div>
          <strong>{loading ? '—' : active}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Provisioning</div>
          <strong>{loading ? '—' : provisioning}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Workloads</div>
          <strong>{workloads.length}</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Assign domain</h3>
            <p className="muted">Local demo: <code>app.localhost</code> + TLS internal → <code>https://app.localhost:8443</code></p>
          </div>
        </header>

        <form className="form-grid form-grid-domains" onSubmit={(event) => void onAssign(event)}>
          <SelectField
            id="domain-project"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
            required
          >
            <option value="" disabled>Select project</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </SelectField>

          <SelectField
            id="domain-workload"
            name="workloadId"
            label="Workload"
            value={workloadId}
            onChange={(event) => setWorkloadId(event.target.value)}
            required
          >
            <option value="" disabled>Select workload</option>
            {workloads.map((workload) => (
              <option key={workload.id} value={workload.id}>
                {workload.name} · {workload.status || 'pending'}
                {workload.hostPort ? ` · :${workload.hostPort}` : ' · no port'}
              </option>
            ))}
          </SelectField>

          <FormField id="hostname" name="hostname" label="Hostname" required placeholder="app.localhost" />

          <SelectField id="tls-mode" name="tlsMode" label="TLS" defaultValue="internal">
            <option value="internal">internal</option>
            <option value="acme">acme</option>
            <option value="">auto</option>
          </SelectField>

          <Button type="submit" disabled={busy || !workloadId}>
            {busy ? 'Assigning…' : 'Assign domain'}
          </Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Domains &amp; routes</h3>
            <p className="muted">Links open the edge listener on port 8443 (HTTPS) or 8088 (HTTP).</p>
          </div>
        </header>

        {loading ? (
          <div className="loading-row" style={{ margin: '0 20px 16px' }}><span className="spinner" aria-hidden /><span>Loading domains…</span></div>
        ) : null}

        {!loading && routes.length === 0 ? (
          <div className="empty-state">
            <Globe />
            <strong>No domains yet</strong>
            <p className="muted">Deploy a workload, then assign a hostname above.</p>
          </div>
        ) : null}

        {routes.length > 0 ? (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Hostname</th>
                  <th>TLS</th>
                  <th>Status</th>
                  <th>Generation</th>
                  <th>Endpoint</th>
                  <th className="align-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {routes.map((route) => {
                  const https = domainPublicUrl(route.hostname, route.httpsUrl)
                  const isBusy = busyStatus(route.status)
                  return (
                    <tr key={route.id}>
                      <td>
                        <strong>{route.hostname}</strong>
                        <div className="mono table-sub">{route.workloadId || route.id}</div>
                      </td>
                      <td className="mono">{route.tlsMode || 'auto'}</td>
                      <td>
                        <span className={`status-chip status-${route.status === 'active' ? 'running' : isBusy ? 'pending' : 'stopped'}`}>
                          {route.status || 'pending'}
                        </span>
                      </td>
                      <td className="mono">{String(route.edgeGeneration)}</td>
                      <td>
                        {route.status === 'active' ? (
                          <a className="endpoint-link" href={https} target="_blank" rel="noreferrer">
                            {https.split('://')[1]} <ExternalLink size={12} />
                          </a>
                        ) : (
                          <span className="muted">{isBusy ? 'Reconciling…' : (route.lastError || '—')}</span>
                        )}
                      </td>
                      <td className="align-right">
                        <div className="row-actions">
                          {route.status === 'active' ? (
                            <Button
                              type="button"
                              variant="secondary"
                              onClick={() => window.open(https, '_blank', 'noopener,noreferrer')}
                            >
                              <Link2 size={14} /> HTTPS
                            </Button>
                          ) : null}
                          <Button
                            variant="danger"
                            type="button"
                            disabled={removingId === route.id}
                            onClick={() => void onRemove(route)}
                          >
                            <Trash2 size={14} /> {removingId === route.id ? '…' : 'Remove'}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>
    </section>
  )
}
