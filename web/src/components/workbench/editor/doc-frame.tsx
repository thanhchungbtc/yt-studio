import { ChevronDown, ChevronRight } from 'lucide-react'
import { Fragment, type ReactNode } from 'react'

import { Dropdown, DropdownItem } from '../ui/menu'
import { cn } from '@/core/utils'

export interface DocView {
  id: string
  label: string
  count?: number
}

/**
 * The frame every document draws itself inside: a breadcrumb row, then the body.
 *
 * There is no document toolbar any more. A tab bar plus a toolbar plus a tab
 * strip was three rows of chrome above the content, and the reference manages
 * with two — so the sections that used to be a segmented control are the
 * breadcrumb's last segment, and the actions that used to sit in a toolbar are
 * on the right of the same row.
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
  /** The leading segments. The last one is drawn as the document's own name. */
  crumbs: ReactNode[]
  /** Sections of this document, offered as the final segment's dropdown. */
  views?: DocView[]
  activeView?: string
  onSelectView?: (id: string) => void
  actions?: ReactNode
  children: ReactNode
}) {
  const current = views?.find((view) => view.id === activeView) ?? views?.[0]

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-[26px] shrink-0 items-center gap-1 border-b border-[hsl(var(--border))] px-3 no-select">
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-hidden">
          {crumbs.map((crumb, index) => (
            <Fragment key={index}>
              {index > 0 && <ChevronRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />}
              <span
                className={cn(
                  'truncate text-[11.5px]',
                  index === crumbs.length - 1 && !views ? 'text-fg' : 'text-muted',
                )}
              >
                {crumb}
              </span>
            </Fragment>
          ))}

          {views && views.length > 0 && current && (
            <>
              <ChevronRight className="h-3 w-3 shrink-0 text-subtle" aria-hidden />
              <Dropdown
                align="start"
                trigger={
                  <button
                    type="button"
                    className="flex shrink-0 items-center gap-1 rounded-[var(--radius-xs)] px-1 py-0.5 text-[11.5px] text-fg transition-colors hover:bg-[hsl(var(--bg-hover))]"
                  >
                    {current.label}
                    {current.count !== undefined && (
                      <span className="tabular text-subtle">{current.count}</span>
                    )}
                    <ChevronDown className="h-3 w-3 text-subtle" aria-hidden />
                  </button>
                }
                items={views.map((view) => (
                  <DropdownItem
                    key={view.id}
                    selected={view.id === current.id}
                    onSelect={() => onSelectView?.(view.id)}
                  >
                    <span className="flex-1">{view.label}</span>
                    {view.count !== undefined && (
                      <span className="tabular text-subtle">{view.count}</span>
                    )}
                  </DropdownItem>
                ))}
              />
            </>
          )}
        </div>

        {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
    </div>
  )
}
