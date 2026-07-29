import { Slot } from '@radix-ui/react-slot'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { forwardRef } from 'react'

import { cn } from '@/lib/utils'

type Variant = 'default' | 'primary' | 'ghost' | 'danger' | 'success' | 'outline'
type Size = 'xs' | 'sm' | 'md' | 'icon'

const VARIANTS: Record<Variant, string> = {
  default:
    'bg-[hsl(var(--bg-elevated))] text-fg border border-[hsl(var(--border-strong))] hover:bg-[hsl(var(--bg-hover))]',
  primary:
    'bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))] border border-transparent hover:brightness-110',
  ghost:
    'bg-transparent text-muted border border-transparent hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
  danger: 'bg-[hsl(var(--danger))] text-white border border-transparent hover:brightness-110',
  success: 'bg-[hsl(var(--success))] text-white border border-transparent hover:brightness-110',
  outline:
    'bg-transparent text-fg border border-[hsl(var(--border-strong))] hover:bg-[hsl(var(--bg-hover))]',
}

const SIZES: Record<Size, string> = {
  xs: 'h-6 px-2 text-[11px] gap-1 rounded-[var(--radius-xs)]',
  sm: 'h-7 px-2.5 text-[12px] gap-1.5 rounded-[var(--radius-sm)]',
  md: 'h-8 px-3 text-[13px] gap-2 rounded-[var(--radius-sm)]',
  icon: 'h-7 w-7 justify-center rounded-[var(--radius-sm)]',
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  asChild?: boolean
  children?: ReactNode
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { className, variant = 'default', size = 'md', asChild, ...props },
  ref,
) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      ref={ref}
      className={cn(
        'inline-flex select-none items-center whitespace-nowrap font-medium transition-[background-color,border-color,filter] duration-100',
        'disabled:pointer-events-none disabled:opacity-45',
        VARIANTS[variant],
        SIZES[size],
        className,
      )}
      {...props}
    />
  )
})
