import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { Search, X } from 'lucide-react'
import type { HTMLAttributes, ReactNode, RefObject } from 'react'

import { TONE_FILL, type Tone } from './controls'
import { keycaps } from '../lib/keys'
import { cn } from '@/core/utils'

/* ---------------------------------------------------------------- tooltip */

export function TooltipProvider({ children }: { children: ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={350} skipDelayDuration={200}>
      {children}
    </TooltipPrimitive.Provider>
  )
}

export function Tooltip({
  label,
  keys,
  side,
  children,
}: {
  label: ReactNode
  /** An optional binding, drawn as keycaps after the label. */
  keys?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
  children: ReactNode
}) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          sideOffset={6}
          className="animate-in-fade z-50 flex max-w-xs items-center gap-2 rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] px-2 py-1 text-[11.5px] text-fg elev-2"
        >
          <span className="min-w-0">{label}</span>
          {keys && <Kbd keys={keys} />}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}

/** One keycap per part. Takes the same binding string the key layer does. */
export function Kbd({ keys, className }: { keys: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-[3px] no-select', className)}>
      {keycaps(keys).map((cap, i) => (
        <kbd
          key={i}
          className="inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-[var(--radius-xs)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] px-1 font-sans text-[10.5px] font-medium leading-none text-muted"
        >
          {cap}
        </kbd>
      ))}
    </span>
  )
}

/* ----------------------------------------------------------------- dialog */

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  wide,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="animate-in-fade fixed inset-0 z-40 bg-black/55 backdrop-blur-[2px]" />
        <DialogPrimitive.Content
          className={cn(
            'animate-in-pop fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-[calc(100vw-2rem)] -translate-x-1/2 -translate-y-1/2 flex-col',
            'rounded-[var(--radius-lg)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] elev-3',
            wide ? 'max-w-3xl' : 'max-w-lg',
          )}
        >
          <div className="flex items-start justify-between gap-4 border-b border-[hsl(var(--border))] px-4 py-3">
            <div className="space-y-0.5">
              <DialogPrimitive.Title className="text-[14px] font-semibold text-fg">
                {title}
              </DialogPrimitive.Title>
              {description && (
                <DialogPrimitive.Description className="text-[12px] text-muted">
                  {description}
                </DialogPrimitive.Description>
              )}
            </div>
            <DialogPrimitive.Close
              className="-mr-1 -mt-1 rounded-[var(--radius-xs)] p-1 text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </DialogPrimitive.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">{children}</div>
          {footer && (
            <div className="flex items-center justify-end gap-2 border-t border-[hsl(var(--border))] bg-subtle px-4 py-3">
              {footer}
            </div>
          )}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

/* --------------------------------------------------------------- progress */

export interface ProgressProps {
  value: number
  total: number
  failed?: number
  running?: boolean
  className?: string
  'aria-label'?: string
}

export function Progress({ value, total, failed = 0, running, className, ...rest }: ProgressProps) {
  const done = total > 0 ? (value / total) * 100 : 0
  const bad = total > 0 ? (failed / total) * 100 : 0
  return (
    <div
      className={cn(
        'relative h-1.5 w-full overflow-hidden rounded-full bg-[hsl(var(--bg-hover))]',
        className,
      )}
      role="progressbar"
      aria-valuenow={Math.round(done)}
      aria-valuemin={0}
      aria-valuemax={100}
      {...rest}
    >
      <div
        className={cn(
          'absolute inset-y-0 left-0 bg-[hsl(var(--accent))] transition-[width] duration-300',
          running && 'stripes',
        )}
        style={{ width: `${done}%` }}
      />
      {bad > 0 && (
        <div
          className="absolute inset-y-0 right-0 bg-[hsl(var(--danger))]"
          style={{ width: `${bad}%` }}
        />
      )}
    </div>
  )
}

/**
 * Progress where a bar would be too wide — a tree row, a tab. Draws failure as a
 * second arc, so a stalled render is visible at 14 pixels.
 */
export function Ring({
  value,
  total,
  failed = 0,
  size = 16,
  className,
  ...rest
}: ProgressProps & { size?: number }) {
  const radius = (size - 2.5) / 2
  const circumference = 2 * Math.PI * radius
  const done = total > 0 ? Math.min(1, value / total) : 0
  const bad = total > 0 ? Math.min(1, failed / total) : 0

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className={cn('shrink-0 -rotate-90', className)}
      role="progressbar"
      aria-valuenow={Math.round(done * 100)}
      aria-valuemin={0}
      aria-valuemax={100}
      {...rest}
    >
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        strokeWidth={2.5}
        className="stroke-[hsl(var(--bg-hover))]"
      />
      {bad > 0 && (
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          strokeWidth={2.5}
          strokeLinecap="round"
          className="stroke-[hsl(var(--danger))]"
          strokeDasharray={`${bad * circumference} ${circumference}`}
          strokeDashoffset={-done * circumference}
        />
      )}
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill="none"
        strokeWidth={2.5}
        strokeLinecap="round"
        className="stroke-[hsl(var(--accent))] transition-[stroke-dasharray] duration-300"
        strokeDasharray={`${done * circumference} ${circumference}`}
      />
    </svg>
  )
}

/* ----------------------------------------------------------------- filter */

