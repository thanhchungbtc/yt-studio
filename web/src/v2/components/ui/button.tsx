import type { ReactNode } from 'react'

import { cn } from '../../core/utils'

interface ButtonProps {
  children: ReactNode
  onClick?: () => void
  /** The one action the strip exists for; at most one per row. */
  primary?: boolean
  disabled?: boolean
  className?: string
  type?: 'button' | 'submit'
  /**
   * The form this button submits, by id.
   *
   * A dialog renders its footer and its form into two subtrees, so the default
   * button is not inside the form it belongs to. This is the association HTML
   * provides for exactly that case — and it is what makes Return submit.
   */
  form?: string
}

/**
 * A macOS push button: small, rounded, and quiet unless it is the default.
 *
 * Two weights only. AppKit's own vocabulary is wider, but every extra weight is
 * a decision at each call site about how loud a thing should be, and the answer
 * is nearly always "this one, and not the others".
 */
export function Button({
  children,
  onClick,
  primary,
  disabled,
  className,
  type = 'button',
  form,
}: ButtonProps) {
  return (
    <button
      type={type}
      form={form}
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'flex h-[22px] shrink-0 items-center justify-center rounded-[5px] px-2.5',
        'text-[12px] font-medium whitespace-nowrap transition-[background-color,opacity]',
        'disabled:pointer-events-none disabled:opacity-40',
        primary ? 'text-white hover:brightness-105' : 'text-primary hover:bg-[var(--hover)]',
        className,
      )}
      style={
        primary
          ? { backgroundColor: 'var(--accent)' }
          : { backgroundColor: 'var(--raised)', boxShadow: '0 0 0 0.5px var(--separator-strong)' }
      }
    >
      {children}
    </button>
  )
}
