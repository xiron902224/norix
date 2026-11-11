import { create } from '@bufbuild/protobuf'
import { agentClient } from '../../../api/clients'
import {
  CordonNodeRequestSchema,
  CreateJoinTokenRequestSchema,
  DrainNodeRequestSchema,
  ListNodesRequestSchema,
  UpdateNodeScheduleRequestSchema,
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

export async function cordonNode(nodeId: string, cordoned: boolean): Promise<Node> {
  const response = await agentClient.cordonNode(create(CordonNodeRequestSchema, { nodeId, cordoned }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}

export async function drainNode(nodeId: string, draining: boolean): Promise<Node> {
  const response = await agentClient.drainNode(create(DrainNodeRequestSchema, { nodeId, draining }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}

export async function updateNodeSchedule(input: {
  nodeId: string
  role?: string
  zone?: string
  labels?: Record<string, string>
}): Promise<Node> {
  const response = await agentClient.updateNodeSchedule(create(UpdateNodeScheduleRequestSchema, {
    nodeId: input.nodeId,
    role: input.role ?? '',
    zone: input.zone ?? '',
    labels: input.labels ?? {},
  }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}
