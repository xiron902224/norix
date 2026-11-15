import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronRight, Plus, Search } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import { shortId } from '../../../shared/lib/format'
import type { Project } from '../../../gen/v1/workload_pb'
import { createProject, listProjects, listWorkloads } from '../services/projects-api'

interface ProjectSummary extends Project {
  workloadCount: number
  runningCount: number
  failedCount: number
}

function healthStatus(project: ProjectSummary) {
  if (project.failedCount > 0) return 'failed'
  if (project.runningCount > 0) return 'running'
  if (project.workloadCount > 0) return 'stopped'
  return 'pending'
}

export function ProjectsPage() {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<ProjectSummary[]>([])
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)

  async function load() {
    const items = await listProjects()
    const summaries = await Promise.all(items.map(async (project) => {
      try {
        const workloads = await listWorkloads(project.id)
        const visible = workloads.filter((item) => item.status !== 'deleting')
        return {
          ...project,
          workloadCount: visible.length,
          runningCount: visible.filter((item) => item.status === 'running').length,
          failedCount: visible.filter((item) => item.status === 'failed').length,
        } satisfies ProjectSummary
      } catch {
        return { ...project, workloadCount: 0, runningCount: 0, failedCount: 0 } satisfies ProjectSummary
      }
    }))
    setProjects(summaries)
  }

  useEffect(() => {
    void (async () => {
      setLoading(true)
      try {
        await load()
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Could not load projects')
      } finally {
        setLoading(false)
      }
    })()
  }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return projects
    return projects.filter((project) => project.name.toLowerCase().includes(q))
  }, [projects, query])

  const totalWorkloads = projects.reduce((sum, item) => sum + item.workloadCount, 0)
  const totalRunning = projects.reduce((sum, item) => sum + item.runningCount, 0)
  const totalFailed = projects.reduce((sum, item) => sum + item.failedCount, 0)

  const rows: ResourceRow[] = useMemo(() => filtered.map((project) => ({
    id: project.id,
    name: project.name,
    subtitle: shortId(project.id, 12),
    status: healthStatus(project),
    values: [
      String(project.workloadCount),
      String(project.runningCount),
      String(project.failedCount),
    ],
    action: (
      <button
        type="button"
        className="compass-row-chevron"
        aria-label={`Open ${project.name}`}
        onClick={() => navigate(`/projects/${project.id}`)}
      >
        <ChevronRight size={16} />
      </button>
    ),
  })), [filtered, navigate])

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setCreating(true)
    setError('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await createProject(String(form.get('name') ?? ''))
      setModalOpen(false)
      await load()
      navigate(`/projects/${created.id}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create project')
    } finally {
      setCreating(false)
    }
  }

  return (
    <section className="compass-page compass-resource-page">
      <PageHero
        title="Projects"
        subtitle="Projects contain applications (workloads)"
        actions={(
          <Button type="button" onClick={() => setModalOpen(true)}>
            <Plus size={14} /> New project
          </Button>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}

      <MetricStrip
        items={[
          { label: 'Projects', value: loading ? '—' : projects.length },
          { label: 'Applications', value: loading ? '—' : totalWorkloads },
          { label: 'Running', value: loading ? '—' : totalRunning },
          { label: 'Failed', value: loading ? '—' : totalFailed },
        ]}
      />

      <section className="compass-resource-section">
        <div className="compass-toolbar">
          <label className="compass-inline-search">
            <Search size={16} aria-hidden />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label="Search projects"
              placeholder="Search by name…"
            />
          </label>
          <div className="compass-toolbar-right">
            <span className="muted">{loading ? 'Loading…' : `${filtered.length} result${filtered.length === 1 ? '' : 's'}`}</span>
          </div>
        </div>

        <ResourceTable
          headers={['Applications', 'Running', 'Failed']}
          rows={rows}
          onSelect={(row) => navigate(`/projects/${row.id}`)}
          empty={loading ? 'Loading projects…' : 'No projects yet. Create one to deploy applications.'}
          leadingHeader="Project"
        />
      </section>

      <CreateModal
        open={modalOpen}
        title="New project"
        submitLabel="Create project"
        busy={creating}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreate(event)}
      >
        <FormField id="project-name" name="name" label="Name" required placeholder="api-platform" />
        <p className="muted">After creating the project, add applications (workloads) from image or git.</p>
      </CreateModal>
    </section>
  )
}
