import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { ExternalLink, Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import type { PRPreview } from '../../../gen/v1/preview_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { destroyPreview, listPreviews, upsertPreview } from '../services/previews-api'

export function PreviewsPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<PRPreview[]>([])
  const [projectId, setProjectId] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [actionBusy, setActionBusy] = useState('')

  async function refresh(selectedProject = projectId) {
    const [projectItems, previews] = await Promise.all([
      listProjects(),
      listPreviews(selectedProject),
    ])
    setProjects(projectItems)
    setItems(previews)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
    if (selectedId && !previews.some((item) => item.id === selectedId)) setSelectedId('')
    setLoading(false)
  }

  useEffect(() => {
    void refresh().catch((cause) => {
      setError(cause instanceof Error ? cause.message : 'Could not load previews')
      setLoading(false)
    })
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listPreviews(projectId).then((previews) => {
      setItems(previews)
      if (selectedId && !previews.some((item) => item.id === selectedId)) setSelectedId('')
    }).catch(() => { /* keep */ })
  }, [projectId])

  const selected = items.find((item) => item.id === selectedId) ?? null
  const active = items.filter((item) => item.status !== 'destroyed').length
  const building = items.filter((item) => item.status === 'building' || item.status === 'pending').length
  const destroyed = items.filter((item) => item.status === 'destroyed').length

  const rows: ResourceRow[] = useMemo(() => items.map((item) => ({
    id: item.id,
    name: item.repo,
    subtitle: `PR #${item.prNumber}`,
    status: item.status || 'pending',
    values: [
      `#${item.prNumber}`,
      item.previewUrl ? item.previewUrl.replace(/^https?:\/\//, '') : '—',
    ],
  })), [items])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await upsertPreview({
        projectId: String(form.get('projectId') ?? projectId),
        repo: String(form.get('repo') ?? ''),
        prNumber: Number(form.get('prNumber') ?? 0),
        previewUrl: String(form.get('previewUrl') ?? ''),
        status: 'pending',
      })
      setNotice('Preview registered')
      setModalOpen(false)
      await refresh(projectId)
      if (created?.id) setSelectedId(created.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not register preview')
    } finally {
      setBusy(false)
    }
  }

  async function onDestroy(item: PRPreview) {
    setActionBusy(item.id)
    setError('')
    try {
      await destroyPreview(item)
      setNotice('Preview marked as destroyed')
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not destroy preview')
    } finally {
      setActionBusy('')
    }
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Previews PR"
        subtitle="Ephemeral environments per pull request with auto-destroy on merge"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> Register preview
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Total', value: loading ? '—' : items.length },
          { label: 'Active', value: loading ? '—' : active },
          { label: 'Building', value: loading ? '—' : building },
          { label: 'Destroyed', value: loading ? '—' : destroyed },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <SelectField id="pv-project-filter" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
            <option value="" disabled>Select project</option>
            {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
          </SelectField>
        </div>
        <div className="compass-split">
          <ResourceTable
            leadingHeader="Repository"
            headers={['PR', 'URL']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No previews in this project."
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a preview to view URL and actions.</p>
              </>
            ) : (
              <>
                <h3>{selected.repo}</h3>
                <p className="muted">PR #{selected.prNumber} · {selected.status}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>Status</dt><dd>{selected.status || '—'}</dd></div>
                  <div><dt>PR</dt><dd>#{selected.prNumber}</dd></div>
                  <div><dt>URL</dt><dd>{selected.previewUrl || '—'}</dd></div>
                  <div><dt>ID</dt><dd>{selected.id}</dd></div>
                </dl>
                <div className="page-actions" style={{ marginTop: 16 }}>
                  {selected.previewUrl ? (
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => window.open(selected.previewUrl, '_blank', 'noopener,noreferrer')}
                    >
                      <ExternalLink size={14} /> Open URL
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    variant="danger"
                    disabled={actionBusy === selected.id || selected.status === 'destroyed'}
                    onClick={() => void onDestroy(selected)}
                  >
                    {actionBusy === selected.id ? '…' : 'Destroy'}
                  </Button>
                </div>
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="Register preview"
        submitLabel="Save preview"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreate(event)}
      >
        <SelectField id="pv-project" name="projectId" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
        </SelectField>
        <FormField id="pv-repo" name="repo" label="Repository" required placeholder="org/app" />
        <FormField id="pv-pr" name="prNumber" label="PR #" type="number" required defaultValue="1" />
        <FormField id="pv-url" name="previewUrl" label="Preview URL" placeholder="https://pr-42.preview.local" />
      </CreateModal>
    </section>
  )
}
