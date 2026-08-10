import { ChevronRight } from 'lucide-react'
import { Fragment, type ReactNode } from 'react'

import { cn } from '@/core/utils'

export interface DocView {
  id: string
  label: string
  count?: number
}

/**
 * The frame every document draws itself inside: one 30px row, then the body.
 *
 * There is no document toolbar. A tab bar plus a toolbar plus a tab strip was
 * three rows of chrome above the content; this row carries all three jobs —
 * where you are, which section, and what you can do to it.
 *
 * The sections were briefly a dropdown, on the theory that the reference puts
 * navigation in its breadcrumb. That was wrong: a breadcrumb dropdown is for
 * jumping *within* a document you are reading, and these are the document's
 * primary views, switched constantly. Anything clicked that often should cost
 * one click, so they are tabs.
 *
 * A document renders this itself rather than publishing its chrome upward
 * through a context: the group would then need an effect to receive it, and an
 * effect that runs after paint is how a breadcrumb ends up briefly describing
 * the document you just closed.
 */
export function DocFrame({
  crumbs,
  views,
  activeView,
  onSelectView,
  actions,
  children,
}: {
  /** The leading segments; the last is the document's own name. */
  crumbs: ReactNode[]
  /** This document's sections, drawn as tabs. */
  views?: DocView[]
  activeView?: string
  onSelectView?: (id: string) => void
  actions?: ReactNode
  children: ReactNode
}) {
  const current = views?.find((view) => view.id === activeView) ?? views?.[0]

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-[30px] shrink-0 items-stretch gap-2 border-b border-[hsl(var(--border))] px-3 no-select">
        {/* The breadcrumb is the only part that yields: at half width in a split
            it truncates towards the ref, and the tabs and actions stay whole. */}
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-hidden">
          {crumbs.map((crumb, index) => (
            <Fragment key={index}>
              {index > 0 && <ChevronRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />}
              <span
                className={cn(
                  'truncate text-[11.5px]',
                  index === crumbs.length - 1 ? 'text-fg' : 'shrink-0 text-muted',
                )}
              >
                {crumb}
              </span>
            </Fragment>
          ))}
        </div>

        {views && views.length > 0 && current && (
          <div role="tablist" aria-label="Sections" className="flex shrink-0 items-stretch">
            {views.map((view) => {
              const selected = view.id === current.id
              return (
                <button
                  key={view.id}
                  type="button"
                  role="tab"
                  aria-selected={selected}
                  onClick={() => onSelectView?.(view.id)}
                  className={cn(
                    'relative flex items-center gap-1.5 px-2.5 text-[11.5px] transition-colors',
                    selected ? 'font-medium text-fg' : 'text-muted hover:text-fg',
                  )}
                >
                  {view.label}
                  {view.count !== undefined && (
                    <span
                      className={cn(
                        'tabular rounded-full px-1 text-[10px] leading-[15px] transition-colors',
                        selected
                          ? 'bg-[hsl(var(--accent)/0.16)] text-[hsl(var(--accent))]'
                          : 'bg-[hsl(var(--fg)/0.07)] text-subtle',
                      )}
                    >
                      {view.count}
                    </span>
                  )}
                  {selected && (
                    <span
                      aria-hidden
                      className="absolute inset-x-2 bottom-0 h-[2px] rounded-t-full bg-[hsl(var(--accent))]"
                    />
                  )}
                </button>
              )
            })}
          </div>
        )}

        {actions && (
          <div className="flex shrink-0 items-center gap-1.5">
            {views && views.length > 0 && (
              <span aria-hidden className="mr-0.5 h-3.5 w-px bg-[hsl(var(--border))]" />
            )}
            {actions}
          </div>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
    </div>
  )
}
