import { create } from '@bufbuild/protobuf'
import { projectClient, workloadClient } from '../../../api/clients'
import {
  CreateProjectRequestSchema,
  CreateWorkloadRequestSchema,
  DeleteProjectRequestSchema,
  DeleteWorkloadRequestSchema,
  DeployFromGitRequestSchema,
  DeployRequestSchema,
  GetBuildLogsRequestSchema,
  GetWorkloadLogsRequestSchema,
  ListBuildsRequestSchema,
  ListProjectsRequestSchema,
  ListWorkloadsRequestSchema,
  RuntimeType,
  UpdateWorkloadRequestSchema,
  WorkloadSchema,
  type Build,
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
  imageOrSource?: string
  containerPort?: number
  sourceKind?: 'image' | 'git'
  gitUrl?: string
  gitRef?: string
  dockerfilePath?: string
  replicaCount?: number
  scheduleStrategy?: string
  requireKvm?: boolean
  preferredZone?: string
}): Promise<Workload> {
  const sourceKind = input.sourceKind ?? (input.gitUrl ? 'git' : 'image')
  const response = await workloadClient.createWorkload(create(CreateWorkloadRequestSchema, {
    workload: create(WorkloadSchema, {
      projectId: input.projectId,
      name: input.name,
      imageOrSource: input.imageOrSource || (sourceKind === 'git' ? 'pending-build' : ''),
      runtime: RuntimeType.CONTAINER,
      cpuShares: 1024,
      memoryLimitMb: BigInt(512),
      containerPort: input.containerPort ?? 80,
      sourceKind,
      gitUrl: input.gitUrl ?? '',
      gitRef: input.gitRef ?? 'main',
      dockerfilePath: input.dockerfilePath ?? 'Dockerfile',
      replicaCount: input.replicaCount ?? 1,
      scheduleStrategy: input.scheduleStrategy ?? 'spread',
      requireKvm: input.requireKvm ?? false,
      preferredZone: input.preferredZone ?? 'default',
    }),
  }))
  if (!response.workload) throw new Error('Workload was not returned')
  return response.workload
}

export async function deleteWorkload(id: string): Promise<void> {
  await workloadClient.deleteWorkload(create(DeleteWorkloadRequestSchema, { id }))
}

export async function updateWorkload(input: {
  id: string
  projectId: string
  name: string
  imageOrSource?: string
  containerPort?: number
  replicaCount?: number
  scheduleStrategy?: string
  requireKvm?: boolean
  preferredZone?: string
  sourceKind?: string
  gitUrl?: string
  gitRef?: string
  dockerfilePath?: string
  memoryLimitMb?: number
  cpuShares?: number
}): Promise<Workload> {
  const response = await workloadClient.updateWorkload(create(UpdateWorkloadRequestSchema, {
    workload: create(WorkloadSchema, {
      id: input.id,
      projectId: input.projectId,
      name: input.name,
      imageOrSource: input.imageOrSource || '',
      runtime: RuntimeType.CONTAINER,
      cpuShares: input.cpuShares ?? 1024,
      memoryLimitMb: BigInt(input.memoryLimitMb ?? 512),
      containerPort: input.containerPort ?? 80,
      sourceKind: input.sourceKind || (input.gitUrl ? 'git' : 'image'),
      gitUrl: input.gitUrl ?? '',
      gitRef: input.gitRef ?? 'main',
      dockerfilePath: input.dockerfilePath ?? 'Dockerfile',
      replicaCount: input.replicaCount ?? 1,
      scheduleStrategy: input.scheduleStrategy ?? 'spread',
      requireKvm: input.requireKvm ?? false,
      preferredZone: input.preferredZone ?? 'default',
    }),
  }))
  if (!response.workload) throw new Error('Workload was not returned')
  return response.workload
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

export async function deployFromGit(input: {
  workloadId: string
  gitUrl: string
  gitRef?: string
  dockerfilePath?: string
  containerPort?: number
}) {
  return workloadClient.deployFromGit(create(DeployFromGitRequestSchema, {
    workloadId: input.workloadId,
    gitUrl: input.gitUrl,
    gitRef: input.gitRef ?? 'main',
    dockerfilePath: input.dockerfilePath ?? 'Dockerfile',
    containerPort: input.containerPort ?? 80,
  }))
}

export async function listBuilds(workloadId: string): Promise<Build[]> {
  const response = await workloadClient.listBuilds(create(ListBuildsRequestSchema, { workloadId }))
  return response.builds
}

export async function getBuildLogs(buildId: string, tailLines = 200): Promise<{ build?: Build; lines: string[] }> {
  const response = await workloadClient.getBuildLogs(create(GetBuildLogsRequestSchema, {
    buildId,
    tailLines,
  }))
  return { build: response.build, lines: response.lines }
}
