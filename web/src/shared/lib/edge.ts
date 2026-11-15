/** Public HTTPS URL for a domain behind the local/dev Caddy edge. */
export function domainPublicUrl(hostname: string, httpsUrl?: string): string {
  const host = (hostname || '').trim()
  if (!host) return ''
  if (httpsUrl && /^https:\/\//i.test(httpsUrl) && /:\d+/.test(httpsUrl)) {
    return httpsUrl
  }
  const port = import.meta.env.VITE_EDGE_HTTPS_PORT || '8443'
  if (port === '443') return `https://${host}`
  return `https://${host}:${port}`
}

export function domainHttpUrl(hostname: string): string {
  const host = (hostname || '').trim()
  if (!host) return ''
  const port = import.meta.env.VITE_EDGE_HTTP_PORT || '8088'
  if (port === '80') return `http://${host}`
  return `http://${host}:${port}`
}
