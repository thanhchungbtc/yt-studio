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
    <DragRegion className="edge-line relative z-[1] flex h-[48px] shrink-0 items-center gap-2.5 px-4">
      <Avatar name={initial ?? title} seed={seed ?? title} icon={icon} className="size-7" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-base leading-tight font-semibold tracking-[-0.005em] text-fg">
          {title}
        </div>
        {status ? (
          <div className="mt-0.5 flex items-center gap-1.5 text-xs text-fg-muted">
            {statusColor ? (
              <span
                className="size-1.5 shrink-0 rounded-full"
                style={{ backgroundColor: statusColor }}
              />
            ) : null}
            <span className="truncate">{status}</span>
          </div>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-1.5">{actions}</div> : null}
    </DragRegion>
  )
}
