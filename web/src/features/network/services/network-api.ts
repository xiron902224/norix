import { create } from '@bufbuild/protobuf'
import { agentClient } from '../../../api/clients'
import {
  GetNetworkStatusRequestSchema,
  ListEgressPoliciesRequestSchema,
  UpsertEgressPolicyRequestSchema,
  type EgressPolicy,
  type GetNetworkStatusResponse,
} from '../../../gen/v1/agent_pb'

export async function getNetworkStatus(): Promise<GetNetworkStatusResponse> {
  return agentClient.getNetworkStatus(create(GetNetworkStatusRequestSchema, {}))
}

export async function listEgressPolicies(): Promise<EgressPolicy[]> {
  const response = await agentClient.listEgressPolicies(create(ListEgressPoliciesRequestSchema, {}))
  return response.policies
}

export async function upsertEgressPolicy(input: {
  workloadId: string
  mode: string
  allowlist?: string[]
}): Promise<{ workloadId: string; mode: string; allowlist: string[] }> {
  const response = await agentClient.upsertEgressPolicy(create(UpsertEgressPolicyRequestSchema, {
    workloadId: input.workloadId,
    mode: input.mode,
    allowlist: input.allowlist ?? [],
  }))
  return {
    workloadId: response.workloadId,
    mode: response.mode,
    allowlist: response.allowlist,
  }
}
