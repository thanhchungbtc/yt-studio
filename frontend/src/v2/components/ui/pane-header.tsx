import type { ReactNode } from 'react'

import { DragRegion } from './drag-region'
import { Kbd } from './kbd'

interface PaneHeaderProps {
  title: string
  shortcut?: string
  actions?: ReactNode
}

export function PaneHeader({ title, shortcut, actions }: PaneHeaderProps) {
  return (
    <DragRegion className="flex h-[var(--strip-height)] shrink-0 items-center gap-2 pr-2 pl-3.5">
      <span className="min-w-0 flex-1 truncate text-[12px] font-semibold text-secondary">
        {title}
      </span>
      {actions ? <div className="glass-pill">{actions}</div> : null}
      {shortcut ? <Kbd keys={shortcut} className="opacity-80" /> : null}
    </DragRegion>
  )
}
