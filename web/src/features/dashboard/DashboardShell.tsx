import { useEffect, useState, type ReactNode } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  Bell,
  Box,
  Boxes,
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
  Plus,
  Rocket,
  Search,
  Server,
  Settings,
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
import { fetchControlPlaneStatus } from '../../shared/lib/status'

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

function orgSlug(user: SessionUser) {
  const source = (user.name || user.email || 'workspace').toLowerCase()
  return source.replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 24) || 'workspace'
}

function readCollapsed() {
  return window.localStorage.getItem('norix-sidebar') === 'collapsed'
}

export function DashboardShell({ user, onSignOut }: DashboardShellProps) {
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(() => typeof window !== 'undefined' && readCollapsed())
  const [dark, setDark] = useState(() => {
    if (typeof window === 'undefined') return true
    const stored = window.localStorage.getItem('norix-theme')
    if (stored === 'light') return false
    return true
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

  useEffect(() => { setSidebarOpen(false) }, [location.pathname])

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

  const [cpVersion, setCpVersion] = useState('')

  useEffect(() => {
    async function fleetPulse() {
      try {
        const [projects, nodes] = await Promise.all([listProjects(), listNodes()])
        setProjectCount(projects.length)
        setNodesTotal(nodes.length)
        setNodesOnline(nodes.filter((node) => node.status === 'online').length)
      } catch { /* ignore */ }
    }
    void fleetPulse()
    const timer = window.setInterval(() => { void fleetPulse() }, 8000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    let cancelled = false
    void fetchControlPlaneStatus().then((body) => {
      if (cancelled || !body) return
      const ver = typeof body.version === 'string' ? body.version.trim() : ''
      if (ver) setCpVersion(ver.startsWith('v') ? ver : `v${ver}`)
    })
    return () => {
      cancelled = true
    }
  }, [])

  const capacityPct = nodesTotal > 0 ? Math.round((nodesOnline / nodesTotal) * 100) : 0
  const org = orgSlug(user)
  const shellClass = ['app-shell', collapsed ? 'sidebar-collapsed' : '', sidebarOpen ? 'sidebar-open' : ''].filter(Boolean).join(' ')

  return (
    <div className={shellClass}>
      {sidebarOpen ? (
        <button type="button" className="sidebar-backdrop" aria-label="Close navigation" onClick={() => setSidebarOpen(false)} />
      ) : null}

      <aside className="sidebar" aria-label="Primary">
        <div className="sidebar-brand">
          <div className="brand-mark-stack">
            <div className="brand-mark" aria-hidden>N</div>
            {cpVersion ? (
              <span className="brand-beta brand-beta-under-logo" title="Control plane version from /status">
                {cpVersion}
              </span>
            ) : null}
          </div>
          <div className="sidebar-brand-copy">
            <strong>norix</strong>
          </div>
        </div>

        <div className="workspace-switcher compass-org" aria-label="Organization">
          <div className="compass-org-head">
            <span className="compass-org-avatar">{org.slice(0, 2).toUpperCase()}</span>
            <div className="compass-org-copy">
              <div className="workspace-switcher-sub">Organization</div>
              <div className="workspace-switcher-title">{org}</div>
            </div>
          </div>
          <div className="compass-plan">
            <div className="compass-plan-row">
              <span>Plan</span>
              <strong>{capacityPct}%</strong>
            </div>
            <div className="compass-plan-track" aria-hidden>
              <span style={{ width: `${Math.min(100, Math.max(capacityPct, 4))}%` }} />
            </div>
            <div className="compass-plan-meta mono">{nodesOnline}/{nodesTotal || 0} nodes online</div>
          </div>
        </div>

        <nav className="sidebar-nav">
          <NavItem to="/overview" icon={<LayoutDashboard />} label="Overview" end />
          <NavItem to="/projects" icon={<FolderKanban />} label="Projects" badge={projectCount > 0 ? String(projectCount) : undefined} />
          <NavItem to="/ops" icon={<Rocket />} label="Deployments" />
          <NavItem to="/databases" icon={<Database />} label="Databases" />
          <NavItem to="/domains" icon={<Globe />} label="Domains" />
          <NavItem to="/nodes" icon={<Server />} label="Regions & nodes" dot={nodesOnline > 0 ? 'ok' : undefined} />
          <NavItem to="/wasm" icon={<Box />} label="Runtimes" />
          <NavItem to="/network" icon={<Network />} label="Networks" />
          <NavItem to="/sandboxes" icon={<Boxes />} label="Sandboxes" />
          <NavItem to="/previews" icon={<GitPullRequest />} label="Previews" />
          <NavItem to="/observability" icon={<LineChart />} label="Observability" />
          <NavItem to="/teams" icon={<Users />} label="Access" />
          <NavItem to="/teams" icon={<Settings />} label="Settings" />
        </nav>

        <div className="sidebar-footer">
          <button type="button" className="nav-item collapse-toggle" onClick={() => setCollapsed((v) => !v)}>
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
        <header className="topbar">
          <div className="topbar-left">
            <button type="button" className="icon-button mobile-only" aria-label="Open navigation" onClick={() => setSidebarOpen(true)}>
              {sidebarOpen ? <X /> : <Menu />}
            </button>
            <div className="env-strip" aria-label="Environment">
              <span className="env-org mono">{org}</span>
              <span className="env-sep">/</span>
              <span className="env-chip is-active">production</span>
              <span className="env-chip" title="Coming soon">staging</span>
              <span className="env-chip" title="Coming soon">development</span>
            </div>
            <label className="topbar-search" aria-label="Search">
              <Search size={14} aria-hidden />
              <input type="search" placeholder="Search apps, domains, nodes…" disabled title="Coming soon" />
              <kbd>⌘K</kbd>
            </label>
          </div>
          <div className="topbar-actions">
            <NavLink to="/projects" className="primary-button topbar-cta">
              <Plus size={14} /> New app
            </NavLink>
            <button type="button" className="icon-button" aria-label="Alerts" title="Observability">
              <Bell size={16} />
            </button>
            <button
              type="button"
              className="icon-button theme-toggle"
              aria-label={dark ? 'Light mode' : 'Dark mode'}
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

        <div className="content">
          <div className="page-enter" key={location.pathname}>
            <Outlet />
          </div>
        </div>
      </div>
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
    <NavLink to={to} end={end} className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`} title={label}>
      {icon}
      <span className="nav-label">{label}</span>
      {badge ? <span className="nav-badge">{badge}</span> : null}
      {dot === 'ok' ? <span className="nav-badge" style={{ border: 0, padding: 0 }}><span className="sidebar-telemetry-dot" /></span> : null}
    </NavLink>
  )
}
