export type DeployStepId = 'queued' | 'building' | 'scheduled' | 'running' | 'routed'

export interface DeployStep {
  id: DeployStepId
  label: string
}

const STEPS: DeployStep[] = [
  { id: 'queued', label: 'Queued' },
  { id: 'building', label: 'Building' },
  { id: 'scheduled', label: 'Scheduled' },
  { id: 'running', label: 'Running' },
  { id: 'routed', label: 'Routed' },
]

function activeIndex(status: string, hasEndpoint: boolean, sourceKind: string): number {
  switch (status) {
    case 'pending':
      return 0
    case 'building':
      return 1
    case 'deploying':
      return 2
    case 'running':
      return hasEndpoint ? 4 : 3
    case 'failed':
      // Approximate failing step from last known phase.
      if (sourceKind === 'git') return 1
      return 2
    case 'stopped':
      return 0
    default:
      return 0
  }
}

interface DeployProgressStepsProps {
  status: string
  sourceKind?: string
  endpoint?: string
  lastError?: string
}

export function DeployProgressSteps({
  status,
  sourceKind = 'image',
  endpoint = '',
  lastError = '',
}: DeployProgressStepsProps) {
  const current = activeIndex(status, !!endpoint, sourceKind)
  const failed = status === 'failed'

  return (
    <div className="deploy-progress" aria-label="Deploy progress">
      <ol className="deploy-progress-steps">
        {STEPS.map((step, index) => {
          let state: 'done' | 'current' | 'todo' | 'fail' = 'todo'
          if (failed && index === current) state = 'fail'
          else if (index < current) state = 'done'
          else if (index === current) state = 'current'
          return (
            <li key={step.id} className={`deploy-progress-step is-${state}`}>
              <span className="deploy-progress-dot" aria-hidden />
              <span className="deploy-progress-label">{step.label}</span>
            </li>
          )
        })}
      </ol>
      {failed && lastError ? (
        <p className="deploy-progress-fail muted" role="status">
          Failed at <strong>{STEPS[current]?.label ?? 'step'}</strong>: {lastError}
        </p>
      ) : null}
    </div>
  )
}
