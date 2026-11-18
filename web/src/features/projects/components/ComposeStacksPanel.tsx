import { useEffect, useState } from 'react'
import { Layers } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { StatusPill } from '../../../shared/ui/StatusPill'
import { shortId } from '../../../shared/lib/format'
import type { ComposeStack, ComposeStackLogLine, ComposeStackService } from '../../../gen/v1/workload_pb'
import {
  composeStackAction,
  getComposeStack,
  listComposeStacks,
  type ComposeStackAction,
} from '../services/projects-api'

function serviceStatus(svc: ComposeStackService): string {
  if (svc.state === 'exited') return svc.exitCode === 0 ? 'stopped' : 'failed'
  if (svc.state === 'running' && svc.health === 'unhealthy') return 'degraded'
  if (svc.state === 'running' && svc.health === 'starting') return 'deploying'
  if (svc.state === 'running') return 'running'
  if (svc.state === 'restarting' || svc.state === 'dead') return 'failed'
  return 'pending'
}

function serviceLabel(svc: ComposeStackService): string {
  if (svc.state === 'exited') return svc.exitCode === 0 ? 'Completed' : `Exited ${svc.exitCode}`
  if (svc.health) return `${svc.state} · ${svc.health}`
  return svc.state || 'unknown'
}

function stackHint(stack: ComposeStack): string {
  if (stack.message) return stack.message
  if (!stack.assignedNodeId) return 'Waiting for an agent with Docker to claim this stack.'
  if (stack.status === 'pending') return 'Queued. The agent applies it on its next heartbeat.'
  if (stack.status === 'removing') return 'Running docker compose down. Volumes are kept.'
  return ''
}

function StackLogs({ stack }: { stack: ComposeStack }) {
  const [service, setService] = useState('')
  const [lines, setLines] = useState<ComposeStackLogLine[]>([])
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    const load = () => {
      void getComposeStack(stack.id, service, 300)
        .then((result) => {
          if (cancelled) return
          setLines(result.logs)
          setLoaded(true)
        })
        .catch(() => { /* keep last lines */ })
    }
    load()
    const timer = window.setInterval(load, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [stack.id, service])

  const names = ['_deploy', ...stack.services.map((svc) => svc.name)]
  return (
    <div className="compose-stack-logs">
      <label htmlFor={`stack-logs-${stack.id}`}>
        Service
        <select id={`stack-logs-${stack.id}`} value={service} onChange={(event) => setService(event.target.value)}>
          <option value="">All services</option>
          {names.map((name) => (
            <option key={name} value={name}>{name === '_deploy' ? 'deploy output' : name}</option>
          ))}
        </select>
      </label>
      <pre className="log-view log-view-compact">
        {lines.length
          ? lines.slice(-200).map((line) => (service ? line.line : `${line.service.padEnd(14)} | ${line.line}`)).join('\n')
          : (loaded ? 'No log lines yet.' : 'Loading logs…')}
      </pre>
    </div>
  )
}

export function ComposeStacksPanel({ projectId, refreshKey }: { projectId: string; refreshKey: number }) {
  const [stacks, setStacks] = useState<ComposeStack[]>([])
  const [openLogs, setOpenLogs] = useState<string>('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = () => {
      void listComposeStacks(projectId)
        .then((items) => { if (!cancelled) setStacks(items) })
        .catch(() => { /* keep */ })
    }
    load()
    const timer = window.setInterval(load, 4000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [projectId, refreshKey])

  async function onAction(stack: ComposeStack, action: ComposeStackAction) {
    if (action === 'delete' && !window.confirm(`Delete stack “${stack.name}”? Containers are removed; named volumes are kept.`)) return
    setBusy(`${stack.id}:${action}`)
    setError('')
    try {
      const updated = await composeStackAction(stack.id, action)
      setStacks((current) => (
        updated?.status === 'removed'
          ? current.filter((item) => item.id !== stack.id)
          : current.map((item) => (item.id === stack.id && updated ? updated : item))
      ))
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : `Could not ${action} stack`)
    } finally {
      setBusy('')
    }
  }

  if (stacks.length === 0) return null

  return (
    <div className="compass-resource-section">
      <div className="compass-toolbar">
        <strong><Layers size={14} /> {stacks.length} compose stack{stacks.length === 1 ? '' : 's'}</strong>
        <span className="muted">Applied as a unit with docker compose on one agent</span>
      </div>
      {error ? <div className="error" role="alert">{error}</div> : null}
      {stacks.map((stack) => {
        const hint = stackHint(stack)
        const stopped = stack.desiredAction === 'stop'
        return (
          <article key={stack.id} className="compose-stack-card">
            <header className="compose-stack-head">
              <div>
                <strong>{stack.name}</strong>
                <span className="muted mono"> {shortId(stack.gitUrl, 48)} @ {stack.gitRef} · {stack.composePath}</span>
              </div>
              <StatusPill status={stack.status} />
            </header>
            {hint ? <p className={stack.status === 'failed' ? 'error' : 'muted'}>{hint}</p> : null}
            {stack.services.length ? (
              <ul className="compose-stack-services">
                {stack.services.map((svc) => (
                  <li key={svc.name}>
                    <StatusPill status={serviceStatus(svc)} label={svc.name} />
                    <span className="muted"> {serviceLabel(svc)}</span>
                    {svc.ports ? <span className="mono muted"> · {svc.ports}</span> : null}
                  </li>
                ))}
              </ul>
            ) : null}
            <div className="page-actions">
              {stopped ? (
                <Button type="button" variant="secondary" disabled={!!busy} onClick={() => void onAction(stack, 'start')}>Start</Button>
              ) : (
                <Button type="button" variant="secondary" disabled={!!busy} onClick={() => void onAction(stack, 'stop')}>Stop</Button>
              )}
              <Button type="button" variant="secondary" disabled={!!busy} onClick={() => void onAction(stack, 'redeploy')}>Redeploy</Button>
              <Button type="button" variant="secondary" onClick={() => setOpenLogs(openLogs === stack.id ? '' : stack.id)}>
                {openLogs === stack.id ? 'Hide logs' : 'Logs'}
              </Button>
              <Button type="button" variant="secondary" disabled={!!busy} onClick={() => void onAction(stack, 'delete')}>Delete</Button>
              <span className="muted mono">{stack.composeProject} · gen {String(stack.observedGeneration)}/{String(stack.desiredGeneration)}</span>
            </div>
            {openLogs === stack.id ? <StackLogs stack={stack} /> : null}
          </article>
        )
      })}
    </div>
  )
}
