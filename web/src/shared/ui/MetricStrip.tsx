import type { ReactNode } from 'react'

export function MetricStrip({
  items,
}: {
  items: Array<{
    label: string
    value: string | number
    hint?: ReactNode
    tone?: 'up' | 'down' | 'neutral'
  }>
}) {
  return (
    <div className="compass-metric-strip">
      {items.map((item) => (
        <article key={item.label} className="compass-metric-cell">
          <div className="compass-metric-label">{item.label}</div>
          <div className="compass-metric-value">{item.value}</div>
          {item.hint ? (
            <div className={`compass-metric-hint ${item.tone || 'neutral'}`}>{item.hint}</div>
          ) : null}
        </article>
      ))}
    </div>
  )
}
