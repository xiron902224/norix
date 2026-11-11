import { LoaderCircle } from 'lucide-react'

interface ActivityBannerProps {
  title: string
  detail?: string
  tone?: 'info' | 'warn' | 'ok'
}

export function ActivityBanner({ title, detail, tone = 'info' }: ActivityBannerProps) {
  return (
    <div className={`activity-banner activity-${tone}`} role="status" aria-live="polite">
      <LoaderCircle className="spin" size={16} aria-hidden />
      <div>
        <strong>{title}</strong>
        {detail ? <p>{detail}</p> : null}
      </div>
    </div>
  )
}
