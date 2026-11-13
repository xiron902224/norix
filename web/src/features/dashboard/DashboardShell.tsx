import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  Activity,
  Bell,
  Box,
  Boxes,
  Cloud,
  Database,
  FolderKanban,
  GitPullRequest,
  Globe,
  LayoutDashboard,
  LineChart,
  LogOut,
  Menu,
  Moon,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  PanelsTopLeft,
  Plus,
  Search,
  Server,
  Sun,
  Users,
  X,
} from 'lucide-react'
import { Button } from '../../shared/ui/Button'
import type { SessionUser } from '../auth/services/session'
import { renewSession } from '../auth/services/auth-api'
import { sessionStore } from '../auth/services/session'
import { listNodes } from '../nodes/services/nodes-api'
import { listProjects } from '../projects/services/projects-api'

interface DashboardShellProps {
  user: SessionUser
  onSignOut: () => void
}

function roleLabel(role: number) {
  if (role === 1) return 'owner'
  if (role === 2) return 'admin'
  if (role === 3) return 'member'
  return 'user'
}

function initials(user: SessionUser) {
  const source = user.name || user.email || 'N'
  return source
    .split(/[\s@]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('')
}

function readCollapsed() {
  return window.localStorage.getItem('norix-sidebar') === 'collapsed'
}

function titleFromPath(pathname: string) {
  if (pathname.startsWith('/projects/')) return 'Workload'
  if (pathname.startsWith('/projects')) return 'Projects & Apps'
  if (pathname.startsWith('/nodes')) return 'Compute Nodes'
  if (pathname.startsWith('/domains')) return 'Domains & Edge'
  if (pathname.startsWith('/network')) return 'WireGuard Mesh'
  if (pathname.startsWith('/observability')) return 'Observability'
  if (pathname.startsWith('/databases')) return 'Databases'
  if (pathname.startsWith('/sandboxes')) return 'Sandboxes'
  if (pathname.startsWith('/previews')) return 'PR Previews'
  if (pathname.startsWith('/wasm')) return 'WebAssembly'
  if (pathname.startsWith('/ops')) return 'Backups & Storage'
  if (pathname.startsWith('/teams')) return 'Teams & Quotas'
  return 'Cluster Overview'
}

function crumbFromPath(pathname: string) {
  const parts = pathname.split('/').filter(Boolean)
  if (parts.length === 0) return 'overview · reconciliation synced'
  return parts.join(' · ')
}

export function DashboardShell({ user, onSignOut }: DashboardShellProps) {
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(() => {
    if (typeof window === 'undefined') return false
    return readCollapsed()
  })
  const [dark, setDark] = useState(() => {
    if (typeof window === 'undefined') return true
    const stored = window.localStorage.getItem('norix-theme')
    if (stored === 'light') return false
    if (stored === 'dark') return true
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  })
  const [projectCount, setProjectCount] = useState(0)
  const [nodesOnline, setNodesOnline] = useState(0)
  const [nodesTotal, setNodesTotal] = useState(0)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    window.localStorage.setItem('norix-theme', dark ? 'dark' : 'light')
  }, [dark])

  useEffect(() => {
    window.localStorage.setItem('norix-sidebar', collapsed ? 'collapsed' : 'expanded')
  }, [collapsed])

  useEffect(() => {
    setSidebarOpen(false)
  }, [location.pathname])

  useEffect(() => {
    const token = sessionStore.getToken()
    if (!token) return

    const renew = () => {
      void renewSession(token).catch(() => {
        sessionStore.clear()
        onSignOut()
      })
    }
    renew()
    const timer = window.setInterval(renew, 60 * 60 * 1000)
    return () => window.clearInterval(timer)
  }, [onSignOut])

  useEffect(() => {
    async function fleetPulse() {
      try {
        const [projects, nodes] = await Promise.all([listProjects(), listNodes()])
        setProjectCount(projects.length)
        setNodesTotal(nodes.length)
        setNodesOnline(nodes.filter((node) => node.status === 'online').length)
      } catch {
        /* shell telemetry is best-effort */
      }
    }
    void fleetPulse()
    const timer = window.setInterval(() => { void fleetPulse() }, 8000)
    return () => window.clearInterval(timer)
  }, [])

  const shellClass = [
    'app-shell',
    collapsed ? 'sidebar-collapsed' : '',
    sidebarOpen ? 'sidebar-open' : '',
  ].filter(Boolean).join(' ')

  return (
    <div className={shellClass}>
      {sidebarOpen ? (
        <button
          type="button"
          className="sidebar-backdrop"
          aria-label="Close navigation"
          onClick={() => setSidebarOpen(false)}
        />
      ) : null}

      <aside className="sidebar flux-frosted" aria-label="Primary">
        <div className="sidebar-brand">
          <div className="brand-mark" aria-hidden>
            <PanelsTopLeft />
          </div>
          <div className="sidebar-brand-copy">
            <div className="brand-title-row">
              <strong>Norix</strong>
              <span className="brand-beta" title="Stitch DevOps Hyperstructure">v0.5-beta</span>
            </div>
            <span>control plane</span>
          </div>
        </div>

        <WorkspaceSwitcher />

        <nav className="sidebar-nav">
          <div className="nav-section-label">Overview</div>
          <NavItem to="/overview" icon={<LayoutDashboard />} label="Dashboard" end />
          <NavItem
            to="/projects"
            icon={<FolderKanban />}
            label="Projects"
            badge={projectCount > 0 ? String(projectCount) : undefined}
          />

          <div className="nav-section-label">Infrastructure</div>
          <NavItem
            to="/nodes"
            icon={<Server />}
            label="Nodes"
            dot={nodesOnline > 0 ? 'ok' : undefined}
          />
          <NavItem to="/sandboxes" icon={<Boxes />} label="Sandboxes" />

          <div className="nav-section-label">Traffic</div>
          <NavItem to="/domains" icon={<Globe />} label="Domains" />
          <NavItem to="/network" icon={<Network />} label="Network" />

          <div className="nav-section-label">Operations</div>
          <NavItem to="/observability" icon={<LineChart />} label="Observability" />
          <NavItem to="/ops" icon={<Activity />} label="Ops / Backups" />
          <NavItem to="/teams" icon={<Users />} label="Teams & Quotas" />

          <div className="nav-section-label">Platform</div>
          <NavItem to="/databases" icon={<Database />} label="Databases" />
          <NavItem to="/previews" icon={<GitPullRequest />} label="Previews" />
          <NavItem to="/wasm" icon={<Box />} label="Wasm" />
        </nav>

        <div className="sidebar-telemetry" aria-label="Control plane telemetry">
          <div className="sidebar-telemetry-row">
            <span className="sidebar-telemetry-dot" aria-hidden />
            <span>HTTP/Connect</span>
            <span className="mono telemetry-ok">200 OK</span>
          </div>
          <div className="sidebar-telemetry-row">
            <span className="sidebar-telemetry-dot" aria-hidden />
            <span>Postgres 16</span>
            <span className="mono telemetry-ok">Healthy</span>
          </div>
          <div className="sidebar-telemetry-row">
            <span className="sidebar-telemetry-dot" aria-hidden />
            <span>Agent heartbeat</span>
            <span className="mono">{nodesOnline}/{nodesTotal || '—'} live</span>
          </div>
        </div>

        <div className="sidebar-footer">
          <button
            type="button"
            className="nav-item collapse-toggle"
            onClick={() => setCollapsed((value) => !value)}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            <span className="nav-label">{collapsed ? 'Expand' : 'Collapse'}</span>
          </button>
          <button type="button" className="nav-item signout-mobile" onClick={onSignOut}>
            <LogOut />
            <span className="nav-label">Sign out</span>
          </button>
        </div>
      </aside>

      <div className="shell-main">
        <header className="topbar flux-frosted">
          <div className="topbar-left">
            <button
              type="button"
              className="icon-button mobile-only"
              aria-label="Open navigation"
              onClick={() => setSidebarOpen(true)}
            >
              {sidebarOpen ? <X /> : <Menu />}
            </button>
            <label className="topbar-search" aria-label="Search workloads">
              <Search size={14} aria-hidden />
              <input type="search" placeholder="Search workloads…" disabled title="Coming soon" />
              <kbd>Ctrl K</kbd>
            </label>
            <div className="topbar-stats" aria-label="Fleet snapshot">
              <span>Nodes <strong>{nodesOnline}/{nodesTotal}</strong></span>
              <span>Projects <strong>{projectCount}</strong></span>
              <span className="telemetry-ok">KVM L2 OK</span>
            </div>
          </div>

          <div className="topbar-actions">
            <NavLink to="/projects" className="primary-button flux-glow" style={{ padding: '8px 14px', marginTop: 0 }}>
              <Plus size={14} /> New workload
            </NavLink>
            <button type="button" className="icon-button" aria-label="Notifications" title="Alerts on Ops page">
              <Bell size={16} />
            </button>
            <button
              type="button"
              className="icon-button theme-toggle"
              aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
              onClick={() => setDark((value) => !value)}
            >
              {dark ? <Sun /> : <Moon />}
            </button>
            <div className="user-chip">
              <span className="user-avatar">{initials(user)}</span>
              <div className="user-meta">
                <strong>{user.name || user.email}</strong>
                <span>{roleLabel(user.role)}</span>
              </div>
            </div>
            <Button variant="secondary" type="button" className="signout-desktop" onClick={onSignOut}>
              Sign out
            </Button>
          </div>
        </header>

        <PageCrumbHeader pathname={location.pathname} title={titleFromPath(location.pathname)} crumb={crumbFromPath(location.pathname)} />

        <div className="content flux-dot-grid">
          <div className="page-enter" key={location.pathname}>
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  )
}

