import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  ChevronRight,
  ExternalLink,
  Rocket,
  ScrollText,
  TriangleAlert,
} from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { StatusPill } from '../../../shared/ui/StatusPill'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import { shortId, workloadHint } from '../../../shared/lib/format'
import { DeployProgressSteps } from './DeployProgressSteps'
import type { Project, Workload } from '../../../gen/v1/workload_pb'
import { listNodes } from '../../nodes/services/nodes-api'
import {
  deleteWorkload,
  deployFromGit,
  deployWorkload,
  getBuildLogs,
  getWorkloadLogs,
  listProjects,
  listWorkloadReleases,
  listWorkloads,
  revealWorkloadEnv,
  rollbackWorkload,
  updateWorkload,
} from '../services/projects-api'
import { EnvVarsEditor, envMapToRows, envRowsToMap, type EnvRow } from './EnvVarsEditor'
import type { WorkloadRelease } from '../../../gen/v1/workload_pb'

type DetailTab = 'summary' | 'build' | 'releases' | 'edit'

function canDeployImage(workload: Workload) {
  const image = (workload.imageOrSource || '').trim()
  return !!image && image !== 'pending-build' && workload.status !== 'building'
}

function needsStart(workload: Workload) {
  const status = workload.status || 'pending'
  return status === 'failed' || status === 'stopped' || status === 'pending'
}

function deployLabel(workload: Workload, busy: boolean) {
  if (busy) return '…'
  if (needsStart(workload)) return 'Start'
  return 'Deploy'
}

