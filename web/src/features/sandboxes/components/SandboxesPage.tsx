import { useEffect, useState, type FormEvent } from 'react'
import { Boxes } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { SelectField } from '../../../shared/ui/SelectField'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
import type { Sandbox } from '../../../gen/v1/sandbox_pb'
import type { Project } from '../../../gen/v1/workload_pb'
import { listProjects } from '../../projects/services/projects-api'
import { createSandbox, destroySandbox, listSandboxes } from '../services/sandboxes-api'

function statusClass(status: string) {
  if (status === 'running' || status === 'ready') return 'status-running'
  if (status === 'failed') return 'status-failed'
  if (status === 'pending' || status === 'provisioning') return 'status-pending'
  return 'status-stopped'
}

export function SandboxesPage() {
  const [projects, setProjects] = useState<Project[]>([])
  const [items, setItems] = useState<Sandbox[]>([])
  const [projectId, setProjectId] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [destroyingId, setDestroyingId] = useState('')

  async function refresh(selectedProject = projectId) {
    const [projectItems, sandboxes] = await Promise.all([
      listProjects(),
      listSandboxes(selectedProject),
    ])
    setProjects(projectItems)
    setItems(sandboxes)
    if (!selectedProject && projectItems[0]) setProjectId(projectItems[0].id)
  }

  useEffect(() => {
    void refresh().catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not load sandboxes'))
  }, [])

  useEffect(() => {
    if (!projectId) return
    void listSandboxes(projectId).then(setItems).catch(() => { /* keep */ })
  }, [projectId])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    const form = new FormData(event.currentTarget)
    try {
      await createSandbox({
        projectId: String(form.get('projectId') ?? projectId),
        name: String(form.get('name') ?? ''),
        template: String(form.get('template') ?? 'default'),
        ttlSeconds: Number(form.get('ttlSeconds') ?? 3600),
        cpu: Number(form.get('cpu') ?? 1),
        memoryMb: Number(form.get('memoryMb') ?? 512),
        egressMode: String(form.get('egressMode') ?? 'restricted'),
      })
      setNotice('Sandbox provisioning queued')
      event.currentTarget.reset()
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create sandbox')
    } finally {
      setBusy(false)
    }
  }

  async function onDestroy(id: string) {
    setDestroyingId(id)
    setError('')
    try {
      await destroySandbox(id)
      setNotice('Sandbox destroy requested')
      await refresh(projectId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not destroy sandbox')
    } finally {
      setDestroyingId('')
    }
  }

  const poolReady = items.filter((item) => item.poolReady).length

  return (
    <section className="page">
      <header className="page-header">
        <div>
          <h2>Sandboxes</h2>
          <p>Ephemeral micro-VMs with TTL, pools, and KVM isolation (S11–S12).</p>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="notice" role="status">{notice}</div> : null}

      <ActivityBanner
        title="Isolation modes: container · microvm_preferred · microvm_required"
        detail="Nodes advertise KVM; required workloads stay Unschedulable without hardware virtualization."
      />

      <div className="stats-grid stats-grid-4">
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label"><Boxes /> Active sandboxes</div>
          <strong>{items.length}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Pool ready</div>
          <strong className="mono">{poolReady}</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Default TTL</div>
          <strong className="mono">3600s</strong>
        </article>
        <article className="stat-card flux-shadow lift-card">
          <div className="stat-label">Egress</div>
          <strong className="mono">restricted</strong>
        </article>
      </div>

      <section className="panel flux-shadow lift-card">
        <header className="panel-header">
          <div>
            <h3>Create sandbox</h3>
            <p className="muted">Calls SandboxService.CreateSandbox via Connect.</p>
          </div>
        </header>
        <form className="form-grid form-grid-4" onSubmit={(event) => void onCreate(event)}>
          <SelectField
            id="sb-project"
            name="projectId"
            label="Project"
            value={projectId}
            onChange={(event) => setProjectId(event.target.value)}
          >
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
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
          <Button type="submit" disabled={busy || !projectId}>{busy ? 'Creating…' : 'Create sandbox'}</Button>
        </form>
      </section>

      <section className="panel flux-shadow lift-card panel-table">
        <header className="panel-header">
          <div>
            <h3>Sandbox pool</h3>
            <p className="muted">{items.length} in scope · dense table view</p>
          </div>
        </header>
        {items.length === 0 ? (
          <div className="empty-state compact">
            <Boxes />
            <strong>No sandboxes yet</strong>
            <p className="muted">Create one above or via CLI once the agent pool is warm.</p>
          </div>
        ) : (
          <div className="table-scroll">
            <table className="dense-table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Template</th>
                  <th>Status</th>
                  <th>TTL</th>
                  <th>Resources</th>
                  <th>Egress</th>
                  <th className="align-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <td>
                      <strong>{item.name}</strong>
                      <div className="mono table-sub">{item.id}</div>
                    </td>
                    <td className="mono">{item.template || '—'}</td>
                    <td>
                      <span className={`status-chip ${statusClass(item.status)}`}>{item.status || 'unknown'}</span>
                      {item.poolReady ? <span className="chip chip-info">pool</span> : null}
                    </td>
                    <td className="mono">{item.ttlSeconds}s</td>
                    <td className="mono">{item.cpu} vCPU · {item.memoryMb} MB</td>
                    <td className="mono">{item.egressMode || '—'}</td>
                    <td className="align-right">
                      <Button
                        type="button"
                        variant="danger"
                        disabled={destroyingId === item.id}
                        onClick={() => void onDestroy(item.id)}
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
