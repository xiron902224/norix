import { Plus, Trash2 } from 'lucide-react'

/** Must match control-plane `db.EnvVarMaskedPlaceholder` (#8). */
export const ENV_MASK = '********'

export type EnvRow = { key: string; value: string }

export function envMapToRows(env: Record<string, string> | undefined): EnvRow[] {
  if (!env) return []
  return Object.entries(env).map(([key, value]) => ({ key, value }))
}

export function envRowsToMap(rows: EnvRow[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) {
    const key = row.key.trim()
    if (!key) continue
    out[key] = row.value
  }
  return out
}

type Props = {
  idPrefix: string
  rows: EnvRow[]
  onChange: (rows: EnvRow[]) => void
  /** Create forms: empty values OK. Edit: leave mask to keep previous secret. */
  hint?: string
}

export function EnvVarsEditor({ idPrefix, rows, onChange, hint }: Props) {
  function updateRow(index: number, patch: Partial<EnvRow>) {
    onChange(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  function removeRow(index: number) {
    onChange(rows.filter((_, i) => i !== index))
  }

  function addRow() {
    onChange([...rows, { key: '', value: '' }])
  }

  return (
    <div className="env-vars-editor">
      <div className="env-vars-editor-head">
        <strong>Environment variables</strong>
        <button type="button" className="linkish" onClick={addRow}>
          <Plus size={14} /> Add
        </button>
      </div>
      {hint ? <p className="muted env-vars-hint">{hint}</p> : null}
      {rows.length === 0 ? (
        <p className="muted">No env vars. Add keys like <code>FOO</code> / <code>secret</code>.</p>
      ) : (
        <ul className="env-vars-list">
          {rows.map((row, index) => (
            <li key={`${idPrefix}-${index}`}>
              <label className="sr-only" htmlFor={`${idPrefix}-key-${index}`}>Key</label>
              <input
                id={`${idPrefix}-key-${index}`}
                className="mono"
                placeholder="KEY"
                value={row.key}
                autoComplete="off"
                onChange={(e) => updateRow(index, { key: e.target.value })}
              />
              <label className="sr-only" htmlFor={`${idPrefix}-val-${index}`}>Value</label>
              <input
                id={`${idPrefix}-val-${index}`}
                className="mono"
                type="password"
                placeholder="value"
                value={row.value}
                autoComplete="new-password"
                onChange={(e) => updateRow(index, { value: e.target.value })}
                onFocus={(e) => {
                  // If still masked, select all so typing replaces the placeholder.
                  if (row.value === ENV_MASK) e.target.select()
                }}
              />
              <button
                type="button"
                className="icon-btn"
                aria-label={`Remove ${row.key || 'row'}`}
                onClick={() => removeRow(index)}
              >
                <Trash2 size={14} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
