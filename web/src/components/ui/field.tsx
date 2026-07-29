import type {
  InputHTMLAttributes,
  LabelHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react'
import { forwardRef, useId } from 'react'

import { cn } from '@/lib/utils'

const CONTROL =
  'w-full rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg))] px-2.5 text-[13px] text-fg ' +
  'placeholder:text-subtle transition-colors focus:border-[hsl(var(--accent))] disabled:opacity-50'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...props }, ref) {
    return <input ref={ref} className={cn(CONTROL, 'h-8', className)} {...props} />
  },
)

export const Textarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement>
>(function Textarea({ className, ...props }, ref) {
  return (
    <textarea
      ref={ref}
      className={cn(CONTROL, 'py-2 leading-relaxed font-mono text-[12.5px] resize-y', className)}
      {...props}
    />
  )
})

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(
  function Select({ className, children, ...props }, ref) {
    return (
      <select ref={ref} className={cn(CONTROL, 'h-8 cursor-pointer pr-8', className)} {...props}>
        {children}
      </select>
    )
  },
)

export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn(
        'block text-[11px] font-semibold uppercase tracking-wide text-subtle',
        className,
      )}
      {...props}
    />
  )
}

export interface FieldProps {
  label: string
  hint?: ReactNode
  error?: string
  className?: string
  children: (id: string) => ReactNode
}

/** A labelled control with its hint and error wired up for screen readers. */
export function Field({ label, hint, error, className, children }: FieldProps) {
  const id = useId()
  return (
    <div className={cn('space-y-1.5', className)}>
      <Label htmlFor={id}>{label}</Label>
      {children(id)}
      {error ? (
        <p className="text-[11.5px] text-[hsl(var(--danger))]" role="alert">
          {error}
        </p>
      ) : hint ? (
        <p className="text-[11.5px] text-subtle">{hint}</p>
      ) : null}
    </div>
  )
}
