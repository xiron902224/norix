import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
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
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
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
  listProjects,
  listWorkloads,
  updateWorkload,
} from '../services/projects-api'

function canDeployImage(workload: Workload) {
  const image = (workload.imageOrSource || '').trim()
  return !!image && image !== 'pending-build' && workload.status !== 'building'
}

export function ProjectDetailPage() {
  const { projectId = '' } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
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
  const [savingEdit, setSavingEdit] = useState(false)
  const [logMode, setLogMode] = useState<'runtime' | 'build'>('runtime')
  const [logBuildId, setLogBuildId] = useState('')
  const [editName, setEditName] = useState('')
  const [editImage, setEditImage] = useState('')
  const [editPort, setEditPort] = useState(80)
  const [editReplicas, setEditReplicas] = useState(1)
  const [editStrategy, setEditStrategy] = useState('spread')
  const [editZone, setEditZone] = useState('default')
  const [editGitUrl, setEditGitUrl] = useState('')
  const [editGitRef, setEditGitRef] = useState('main')
  const [editDockerfile, setEditDockerfile] = useState('Dockerfile')

  function visibleWorkloads(items: Workload[]) {
    return items.filter((item) => item.status !== 'deleting')
  }

  async function refresh() {
    if (!projectId) return
    setError('')
    try {
      const [items, nodes] = await Promise.all([listWorkloads(projectId), listNodes()])
      const visible = visibleWorkloads(items)
      setWorkloads(visible)
      setTotalNodes(nodes.length)
      setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
      if (selected) {
        setSelected(visible.find((item) => item.id === selected.id) ?? null)
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
      if (!projectId) return
      setLoading(true)
      try {
        const [projects, items, nodes] = await Promise.all([
          listProjects(),
          listWorkloads(projectId),
          listNodes(),
        ])
        if (cancelled) return
        const found = projects.find((item) => item.id === projectId) ?? null
        setProject(found)
        if (!found) {
          setError('Project not found')
          setLoading(false)
          return
        }
        const visible = visibleWorkloads(items)
        setWorkloads(visible)
        setTotalNodes(nodes.length)
        setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Could not load workloads')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    const timer = window.setInterval(() => {
      if (!projectId) return
      void Promise.all([listWorkloads(projectId), listNodes()])
        .then(([items, nodes]) => {
          if (cancelled) return
          const visible = visibleWorkloads(items)
          setWorkloads(visible)
          setTotalNodes(nodes.length)
          setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
          setSelected((current) => current ? visible.find((item) => item.id === current.id) ?? null : null)
        })
        .catch(() => { /* keep last snapshot */ })
    }, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [projectId])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setNotice('')
    const formEl = event.currentTarget
    const form = new FormData(formEl)
    try {
      await createWorkload({
        projectId,
        name: String(form.get('name') ?? ''),
        imageOrSource: String(form.get('image') ?? ''),
        containerPort: Number(form.get('port') || 80),
        replicaCount: Math.max(1, Number(form.get('replicas') || 1)),
        scheduleStrategy: String(form.get('strategy') || 'spread'),
        preferredZone: String(form.get('zone') || 'default'),
        requireKvm: form.get('requireKvm') === 'on',
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
    const dockerfilePath = String(form.get('dockerfile') || 'Dockerfile')
    const containerPort = Number(form.get('port') || (dockerfilePath.includes('multi') ? 80 : 80))
    try {
      const workload = await createWorkload({
        projectId,
        name: String(form.get('name') ?? ''),
        sourceKind: 'git',
        gitUrl: String(form.get('gitUrl') ?? ''),
        gitRef: String(form.get('gitRef') || 'main'),
        dockerfilePath,
        containerPort,
      })
      const build = await deployFromGit({
        workloadId: workload.id,
        gitUrl: workload.gitUrl,
        gitRef: workload.gitRef || 'main',
        dockerfilePath: workload.dockerfilePath || dockerfilePath,
        containerPort: workload.containerPort || containerPort,
      })
      formEl.reset()
      setNotice(`Git build queued (${shortId(build.buildId)}). Live build logs open below — deploy starts when the image is ready.`)
      await refresh()
      if (build.buildId) {
        await openBuildLogs(workload, build.buildId)
      }
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
    setNotice('')
    setBusyId(workload.id)
    // Optimistic remove so the row disappears immediately while the agent stops the container.
    setWorkloads((current) => current.filter((item) => item.id !== workload.id))
    if (selected?.id === workload.id) setSelected(null)
    if (logWorkload?.id === workload.id) setLogWorkload(null)
    try {
      await deleteWorkload(workload.id)
      setNotice(`Removing ${workload.name}… container stop is in progress on the node.`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete workload')
      await refresh()
    } finally {
      setBusyId('')
    }
  }

  async function onDeploy(workload: Workload) {
    setError('')
    setNotice('')
    if (!canDeployImage(workload)) {
      setError('Image not ready yet. For git workloads wait for the build to finish (open Build logs), then Deploy.')
      return
    }
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

  function selectWorkload(workload: Workload) {
    setSelected(workload)
    setEditName(workload.name)
    setEditImage(workload.imageOrSource || '')
    setEditPort(workload.containerPort || 80)
    setEditReplicas(workload.replicaCount || 1)
    setEditStrategy(workload.scheduleStrategy || 'spread')
    setEditZone(workload.preferredZone || 'default')
    setEditGitUrl(workload.gitUrl || '')
    setEditGitRef(workload.gitRef || 'main')
    setEditDockerfile(workload.dockerfilePath || 'Dockerfile')
  }

  async function onSaveEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    setSavingEdit(true)
    setError('')
    setNotice('')
    try {
      const updated = await updateWorkload({
        id: selected.id,
        projectId,
        name: editName,
        imageOrSource: editImage,
        containerPort: editPort,
        replicaCount: editReplicas,
        scheduleStrategy: editStrategy,
        preferredZone: editZone,
        sourceKind: editGitUrl ? 'git' : (selected.sourceKind || 'image'),
        gitUrl: editGitUrl,
        gitRef: editGitRef,
        dockerfilePath: editDockerfile,
        memoryLimitMb: Number(selected.memoryLimitMb || 512n),
        cpuShares: selected.cpuShares || 1024,
      })
      setSelected(updated)
      setNotice('Workload details saved.')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update workload')
    } finally {
      setSavingEdit(false)
    }
  }

  async function openLogs(workload: Workload) {
    setLogWorkload(workload)
    setLogMode('runtime')
    setLogBuildId('')
    setLogTitle(`${workload.name} runtime logs (live)`)
    setLogLoading(true)
    setLogLines(['Connecting…'])
  }

  async function openBuildLogs(workload: Workload, buildId?: string) {
    const id = buildId || workload.latestBuildId
    if (!id) {
      setError('No build yet for this workload')
      return
    }
    setLogWorkload(workload)
    setLogMode('build')
    setLogBuildId(id)
    setLogTitle(`${workload.name} build logs (live)`)
    setLogLoading(true)
    setLogLines(['Connecting…'])
  }

  useEffect(() => {
    if (!logWorkload) return
    let cancelled = false
    const tick = async () => {
      try {
        if (logMode === 'build' && logBuildId) {
          const result = await getBuildLogs(logBuildId, 300)
          if (cancelled) return
          const header = result.build
            ? [`build ${result.build.id} · ${result.build.status} · image ${result.build.image || '—'}`, '']
            : []
          setLogLines([
            ...header,
            ...(result.lines.length ? result.lines : ['No build log lines yet. Waiting for builder…']),
          ])
        } else {
          const lines = await getWorkloadLogs(logWorkload.id, 200)
          if (cancelled) return
          setLogLines(lines.length ? lines : [
            'No log lines yet.',
            'Waiting for the runtime to report container logs…',
          ])
        }
      } catch (cause) {
        if (!cancelled) setLogLines([cause instanceof Error ? cause.message : 'Could not load logs'])
      } finally {
        if (!cancelled) setLogLoading(false)
      }
    }
    void tick()
    const timer = window.setInterval(() => { void tick() }, 1000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [logWorkload, logMode, logBuildId])

  const running = workloads.filter((item) => item.status === 'running').length
  const deploying = workloads.filter((item) => item.status === 'deploying').length
  const building = workloads.filter((item) => item.status === 'building').length
  const hasOnlineNode = onlineNodes > 0
  const selectedBusy = selected
    ? selected.status === 'deploying' || selected.status === 'building' || busyId === selected.id || gitBusyId === selected.id
    : false

  if (!project) {
    return (
      <section className="page">
        <div className="loading-row">
          <span className="spinner" aria-hidden />
          <span>{loading ? 'Loading project…' : (error || 'Project not found')}</span>
        </div>
        <Button variant="secondary" type="button" onClick={() => navigate('/projects')}>Back to projects</Button>
      </section>
    )
  }

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <div className="breadcrumb" aria-label="Breadcrumb">
            <Link to="/projects">Projects</Link>
            <ChevronRight size={14} aria-hidden />
            <strong>{project.name}</strong>
          </div>
          <h2>{project.name}</h2>
          <p>Project ID <span className="mono">{project.id}</span> · org <span className="mono">{shortId(project.organizationId, 12)}</span></p>
        </div>
        <div className="page-actions">
          <Button variant="secondary" type="button" onClick={() => navigate('/projects')}>Back to projects</Button>
        </div>
      </header>

      {building > 0 ? (
        <ActivityBanner
          title={`Building ${building} workload${building === 1 ? '' : 's'} from git`}
          detail="Clone → Dockerfile build → image tag → automatic deploy. Open Build logs for live output."
        />
      ) : null}
      {deploying > 0 ? (
        <ActivityBanner
          title={`Deploying ${deploying} workload${deploying === 1 ? '' : 's'}`}
          detail="Desired state is queued; the agent reconciles on the next heartbeat (~5–10s)."
        />
      ) : null}
      {busyId || gitBusyId || creating ? (
        <ActivityBanner
          title={creating ? 'Submitting form…' : gitBusyId ? 'Queuing git rebuild…' : 'Working on workload…'}
          detail="UI actions are disabled for the busy row until the API responds."
          tone="warn"
        />
      ) : null}

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
          <FormField id="workload-replicas" name="replicas" label="Replicas" type="number" placeholder="1" defaultValue={1} min={1} max={32} />
          <label htmlFor="workload-strategy">
            Strategy
            <select id="workload-strategy" name="strategy" defaultValue="spread">
              <option value="spread">spread</option>
              <option value="binpack">binpack</option>
            </select>
          </label>
          <FormField id="workload-zone" name="zone" label="Preferred zone" placeholder="default" defaultValue="default" />
          <label htmlFor="workload-kvm" className="checkbox-row">
            <input id="workload-kvm" name="requireKvm" type="checkbox" />
            Require KVM
          </label>
          <Button type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create workload'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Build &amp; deploy from git</h3>
            <p className="muted">Clones the repo, auto-detects Dockerfile/Containerfile or compose.yaml, builds an image, then deploys.</p>
          </div>
        </header>
        <form className="form-grid form-grid-4" onSubmit={(event) => void onCreateGit(event)}>
          <FormField id="git-name" name="name" label="Workload name" required placeholder="hello-go" />
          <FormField id="git-url" name="gitUrl" label="Git URL" required placeholder="https://github.com/org/repo.git" />
          <FormField id="git-ref" name="gitRef" label="Ref" placeholder="main" defaultValue="main" />
          <FormField id="git-dockerfile" name="dockerfile" label="Dockerfile or compose path" placeholder="Dockerfile.multi or compose.yaml" defaultValue="Dockerfile" />
          <FormField id="git-port" name="port" label="Container port" type="number" placeholder="80" defaultValue={80} />
          <p className="hint-line" style={{ gridColumn: '1 / -1' }}>
            For <code>dockersamples/blog-react-app</code> leave Dockerfile (auto-picks <code>Dockerfile.multi</code>) and port <code>80</code>. Build logs open live after Deploy.
          </p>
          <Button type="submit" disabled={creating || !hasOnlineNode}>
            {creating ? 'Queuing…' : 'Build & deploy'}
          </Button>
        </form>
      </section>

      <div className="split-detail">
        <section className="panel flux-shadow lift-card panel-table">
          <header className="panel-header">
            <div>
              <h3>Workload instances</h3>
              <p className="muted">Select a row for full detail. Polls every 4s.</p>
            </div>
          </header>

          {loading ? (
            <div className="loading-row" style={{ margin: '0 20px 16px' }}><span className="spinner" aria-hidden /><span>Loading workloads…</span></div>
          ) : null}

          {!loading && workloads.length === 0 ? (
            <div className="empty-state">
              <Boxes />
              <strong>No workloads yet</strong>
              <p className="muted">Create nginx:alpine, then Deploy with an online agent.</p>
            </div>
          ) : null}

          {workloads.length > 0 ? (
            <div className="table-scroll">
              <table className="dense-table">
                <thead>
                  <tr>
                    <th>Workload</th>
                    <th>Artifact</th>
                    <th>Status</th>
                    <th>Generation</th>
                    <th>Port</th>
                    <th className="align-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {workloads.map((workload) => {
                    const statusLabel =
                      busyId === workload.id || gitBusyId === workload.id
                        ? (gitBusyId === workload.id ? 'queuing…' : 'working…')
                        : (workload.status || 'pending')
                    const chipClass =
                      workload.status === 'running' ? 'status-running'
                        : workload.status === 'building' || workload.status === 'deploying' ? 'status-building'
                          : workload.status === 'failed' ? 'status-failed'
                            : 'status-pending'
                    return (
                      <tr
                        key={workload.id}
                        className={`row-clickable ${selected?.id === workload.id ? 'is-selected' : ''}`}
                        onClick={() => selectWorkload(workload)}
                      >
                        <td><strong>{workload.name}</strong></td>
                        <td className="mono">{workload.imageOrSource || '—'}</td>
                        <td>
                          <span className={`status-chip ${chipClass}`}>{statusLabel}</span>
                        </td>
                        <td className="mono">{String(workload.observedGeneration)}/{String(workload.desiredGeneration)}</td>
                        <td className="mono">{workload.hostPort ? `:${workload.hostPort}` : '—'}</td>
                        <td className="align-right">
                          <div className="row-actions" onClick={(event) => event.stopPropagation()}>
                            <Button type="button" disabled={busyId === workload.id || gitBusyId === workload.id || !hasOnlineNode || !canDeployImage(workload)} onClick={() => void onDeploy(workload)}>
                              <Rocket size={14} /> {busyId === workload.id ? '…' : 'Deploy'}
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
                                Build
                              </Button>
                            ) : null}
                            <Button type="button" variant="secondary" disabled={busyId === workload.id} onClick={() => void openLogs(workload)}>
                              <ScrollText size={14} />
                            </Button>
                            <Button variant="danger" type="button" disabled={busyId === workload.id} onClick={() => void onDelete(workload)}>
                              Delete
                            </Button>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          ) : null}
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
            <>
              {selectedBusy ? (
                <ActivityBanner
                  title={
                    selected.status === 'building'
                      ? 'Build in progress'
                      : selected.status === 'deploying' || busyId === selected.id
                        ? 'Deploy in progress'
                        : 'Action in progress'
                  }
                  detail={workloadHint(selected.status, selected.observedGeneration, selected.desiredGeneration, hasOnlineNode)}
                />
              ) : null}
              {selected.lastError ? (
                <div className="callout callout-warn" role="alert" style={{ marginBottom: 12 }}>
                  <TriangleAlert size={16} />
                  <div>
                    <strong>Last error</strong>
                    <p className="mono" style={{ margin: '4px 0 0', whiteSpace: 'pre-wrap' }}>{selected.lastError}</p>
                  </div>
                </div>
              ) : null}
              <dl className="detail-grid">
                <div><dt>ID</dt><dd className="mono">{selected.id}</dd></div>
                <div>
                  <dt>Status</dt>
                  <dd>
                    <span className={`status-badge status-${selected.status} ${selectedBusy ? 'is-busy' : ''}`}>
                      {selected.status}
                    </span>
                  </dd>
                </div>
                <div><dt>Source</dt><dd>{selected.sourceKind || 'image'}</dd></div>
                <div><dt>Image</dt><dd className="mono">{selected.imageOrSource || '—'}</dd></div>
                <div><dt>Generations</dt><dd>observed {String(selected.observedGeneration)} / desired {String(selected.desiredGeneration)}</dd></div>
                <div><dt>Ports</dt><dd>host {selected.hostPort || '—'} → container {selected.containerPort || 80}</dd></div>
                <div>
                  <dt>Endpoint</dt>
                  <dd>
                    {selected.endpoint ? (
                      <a className="endpoint-link" href={selected.endpoint} target="_blank" rel="noreferrer">
                        {selected.endpoint} <ExternalLink size={12} />
                      </a>
                    ) : '—'}
                  </dd>
                </div>
                <div><dt>Assigned node</dt><dd className="mono">{selected.assignedNodeId || '—'}</dd></div>
                <div><dt>Container</dt><dd className="mono">{shortId(selected.containerId, 16)}</dd></div>
                <div className="detail-span"><dt>Now</dt><dd>{workloadHint(selected.status, selected.observedGeneration, selected.desiredGeneration, hasOnlineNode)}</dd></div>
              </dl>

              <form className="form-grid form-grid-4" style={{ marginTop: 14 }} onSubmit={(event) => void onSaveEdit(event)}>
                <FormField id="edit-name" label="Name" value={editName} onChange={(e) => setEditName(e.target.value)} required />
                <FormField id="edit-image" label="Image" value={editImage} onChange={(e) => setEditImage(e.target.value)} placeholder="nginx:alpine or pending-build" />
                <FormField id="edit-port" label="Container port" type="number" value={editPort} onChange={(e) => setEditPort(Number(e.target.value || 80))} />
                <FormField id="edit-replicas" label="Replicas" type="number" min={1} max={32} value={editReplicas} onChange={(e) => setEditReplicas(Number(e.target.value || 1))} />
                <label htmlFor="edit-strategy">
                  Strategy
                  <select id="edit-strategy" value={editStrategy} onChange={(e) => setEditStrategy(e.target.value)}>
                    <option value="spread">spread</option>
                    <option value="binpack">binpack</option>
                  </select>
                </label>
                <FormField id="edit-zone" label="Zone" value={editZone} onChange={(e) => setEditZone(e.target.value)} />
                <FormField id="edit-git" label="Git URL" value={editGitUrl} onChange={(e) => setEditGitUrl(e.target.value)} placeholder="https://github.com/org/repo.git" />
                <FormField id="edit-ref" label="Git ref" value={editGitRef} onChange={(e) => setEditGitRef(e.target.value)} />
                <FormField id="edit-dockerfile" label="Dockerfile / compose" value={editDockerfile} onChange={(e) => setEditDockerfile(e.target.value)} />
                <Button type="submit" disabled={savingEdit}>{savingEdit ? 'Saving…' : 'Save details'}</Button>
              </form>

              <div className="detail-actions">
                <Button type="button" disabled={!hasOnlineNode || selectedBusy || !canDeployImage(selected)} onClick={() => void onDeploy(selected)}>
                  <Rocket size={14} /> {selectedBusy && selected.status === 'deploying' ? 'Deploying…' : 'Deploy'}
                </Button>
                <Button type="button" variant="secondary" onClick={() => void openLogs(selected)}>
                  <ScrollText size={14} /> Live runtime logs
                </Button>
                {selected.latestBuildId ? (
                  <Button type="button" variant="secondary" onClick={() => void openBuildLogs(selected)}>
                    Live build logs
                  </Button>
                ) : null}
                {(selected.sourceKind === 'git' || selected.gitUrl) ? (
                  <Button type="button" variant="secondary" disabled={!hasOnlineNode || !!gitBusyId} onClick={() => void onRebuildGit(selected)}>
                    Rebuild from git
                  </Button>
                ) : null}
              </div>
            </>
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