export function SearchField({
  value,
  onChange,
  placeholder,
  inputRef,
  className,
  keys,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
  inputRef?: RefObject<HTMLInputElement | null>
  className?: string
  /** The binding that focuses this field, drawn inside it while it is empty. */
  keys?: string
}) {
  return (
    <div className={cn('relative flex min-w-0 items-center', className)}>
      <Search className="pointer-events-none absolute left-2 h-3.5 w-3.5 text-subtle" aria-hidden />
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key !== 'Escape') return
          event.stopPropagation()
          if (value) onChange('')
          else event.currentTarget.blur()
        }}
        placeholder={placeholder}
        aria-label={placeholder}
        spellCheck={false}
        autoComplete="off"
        className={cn(
          'h-7 w-full rounded-[var(--radius-sm)] border border-[hsl(var(--border))] bg-[hsl(var(--bg))]',
          'pl-7 pr-7 text-[12px] text-fg transition-colors placeholder:text-subtle',
          'hover:border-[hsl(var(--border-strong))] focus:border-[hsl(var(--accent))]',
        )}
      />
      {value ? (
        <button
          type="button"
          onClick={() => onChange('')}
          aria-label="Clear the filter"
          className="absolute right-1 flex h-5 w-5 items-center justify-center rounded-[var(--radius-xs)] text-subtle hover:bg-[hsl(var(--bg-hover))] hover:text-fg"
        >
          <X className="h-3 w-3" />
        </button>
      ) : (
        keys && <Kbd keys={keys} className="pointer-events-none absolute right-1.5 opacity-70" />
      )}
    </div>
  )
}

/**
 * A filter as a pill with its own count. The count is what makes a filter bar
 * worth having: it says how much is behind each one before it is clicked.
 */
export function FilterChip({
  label,
  count,
  tone = 'neutral',
  selected,
  onClick,
}: {
  label: ReactNode
  count?: number
  tone?: Tone
  selected: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={selected}
      className={cn(
        'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-[1px] text-[11px] font-medium leading-[18px] transition-colors',
        selected
          ? 'border-[hsl(var(--accent))] bg-[hsl(var(--accent))] text-[hsl(var(--accent-fg))]'
          : 'border-[hsl(var(--border))] bg-[hsl(var(--bg-elevated))] text-muted hover:border-[hsl(var(--border-strong))] hover:text-fg',
      )}
    >
      {!selected && tone !== 'neutral' && (
        <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', TONE_FILL[tone])} aria-hidden />
      )}
      {label}
      {count !== undefined && (
        <span className={cn('tabular', selected ? 'opacity-80' : 'text-subtle')}>{count}</span>
      )}
    </button>
  )
}

/* --------------------------------------------------------------- feedback */

export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('sweep rounded-[var(--radius-sm)] bg-[hsl(var(--bg-hover))]', className)}
      {...props}
    />
  )
}

export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-2 px-6 py-14 text-center',
        className,
      )}
    >
      {icon && (
        <div className="mb-1 flex h-11 w-11 items-center justify-center rounded-full bg-[hsl(var(--bg-hover))] text-subtle [&>svg]:h-5 [&>svg]:w-5">
          {icon}
        </div>
      )}
      <p className="text-[13px] font-medium text-fg">{title}</p>
      {description && <p className="max-w-sm text-[12px] text-muted">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}

export function ErrorNotice({ error, className }: { error: unknown; className?: string }) {
  const message = error instanceof Error ? error.message : String(error)
  return (
    <div
      role="alert"
      className={cn(
        'rounded-[var(--radius-sm)] border border-[hsl(var(--danger)/0.4)] bg-[hsl(var(--danger-soft))] px-3 py-2 text-[12px] text-[hsl(var(--danger))]',
        className,
      )}
    >
      {message}
    </div>
  )
}

/* ------------------------------------------------------------------- misc */

export function KeyValue({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1">
      <dt className="shrink-0 text-[11.5px] text-subtle">{label}</dt>
      <dd className="tabular min-w-0 truncate text-right text-[12px] text-fg">{children}</dd>
    </div>
  )
}

export function Mono({ className, ...props }: HTMLAttributes<HTMLSpanElement>) {
  return <span className={cn('font-mono text-[11.5px]', className)} {...props} />
}

/** A hairline between groups of controls. */
export function Divider({ className }: { className?: string }) {
  return <span aria-hidden className={cn('h-4 w-px bg-[hsl(var(--border))]', className)} />
}

/** A titled block inside a document body. */
export function Section({
  title,
  actions,
  children,
}: {
  title: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="space-y-2">
      <div className="flex items-center gap-2">
        <h3 className="text-[10.5px] font-semibold uppercase tracking-[0.08em] text-subtle">
          {title}
        </h3>
        {actions && <div className="ml-auto flex items-center gap-1">{actions}</div>}
      </div>
      {children}
    </section>
  )
}

/** A side panel's title row. Fixed height, so both sidebars start on one line. */
export function PaneHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="flex h-9 shrink-0 items-center gap-1.5 border-b border-[hsl(var(--border))] px-2.5 no-select">
      <h2 className="truncate text-[10.5px] font-semibold uppercase tracking-[0.08em] text-subtle">
        {title}
      </h2>
      {children && <div className="ml-auto flex shrink-0 items-center gap-0.5">{children}</div>}
    </div>
  )
}
