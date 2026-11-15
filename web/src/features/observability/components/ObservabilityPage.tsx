import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Search } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { listNodes } from '../../nodes/services/nodes-api'
import type { Node } from '../../../gen/v1/agent_pb'
import type { LogSegment, MetricSample, ObservabilityAlertEvent } from '../../../gen/v1/metrics_pb'
import { getClusterMetrics, listAlerts, queryLogs } from '../services/observability-api'

export function ObservabilityPage() {
  const [nodes, setNodes] = useState<Node[]>([])
  const [samples, setSamples] = useState<MetricSample[]>([])
  const [events, setEvents] = useState<ObservabilityAlertEvent[]>([])
  const [logs, setLogs] = useState<LogSegment[]>([])
  const [logQuery, setLogQuery] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [searchBusy, setSearchBusy] = useState(false)

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
        if (selectedId && !nodeItems.some((node) => node.id === selectedId)) setSelectedId('')
        setError('')
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Could not load metrics')
      } finally {
        setLoading(false)
      }
    }
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 5000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    setSearchBusy(true)
    void queryLogs({ query: logQuery, limit: 80 })
      .then((response) => setLogs(response.segments))
      .catch(() => setLogs([]))
      .finally(() => setSearchBusy(false))
  }, [logQuery])

  const online = nodes.filter((node) => node.status === 'online')
  const selected = nodes.find((node) => node.id === selectedId) ?? null
  const avgCpu = online.length
    ? (online.reduce((sum, node) => sum + Number(node.cpuUsagePercent ?? 0), 0) / online.length).toFixed(1)
    : null
  const freeMemGb = online.length
    ? Math.round(online.reduce((sum, node) => sum + Number(node.memoryFreeMb ?? 0), 0) / 1024)
    : null

  const rows: ResourceRow[] = useMemo(() => nodes.map((node) => ({
    id: node.id,
    name: node.name,
    subtitle: node.id,
    status: node.status || 'offline',
    values: [
      `${Number(node.cpuUsagePercent || 0).toFixed(1)}%`,
      `${String(node.memoryFreeMb || 0)} MB`,
      `${String(node.diskFreeMb || 0)} MB`,
    ],
  })), [nodes])

  function onSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    setLogQuery(String(form.get('q') ?? '').trim())
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Observability"
        subtitle="Node telemetry, log search, samples, and alerts"
      />

      {error ? <div className="error" role="alert">{error}</div> : null}

      <MetricStrip
        items={[
          { label: 'Nodes online', value: loading ? '—' : online.length },
          { label: 'Avg CPU', value: avgCpu != null ? `${avgCpu}%` : '—' },
          { label: 'Free RAM', value: freeMemGb != null ? `${freeMemGb} GB` : '—' },
          { label: 'Samples', value: loading ? '—' : samples.length },
          { label: 'Alerts', value: loading ? '—' : events.length },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${nodes.length} node${nodes.length === 1 ? '' : 's'}`}</strong>
          <span className="muted">Poll every 5s · {samples.length} recent samples</span>
        </div>
        <div className="compass-split">
          <ResourceTable
            leadingHeader="Node"
            headers={['CPU', 'Free RAM', 'Free disk']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No enrolled nodes."
          />
          <aside className="compass-side-panel">
            {selected ? (
              <>
                <h3>{selected.name}</h3>
                <p className="muted">{selected.status || 'offline'}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>ID</dt><dd className="mono">{selected.id}</dd></div>
                  <div><dt>CPU</dt><dd>{Number(selected.cpuUsagePercent || 0).toFixed(1)}%</dd></div>
                  <div><dt>Free RAM</dt><dd>{String(selected.memoryFreeMb || 0)} MB</dd></div>
                  <div><dt>Free disk</dt><dd>{String(selected.diskFreeMb || 0)} MB</dd></div>
                </dl>
              </>
            ) : (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a node or search logs / review alerts below.</p>
              </>
            )}

            <h4 style={{ marginTop: 20 }}>Search logs</h4>
            <form className="compass-create-form" style={{ marginTop: 8 }} onSubmit={onSearch}>
              <FormField id="obs-log-q" name="q" label="Query" placeholder="Search logs…" defaultValue={logQuery} />
              <Button type="submit" variant="secondary" disabled={searchBusy}>
                <Search size={14} /> {searchBusy ? '…' : 'Search'}
              </Button>
            </form>
            {logs.length === 0 ? (
              <p className="muted" style={{ marginTop: 8 }}>No matching lines.</p>
            ) : (
              <ul className="resource-list" style={{ marginTop: 8, maxHeight: 180, overflow: 'auto' }}>
                {logs.slice(0, 20).map((line) => (
                  <li key={line.id}>
                    <div className="resource-link">
                      <span className="status-chip">{line.level || 'info'}</span>
                      <p className="hint-line mono">{line.body}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}

            <h4 style={{ marginTop: 20 }}>Alerts</h4>
            {events.length === 0 ? (
              <p className="muted" style={{ marginTop: 8 }}>No alert events.</p>
            ) : (
              <ul className="resource-list" style={{ marginTop: 8 }}>
                {events.slice(0, 10).map((event) => (
                  <li key={event.id}>
                    <div className="resource-link">
                      <strong>{event.status}</strong>
                      <p className="hint-line">{event.message}</p>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </aside>
        </div>
      </div>
    </section>
  )
}
