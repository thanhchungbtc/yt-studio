import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { Avatar } from '../ui/avatar'
import { DragRegion } from '../ui/drag-region'

interface EditorTitleBarProps {
  title: string
  /** The channel token, so the strip matches the tab and the row above it. */
  seed?: string
  initial?: string
  icon?: LucideIcon
  /** The state dot and the line beside it: what this document is doing. */
  status?: ReactNode
  statusColor?: string
  actions?: ReactNode
}

/**
 * The strip a document wears above itself: what this is, and how it is doing.
 *
 * Translucent and outside the document's own surface, because it is chrome —
 * which is also why it is a drag region, and why the window can be picked up
 * from the widest piece of it on screen.
 */
export function EditorTitleBar({
  title,
  seed,
  initial,
  icon,
  status,
  statusColor,
  actions,
}: EditorTitleBarProps) {
  return (
    <DragRegion className="surface-chrome hairline-b flex h-[50px] shrink-0 items-center gap-2.5 px-3.5">
      <Avatar name={initial ?? title} seed={seed ?? title} icon={icon} className="size-7" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] font-semibold text-primary">{title}</div>
        {status ? (
          <div className="flex items-center gap-1.5 text-[11px] text-secondary">
            <span
              className="size-[7px] shrink-0 rounded-full"
              style={{ backgroundColor: statusColor ?? 'var(--accent)' }}
            />
            <span className="truncate">{status}</span>
          </div>
        ) : null}
      </div>
      {actions}
    </DragRegion>
  )
}
