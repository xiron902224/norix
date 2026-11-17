import { create } from '@bufbuild/protobuf'
import { authClient } from '../../../api/clients'
import {
  CreatePersonalAccessTokenRequestSchema,
  ListPersonalAccessTokensRequestSchema,
  LogoutRequestSchema,
  RenewSessionRequestSchema,
  RevokePersonalAccessTokenRequestSchema,
  type PersonalAccessToken,
} from '../../../gen/v1/auth_pb'

export async function logoutSession(token: string): Promise<void> {
  await authClient.logout(create(LogoutRequestSchema, { token }))
}

export async function renewSession(token: string): Promise<number> {
  const response = await authClient.renewSession(create(RenewSessionRequestSchema, { token }))
  return Number(response.expiresAtUnix)
}

export async function listPersonalAccessTokens(): Promise<PersonalAccessToken[]> {
  const response = await authClient.listPersonalAccessTokens(create(ListPersonalAccessTokensRequestSchema, {}))
  return response.tokens
}

export async function createPersonalAccessToken(name: string, scopes: string[] = ['*']): Promise<{
  token?: PersonalAccessToken
  plaintext: string
}> {
  const response = await authClient.createPersonalAccessToken(create(CreatePersonalAccessTokenRequestSchema, {
    name,
    scopes,
  }))
  return { token: response.token, plaintext: response.plaintext }
}

export async function revokePersonalAccessToken(id: string): Promise<void> {
  await authClient.revokePersonalAccessToken(create(RevokePersonalAccessTokenRequestSchema, { id }))
}
