import { useEffect, useState, type FormEvent } from 'react'
import { Layers } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { StatusPill } from '../../../shared/ui/StatusPill'
import { shortId } from '../../../shared/lib/format'
import type { ComposeStack, ComposeStackLogLine, ComposeStackService } from '../../../gen/v1/workload_pb'
import {
  composeStackAction,
  getComposeStack,
  listComposeStacks,
  updateComposeStack,
  type ComposeStackAction,
  type ComposeStackSettings,
} from '../services/projects-api'
import {
  domainsToLines,
  parseDomainLines,
  parseServiceList,
  parseVariableLines,
} from '../services/compose-stack-form'

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
  if (stack.status === 'removing') return 'Running docker compose down. Volumes are kept unless you choose to purge them.'
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

/** Read-only summary of what the stack exposes: public services, domains, variables and volumes. */
function StackSettings({ stack }: { stack: ComposeStack }) {
  const domains = Object.entries(stack.domains)
  return (
    <dl className="compose-stack-settings">
      <dt>Public services</dt>
      <dd className="mono">{stack.publicServices.length ? stack.publicServices.join(', ') : 'none (all internal)'}</dd>
      <dt>Domains</dt>
      <dd className="mono">
        {domains.length ? domains.map(([service, host]) => `${host} → ${service}`).join(' · ') : 'none'}
      </dd>
      <dt>Variables</dt>
      <dd className="mono">{stack.variableNames.length ? stack.variableNames.join(', ') : 'none'}</dd>
      <dt>Volumes</dt>
      <dd className="mono">
        {stack.volumes.length
          ? stack.volumes.map((volume) => `${volume.name} (${volume.kind}${volume.source ? `: ${volume.source}` : ''})`).join(' · ')
          : 'none'}
      </dd>
    </dl>
  )
}

/**
 * Edits public services, domains and variables. Values are write-only: the
 * textarea starts empty, and "NAME=" removes a variable.
 */
function StackConfigForm({ stack, onSaved }: { stack: ComposeStack; onSaved: (stack: ComposeStack) => void }) {
  const [publicText, setPublicText] = useState(stack.publicServices.join(', '))
  const [domainText, setDomainText] = useState(domainsToLines(stack.domains))
  const [variableText, setVariableText] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    let settings: ComposeStackSettings
    try {
      settings = {
        publicServices: parseServiceList(publicText),
        domains: parseDomainLines(domainText),
        variables: parseVariableLines(variableText, false),
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Invalid configuration')
      return
    }
    setSaving(true)
    try {
      const updated = await updateComposeStack({ id: stack.id, ...settings })
      setVariableText('')
      if (updated) onSaved(updated)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save configuration')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form className="compass-edit-form compose-stack-config" onSubmit={(event) => void onSubmit(event)}>
      <label htmlFor={`stack-public-${stack.id}`}>
        Public services (published host ports; comma-separated)
        <input id={`stack-public-${stack.id}`} value={publicText} onChange={(event) => setPublicText(event.target.value)} placeholder="web" />
      </label>
      <label htmlFor={`stack-domains-${stack.id}`}>
        Domains (one service=hostname per line; HTTPS/WSS via Caddy)
        <textarea id={`stack-domains-${stack.id}`} rows={3} value={domainText} onChange={(event) => setDomainText(event.target.value)} placeholder="web=app.example.com" />
      </label>
      <label htmlFor={`stack-vars-${stack.id}`}>
        Set variables (NAME=value per line; NAME= removes it; values are never shown again)
        <textarea id={`stack-vars-${stack.id}`} rows={3} value={variableText} onChange={(event) => setVariableText(event.target.value)} placeholder="DATABASE_PASSWORD=change-me" />
      </label>
      {error ? <div className="error" role="alert">{error}</div> : null}
      <div className="page-actions">
        <Button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save configuration'}</Button>
      </div>
    </form>
  )
}

export function ComposeStacksPanel({ projectId, refreshKey }: { projectId: string; refreshKey: number }) {
  const [stacks, setStacks] = useState<ComposeStack[]>([])
  const [openLogs, setOpenLogs] = useState<string>('')
  const [openConfig, setOpenConfig] = useState<string>('')
  const [purgeOnDelete, setPurgeOnDelete] = useState<Record<string, boolean>>({})
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

  function replaceStack(updated: ComposeStack) {
    setStacks((current) => current.map((item) => (item.id === updated.id ? updated : item)))
  }

  async function onAction(stack: ComposeStack, action: ComposeStackAction) {
    const purge = action === 'delete' && !!purgeOnDelete[stack.id]
    if (action === 'delete') {
      const message = purge
        ? `Delete stack “${stack.name}” and PURGE its named volumes and checkout? Data cannot be recovered.`
        : `Delete stack “${stack.name}”? Containers are removed; named volumes are kept.`
      if (!window.confirm(message)) return
    }
    setBusy(`${stack.id}:${action}`)
    setError('')
    try {
      const updated = await composeStackAction(stack.id, action, purge)
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
            <StackSettings stack={stack} />
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
              <Button type="button" variant="secondary" onClick={() => setOpenConfig(openConfig === stack.id ? '' : stack.id)}>
                {openConfig === stack.id ? 'Close settings' : 'Settings'}
              </Button>
              <label className="muted">
                <input
                  type="checkbox"
                  checked={!!purgeOnDelete[stack.id]}
                  onChange={(event) => setPurgeOnDelete((current) => ({ ...current, [stack.id]: event.target.checked }))}
                /> Purge volumes on delete
              </label>
              <Button type="button" variant="secondary" disabled={!!busy} onClick={() => void onAction(stack, 'delete')}>Delete</Button>
              <span className="muted mono">{stack.composeProject} · gen {String(stack.observedGeneration)}/{String(stack.desiredGeneration)}</span>
            </div>
            {openConfig === stack.id ? <StackConfigForm stack={stack} onSaved={replaceStack} /> : null}
            {openLogs === stack.id ? <StackLogs stack={stack} /> : null}
          </article>
        )
      })}
    </div>
  )
}
