import { useEffect, useMemo, useState, type FormEvent } from 'react'
import {
  Check,
  Copy,
  Plus,
  TriangleAlert,
} from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import { StatusPill } from '../../../shared/ui/StatusPill'
import { formatWhen, relativeAge, shortId } from '../../../shared/lib/format'
import type { Node } from '../../../gen/v1/agent_pb'
import { cordonNode, createJoinToken, drainNode, listNodes, updateNodeSchedule } from '../services/nodes-api'

export function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [joinToken, setJoinToken] = useState<{ token: string; expiresAt: string } | null>(null)
  const [copied, setCopied] = useState(false)
  const [actionBusy, setActionBusy] = useState('')
  const [scheduleRole, setScheduleRole] = useState('worker')
  const [scheduleZone, setScheduleZone] = useState('default')

  const selected = nodes.find((node) => node.id === selectedId) ?? null

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setError('')
      setLoading(true)
      try {
        const items = await listNodes()
        if (!cancelled) setNodes(items)
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Could not load nodes')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    const timer = window.setInterval(() => {
      void listNodes()
        .then((items) => {
          if (cancelled) return
          setNodes(items)
        })
        .catch(() => { /* keep */ })
    }, 3000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  function selectNode(id: string) {
    setSelectedId(id)
    const node = nodes.find((item) => item.id === id)
    if (node) {
      setScheduleRole(node.role || 'worker')
      setScheduleZone(node.zone || 'default')
    }
  }

  async function onCreateToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setCopied(false)
    const form = new FormData(event.currentTarget)
    try {
      const created = await createJoinToken({
        nodeName: String(form.get('name') ?? ''),
      })
      setJoinToken(created)
      setModalOpen(false)
      const items = await listNodes()
      setNodes(items)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create join token')
    } finally {
      setCreating(false)
    }
  }

  async function copyToken() {
    if (!joinToken) return
    await navigator.clipboard.writeText(joinToken.token)
    setCopied(true)
    window.setTimeout(() => setCopied(false), 2000)
  }

  async function refreshNodes(preferId?: string) {
    const items = await listNodes()
    setNodes(items)
    const id = preferId ?? selectedId
    if (id && !items.some((item) => item.id === id)) setSelectedId('')
    const next = id ? items.find((item) => item.id === id) : null
    if (next) {
      setScheduleRole(next.role || 'worker')
      setScheduleZone(next.zone || 'default')
    }
  }

  async function onCordon(cordoned: boolean) {
    if (!selected) return
    setActionBusy(cordoned ? 'cordon' : 'uncordon')
    setError('')
    try {
      await cordonNode(selected.id, cordoned)
      await refreshNodes(selected.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update cordon')
    } finally {
      setActionBusy('')
    }
  }

  async function onDrain(draining: boolean) {
    if (!selected) return
    setActionBusy(draining ? 'drain' : 'undrain')
    setError('')
    try {
      await drainNode(selected.id, draining)
      await refreshNodes(selected.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update drain')
    } finally {
      setActionBusy('')
    }
  }

  async function onSaveSchedule(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    setActionBusy('schedule')
    setError('')
    try {
      await updateNodeSchedule({
        nodeId: selected.id,
        role: scheduleRole,
        zone: scheduleZone,
        labels: selected.labels ?? {},
      })
      await refreshNodes(selected.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save schedule')
    } finally {
      setActionBusy('')
    }
  }

  const online = nodes.filter((node) => node.status === 'online').length
  const installCommand = joinToken
    ? `cd agent && NORIX_JOIN_TOKEN='${joinToken.token}' NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080 ./target/release/norix-agent`
    : ''

  const rows: ResourceRow[] = useMemo(() => nodes.map((node) => ({
    id: node.id,
    name: node.name,
    subtitle: node.isLocal ? 'local · VPS' : (node.capabilities?.hostname || shortId(node.id, 12)),
    status: node.status || 'offline',
    values: [
      `${node.role || 'worker'} · ${node.zone || 'default'}`,
      relativeAge(node.lastSeenAt),
      `${node.reportedWorkloadIds?.length ?? 0} reported`,
      node.cordoned ? 'cordoned' : node.draining ? 'drain' : 'ok',
    ],
  })), [nodes])

  return (
    <section className="compass-page">
      <PageHero
        title="Nodes"
        subtitle="Online agents (heartbeat &lt; ~45s) before Deploy"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> Join token
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}

      {online === 0 ? (
        <div className="callout callout-warn" role="status">
          <TriangleAlert size={18} />
          <div>
            <strong>Fleet offline</strong>
            <p>
              {nodes.length === 0
                ? 'Create a join token or wait for the control-plane local node.'
                : 'Enrolled agents offline. Start norix-agent on the host.'}
            </p>
          </div>
        </div>
      ) : null}

      {joinToken ? (
        <div className="compass-notice" role="status">
          <div>
            <strong>Join token</strong>
            <span className="muted"> · expires {formatWhen(joinToken.expiresAt)}</span>
            <code className="token-value" style={{ display: 'block', marginTop: 8 }}>{joinToken.token}</code>
            <code className="install-command" style={{ display: 'block', marginTop: 8 }}>{installCommand}</code>
          </div>
          <Button type="button" variant="secondary" onClick={() => void copyToken()}>
            {copied ? <><Check size={14} /> Copied</> : <><Copy size={14} /> Copy</>}
          </Button>
        </div>
      ) : null}

      <MetricStrip
        items={[
          { label: 'Enrolled', value: loading ? '—' : nodes.length },
          { label: 'Online', value: loading ? '—' : online },
          { label: 'Offline', value: loading ? '—' : nodes.length - online },
          { label: 'Runtime', value: 'docker' },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${nodes.length} result${nodes.length === 1 ? '' : 's'}`}</strong>
          <span className="muted">Poll every 3s</span>
        </div>
        <div className="compass-split">
          <ResourceTable
            headers={['Role / zone', 'Seen', 'Schedule']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => selectNode(row.id)}
            empty="No agents. Create a join token and start norix-agent."
            leadingHeader="Node"
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a node to view hardware and schedule.</p>
              </>
            ) : (
              <>
                <h3>{selected.name}</h3>
                <p className="muted">{selected.isLocal ? 'Control-plane host' : 'Remote agent'}</p>
                <div style={{ marginTop: 10 }}>
                  <StatusPill status={selected.status || 'offline'} />
                </div>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>ID</dt><dd className="mono">{shortId(selected.id, 16)}</dd></div>
                  <div><dt>Role / zone</dt><dd>{selected.role || 'worker'} · {selected.zone || 'default'}</dd></div>
                  <div><dt>Schedule</dt><dd>{selected.cordoned ? 'cordoned' : 'schedulable'}{selected.draining ? ' · draining' : ''}</dd></div>
                  <div><dt>Seen</dt><dd>{relativeAge(selected.lastSeenAt)}</dd></div>
                  <div><dt>Hostname</dt><dd>{selected.capabilities?.hostname || '—'}</dd></div>
                  <div><dt>CPU / RAM</dt><dd>{selected.capabilities?.cpuCores ?? 0}c · {String(selected.capabilities?.memoryTotalMb ?? 0)} MB</dd></div>
                  <div><dt>CPU uso</dt><dd>{Number(selected.cpuUsagePercent || 0).toFixed(1)}%</dd></div>
                  <div><dt>RAM libre</dt><dd>{String(selected.memoryFreeMb || 0)} MB</dd></div>
                  <div><dt>Docker</dt><dd>{selected.capabilities?.dockerSocket ? 'yes' : 'no'}</dd></div>
                  <div><dt>KVM</dt><dd>{selected.capabilities?.kvmLevel ?? 0}</dd></div>
                </dl>
                <div className="page-actions" style={{ marginTop: 16, flexWrap: 'wrap' }}>
                  <Button type="button" variant="secondary" disabled={!!actionBusy || selected.cordoned} onClick={() => void onCordon(true)}>
                    {actionBusy === 'cordon' ? '…' : 'Cordon'}
                  </Button>
                  <Button type="button" variant="secondary" disabled={!!actionBusy || !selected.cordoned} onClick={() => void onCordon(false)}>
                    Uncordon
                  </Button>
                  <Button type="button" variant="secondary" disabled={!!actionBusy || selected.draining} onClick={() => void onDrain(true)}>
                    Drain
                  </Button>
                  <Button type="button" variant="secondary" disabled={!!actionBusy || !selected.draining} onClick={() => void onDrain(false)}>
                    Stop drain
                  </Button>
                </div>
                <form className="compass-create-form" style={{ marginTop: 16 }} onSubmit={(event) => void onSaveSchedule(event)}>
                  <label>
                    <span>Role</span>
                    <select value={scheduleRole} onChange={(event) => setScheduleRole(event.target.value)}>
                      <option value="worker">worker</option>
                      <option value="edge">edge</option>
                    </select>
                  </label>
                  <FormField id="node-zone" label="Zone" value={scheduleZone} onChange={(event) => setScheduleZone(event.target.value)} />
                  <Button type="submit" disabled={!!actionBusy}>
                    {actionBusy === 'schedule' ? 'Saving…' : 'Save schedule'}
                  </Button>
                </form>
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="Create join token"
        submitLabel="Create token"
        busy={creating}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreateToken(event)}
      >
        <FormField id="node-name" name="name" label="Suggested name" placeholder="edge-1" />
        <p className="muted">Single-use token (~1h). After enroll, credentials in agent/.agent/node-state.json.</p>
      </CreateModal>
    </section>
  )
}
