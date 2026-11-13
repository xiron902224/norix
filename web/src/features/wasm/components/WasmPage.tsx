import { useEffect, useState, type FormEvent } from 'react'
import { Box } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import type { WasmFunction } from '../../../gen/v1/wasm_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { createWasmFunction, listWasmFunctions } from '../services/wasm-api'

function statusClass(status: string) {
  if (status === 'running' || status === 'ready') return 'status-running'
  if (status === 'failed') return 'status-failed'
  if (status === 'pending' || status === 'deploying') return 'status-pending'
  return 'status-stopped'
}

export function WasmPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<WasmFunction[]>([])
  const [projectId, setProjectId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)

  async function refresh(selectedProject = projectId) {
    const [projectItems, functions] = await Promise.all([
      listProjects(),
      listWasmFunctions(selectedProject),
    ])
    setProjects(projectItems)
    setItems(functions)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
  }

  useEffect(() => {
    void refresh().catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load Wasm functions'))
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listWasmFunctions(projectId).then(setItems).catch(() => { /* keep */ })
  }, [projectId])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      await createWasmFunction({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        imageRef: String(form.get('imageRef') ?? ''),
        fuelLimit: BigInt(String(form.get('fuelLimit') ?? '1000000000')),
        memoryLimitMb: Number(form.get('memoryLimitMb') ?? 64),
      })
      setNotice('Wasm function created')
      event.currentTarget.reset()
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create Wasm function')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>WebAssembly</h2>
          <p>Wasm components with fuel/memory limits and WASI HTTP (S15).</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      <ActivityBanner
        title="runtime-wasm + wasm-worker"
        detail="CreateWasmFunction is live; DestroyWasmFunction RPC not defined in v1/wasm.proto yet."
      />

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Box /> Functions</div>
          <strong>{items.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Default fuel</div>
          <strong className="mono">1e9</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Default memory</div>
          <strong className="mono">64 MB</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Publish function</h3>
            <p className="muted">Registers a wasm_functions row for the agent dry-run path.</p>
          </div>
        </header>
        <form className="form-grid" onSubmit={(event) => void onCreate(event)}>
          <SelectField
            id="wasm-project"
            name="projectId"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </SelectField>
          <FormField id="wasm-name" name="name" label="Name" required placeholder="token-hasher" />
          <FormField id="wasm-image" name="imageRef" label="OCI Wasm ref" required placeholder="ghcr.io/org/app.wasm:v1" />
          <FormField id="wasm-fuel" name="fuelLimit" label="Fuel limit" defaultValue="1000000000" />
          <FormField id="wasm-mem" name="memoryLimitMb" label="Memory (MB)" type="number" defaultValue="64" />
          <Button type="submit" disabled={busy || !projectId}>{busy ? 'Creating…' : 'Create function'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Functions</h3>
            <p className="muted">{items.length} in scope</p>
          </div>
        </header>
        {items.length === 0 ? (
          <div className="empty-state compact">
            <Box />
            <strong>No Wasm functions</strong>
            <p className="muted">Publish OCI Wasm artifacts, then register a route.</p>
          </div>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Image ref</th>
                  <th>Status</th>
                  <th>Fuel</th>
                  <th>Memory</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <td>
                      <strong>{item.name}</strong>
                      <div className="mono table-sub">{item.id}</div>
                    </td>
                    <td className="mono">{item.imageRef}</td>
                    <td>
                      <span className={`status-chip ${statusClass(item.status)}`}>{item.status || 'unknown'}</span>
                    </td>
                    <td className="mono">{String(item.fuelLimit)}</td>
                    <td className="mono">{item.memoryLimitMb} MB</td>
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
