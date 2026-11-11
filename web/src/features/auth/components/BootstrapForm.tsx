import { create } from '@bufbuild/protobuf'
import { useState, type FormEvent } from 'react'
import { authClient } from '../../../auth-client'
import { CreateInitialAdminRequestSchema } from '../../../gen/v1/auth_pb'
import { sessionStore, type SessionUser } from '../services/session'
import { Button } from '../../../shared/ui/Button'
import { FormField } from '../../../shared/ui/FormField'

interface BootstrapFormProps {
  onConfigured: (user: SessionUser | null) => void
}

export function BootstrapForm({ onConfigured }: BootstrapFormProps) {
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    const form = new FormData(event.currentTarget)
    const password = String(form.get('password') ?? '')
    const confirmation = String(form.get('confirmation') ?? '')

    if (password !== confirmation) {
      setError('Passwords do not match')
      return
    }
    if (password.length < 12) {
      setError('Password must contain at least 12 characters')
      return
    }

    setLoading(true)
    try {
      const response = await authClient.createInitialAdmin(create(CreateInitialAdminRequestSchema, {
        organizationName: String(form.get('organization') ?? ''),
        adminName: String(form.get('name') ?? ''),
        adminEmail: String(form.get('email') ?? ''),
        adminPassword: password,
      }))
      const user: SessionUser | null = response.user ? {
        id: response.user.id,
        email: response.user.email,
        name: response.user.name,
        role: response.user.role,
      } : null
      if (response.token && user) sessionStore.save(response.token, user)
      onConfigured(user)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Administrator setup failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={submit}>
      {error && <div className="error" role="alert">{error}</div>}
      <FormField id="setup-organization" name="organization" label="Organization" required autoComplete="organization" />
      <FormField id="setup-name" name="name" label="Administrator name" required autoComplete="name" />
      <FormField id="setup-email" name="email" label="Email" type="email" required autoComplete="email" />
      <FormField id="setup-password" name="password" label="Password" type="password" required minLength={12} autoComplete="new-password" />
      <FormField id="setup-confirmation" name="confirmation" label="Confirm password" type="password" required minLength={12} autoComplete="new-password" />
      <Button type="submit" disabled={loading}>{loading ? 'Creating administrator…' : 'Create administrator'}</Button>
    </form>
  )
}
