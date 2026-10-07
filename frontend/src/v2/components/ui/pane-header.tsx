import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { Tooltip } from '@/kit/ui/Tooltip'

/** A one-view strip in the kit's view-container style. */
export function PaneHeader({
  title,
  icon: Icon,
  command,
  side = 'bottom',
  actions,
}: {
  title: string
  icon: LucideIcon
  command?: string
  side?: 'top' | 'bottom'
  actions?: ReactNode
}) {
  return (
    <div className="drag-region flex h-[var(--strip-height)] shrink-0 items-center gap-2 px-2">
      <Tooltip content={`Toggle ${title}`} command={command} side={side}>
        <div className="no-drag flex h-7 items-center rounded-full bg-well p-0.5">
          <span className="flex h-6 items-center gap-1.5 rounded-full bg-content px-2.5 text-fg shadow-[0_1px_2px_rgb(0_0_0/0.08)] hairline">
            <Icon className="size-[15px]" strokeWidth={1.8} />
            <span className="text-xs font-medium">{title}</span>
          </span>
        </div>
      </Tooltip>
      <div className="flex-1" />
      {actions ? <div className="no-drag glass-pill">{actions}</div> : null}
    </div>
  )
}
