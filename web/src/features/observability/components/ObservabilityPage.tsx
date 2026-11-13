import { useEffect, useState } from 'react'
import { Activity, HardDrive, LineChart, Radio } from 'lucide-react'
import { listNodes } from '../../nodes/services/nodes-api'
import type { Node } from '../../../gen/v1/agent_pb'
import type { MetricSample, ObservabilityAlertEvent } from '../../../gen/v1/metrics_pb'
import { getClusterMetrics, listAlerts } from '../services/observability-api'

export function ObservabilityPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [samples, setSamples] = useState<MetricSample[]>([])
  const [events, setEvents] = useState<ObservabilityAlertEvent[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    async function refresh() {
      try {
        const [nodeItems, metrics, alerts] = await Promise.all([
          listNodes(),
          getClusterMetrics().catch(() => null),
          listAlerts().catch(() => null),
        ])
        setNodes(nodeItems)
        if (metrics) setSamples(metrics.samples)
        if (alerts) setEvents(alerts.events)
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Could not load metrics')
      }
    }
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 5000)
    return () => window.clearInterval(timer)
  }, [])

  const online = nodes.filter((node) => node.status === 'online')

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Observability</h2>
          <p>Live node telemetry, metric samples, and alert events (S9).</p>
        </div>
      </header>
      {error ? <div className="error" role="alert">{error}</div> : null}

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Radio /> Online nodes</div>
          <strong>{online.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Activity /> Avg CPU</div>
          <strong className="mono">
            {online.length
              ? `${(online.reduce((sum, node) => sum + Number(node.cpuUsagePercent ?? 0), 0) / online.length).toFixed(1)}%`
              : '—'}
          </strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><HardDrive /> Free memory</div>
          <strong className="mono">
            {online.length
              ? `${Math.round(online.reduce((sum, node) => sum + Number(node.memoryFreeMb ?? 0), 0) / 1024)} GB`
              : '—'}
          </strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><LineChart /> Samples</div>
          <strong>{samples.length}</strong>
          <p className="muted">recent ingest</p>
        </article>
      </div>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Node samples</h3>
            <p className="muted">CPU / memory / disk from heartbeat + metric_samples.</p>
          </div>
        </header>
        {nodes.length === 0 ? (
          <p className="muted">No nodes enrolled.</p>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Node</th>
                  <th>Status</th>
                  <th>CPU</th>
                  <th>Mem free</th>
                  <th>Disk free</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((node) => (
                  <tr key={node.id}>
                    <td>
                      <strong>{node.name}</strong>
                      <div className="mono table-sub">{node.id}</div>
                    </td>
                    <td>
                      <span className={`status-chip status-${node.status === 'online' ? 'running' : 'stopped'}`}>{node.status}</span>
                    </td>
                    <td className="mono">{Number(node.cpuUsagePercent || 0).toFixed(1)}%</td>
                    <td className="mono">{String(node.memoryFreeMb || 0)} MB</td>
                    <td className="mono">{String(node.diskFreeMb || 0)} MB</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Alert events</h3>
            <p className="muted">From alert_rules / alert_events.</p>
          </div>
        </header>
        {events.length === 0 ? (
          <p className="muted">No alert events yet.</p>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                {events.map((event) => (
                  <tr key={event.id}>
                    <td><span className="status-chip">{event.status}</span></td>
                    <td>{event.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </section>
  )
}
