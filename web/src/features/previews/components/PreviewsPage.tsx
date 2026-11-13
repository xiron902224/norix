import { useEffect, useState, type FormEvent } from 'react'
import { ExternalLink, GitPullRequest } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import type { PRPreview } from '../../../gen/v1/preview_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { destroyPreview, listPreviews, upsertPreview } from '../services/previews-api'

function statusClass(status: string) {
  if (status === 'running' || status === 'ready') return 'status-running'
  if (status === 'destroyed' || status === 'stopped') return 'status-stopped'
  if (status === 'building' || status === 'pending') return 'status-building'
  if (status === 'failed') return 'status-failed'
  return 'status-stopped'
}

export function PreviewsPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<PRPreview[]>([])
  const [projectId, setProjectId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [destroyingId, setDestroyingId] = useState('')

  async function refresh(selectedProject = projectId) {
    const [projectItems, previews] = await Promise.all([
      listProjects(),
      listPreviews(selectedProject),
    ])
    setProjects(projectItems)
    setItems(previews)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
  }

  useEffect(() => {
    void refresh().catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load previews'))
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listPreviews(projectId).then(setItems).catch(() => { /* keep */ })
  }, [projectId])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      await upsertPreview({
        projectId: String(form.get('projectId') ?? projectId),
        repo: String(form.get('repo') ?? ''),
        prNumber: Number(form.get('prNumber') ?? 0),
        previewUrl: String(form.get('previewUrl') ?? ''),
        status: 'pending',
      })
      setNotice('Preview row upserted')
      event.currentTarget.reset()
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not upsert preview')
    } finally {
      setBusy(false)
    }
  }

  async function onDestroy(item: PRPreview) {
    setDestroyingId(item.id)
    setError('')
    try {
      await destroyPreview(item)
      setNotice('Preview marked destroyed')
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not destroy preview')
    } finally {
      setDestroyingId('')
    }
  }

  const active = items.filter((item) => item.status !== 'destroyed').length

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>PR Previews</h2>
          <p>Ephemeral environments per pull request with auto-destroy on merge (S14).</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      <ActivityBanner
        title="GitHub webhook endpoint"
        detail="POST /hooks/github/{token} · destroy uses UpsertPreview status=destroyed (no Delete RPC yet)."
      />

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><GitPullRequest /> Active previews</div>
          <strong>{active}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Total rows</div>
          <strong className="mono">{items.length}</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Register preview</h3>
            <p className="muted">Manual UpsertPreview for webhook-less testing.</p>
          </div>
        </header>
        <form className="form-grid-domains" onSubmit={(event) => void onCreate(event)}>
          <SelectField
            id="pv-project"
            name="projectId"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </SelectField>
          <FormField id="pv-repo" name="repo" label="Repository" required placeholder="org/app" />
          <FormField id="pv-pr" name="prNumber" label="PR #" type="number" required defaultValue="1" />
          <FormField id="pv-url" name="previewUrl" label="Preview URL" placeholder="https://pr-42.preview.local" />
          <Button type="submit" disabled={busy || !projectId}>{busy ? 'Saving…' : 'Upsert preview'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Preview instances</h3>
            <p className="muted">{items.length} rows</p>
          </div>
        </header>
        {items.length === 0 ? (
          <div className="empty-state compact">
            <GitPullRequest />
            <strong>No active previews</strong>
            <p className="muted">Open a PR against a linked repo or register one above.</p>
          </div>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Repository</th>
                  <th>PR</th>
                  <th>Status</th>
                  <th>URL</th>
                  <th className="align-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <td className="mono">{item.repo}</td>
                    <td className="mono">#{item.prNumber}</td>
                    <td>
                      <span className={`status-chip ${statusClass(item.status)}`}>{item.status}</span>
                    </td>
                    <td>
                      {item.previewUrl ? (
                        <a className="endpoint-link" href={item.previewUrl} target="_blank" rel="noreferrer">
                          {item.previewUrl}
                          <ExternalLink aria-hidden />
                        </a>
                      ) : (
                        <span className="muted">—</span>
                      )}
                    </td>
                    <td className="align-right">
                      <Button
                        type="button"
                        variant="danger"
                        disabled={destroyingId === item.id || item.status === 'destroyed'}
                        onClick={() => void onDestroy(item)}
                      >
                        {destroyingId === item.id ? 'Destroying…' : 'Destroy'}
                      </Button>
                    </td>
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
