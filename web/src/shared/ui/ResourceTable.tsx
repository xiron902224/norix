import type { ReactNode } from 'react'

export interface ResourceRow {
  id: string
  name: string
  subtitle?: string
  status: string
  values: ReactNode[]
  action?: ReactNode
}

const OK = new Set(['running', 'active', 'online', 'ready', 'synced', 'enabled', 'resolved'])
const BAD = new Set(['failed', 'critical', 'degraded', 'error', 'offline'])

function statusTone(status: string) {
  const key = (status || '').toLowerCase()
  if (OK.has(key)) return 'ok'
  if (BAD.has(key)) return 'danger'
  return 'warn'
}

function statusText(status: string) {
  const key = (status || '').toLowerCase()
  if (key === 'running' || key === 'active' || key === 'online' || key === 'ready') return 'Active'
  if (key === 'synced') return 'Synced'
  if (key === 'building') return 'Building'
  if (key === 'deploying') return 'Deploying'
  if (key === 'failed') return 'Failed'
  if (key === 'stopped' || key === 'idle') return 'Idle'
  if (key === 'pending') return 'Pending'
  if (key === 'provisioning') return 'Provisioning'
  return status || 'Pending'
}

export function ResourceTable({
  headers,
  rows,
  selectedId,
  onSelect,
  empty,
  leadingHeader = 'Name',
}: {
  headers: string[]
  rows: ResourceRow[]
  selectedId?: string
  onSelect?: (row: ResourceRow) => void
  empty?: ReactNode
  leadingHeader?: string
}) {
  const hasAction = rows.some((row) => row.action != null)

  if (rows.length === 0) {
    return (
      <div className="compass-empty">
        {empty || 'No resources yet.'}
      </div>
    )
  }

  return (
    <div className="table-scroll compass-table-wrap">
      <table className="compass-data-table">
        <thead>
          <tr>
            <th>{leadingHeader}</th>
            <th>Status</th>
            {headers.map((header) => (
              <th key={header}>{header}</th>
            ))}
            {hasAction ? <th aria-label="Actions" /> : null}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={row.id}
              className={selectedId === row.id ? 'is-selected' : ''}
              onClick={() => onSelect?.(row)}
            >
              <td>
                <button
                  type="button"
                  className="compass-table-link"
                  onClick={(event) => {
                    event.stopPropagation()
                    onSelect?.(row)
                  }}
                >
                  {row.name}
                </button>
                {row.subtitle ? <div className="table-sub">{row.subtitle}</div> : null}
              </td>
              <td>
                <span className={`compass-status-inline tone-${statusTone(row.status)}`}>
                  <span className="status-dot" aria-hidden />
                  {statusText(row.status)}
                </span>
              </td>
              {row.values.map((value, index) => (
                <td
                  key={`${row.id}-${index}`}
                  className={index === 0 ? '' : 'mono muted'}
                >
                  {value}
                </td>
              ))}
              {row.action ? <td onClick={(event) => event.stopPropagation()}>{row.action}</td> : null}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
