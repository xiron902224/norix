import { create } from '@bufbuild/protobuf'
import { teamClient } from '../../../api/clients'
import {
  AddTeamMemberRequestSchema,
  CreateTeamRequestSchema,
  GetTeamUsageRequestSchema,
  ListSSOProvidersRequestSchema,
  ListTeamsRequestSchema,
  SetTeamQuotaRequestSchema,
  TeamQuotaSchema,
  UpsertSSOProviderRequestSchema,
  EnableTOTPRequestSchema,
  VerifyTOTPRequestSchema,
  SSOProviderSchema,
} from '../../../gen/v1/team_pb'
import { sessionStore } from '../../auth/services/session'

export type TeamItem = {
  id: string
  name: string
  slug: string
  isDefault: boolean
}

export type TeamQuotaItem = {
  teamId: string
  maxCpu: number
  maxMemoryMb: bigint
  maxDiskGb: bigint
  maxSandboxes: number
  maxPreviews: number
}

export type TeamUsageItem = {
  teamId: string
  cpuUsed: number
  memoryMbUsed: bigint
  diskGbUsed: bigint
  sandboxesUsed: number
  previewsUsed: number
}

export async function listTeams(): Promise<TeamItem[]> {
  const response = await teamClient.listTeams(create(ListTeamsRequestSchema, {}))
  return response.teams.map((team) => ({
    id: team.id,
    name: team.name,
    slug: team.slug,
    isDefault: team.isDefault,
  }))
}

export async function createTeam(name: string, slug: string) {
  const response = await teamClient.createTeam(create(CreateTeamRequestSchema, { name, slug }))
  return response.team
}

export async function addTeamMember(teamId: string, userEmail: string, role: string) {
  const response = await teamClient.addTeamMember(create(AddTeamMemberRequestSchema, { teamId, userEmail, role }))
  return response.member
}

export async function setTeamQuota(quota: TeamQuotaItem) {
  const response = await teamClient.setTeamQuota(create(SetTeamQuotaRequestSchema, {
    quota: create(TeamQuotaSchema, {
      teamId: quota.teamId,
      maxCpu: quota.maxCpu,
      maxMemoryMb: quota.maxMemoryMb,
      maxDiskGb: quota.maxDiskGb,
      maxSandboxes: quota.maxSandboxes,
      maxPreviews: quota.maxPreviews,
    }),
  }))
  return response.quota
}

export async function getTeamUsage(teamId: string): Promise<{ usage: TeamUsageItem; quota: TeamQuotaItem }> {
  const response = await teamClient.getTeamUsage(create(GetTeamUsageRequestSchema, { teamId }))
  const usage = response.usage!
  const quota = response.quota!
  return {
    usage: {
      teamId: usage.teamId,
      cpuUsed: usage.cpuUsed,
      memoryMbUsed: usage.memoryMbUsed,
      diskGbUsed: usage.diskGbUsed,
      sandboxesUsed: usage.sandboxesUsed,
      previewsUsed: usage.previewsUsed,
    },
    quota: {
      teamId: quota.teamId,
      maxCpu: quota.maxCpu,
      maxMemoryMb: quota.maxMemoryMb,
      maxDiskGb: quota.maxDiskGb,
      maxSandboxes: quota.maxSandboxes,
      maxPreviews: quota.maxPreviews,
    },
  }
}

export async function listSSOProviders() {
  const response = await teamClient.listSSOProviders(create(ListSSOProvidersRequestSchema, {}))
  return response.providers
}

export async function upsertSSOProvider(input: { id?: string; name: string; kind: string; issuerUrl: string; clientId: string; enabled: boolean; clientSecret?: string }) {
  const response = await teamClient.upsertSSOProvider(create(UpsertSSOProviderRequestSchema, {
    provider: create(SSOProviderSchema, {
      id: input.id ?? '',
      name: input.name,
      kind: input.kind,
      issuerUrl: input.issuerUrl,
      clientId: input.clientId,
      enabled: input.enabled,
    }),
    clientSecret: input.clientSecret ?? '',
  }))
  return response.provider
}

export async function enableTOTP() {
  return teamClient.enableTOTP(create(EnableTOTPRequestSchema, {}))
}

export async function verifyTOTP(code: string) {
  return teamClient.verifyTOTP(create(VerifyTOTPRequestSchema, { code }))
}

export function auditExportUrl(format: 'json' | 'csv' = 'json') {
  const base = import.meta.env.VITE_NORIX_API_URL ?? ''
  const token = sessionStore.getToken()
  const path = `/api/ops/audit/export?format=${format}`
  return { url: `${base}${path}`, token }
}

export function usageExportUrl(format: 'json' | 'csv' = 'json') {
  const base = import.meta.env.VITE_NORIX_API_URL ?? ''
  const path = `/api/ops/usage/export?format=${format}`
  return { url: `${base}${path}`, token: sessionStore.getToken() }
}
