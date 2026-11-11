import { create } from '@bufbuild/protobuf'
import { useEffect, useState } from 'react'
import { PanelsTopLeft } from 'lucide-react'
import './App.css'
import { authClient } from './api/clients'
import { GetBootstrapStatusRequestSchema } from './gen/v1/auth_pb'
import { BootstrapForm } from './features/auth/components/BootstrapForm'
import { LoginForm } from './features/auth/components/LoginForm'
import { sessionStore, type SessionUser } from './features/auth/services/session'
import { logoutSession } from './features/auth/services/auth-api'
import { DashboardShell } from './features/dashboard/DashboardShell'

function applyStoredTheme() {
  const stored = window.localStorage.getItem('norix-theme')
  const dark = stored
    ? stored === 'dark'
    : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.classList.toggle('dark', dark)
}

function App() {
  const [configured, setConfigured] = useState<boolean | null>(null)
  const [error, setError] = useState('')
  const [user, setUser] = useState<SessionUser | null>(() => sessionStore.getUser())

  useEffect(() => {
    applyStoredTheme()
    void authClient.getBootstrapStatus(create(GetBootstrapStatusRequestSchema, {}))
      .then((response) => setConfigured(response.configured))
      .catch((cause) => setError(cause instanceof Error ? cause.message : 'Could not reach Norix'))
  }, [])

  async function signOut() {
    const token = sessionStore.getToken()
    if (token) {
      try { await logoutSession(token) } catch { /* clear local state even if backend is unavailable */ }
    }
    sessionStore.clear()
    setUser(null)
  }

  if (user) {
    return <DashboardShell user={user} onSignOut={() => void signOut()} />
  }

  if (configured === null) {
    return (
      <main className="norix-shell">
        <section className="auth-card">
          <div className="loading-row"><span className="spinner" aria-hidden /><span>Checking Norix installation…</span></div>
        </section>
      </main>
    )
  }

  return (
    <main className="norix-shell">
      <section className="auth-card">
        <div className="auth-brand">
          <div className="brand-mark" aria-hidden>
            <PanelsTopLeft />
          </div>
          <span className="eyebrow">Norix control plane</span>
        </div>
        <h1>{configured ? 'Welcome back' : 'Create your administrator'}</h1>
        <p>
          {configured
            ? 'Sign in to manage projects, workloads, and your platform.'
            : 'This one-time setup creates the first organization and owner account.'}
        </p>
        {error && <div className="error" role="alert">{error}</div>}
        {configured
          ? <LoginForm onAuthenticated={setUser} />
          : <BootstrapForm onConfigured={(createdUser) => {
              setConfigured(true)
              setUser(createdUser)
            }} />}
      </section>
    </main>
  )
}

export default App
