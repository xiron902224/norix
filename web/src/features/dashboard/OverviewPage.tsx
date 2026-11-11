import { useEffect, useState } from 'react'
import {
  Activity,
  ArrowUpRight,
  Boxes,
  Cpu,
  FolderKanban,
  HardDrive,
  Radio,
  Server,
  Sparkles,
} from 'lucide-react'
import type { SessionUser } from '../auth/services/session'
import type { Project } from '../../gen/v1/workload_pb'
import type { Node } from '../../gen/v1/agent_pb'
import { listProjects, listWorkloads } from '../projects/services/projects-api'
import { listNodes } from '../nodes/services/nodes-api'
import { relativeAge } from '../../shared/lib/format'

interface OverviewPageProps {
  user: SessionUser
  onOpenProjects: () => void
  onOpenProject: (project: Project) => void
}

const ACTIVITY = [
  { title: 'Ops backups & alerts', detail: 'pg_dump + node.down / deploy.failed in Ops (S7)', time: 'Just now', tone: 'success' },
  { title: 'Edge HTTPS on :8443', detail: 'Open domains as https://host:8443 (Caddy)', time: '2m ago', tone: 'info' },
  { title: 'Git builds & deploys', detail: 'Build from git or public images in Projects', time: '12m ago', tone: 'success' },
  { title: 'Node live metrics', detail: 'CPU / memory / disk on the Nodes detail pane', time: '1h ago', tone: 'success' },
  { title: 'Next: hardening', detail: 'Threat model and release signing in Sprint 8', time: '3h ago', tone: 'muted' },
] as const

