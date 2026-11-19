import { create } from '@bufbuild/protobuf'
import { projectClient, workloadClient } from '../../../api/clients'
import {
  AnalyzeComposeFromGitRequestSchema,
  ComposeServiceSpecSchema,
  ComposeStackActionRequestSchema,
  CreateComposeStackRequestSchema,
  GetComposeStackRequestSchema,
  ListComposeStacksRequestSchema,
  UpdateComposeStackRequestSchema,
  type ComposeStack,
  type ComposeStackLogLine,
  CreateProjectRequestSchema,
  CreateWorkloadRequestSchema,
  DeleteProjectRequestSchema,
  DeleteWorkloadRequestSchema,
  DeployComposeStackRequestSchema,
  DeployFromGitRequestSchema,
  DeployRequestSchema,
  GetBuildLogsRequestSchema,
  GetWorkloadLogsRequestSchema,
  ListBuildsRequestSchema,
  ListProjectsRequestSchema,
  ListWorkloadReleasesRequestSchema,
  ListWorkloadsRequestSchema,
  RevealWorkloadEnvRequestSchema,
  RollbackWorkloadRequestSchema,
  StopWorkloadRequestSchema,
  RuntimeType,
  UpdateWorkloadRequestSchema,
  WorkloadSchema,
  VolumeMountSchema,
  BackupWorkloadVolumeRequestSchema,
  DeleteWorkloadVolumeRequestSchema,
  type Build,
  type ComposeServiceSpec,
  type Project,
  type Workload,
  type WorkloadRelease,
  type VolumeMount,
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
  envVars?: Record<string, string>
  volumeMounts?: VolumeMount[]
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
      envVars: input.envVars ?? {},
      volumeMounts: input.volumeMounts ?? [],
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
  envVars?: Record<string, string>
  updateStrategy?: string
  maxSurge?: number
  maxUnavailable?: number
  probeSpecJson?: string
  volumeMounts?: VolumeMount[]
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
      envVars: input.envVars ?? {},
      updateStrategy: input.updateStrategy ?? 'rolling',
      maxSurge: input.maxSurge ?? 1,
      maxUnavailable: input.maxUnavailable ?? 0,
      probeSpecJson: input.probeSpecJson ?? '',
      volumeMounts: input.volumeMounts ?? [],
    }),
  }))
  if (!response.workload) throw new Error('Workload was not returned')
  return response.workload
}

export function parseVolumeMountsJson(raw: string): VolumeMount[] {
  const trimmed = raw.trim()
  if (!trimmed) return []
  const parsed = JSON.parse(trimmed) as Array<{ name?: string; mount_path?: string; mountPath?: string; read_only?: boolean; readOnly?: boolean }>
  if (!Array.isArray(parsed)) throw new Error('volume mounts must be a JSON array')
  return parsed.map((m) => create(VolumeMountSchema, {
    name: (m.name ?? '').trim(),
    mountPath: (m.mount_path ?? m.mountPath ?? '').trim(),
    readOnly: Boolean(m.read_only ?? m.readOnly),
  }))
}

export async function backupWorkloadVolume(workloadId: string, volumeName: string): Promise<{ opId: string; status: string }> {
  const response = await workloadClient.backupWorkloadVolume(create(BackupWorkloadVolumeRequestSchema, {
    workloadId,
    volumeName,
  }))
  return { opId: response.opId, status: response.status }
}

export async function deleteWorkloadVolume(workloadId: string, volumeName: string): Promise<{ opId: string; status: string }> {
  const response = await workloadClient.deleteWorkloadVolume(create(DeleteWorkloadVolumeRequestSchema, {
    workloadId,
    volumeName,
    confirm: true,
  }))
  return { opId: response.opId, status: response.status }
}

