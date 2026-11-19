// Parsers for the compose stack forms (#144–#147). Pure functions so the
// rules can be unit-tested without rendering React.

/** "web, api  worker" → ["web", "api", "worker"]. Commas or spaces. */
export function parseServiceList(raw: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of raw.split(/[\s,]+/)) {
    const name = part.trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push(name)
  }
  return out
}

/** One "service=hostname" per line. Blank lines and # comments are ignored. */
export function parseDomainLines(raw: string): Record<string, string> {
  const out: Record<string, string> = {}
  raw.split('\n').forEach((line, index) => {
    const text = line.trim()
    if (!text || text.startsWith('#')) return
    const eq = text.indexOf('=')
    const service = eq > 0 ? text.slice(0, eq).trim() : ''
    const host = eq > 0 ? text.slice(eq + 1).trim() : ''
    if (!service || !host) {
      throw new Error(`Domain line ${index + 1} must look like service=hostname`)
    }
    out[service] = host
  })
  return out
}

/**
 * One "NAME=value" per line. Values are write-only: they are sent once and
 * stored encrypted. On create, empty values are dropped. On update the caller
 * passes `dropEmpty=false`, so "NAME=" deletes the variable.
 */
export function parseVariableLines(raw: string, dropEmpty: boolean): Record<string, string> {
  const out: Record<string, string> = {}
  raw.split('\n').forEach((line, index) => {
    const text = line.trim()
    if (!text || text.startsWith('#')) return
    const eq = text.indexOf('=')
    const name = eq > 0 ? text.slice(0, eq).trim() : ''
    if (!name) {
      throw new Error(`Variable line ${index + 1} must look like NAME=value`)
    }
    const value = text.slice(eq + 1)
    if (value === '' && dropEmpty) return
    out[name] = value
  })
  return out
}

/** Inverse of parseDomainLines, used to prefill the edit form. */
export function domainsToLines(domains: Record<string, string>): string {
  return Object.entries(domains).map(([service, host]) => `${service}=${host}`).join('\n')
}
