import { create } from '@bufbuild/protobuf'
import { authClient } from '../../../api/clients'
import { LogoutRequestSchema, RenewSessionRequestSchema } from '../../../gen/v1/auth_pb'

export async function logoutSession(token: string): Promise<void> {
  await authClient.logout(create(LogoutRequestSchema, { token }))
}

export async function renewSession(token: string): Promise<number> {
  const response = await authClient.renewSession(create(RenewSessionRequestSchema, { token }))
  return Number(response.expiresAtUnix)
}
