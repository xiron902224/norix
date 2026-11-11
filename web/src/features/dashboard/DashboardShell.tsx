import { useEffect, useState } from 'react'
import {
  FolderKanban,
  Globe,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  PanelsTopLeft,
  Server,
  Sun,
  X,
} from 'lucide-react'
import { Button } from '../../shared/ui/Button'
import type { SessionUser } from '../auth/services/session'
import { renewSession } from '../auth/services/auth-api'
import { sessionStore } from '../auth/services/session'
import type { Project } from '../../gen/v1/workload_pb'
import { ProjectsPage } from '../projects/components/ProjectsPage'
import { ProjectDetailPage } from '../projects/components/ProjectDetailPage'
import { NodesPage } from '../nodes/components/NodesPage'
import { DomainsPage } from '../domains/components/DomainsPage'
import { OverviewPage } from './OverviewPage'

interface DashboardShellProps {
  user: SessionUser
  onSignOut: () => void
}

type NavKey = 'overview' | 'projects' | 'nodes' | 'domains'

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

export function DashboardShell({ user, onSignOut }: DashboardShellProps) {
  const [selected, setSelected] = useState<Project | null>(null)
  const [nav, setNav] = useState<NavKey>('overview')
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

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    window.localStorage.setItem('norix-theme', dark ? 'dark' : 'light')
  }, [dark])

  useEffect(() => {
    window.localStorage.setItem('norix-sidebar', collapsed ? 'collapsed' : 'expanded')
  }, [collapsed])

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

  function go(next: NavKey) {
    setNav(next)
    setSelected(null)
    setSidebarOpen(false)
  }

  function openProject(project: Project) {
    setNav('projects')
    setSelected(project)
    setSidebarOpen(false)
  }

  const title = selected
    ? selected.name
    : nav === 'overview'
      ? 'Overview'
      : nav === 'nodes'
        ? 'Nodes'
        : nav === 'domains'
          ? 'Domains'
          : 'Projects'

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

      <aside className="sidebar" aria-label="Primary">
        <div className="sidebar-brand">
          <div className="brand-mark" aria-hidden>
            <PanelsTopLeft />
          </div>
          <div className="sidebar-brand-copy">
            <strong>Norix</strong>
            <span>control plane</span>
          </div>
        </div>

        <nav className="sidebar-nav">
          <div className="nav-section-label">Workspace</div>
          <button
            type="button"
            className={`nav-item ${nav === 'overview' && !selected ? 'active' : ''}`}
            onClick={() => go('overview')}
            title="Overview"
          >
            <LayoutDashboard />
            <span className="nav-label">Overview</span>
          </button>
          <button
            type="button"
            className={`nav-item ${nav === 'projects' || selected ? 'active' : ''}`}
            onClick={() => go('projects')}
            title="Projects"
          >
            <FolderKanban />
            <span className="nav-label">Projects</span>
          </button>
          <button
            type="button"
            className={`nav-item ${nav === 'nodes' ? 'active' : ''}`}
            onClick={() => go('nodes')}
            title="Nodes"
          >
            <Server />
            <span className="nav-label">Nodes</span>
          </button>
          <button
            type="button"
            className={`nav-item ${nav === 'domains' ? 'active' : ''}`}
            onClick={() => go('domains')}
            title="Domains"
          >
            <Globe />
            <span className="nav-label">Domains</span>
          </button>
        </nav>

        <div className="sidebar-footer">
          <button
            type="button"
            className="nav-item collapse-toggle"
            onClick={() => setCollapsed((value) => !value)}
            title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            aria-pressed={collapsed}
          >
            {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            <span className="nav-label">{collapsed ? 'Expand' : 'Collapse'}</span>
          </button>
          <button type="button" className="nav-item" onClick={onSignOut} title="Sign out">
            <LogOut />
            <span className="nav-label">Sign out</span>
          </button>
        </div>
      </aside>

      <div className="app-main">
        <header className="topbar flux-frosted">
          <div className="topbar-left">
            <button
              type="button"
              className="icon-button menu-toggle"
              aria-label={sidebarOpen ? 'Close navigation' : 'Open navigation'}
              aria-expanded={sidebarOpen}
              onClick={() => setSidebarOpen((open) => !open)}
            >
              {sidebarOpen ? <X /> : <Menu />}
            </button>
            <button
              type="button"
              className="icon-button desktop-collapse"
              aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              aria-pressed={collapsed}
              onClick={() => setCollapsed((value) => !value)}
            >
              {collapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            </button>
            <nav className="crumb-trail" aria-label="Breadcrumb">
              <button type="button" onClick={() => go('overview')}>Norix</button>
              <span aria-hidden>/</span>
              {selected ? (
                <>
                  <button type="button" onClick={() => go('projects')}>Projects</button>
                  <span aria-hidden>/</span>
                  <strong>{selected.name}</strong>
                </>
              ) : (
                <strong>{title}</strong>
              )}
            </nav>
          </div>

          <div className="topbar-actions">
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

        <div className="content flux-dot-grid">
          <div className="page-enter" key={selected ? selected.id : nav}>
            {nav === 'overview' && !selected ? (
              <OverviewPage
                user={user}
                onOpenProjects={() => go('projects')}
                onOpenProject={openProject}
              />
            ) : null}

            {nav === 'nodes' ? <NodesPage /> : null}
            {nav === 'domains' ? <DomainsPage /> : null}

            {nav === 'projects' && selected ? (
              <ProjectDetailPage project={selected} onBack={() => setSelected(null)} />
            ) : null}

            {nav === 'projects' && !selected ? (
              <ProjectsPage onOpenProject={setSelected} />
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}
