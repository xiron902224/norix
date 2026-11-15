import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
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
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)

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
      if (selectedId && !egress.some((policy) => (policy.id || policy.workloadId) === selectedId)) {
        setSelectedId('')
      }
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
    const targetWorkload = String(form.get('workloadId') ?? workloadId)
    try {
      await upsertEgressPolicy({
        workloadId: targetWorkload,
        mode,
        allowlist,
      })
      setNotice(`Egress policy: ${mode}`)
      setModalOpen(false)
      await refresh()
      setSelectedId(targetWorkload)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update egress policy')
    } finally {
      setBusy(false)
    }
  }

  const nodes = status?.nodes ?? []
  const records = status?.records ?? []
  const pending = nodes.filter((node) => node.networkStatus === 'pending' || node.networkStatus === 'degraded').length
  const selected = policies.find((policy) => (policy.id || policy.workloadId) === selectedId) ?? null
  const selectedNode = selected
    ? null
    : nodes.find((node) => node.nodeId === selectedId) ?? null

  const policyRows: ResourceRow[] = useMemo(() => policies.map((policy) => ({
    id: policy.id || policy.workloadId,
    name: policy.workloadId,
    subtitle: policy.id || undefined,
    status: policy.mode || 'allow',
    values: [
      policy.allowlist.length > 0 ? policy.allowlist.join(', ') : '—',
    ],
  })), [policies])

  const nodeRows: ResourceRow[] = useMemo(() => nodes.map((node) => ({
    id: node.nodeId,
    name: node.name,
    subtitle: node.wgAddress || undefined,
    status: node.networkStatus || 'pending',
    values: [
      node.wgAddress || '—',
      node.lastError || '—',
    ],
  })), [nodes])

  return (
    <section className="compass-page">
      <PageHero
        title="Network"
        subtitle="WireGuard overlay, internal DNS (*.norix.internal), isolation, and egress"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> Egress policy
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      {pending > 0 ? (
        <div className="callout callout-warn" role="status">
          <div>
            <strong>{pending} node{pending === 1 ? '' : 's'} with pending network</strong>
            <p>Localruntime writes mesh configs on each reconcile. Set NORIX_APPLY_WIREGUARD=1 to bring up wg-norix.</p>
          </div>
        </div>
      ) : null}

      <MetricStrip
        items={[
          { label: 'Nodes ready', value: loading ? '—' : `${status?.nodesReady ?? 0}/${status?.nodesTotal ?? 0}` },
          { label: 'DNS', value: loading ? '—' : status?.dnsRecords ?? 0 },
          { label: 'Certificates', value: loading ? '—' : status?.certificates ?? 0 },
          { label: 'Overlay', value: status?.overlayCidr || '10.88.0.0/16' },
          { label: 'Apply', value: status?.applyEnabled ? 'active' : 'dry-run' },
          { label: 'Policies', value: loading ? '—' : policies.length },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${policies.length} polic${policies.length === 1 ? 'y' : 'ies'}`}</strong>
          <span className="muted">{records.length} DNS record{records.length === 1 ? '' : 's'} · poll 5s</span>
        </div>
        <div className="compass-split">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16, minWidth: 0 }}>
            <ResourceTable
              leadingHeader="Workload"
              headers={['Allowlist']}
              rows={policyRows}
              selectedId={selected ? selectedId : ''}
              onSelect={(row) => setSelectedId(row.id)}
              empty="No egress policies. Create one for allow / deny / allowlist."
            />
            {nodes.length > 0 ? (
              <ResourceTable
                leadingHeader="Mesh node"
                headers={['WG', 'Error']}
                rows={nodeRows}
                selectedId={!selected && selectedNode ? selectedId : ''}
                onSelect={(row) => setSelectedId(row.id)}
                empty="No network nodes."
              />
            ) : null}
          </div>
          <aside className="compass-side-panel">
            {selected ? (
              <>
                <h3>{selected.workloadId}</h3>
                <p className="muted">Egress policy</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>Mode</dt><dd>{selected.mode || 'allow'}</dd></div>
                  <div><dt>ID</dt><dd className="mono">{selected.id || '—'}</dd></div>
                  <div><dt>Allowlist</dt><dd className="mono">{selected.allowlist.length > 0 ? selected.allowlist.join(', ') : '—'}</dd></div>
                </dl>
                <p className="muted" style={{ marginTop: 12 }}>Applied in nftables when apply is active.</p>
              </>
            ) : selectedNode ? (
              <NodeDetail node={selectedNode} />
            ) : (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a policy or a mesh node.</p>
                {records.length > 0 ? (
                  <>
                    <h4 style={{ marginTop: 16 }}>Internal DNS</h4>
                    <ul className="resource-list" style={{ marginTop: 8 }}>
                      {records.slice(0, 8).map((record) => (
                        <li key={`${record.name}-${record.type}`}>
                          <div className="resource-link">
                            <strong className="mono">{record.name}</strong>
                            <p className="hint-line mono">{record.type} → {record.value}</p>
                          </div>
                        </li>
                      ))}
                    </ul>
                    {records.length > 8 ? <p className="muted">+{records.length - 8} more</p> : null}
                  </>
                ) : (
                  <p className="muted" style={{ marginTop: 12 }}>Deploy a workload to publish DNS (name.project.norix.internal).</p>
                )}
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="Egress policy"
        submitLabel="Save policy"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onEgress(event)}
      >
        <SelectField
          id="net-project"
          label="Project"
          value={projectId}
          onChange={(event) => setProjectId(event.target.value)}
          required
        >
          <option value="" disabled>Select</option>
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
          required
        >
          <option value="" disabled>Select</option>
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
      </CreateModal>
    </section>
  )
}

function NodeDetail({ node }: { node: NodeNetworkStatus }) {
  return (
    <>
      <h3>{node.name}</h3>
      <p className="muted">WireGuard mesh node</p>
      <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
        <div><dt>Status</dt><dd>{node.networkStatus || 'pending'}</dd></div>
        <div><dt>WG</dt><dd className="mono">{node.wgAddress || '—'}</dd></div>
        <div><dt>Error</dt><dd>{node.lastError || '—'}</dd></div>
      </dl>
    </>
  )
}
