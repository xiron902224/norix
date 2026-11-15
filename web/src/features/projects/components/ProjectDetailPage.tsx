import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ChevronRight, Layers, Plus, TriangleAlert } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal, Modal } from '../../../shared/ui/Modal'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import { shortId } from '../../../shared/lib/format'
import type { ComposeServiceSpec, Project, Workload } from '../../../gen/v1/workload_pb'
import { listNodes } from '../../nodes/services/nodes-api'
import {
  analyzeComposeFromGit,
  createWorkload,
  deployComposeStack,
  deployFromGit,
  listProjects,
  listWorkloads,
} from '../services/projects-api'

function artifactLabel(workload: Workload) {
  if (workload.sourceKind === 'git' || workload.gitUrl) {
    return workload.gitUrl || workload.imageOrSource || 'git'
  }
  return workload.imageOrSource || '—'
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
  const [modalMode, setModalMode] = useState<'image' | 'git' | 'compose' | null>(null)
  const [composeStep, setComposeStep] = useState<'repo' | 'services'>('repo')
  const [composeServices, setComposeServices] = useState<ComposeServiceSpec[]>([])
  const [composeFile, setComposeFile] = useState('docker-compose.yml')
  const [composeGitUrl, setComposeGitUrl] = useState('')
  const [composeGitRef, setComposeGitRef] = useState('main')
  const [composeStackName, setComposeStackName] = useState('')
  const [composeAnalyzing, setComposeAnalyzing] = useState(false)

  function visibleWorkloads(items: Workload[]) {
    return items.filter((item) => item.status !== 'deleting')
  }

  async function refresh() {
    if (!projectId) return
    setError('')
    try {
      const [items, nodes] = await Promise.all([listWorkloads(projectId), listNodes()])
      setWorkloads(visibleWorkloads(items))
      setTotalNodes(nodes.length)
      setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
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
        setWorkloads(visibleWorkloads(items))
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
          setWorkloads(visibleWorkloads(items))
          setTotalNodes(nodes.length)
          setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
        })
        .catch(() => { /* keep */ })
    }, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [projectId])

  async function onCreateImage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await createWorkload({
        projectId,
        name: String(form.get('name') ?? ''),
        imageOrSource: String(form.get('image') ?? ''),
        containerPort: Number(form.get('port') || 80),
        replicaCount: Math.max(1, Number(form.get('replicas') || 1)),
        scheduleStrategy: String(form.get('strategy') || 'spread'),
        preferredZone: String(form.get('zone') || 'default'),
        requireKvm: form.get('requireKvm') === 'on',
      })
      setModalMode(null)
      setNotice('Workload created.')
      await refresh()
      navigate(`/projects/${projectId}/workloads/${created.id}`)
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
    const form = new FormData(event.currentTarget)
    const dockerfilePath = String(form.get('dockerfile') || 'Dockerfile')
    const containerPort = Number(form.get('port') || 80)
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
      setModalMode(null)
      setNotice(`Build queued (${shortId(build.buildId)}).`)
      await refresh()
      navigate(`/projects/${projectId}/workloads/${workload.id}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not start build')
    } finally {
      setCreating(false)
    }
  }

  async function onAnalyzeCompose(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setComposeAnalyzing(true)
    setError('')
    const form = new FormData(event.currentTarget)
    const gitUrl = String(form.get('gitUrl') ?? '')
    const gitRef = String(form.get('gitRef') || 'main')
    const composePath = String(form.get('composePath') || 'docker-compose.yml')
    const stackName = String(form.get('stackName') ?? '')
    try {
      const result = await analyzeComposeFromGit({ gitUrl, gitRef, composePath })
      setComposeGitUrl(gitUrl)
      setComposeGitRef(gitRef)
      setComposeFile(result.composeFile || composePath)
      setComposeStackName(stackName)
      setComposeServices(result.services.map((svc) => ({ ...svc })))
      setComposeStep('services')
      setNotice(`Detected ${result.services.length} compose service${result.services.length === 1 ? '' : 's'} in ${result.composeFile || composePath}.`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not analyze compose file')
    } finally {
      setComposeAnalyzing(false)
    }
  }

  async function onDeployCompose() {
    setCreating(true)
    setError('')
    setNotice('')
    try {
      const result = await deployComposeStack({
        projectId,
        stackName: composeStackName,
        gitUrl: composeGitUrl,
        gitRef: composeGitRef,
        composePath: composeFile,
        services: composeServices,
      })
      setModalMode(null)
      setNotice(`Stack ${shortId(result.stackId)} created with ${result.workloads.length} service${result.workloads.length === 1 ? '' : 's'}.`)
      await refresh()
      const first = result.workloads[0]
      if (first?.id) navigate(`/projects/${projectId}/workloads/${first.id}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not deploy compose stack')
    } finally {
      setCreating(false)
    }
  }

  const running = workloads.filter((item) => item.status === 'running').length
  const deploying = workloads.filter((item) => item.status === 'deploying').length
  const building = workloads.filter((item) => item.status === 'building').length
  const down = workloads.filter((item) => {
    const status = item.status || 'pending'
    return status === 'failed' || status === 'stopped' || status === 'pending'
  }).length
  const hasOnlineNode = onlineNodes > 0

  const rows: ResourceRow[] = useMemo(() => workloads.map((workload) => ({
    id: workload.id,
    name: workload.name,
    subtitle: workload.composeService
      ? `compose · ${workload.composeService}${workload.stackId ? ` · stack ${shortId(workload.stackId, 8)}` : ''}`
      : shortId(artifactLabel(workload), 36),
    status: workload.status || 'pending',
    values: [
      `${String(workload.observedGeneration)}/${String(workload.desiredGeneration)}`,
      workload.hostPort ? `:${workload.hostPort}` : '—',
      String(workload.replicaCount || 1),
    ],
    action: (
      <button
        type="button"
        className="compass-row-chevron"
        aria-label={`Open ${workload.name}`}
        onClick={() => navigate(`/projects/${projectId}/workloads/${workload.id}`)}
      >
        <ChevronRight size={16} />
      </button>
    ),
  })), [workloads, navigate, projectId])

  if (!project) {
    return (
      <section className="compass-page">
        <div className="loading-row">
          <span className="spinner" aria-hidden />
          <span>{loading ? 'Loading project…' : (error || 'Project not found')}</span>
        </div>
        <Button variant="secondary" type="button" onClick={() => navigate('/projects')}>Back</Button>
      </section>
    )
  }

  return (
    <section className="compass-page">
      <PageHero
        eyebrow={(
          <span className="breadcrumb" aria-label="Breadcrumb">
            <Link to="/projects">Projects</Link>
            <ChevronRight size={14} aria-hidden />
            <strong>{project.name}</strong>
          </span>
        )}
        title={project.name}
        subtitle={`Applications (workloads) · ${shortId(project.id, 12)} · agents ${onlineNodes}/${totalNodes}`}
        actions={(
          <>
            <Button type="button" variant="secondary" onClick={() => {
              setComposeStep('repo')
              setComposeServices([])
              setComposeFile('docker-compose.yml')
              setComposeGitUrl('')
              setComposeGitRef('main')
              setComposeStackName('')
              setModalMode('compose')
            }}>
              <Layers size={14} /> Compose stack
            </Button>
            <Button type="button" variant="secondary" onClick={() => setModalMode('git')}>From git</Button>
            <Button type="button" onClick={() => setModalMode('image')}>
              <Plus size={14} /> New application
            </Button>
          </>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      {down > 0 ? (
        <div className="callout callout-warn" role="status">
          <TriangleAlert size={18} />
          <div>
            <strong>{down} application{down === 1 ? '' : 's'} down</strong>
            <p>Open an application and use Start to bring the container up.</p>
          </div>
        </div>
      ) : null}
      {building > 0 ? (
        <ActivityBanner
          title={`Building ${building} application${building === 1 ? '' : 's'} from git`}
          detail="Clone → Dockerfile → tag → automatic deploy."
        />
      ) : null}
      {deploying > 0 ? (
        <ActivityBanner
          title={`Deploying ${deploying} application${deploying === 1 ? '' : 's'}`}
          detail="Desired state queued; the agent reconciles in ~5–10s."
        />
      ) : null}
      {!hasOnlineNode ? (
        <div className="callout callout-warn" role="status">
          <TriangleAlert size={18} />
          <div>
            <strong>No online agent</strong>
            <p>
              {totalNodes === 0
                ? 'Enroll a node from Nodes (join token → norix-agent + Docker).'
                : `${totalNodes} node(s) enrolled but offline. Start the agent.`}
            </p>
          </div>
        </div>
      ) : null}

      <MetricStrip
        items={[
          { label: 'Applications', value: loading ? '—' : workloads.length },
          { label: 'Active', value: loading ? '—' : running },
          { label: 'In progress', value: loading ? '—' : deploying + building },
          { label: 'Agents', value: loading ? '—' : `${onlineNodes}/${totalNodes}` },
        ]}
      />

      <div className="compass-resource-section">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${workloads.length} result${workloads.length === 1 ? '' : 's'}`}</strong>
          <span className="muted">Open a row for Summary · Build logs · Edit</span>
        </div>
        <ResourceTable
          headers={['Gen', 'Port', 'Replicas']}
          rows={rows}
          onSelect={(row) => navigate(`/projects/${projectId}/workloads/${row.id}`)}
          empty="No applications yet. Create one from an image or git."
          leadingHeader="Application"
        />
      </div>

      <CreateModal
        open={modalMode === 'image'}
        title="New application · image"
        submitLabel="Create"
        busy={creating}
        onClose={() => setModalMode(null)}
        onSubmit={(event) => void onCreateImage(event)}
      >
        <FormField id="workload-name" name="name" label="Name" required placeholder="web" />
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
        <FormField id="workload-zone" name="zone" label="Zone" placeholder="default" defaultValue="default" />
        <label htmlFor="workload-kvm" className="checkbox-row">
          <input id="workload-kvm" name="requireKvm" type="checkbox" />
          Requires KVM
        </label>
      </CreateModal>

      <CreateModal
        open={modalMode === 'git'}
        title="Build & deploy from git"
        submitLabel="Build & deploy"
        busy={creating}
        onClose={() => setModalMode(null)}
        onSubmit={(event) => void onCreateGit(event)}
      >
        <FormField id="git-name" name="name" label="Name" required placeholder="hello-go" />
        <FormField id="git-url" name="gitUrl" label="Git URL" required placeholder="https://github.com/org/repo.git" />
        <FormField id="git-ref" name="gitRef" label="Ref" placeholder="main" defaultValue="main" />
        <FormField id="git-dockerfile" name="dockerfile" label="Dockerfile / compose" placeholder="Dockerfile or docker-compose.yml" defaultValue="Dockerfile" />
        <FormField id="git-port" name="port" label="Container port" type="number" placeholder="80" defaultValue={80} />
        <p className="muted">Single Dockerfile apps. For multi-service compose, use <strong>Compose stack</strong>.</p>
        {!hasOnlineNode ? <p className="muted">You need an online agent (or local runtime) to build.</p> : null}
      </CreateModal>

      <Modal
        open={modalMode === 'compose'}
        title={composeStep === 'repo' ? 'Compose stack · analyze' : `Compose services · ${composeFile}`}
        onClose={() => setModalMode(null)}
        footer={composeStep === 'services' ? (
          <>
            <Button type="button" variant="secondary" onClick={() => setComposeStep('repo')}>Back</Button>
            <Button type="button" disabled={creating || !composeServices.some((s) => s.enabled)} onClick={() => void onDeployCompose()}>
              {creating ? 'Deploying…' : `Deploy ${composeServices.filter((s) => s.enabled).length} services`}
            </Button>
          </>
        ) : undefined}
      >
        {composeStep === 'repo' ? (
          <form className="compass-edit-form" onSubmit={(event) => void onAnalyzeCompose(event)}>
            <p className="muted">Like Coolify: parse docker-compose, review each service, ports and dependencies, then deploy as a linked stack.</p>
            <FormField id="compose-name" name="stackName" label="Stack name" required placeholder="todo" />
            <FormField id="compose-url" name="gitUrl" label="Git URL" required placeholder="https://github.com/ajeetraina/todo-app-nodejs-docker.git" />
            <FormField id="compose-ref" name="gitRef" label="Ref" defaultValue="main" />
            <FormField id="compose-path" name="composePath" label="Compose file" defaultValue="docker-compose.yml" placeholder="docker-compose.yml" />
            <div className="page-actions">
              <Button type="submit" disabled={composeAnalyzing}>{composeAnalyzing ? 'Analyzing…' : 'Analyze compose'}</Button>
            </div>
          </form>
        ) : (
          <div className="compose-services-editor">
            <p className="muted">
              Services talk over docker network <code>norix-stack-*</code> using aliases (service names).
              Env vars <code>NORIX_SERVICE_&lt;NAME&gt;_URL</code> are injected automatically.
            </p>
            <div className="table-scroll">
              <table className="compass-data-table">
                <thead>
                  <tr>
                    <th>On</th>
                    <th>Service</th>
                    <th>Build / image</th>
                    <th>Depends</th>
                    <th>Public</th>
                    <th>Host</th>
                    <th>Container</th>
                  </tr>
                </thead>
                <tbody>
                  {composeServices.map((svc, index) => (
                    <tr key={svc.name}>
                      <td>
                        <input
                          type="checkbox"
                          checked={svc.enabled}
                          onChange={(event) => {
                            const next = [...composeServices]
                            next[index] = { ...svc, enabled: event.target.checked }
                            setComposeServices(next)
                          }}
                        />
                      </td>
                      <td>
                        <strong>{svc.name}</strong>
                      </td>
                      <td className="mono muted">
                        {svc.hasBuild
                          ? `${svc.buildContext || '.'}/${svc.dockerfile || 'Dockerfile'}`
                          : (svc.image || '—')}
                      </td>
                      <td className="mono muted">{svc.dependsOn?.length ? svc.dependsOn.join(', ') : '—'}</td>
                      <td>
                        <input
                          type="checkbox"
                          checked={svc.exposePublic}
                          onChange={(event) => {
                            const next = [...composeServices]
                            next[index] = { ...svc, exposePublic: event.target.checked }
                            setComposeServices(next)
                          }}
                        />
                      </td>
                      <td>
                        <input
                          className="compose-port-input"
                          type="number"
                          min={0}
                          value={svc.hostPort || ''}
                          disabled={!svc.exposePublic}
                          onChange={(event) => {
                            const next = [...composeServices]
                            next[index] = { ...svc, hostPort: Number(event.target.value || 0) }
                            setComposeServices(next)
                          }}
                        />
                      </td>
                      <td>
                        <input
                          className="compose-port-input"
                          type="number"
                          min={1}
                          value={svc.containerPort || 80}
                          onChange={(event) => {
                            const next = [...composeServices]
                            next[index] = { ...svc, containerPort: Number(event.target.value || 80) }
                            setComposeServices(next)
                          }}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </Modal>
    </section>
  )
}
