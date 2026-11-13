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

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>WireGuard mesh nodes</h3>
            <p className="muted">Each node gets a WireGuard identity on 10.88.0.0/16.</p>
          </div>
        </header>
        {nodes.length === 0 ? (
          <p className="muted" style={{ margin: '0 20px 16px' }}>No nodes yet. Local control-plane node appears after first reconcile.</p>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Network status</th>
                  <th>WG address</th>
                  <th>Error</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((node) => (
                  <NodeTableRow key={node.nodeId} node={node} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Internal DNS</h3>
            <p className="muted">Service discovery as name.project.norix.internal</p>
          </div>
        </header>
        {records.length === 0 ? (
          <p className="muted" style={{ margin: '0 20px 16px' }}>Deploy a workload to publish DNS records.</p>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Value</th>
                </tr>
              </thead>
              <tbody>
                {records.map((record) => (
                  <tr key={`${record.name}-${record.type}`}>
                    <td className="mono">{record.name}</td>
                    <td className="mono">{record.type}</td>
                    <td className="mono">{record.value}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
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
          <div className="table-scroll" style={{ marginTop: '1rem' }}>
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Workload</th>
                  <th>Mode</th>
                  <th>Allowlist</th>
                </tr>
              </thead>
              <tbody>
                {policies.map((policy) => (
                  <tr key={policy.id || policy.workloadId}>
                    <td className="mono">{policy.workloadId}</td>
                    <td><span className="status-chip status-running">{policy.mode}</span></td>
                    <td className="mono">{policy.allowlist.length > 0 ? policy.allowlist.join(', ') : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>
    </section>
  )
}

function NodeTableRow({ node }: { node: NodeNetworkStatus }) {
  const netStatus = node.networkStatus || 'pending'
  const statusClass =
    netStatus === 'ready' || netStatus === 'online' ? 'status-running' : netStatus === 'degraded' ? 'status-pending' : 'status-stopped'
  return (
    <tr>
      <td><strong>{node.name}</strong></td>
      <td><span className={`status-chip ${statusClass}`}>{netStatus}</span></td>
      <td className="mono">{node.wgAddress || '—'}</td>
      <td className="error-inline">{node.lastError || '—'}</td>
    </tr>
  )
}
