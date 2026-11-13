import { create } from '@bufbuild/protobuf'
import { sandboxClient } from '../../../api/clients'
import {
  CreateSandboxRequestSchema,
  DestroySandboxRequestSchema,
  ListSandboxesRequestSchema,
  type Sandbox,
} from '../../../gen/v1/sandbox_pb'

export async function listSandboxes(projectId = ''): Promise<Sandbox[]> {
  const response = await sandboxClient.listSandboxes(
    create(ListSandboxesRequestSchema, { projectId }),
  )
  return response.sandboxes
}

export async function createSandbox(input: {
  projectId: string
  name: string
  template?: string
  ttlSeconds?: number
  cpu?: number
  memoryMb?: number
  egressMode?: string
}): Promise<Sandbox> {
  const response = await sandboxClient.createSandbox(
    create(CreateSandboxRequestSchema, {
      projectId: input.projectId,
      name: input.name,
      template: input.template ?? 'default',
      ttlSeconds: input.ttlSeconds ?? 3600,
      cpu: input.cpu ?? 1,
      memoryMb: input.memoryMb ?? 512,
      egressMode: input.egressMode ?? 'restricted',
    }),
  )
  if (!response.sandbox) throw new Error('Sandbox was not returned')
  return response.sandbox
}

export async function destroySandbox(id: string): Promise<void> {
  await sandboxClient.destroySandbox(create(DestroySandboxRequestSchema, { id }))
}
