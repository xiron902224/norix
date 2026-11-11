import { useEffect, useState, type FormEvent } from 'react'
import {
  Boxes,
  ChevronRight,
  ExternalLink,
  Info,
  Rocket,
  ScrollText,
  Server,
  TriangleAlert,
  X,
} from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { shortId, workloadHint } from '../../../shared/lib/format'
import type { Project, Workload } from '../../../gen/v1/workload_pb'
import { listNodes } from '../../nodes/services/nodes-api'
import {
  createWorkload,
  deleteWorkload,
  deployFromGit,
  deployWorkload,
  getBuildLogs,
  getWorkloadLogs,
  listWorkloads,
} from '../services/projects-api'

interface ProjectDetailPageProps {
  project: Project
  onBack: () => void
}

export function ProjectDetailPage({ project, onBack }: ProjectDetailPageProps) {
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [onlineNodes, setOnlineNodes] = useState(0)
  const [totalNodes, setTotalNodes] = useState(0)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [busyId, setBusyId] = useState('')
  const [logWorkload, setLogWorkload] = useState<Workload | null>(null)
  const [logLines, setLogLines] = useState<string[]>([])
  const [logLoading, setLogLoading] = useState(false)
  const [logTitle, setLogTitle] = useState('logs')
  const [selected, setSelected] = useState<Workload | null>(null)
  const [gitBusyId, setGitBusyId] = useState('')

  async function refresh() {
    setError('')
    try {
      const [items, nodes] = await Promise.all([listWorkloads(project.id), listNodes()])
      setWorkloads(items)
      setTotalNodes(nodes.length)
      setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
      if (selected) {
        setSelected(items.find((item) => item.id === selected.id) ?? null)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load workloads')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setLoading(true)
      try {
        const [items, nodes] = await Promise.all([listWorkloads(project.id), listNodes()])
        if (cancelled) return
        setWorkloads(items)
        setTotalNodes(nodes.length)
        setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Could not load workloads')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    const timer = window.setInterval(() => {
      void Promise.all([listWorkloads(project.id), listNodes()])
        .then(([items, nodes]) => {
          if (cancelled) return
          setWorkloads(items)
          setTotalNodes(nodes.length)
          setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
          setSelected((current) => current ? items.find((item) => item.id === current.id) ?? current : null)
        })
        .catch(() => { /* keep last snapshot */ })
    }, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [project.id])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setNotice('')
    const formEl = event.currentTarget
    const form = new FormData(formEl)
    try {
      await createWorkload({
        projectId: project.id,
        name: String(form.get('name') ?? ''),
        imageOrSource: String(form.get('image') ?? ''),
        containerPort: Number(form.get('port') || 80),
      })
      formEl.reset()
      setNotice('Workload created. Deploy uses an online agent, or the control-plane VPS when no agents are enrolled.')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create workload')
    } finally {
      setCreating(false)
    }
  }

  async function onCreateGit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setNotice('')
    const formEl = event.currentTarget
    const form = new FormData(formEl)
    try {
      const workload = await createWorkload({
        projectId: project.id,
        name: String(form.get('name') ?? ''),
        sourceKind: 'git',
        gitUrl: String(form.get('gitUrl') ?? ''),
        gitRef: String(form.get('gitRef') || 'main'),
        dockerfilePath: String(form.get('dockerfile') || 'Dockerfile'),
        containerPort: Number(form.get('port') || 8080),
      })
      const build = await deployFromGit({
        workloadId: workload.id,
        gitUrl: workload.gitUrl,
        gitRef: workload.gitRef || 'main',
        dockerfilePath: workload.dockerfilePath || 'Dockerfile',
        containerPort: workload.containerPort || 8080,
      })
      formEl.reset()
      setNotice(`Git build queued (${shortId(build.buildId)}). Watch build logs; deploy starts after the image is ready.`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not start git build')
    } finally {
      setCreating(false)
    }
  }

  async function onRebuildGit(workload: Workload) {
    setError('')
    setNotice('')
    setGitBusyId(workload.id)
    try {
      const build = await deployFromGit({
        workloadId: workload.id,
        gitUrl: workload.gitUrl,
        gitRef: workload.gitRef || 'main',
        dockerfilePath: workload.dockerfilePath || 'Dockerfile',
        containerPort: workload.containerPort || 80,
      })
      setNotice(`Rebuild queued (${shortId(build.buildId)}).`)
      await refresh()
      if (build.buildId) {
        await openBuildLogs(workload, build.buildId)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not rebuild from git')
    } finally {
      setGitBusyId('')
    }
  }

  async function onDelete(workload: Workload) {
    setError('')
    setBusyId(workload.id)
    try {
      await deleteWorkload(workload.id)
      if (selected?.id === workload.id) setSelected(null)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete workload')
    } finally {
      setBusyId('')
    }
  }

  async function onDeploy(workload: Workload) {
    setError('')
    setNotice('')
    setBusyId(workload.id)
    try {
      const result = await deployWorkload(workload.id, workload.containerPort || 80)
      setNotice(
        `Deploy queued on node ${shortId(result.assignedNodeId)} → host port ${result.hostPort}. ` +
        'Wait for the next agent heartbeat (~10s) to pull/run the image.',
      )
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not deploy workload')
    } finally {
      setBusyId('')
    }
  }

  async function openLogs(workload: Workload) {
    setLogWorkload(workload)
    setLogTitle(`${workload.name} runtime logs`)
    setLogLoading(true)
    try {
      const lines = await getWorkloadLogs(workload.id, 150)
      setLogLines(lines.length ? lines : [
        'No log lines yet.',
        'Logs appear after the agent starts the container and reports observed state.',
        onlineNodes === 0
          ? 'Right now there is no online agent — start norix-agent first.'
          : 'If status is still deploying, wait one heartbeat cycle.',
      ])
    } catch (cause) {
      setLogLines([cause instanceof Error ? cause.message : 'Could not load logs'])
    } finally {
      setLogLoading(false)
    }
  }

  async function openBuildLogs(workload: Workload, buildId?: string) {
    const id = buildId || workload.latestBuildId
    if (!id) {
      setError('No build yet for this workload')
      return
    }
    setLogWorkload(workload)
    setLogTitle(`${workload.name} build logs`)
    setLogLoading(true)
    try {
      const result = await getBuildLogs(id, 250)
      const header = result.build
        ? [`build ${result.build.id} · ${result.build.status} · image ${result.build.image || '—'}`, '']
        : []
      setLogLines([
        ...header,
        ...(result.lines.length ? result.lines : ['No build log lines yet. Wait for the builder node to clone/build.']),
      ])
    } catch (cause) {
      setLogLines([cause instanceof Error ? cause.message : 'Could not load build logs'])
    } finally {
      setLogLoading(false)
    }
  }

  const running = workloads.filter((item) => item.status === 'running').length
  const deploying = workloads.filter((item) => item.status === 'deploying').length
  const hasOnlineNode = onlineNodes > 0

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <div className="breadcrumb" aria-label="Breadcrumb">
            <button type="button" onClick={onBack}>Projects</button>
            <ChevronRight size={14} aria-hidden />
            <strong>{project.name}</strong>
          </div>
          <h2>{project.name}</h2>
          <p>Project ID <span className="mono">{project.id}</span> · org <span className="mono">{shortId(project.organizationId, 12)}</span></p>
        </div>
        <div className="page-actions">
          <Button variant="secondary" type="button" onClick={onBack}>Back to projects</Button>
        </div>
      </header>

      {!hasOnlineNode ? (
        <div className="callout callout-warn" role="status">
          <TriangleAlert size={18} />
          <div>
            <strong>No online agent</strong>
            <p>
              {totalNodes === 0
                ? 'Enroll a node from Nodes (join token → start norix-agent with Docker).'
                : `${totalNodes} node(s) enrolled but offline. Start the agent so heartbeats resume; stuck deploys will be reclaimed automatically.`}
            </p>
          </div>
        </div>
      ) : (
        <div className="callout callout-ok" role="status">
          <Server size={18} />
          <div>
            <strong>{onlineNodes} agent online</strong>
            <p>Deploy will assign a host port and the agent reconciles on the next heartbeat (~10s).</p>
          </div>
        </div>
      )}

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Boxes /> Workloads</div>
          <strong>{loading ? '—' : workloads.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Running</div>
          <strong>{loading ? '—' : running}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Rocket /> Deploying</div>
          <strong>{loading ? '—' : deploying}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Server /> Agents</div>
          <strong>{loading ? '—' : `${onlineNodes}/${totalNodes}`}</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Add from image</h3>
            <p className="muted">Example: image <code>nginx:alpine</code>, container port <code>80</code>.</p>
          </div>
        </header>

        {error && <div className="error" role="alert">{error}</div>}
        {notice && <div className="notice" role="status">{notice}</div>}

        <form className="form-grid form-grid-4" onSubmit={(event) => void onCreate(event)}>
          <FormField id="workload-name" name="name" label="Workload name" required placeholder="web" />
          <FormField id="workload-image" name="image" label="Image" required placeholder="nginx:alpine" />
          <FormField id="workload-port" name="port" label="Container port" type="number" placeholder="80" defaultValue={80} />
          <Button type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create workload'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Build &amp; deploy from git</h3>
            <p className="muted">Clones the repo, builds the Dockerfile, pushes to the local registry when available, then deploys.</p>
          </div>
        </header>
        <form className="form-grid form-grid-4" onSubmit={(event) => void onCreateGit(event)}>
          <FormField id="git-name" name="name" label="Workload name" required placeholder="hello-go" />
          <FormField id="git-url" name="gitUrl" label="Git URL" required placeholder="https://github.com/org/repo.git" />
          <FormField id="git-ref" name="gitRef" label="Ref" placeholder="main" defaultValue="main" />
          <FormField id="git-dockerfile" name="dockerfile" label="Dockerfile path" placeholder="Dockerfile" defaultValue="Dockerfile" />
          <FormField id="git-port" name="port" label="Container port" type="number" placeholder="8080" defaultValue={8080} />
          <Button type="submit" disabled={creating || !hasOnlineNode}>
            {creating ? 'Queuing…' : 'Build & deploy'}
          </Button>
        </form>
      </section>

      <div className="split-detail">
        <section className="panel flux-shadow lift-card">
          <header className="panel-header">
            <div>
              <h3>Workloads</h3>
              <p className="muted">Select a row for full detail. Polls every 4s.</p>
            </div>
          </header>

          {loading ? (
            <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading workloads…</span></div>
          ) : null}

          {!loading && workloads.length === 0 ? (
            <div className="empty-state">
              <Boxes />
              <strong>No workloads yet</strong>
              <p className="muted">Create nginx:alpine, then Deploy with an online agent.</p>
            </div>
          ) : null}

          <ul className="resource-list">
            {workloads.map((workload) => (
              <li
                key={workload.id}
                className={`workload-row ${selected?.id === workload.id ? 'is-selected' : ''}`}
              >
                <button type="button" className="resource-link" onClick={() => setSelected(workload)}>
                  <strong>{workload.name}</strong>
                  <div className="resource-meta">
                    <span className="mono">{workload.imageOrSource}</span>
                    <span className={`status-badge status-${workload.status || 'pending'}`}>
                      {workload.status || 'pending'}
                    </span>
                    <span className="chip">gen {String(workload.observedGeneration)}/{String(workload.desiredGeneration)}</span>
                  </div>
                  <p className="hint-line">
                    {workloadHint(workload.status, workload.observedGeneration, workload.desiredGeneration, hasOnlineNode)}
                  </p>
                </button>
                <div className="row-actions">
                  <Button type="button" disabled={busyId === workload.id || !hasOnlineNode} onClick={() => void onDeploy(workload)}>
                    <Rocket size={14} /> Deploy
                  </Button>
                  {workload.sourceKind === 'git' || workload.gitUrl ? (
                    <Button
                      type="button"
                      variant="secondary"
                      disabled={gitBusyId === workload.id || !hasOnlineNode || !workload.gitUrl}
                      onClick={() => void onRebuildGit(workload)}
                    >
                      Rebuild
                    </Button>
                  ) : null}
                  {workload.latestBuildId ? (
                    <Button type="button" variant="secondary" onClick={() => void openBuildLogs(workload)}>
                      Build logs
                    </Button>
                  ) : null}
                  <Button type="button" variant="secondary" disabled={busyId === workload.id} onClick={() => void openLogs(workload)}>
                    <ScrollText size={14} /> Logs
                  </Button>
                  <Button variant="danger" type="button" disabled={busyId === workload.id} onClick={() => void onDelete(workload)}>
                    Delete
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </section>

        <aside className="panel flux-shadow lift-card detail-pane">
          <header className="panel-header">
            <div>
              <h3>Workload detail</h3>
              <p className="muted">{selected ? selected.name : 'Select a workload'}</p>
            </div>
            <Info size={16} className="muted-icon" aria-hidden />
          </header>

          {!selected ? (
            <div className="empty-state compact">
              <Info />
              <strong>Nothing selected</strong>
              <p className="muted">Click a workload to inspect ports, node assignment, and generations.</p>
            </div>
          ) : (
            <dl className="detail-grid">
              <div><dt>ID</dt><dd className="mono">{selected.id}</dd></div>
              <div><dt>Status</dt><dd><span className={`status-badge status-${selected.status}`}>{selected.status}</span></dd></div>
              <div><dt>Source</dt><dd>{selected.sourceKind || 'image'}</dd></div>
              <div><dt>Image</dt><dd className="mono">{selected.imageOrSource}</dd></div>
              {selected.gitUrl ? <div><dt>Git</dt><dd className="mono">{selected.gitUrl}@{selected.gitRef || 'main'}</dd></div> : null}
              {selected.latestBuildId ? <div><dt>Latest build</dt><dd className="mono">{selected.latestBuildId}</dd></div> : null}
              <div><dt>Generations</dt><dd>observed {String(selected.observedGeneration)} / desired {String(selected.desiredGeneration)}</dd></div>
              <div><dt>Ports</dt><dd>host {selected.hostPort || '—'} → container {selected.containerPort || 80}</dd></div>
              <div><dt>Endpoint</dt><dd>{selected.endpoint ? <a className="endpoint-link" href={selected.endpoint} target="_blank" rel="noreferrer">{selected.endpoint} <ExternalLink size={12} /></a> : '—'}</dd></div>
              <div><dt>Assigned node</dt><dd className="mono">{selected.assignedNodeId || '—'}</dd></div>
              <div><dt>Container</dt><dd className="mono">{shortId(selected.containerId, 16)}</dd></div>
              <div><dt>Memory / CPU</dt><dd>{String(selected.memoryLimitMb)} MB · shares {selected.cpuShares}</dd></div>
              <div><dt>Last error</dt><dd>{selected.lastError || 'none'}</dd></div>
              <div className="detail-span"><dt>Hint</dt><dd>{workloadHint(selected.status, selected.observedGeneration, selected.desiredGeneration, hasOnlineNode)}</dd></div>
            </dl>
          )}
        </aside>
      </div>

      {logWorkload ? (
        <div className="drawer-backdrop" role="presentation" onClick={() => setLogWorkload(null)}>
          <aside
            className="log-drawer flux-shadow"
            role="dialog"
            aria-label={`Logs for ${logWorkload.name}`}
            onClick={(event) => event.stopPropagation()}
          >
            <header className="panel-header">
              <div>
                <h3>{logTitle}</h3>
                <p className="muted">Reported by the builder/runtime node.</p>
              </div>
              <button type="button" className="icon-button" aria-label="Close logs" onClick={() => setLogWorkload(null)}>
                <X />
              </button>
            </header>
            {logLoading ? (
              <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading logs…</span></div>
            ) : (
              <pre className="log-view">{logLines.join('\n')}</pre>
            )}
          </aside>
        </div>
      ) : null}
    </section>
  )
}
