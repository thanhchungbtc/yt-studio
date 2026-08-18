import { Slot } from '@radix-ui/react-slot'
import type {
  ButtonHTMLAttributes,
  HTMLAttributes,
  InputHTMLAttributes,
  LabelHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react'
import { forwardRef, useId } from 'react'

import { cn } from '@/core/utils'

/**
 * The workbench's own controls. Copied from the shell's kit rather than shared
 * with it, so this UI can move without dragging the one it replaces along —
 * and so deleting that one is deleting a directory.
 */

/* ----------------------------------------------------------------- button */

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

/**
 * A square icon button at chrome scale. The kit's `size="icon"` is 28px, which
 * is a row too tall for a 36px panel header or a 35px tab strip.
 */
export const IconButton = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { active?: boolean }
>(function IconButton({ className, active, ...props }, ref) {
  return (
    <button
      ref={ref}
      type="button"
      aria-pressed={active}
      className={cn(
        'flex h-6 w-6 shrink-0 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
        'disabled:pointer-events-none disabled:opacity-40',
        active
          ? 'bg-[hsl(var(--bg-active))] text-fg'
          : 'text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg',
        className,
      )}
      {...props}
    />
  )
})

/* ------------------------------------------------------------------ badge */

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

/**
 * The same tones as a flat fill, for the places that want the colour without
 * the pill. Written out per tone because Tailwind only emits classes it can see
 * as literals.
 */
export const TONE_FILL: Record<Tone, string> = {
  neutral: 'bg-[hsl(var(--fg-subtle))]',
  accent: 'bg-[hsl(var(--accent))]',
  success: 'bg-[hsl(var(--success))]',
  warning: 'bg-[hsl(var(--warning))]',
  danger: 'bg-[hsl(var(--danger))]',
  info: 'bg-[hsl(var(--info))]',
  violet: 'bg-[hsl(var(--violet))]',
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
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2 py-[1px] text-[11px] font-medium leading-[18px]',
        TONES[tone],
        className,
      )}
      {...props}
    >
      {dot && (
        <span
          aria-hidden
          className={cn(
            'h-1.5 w-1.5 shrink-0 rounded-full',
            TONE_FILL[tone],
            pulse && 'pulse-live',
          )}
        />
      )}
      {children}
    </span>
  )
}

/* ------------------------------------------------------------------ field */

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
      className={cn(CONTROL, 'resize-y py-2 font-mono text-[12.5px] leading-relaxed', className)}
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

/** A labelled control with its hint and error wired up for screen readers. */
export function Field({
  label,
  hint,
  error,
  className,
  children,
}: {
  label: string
  hint?: ReactNode
  error?: string
  className?: string
  children: (id: string) => ReactNode
}) {
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

/* -------------------------------------------------------------- segmented */

export interface SegmentedOption {
  value: string
  label: string
  /** Native tooltip, for anything the label alone cannot say. */
  title?: string
}

/**
 * A closed shortlist, laid out rather than folded away.
 *
 * It is the counterpart to Select, and the reason that one is now rarely right
 * here: a dropdown earns its click when the list is long or unbounded, and
 * costs one for nothing when the list is three registry names that would have
 * fit on the row. Same job, opposite trade.
 *
 * The selected segment is lifted rather than coloured — a raised plate on a
 * recessed track, the way bg-active marks a pressed IconButton — because the
 * accent in this kit means "the thing you are about to do", and a setting that
 * merely is what it is should not speak in the same voice as a primary button.
 */
export function Segmented({
  value,
  options,
  onChange,
  size = 'md',
  disabled,
  className,
  'aria-label': ariaLabel,
}: {
  value: string
  options: SegmentedOption[]
  onChange: (value: string) => void
  size?: 'sm' | 'md'
  disabled?: boolean
  className?: string
  'aria-label'?: string
}) {
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={cn(
        'inline-flex flex-wrap items-center gap-0.5 rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-subtle p-0.5 no-select',
        className,
      )}
    >
      {options.map((option) => {
        const on = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={on}
            disabled={disabled}
            title={option.title}
            onClick={() => {
              if (option.value !== value) onChange(option.value)
            }}
            className={cn(
              'min-w-0 truncate rounded-[var(--radius-xs)] font-medium transition-colors duration-100',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring)/0.45)]',
              'disabled:pointer-events-none',
              size === 'sm' ? 'h-[22px] px-2 text-[11px]' : 'h-6 px-2.5 text-[11.5px]',
              on ? 'bg-[hsl(var(--bg-elevated))] text-fg elev-1' : 'text-muted hover:text-fg',
            )}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

/**
 * A boolean as a switch. Hand-rolled on a `role="switch"` button: this is the
 * whole of the behaviour, and a dependency for it would be larger than it is.
 */
export function Switch({
  checked,
  onCheckedChange,
  disabled,
  id,
  'aria-label': ariaLabel,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  disabled?: boolean
  id?: string
  'aria-label'?: string
}) {
  return (
    <button
      type="button"
      role="switch"
      id={id}
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        'relative inline-flex h-[20px] w-[34px] shrink-0 items-center rounded-full border no-select',
        'transition-colors duration-150 disabled:pointer-events-none disabled:opacity-45',
        checked
          ? 'border-transparent bg-[hsl(var(--accent))]'
          : 'border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-hover))]',
      )}
    >
      <span
        aria-hidden
        className={cn(
          'block h-[14px] w-[14px] rounded-full transition-transform duration-150 ease-out elev-1',
          checked
            ? 'translate-x-[17px] bg-[hsl(var(--accent-fg))]'
            : 'translate-x-[3px] bg-[hsl(var(--bg-elevated))]',
        )}
      />
    </button>
  )
}
