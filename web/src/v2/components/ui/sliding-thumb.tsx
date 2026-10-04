import { useCallback, useLayoutEffect, useRef, useState } from 'react'

import { cn } from '../../core/utils'

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
    // Read layout outside the updater; React runs it mid-render.
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
