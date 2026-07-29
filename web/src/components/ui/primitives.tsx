import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { X } from 'lucide-react'
import type { HTMLAttributes, ReactNode } from 'react'

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

/* -------------------------------------------------------------- feedback */

export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'animate-pulse rounded-[var(--radius-sm)] bg-[hsl(var(--bg-hover))]',
        className,
      )}
      {...props}
    />
  )
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-14 text-center">
      {icon && <div className="text-subtle [&>svg]:h-7 [&>svg]:w-7">{icon}</div>}
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
    <TooltipPrimitive.Provider delayDuration={250} skipDelayDuration={120}>
      {children}
    </TooltipPrimitive.Provider>
  )
}

export function Tooltip({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          sideOffset={6}
          className="z-50 max-w-xs rounded-[var(--radius-sm)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] px-2 py-1 text-[11.5px] text-fg shadow-lg"
        >
          {label}
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
        <DialogPrimitive.Overlay className="fixed inset-0 z-40 bg-black/55 backdrop-blur-[1px]" />
        <DialogPrimitive.Content
          className={cn(
            'fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-[calc(100vw-2rem)] -translate-x-1/2 -translate-y-1/2 flex-col',
            'rounded-[var(--radius-lg)] border border-[hsl(var(--border-strong))] bg-[hsl(var(--bg-elevated))] shadow-2xl',
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
            <div className="flex items-center justify-end gap-2 border-t border-[hsl(var(--border))] px-4 py-3">
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
