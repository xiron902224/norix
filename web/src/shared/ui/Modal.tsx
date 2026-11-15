import type { FormEvent, ReactNode } from 'react'
import { X } from 'lucide-react'
import { Button } from './Button'

export function Modal({
  open,
  title,
  children,
  onClose,
  footer,
}: {
  open: boolean
  title: string
  children: ReactNode
  onClose: () => void
  footer?: ReactNode
}) {
  if (!open) return null
  return (
    <div className="compass-modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="compass-modal"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="compass-modal-header">
          <h3>{title}</h3>
          <button type="button" className="icon-button" aria-label="Close" onClick={onClose}>
            <X size={16} />
          </button>
        </header>
        <div className="compass-modal-body">{children}</div>
        {footer ? <footer className="compass-modal-footer">{footer}</footer> : null}
      </div>
    </div>
  )
}

export function CreateModal({
  open,
  title,
  submitLabel,
  busy,
  onClose,
  onSubmit,
  children,
}: {
  open: boolean
  title: string
  submitLabel: string
  busy?: boolean
  onClose: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  children: ReactNode
}) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onClose}
      footer={null}
    >
      <form className="compass-create-form" onSubmit={onSubmit}>
        {children}
        <div className="compass-modal-footer" style={{ padding: 0, border: 0 }}>
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={busy}>{busy ? 'Saving…' : submitLabel}</Button>
        </div>
      </form>
    </Modal>
  )
}
