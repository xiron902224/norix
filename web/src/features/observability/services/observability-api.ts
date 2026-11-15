import { create } from '@bufbuild/protobuf'
import { metricsClient } from '../../../api/clients'
import {
  GetClusterMetricsRequestSchema,
  ListAlertsRequestSchema,
  QueryLogsRequestSchema,
  type GetClusterMetricsResponse,
  type ListAlertsResponse,
  type QueryLogsResponse,
} from '../../../gen/v1/metrics_pb'

export async function getClusterMetrics(): Promise<GetClusterMetricsResponse> {
  return metricsClient.getClusterMetrics(create(GetClusterMetricsRequestSchema, { limit: 100 }))
}

export async function queryLogs(opts?: {
  projectId?: string
  workloadId?: string
  level?: string
  query?: string
  limit?: number
}): Promise<QueryLogsResponse> {
  return metricsClient.queryLogs(
    create(QueryLogsRequestSchema, {
      projectId: opts?.projectId ?? '',
      workloadId: opts?.workloadId ?? '',
      level: opts?.level ?? '',
      query: opts?.query ?? '',
      limit: opts?.limit ?? 100,
    }),
  )
}

export async function listAlerts(): Promise<ListAlertsResponse> {
  return metricsClient.listAlerts(create(ListAlertsRequestSchema, { limit: 50 }))
}
