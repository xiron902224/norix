import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { Download, KeyRound, Plus } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'
import { PageHero } from '../../../shared/ui/PageHero'
import { MetricStrip } from '../../../shared/ui/MetricStrip'
import { ResourceTable, type ResourceRow } from '../../../shared/ui/ResourceTable'
import { CreateModal } from '../../../shared/ui/Modal'
import {
  addTeamMember,
  auditExportUrl,
  createTeam,
  enableTOTP,
  getTeamUsage,
  listSSOProviders,
  listTeams,
  setTeamQuota,
  upsertSSOProvider,
  usageExportUrl,
  verifyTOTP,
  type TeamItem,
  type TeamUsageItem,
} from '../services/teams-api'

function bigintInput(value: bigint) {
  const n = Number(value)
  return Number.isFinite(n) ? String(n) : '0'
}

export function TeamsPage() {
  const [teams, setTeams] = useState<TeamItem[]>([])
  const [selectedId, setSelectedId] = useState('')
  const [usage, setUsage] = useState<TeamUsageItem | null>(null)
  const [ssoProviders, setSsoProviders] = useState<{ id: string; name: string; kind: string; enabled: boolean }[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [actionBusy, setActionBusy] = useState('')
  const [memberEmail, setMemberEmail] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [oidcForm, setOidcForm] = useState({
    name: 'Local OIDC',
    issuerUrl: 'http://127.0.0.1:5556/dex',
    clientId: 'norix-local',
    clientSecret: '',
  })
  const [quotaForm, setQuotaForm] = useState({
    maxCpu: '4',
    maxMemoryMb: '8192',
    maxSandboxes: '5',
    maxPreviews: '10',
    maxDiskGb: '50',
  })

  async function refresh(teamId = selectedId) {
    try {
      const [teamList, providers] = await Promise.all([listTeams(), listSSOProviders()])
      setTeams(teamList)
      setSsoProviders(providers.map((p) => ({ id: p.id, name: p.name, kind: p.kind, enabled: p.enabled })))
      const active = teamId && teamList.some((t) => t.id === teamId)
        ? teamId
        : (teamList[0]?.id || '')
      setSelectedId(active)
      if (active) {
        const snapshot = await getTeamUsage(active)
        setUsage(snapshot.usage)
        setQuotaForm({
          maxCpu: String(snapshot.quota.maxCpu || 0),
          maxMemoryMb: bigintInput(snapshot.quota.maxMemoryMb),
          maxSandboxes: String(snapshot.quota.maxSandboxes || 0),
          maxPreviews: String(snapshot.quota.maxPreviews || 0),
          maxDiskGb: bigintInput(snapshot.quota.maxDiskGb),
        })
      } else {
        setUsage(null)
      }
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load teams')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  async function onCreateTeam(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const name = String(form.get('name') ?? '').trim()
    if (!name) return
    setBusy(true)
    setNotice('')
    setError('')
    try {
      const created = await createTeam(name, '')
      setModalOpen(false)
      setNotice('Team created')
      await refresh(created?.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create team')
    } finally {
      setBusy(false)
    }
  }

  async function onSaveQuota(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedId) return
    setActionBusy('quota')
    setNotice('')
    setError('')
    try {
      await setTeamQuota({
        teamId: selectedId,
        maxCpu: Number(quotaForm.maxCpu) || 0,
        maxMemoryMb: BigInt(quotaForm.maxMemoryMb || '0'),
        maxDiskGb: BigInt(quotaForm.maxDiskGb || '0'),
        maxSandboxes: Number(quotaForm.maxSandboxes) || 0,
        maxPreviews: Number(quotaForm.maxPreviews) || 0,
      })
      setNotice('Quota updated')
      await refresh(selectedId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not save quota')
    } finally {
      setActionBusy('')
    }
  }

  async function onAddMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedId || !memberEmail.trim()) return
    setActionBusy('member')
    setNotice('')
    setError('')
    try {
      await addTeamMember(selectedId, memberEmail.trim(), 'member')
      setMemberEmail('')
      setNotice('Member added')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not add member')
    } finally {
      setActionBusy('')
    }
  }

  async function onUpsertOIDC() {
    setActionBusy('sso')
    setNotice('')
    setError('')
    try {
      await upsertSSOProvider({
        name: oidcForm.name.trim() || 'Local OIDC',
        kind: 'oidc',
        issuerUrl: oidcForm.issuerUrl.trim(),
        clientId: oidcForm.clientId.trim(),
        clientSecret: oidcForm.clientSecret.trim() || undefined,
        enabled: true,
      })
      setNotice('OIDC provider saved — use Sign in with SSO on the login page')
      setOidcForm((prev) => ({ ...prev, clientSecret: '' }))
      await refresh(selectedId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not update SSO')
    } finally {
      setActionBusy('')
    }
  }

  async function onEnableTOTP() {
    setActionBusy('totp')
    setNotice('')
    setError('')
    try {
      const result = await enableTOTP()
      setTotpSecret(result.secret)
      setNotice('Scan or copy the TOTP secret and verify with a 6-digit code')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not enable TOTP')
    } finally {
      setActionBusy('')
    }
  }

  async function onVerifyTOTP() {
    setActionBusy('verify')
    setNotice('')
    setError('')
    try {
      const result = await verifyTOTP(totpCode)
      setNotice(result.verified ? '2FA enabled' : 'Verification failed')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not verify TOTP')
    } finally {
      setActionBusy('')
    }
  }

  async function downloadExport(kind: 'audit' | 'usage', format: 'json' | 'csv') {
    setError('')
    try {
      const meta = kind === 'audit' ? auditExportUrl(format) : usageExportUrl(format)
      const response = await fetch(meta.url, {
        headers: meta.token ? { Authorization: `Bearer ${meta.token}` } : {},
      })
      if (!response.ok) throw new Error(await response.text())
      const blob = await response.blob()
      const href = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = href
      anchor.download = `${kind}-export.${format}`
      anchor.click()
      URL.revokeObjectURL(href)
      setNotice(`Export ${kind} (${format}) downloaded`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Export failed')
    }
  }

  async function selectTeam(id: string) {
    setSelectedId(id)
    setLoading(true)
    await refresh(id)
  }

  const selected = teams.find((team) => team.id === selectedId)
  const defaults = teams.filter((team) => team.isDefault).length

  const rows: ResourceRow[] = useMemo(() => teams.map((team) => ({
    id: team.id,
    name: team.name,
    subtitle: team.slug,
    status: 'active',
    values: [team.isDefault ? 'yes' : '—'],
  })), [teams])

  return (
    <section className="compass-page">
      <PageHero
        title="Teams"
        subtitle="Teams, quotas, SSO, and usage export"
        actions={(
          <>
            <Button variant="secondary" type="button" onClick={() => void downloadExport('audit', 'json')}>
              <Download size={14} /> Audit JSON
            </Button>
            <Button variant="secondary" type="button" onClick={() => void downloadExport('usage', 'csv')}>
              <Download size={14} /> Usage CSV
            </Button>
            <Button type="button" onClick={() => setModalOpen(true)}>
              <Plus size={14} /> New team
            </Button>
          </>
        )}
      />

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <div className="compass-notice" role="status">{notice}</div> : null}

      <MetricStrip
        items={[
          { label: 'Teams', value: loading ? '—' : teams.length },
          { label: 'Default', value: loading ? '—' : defaults },
          { label: 'SSO', value: loading ? '—' : ssoProviders.length },
          { label: 'CPU used', value: usage ? String(usage.cpuUsed) : '—' },
        ]}
      />

      <div className="compass-panel">
        <div className="compass-toolbar">
          <strong>{loading ? 'Loading…' : `${teams.length} result${teams.length === 1 ? '' : 's'}`}</strong>
        </div>
        <div className="compass-split">
          <ResourceTable
            leadingHeader="Team"
            headers={['Default']}
            rows={rows}
            selectedId={selectedId}
            onSelect={(row) => void selectTeam(row.id)}
            empty="No teams. Create one to manage quotas and members."
          />
          <aside className="compass-side-panel">
            {!selected ? (
              <>
                <h3>Detail</h3>
                <p className="muted">Select a team for quota, members, and SSO.</p>
              </>
            ) : (
              <>
                <h3>{selected.name}</h3>
                <p className="muted">{selected.slug}{selected.isDefault ? ' · default' : ''}</p>

                {usage ? (
                  <dl className="compass-detail-grid" style={{ marginTop: 12 }}>
                    <div><dt>CPU used</dt><dd>{usage.cpuUsed}</dd></div>
                    <div><dt>RAM MB</dt><dd>{String(usage.memoryMbUsed)}</dd></div>
                    <div><dt>Sandboxes</dt><dd>{usage.sandboxesUsed}</dd></div>
                    <div><dt>Previews</dt><dd>{usage.previewsUsed}</dd></div>
                    <div><dt>Disk GB</dt><dd>{String(usage.diskGbUsed)}</dd></div>
                  </dl>
                ) : null}

                <form className="compass-create-form" style={{ marginTop: 16 }} onSubmit={(event) => void onSaveQuota(event)}>
                  <h4>Quota</h4>
                  <FormField
                    id="quota-cpu"
                    label="Max vCPU"
                    value={quotaForm.maxCpu}
                    onChange={(e) => setQuotaForm((v) => ({ ...v, maxCpu: e.target.value }))}
                  />
                  <FormField
                    id="quota-ram"
                    label="Max RAM MB"
                    value={quotaForm.maxMemoryMb}
                    onChange={(e) => setQuotaForm((v) => ({ ...v, maxMemoryMb: e.target.value }))}
                  />
                  <FormField
                    id="quota-sandboxes"
                    label="Max sandboxes"
                    value={quotaForm.maxSandboxes}
                    onChange={(e) => setQuotaForm((v) => ({ ...v, maxSandboxes: e.target.value }))}
                  />
                  <FormField
                    id="quota-previews"
                    label="Max previews"
                    value={quotaForm.maxPreviews}
                    onChange={(e) => setQuotaForm((v) => ({ ...v, maxPreviews: e.target.value }))}
                  />
                  <FormField
                    id="quota-disk"
                    label="Max disk GB"
                    value={quotaForm.maxDiskGb}
                    onChange={(e) => setQuotaForm((v) => ({ ...v, maxDiskGb: e.target.value }))}
                  />
                  <p className="muted">0 = unlimited. Exceeding quota → FailedPrecondition without affecting other teams.</p>
                  <Button type="submit" disabled={actionBusy === 'quota'}>
                    {actionBusy === 'quota' ? 'Saving…' : 'Save quota'}
                  </Button>
                </form>

                <form className="compass-create-form" style={{ marginTop: 16 }} onSubmit={(event) => void onAddMember(event)}>
                  <h4>Members</h4>
                  <FormField
                    id="team-member"
                    label="Email"
                    value={memberEmail}
                    onChange={(e) => setMemberEmail(e.target.value)}
                    placeholder="user@example.com"
                  />
                  <Button type="submit" disabled={actionBusy === 'member' || !memberEmail.trim()}>
                    {actionBusy === 'member' ? '…' : 'Add member'}
                  </Button>
                </form>

                <div style={{ marginTop: 16 }}>
                  <h4>SSO and 2FA</h4>
                  <p className="muted">
                    Providers: {ssoProviders.length
                      ? ssoProviders.map((p) => `${p.name} (${p.kind}${p.enabled ? '' : ', off'})`).join(', ')
                      : 'none'}
                  </p>
                  <div className="compass-create-form" style={{ marginTop: 8 }}>
                    <FormField
                      id="oidc-name"
                      label="OIDC name"
                      value={oidcForm.name}
                      onChange={(e) => setOidcForm((prev) => ({ ...prev, name: e.target.value }))}
                    />
                    <FormField
                      id="oidc-issuer"
                      label="Issuer URL"
                      value={oidcForm.issuerUrl}
                      onChange={(e) => setOidcForm((prev) => ({ ...prev, issuerUrl: e.target.value }))}
                      placeholder="http://127.0.0.1:5556/dex"
                    />
                    <FormField
                      id="oidc-client-id"
                      label="Client ID"
                      value={oidcForm.clientId}
                      onChange={(e) => setOidcForm((prev) => ({ ...prev, clientId: e.target.value }))}
                    />
                    <FormField
                      id="oidc-client-secret"
                      label="Client secret"
                      type="password"
                      value={oidcForm.clientSecret}
                      onChange={(e) => setOidcForm((prev) => ({ ...prev, clientSecret: e.target.value }))}
                      placeholder="leave blank to keep existing"
                    />
                  </div>
                  <div className="page-actions" style={{ marginTop: 8, flexWrap: 'wrap' }}>
                    <Button
                      variant="secondary"
                      type="button"
                      disabled={actionBusy === 'sso' || !oidcForm.issuerUrl.trim() || !oidcForm.clientId.trim()}
                      onClick={() => void onUpsertOIDC()}
                    >
                      {actionBusy === 'sso' ? '…' : 'Save OIDC provider'}
                    </Button>
                    <Button
                      variant="secondary"
                      type="button"
                      disabled={actionBusy === 'totp'}
                      onClick={() => void onEnableTOTP()}
                    >
                      <KeyRound size={14} /> {actionBusy === 'totp' ? '…' : 'Enable TOTP'}
                    </Button>
                  </div>
                  {totpSecret ? (
                    <p className="mono" style={{ marginTop: 8, wordBreak: 'break-all' }}>{totpSecret}</p>
                  ) : null}
                  <div className="compass-create-form" style={{ marginTop: 8 }}>
                    <FormField
                      id="totp-code"
                      label="TOTP code"
                      value={totpCode}
                      onChange={(e) => setTotpCode(e.target.value)}
                      placeholder="123456"
                    />
                    <Button
                      type="button"
                      disabled={actionBusy === 'verify' || !totpCode.trim()}
                      onClick={() => void onVerifyTOTP()}
                    >
                      {actionBusy === 'verify' ? '…' : 'Verify'}
                    </Button>
                  </div>
                </div>
              </>
            )}
          </aside>
        </div>
      </div>

      <CreateModal
        open={modalOpen}
        title="New team"
        submitLabel="Create team"
        busy={busy}
        onClose={() => setModalOpen(false)}
        onSubmit={(event) => void onCreateTeam(event)}
      >
        <FormField id="team-name" name="name" label="Name" required placeholder="Engineering" />
      </CreateModal>
    </section>
  )
}