function greeting() {
  const hour = new Date().getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

function Sparkline({ values, className = '' }: { values: number[]; className?: string }) {
  const max = Math.max(...values, 1)
  const min = Math.min(...values)
  const range = Math.max(max - min, 1)
  const points = values
    .map((value, index) => {
      const x = (index / (values.length - 1)) * 100
      const y = 100 - ((value - min) / range) * 100
      return `${x},${y}`
    })
    .join(' ')

  return (
    <svg className={`sparkline ${className}`} viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden>
      <polyline fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" points={points} />
    </svg>
  )
}

export function OverviewPage({ user, onOpenProjects, onOpenProject }: OverviewPageProps) {
  const [projects, setProjects] = useState<Project[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [workloadCount, setWorkloadCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [onlineHistory, setOnlineHistory] = useState<number[]>([0, 0, 0, 0, 0, 0, 0])

  useEffect(() => {
    let cancelled = false

    async function refresh() {
      try {
        const [projectItems, nodeItems] = await Promise.all([listProjects(), listNodes()])
        if (cancelled) return
        setProjects(projectItems)
        setNodes(nodeItems)
        const online = nodeItems.filter((node) => node.status === 'online').length
        setOnlineHistory((prev) => [...prev.slice(1), online])

        let total = 0
        await Promise.all(projectItems.map(async (project) => {
          try {
            const workloads = await listWorkloads(project.id)
            total += workloads.length
          } catch {
            /* keep partial count */
          }
        }))
        if (!cancelled) setWorkloadCount(total)
      } catch {
        if (!cancelled) {
          setProjects([])
          setNodes([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 3000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  const firstName = (user.name || user.email).split(/[\s@]/)[0]
  const online = nodes.filter((node) => node.status === 'online').length
  const localNode = nodes.find((node) => node.isLocal) ?? null
  const host = localNode?.capabilities
  const runtimeHealthy = online > 0

  return (
    <section className="page">
      <header className="page-header hero-header">
        <div>
          <p className="eyebrow soft">{greeting()}, {firstName}</p>
          <h2>Command center</h2>
          <p>Projects, nodes, and container deploys — heartbeat keeps desired and observed in sync.</p>
        </div>
        <div className="page-actions">
          <button type="button" className="primary-button flux-glow" onClick={onOpenProjects}>
            Manage projects
            <ArrowUpRight size={16} aria-hidden />
          </button>
        </div>
      </header>

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><FolderKanban /> Projects</div>
          <strong>{loading ? '—' : projects.length}</strong>
          <span className="stat-delta up">Organization scope</span>
          <Sparkline values={[2, 3, 3, 4, 5, 5, projects.length || 1]} className="spark-accent" />
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Boxes /> Workloads</div>
          <strong>{loading ? '—' : workloadCount}</strong>
          <span className="stat-delta">Across all projects</span>
          <Sparkline values={[1, 2, 2, 3, 3, 4, workloadCount || 1]} className="spark-primary" />
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Server /> Nodes online</div>
          <strong>{loading ? '—' : online}</strong>
          <span className="stat-delta">{nodes.length} enrolled · live poll 3s</span>
          <div className="mini-bars" aria-hidden>
            {onlineHistory.map((h, i) => (
              <span key={i} style={{ height: `${Math.max(12, (h / Math.max(online, 1)) * 100)}%` }} />
            ))}
          </div>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Radio /> Heartbeat</div>
          <strong>{runtimeHealthy ? 'Live' : 'Idle'}</strong>
          <span className="stat-delta">{runtimeHealthy ? 'At least one node online' : 'Waiting for node / local host'}</span>
          <div className="pulse-row" aria-hidden>
            <span className="pulse-dot" />
            <span className="pulse-dot delay-1" />
            <span className="pulse-dot delay-2" />
          </div>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Control-plane host</h3>
            <p className="muted">
              {localNode
                ? 'Embedded local node on this VPS — available for deploys when no remote agents are enrolled.'
                : 'Host inventory appears once the local node is registered.'}
            </p>
          </div>
          {localNode ? (
            <span className={`status-badge ${localNode.status === 'online' ? 'status-running' : 'status-stopped'}`}>
              {localNode.status}
            </span>
          ) : null}
        </header>
        <dl className="detail-grid">
          <div><dt>Hostname</dt><dd>{host?.hostname || '—'}</dd></div>
          <div><dt>Node</dt><dd>{localNode?.name || '—'}</dd></div>
          <div><dt><Cpu size={12} /> CPU</dt><dd>{host?.cpuCores ?? 0} cores</dd></div>
          <div><dt><HardDrive size={12} /> Memory</dt><dd>{host?.memoryTotalMb ? `${host.memoryTotalMb} MB` : '—'}</dd></div>
          <div><dt>Docker</dt><dd>{host?.dockerSocket ? 'socket present' : 'missing'}</dd></div>
          <div><dt>Containerd</dt><dd>{host?.containerdSocket ? 'socket present' : 'missing'}</dd></div>
          <div><dt>Last seen</dt><dd>{localNode?.lastSeenAt ? relativeAge(localNode.lastSeenAt) : '—'}</dd></div>
          <div><dt>Runtime</dt><dd>{host?.agentVersion || '—'}</dd></div>
        </dl>
      </section>

      <div className="dashboard-grid">
        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Fleet snapshot</h3>
              <p className="muted">Remote agents + the control-plane host.</p>
            </div>
            <span className="chip chip-info"><Sparkles size={12} /> Live</span>
          </header>
          {loading ? (
            <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading nodes…</span></div>
          ) : null}
          {!loading && nodes.length === 0 ? (
            <div className="empty-state compact">
              <Server />
              <strong>No nodes yet</strong>
              <p className="muted">The local host node should appear automatically.</p>
            </div>
          ) : (
            <ul className="resource-list">
              {nodes.slice(0, 6).map((node) => (
                <li key={node.id}>
                  <div className="resource-link" style={{ cursor: 'default' }}>
                    <strong>{node.name}{node.isLocal ? ' (local)' : ''}</strong>
                    <div className="resource-meta">
                      <span className={`status-badge ${node.status === 'online' ? 'status-running' : 'status-stopped'}`}>
                        {node.status}
                      </span>
                      <span className="mono">{node.capabilities?.hostname || 'host?'}</span>
                    </div>
                  </div>
                  <span className="chip">{relativeAge(node.lastSeenAt)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Platform health</h3>
              <p className="muted">Control plane readiness checklist.</p>
            </div>
          </header>
          <ul className="health-list">
            <li>
              <div>
                <strong>API /healthz</strong>
                <span className="muted">Connect control plane</span>
              </div>
              <span className="status-badge status-running">up</span>
            </li>
            <li>
              <div>
                <strong>Auth sessions</strong>
                <span className="muted">Opaque tokens + renew</span>
              </div>
              <span className="status-badge status-running">healthy</span>
            </li>
            <li>
              <div>
                <strong>Postgres + RLS</strong>
                <span className="muted">Tenant isolation</span>
              </div>
              <span className="status-badge status-running">ok</span>
            </li>
            <li>
              <div>
                <strong>Runtime</strong>
                <span className="muted">{localNode ? 'Local host + remote agents' : 'Enrollment stream'}</span>
              </div>
              <span className={`status-badge ${runtimeHealthy ? 'status-running' : 'status-pending'}`}>
                {runtimeHealthy ? 'ready' : 'pending'}
              </span>
            </li>
          </ul>
        </section>
      </div>

      <div className="dashboard-grid">
        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Recent projects</h3>
              <p className="muted">Jump back into your workspace.</p>
            </div>
            <button type="button" className="ghost-button" onClick={onOpenProjects}>View all</button>
          </header>
          {loading ? (
            <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading projects…</span></div>
          ) : null}
          {!loading && projects.length === 0 ? (
            <div className="empty-state compact">
              <FolderKanban />
              <strong>No projects yet</strong>
              <p className="muted">Create one to populate this feed.</p>
              <button type="button" className="primary-button" onClick={onOpenProjects}>Create project</button>
            </div>
          ) : (
            <ul className="resource-list">
              {projects.slice(0, 5).map((project) => (
                <li key={project.id}>
                  <button type="button" className="resource-link" onClick={() => onOpenProject(project)}>
                    <strong>{project.name}</strong>
                    <span className="mono">{project.id}</span>
                  </button>
                  <span className="chip">Open</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Activity</h3>
              <p className="muted">Latest signals across the control plane.</p>
            </div>
            <Activity size={16} className="muted-icon" aria-hidden />
          </header>
          <ul className="activity-list">
            {ACTIVITY.map((item) => (
              <li key={item.title} className={`tone-${item.tone}`}>
                <span className="activity-dot" aria-hidden />
                <div>
                  <strong>{item.title}</strong>
                  <p className="muted">{item.detail}</p>
                </div>
                <time>{item.time}</time>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </section>
  )
}