function WorkspaceSwitcher() {
  return (
    <div className="workspace-switcher" aria-label="Workspace">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
        <Cloud size={14} style={{ color: 'var(--tertiary)', flexShrink: 0 }} aria-hidden />
        <div style={{ minWidth: 0 }}>
          <div className="workspace-switcher-title">Norix workspace</div>
          <div className="workspace-switcher-sub">primary · DevOps Hyperstructure</div>
        </div>
      </div>
    </div>
  )
}

function PageCrumbHeader({
  pathname,
  title,
  crumb,
}: {
  pathname: string
  title: string
  crumb: string
}) {
  if (pathname === '/overview') {
    return (
      <div style={{ padding: '12px 22px 0', borderBottom: '1px solid color-mix(in oklab, var(--border) 70%, transparent)' }}>
        <p className="eyebrow breadcrumb-mono" style={{ margin: 0 }}>us-east-cluster-01 · reconciliation: synced</p>
        <h1 style={{ margin: '4px 0 12px', fontSize: '1.35rem', letterSpacing: '-0.03em' }}>{title}</h1>
      </div>
    )
  }
  return (
    <div style={{ padding: '12px 22px 0' }}>
      <p className="eyebrow breadcrumb-mono" style={{ margin: 0 }}>{crumb}</p>
      <h1 style={{ margin: '4px 0 12px', fontSize: '1.2rem', letterSpacing: '-0.03em' }}>{title}</h1>
    </div>
  )
}

function NavItem({
  to,
  icon,
  label,
  end,
  badge,
  dot,
}: {
  to: string
  icon: ReactNode
  label: string
  end?: boolean
  badge?: string
  dot?: 'ok'
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}
      title={label}
    >
      {icon}
      <span className="nav-label">{label}</span>
      {badge ? <span className="nav-badge">{badge}</span> : null}
      {dot === 'ok' ? <span className="nav-badge" style={{ border: 0, padding: 0 }}><span className="sidebar-telemetry-dot" /></span> : null}
    </NavLink>
  )
}
