import { useState, type FormEvent } from 'react'
import { create } from '@bufbuild/protobuf'
import { authClient } from '../../../auth-client'
import { LoginRequestSchema } from '../../../gen/v1/auth_pb'
import { sessionStore, type SessionUser } from '../services/session'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'

interface LoginFormProps {
  onAuthenticated: (user: SessionUser) => void
}

export function LoginForm({ onAuthenticated }: LoginFormProps) {
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setLoading(true)
    const form = new FormData(event.currentTarget)

    try {
      const response = await authClient.login(create(LoginRequestSchema, {
        email: String(form.get('email') ?? ''),
        password: String(form.get('password') ?? ''),
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
      setError(cause instanceof Error ? cause.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={submit}>
      {error && <div className="error" role="alert">{error}</div>}
      <FormField id="login-email" name="email" label="Email" type="email" required autoComplete="email" />
      <FormField id="login-password" name="password" label="Password" type="password" required autoComplete="current-password" />
      <Button type="submit" disabled={loading}>{loading ? 'Signing in…' : 'Sign in'}</Button>
    </form>
  )
}
