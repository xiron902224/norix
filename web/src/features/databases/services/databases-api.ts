import { create } from '@bufbuild/protobuf'
import { databaseClient } from '../../../api/clients'
import {
  CreateManagedDatabaseRequestSchema,
  ListManagedDatabasesRequestSchema,
  ManagedDatabaseEngine,
  type ManagedDatabase,
} from '../../../gen/v1/database_pb'

export async function listManagedDatabases(projectId = ''): Promise<ManagedDatabase[]> {
  const response = await databaseClient.listManagedDatabases(
    create(ListManagedDatabasesRequestSchema, { projectId }),
  )
  return response.databases
}

export async function createManagedDatabase(input: {
  projectId: string
  name: string
  engine: ManagedDatabaseEngine
  backupSchedule?: string
}): Promise<ManagedDatabase> {
  const response = await databaseClient.createManagedDatabase(
    create(CreateManagedDatabaseRequestSchema, {
      projectId: input.projectId,
      name: input.name,
      engine: input.engine,
      backupSchedule: input.backupSchedule ?? '',
    }),
  )
  if (!response.database) throw new Error('Database was not returned')
  return response.database
}
