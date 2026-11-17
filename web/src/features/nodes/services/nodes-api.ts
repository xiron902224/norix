import { create } from '@bufbuild/protobuf'
import { agentClient } from '../../../api/clients'
import {
  CordonNodeRequestSchema,
  CreateJoinTokenRequestSchema,
  DrainNodeRequestSchema,
  ListNodesRequestSchema,
  RevokeNodeTokenRequestSchema,
  RotateNodeTokenRequestSchema,
  SetNodeClientCertRequestSchema,
  UpdateNodeAgentRequestSchema,
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

export async function drainNode(nodeId: string, draining: boolean): Promise<{ node: Node; warnings: string[] }> {
  const response = await agentClient.drainNode(create(DrainNodeRequestSchema, { nodeId, draining }))
  if (!response.node) throw new Error('Node was not returned')
  return { node: response.node, warnings: response.warnings ?? [] }
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

export async function updateNodeAgent(input: {
  nodeId: string
  version?: string
}): Promise<Node> {
  const response = await agentClient.updateNodeAgent(create(UpdateNodeAgentRequestSchema, {
    nodeId: input.nodeId,
    version: input.version ?? '',
  }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}

export async function rotateNodeToken(nodeId: string): Promise<{ node: Node; message: string }> {
  const response = await agentClient.rotateNodeToken(create(RotateNodeTokenRequestSchema, { nodeId }))
  if (!response.node) throw new Error('Node was not returned')
  return { node: response.node, message: response.message || 'Rotation queued' }
}

export async function revokeNodeToken(nodeId: string): Promise<Node> {
  const response = await agentClient.revokeNodeToken(create(RevokeNodeTokenRequestSchema, { nodeId }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}

export async function setNodeClientCert(nodeId: string, certificateSha256: string): Promise<Node> {
  const response = await agentClient.setNodeClientCert(create(SetNodeClientCertRequestSchema, {
    nodeId,
    certificateSha256,
  }))
  if (!response.node) throw new Error('Node was not returned')
  return response.node
}
