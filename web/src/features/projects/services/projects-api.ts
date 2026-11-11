import { create } from '@bufbuild/protobuf'
import { projectClient, workloadClient } from '../../../api/clients'
import {
  CreateProjectRequestSchema,
  CreateWorkloadRequestSchema,
  DeleteProjectRequestSchema,
  DeleteWorkloadRequestSchema,
  DeployRequestSchema,
  GetWorkloadLogsRequestSchema,
  ListProjectsRequestSchema,
  ListWorkloadsRequestSchema,
  RuntimeType,
  WorkloadSchema,
  type Project,
  type Workload,
} from '../../../gen/v1/workload_pb'

export async function listProjects(): Promise<Project[]> {
  const response = await projectClient.listProjects(create(ListProjectsRequestSchema, {}))
  return response.projects
}

export async function createProject(name: string): Promise<Project> {
  const response = await projectClient.createProject(create(CreateProjectRequestSchema, { name }))
  if (!response.project) throw new Error('Project was not returned')
  return response.project
}

export async function deleteProject(id: string): Promise<void> {
  await projectClient.deleteProject(create(DeleteProjectRequestSchema, { id }))
}

export async function listWorkloads(projectId: string): Promise<Workload[]> {
  const response = await workloadClient.listWorkloads(create(ListWorkloadsRequestSchema, { projectId }))
  return response.workloads
}

export async function createWorkload(input: {
  projectId: string
  name: string
  imageOrSource: string
  containerPort?: number
}): Promise<Workload> {
  const response = await workloadClient.createWorkload(create(CreateWorkloadRequestSchema, {
    workload: create(WorkloadSchema, {
      projectId: input.projectId,
      name: input.name,
      imageOrSource: input.imageOrSource,
      runtime: RuntimeType.CONTAINER,
      cpuShares: 1024,
      memoryLimitMb: BigInt(512),
      containerPort: input.containerPort ?? 80,
    }),
  }))
  if (!response.workload) throw new Error('Workload was not returned')
  return response.workload
}

export async function deleteWorkload(id: string): Promise<void> {
  await workloadClient.deleteWorkload(create(DeleteWorkloadRequestSchema, { id }))
}

export async function deployWorkload(id: string, containerPort?: number) {
  const response = await workloadClient.deploy(create(DeployRequestSchema, {
    workload: create(WorkloadSchema, { id }),
    containerPort: containerPort ?? 0,
  }))
  return response
}

export async function getWorkloadLogs(workloadId: string, tailLines = 100): Promise<string[]> {
  const response = await workloadClient.getWorkloadLogs(create(GetWorkloadLogsRequestSchema, {
    workloadId,
    tailLines,
  }))
  return response.lines
}