export function ApplicationDetailPage() {
  const { projectId = '', workloadId = '' } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [workload, setWorkload] = useState<Workload | null>(null)
  const [onlineNodes, setOnlineNodes] = useState(0)
  const [totalNodes, setTotalNodes] = useState(0)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [gitBusy, setGitBusy] = useState(false)
  const [savingEdit, setSavingEdit] = useState(false)
  const [tab, setTab] = useState<DetailTab>('summary')
  const [runtimeLines, setRuntimeLines] = useState<string[]>([])
  const [buildLines, setBuildLines] = useState<string[]>([])
  const [logsLoading, setLogsLoading] = useState(false)

  const [editName, setEditName] = useState('')
  const [editImage, setEditImage] = useState('')
  const [editPort, setEditPort] = useState(80)
  const [editReplicas, setEditReplicas] = useState(1)
  const [editStrategy, setEditStrategy] = useState('spread')
  const [editZone, setEditZone] = useState('default')
  const [editGitUrl, setEditGitUrl] = useState('')
  const [editGitRef, setEditGitRef] = useState('main')
  const [editDockerfile, setEditDockerfile] = useState('Dockerfile')
  const [editEnvRows, setEditEnvRows] = useState<EnvRow[]>([])
  const [editUpdateStrategy, setEditUpdateStrategy] = useState('rolling')
  const [editMaxSurge, setEditMaxSurge] = useState(1)
  const [editMaxUnavailable, setEditMaxUnavailable] = useState(0)
  const [editProbeJson, setEditProbeJson] = useState('{"readiness":{"type":"http","path":"/"},"liveness":{"type":"none"}}')
  const [releases, setReleases] = useState<WorkloadRelease[]>([])
  const [releasesLoading, setReleasesLoading] = useState(false)
  const [rollbackId, setRollbackId] = useState('')
  const [revealBusy, setRevealBusy] = useState(false)
  const tabRef = useRef(tab)
  tabRef.current = tab

  async function refreshReleases() {
    if (!workloadId) return
    setReleasesLoading(true)
    try {
      setReleases(await listWorkloadReleases(workloadId))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load releases')
    } finally {
      setReleasesLoading(false)
    }
  }

  function fillEdit(item: Workload) {
    setEditName(item.name)
    setEditImage(item.imageOrSource || '')
    setEditPort(item.containerPort || 80)
    setEditReplicas(item.replicaCount || 1)
    setEditStrategy(item.scheduleStrategy || 'spread')
    setEditZone(item.preferredZone || 'default')
    setEditGitUrl(item.gitUrl || '')
    setEditGitRef(item.gitRef || 'main')
    setEditDockerfile(item.dockerfilePath || 'Dockerfile')
    setEditEnvRows(envMapToRows(item.envVars))
    setEditUpdateStrategy(item.updateStrategy || 'rolling')
    setEditMaxSurge(item.maxSurge || 1)
    setEditMaxUnavailable(item.maxUnavailable || 0)
    setEditProbeJson(item.probeSpecJson || '{"readiness":{"type":"http","path":"/"},"liveness":{"type":"none"}}')
  }

  async function refresh() {
    if (!projectId || !workloadId) return
    const [projects, items, nodes] = await Promise.all([
      listProjects(),
      listWorkloads(projectId),
      listNodes(),
    ])
    const foundProject = projects.find((item) => item.id === projectId) ?? null
    const foundWorkload = items.find((item) => item.id === workloadId && item.status !== 'deleting') ?? null
    setProject(foundProject)
    setWorkload(foundWorkload)
    setTotalNodes(nodes.length)
    setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
    // Don't clobber the Edit form while the user is typing (env values especially).
    if (foundWorkload && tabRef.current !== 'edit') fillEdit(foundWorkload)
    if (!foundProject) setError('Project not found')
    else if (!foundWorkload) setError('Application not found')
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setLoading(true)
      try {
        await refresh()
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Could not load application')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    const timer = window.setInterval(() => {
      void refresh().catch(() => { /* keep */ })
    }, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [projectId, workloadId])

  useEffect(() => {
    if (!workload) return
    if (workload.status === 'building' && tab !== 'build') {
      setTab('build')
    }
  }, [workload?.status, workload?.id])

  useEffect(() => {
    if (tab !== 'releases') return
    void refreshReleases()
  }, [tab, workloadId, workload?.desiredGeneration, workload?.status])

  useEffect(() => {
    if (!workload || (tab !== 'summary' && tab !== 'build')) return
    let cancelled = false
    const tick = async () => {
      try {
        if (tab === 'build') {
          const buildId = workload.latestBuildId
          if (!buildId) {
            if (!cancelled) setBuildLines(['No build yet. Deploy from git to produce build logs.'])
            return
          }
          const result = await getBuildLogs(buildId, 300)
          if (cancelled) return
          const header = result.build
            ? [`build ${result.build.id} · ${result.build.status} · image ${result.build.image || '—'}`, '']
            : []
          setBuildLines([
            ...header,
            ...(result.lines.length ? result.lines : ['No build log lines yet. Waiting for builder…']),
          ])
        } else {
          const lines = await getWorkloadLogs(workload.id, 80)
          if (cancelled) return
          setRuntimeLines(lines.length ? lines : [])
        }
      } catch (cause) {
        if (cancelled) return
        const message = cause instanceof Error ? cause.message : 'Could not load logs'
        if (tab === 'build') setBuildLines([message])
        else setRuntimeLines([message])
      } finally {
        if (!cancelled) setLogsLoading(false)
      }
    }
    setLogsLoading(true)
    void tick()
    const timer = window.setInterval(() => { void tick() }, 1500)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [tab, workload])

  const hasOnlineNode = onlineNodes > 0
  const selectedBusy = !!workload && (
    workload.status === 'deploying'
    || workload.status === 'building'
    || busy
    || gitBusy
  )

  async function onDeploy() {
    if (!workload) return
    setError('')
    setNotice('')
    if (!canDeployImage(workload)) {
      setError('Image not ready yet. Wait for the build, then Deploy.')
      return
    }
    setBusy(true)
    try {
      const result = await deployWorkload(workload.id, workload.containerPort || 80)
      setNotice(`Deploy queued on node ${shortId(result.assignedNodeId)} → host port ${result.hostPort}.`)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not deploy')
    } finally {
      setBusy(false)
    }
  }

  async function onRevealEnv() {
    if (!workload) return
    if (!window.confirm('Reveal plaintext secrets? This action is audited.')) return
    setRevealBusy(true)
    setError('')
    try {
      const env = await revealWorkloadEnv(workload.id)
      setEditEnvRows(envMapToRows(env))
      setNotice('Secrets revealed in the editor (audited). Save only if you intend to change them.')
      setTab('edit')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not reveal env')
    } finally {
      setRevealBusy(false)
    }
  }

  async function onRollback(releaseId: string) {
    if (!workload) return
    if (!window.confirm('Roll back to this release? Current image/env will be replaced and redeployed.')) return
    setError('')
    setNotice('')
    setRollbackId(releaseId)
    try {
      await rollbackWorkload(workload.id, releaseId)
      setNotice('Rollback queued. Watching status…')
      await refresh()
      await refreshReleases()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not roll back')
    } finally {
      setRollbackId('')
    }
  }

  async function onRebuildGit() {
    if (!workload) return
    setError('')
    setNotice('')
    setGitBusy(true)
    try {
      const build = await deployFromGit({
        workloadId: workload.id,
        gitUrl: workload.gitUrl,
        gitRef: workload.gitRef || 'main',
        dockerfilePath: workload.dockerfilePath || 'Dockerfile',
        containerPort: workload.containerPort || 80,
      })
      setNotice(`Rebuild queued (${shortId(build.buildId)}).`)
      setTab('build')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not rebuild from git')
    } finally {
      setGitBusy(false)
    }
  }

  async function onDelete() {
    if (!workload) return
    if (!window.confirm(`Delete application “${workload.name}”? This cannot be undone.`)) return
    setBusy(true)
    setError('')
    try {
      await deleteWorkload(workload.id)
      navigate(`/projects/${projectId}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete')
      setBusy(false)
    }
  }

  async function onSaveEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!workload) return
    setSavingEdit(true)
    setError('')
    try {
      const updated = await updateWorkload({
        id: workload.id,
        projectId,
        name: editName,
        imageOrSource: editImage,
        containerPort: editPort,
        replicaCount: editReplicas,
        scheduleStrategy: editStrategy,
        preferredZone: editZone,
        sourceKind: editGitUrl ? 'git' : (workload.sourceKind || 'image'),
        gitUrl: editGitUrl,
        gitRef: editGitRef,
        dockerfilePath: editDockerfile,
        memoryLimitMb: Number(workload.memoryLimitMb || 512n),
        cpuShares: workload.cpuShares || 1024,
        envVars: envRowsToMap(editEnvRows),
        updateStrategy: editUpdateStrategy,
        maxSurge: editMaxSurge,
        maxUnavailable: editMaxUnavailable,
        probeSpecJson: editProbeJson,
      })
      setWorkload(updated)
      fillEdit(updated)
      setNotice('Details saved.')
      setTab('summary')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update')
    } finally {
      setSavingEdit(false)
    }
  }

  if (!project || !workload) {
    return (
      <section className="compass-page">
        <div className="loading-row">
          <span className="spinner" aria-hidden />
          <span>{loading ? 'Loading application…' : (error || 'Application not found')}</span>
        </div>
        <Button variant="secondary" type="button" onClick={() => navigate('/projects')}>Back to projects</Button>
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
            <Link to={`/projects/${project.id}`}>{project.name}</Link>
            <ChevronRight size={14} aria-hidden />
            <strong>{workload.name}</strong>
          </span>
        )}
        title={workload.name}
        subtitle={`${workload.sourceKind || 'image'} · ${shortId(workload.id, 12)} · agents ${onlineNodes}/${totalNodes}`}
        actions={(
          <>
            <Button type="button" variant="secondary" onClick={() => navigate(`/projects/${project.id}`)}>
              Project
            </Button>
            {(workload.sourceKind === 'git' || workload.gitUrl) ? (
              <Button type="button" variant="secondary" disabled={!hasOnlineNode || gitBusy} onClick={() => void onRebuildGit()}>
                Rebuild
              </Button>
            ) : null}
            <Button
              type="button"
              disabled={!hasOnlineNode || selectedBusy || !canDeployImage(workload)}
              onClick={() => void onDeploy()}
            >
              <Rocket size={14} /> {deployLabel(workload, busy)}
            </Button>
          </>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <div className="compass-detail-status-row">
        <StatusPill status={workload.status || 'pending'} />
      </div>

      <DeployProgressSteps
        status={workload.status || 'pending'}
        sourceKind={workload.sourceKind || (workload.gitUrl ? 'git' : 'image')}
        endpoint={workload.endpoint || ''}
        lastError={workload.lastError || ''}
      />

      {selectedBusy ? (
        <ActivityBanner
          title={
            workload.status === 'building'
              ? 'Build in progress — logs stream below'
              : workload.status === 'deploying' || busy
                ? 'Deploy in progress'
                : 'Action in progress'
          }
          detail={workloadHint(workload.status, workload.observedGeneration, workload.desiredGeneration, hasOnlineNode)}
        />
      ) : null}

      {workload.lastError ? (
        <div className="callout callout-warn" role="alert">
          <TriangleAlert size={16} />
          <div>
            <strong>Last error</strong>
            <p className="mono" style={{ margin: '4px 0 0', whiteSpace: 'pre-wrap' }}>{workload.lastError}</p>
          </div>
        </div>
      ) : null}

      <div className="compass-tabs" role="tablist" aria-label="Application sections">
        {([
          ['summary', 'Summary'],
          ['build', 'Build logs'],
          ['releases', 'Releases'],
          ['edit', 'Edit'],
        ] as const).map(([id, label]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={tab === id}
            className={`compass-tab${tab === id ? ' is-active' : ''}`}
            onClick={() => setTab(id)}
          >
            {label}
          </button>
        ))}
      </div>

      {tab === 'summary' ? (
        <div className="compass-tab-panel" role="tabpanel">
          <MetricStrip
            items={[
              { label: 'Replicas', value: workload.replicaCount || 1 },
              { label: 'Host port', value: workload.hostPort ? `:${workload.hostPort}` : '—' },
              { label: 'Generation', value: `${String(workload.observedGeneration)}/${String(workload.desiredGeneration)}` },
              { label: 'Region', value: workload.preferredZone || 'default' },
              { label: 'Ready', value: workload.ready === false || workload.status === 'unready' ? 'no' : 'yes' },
            ]}
          />
          {workload.probeStatusJson && workload.probeStatusJson !== '{}' ? (
            <p className="muted mono" style={{ marginTop: 8 }}>probes: {workload.probeStatusJson}</p>
          ) : null}

          <div className="compass-ov-card compass-app-detail-card">
            <h2 className="compass-ov-card-title">Application summary</h2>
            <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
              <div><dt>Image</dt><dd className="mono">{workload.imageOrSource || '—'}</dd></div>
              <div><dt>Source</dt><dd>{workload.sourceKind || 'image'}</dd></div>
              <div><dt>Ports</dt><dd>host {workload.hostPort || '—'} → container {workload.containerPort || 80}</dd></div>
              <div>
                <dt>Endpoint</dt>
                <dd>
                  {workload.endpoint ? (
                    <a className="endpoint-link" href={workload.endpoint} target="_blank" rel="noreferrer">
                      {workload.endpoint} <ExternalLink size={12} />
                    </a>
                  ) : '—'}
                </dd>
              </div>
              <div><dt>Assigned node</dt><dd className="mono">{workload.assignedNodeId || '—'}</dd></div>
              <div><dt>Container</dt><dd className="mono">{shortId(workload.containerId, 16)}</dd></div>
              <div className="detail-span">
                <dt>Environment</dt>
                <dd>
                  {Object.keys(workload.envVars || {}).length === 0 ? (
                    <span className="muted">No env vars</span>
                  ) : (
                    <ul className="env-vars-summary">
                      {Object.entries(workload.envVars).map(([key, value]) => (
                        <li key={key}>
                          <span className="env-key">{key}</span>
                          <span className="muted mono">{value}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </dd>
              </div>
              <div className="detail-span">
                <dt>Now</dt>
                <dd>{workloadHint(workload.status, workload.observedGeneration, workload.desiredGeneration, hasOnlineNode)}</dd>
              </div>
            </dl>

            <div className="compass-summary-logs">
              <div className="compass-summary-logs-head">
                <strong><ScrollText size={14} /> Recent runtime logs</strong>
                <button type="button" className="linkish" onClick={() => setTab('build')}>Build logs →</button>
              </div>
              {logsLoading && runtimeLines.length === 0 ? (
                <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading logs…</span></div>
              ) : (
                <pre className="log-view log-view-compact">
                  {runtimeLines.length ? runtimeLines.slice(-40).join('\n') : 'No runtime log lines yet.'}
                </pre>
              )}
            </div>

            <div className="compass-danger-zone">
              <div>
                <strong>Delete application</strong>
                <p className="muted">Stops the container and removes this workload from the project.</p>
              </div>
              <Button type="button" variant="danger" disabled={busy} onClick={() => void onDelete()}>
                Delete
              </Button>
            </div>
          </div>
        </div>
      ) : null}

      {tab === 'build' ? (
        <div className="compass-tab-panel" role="tabpanel">
          <div className="compass-ov-card">
            <div className="compass-ov-card-head" style={{ marginBottom: 12 }}>
              <h2 className="compass-ov-card-title" style={{ margin: 0 }}>Build logs</h2>
              <span className="muted mono">{workload.latestBuildId ? shortId(workload.latestBuildId, 12) : 'no build'}</span>
            </div>
            {logsLoading && buildLines.length === 0 ? (
              <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading build logs…</span></div>
            ) : (
              <pre className="log-view">{buildLines.join('\n')}</pre>
            )}
          </div>
        </div>
      ) : null}

      {tab === 'releases' ? (
        <div className="compass-tab-panel" role="tabpanel">
          <div className="compass-ov-card">
            <div className="compass-ov-card-head" style={{ marginBottom: 12 }}>
              <h2 className="compass-ov-card-title" style={{ margin: 0 }}>Release history</h2>
              <button type="button" className="linkish" onClick={() => void refreshReleases()}>Refresh</button>
            </div>
            <p className="muted" style={{ marginTop: 0 }}>
              Each deploy stores an immutable snapshot (image + env + limits). Roll back to restore and redeploy.
            </p>
            {releasesLoading && releases.length === 0 ? (
              <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading releases…</span></div>
            ) : releases.length === 0 ? (
              <p className="muted">No releases yet. Deploy once to create history.</p>
            ) : (
              <ul className="release-list">
                {releases.map((rel, index) => (
                  <li key={rel.id} className="release-row">
                    <div className="release-meta">
                      <strong className="mono">gen {String(rel.generation)}</strong>
                      <span className="muted">{rel.source || 'deploy'}</span>
                      <span className="muted mono">{rel.createdAt || ''}</span>
                      {index === 0 ? <span className="status-pill status-pill-running">latest</span> : null}
                    </div>
                    <div className="release-image mono">{rel.imageOrSource || '—'}</div>
                    {rel.imageDigest ? <div className="muted mono">{rel.imageDigest}</div> : null}
                    <div className="release-actions">
                      <Button
                        type="button"
                        variant="secondary"
                        disabled={index === 0 || !!rollbackId || selectedBusy || !hasOnlineNode}
                        onClick={() => void onRollback(rel.id)}
                      >
                        {rollbackId === rel.id ? 'Rolling back…' : 'Roll back'}
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      ) : null}

      {tab === 'edit' ? (
        <div className="compass-tab-panel" role="tabpanel">
          <form className="compass-ov-card compass-edit-form" onSubmit={(event) => void onSaveEdit(event)}>
            <h2 className="compass-ov-card-title">Edit application</h2>
            <div className="compass-edit-grid">
              <FormField id="edit-name" label="Name" value={editName} onChange={(e) => setEditName(e.target.value)} required />
              <FormField id="edit-image" label="Image" value={editImage} onChange={(e) => setEditImage(e.target.value)} />
              <FormField id="edit-port" label="Port" type="number" value={editPort} onChange={(e) => setEditPort(Number(e.target.value || 80))} />
              <FormField id="edit-replicas" label="Replicas" type="number" min={1} max={32} value={editReplicas} onChange={(e) => setEditReplicas(Number(e.target.value || 1))} />
              <label htmlFor="edit-strategy">
                Strategy
                <select id="edit-strategy" value={editStrategy} onChange={(e) => setEditStrategy(e.target.value)}>
                  <option value="spread">spread</option>
                  <option value="binpack">binpack</option>
                </select>
              </label>
              <FormField id="edit-zone" label="Zone" value={editZone} onChange={(e) => setEditZone(e.target.value)} />
              <FormField id="edit-git" label="Git URL" value={editGitUrl} onChange={(e) => setEditGitUrl(e.target.value)} />
              <FormField id="edit-ref" label="Git ref" value={editGitRef} onChange={(e) => setEditGitRef(e.target.value)} />
              <FormField id="edit-dockerfile" label="Dockerfile" value={editDockerfile} onChange={(e) => setEditDockerfile(e.target.value)} />
              <label htmlFor="edit-update-strategy">
                Update strategy
                <select id="edit-update-strategy" value={editUpdateStrategy} onChange={(e) => setEditUpdateStrategy(e.target.value)}>
                  <option value="rolling">rolling (start before stop)</option>
                  <option value="recreate">recreate (stop then start)</option>
                </select>
              </label>
              <FormField id="edit-max-surge" label="Max surge" type="number" min={0} max={32} value={editMaxSurge} onChange={(e) => setEditMaxSurge(Number(e.target.value || 0))} />
              <FormField id="edit-max-unavail" label="Max unavailable" type="number" min={0} max={32} value={editMaxUnavailable} onChange={(e) => setEditMaxUnavailable(Number(e.target.value || 0))} />
            </div>
            <label htmlFor="edit-probes" style={{ display: 'grid', gap: 6, marginTop: 12 }}>
              Probes (JSON)
              <textarea
                id="edit-probes"
                className="mono"
                rows={5}
                value={editProbeJson}
                onChange={(e) => setEditProbeJson(e.target.value)}
                spellCheck={false}
              />
              <span className="muted">readiness/liveness: type http|tcp|exec|none. Liveness fail restarts; readiness fail removes edge routing.</span>
            </label>
            <EnvVarsEditor
              idPrefix="edit-env"
              rows={editEnvRows}
              onChange={setEditEnvRows}
              onReveal={() => void onRevealEnv()}
              revealBusy={revealBusy}
              hint="Leave ******** to keep the current secret. Import .env to merge keys. Reveal (owner/admin) loads plaintext and is audited."
            />
            <div className="page-actions" style={{ marginTop: 16 }}>
              <Button type="submit" disabled={savingEdit}>{savingEdit ? 'Saving…' : 'Save changes'}</Button>
              <Button type="button" variant="secondary" onClick={() => { fillEdit(workload); setTab('summary') }}>Cancel</Button>
            </div>
          </form>
        </div>
      ) : null}
    </section>
  )
}
