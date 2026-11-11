import { useEffect, useState, type FormEvent } from 'react'
import { Network, Shield, Server } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import type { EgressPolicy, GetNetworkStatusResponse, NodeNetworkStatus } from '../../../gen/v1/agent_pb'
import type { Project, Workload } from '../../../gen/v1/workload_pb'
import { listProjects, listWorkloads } from '../../projects/services/projects-api'
import { getNetworkStatus, listEgressPolicies, upsertEgressPolicy } from '../services/network-api'

export function NetworkPage() {
  const [status, setStatus] = useState<GetNetworkStatusResponse | null>(null)
  const [policies, setPolicies] = useState<EgressPolicy[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [projectId, setProjectId] = useState('')
  const [workloadId, setWorkloadId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  async function refresh() {
    try {
      const [networkStatus, egress, projectItems] = await Promise.all([
        getNetworkStatus(),
        listEgressPolicies(),
        listProjects(),
      ])
      setStatus(networkStatus)
      setPolicies(egress)
      setProjects(projectItems)
      if (!projectId && projectItems[0]) setProjectId(projectItems[0].id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load network status')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
    const timer = window.setInterval(() => {
      void getNetworkStatus().then(setStatus).catch(() => { /* keep last */ })
    }, 5000)
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
        setWorkloadId((current) =>
          current && items.some((item) => item.id === current) ? current : (items[0]?.id ?? ''),
        )
      })
      .catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load workloads'))
  }, [projectId])

  async function onEgress(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    const mode = String(form.get('mode') ?? 'allow')
    const allowRaw = String(form.get('allowlist') ?? '')
    const allowlist = allowRaw
      .split(/[\s,]+/)
      .map((item) => item.trim())
      .filter(Boolean)
    try {
      await upsertEgressPolicy({
        workloadId: String(form.get('workloadId') ?? workloadId),
        mode,
        allowlist,
      })
      setNotice(`Egress policy set to ${mode}`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update egress policy')
    } finally {
      setBusy(false)
    }
  }

  const nodes = status?.nodes ?? []
  const records = status?.records ?? []
  const pending = nodes.filter((node) => node.networkStatus === 'pending' || node.networkStatus === 'degraded').length

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Network</h2>
          <p>WireGuard overlay, internal DNS (*.norix.internal), project isolation, and egress policy.</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      {pending > 0 ? (
        <ActivityBanner
          title={`${pending} node network state pending`}
          detail="Localruntime writes mesh configs each reconcile. Set NORIX_APPLY_WIREGUARD=1 to bring up wg-norix."
        />
      ) : null}

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Server /> Nodes ready</div>
          <strong>{loading ? '—' : `${status?.nodesReady ?? 0}/${status?.nodesTotal ?? 0}`}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Network /> DNS records</div>
          <strong>{loading ? '—' : status?.dnsRecords ?? 0}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Shield /> Certificates</div>
          <strong>{loading ? '—' : status?.certificates ?? 0}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Overlay</div>
          <strong>{status?.overlayCidr || '10.88.0.0/16'}</strong>
          <p className="muted">{status?.applyEnabled ? 'apply enabled' : 'dry-run configs only'}</p>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Mesh nodes</h3>
            <p className="muted">Each node gets a WireGuard identity on 10.88.0.0/16.</p>
          </div>
        </header>
        {nodes.length === 0 ? (
          <p className="muted">No nodes yet. Local control-plane node appears after first reconcile.</p>
        ) : (
          <ul className="resource-list">
            {nodes.map((node) => (
              <NodeRow key={node.nodeId} node={node} />
            ))}
          </ul>
        )}
      </section>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Internal DNS</h3>
            <p className="muted">Service discovery as name.project.norix.internal</p>
          </div>
        </header>
        {records.length === 0 ? (
          <p className="muted">Deploy a workload to publish DNS records.</p>
        ) : (
          <ul className="resource-list">
            {records.map((record) => (
              <li key={`${record.name}-${record.type}`}>
                <code>{record.name}</code>
                <span className="muted"> {record.type} → {record.value}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Egress policy</h3>
            <p className="muted">allow / deny / allowlist — rendered into nftables on apply.</p>
          </div>
        </header>
        <form className="form-grid" onSubmit={(event) => void onEgress(event)}>
          <SelectField
            id="net-project"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            <option value="" disabled>Select project</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </SelectField>
          <SelectField
            id="net-workload"
            name="workloadId"
            label="Workload"
            value={workloadId}
            onChange={(event) => setWorkloadId(event.target.value)}
          >
            <option value="" disabled>Select workload</option>
            {workloads.map((workload) => (
              <option key={workload.id} value={workload.id}>{workload.name}</option>
            ))}
          </SelectField>
          <SelectField id="net-mode" name="mode" label="Mode" defaultValue="allow">
            <option value="allow">allow</option>
            <option value="deny">deny</option>
            <option value="allowlist">allowlist</option>
          </SelectField>
          <FormField
            id="net-allowlist"
            label="Allowlist CIDRs"
            name="allowlist"
            placeholder="1.1.1.1/32 8.8.8.8/32"
          />
          <Button type="submit" disabled={busy || !workloadId}>
            {busy ? 'Saving…' : 'Save egress policy'}
          </Button>
        </form>

        {policies.length > 0 ? (
          <ul className="resource-list" style={{ marginTop: '1rem' }}>
            {policies.map((policy) => (
              <li key={policy.id || policy.workloadId}>
                <code>{policy.workloadId.slice(0, 8)}</code>
                <span> {policy.mode}</span>
                {policy.allowlist.length > 0 ? (
                  <span className="muted"> ({policy.allowlist.join(', ')})</span>
                ) : null}
              </li>
            ))}
          </ul>
        ) : null}
      </section>
    </section>
  )
}

function NodeRow({ node }: { node: NodeNetworkStatus }) {
  return (
    <li>
      <strong>{node.name}</strong>
      <span className="muted"> · {node.networkStatus || 'pending'}</span>
      {node.wgAddress ? <div><code>{node.wgAddress}</code></div> : null}
      {node.lastError ? <div className="error-inline">{node.lastError}</div> : null}
    </li>
  )
}
