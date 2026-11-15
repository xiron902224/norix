import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import type { Sandbox } from '../../../gen/v1/sandbox_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { createSandbox, destroySandbox, listSandboxes } from '../services/sandboxes-api'

export function SandboxesPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<Sandbox[]>([])
  const [projectId, setProjectId] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [loading, setLoading] = useState(true)
  const [actionBusy, setActionBusy] = useState('')

  async function refresh(selectedProject = projectId) {
    const [projectItems, sandboxes] = await Promise.all([
      listProjects(),
      listSandboxes(selectedProject),
    ])
    setProjects(projectItems)
    setItems(sandboxes)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
    if (selectedId && !sandboxes.some((item) => item.id === selectedId)) setSelectedId('')
    setLoading(false)
  }

  useEffect(() => {
    void refresh().catch((cause) => {
      setError(cause instanceof Error ? cause.message : 'Could not load sandboxes')
      setLoading(false)
    })
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listSandboxes(projectId).then((sandboxes) => {
      setItems(sandboxes)
      if (selectedId && !sandboxes.some((item) => item.id === selectedId)) setSelectedId('')
    }).catch(() => { /* keep */ })
  }, [projectId])

  const selected = items.find((item) => item.id === selectedId) ?? null
  const poolReady = items.filter((item) => item.poolReady).length
  const running = items.filter((item) => item.status === 'running' || item.status === 'ready').length
  const failed = items.filter((item) => item.status === 'failed').length

  const rows: ResourceRow[] = useMemo(() => items.map((item) => ({
    id: item.id,
    name: item.name,
    subtitle: item.id,
    status: item.status || 'pending',
    values: [
      item.template || '—',
      `${item.ttlSeconds}s`,
      item.poolReady ? 'ready' : '—',
    ],
  })), [items])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await createSandbox({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        template: String(form.get('template') ?? 'default'),
        ttlSeconds: Number(form.get('ttlSeconds') ?? 3600),
        cpu: Number(form.get('cpu') ?? 1),
        memoryMb: Number(form.get('memoryMb') ?? 512),
        egressMode: String(form.get('egressMode') ?? 'restricted'),
      })
      setNotice('Sandbox provisioning queued')
      setModalOpen(false)
      await refresh(projectId)
      if (created?.id) setSelectedId(created.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create sandbox')
    } finally {
      setBusy(false)
    }
  }

  async function onDestroy(item: Sandbox) {
    setActionBusy(item.id)
    setError('')
    try {
      await destroySandbox(item.id)
      setNotice('Sandbox destruction requested')
      setSelectedId('')
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not destroy sandbox')
    } finally {
      setActionBusy('')
    }
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Sandboxes"
        subtitle="Ephemeral micro-VMs with TTL, pools, and KVM isolation"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> New sandbox
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Sandboxes', value: loading ? '—' : items.length },
          { label: 'Running', value: loading ? '—' : running },
          { label: 'Pool ready', value: loading ? '—' : poolReady },
          { label: 'Failed', value: loading ? '—' : failed },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <SelectField id="sb-project-filter" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
            <option value="" disabled>Select project</option>
            {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
          </SelectField>
        </div>
        <div className="compass-split">
          <ResourceTable
            headers={['Template', 'TTL', 'Pool']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No sandboxes in this project."
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a sandbox to view resources, egress, and actions.</p>
              </>
            ) : (
              <>
                <h3>{selected.name}</h3>
                <p className="muted">{selected.status || 'pending'}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>Template</dt><dd>{selected.template || '—'}</dd></div>
                  <div><dt>TTL</dt><dd>{selected.ttlSeconds}s</dd></div>
                  <div><dt>CPU</dt><dd>{selected.cpu} vCPU</dd></div>
                  <div><dt>Memory</dt><dd>{selected.memoryMb} MB</dd></div>
                  <div><dt>Egress</dt><dd>{selected.egressMode || '—'}</dd></div>
                  <div><dt>Pool</dt><dd>{selected.poolReady ? 'ready' : 'no'}</dd></div>
                  <div><dt>ID</dt><dd>{selected.id}</dd></div>
                </dl>
                <div className="page-actions" style={{ marginTop: 16 }}>
                  <Button
                    type="button"
                    variant="danger"
                    disabled={actionBusy === selected.id}
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
        title="New sandbox"
        submitLabel="Create sandbox"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreate(event)}
      >
        <SelectField id="sb-project" name="projectId" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
        </SelectField>
        <FormField id="sb-name" name="name" label="Name" required placeholder="eval-01" />
        <FormField id="sb-template" name="template" label="Template" defaultValue="default" />
        <FormField id="sb-ttl" name="ttlSeconds" label="TTL (s)" type="number" defaultValue="3600" />
        <FormField id="sb-cpu" name="cpu" label="vCPU" type="number" defaultValue="1" />
        <FormField id="sb-mem" name="memoryMb" label="Memory (MB)" type="number" defaultValue="512" />
        <SelectField id="sb-egress" name="egressMode" label="Egress" defaultValue="restricted">
          <option value="restricted">restricted</option>
          <option value="open">open</option>
          <option value="none">none</option>
        </SelectField>
      </CreateModal>
    </section>
  )
}
