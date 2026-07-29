import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { X } from 'lucide-react'
import type { HTMLAttributes, ReactNode } from 'react'

import { keycaps } from '@/lib/hotkeys'
import { cn } from '@/lib/utils'

/* --------------------------------------------------------------- surfaces */

export function Panel({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('surface', className)} {...props} />
}

export function PanelHeader({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex items-center justify-between gap-3 border-b border-[hsl(var(--border))] px-3 py-2',
        className,
      )}
      {...props}
    />
  )
}

export function PanelTitle({ className, ...props }: HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h2
      className={cn('text-[11px] font-semibold uppercase tracking-wider text-subtle', className)}
      {...props}
    />
  )
}

/**
 * The strip of controls above a pane. Every view has exactly one, at a fixed
 * height, so switching views never shifts the content beneath it.
 */
export function Toolbar({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'flex h-11 shrink-0 items-center gap-2 border-b border-[hsl(var(--border))] bg-subtle px-3',
        className,
      )}
      {...props}
    />
  )
}

/* -------------------------------------------------------------- progress */

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
 * A progress ring, for places where a bar would be too wide — a list row, a
 * toolbar. Draws failure as a second arc so a stalled render is visible at
 * 16 pixels.
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

/* -------------------------------------------------------------- controls */

export interface SegmentedOption<T extends string> {
  value: T
  label: ReactNode
  count?: number
}

/**
 * A segmented control: the tab metaphor for a pane that swaps its whole body.
 * The moving indicator is a single absolutely positioned element rather than a
 * per-item border, so it slides rather than jumps.
 */
export function Segmented<T extends string>({
  options,
  value,
  onChange,
  className,
  'aria-label': ariaLabel,
}: {
  options: SegmentedOption<T>[]
  value: T
  onChange: (value: T) => void
  className?: string
  'aria-label'?: string
}) {
  const index = Math.max(
    0,
    options.findIndex((option) => option.value === value),
  )

  return (
    <div
      role="tablist"
      aria-label={ariaLabel}
      className={cn(
        'relative inline-flex h-7 items-center rounded-[var(--radius-sm)] bg-[hsl(var(--bg-hover))] p-[2px] no-select',
        className,
      )}
    >
      <span
        aria-hidden
        className="absolute inset-y-[2px] rounded-[var(--radius-xs)] bg-[hsl(var(--bg-elevated))] elev-1 transition-[left,width] duration-150 ease-out"
        style={{
          width: `calc((100% - 4px) / ${options.length})`,
          left: `calc(2px + (100% - 4px) * ${index} / ${options.length})`,
        }}
      />
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          role="tab"
          aria-selected={option.value === value}
          onClick={() => onChange(option.value)}
          className={cn(
            'relative z-10 flex flex-1 items-center justify-center gap-1.5 whitespace-nowrap px-2.5 text-[12px] transition-colors',
            option.value === value ? 'font-medium text-fg' : 'text-muted hover:text-fg',
          )}
        >
          {option.label}
          {option.count !== undefined && (
            <span
              className={cn(
                'tabular rounded-full px-1 text-[10px] leading-[15px]',
                option.value === value
                  ? 'bg-[hsl(var(--accent)/0.15)] text-[hsl(var(--accent))]'
                  : 'bg-[hsl(var(--fg)/0.08)] text-subtle',
              )}
            >
              {option.count}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}

/** One keycap. `Kbd` takes the same binding string the hotkey layer does. */
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

/* -------------------------------------------------------------- feedback */

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

/* --------------------------------------------------------------- tooltip */

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
  /** An optional shortcut, drawn as keycaps after the label. */
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

/* ---------------------------------------------------------------- dialog */

export interface ModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  wide,
}: ModalProps) {
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

/* ------------------------------------------------------------------ misc */

export function KeyValue({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1">
      <dt className="shrink-0 text-[11.5px] text-subtle">{label}</dt>
      <dd className="min-w-0 truncate text-right text-[12px] text-fg tabular">{children}</dd>
    </div>
  )
}

export function Mono({ className, ...props }: HTMLAttributes<HTMLSpanElement>) {
  return <span className={cn('font-mono text-[11.5px]', className)} {...props} />
}

/** A hairline between toolbar groups. */
export function Divider({ className }: { className?: string }) {
  return <span aria-hidden className={cn('h-4 w-px bg-[hsl(var(--border))]', className)} />
}
