import type { SelectHTMLAttributes, ReactNode } from 'react'

interface SelectFieldProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: string
  children: ReactNode
}

export function SelectField({ label, id, children, className, ...props }: SelectFieldProps) {
  return (
    <label htmlFor={id} className={className}>
      {label}
      <select id={id} {...props}>
        {children}
      </select>
    </label>
  )
}
