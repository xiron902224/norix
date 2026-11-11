import { useEffect, useState, type FormEvent } from 'react'
import {
  Check,
  Copy,
  Cpu,
  HardDrive,
  LoaderCircle,
  Radio,
  Server,
  Terminal,
} from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { formatWhen, relativeAge, shortId } from '../../../shared/lib/format'
import type { Node } from '../../../gen/v1/agent_pb'
import { createJoinToken, listNodes } from '../services/nodes-api'

function statusClass(status: string) {
  if (status === 'online') return 'status-running'
  if (status === 'degraded') return 'status-pending'
  return 'status-stopped'
}

export function NodesPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [selected, setSelected] = useState<Node | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [joinToken, setJoinToken] = useState<{ token: string; expiresAt: string } | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setError('')
      setLoading(true)
      try {
        const items = await listNodes()
        if (!cancelled) {
          setNodes(items)
          setSelected((current) => current ? items.find((item) => item.id === current.id) ?? null : null)
        }
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
          setSelected((current) => current ? items.find((item) => item.id === current.id) ?? current : null)
        })
        .catch(() => { /* keep last good snapshot while polling */ })
    }, 3000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  async function onCreateToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setCopied(false)
    const formEl = event.currentTarget
    const form = new FormData(formEl)
    try {
      const created = await createJoinToken({
        nodeName: String(form.get('name') ?? ''),
      })
      setJoinToken(created)
      formEl.reset()
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

  const online = nodes.filter((node) => node.status === 'online').length
  const installCommand = joinToken
    ? `cd agent && NORIX_JOIN_TOKEN='${joinToken.token}' NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080 ./target/release/norix-agent`
    : `cd agent && NORIX_JOIN_TOKEN='<token>' NORIX_CONTROL_PLANE_URL=http://127.0.0.1:8080 ./target/release/norix-agent`

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Nodes</h2>
          <p>Agents must be <strong>online</strong> (heartbeat &lt; ~45s) before Deploy can reconcile containers.</p>
        </div>
      </header>

      {online === 0 ? (
        <div className="callout callout-warn" role="status">
          <TriangleWarn />
          <div>
            <strong>Fleet offline</strong>
            <p>
              {nodes.length === 0
                ? 'Waiting for the control-plane host node, or create a join token to enroll a remote agent.'
                : 'Enrolled remote agent(s) are offline. The local control-plane node should stay online for deploys on this VPS.'}
            </p>
          </div>
        </div>
      ) : null}

      <div className="stats-grid">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Server /> Enrolled</div>
          <strong>{loading ? '—' : nodes.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Radio /> Online</div>
          <strong>{loading ? '—' : online}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Terminal /> Runtime</div>
          <strong style={{ fontSize: '1.05rem' }}>docker / nerdctl</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Create join token</h3>
            <p className="muted">One-time token (1h). After enroll, credentials live in <code>agent/.agent/node-state.json</code>.</p>
          </div>
        </header>

        {error && <div className="error" role="alert">{error}</div>}

        <form className="inline-form" onSubmit={(event) => void onCreateToken(event)}>
          <FormField id="node-name" name="name" label="Suggested node name" placeholder="edge-1" />
          <Button type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create join token'}</Button>
        </form>

        {joinToken ? (
          <div className="token-card">
            <div className="token-meta">
              <strong>Join token</strong>
              <span className="muted">Expires {formatWhen(joinToken.expiresAt)}</span>
            </div>
            <code className="token-value">{joinToken.token}</code>
            <div className="page-actions">
              <Button type="button" variant="secondary" onClick={() => void copyToken()}>
                {copied ? <><Check size={14} /> Copied</> : <><Copy size={14} /> Copy token</>}
              </Button>
            </div>
            <p className="muted" style={{ marginTop: 12 }}>Start agent (needs Docker):</p>
            <code className="install-command">{installCommand}</code>
          </div>
        ) : null}
      </section>

      <div className="split-detail">
        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Fleet</h3>
              <p className="muted">Offline after ~45s without heartbeat. Click a node for capabilities.</p>
            </div>
          </header>

          {loading ? (
            <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading nodes…</span></div>
          ) : null}

          {!loading && nodes.length === 0 ? (
            <div className="empty-state">
              <Server />
              <strong>No agents enrolled</strong>
              <p className="muted">Create a join token and run the agent on the target host.</p>
            </div>
          ) : null}

          <ul className="resource-list">
            {nodes.map((node) => (
              <li key={node.id} className={selected?.id === node.id ? 'is-selected' : ''}>
                <button type="button" className="resource-link" onClick={() => setSelected(node)}>
                  <strong>{node.name}{node.isLocal ? ' · local' : ''}</strong>
                  <div className="resource-meta">
                    <span className={`status-badge ${statusClass(node.status)}`}>{node.status}</span>
                    <span className="mono">seen {relativeAge(node.lastSeenAt)}</span>
                  </div>
                  <p className="hint-line">
                    {node.capabilities?.hostname || 'unknown host'} · {node.isLocal ? 'control-plane' : `agent v${node.capabilities?.agentVersion || '?'}`}
                  </p>
                </button>
                {node.isLocal
                  ? <span className="chip chip-info">VPS host</span>
                  : node.status === 'online'
                    ? <span className="chip chip-info"><LoaderCircle size={12} className="spin-slow" /> Live</span>
                    : <span className="chip">Offline</span>}
              </li>
            ))}
          </ul>
        </section>

        <aside className="panel flux-shadow lift-card detail-pane">
          <header className="panel-header">
            <div>
              <h3>Node detail</h3>
              <p className="muted">{selected ? selected.name : 'Select a node'}</p>
            </div>
          </header>
          {!selected ? (
            <div className="empty-state compact">
              <Server />
              <strong>Nothing selected</strong>
              <p className="muted">Pick a fleet member to inspect hardware and runtime sockets.</p>
            </div>
          ) : (
            <dl className="detail-grid">
              <div><dt>ID</dt><dd className="mono">{selected.id}</dd></div>
              <div><dt>Status</dt><dd><span className={`status-badge ${statusClass(selected.status)}`}>{selected.status}</span></dd></div>
              <div><dt>Last seen</dt><dd>{formatWhen(selected.lastSeenAt)} ({relativeAge(selected.lastSeenAt)})</dd></div>
              <div><dt>Kind</dt><dd>{selected.isLocal ? 'Control-plane host (embedded)' : 'Remote agent'}</dd></div>
              <div><dt>Hostname</dt><dd>{selected.capabilities?.hostname || '—'}</dd></div>
              <div><dt>Agent</dt><dd>{selected.capabilities?.agentVersion || '—'}</dd></div>
              <div><dt><Cpu size={12} /> CPU</dt><dd>{selected.capabilities?.cpuCores ?? 0} cores</dd></div>
              <div><dt><HardDrive size={12} /> Memory</dt><dd>{selected.capabilities?.memoryTotalMb ?? 0} MB</dd></div>
              <div><dt>Docker socket</dt><dd>{selected.capabilities?.dockerSocket ? 'yes' : 'no'}</dd></div>
              <div><dt>Containerd socket</dt><dd>{selected.capabilities?.containerdSocket ? 'yes' : 'no'}</dd></div>
              <div><dt>KVM level</dt><dd>{selected.capabilities?.kvmLevel ?? 0} (0=none, 1=/dev/kvm, 2=nested)</dd></div>
              <div className="detail-span">
                <dt>Live metrics</dt>
                <dd>
                  <div className="metric-bars">
                    <div className="metric-bar">
                      <label><span>CPU usage</span><span>{Number(selected.cpuUsagePercent || 0).toFixed(1)}%</span></label>
                      <div className="track"><div className="fill" style={{ width: `${Math.min(100, Math.max(0, Number(selected.cpuUsagePercent || 0)))}%` }} /></div>
                    </div>
                    <div className="metric-bar">
                      <label>
                        <span>Memory free</span>
                        <span>
                          {String(selected.memoryFreeMb || 0)} / {String(selected.capabilities?.memoryTotalMb || 0)} MB
                        </span>
                      </label>
                      <div className="track">
                        <div
                          className="fill"
                          style={{
                            width: `${selected.capabilities?.memoryTotalMb
                              ? Math.min(100, Math.max(0, (1 - Number(selected.memoryFreeMb || 0) / Number(selected.capabilities.memoryTotalMb)) * 100))
                              : 0}%`,
                          }}
                        />
                      </div>
                    </div>
                    <div className="metric-bar">
                      <label><span>Disk free</span><span>{String(selected.diskFreeMb || 0)} MB</span></label>
                      <div className="track"><div className="fill" style={{ width: selected.diskFreeMb ? '35%' : '0%' }} /></div>
                    </div>
                  </div>
                </dd>
              </div>
              <div className="detail-span">
                <dt>Org</dt>
                <dd className="mono">{shortId(selected.organizationId, 18)}</dd>
              </div>
            </dl>
          )}
        </aside>
      </div>
    </section>
  )
}

function TriangleWarn() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
      <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
  )
}
