import type { InputHTMLAttributes } from 'react'

interface FormFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
}

export function FormField({ label, id, ...props }: FormFieldProps) {
  return (
    <label htmlFor={id}>
      {label}
      <input id={id} {...props} />
    </label>
  )
}
