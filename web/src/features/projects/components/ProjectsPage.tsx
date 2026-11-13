import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { FolderKanban, Plus, Server } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { shortId } from '../../../shared/lib/format'
import type { Project } from '../../../gen/v1/workload_pb'
import { listNodes } from '../../nodes/services/nodes-api'
import { createProject, deleteProject, listProjects, listWorkloads } from '../services/projects-api'

interface ProjectSummary extends Project {
  workloadCount: number
  runningCount: number
}

export function ProjectsPage() {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [selected, setSelected] = useState<ProjectSummary | null>(null)
  const openProject = (project: Project) => navigate(`/projects/${project.id}`)
  const [onlineNodes, setOnlineNodes] = useState(0)
  const [totalNodes, setTotalNodes] = useState(0)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)

  async function loadSummaries() {
    const [items, nodes] = await Promise.all([listProjects(), listNodes()])
    setTotalNodes(nodes.length)
    setOnlineNodes(nodes.filter((node) => node.status === 'online').length)
    const summaries = await Promise.all(items.map(async (project) => {
      try {
        const workloads = await listWorkloads(project.id)
        return {
          ...project,
          workloadCount: workloads.length,
          runningCount: workloads.filter((item) => item.status === 'running').length,
        } satisfies ProjectSummary
      } catch {
        return { ...project, workloadCount: 0, runningCount: 0 } satisfies ProjectSummary
      }
    }))
    setProjects(summaries)
    setSelected((current) => current ? summaries.find((item) => item.id === current.id) ?? null : null)
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      setError('')
      setLoading(true)
      try {
        await loadSummaries()
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Could not load projects')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    const formEl = event.currentTarget
    const form = new FormData(formEl)
    try {
      await createProject(String(form.get('name') ?? ''))
      formEl.reset()
      await loadSummaries()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create project')
    } finally {
      setCreating(false)
    }
  }

  async function onDelete(project: Project) {
    setError('')
    try {
      await deleteProject(project.id)
      if (selected?.id === project.id) setSelected(null)
      await loadSummaries()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete project')
    }
  }

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Projects</h2>
          <p>Group workloads, then deploy images to an online agent from the project detail page.</p>
        </div>
      </header>

      <div className="stats-grid">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><FolderKanban /> Projects</div>
          <strong>{loading ? '—' : projects.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Plus /> Workloads</div>
          <strong>{loading ? '—' : projects.reduce((sum, item) => sum + item.workloadCount, 0)}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Server /> Agents online</div>
          <strong>{loading ? '—' : `${onlineNodes}/${totalNodes}`}</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Create project</h3>
            <p className="muted">A project is a folder for workloads (containers) you will deploy.</p>
          </div>
        </header>

        {error && <div className="error" role="alert">{error}</div>}

        <form className="inline-form" onSubmit={(event) => void onCreate(event)}>
          <FormField id="project-name" name="name" label="New project" required placeholder="api-service" />
          <Button type="submit" disabled={creating}>{creating ? 'Creating…' : 'Create project'}</Button>
        </form>
      </section>

      <div className="split-detail">
        <section className="panel flux-shadow lift-card panel-table">
          <header className="panel-header">
            <div>
              <h3>All projects</h3>
              <p className="muted">Open to manage workloads, or select for a quick summary.</p>
            </div>
          </header>

          {loading ? (
            <div className="loading-row"><span className="spinner" aria-hidden /><span>Loading projects…</span></div>
          ) : null}

          {!loading && projects.length === 0 ? (
            <div className="empty-state">
              <FolderKanban />
              <strong>No projects yet</strong>
              <p className="muted">Create your first project to start organizing workloads.</p>
            </div>
          ) : null}

          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Project</th>
                  <th>Workloads</th>
                  <th>Running</th>
                  <th>ID</th>
                  <th className="align-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {projects.map((project) => (
                  <tr
                    key={project.id}
                    className={`row-clickable ${selected?.id === project.id ? 'is-selected' : ''}`}
                    onClick={() => setSelected(project)}
                    onDoubleClick={() => openProject(project)}
                  >
                    <td><strong>{project.name}</strong></td>
                    <td className="mono">{project.workloadCount}</td>
                    <td>
                      <span className="status-chip status-running">{project.runningCount}</span>
                    </td>
                    <td className="mono">{shortId(project.id, 12)}</td>
                    <td className="align-right">
                      <div className="row-actions">
                        <Button type="button" variant="secondary" onClick={(event) => { event.stopPropagation(); openProject(project) }}>Open</Button>
                        <Button variant="danger" type="button" onClick={(event) => { event.stopPropagation(); void onDelete(project) }}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <aside className="panel flux-shadow lift-card detail-pane">
          <header className="panel-header">
            <div>
              <h3>Project detail</h3>
              <p className="muted">{selected ? selected.name : 'Select a project'}</p>
            </div>
          </header>
          {!selected ? (
            <div className="empty-state compact">
              <FolderKanban />
              <strong>Nothing selected</strong>
              <p className="muted">Select a project for IDs and workload counts. Double-click or Open to enter.</p>
            </div>
          ) : (
            <dl className="detail-grid">
              <div><dt>Name</dt><dd>{selected.name}</dd></div>
              <div><dt>ID</dt><dd className="mono">{selected.id}</dd></div>
              <div><dt>Organization</dt><dd className="mono">{selected.organizationId}</dd></div>
              <div><dt>Workloads</dt><dd>{selected.workloadCount}</dd></div>
              <div><dt>Running</dt><dd>{selected.runningCount}</dd></div>
              <div><dt>Agents online</dt><dd>{onlineNodes}/{totalNodes}</dd></div>
              <div className="detail-span">
                <dt>Next step</dt>
                <dd>
                  {onlineNodes === 0
                    ? 'Start an agent from Nodes, then open this project and Deploy a workload.'
                    : 'Open the project, create nginx:alpine, click Deploy, wait ~10s for reconcile.'}
                </dd>
              </div>
            </dl>
          )}
          {selected ? (
            <div className="page-actions" style={{ marginTop: 16 }}>
              <Button type="button" onClick={() => openProject(selected)}>Open project</Button>
            </div>
          ) : null}
        </aside>
      </div>
    </section>
  )
}
