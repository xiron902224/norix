import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import type { WasmFunction } from '../../../gen/v1/wasm_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { createWasmFunction, listWasmFunctions } from '../services/wasm-api'

export function WasmPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<WasmFunction[]>([])
  const [projectId, setProjectId] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [loading, setLoading] = useState(true)

  async function refresh(selectedProject = projectId) {
    const [projectItems, functions] = await Promise.all([
      listProjects(),
      listWasmFunctions(selectedProject),
    ])
    setProjects(projectItems)
    setItems(functions)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
    if (selectedId && !functions.some((item) => item.id === selectedId)) setSelectedId('')
    setLoading(false)
  }

  useEffect(() => {
    void refresh().catch((cause) => {
      setError(cause instanceof Error ? cause.message : 'Could not load Wasm functions')
      setLoading(false)
    })
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listWasmFunctions(projectId).then((functions) => {
      setItems(functions)
      if (selectedId && !functions.some((item) => item.id === selectedId)) setSelectedId('')
    }).catch(() => { /* keep */ })
  }, [projectId])

  const selected = items.find((item) => item.id === selectedId) ?? null
  const ready = items.filter((item) => item.status === 'running' || item.status === 'ready').length
  const pending = items.filter((item) => item.status === 'pending' || item.status === 'deploying').length
  const failed = items.filter((item) => item.status === 'failed').length

  const rows: ResourceRow[] = useMemo(() => items.map((item) => ({
    id: item.id,
    name: item.name,
    subtitle: item.id,
    status: item.status || 'pending',
    values: [
      item.imageRef || '—',
      String(item.fuelLimit),
      `${item.memoryLimitMb} MB`,
    ],
  })), [items])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await createWasmFunction({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        imageRef: String(form.get('imageRef') ?? ''),
        fuelLimit: BigInt(String(form.get('fuelLimit') ?? '1000000000')),
        memoryLimitMb: Number(form.get('memoryLimitMb') ?? 64),
      })
      setNotice('Wasm function created')
      setModalOpen(false)
      await refresh(projectId)
      if (created?.id) setSelectedId(created.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create Wasm function')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="compass-page">
      <PageHero
        title="Wasm functions"
        subtitle="Wasm components with fuel/memory limits and WASI HTTP"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> New function
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Functions', value: loading ? '—' : items.length },
          { label: 'Active', value: loading ? '—' : ready },
          { label: 'Pending', value: loading ? '—' : pending },
          { label: 'Failed', value: loading ? '—' : failed },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <SelectField id="wasm-project-filter" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
            <option value="" disabled>Select project</option>
            {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
          </SelectField>
        </div>
        <div className="compass-split">
          <ResourceTable
            headers={['Image', 'Fuel', 'Memory']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => setSelectedId(row.id)}
            empty="No Wasm functions in this project."
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a function to view image, fuel, and memory.</p>
              </>
            ) : (
              <>
                <h3>{selected.name}</h3>
                <p className="muted">{selected.status || 'pending'}</p>
                <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                  <div><dt>Status</dt><dd>{selected.status || '—'}</dd></div>
                  <div><dt>Image</dt><dd>{selected.imageRef || '—'}</dd></div>
                  <div><dt>Fuel</dt><dd>{String(selected.fuelLimit)}</dd></div>
                  <div><dt>Memory</dt><dd>{selected.memoryLimitMb} MB</dd></div>
                  <div><dt>ID</dt><dd>{selected.id}</dd></div>
                </dl>
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="New Wasm function"
        submitLabel="Create function"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreate(event)}
      >
        <SelectField id="wasm-project" name="projectId" label="Project" value={projectId} onChange={(e) => setProjectId(e.target.value)} required>
          {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
        </SelectField>
        <FormField id="wasm-name" name="name" label="Name" required placeholder="token-hasher" />
        <FormField id="wasm-image" name="imageRef" label="OCI Wasm reference" required placeholder="ghcr.io/org/app.wasm:v1" />
        <FormField id="wasm-fuel" name="fuelLimit" label="Fuel limit" defaultValue="1000000000" />
        <FormField id="wasm-mem" name="memoryLimitMb" label="Memory (MB)" type="number" defaultValue="64" />
      </CreateModal>
    </section>
  )
}
