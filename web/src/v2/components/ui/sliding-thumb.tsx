import { useCallback, useLayoutEffect, useRef, useState } from 'react'

import { cn } from '../../core/utils'

/**
 * The selection of a segmented control, gliding between options.
 *
 * Render it first inside the control (which must be `relative`); options carry
 * `data-thumb-key` and draw no background of their own. It measures within its
 * own parent, does not animate on first paint, and follows size changes —
 * which snap rather than glide, because a resize is not a choice.
 */
export function SlidingThumb({ active, className }: { active: string | undefined; className?: string }) {
  const thumb = useRef<HTMLSpanElement>(null)
  const [box, setBox] = useState<{ x: number; y: number; w: number; h: number }>()
  const [moving, setMoving] = useState(false)
  const latest = useRef(active)
  latest.current = active

  const measure = useCallback(() => {
    const root = thumb.current?.parentElement
    const key = latest.current
    const element =
      !root || key === undefined
        ? null
        : root.querySelector<HTMLElement>(`[data-thumb-key="${CSS.escape(key)}"]`)
    // Read here, not in the updater. React runs an updater during its next
    // render, and by then the click may have committed a whole document under
    // this control — so a read there forces the style of all of it, inside the
    // render, before the frame that would have computed it anyway.
    const next = element
      ? {
          x: element.offsetLeft,
          y: element.offsetTop,
          w: element.offsetWidth,
          h: element.offsetHeight,
        }
      : undefined
    setBox((previous) => {
      if (!next) return undefined
      return previous &&
        previous.x === next.x &&
        previous.y === next.y &&
        previous.w === next.w &&
        previous.h === next.h
        ? previous
        : next
    })
  }, [])

  const first = useRef(true)
  useLayoutEffect(() => {
    if (first.current) first.current = false
    else setMoving(true)
    measure()
  }, [active, measure])

  useLayoutEffect(() => {
    const root = thumb.current?.parentElement
    if (!root) return
    let size = `${root.offsetWidth}x${root.offsetHeight}`
    const observer = new ResizeObserver(() => {
      const now = `${root.offsetWidth}x${root.offsetHeight}`
      if (now === size) return
      size = now
      setMoving(false)
      measure()
    })
    observer.observe(root)
    return () => observer.disconnect()
  }, [measure])

  return (
    <span
      ref={thumb}
      aria-hidden
      data-moving={moving || undefined}
      className={cn(
        'sliding-thumb glass-thumb pointer-events-none absolute top-0 left-0 rounded-full',
        !box && 'invisible',
        className,
      )}
      style={
        box
          ? { width: box.w, height: box.h, transform: `translate(${box.x}px, ${box.y}px)` }
          : undefined
      }
    />
  )
}
