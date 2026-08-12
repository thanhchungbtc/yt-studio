import type { PointerEvent as ReactPointerEvent } from 'react'
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from 'react'

import type { Design, DesignElement } from '@/core/thumbnail/doc'
import { FRAME_HEIGHT, FRAME_WIDTH, hitTest } from '@/core/thumbnail/doc'
import type { ImageBank } from '@/core/thumbnail/render'
import { renderDesign } from '@/core/thumbnail/render'
import { resourceUrl } from '@/core/thumbnail/loading'
import { assetUrl } from '@/core/api'
import { cn } from '@/core/utils'

/** The corner grips, and the whole-element drag. */
type Grip = 'nw' | 'ne' | 'sw' | 'se' | 'move'

const GRIP_SIZE = 14
const MIN_SIZE = 16

export interface ThumbnailCanvasHandle {
  /** The frame as a PNG blob — the bytes that get uploaded.
   *
   *  Taken from the base canvas, which never carries selection chrome: the
   *  handles are drawn on a second canvas stacked over it, so what is exported
   *  is exactly what the operator sees minus the editing furniture. */
  toBlob: () => Promise<Blob | null>
}

export interface ThumbnailCanvasProps {
  design: Design
  images: ImageBank
  /** Bumped when an image finishes loading; the Map itself never changes. */
  generation: number
  fontFamily: string
  selectedId?: string
  onSelect: (id: string | undefined) => void
  onChange: (design: Design) => void
  /** Called once per gesture, not per pointer move, so undo and autosave see
   *  one edit rather than two hundred. */
  onCommit: () => void
  className?: string
}

/**
 * The editing surface: the design painted at full size, with direct
 * manipulation over the top.
 *
 * Dragging is safe here in a way it would not be against the Go renderer. There
 * the layout is a solver that runs again on every render, so a hand-placed
 * element would be overwritten the next time the grid changed. Here the
 * browser's output *is* the artifact — nothing re-solves it — so absolute
 * positions are exactly what the document should hold.
 */
