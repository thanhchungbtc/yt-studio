import type { PointerEvent as ReactPointerEvent, ReactNode } from 'react'

import { beginWindowDrag, zoomWindow } from '../../core/desktop'
import { cn } from '../../core/utils'

/** Anything inside a drag region that should answer the pointer itself. */
const INTERACTIVE = 'button, a, input, textarea, select, [role="button"], [data-no-drag]'

interface DragRegionProps {
  className?: string
  children?: ReactNode
}

/**
 * A strip that moves the window, the way a macOS titlebar does.
 *
 * The primary sidebar, the secondary sidebar and the editor's title area are
 * all one of these, so the app can be picked up anywhere the eye reads as
 * chrome — which on a translucent window is most of what is not a document.
 *
 * The press is handed to AppKit and nothing further is tracked here: the drag
 * belongs to the window manager, and every gesture layered on top of it —
 * snapping, moving between displays, tiling — comes free for exactly as long
 * as this stays a request rather than an implementation.
 *
 * Controls placed inside keep working: a press that lands on one is left alone
 * rather than being turned into a drag.
 */
export function DragRegion({ className, children }: DragRegionProps) {
  const interactive = (event: ReactPointerEvent | { target: EventTarget | null }) =>
    event.target instanceof Element && event.target.closest(INTERACTIVE)

  return (
    <div
      className={cn('select-none', className)}
      onPointerDown={(event) => {
        if (event.button !== 0 || interactive(event)) return
        beginWindowDrag()
      }}
      onDoubleClick={(event) => {
        if (interactive(event)) return
        zoomWindow()
      }}
    >
      {children}
    </div>
  )
}
