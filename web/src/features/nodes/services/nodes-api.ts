import { create } from '@bufbuild/protobuf'
import { agentClient } from '../../../api/clients'
import {
  CreateJoinTokenRequestSchema,
  ListNodesRequestSchema,
  type Node,
} from '../../../gen/v1/agent_pb'

export async function listNodes(): Promise<Node[]> {
  const response = await agentClient.listNodes(create(ListNodesRequestSchema, {}))
  return response.nodes
}

export async function createJoinToken(input: {
  nodeName?: string
  ttlSeconds?: bigint
}): Promise<{ token: string; expiresAt: string }> {
  const response = await agentClient.createJoinToken(create(CreateJoinTokenRequestSchema, {
    nodeName: input.nodeName ?? '',
    ttlSeconds: input.ttlSeconds ?? 3600n,
  }))
  return {
    token: response.token,
    expiresAt: response.expiresAt,
  }
}
