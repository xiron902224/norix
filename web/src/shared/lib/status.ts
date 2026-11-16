/** Public control-plane status (`GET /status` or `/api/status`). */
export type ControlPlaneStatus = {
  status?: string
  version?: string
  public_url?: string
}

/**
 * Fetch CP status. Tries `/status` then `/api/status` so Vite (proxied) and
 * embedded UI both work. Ignores non-JSON responses (SPA HTML fallback).
 */
export async function fetchControlPlaneStatus(): Promise<ControlPlaneStatus | null> {
  for (const path of ['/status', '/api/status'] as const) {
    try {
      const response = await fetch(path)
      if (!response.ok) continue
      const contentType = response.headers.get('content-type') || ''
      if (!contentType.includes('json')) continue
      return (await response.json()) as ControlPlaneStatus
    } catch {
      /* try next */
    }
  }
  return null
}
