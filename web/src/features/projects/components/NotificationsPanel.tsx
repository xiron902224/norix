import { useEffect, useState, type FormEvent } from 'react'
import { Bell, Trash2 } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import type { NotificationChannel } from '../../../gen/v1/workload_pb'
import {
  deleteNotificationChannel,
  listNotificationChannels,
  upsertNotificationChannel,
} from '../services/projects-api'

const EVENT_OPTIONS = [
  { id: 'deploy.succeeded', label: 'Deploy succeeded' },
  { id: 'deploy.failed', label: 'Deploy failed' },
  { id: 'node.down', label: 'Node down' },
] as const

const KIND_OPTIONS = [
  { id: 'webhook', label: 'Webhook', placeholder: 'https://hooks.example.com/norix' },
  { id: 'slack', label: 'Slack', placeholder: 'https://hooks.slack.com/services/…' },
  { id: 'discord', label: 'Discord', placeholder: 'https://discord.com/api/webhooks/…' },
  { id: 'email', label: 'Email', placeholder: 'ops@example.com' },
] as const

function kindLabel(kind: string): string {
  return KIND_OPTIONS.find((item) => item.id === kind)?.label ?? kind
}

function targetPlaceholder(kind: string): string {
  return KIND_OPTIONS.find((item) => item.id === kind)?.placeholder ?? ''
}

export function NotificationsPanel({ projectId }: { projectId: string }) {
  const [channels, setChannels] = useState<NotificationChannel[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState<string>('slack')
  const [target, setTarget] = useState('')
  const [events, setEvents] = useState<string[]>(['deploy.failed', 'node.down'])
  const [enabled, setEnabled] = useState(true)

  async function refresh() {
    setError('')
    try {
      const items = await listNotificationChannels(projectId)
      setChannels(items)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load notification channels')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    setLoading(true)
    void refresh()
  }, [projectId])

  function toggleEvent(eventId: string) {
    setEvents((current) => (
      current.includes(eventId)
        ? current.filter((item) => item !== eventId)
        : [...current, eventId]
    ))
  }

  async function onCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setNotice('')
    try {
      await upsertNotificationChannel({
        projectId,
        name: name.trim(),
        kind,
        target: target.trim(),
        events,
        enabled,
      })
      setName('')
      setTarget('')
      setNotice('Notification channel saved. Targets are write-only; only the host hint is shown again.')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save channel')
    } finally {
      setBusy(false)
    }
  }

  async function onToggleEnabled(channel: NotificationChannel) {
    setBusy(true)
    setError('')
    try {
      await upsertNotificationChannel({
        id: channel.id,
        projectId,
        name: channel.name,
        kind: channel.kind,
        events: [...channel.events],
        enabled: !channel.enabled,
      })
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update channel')
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(channelId: string) {
    setBusy(true)
    setError('')
    try {
      await deleteNotificationChannel(channelId)
      setNotice('Notification channel deleted.')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not delete channel')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="compass-resource-section" style={{ marginTop: 28 }}>
      <div className="compass-toolbar">
        <strong>
          <Bell size={14} style={{ marginRight: 6, verticalAlign: '-2px' }} />
          Notifications
        </strong>
        <span className="muted">Webhook, Slack, Discord, or email · per project</span>
      </div>

      <p className="muted" style={{ marginTop: 0 }}>
        Events go through an outbox with retries. Webhook URLs are checked against the same SSRF rules as Git clones.
        Email needs <code>NORIX_SMTP_ADDR</code> and <code>NORIX_SMTP_FROM</code> on the control plane.
      </p>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <form className="compass-create-form" onSubmit={(event) => void onCreate(event)} style={{ marginBottom: 16 }}>
        <FormField
          id="notify-name"
          label="Name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="ops-slack"
          required
        />
        <label htmlFor="notify-kind">
          Kind
          <select id="notify-kind" value={kind} onChange={(event) => setKind(event.target.value)}>
            {KIND_OPTIONS.map((option) => (
              <option key={option.id} value={option.id}>{option.label}</option>
            ))}
          </select>
        </label>
        <FormField
          id="notify-target"
          label={kind === 'email' ? 'Email address' : 'Webhook URL'}
          value={target}
          onChange={(event) => setTarget(event.target.value)}
          placeholder={targetPlaceholder(kind)}
          required
        />
        <fieldset style={{ border: 0, padding: 0, margin: 0 }}>
          <legend className="muted" style={{ marginBottom: 6 }}>Events</legend>
          {EVENT_OPTIONS.map((option) => (
            <label key={option.id} htmlFor={`notify-event-${option.id}`} className="checkbox-row">
              <input
                id={`notify-event-${option.id}`}
                type="checkbox"
                checked={events.includes(option.id)}
                onChange={() => toggleEvent(option.id)}
              />
              {option.label}
            </label>
          ))}
        </fieldset>
        <label htmlFor="notify-enabled" className="checkbox-row">
          <input
            id="notify-enabled"
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
          />
          Enabled
        </label>
        <Button type="submit" disabled={busy || !name.trim() || !target.trim() || events.length === 0}>
          {busy ? '…' : 'Add channel'}
        </Button>
      </form>

      {loading ? (
        <p className="muted">Loading channels…</p>
      ) : channels.length === 0 ? (
        <p className="muted">No channels yet. Add Slack, Discord, a generic webhook, or email.</p>
      ) : (
        <ul className="release-list">
          {channels.map((channel) => (
            <li key={channel.id} className="release-row">
              <div className="release-meta">
                <strong>{channel.name}</strong>
                <span className="muted">
                  {kindLabel(channel.kind)} · {channel.targetHint || 'configured'}
                  {channel.enabled ? '' : ' · disabled'}
                </span>
                <span className="muted mono">{channel.events.join(', ')}</span>
              </div>
              <div className="release-actions">
                <Button
                  type="button"
                  variant="secondary"
                  disabled={busy}
                  onClick={() => void onToggleEnabled(channel)}
                >
                  {channel.enabled ? 'Disable' : 'Enable'}
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={busy}
                  aria-label={`Delete ${channel.name}`}
                  onClick={() => void onDelete(channel.id)}
                >
                  <Trash2 size={14} />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
