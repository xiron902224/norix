import { useEffect, useState, type FormEvent } from 'react'
import { create } from '@bufbuild/protobuf'
import { ConnectError, Code } from '@connectrpc/connect'
import { authClient } from '../../../auth-client'
import { GetCurrentUserRequestSchema, LoginRequestSchema } from '../../../gen/v1/auth_pb'
import { sessionStore, type SessionUser } from '../services/session'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'

interface LoginFormProps {
  onAuthenticated: (user: SessionUser) => void
}

interface PublicOIDCProvider {
  id: string
  name: string
}

function apiOrigin() {
  const configured = import.meta.env.VITE_NORIX_API_URL as string | undefined
  if (configured && /^https?:\/\//.test(configured)) {
    return configured.replace(/\/api\/?$/, '')
  }
  return ''
}

export function LoginForm({ onAuthenticated }: LoginFormProps) {
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [needsTotp, setNeedsTotp] = useState(false)
  const [providers, setProviders] = useState<PublicOIDCProvider[]>([])
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const ssoToken = params.get('sso_token')
    if (ssoToken) {
      void completeSSO(ssoToken)
      return
    }
    const base = apiOrigin()
    void fetch(`${base}/auth/oidc/providers`)
      .then(async (response) => {
        if (!response.ok) return []
        return response.json() as Promise<PublicOIDCProvider[]>
      })
      .then((items) => setProviders(Array.isArray(items) ? items : []))
      .catch(() => setProviders([]))
  }, [])

  async function completeSSO(token: string) {
    setLoading(true)
    setError('')
    try {
      sessionStore.save(token, { id: '', email: '', name: '', role: 0 })
      const me = await authClient.getCurrentUser(create(GetCurrentUserRequestSchema, {}))
      const user: SessionUser = {
        id: me.user?.id ?? '',
        email: me.user?.email ?? '',
        name: me.user?.name ?? '',
        role: me.user?.role ?? 0,
      }
      sessionStore.save(token, user)
      window.history.replaceState({}, '', '/login')
      onAuthenticated(user)
    } catch (cause) {
      sessionStore.clear()
      setError(cause instanceof Error ? cause.message : 'SSO sign-in failed')
    } finally {
      setLoading(false)
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setLoading(true)

    try {
      const response = await authClient.login(create(LoginRequestSchema, {
        email,
        password,
        totpCode: needsTotp ? totpCode : '',
      }))
      const user: SessionUser = {
        id: response.user?.id ?? '',
        email: response.user?.email ?? '',
        name: response.user?.name ?? '',
        role: response.user?.role ?? 0,
      }
      sessionStore.save(response.token, user)
      onAuthenticated(user)
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Could not sign in'
      const isTotpRequired =
        message.includes('TOTP_REQUIRED') ||
        (cause instanceof ConnectError && cause.code === Code.FailedPrecondition && message.includes('TOTP'))
      if (isTotpRequired) {
        setNeedsTotp(true)
        setError('Enter the 6-digit code from your authenticator app')
      } else {
        setError(message)
      }
    } finally {
      setLoading(false)
    }
  }

  function startSSO(providerId: string) {
    const base = apiOrigin()
    window.location.href = `${base}/auth/oidc/start?provider_id=${encodeURIComponent(providerId)}`
  }

  return (
    <form onSubmit={submit}>
      {error && <div className="error" role="alert">{error}</div>}
      <FormField
        id="login-email"
        name="email"
        label="Email"
        type="email"
        required
        autoComplete="email"
        value={email}
        onChange={(event) => setEmail(event.target.value)}
      />
      <FormField
        id="login-password"
        name="password"
        label="Password"
        type="password"
        required
        autoComplete="current-password"
        value={password}
        onChange={(event) => setPassword(event.target.value)}
      />
      {needsTotp ? (
        <FormField
          id="login-totp"
          name="totp"
          label="Authenticator code"
          inputMode="numeric"
          autoComplete="one-time-code"
          required
          value={totpCode}
          onChange={(event) => setTotpCode(event.target.value)}
          placeholder="123456"
        />
      ) : null}
      <Button type="submit" disabled={loading}>{loading ? 'Signing in…' : 'Sign in'}</Button>
      {providers.length > 0 ? (
        <div style={{ marginTop: 16, display: 'grid', gap: 8 }}>
          <p className="muted" style={{ margin: 0 }}>Or continue with SSO</p>
          {providers.map((provider) => (
            <Button
              key={provider.id}
              type="button"
              variant="secondary"
              disabled={loading}
              onClick={() => startSSO(provider.id)}
            >
              Sign in with {provider.name}
            </Button>
          ))}
        </div>
      ) : null}
    </form>
  )
}
