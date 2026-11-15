import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { ExternalLink, Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import { domainPublicUrl } from '../../../shared/lib/edge'
import type { DomainRoute, Project, Workload } from '../../../gen/v1/workload_pb'
import { listProjects, listWorkloads } from '../../projects/services/projects-api'
import { assignDomain, listDomains, unassignDomain } from '../services/domains-api'

export function DomainsPage() {
  const [routes, setRoutes] = useState<DomainRoute[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [projectId, setProjectId] = useState('')
  const [workloadId, setWorkloadId] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [actionBusy, setActionBusy] = useState('')

  async function refresh() {
    try {
      const [domainRoutes, projectItems] = await Promise.all([listDomains(), listProjects()])
      setRoutes(domainRoutes)
      setProjects(projectItems)
      if (!projectId && projectItems[0]) setProjectId(projectItems[0].id)
      if (selectedId && !domainRoutes.some((route) => route.id === selectedId)) setSelectedId('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load domains')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => {
      void listDomains().then(setRoutes).catch(() => { /* keep */ })
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

  const selected = routes.find((route) => route.id === selectedId) ?? null
  const active = routes.filter((route) => route.status === 'active').length
  const failed = routes.filter((route) => route.status === 'failed').length
  const pending = routes.filter((route) => route.status === 'pending' || route.status === 'provisioning').length

  const rows: ResourceRow[] = useMemo(() => routes.map((route) => {
    const https = domainPublicUrl(route.hostname, route.httpsUrl)
    return {
      id: route.id,
      name: route.hostname,
      subtitle: route.workloadId,
      status: route.status || 'pending',
      values: [
        route.tlsMode || 'auto',
        String(route.edgeGeneration),
        route.status === 'active' ? https.replace(/^https?:\/\//, '') : (route.lastError || '—'),
      ],
    }
  }), [routes])

  async function onAssign(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const route = await assignDomain({
        workloadId: String(form.get('workloadId') ?? workloadId),
        hostname: String(form.get('hostname') ?? '').trim(),
        tlsMode: String(form.get('tlsMode') ?? ''),
      })
      setNotice(`Domain ${route.hostname} queued (${route.tlsMode}).`)
      setModalOpen(false)
      await refresh()
      setSelectedId(route.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not assign domain')
    } finally {
      setBusy(false)
    }
  }

  async function onRetry(route: DomainRoute) {
    setActionBusy(route.id)
    setError('')
    try {
      await assignDomain({
        workloadId: route.workloadId,
        hostname: route.hostname,
        tlsMode: route.tlsMode || 'internal',
      })
      setNotice(`Retry queued for ${route.hostname}`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not retry')
    } finally {
      setActionBusy('')
    }
  }

  async function onRemove(route: DomainRoute) {
    setActionBusy(route.id)
    setError('')
    try {
      await unassignDomain(route.id)
      setSelectedId('')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete')
    } finally {
      setActionBusy('')
    }
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Domains"
        subtitle="Public routing and TLS certificates"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> Add domain
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Domains', value: loading ? '—' : routes.length },
          { label: 'TLS active', value: loading ? '—' : active },
          { label: 'Pending', value: loading ? '—' : pending },
          { label: 'Failed', value: loading ? '—' : failed },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${routes.length} result${routes.length === 1 ? '' : 's'}`}</strong>
        </div>
        <div className="compass-split">
          <ResourceTable
            leadingHeader="Domain"
            headers={['TLS', 'Generation', 'Endpoint']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No domains. Add a hostname to publish a workload."
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a domain to view TLS, errors, and actions.</p>
              </>
            ) : (
              <>
                <h3>{selected.hostname}</h3>
                <p className="muted">{selected.status}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>TLS</dt><dd>{selected.tlsMode || 'auto'}</dd></div>
                  <div><dt>Generation</dt><dd>{String(selected.edgeGeneration)}</dd></div>
                  <div><dt>Workload</dt><dd>{selected.workloadId}</dd></div>
                  <div><dt>Error</dt><dd>{selected.lastError || '—'}</dd></div>
                </dl>
                <div className="page-actions" style={{ marginTop: 16 }}>
                  {selected.status === 'active' ? (
                    <Button type="button" variant="secondary" onClick={() => window.open(domainPublicUrl(selected.hostname, selected.httpsUrl), '_blank', 'noopener,noreferrer')}>
                      <ExternalLink size={14} /> Open HTTPS
                    </Button>
                  ) : (
                    <Button type="button" variant="secondary" disabled={actionBusy === selected.id} onClick={() => void onRetry(selected)}>
                      {actionBusy === selected.id ? '…' : 'Retry'}
                    </Button>
                  )}
                  <Button type="button" variant="danger" disabled={actionBusy === selected.id} onClick={() => void onRemove(selected)}>
                    Delete
                  </Button>
                </div>
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="Add domain"
        submitLabel="Assign domain"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onAssign(event)}
      >
        <SelectField id="domain-project" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          <option value="" disabled>Select</option>
          {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
        </SelectField>
        <SelectField id="domain-workload" name="workloadId" label="Target application" value={workloadId} onChange={(e) => setWorkloadId(e.target.value)} required>
          <option value="" disabled>Select</option>
          {workloads.map((workload) => (
            <option key={workload.id} value={workload.id}>
              {workload.name} · {workload.status || 'pending'}{workload.hostPort ? ` · :${workload.hostPort}` : ''}
            </option>
          ))}
        </SelectField>
        <FormField id="hostname" name="hostname" label="Domain" required placeholder="app.localhost" />
        <SelectField id="tls-mode" name="tlsMode" label="TLS" defaultValue="internal">
          <option value="internal">internal</option>
          <option value="acme">acme</option>
          <option value="">auto</option>
        </SelectField>
      </CreateModal>
    </section>
  )
}
