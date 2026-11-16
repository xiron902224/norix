import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  Activity,
  Gauge,
  GitBranch,
  MemoryStick,
  Search,
  Server,
  Terminal,
} from 'lucide-react'
import {
  Area,
  AreaChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { SessionUser } from '../auth/services/session'
import type { Project, Workload } from '../../gen/v1/workload_pb'
import type { Node } from '../../gen/v1/agent_pb'
import type { MetricSample, ObservabilityAlertEvent } from '../../gen/v1/metrics_pb'
import { listProjects, listWorkloads } from '../projects/services/projects-api'
import { listNodes } from '../nodes/services/nodes-api'
import { getClusterMetrics, listAlerts } from '../observability/services/observability-api'
import { relativeAge, shortId } from '../../shared/lib/format'
import { statusLabel } from '../../shared/ui/StatusPill'

interface OverviewPageProps {
  user: SessionUser
}

interface AppRow {
  id: string
  name: string
  projectId: string
  projectName: string
  status: string
  zone: string
  replicas: number
  runtime: string
  image: string
  error: string
  cpu: number | null
  memory: number | null
  commit: string
  ago: string
}

interface ChartPoint {
  h: string
  cpu: number
  memFreeGb: number
}

const STATUS_META: Record<string, { label: string; tone: string }> = {
  running: { label: 'Active', tone: 'ok' },
  building: { label: 'Building', tone: 'info' },
  deploying: { label: 'Deploying', tone: 'info' },
  failed: { label: 'Failed', tone: 'danger' },
  stopped: { label: 'Idle', tone: 'muted' },
  pending: { label: 'Pending', tone: 'warn' },
}

function mbToGb(mb: number) {
  return mb / 1024
}

function formatGb(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0'
  return value >= 10 ? value.toFixed(0) : value.toFixed(1)
}

function nodeMem(node: Node) {
  const totalMb = Number(node.capabilities?.memoryTotalMb ?? 0)
  const freeMb = Number(node.memoryFreeMb ?? 0)
  return { totalMb, freeMb, usedMb: totalMb > 0 ? Math.max(0, totalMb - freeMb) : 0 }
}

function buildAgentSeries(samples: MetricSample[]): ChartPoint[] {
  const cpuByTs = new Map<number, number[]>()
  const memByTs = new Map<number, number[]>()

  for (const sample of samples) {
    const ts = Number(sample.timestamp || 0)
    if (!ts) continue
    const bucket = Math.floor(ts / 60) * 60
    if (sample.name === 'node_cpu_usage_percent') {
      const list = cpuByTs.get(bucket) ?? []
      list.push(sample.value)
      cpuByTs.set(bucket, list)
    }
    if (sample.name === 'node_memory_free_mb') {
      const list = memByTs.get(bucket) ?? []
      list.push(sample.value)
      memByTs.set(bucket, list)
    }
  }

  const buckets = [...new Set([...cpuByTs.keys(), ...memByTs.keys()])].sort((a, b) => a - b).slice(-24)
  return buckets.map((bucket) => {
    const cpuVals = cpuByTs.get(bucket) ?? []
    const memVals = memByTs.get(bucket) ?? []
    const cpu = cpuVals.length ? cpuVals.reduce((a, b) => a + b, 0) / cpuVals.length : 0
    const memFree = memVals.length ? memVals.reduce((a, b) => a + b, 0) / memVals.length : 0
    const date = new Date(bucket * (bucket > 1e12 ? 1 : 1000))
    return {
      h: date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      cpu: Math.round(cpu * 10) / 10,
      memFreeGb: Math.round(mbToGb(memFree) * 10) / 10,
    }
  })
}

export function OverviewPage({ user }: OverviewPageProps) {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[]>([])
  const [nodes, setNodes] = useState<Node[]>([])
  const [apps, setApps] = useState<AppRow[]>([])
  const [alerts, setAlerts] = useState<ObservabilityAlertEvent[]>([])
  const [chart, setChart] = useState<ChartPoint[]>([])
  const [query, setQuery] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false

    async function refresh() {
      try {
        const [projectItems, nodeItems, metrics, alertResp] = await Promise.all([
          listProjects(),
          listNodes(),
          getClusterMetrics().catch(() => null),
          listAlerts().catch(() => null),
        ])
        if (cancelled) return
        setProjects(projectItems)
        setNodes(nodeItems)
        if (alertResp?.events) setAlerts(alertResp.events.slice(0, 12))
        setChart(buildAgentSeries(metrics?.samples ?? []))

        const rows: AppRow[] = []
        await Promise.all(projectItems.map(async (project) => {
          try {
            const workloads = await listWorkloads(project.id)
            for (const workload of workloads) {
              if (workload.status === 'deleting') continue
              rows.push(toAppRow(workload, project, nodeItems))
            }
          } catch {
            /* keep partial */
          }
        }))
        if (!cancelled) {
          rows.sort((a, b) => a.name.localeCompare(b.name))
          setApps(rows)
        }
      } catch {
        if (!cancelled) {
          setProjects([])
          setNodes([])
          setApps([])
          setChart([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 5000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  const onlineNodes = nodes.filter((node) => node.status === 'online')
  const online = onlineNodes.length
  const avgCpu = onlineNodes.length
    ? onlineNodes.reduce((sum, node) => sum + (node.cpuUsagePercent || 0), 0) / onlineNodes.length
    : 0

  const memTotals = onlineNodes.reduce((acc, node) => {
    const { totalMb, freeMb, usedMb } = nodeMem(node)
    return {
      totalMb: acc.totalMb + totalMb,
      freeMb: acc.freeMb + freeMb,
      usedMb: acc.usedMb + usedMb,
    }
  }, { totalMb: 0, freeMb: 0, usedMb: 0 })

  const memLabel = (() => {
    if (loading) return '—'
    if (memTotals.totalMb > 0) {
      return `${formatGb(mbToGb(memTotals.usedMb))} / ${formatGb(mbToGb(memTotals.totalMb))} GB`
    }
    if (memTotals.freeMb > 0) {
      return `${formatGb(mbToGb(memTotals.freeMb))} GB free`
    }
    return '—'
  })()

  const memHint = memTotals.totalMb > 0
    ? 'used / total from agent'
    : memTotals.freeMb > 0
      ? 'MemAvailable from agent'
      : 'waiting for agent'

  const runningApps = apps.filter((app) => app.status === 'running').length
  const failedApps = apps.filter((app) => app.status === 'failed')
  const filteredApps = apps.filter((app) => app.name.toLowerCase().includes(query.toLowerCase()))

  const zones = useMemo(() => {
    const counts = new Map<string, { label: string; count: number }>()
    for (const node of nodes) {
      const key = node.zone || (node.isLocal ? 'local' : 'default')
      const current = counts.get(key) ?? { label: key, count: 0 }
      current.count += 1
      counts.set(key, current)
    }
    const total = Math.max(nodes.length, 1)
    return [...counts.entries()]
      .map(([name, meta]) => ({
        name,
        label: meta.label,
        pct: Math.round((meta.count / total) * 100),
      }))
      .sort((a, b) => b.pct - a.pct)
      .slice(0, 3)
  }, [nodes])

  const displayZones = zones.length ? zones : [{ name: 'local', label: 'local', pct: 100 }]
  const liveEvents = useMemo(() => buildEvents(apps, alerts, nodes), [apps, alerts, nodes])
  const incident = failedApps[0] ?? null
  const hasChart = chart.length > 0

  const needsOnboarding = !loading && (online === 0 || projects.length === 0 || apps.length === 0)

  return (
    <section className="compass-overview-exact">
      <header className="compass-ov-hero">
        <div>
          <div className="compass-ov-pulse">
            <span className="compass-ov-pulse-dot" aria-hidden />
            Multi-node cluster · {displayZones.length} region{displayZones.length === 1 ? '' : 's'}
          </div>
          <h1>Norix · Overview</h1>
          <p>Live metrics from node agents · Docker · KVM · WASM</p>
        </div>
        <div className="compass-ov-cli">
          <span><Terminal size={14} aria-hidden /> norix deploy</span>
          <span><GitBranch size={14} aria-hidden /> main</span>
        </div>
      </header>

      {needsOnboarding ? (
        <div className="onboarding-card" role="region" aria-label="First-run setup">
          <div>
            <h2>Get started</h2>
            <p className="muted">
              Finish these steps once after bootstrap. Docs:{' '}
              <a href="https://github.com/xiron902224/norix/blob/main/docs/user/getting-started.md" target="_blank" rel="noreferrer">
                Getting started
              </a>
              .
            </p>
          </div>
          <ol className="onboarding-steps">
            <li className={online > 0 ? 'is-done' : 'is-todo'}>
              <strong>1. Add a node</strong>
              <p className="muted">Create a join token and enroll an agent with Docker.</p>
              <Link to="/nodes">Regions & nodes →</Link>
              {' · '}
              <a href="https://github.com/xiron902224/norix/blob/main/docs/user/nodes-agents.md" target="_blank" rel="noreferrer">
                Nodes guide
              </a>
            </li>
            <li className={apps.length > 0 ? 'is-done' : 'is-todo'}>
              <strong>2. Deploy a sample app</strong>
              <p className="muted">Create a project, then deploy an image (e.g. <code>nginx:alpine</code>) or Git.</p>
              <Link to="/projects">Projects →</Link>
              {' · '}
              <a href="https://github.com/xiron902224/norix/blob/main/docs/user/deploying.md" target="_blank" rel="noreferrer">
                Deploying
              </a>
            </li>
            <li className={apps.some((app) => !!app.image) ? undefined : undefined}>
              <strong>3. Attach a domain</strong>
              <p className="muted">Publish a hostname once the app is running.</p>
              <Link to="/domains">Domains →</Link>
              {' · '}
              <a href="https://github.com/xiron902224/norix/blob/main/docs/user/production-tls.md" target="_blank" rel="noreferrer">
                Production TLS
              </a>
            </li>
          </ol>
        </div>
      ) : null}

      <label className="compass-ov-search">
        <Search size={16} aria-hidden />
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          aria-label="Search apps"
          placeholder="Search apps…"
        />
      </label>

      <div className="compass-ov-metrics">
        {[
          {
            icon: Server,
            label: 'Nodes online',
            value: loading ? '—' : `${online}/${nodes.length}`,
            hint: 'agent heartbeats',
          },
          {
            icon: Activity,
            label: 'Avg CPU',
            value: loading || !online ? '—' : `${Math.round(avgCpu)}%`,
            hint: 'cpu_usage_percent',
          },
          {
            icon: MemoryStick,
            label: 'Memory',
            value: memLabel,
            hint: memHint,
          },
          {
            icon: Gauge,
            label: 'Apps running',
            value: loading ? '—' : String(runningApps),
            hint: `${apps.length} total workloads`,
          },
        ].map((item) => (
          <article key={item.label} className="compass-ov-metric-card">
            <div className="compass-ov-metric-head">
              <span>{item.label}</span>
              <item.icon size={16} aria-hidden />
            </div>
            <div className="compass-ov-metric-value">{item.value}</div>
            <div className="compass-ov-metric-delta muted">{item.hint}</div>
          </article>
        ))}
      </div>

      <div className="compass-ov-mid">
        <article className="compass-ov-card compass-ov-traffic">
          <div className="compass-ov-card-head">
            <h2>Agent metrics</h2>
            <div className="compass-ov-legend">
              <span><i className="swatch swatch-req" /> CPU %</span>
              <span><i className="swatch swatch-err" /> Free RAM (GB)</span>
            </div>
          </div>
          <div className="compass-ov-chart">
            {hasChart ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chart} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="ovCpuFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.35} />
                      <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <XAxis dataKey="h" stroke="var(--muted-fg)" fontSize={11} tickLine={false} axisLine={false} interval="preserveStartEnd" />
                  <YAxis stroke="var(--muted-fg)" fontSize={11} tickLine={false} axisLine={false} width={40} />
                  <Tooltip
                    contentStyle={{
                      background: 'var(--card)',
                      border: '1px solid var(--border)',
                      borderRadius: 6,
                      fontSize: 12,
                    }}
                  />
                  <Area type="monotone" dataKey="cpu" name="CPU %" stroke="var(--chart-1)" strokeWidth={2} fill="url(#ovCpuFill)" />
                  <Area type="monotone" dataKey="memFreeGb" name="Free RAM GB" stroke="var(--chart-4)" strokeWidth={1.5} fill="transparent" />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <div className="compass-empty" style={{ border: 0, height: '100%', display: 'grid', placeItems: 'center' }}>
                {loading ? 'Loading agent samples…' : 'No metric samples yet. Keep an agent online — heartbeats write CPU/RAM samples.'}
              </div>
            )}
          </div>
        </article>

        <article className="compass-ov-card">
          <h2 className="compass-ov-card-title">Regions</h2>
          <ul className="compass-ov-regions">
            {displayZones.map((zone) => (
              <li key={zone.name}>
                <div className="compass-ov-region-row">
                  <span><span className="mono muted">{zone.name}</span> · {zone.label}</span>
                  <span className="mono">{zone.pct}%</span>
                </div>
                <div className="compass-ov-region-track">
                  <span style={{ width: `${zone.pct}%` }} />
                </div>
              </li>
            ))}
          </ul>
          {incident ? (
            <div className="compass-ov-incident" role="status">
              <strong>Active incident</strong>
              <p>
                <button type="button" className="linkish" onClick={() => navigate(`/projects/${incident.projectId}`)}>
                  {incident.name}
                </button>
                {' '}
                {incident.error || `is failing health checks in ${incident.zone}.`}
              </p>
            </div>
          ) : (
            <div className="compass-ov-incident is-ok" role="status">
              <strong>No active incidents</strong>
              <p>{online}/{nodes.length || 0} nodes online · {projects.length} projects · signed in as {user.name || user.email}</p>
            </div>
          )}
        </article>
      </div>

      <article className="compass-ov-card compass-ov-apps">
        <div className="compass-ov-apps-head">
          <h2>Applications</h2>
          <span className="muted">{loading ? 'Loading…' : `${filteredApps.length} result${filteredApps.length === 1 ? '' : 's'}`}</span>
        </div>
        {filteredApps.length === 0 ? (
          <div className="compass-empty" style={{ border: 0 }}>
            {loading ? 'Loading applications…' : 'No applications yet. Create a project and deploy a workload.'}
            {!loading ? <div style={{ marginTop: 12 }}><Link to="/projects" className="primary-button">New project</Link></div> : null}
          </div>
        ) : (
          <div className="table-scroll">
            <table className="compass-data-table">
              <thead>
                <tr>
                  {['Name', 'Status', 'Region', 'Instances', 'Node CPU', 'Node RAM', 'Last deploy'].map((header) => (
                    <th key={header}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filteredApps.slice(0, 8).map((app) => {
                  const meta = STATUS_META[app.status] || STATUS_META.pending
                  return (
                    <tr key={app.id} onClick={() => navigate(`/projects/${app.projectId}/workloads/${app.id}`)}>
                      <td>
                        <button type="button" className="compass-table-link" onClick={(event) => { event.stopPropagation(); navigate(`/projects/${app.projectId}/workloads/${app.id}`) }}>
                          {app.name}
                        </button>
                        <div className="table-sub">{app.projectName} · {app.runtime}</div>
                      </td>
                      <td>
                        <span className={`compass-status-inline tone-${meta.tone}`}>
                          <span className={`status-dot ${app.status === 'building' || app.status === 'deploying' ? 'pulse-dot' : ''}`} aria-hidden />
                          {meta.label}
                        </span>
                      </td>
                      <td className="mono">{app.zone}</td>
                      <td className="mono">{app.replicas}</td>
                      <td>{app.cpu == null ? <span className="muted">—</span> : <UsageBar value={app.cpu} />}</td>
                      <td>{app.memory == null ? <span className="muted">—</span> : <UsageBar value={app.memory} />}</td>
                      <td className="mono muted">{app.commit} · {app.ago}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </article>

      <article className="compass-ov-events">
        <div className="compass-ov-events-head">
          <Terminal size={16} aria-hidden />
          Live events
        </div>
        <div className="compass-ov-events-body">
          {liveEvents.length === 0 ? (
            <p className="muted">No events yet — deploy an app to populate this feed.</p>
          ) : (
            liveEvents.map((event) => (
              <div key={event.id} className="compass-ov-event-row">
                <span className="muted">{event.time}</span>
                <span className={`event-level-text tone-${event.tone}`}>{event.level}</span>
                <span className="event-subject">{event.subject}</span>
                <span>{event.detail}</span>
              </div>
            ))
          )}
        </div>
      </article>
    </section>
  )
}

function UsageBar({ value }: { value: number }) {
  return (
    <div className="compass-usage">
      <div className="compass-usage-track">
        <span className={value > 70 ? 'is-warn' : ''} style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
      </div>
      <span className="mono muted">{value}%</span>
    </div>
  )
}

function runtimeLabel(workload: Workload) {
  if (workload.sourceKind === 'git') return 'git'
  if (workload.requireKvm) return 'Cloud Hypervisor'
  const value = Number(workload.runtime)
  if (value === 3) return 'Cloud Hypervisor'
  if (value === 4) return 'WASM'
  if (value === 2) return 'Compose'
  return 'Docker'
}

function toAppRow(workload: Workload, project: Project, nodes: Node[]): AppRow {
  const node = nodes.find((item) => item.id === workload.assignedNodeId)
  const running = workload.status === 'running'
  let cpu: number | null = null
  let memory: number | null = null
  if (running && node) {
    cpu = Math.round(Number(node.cpuUsagePercent || 0))
    const { totalMb, usedMb } = nodeMem(node)
    if (totalMb > 0) memory = Math.round((usedMb / totalMb) * 100)
  }
  const gen = String(workload.observedGeneration || workload.desiredGeneration || workload.id)
  return {
    id: workload.id,
    name: workload.name,
    projectId: project.id,
    projectName: project.name,
    status: workload.status || 'pending',
    zone: workload.preferredZone || node?.zone || 'default',
    replicas: running || workload.status === 'building' || workload.status === 'deploying' ? (workload.replicaCount || 1) : 0,
    runtime: runtimeLabel(workload),
    image: workload.imageOrSource || '',
    error: workload.lastError || '',
    cpu,
    memory,
    commit: shortId(gen, 7),
    ago: workload.status === 'building' || workload.status === 'deploying' ? 'now' : 'live',
  }
}

function buildEvents(
  apps: AppRow[],
  alerts: ObservabilityAlertEvent[],
  nodes: Node[],
) {
  const events: Array<{ id: string; time: string; level: string; tone: string; subject: string; detail: string }> = []

  for (const alert of alerts.slice(0, 5)) {
    const created = alert.createdAt ? new Date(Number(alert.createdAt) * (Number(alert.createdAt) > 1e12 ? 1 : 1000)) : null
    events.push({
      id: `alert-${alert.id}`,
      time: created ? created.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '—',
      level: 'info',
      tone: (alert.status || '').toLowerCase().includes('fail') || (alert.status || '').toLowerCase().includes('error') ? 'danger' : 'info',
      subject: 'alert',
      detail: alert.message || 'Observability alert',
    })
  }

  for (const app of apps.filter((item) => item.status === 'failed').slice(0, 4)) {
    events.push({
      id: `app-fail-${app.id}`,
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      level: 'error',
      tone: 'danger',
      subject: app.name,
      detail: app.error || 'Health check failed',
    })
  }

  for (const app of apps.filter((item) => item.status === 'building' || item.status === 'deploying').slice(0, 3)) {
    events.push({
      id: `app-busy-${app.id}`,
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
      level: 'info',
      tone: 'info',
      subject: app.name,
      detail: `${statusLabel(app.status)} · project ${app.projectName}`,
    })
  }

  for (const node of nodes.filter((item) => item.status !== 'online').slice(0, 3)) {
    events.push({
      id: `node-${node.id}`,
      time: relativeAge(node.lastSeenAt),
      level: 'warn',
      tone: 'warn',
      subject: node.name,
      detail: `Node ${node.status || 'offline'}`,
    })
  }

  if (events.length === 0) {
    for (const app of apps.filter((item) => item.status === 'running').slice(0, 4)) {
      events.push({
        id: `app-ok-${app.id}`,
        time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
        level: 'info',
        tone: 'info',
        subject: app.name,
        detail: `Active · ${app.zone} · ${app.replicas} instance${app.replicas === 1 ? '' : 's'}`,
      })
    }
  }

  return events.slice(0, 8)
}
