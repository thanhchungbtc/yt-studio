import type { HTMLAttributes } from 'react'

import { cn } from '@/lib/utils'

export type Tone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger' | 'info' | 'violet'

const TONES: Record<Tone, string> = {
  neutral: 'bg-[hsl(var(--bg-hover))] text-muted border-[hsl(var(--border))]',
  accent: 'bg-[hsl(var(--accent-soft))] text-[hsl(var(--accent))] border-[hsl(var(--accent)/0.35)]',
  success:
    'bg-[hsl(var(--success-soft))] text-[hsl(var(--success))] border-[hsl(var(--success)/0.35)]',
  warning:
    'bg-[hsl(var(--warning-soft))] text-[hsl(var(--warning))] border-[hsl(var(--warning)/0.35)]',
  danger: 'bg-[hsl(var(--danger-soft))] text-[hsl(var(--danger))] border-[hsl(var(--danger)/0.35)]',
  info: 'bg-[hsl(var(--info-soft))] text-[hsl(var(--info))] border-[hsl(var(--info)/0.35)]',
  violet: 'bg-[hsl(var(--violet-soft))] text-[hsl(var(--violet))] border-[hsl(var(--violet)/0.35)]',
}

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: Tone
  dot?: boolean
  pulse?: boolean
}

export function Badge({ className, tone = 'neutral', dot, pulse, children, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-[1px] text-[11px] font-medium leading-[18px] whitespace-nowrap',
        TONES[tone],
        className,
      )}
      {...props}
    >
      {dot && (
        <span
          className={cn('h-1.5 w-1.5 shrink-0 rounded-full bg-current', pulse && 'pulse-live')}
          aria-hidden
        />
      )}
      {children}
    </span>
  )
}
