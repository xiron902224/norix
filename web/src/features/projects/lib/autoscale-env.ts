/** Per-workload HPA knobs stored as env vars (#49). Control plane reads the same keys. */

export const AUTOSCALE_ENV_KEYS = [
  'NORIX_AUTOSCALE',
  'AUTOSCALE',
  'NORIX_AUTOSCALE_MIN',
  'AUTOSCALE_MIN',
  'NORIX_AUTOSCALE_MAX',
  'AUTOSCALE_MAX',
  'NORIX_AUTOSCALE_CPU_TARGET',
  'AUTOSCALE_CPU_TARGET',
  'NORIX_AUTOSCALE_MEM_TARGET',
  'AUTOSCALE_MEM_TARGET',
  'NORIX_AUTOSCALE_MEMORY_TARGET',
  'AUTOSCALE_MEMORY_TARGET',
  'NORIX_AUTOSCALE_RPS_TARGET',
  'AUTOSCALE_RPS_TARGET',
  'NORIX_AUTOSCALE_COOLDOWN_MS',
  'AUTOSCALE_COOLDOWN_MS',
] as const

export type AutoscaleForm = {
  enabled: boolean
  minReplicas: number
  maxReplicas: number
  cpuTarget: number
  memTarget: number
  rpsTarget: number
  cooldownMs: number
}

export const defaultAutoscaleForm = (): AutoscaleForm => ({
  enabled: false,
  minReplicas: 1,
  maxReplicas: 4,
  cpuTarget: 70,
  memTarget: 0,
  rpsTarget: 0,
  cooldownMs: 30_000,
})

function pick(env: Record<string, string>, ...keys: string[]): string {
  for (const key of keys) {
    const value = (env[key] || '').trim()
    if (value) return value
  }
  return ''
}

function parseNum(raw: string, fallback: number): number {
  if (!raw) return fallback
  const n = Number(raw)
  return Number.isFinite(n) ? n : fallback
}

export function parseAutoscaleForm(env: Record<string, string> | undefined): AutoscaleForm {
  const base = defaultAutoscaleForm()
  if (!env) return base
  const flag = pick(env, 'NORIX_AUTOSCALE', 'AUTOSCALE').toLowerCase()
  base.enabled = flag === '1' || flag === 'true' || flag === 'yes' || flag === 'on'
  base.minReplicas = Math.max(1, parseNum(pick(env, 'NORIX_AUTOSCALE_MIN', 'AUTOSCALE_MIN'), base.minReplicas))
  base.maxReplicas = Math.max(base.minReplicas, parseNum(pick(env, 'NORIX_AUTOSCALE_MAX', 'AUTOSCALE_MAX'), base.maxReplicas))
  base.cpuTarget = parseNum(pick(env, 'NORIX_AUTOSCALE_CPU_TARGET', 'AUTOSCALE_CPU_TARGET'), base.cpuTarget)
  base.memTarget = parseNum(
    pick(env, 'NORIX_AUTOSCALE_MEM_TARGET', 'AUTOSCALE_MEM_TARGET', 'NORIX_AUTOSCALE_MEMORY_TARGET', 'AUTOSCALE_MEMORY_TARGET'),
    base.memTarget,
  )
  base.rpsTarget = parseNum(pick(env, 'NORIX_AUTOSCALE_RPS_TARGET', 'AUTOSCALE_RPS_TARGET'), base.rpsTarget)
  base.cooldownMs = Math.max(0, parseNum(pick(env, 'NORIX_AUTOSCALE_COOLDOWN_MS', 'AUTOSCALE_COOLDOWN_MS'), base.cooldownMs))
  return base
}

/** Remove legacy/alt keys, then write canonical NORIX_AUTOSCALE_* entries. */
export function applyAutoscaleToEnv(env: Record<string, string>, form: AutoscaleForm): Record<string, string> {
  const next = { ...env }
  for (const key of AUTOSCALE_ENV_KEYS) {
    delete next[key]
  }
  if (!form.enabled) {
    next.NORIX_AUTOSCALE = '0'
    return next
  }
  next.NORIX_AUTOSCALE = '1'
  next.NORIX_AUTOSCALE_MIN = String(Math.max(1, Math.floor(form.minReplicas)))
  next.NORIX_AUTOSCALE_MAX = String(Math.max(form.minReplicas, Math.floor(form.maxReplicas)))
  if (form.cpuTarget > 0) next.NORIX_AUTOSCALE_CPU_TARGET = String(form.cpuTarget)
  if (form.memTarget > 0) next.NORIX_AUTOSCALE_MEM_TARGET = String(form.memTarget)
  if (form.rpsTarget > 0) next.NORIX_AUTOSCALE_RPS_TARGET = String(form.rpsTarget)
  next.NORIX_AUTOSCALE_COOLDOWN_MS = String(Math.max(0, Math.floor(form.cooldownMs)))
  return next
}

export function stripAutoscaleKeys(env: Record<string, string>): Record<string, string> {
  const next = { ...env }
  for (const key of AUTOSCALE_ENV_KEYS) {
    delete next[key]
  }
  return next
}
