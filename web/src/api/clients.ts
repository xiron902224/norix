import { createClient, type Interceptor } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { AgentService } from '../gen/v1/agent_pb'
import { AuthService } from '../gen/v1/auth_pb'
import { DatabaseService } from '../gen/v1/database_pb'
import { MetricsService } from '../gen/v1/metrics_pb'
import { PreviewService } from '../gen/v1/preview_pb'
import { SandboxService } from '../gen/v1/sandbox_pb'
import { WasmService } from '../gen/v1/wasm_pb'
import { TeamService } from '../gen/v1/team_pb'
import { ProjectService, WorkloadService } from '../gen/v1/workload_pb'
import { sessionStore } from '../features/auth/services/session'

const authInterceptor: Interceptor = (next) => async (request) => {
  const token = sessionStore.getToken()
  if (token) {
    request.header.set('Authorization', `Bearer ${token}`)
  }
  return next(request)
}

const transport = createConnectTransport({
  baseUrl: import.meta.env.VITE_NORIX_API_URL ?? '/api',
  interceptors: [authInterceptor],
})

export const authClient = createClient(AuthService, transport)
export const projectClient = createClient(ProjectService, transport)
export const workloadClient = createClient(WorkloadService, transport)
export const agentClient = createClient(AgentService, transport)
export const metricsClient = createClient(MetricsService, transport)
export const databaseClient = createClient(DatabaseService, transport)
export const sandboxClient = createClient(SandboxService, transport)
export const previewClient = createClient(PreviewService, transport)
export const wasmClient = createClient(WasmService, transport)
export const teamClient = createClient(TeamService, transport)
