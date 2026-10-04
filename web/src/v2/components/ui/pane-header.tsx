import type { ReactNode } from 'react'

import { DragRegion } from './drag-region'
import { Kbd } from './kbd'

interface PaneHeaderProps {
  title: string
  /** The key that shows and hides this pane. */
  shortcut?: string
  /** Trailing controls, grouped into one glass pill. */
  actions?: ReactNode
}

/**
 * The strip along the top of a pane card: its name, the key that toggles it,
 * and whatever it carries on the trailing edge.
 *
 * The same height as the tab strip and the library's header, so every card in
 * the window lines up along one top edge. It is chrome, so it moves the window.
 */
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
