import { useEffect, useState } from 'react'
import { Download, KeyRound, Shield, Users } from 'lucide-react'
import { Button } from '../../../shared/ui/Button'
import { ActivityBanner } from '../../../shared/ui/ActivityBanner'
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
  const [newTeamName, setNewTeamName] = useState('')
  const [memberEmail, setMemberEmail] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [quotaForm, setQuotaForm] = useState({ maxCpu: '4', maxMemoryMb: '8192', maxSandboxes: '5', maxPreviews: '10', maxDiskGb: '50' })

  async function refresh(teamId = selectedId) {
    try {
      const [teamList, providers] = await Promise.all([listTeams(), listSSOProviders()])
      setTeams(teamList)
      setSsoProviders(providers.map((p) => ({ id: p.id, name: p.name, kind: p.kind, enabled: p.enabled })))
      const active = teamId || teamList[0]?.id || ''
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

  async function onCreateTeam() {
    if (!newTeamName.trim()) return
    setNotice('')
    try {
      await createTeam(newTeamName.trim(), '')
      setNewTeamName('')
      setNotice('Team created')
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create team failed')
    }
  }

  async function onSaveQuota() {
    if (!selectedId) return
  setNotice('')
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
      setError(cause instanceof Error ? cause.message : 'Save quota failed')
    }
  }

  async function onAddMember() {
    if (!selectedId || !memberEmail.trim()) return
    setNotice('')
    try {
      await addTeamMember(selectedId, memberEmail.trim(), 'member')
      setMemberEmail('')
      setNotice('Member added')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Add member failed')
    }
  }

  async function onUpsertOIDC() {
    setNotice('')
    try {
      await upsertSSOProvider({
        name: 'Local OIDC',
        kind: 'oidc',
        issuerUrl: 'http://127.0.0.1:4180/',
        clientId: 'norix-local',
        enabled: true,
      })
      setNotice('OIDC provider saved (local stub)')
      await refresh(selectedId)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'SSO update failed')
    }
  }

  async function onEnableTOTP() {
    setNotice('')
    try {
      const result = await enableTOTP()
      setTotpSecret(result.secret)
      setNotice('Scan or copy the TOTP secret, then verify with a 6-digit code')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Enable TOTP failed')
    }
  }

  async function onVerifyTOTP() {
    setNotice('')
    try {
      const result = await verifyTOTP(totpCode)
      setNotice(result.verified ? '2FA enabled' : 'Verification failed')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Verify TOTP failed')
    }
  }

  async function downloadExport(kind: 'audit' | 'usage', format: 'json' | 'csv') {
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
  }

  const selected = teams.find((team) => team.id === selectedId)

  return (
    <div className="page-stack">
      <header className="page-header">
        <div>
          <h1>Teams & quotas</h1>
          <p>Organization teams, quota limits, SSO, and usage export for billing.</p>
        </div>
        <div className="header-actions">
          <Button variant="secondary" type="button" onClick={() => void downloadExport('audit', 'json')}>
            <Download size={14} /> Audit JSON
          </Button>
          <Button variant="secondary" type="button" onClick={() => void downloadExport('usage', 'csv')}>
            <Download size={14} /> Usage CSV
          </Button>
        </div>
      </header>

      {error ? <div className="error" role="alert">{error}</div> : null}
      {notice ? <ActivityBanner title={notice} tone="ok" /> : null}

      {loading ? (
        <p className="muted">Loading teams…</p>
      ) : (
        <div className="split-panels">
          <section className="panel">
            <h2><Users size={16} /> Teams</h2>
            <ul className="simple-list">
              {teams.map((team) => (
                <li key={team.id}>
                  <button
                    type="button"
                    className={team.id === selectedId ? 'list-select active' : 'list-select'}
                    onClick={() => { setSelectedId(team.id); void refresh(team.id) }}
                  >
                    <strong>{team.name}</strong>
                    <span className="mono">{team.slug}{team.isDefault ? ' · default' : ''}</span>
                  </button>
                </li>
              ))}
            </ul>
            <div className="form-row" style={{ marginTop: 12 }}>
              <input value={newTeamName} onChange={(e) => setNewTeamName(e.target.value)} placeholder="New team name" />
              <Button type="button" onClick={() => void onCreateTeam()}>Add team</Button>
            </div>
          </section>

          <section className="panel">
            <h2>Quota · {selected?.name ?? '—'}</h2>
            {usage ? (
              <dl className="kv-grid">
                <div><dt>CPU used</dt><dd>{usage.cpuUsed}</dd></div>
                <div><dt>Memory MB</dt><dd>{String(usage.memoryMbUsed)}</dd></div>
                <div><dt>Sandboxes</dt><dd>{usage.sandboxesUsed}</dd></div>
                <div><dt>Previews</dt><dd>{usage.previewsUsed}</dd></div>
                <div><dt>Disk GB</dt><dd>{String(usage.diskGbUsed)}</dd></div>
              </dl>
            ) : null}
            <div className="form-grid" style={{ marginTop: 12 }}>
              <label>Max vCPU<input value={quotaForm.maxCpu} onChange={(e) => setQuotaForm((v) => ({ ...v, maxCpu: e.target.value }))} /></label>
              <label>Max RAM MB<input value={quotaForm.maxMemoryMb} onChange={(e) => setQuotaForm((v) => ({ ...v, maxMemoryMb: e.target.value }))} /></label>
              <label>Max sandboxes<input value={quotaForm.maxSandboxes} onChange={(e) => setQuotaForm((v) => ({ ...v, maxSandboxes: e.target.value }))} /></label>
              <label>Max previews<input value={quotaForm.maxPreviews} onChange={(e) => setQuotaForm((v) => ({ ...v, maxPreviews: e.target.value }))} /></label>
              <label>Max disk GB<input value={quotaForm.maxDiskGb} onChange={(e) => setQuotaForm((v) => ({ ...v, maxDiskGb: e.target.value }))} /></label>
            </div>
            <Button type="button" onClick={() => void onSaveQuota()} style={{ marginTop: 12 }}>Save quota</Button>
            <p className="muted" style={{ marginTop: 8 }}>0 = unlimited. Exceeding quota returns FailedPrecondition without affecting other teams.</p>
          </section>

          <section className="panel">
            <h2><Shield size={16} /> SSO &amp; 2FA</h2>
            <p className="muted">Providers: {ssoProviders.length ? ssoProviders.map((p) => `${p.name} (${p.kind})`).join(', ') : 'none'}</p>
            <Button variant="secondary" type="button" onClick={() => void onUpsertOIDC()}>Save local OIDC stub</Button>
            <TotpBlock
              totpSecret={totpSecret}
              totpCode={totpCode}
              setTotpCode={setTotpCode}
              onEnable={() => void onEnableTOTP()}
              onVerify={() => void onVerifyTOTP()}
            />
          </section>

          <section className="panel">
            <h2>Members</h2>
            <div className="form-row">
              <input value={memberEmail} onChange={(e) => setMemberEmail(e.target.value)} placeholder="user@example.com" />
              <Button type="button" onClick={() => void onAddMember()}>Add</Button>
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

function TotpBlock({
  totpSecret,
  totpCode,
  setTotpCode,
  onEnable,
  onVerify,
}: {
  totpSecret: string
  totpCode: string
  setTotpCode: (value: string) => void
  onEnable: () => void
  onVerify: () => void
}) {
  return (
    <div style={{ marginTop: 12 }}>
      <Button variant="secondary" type="button" onClick={onEnable}><KeyRound size={14} /> Enable TOTP</Button>
      {totpSecret ? <p className="mono" style={{ marginTop: 8, wordBreak: 'break-all' }}>{totpSecret}</p> : null}
      <div className="form-row" style={{ marginTop: 8 }}>
        <input value={totpCode} onChange={(e) => setTotpCode(e.target.value)} placeholder="123456" />
        <Button type="button" onClick={onVerify}>Verify</Button>
      </div>
    </div>
  )
}
