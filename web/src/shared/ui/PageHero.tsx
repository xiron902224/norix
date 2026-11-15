import type { ReactNode } from 'react'

export function PageHero({
  eyebrow = 'norix / control plane',
  title,
  subtitle,
  actions,
}: {
  eyebrow?: ReactNode
  title: string
  subtitle?: string
  actions?: ReactNode
}) {
  return (
    <header className="compass-page-hero">
      <div>
        <div className="compass-eyebrow">{eyebrow}</div>
        <h1>{title}</h1>
        {subtitle ? <p className="compass-subtitle">{subtitle}</p> : null}
      </div>
      {actions ? <div className="compass-hero-actions">{actions}</div> : null}
    </header>
  )
}