export async function stopWorkload(id: string): Promise<void> {
  await workloadClient.stopWorkload(create(StopWorkloadRequestSchema, { workloadId: id }))
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

export type ComposeStackAction = 'start' | 'stop' | 'redeploy' | 'delete'

export interface ComposeStackSettings {
  /** Services that keep a published host port. Everything else stays internal (#146). */
  publicServices?: string[]
  /** service → hostname. Routed through Caddy with HTTPS/WSS (#146). */
  domains?: Record<string, string>
  /** NAME → value. Write-only: values are never returned by the API (#145). */
  variables?: Record<string, string>
}

export async function createComposeStack(input: {
  projectId: string
  name: string
  gitUrl: string
  gitRef?: string
  composePath?: string
} & ComposeStackSettings): Promise<ComposeStack | undefined> {
  const response = await workloadClient.createComposeStack(create(CreateComposeStackRequestSchema, {
    projectId: input.projectId,
    name: input.name,
    gitUrl: input.gitUrl,
    gitRef: input.gitRef ?? 'main',
    composePath: input.composePath ?? 'docker-compose.yml',
    publicServices: input.publicServices ?? [],
    domains: input.domains ?? {},
    variables: input.variables ?? {},
  }))
  return response.stack
}

/**
 * Replaces public services and domains. Variables are upserted: a non-empty
 * value sets it, an empty value deletes it (#145).
 */
export async function updateComposeStack(input: { id: string } & ComposeStackSettings): Promise<ComposeStack | undefined> {
  const response = await workloadClient.updateComposeStack(create(UpdateComposeStackRequestSchema, {
    id: input.id,
    publicServices: input.publicServices ?? [],
    domains: input.domains ?? {},
    variables: input.variables ?? {},
  }))
  return response.stack
}

export async function listComposeStacks(projectId: string): Promise<ComposeStack[]> {
  const response = await workloadClient.listComposeStacks(create(ListComposeStacksRequestSchema, { projectId }))
  return response.stacks
}

export async function getComposeStack(id: string, service = '', tailLines = 300): Promise<{
  stack?: ComposeStack
  logs: ComposeStackLogLine[]
}> {
  const response = await workloadClient.getComposeStack(create(GetComposeStackRequestSchema, { id, service, tailLines }))
  return { stack: response.stack, logs: response.logs }
}

export async function composeStackAction(id: string, action: ComposeStackAction, purgeVolumes = false): Promise<ComposeStack | undefined> {
  const response = await workloadClient.composeStackAction(create(ComposeStackActionRequestSchema, {
    id,
    action,
    // Only honoured for delete: removes named volumes and the checkout (#147).
    purgeVolumes: action === 'delete' && purgeVolumes,
  }))
  return response.stack
}

export async function analyzeComposeFromGit(input: {
  gitUrl: string
  gitRef?: string
  composePath?: string
}) {
  return workloadClient.analyzeComposeFromGit(create(AnalyzeComposeFromGitRequestSchema, {
    gitUrl: input.gitUrl,
    gitRef: input.gitRef ?? 'main',
    composePath: input.composePath ?? 'docker-compose.yml',
  }))
}

export async function deployComposeStack(input: {
  projectId: string
  stackName: string
  gitUrl: string
  gitRef?: string
  composePath?: string
  services: ComposeServiceSpec[]
}) {
  return workloadClient.deployComposeStack(create(DeployComposeStackRequestSchema, {
    projectId: input.projectId,
    stackName: input.stackName,
    gitUrl: input.gitUrl,
    gitRef: input.gitRef ?? 'main',
    composePath: input.composePath ?? 'docker-compose.yml',
    services: input.services.map((svc) => create(ComposeServiceSpecSchema, {
      name: svc.name,
      image: svc.image,
      buildContext: svc.buildContext,
      dockerfile: svc.dockerfile,
      containerPorts: svc.containerPorts,
      publishedPorts: svc.publishedPorts,
      dependsOn: svc.dependsOn,
      environment: svc.environment,
      enabled: svc.enabled,
      exposePublic: svc.exposePublic,
      hostPort: svc.hostPort,
      containerPort: svc.containerPort,
      hasBuild: svc.hasBuild,
      hasImage: svc.hasImage,
    })),
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

export async function listWorkloadReleases(workloadId: string, limit = 20): Promise<WorkloadRelease[]> {
  const response = await workloadClient.listWorkloadReleases(create(ListWorkloadReleasesRequestSchema, {
    workloadId,
    limit,
  }))
  return response.releases
}

export async function rollbackWorkload(workloadId: string, releaseId: string) {
  return workloadClient.rollbackWorkload(create(RollbackWorkloadRequestSchema, {
    workloadId,
    releaseId,
  }))
}

export async function revealWorkloadEnv(workloadId: string): Promise<Record<string, string>> {
  const response = await workloadClient.revealWorkloadEnv(create(RevealWorkloadEnvRequestSchema, {
    workloadId,
  }))
  return response.envVars
}
