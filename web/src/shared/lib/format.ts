export function formatWhen(value: string | undefined) {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

export function shortId(id: string, size = 8) {
  if (!id) return '—'
  return id.length <= size ? id : `${id.slice(0, size)}…`
}

export function relativeAge(value: string | undefined) {
  if (!value) return 'never'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const delta = Date.now() - date.getTime()
  if (delta < 5_000) return 'just now'
  if (delta < 60_000) return `${Math.floor(delta / 1000)}s ago`
  if (delta < 3_600_000) return `${Math.floor(delta / 60_000)}m ago`
  if (delta < 86_400_000) return `${Math.floor(delta / 3_600_000)}h ago`
  return formatWhen(value)
}

export function workloadHint(status: string, observed: bigint | number, desired: bigint | number, hasOnlineNode: boolean) {
  const obs = Number(observed)
  const des = Number(desired)
  if (!hasOnlineNode && (status === 'deploying' || des > obs)) {
    return 'Waiting for an online agent heartbeat to reconcile this deploy.'
  }
  if (status === 'deploying' && des > obs) {
    return 'Desired state queued — agent should pull/run on the next heartbeat (~10s).'
  }
  if (status === 'running' && des === obs) {
    return 'Observed generation matches desired. Container should be reachable on the host port.'
  }
  if (status === 'failed') {
    return 'Agent reported failure. Check last error and logs, then redeploy.'
  }
  if (status === 'pending') {
    return 'Created only. Click Deploy to push desired state to a node.'
  }
  if (status === 'stopped') {
    return 'Workload is stopped.'
  }
  return 'Status updates when the agent reports observed state.'
}
