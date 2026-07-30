import { useCallback, useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

/**
 * The drag handle between two panes: a real `separator` with arrow-key support,
 * whose *hit* area is wider than its *drawn* area — a 1px line grabbable from
 * 5px away.
 */
export function Splitter({
  width,
  onResize,
  min,
  max,
  onDoubleClick,
  'aria-label': ariaLabel,
}: {
  width: number
  onResize: (width: number) => void
  min: number
  max: number
  onDoubleClick?: () => void
  'aria-label'?: string
}) {
  const [dragging, setDragging] = useState(false)
  // Read through refs: the pointer handlers are attached once per drag and must
  // not close over a stale callback or a stale starting width.
  const resizeRef = useRef(onResize)
  resizeRef.current = onResize
  const originRef = useRef({ x: 0, width })

  const onPointerDown = useCallback(
    (event: React.PointerEvent<HTMLDivElement>) => {
      if (event.button !== 0) return
      event.preventDefault()
      // Measuring from where the drag began rather than from the viewport edge
      // keeps the handle under the cursor whatever is to the left of the pane.
      originRef.current = { x: event.clientX, width }
      setDragging(true)
    },
    [width],
  )

  useEffect(() => {
    if (!dragging) return

    const onPointerMove = (event: PointerEvent) => {
      const { x, width: startWidth } = originRef.current
      resizeRef.current(startWidth + (event.clientX - x))
    }
    const stop = () => setDragging(false)

    // A drag must keep working when the pointer leaves the handle, so the
    // listeners go on the window and the cursor is pinned for the duration.
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
    const previousCursor = document.body.style.cursor
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    return () => {
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
      document.body.style.cursor = previousCursor
      document.body.style.userSelect = ''
    }
  }, [dragging])

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={ariaLabel ?? 'Resize'}
      aria-valuenow={width}
      aria-valuemin={min}
      aria-valuemax={max}
      tabIndex={0}
      onPointerDown={onPointerDown}
      onDoubleClick={onDoubleClick}
      onKeyDown={(event) => {
        if (event.key === 'ArrowLeft') onResize(width - (event.shiftKey ? 32 : 8))
        else if (event.key === 'ArrowRight') onResize(width + (event.shiftKey ? 32 : 8))
        else return
        event.preventDefault()
      }}
      className={cn(
        'group relative z-20 -mr-[3px] w-[7px] shrink-0 cursor-col-resize touch-none no-select',
        'focus-visible:outline-none',
      )}
    >
      <span
        aria-hidden
        className={cn(
          'absolute inset-y-0 left-[3px] w-px transition-colors duration-100',
          dragging
            ? 'bg-[hsl(var(--accent))]'
            : 'bg-[hsl(var(--border))] group-hover:bg-[hsl(var(--accent)/0.7)] group-focus-visible:bg-[hsl(var(--accent))]',
        )}
      />
    </div>
  )
}
