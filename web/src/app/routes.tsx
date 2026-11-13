import { Navigate, Route, Routes } from 'react-router-dom'
import type { SessionUser } from '../features/auth/services/session'
import { DashboardShell } from '../features/dashboard/DashboardShell'
import { OverviewPage } from '../features/dashboard/OverviewPage'
import { ProjectsPage } from '../features/projects/components/ProjectsPage'
import { ProjectDetailPage } from '../features/projects/components/ProjectDetailPage'
import { NodesPage } from '../features/nodes/components/NodesPage'
import { DomainsPage } from '../features/domains/components/DomainsPage'
import { NetworkPage } from '../features/network/components/NetworkPage'
import { OpsPage } from '../features/ops/components/OpsPage'
import { ObservabilityPage } from '../features/observability/components/ObservabilityPage'
import { DatabasesPage } from '../features/databases/components/DatabasesPage'
import { SandboxesPage } from '../features/sandboxes/components/SandboxesPage'
import { PreviewsPage } from '../features/previews/components/PreviewsPage'
import { WasmPage } from '../features/wasm/components/WasmPage'
import { TeamsPage } from '../features/teams/components/TeamsPage'

export function AuthenticatedRoutes({
  user,
  onSignOut,
}: {
  user: SessionUser
  onSignOut: () => void
}) {
  return (
    <Routes>
      <Route element={<DashboardShell user={user} onSignOut={onSignOut} />}>
        <Route index element={<Navigate to="/overview" replace />} />
        <Route path="overview" element={<OverviewPage user={user} />} />
        <Route path="projects" element={<ProjectsPage />} />
        <Route path="projects/:projectId" element={<ProjectDetailPage />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="domains" element={<DomainsPage />} />
        <Route path="network" element={<NetworkPage />} />
        <Route path="observability" element={<ObservabilityPage />} />
        <Route path="databases" element={<DatabasesPage />} />
        <Route path="sandboxes" element={<SandboxesPage />} />
        <Route path="previews" element={<PreviewsPage />} />
        <Route path="wasm" element={<WasmPage />} />
        <Route path="ops" element={<OpsPage />} />
        <Route path="teams" element={<TeamsPage />} />
        <Route path="*" element={<Navigate to="/overview" replace />} />
      </Route>
    </Routes>
  )
}
