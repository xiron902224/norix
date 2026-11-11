import { create } from '@bufbuild/protobuf'
import { workloadClient } from '../../../api/clients'
import {
  AssignDomainRequestSchema,
  ListDomainsRequestSchema,
  UnassignDomainRequestSchema,
  type DomainRoute,
} from '../../../gen/v1/workload_pb'

export async function listDomains(projectId = ''): Promise<DomainRoute[]> {
  const response = await workloadClient.listDomains(create(ListDomainsRequestSchema, { projectId }))
  return response.routes
}

export async function assignDomain(input: {
  workloadId: string
  hostname: string
  tlsMode?: string
}): Promise<DomainRoute> {
  const response = await workloadClient.assignDomain(create(AssignDomainRequestSchema, {
    workloadId: input.workloadId,
    hostname: input.hostname,
    tlsMode: input.tlsMode ?? '',
  }))
  if (!response.route) throw new Error('Domain route was not returned')
  return response.route
}

export async function unassignDomain(id: string): Promise<void> {
  await workloadClient.unassignDomain(create(UnassignDomainRequestSchema, { id }))
}