export const ThumbnailCanvas = forwardRef<ThumbnailCanvasHandle, ThumbnailCanvasProps>(
  function ThumbnailCanvas(props, ref) {
    const { design, images, generation, fontFamily, selectedId, onSelect, onChange, onCommit } =
      props

    const baseRef = useRef<HTMLCanvasElement>(null)
    const overlayRef = useRef<HTMLCanvasElement>(null)
    // The gesture in flight. A ref rather than state: pointermove must not
    // queue a render per event, and the canvas is redrawn imperatively anyway.
    const drag = useRef<{
      grip: Grip
      id: string
      startX: number
      startY: number
      origin: { x: number; y: number; w: number; h: number }
    } | null>(null)

    useImperativeHandle(ref, () => ({
      toBlob: () =>
        new Promise((resolve) => {
          const canvas = baseRef.current
          if (!canvas) {
            resolve(null)
            return
          }
          canvas.toBlob((blob) => resolve(blob), 'image/png')
        }),
    }))

    /* ------------------------------------------------------------- paint */

    useEffect(() => {
      const ctx = baseRef.current?.getContext('2d')
      if (!ctx) return
      renderDesign(ctx, {
        design,
        fontFamily,
        images,
        assetUrl: (id) => assetUrl(id) ?? '',
        resourceUrl,
      })
      // generation is not read here, but a change to it means the same document
      // paints differently: an icon or the backdrop has arrived.
    }, [design, images, generation, fontFamily])

    useEffect(() => {
      const ctx = overlayRef.current?.getContext('2d')
      if (!ctx) return
      ctx.clearRect(0, 0, FRAME_WIDTH, FRAME_HEIGHT)
      const el = design.elements.find((e) => e.id === selectedId)
      if (!el || el.hidden) return

      ctx.save()
      ctx.strokeStyle = '#38bdf8'
      ctx.lineWidth = 2
      ctx.setLineDash([6, 4])
      ctx.strokeRect(el.x, el.y, el.w, el.h)
      ctx.setLineDash([])
      if (!el.locked) {
        ctx.fillStyle = '#38bdf8'
        for (const [gx, gy] of corners(el)) {
          ctx.fillRect(gx - GRIP_SIZE / 2, gy - GRIP_SIZE / 2, GRIP_SIZE, GRIP_SIZE)
        }
      }
      ctx.restore()
    }, [design, selectedId])

    /* ----------------------------------------------------------- gesture */

    const toFrame = useCallback((event: ReactPointerEvent<HTMLCanvasElement>) => {
      const rect = event.currentTarget.getBoundingClientRect()
      // The canvas is displayed scaled down; pointer coordinates are mapped
      // back into the 1280x720 frame so the document only ever holds real
      // pixels and the export needs no correction.
      return {
        x: ((event.clientX - rect.left) / rect.width) * FRAME_WIDTH,
        y: ((event.clientY - rect.top) / rect.height) * FRAME_HEIGHT,
      }
    }, [])

    const onPointerDown = useCallback(
      (event: ReactPointerEvent<HTMLCanvasElement>) => {
        const at = toFrame(event)
        const selected = design.elements.find((e) => e.id === selectedId)

        // Grips are tested before elements, so a corner overlapping a
        // neighbouring tile resizes rather than selecting the neighbour.
        if (selected && !selected.locked) {
          const grip = gripAt(selected, at.x, at.y)
          if (grip) {
            event.currentTarget.setPointerCapture(event.pointerId)
            drag.current = {
              grip,
              id: selected.id,
              startX: at.x,
              startY: at.y,
              origin: { x: selected.x, y: selected.y, w: selected.w, h: selected.h },
            }
            return
          }
        }

        const hit = hitTest(design, at.x, at.y)
        onSelect(hit?.id)
        if (!hit || hit.locked) return
        event.currentTarget.setPointerCapture(event.pointerId)
        drag.current = {
          grip: 'move',
          id: hit.id,
          startX: at.x,
          startY: at.y,
          origin: { x: hit.x, y: hit.y, w: hit.w, h: hit.h },
        }
      },
      [design, selectedId, onSelect, toFrame],
    )

    const onPointerMove = useCallback(
      (event: ReactPointerEvent<HTMLCanvasElement>) => {
        const state = drag.current
        if (!state) return
        const at = toFrame(event)
        const dx = at.x - state.startX
        const dy = at.y - state.startY
        const box = resize(state.grip, state.origin, dx, dy, event.shiftKey)
        onChange({
          ...design,
          elements: design.elements.map((el) => (el.id === state.id ? { ...el, ...box } : el)),
        })
      },
      [design, onChange, toFrame],
    )

    const endGesture = useCallback(() => {
      if (!drag.current) return
      drag.current = null
      onCommit()
    }, [onCommit])

    return (
      <div className={cn('relative select-none', props.className)}>
        <canvas
          ref={baseRef}
          width={FRAME_WIDTH}
          height={FRAME_HEIGHT}
          className="block h-auto w-full rounded-[var(--radius-sm)]"
        />
        <canvas
          ref={overlayRef}
          width={FRAME_WIDTH}
          height={FRAME_HEIGHT}
          className="absolute inset-0 block h-full w-full touch-none"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={endGesture}
          onPointerCancel={endGesture}
        />
      </div>
    )
  },
)

/* ------------------------------------------------------------- geometry */

function corners(el: DesignElement): [number, number][] {
  return [
    [el.x, el.y],
    [el.x + el.w, el.y],
    [el.x, el.y + el.h],
    [el.x + el.w, el.y + el.h],
  ]
}

function gripAt(el: DesignElement, x: number, y: number): Grip | undefined {
  const names: Grip[] = ['nw', 'ne', 'sw', 'se']
  const points = corners(el)
  for (let i = 0; i < points.length; i++) {
    const point = points[i]
    const name = names[i]
    if (!point || !name) continue
    if (Math.abs(x - point[0]) <= GRIP_SIZE && Math.abs(y - point[1]) <= GRIP_SIZE) return name
  }
  return undefined
}

interface Box {
  x: number
  y: number
  w: number
  h: number
}

/** Applies a gesture to a box. Shift keeps the aspect, which is what a tile
 *  wants: an icon square stretched to a rectangle reads as a mistake. */
function resize(grip: Grip, origin: Box, dx: number, dy: number, keepAspect: boolean): Box {
  if (grip === 'move') {
    return { ...origin, x: Math.round(origin.x + dx), y: Math.round(origin.y + dy) }
  }
  const east = grip === 'ne' || grip === 'se'
  const south = grip === 'se' || grip === 'sw'

  let w = east ? origin.w + dx : origin.w - dx
  let h = south ? origin.h + dy : origin.h - dy
  w = Math.max(w, MIN_SIZE)
  h = Math.max(h, MIN_SIZE)
  if (keepAspect && origin.w > 0 && origin.h > 0) {
    const scale = Math.max(w / origin.w, h / origin.h)
    w = origin.w * scale
    h = origin.h * scale
  }
  return {
    x: Math.round(east ? origin.x : origin.x + (origin.w - w)),
    y: Math.round(south ? origin.y : origin.y + (origin.h - h)),
    w: Math.round(w),
    h: Math.round(h),
  }
}
