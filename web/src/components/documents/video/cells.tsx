import { AlertTriangle, Play } from 'lucide-react'
import type { ReactNode } from 'react'

import { assetUrl } from '@/core/api'
import { formatClock } from '@/core/format'
import type { Chapter } from '@/core/types'
import { cn } from '@/core/utils'
import type { Cell } from './stages'

/**
 * The asset cells: what exists, and a way to look at it. Nothing else.
 *
 * An earlier pass hung a Radix tooltip on every thumbnail, play button and stale
 * flag, and a retry mutation on every failed cell. Fifteen visible rows meant
 * ninety tooltip roots subscribed to one provider, each opening and closing as
 * the pointer crossed it — which is what made hovering a row feel like work.
 *
 * Where a hint is genuinely worth having it is a native `title`: no React, no
 * state, no cost until the pointer rests.
 */

/* ------------------------------------------------------------------ shared */

/** The four not-done states, one shape each, identical in every column. */
function Placeholder({ cell }: { cell: Cell }) {
  switch (cell.state) {
    case 'running':
      return (
        <span className="sweep h-4 w-10 rounded-[var(--radius-xs)] bg-[hsl(var(--bg-hover))]" />
      )
    case 'queued':
      return <span className="h-1.5 w-1.5 rounded-full bg-[hsl(var(--info))]" title="queued" />
    case 'failed':
      return (
        <AlertTriangle
          className="h-3.5 w-3.5 text-[hsl(var(--danger))]"
          aria-label="failed"
          {...(cell.task?.error ? { title: cell.task.error } : {})}
        />
      )
    default:
      return <span className="text-[11px] text-[hsl(var(--fg-subtle))] opacity-50">·</span>
  }
}

/** Intact, but an input moved after it ran. */
function Stale() {
  return (
    <span
      className="text-[10px] leading-none text-[hsl(var(--warning))]"
      title="Stale — an input changed after this ran"
    >
      ⚑
    </span>
  )
}

function Filled({ cell, children }: { cell: Cell; children: ReactNode }) {
  if (cell.state !== 'done') return <Placeholder cell={cell} />
  return (
    <span className="flex items-center gap-1">
      {children}
      {cell.stale && <Stale />}
    </span>
  )
}

/* ------------------------------------------------------------------ script */

/** Words written, against what the blueprint budgeted one column to the left. */
export function ScriptCell({
  cell,
  words,
  onOpen,
}: {
  cell: Cell
  words: number
  onOpen: () => void
}) {
  return (
    <Filled cell={cell}>
      <button
        type="button"
        onClick={onOpen}
        data-script-words={words}
        className="tabular rounded-[var(--radius-xs)] px-1 py-0.5 text-[11.5px] text-fg hover:bg-[hsl(var(--fg)/0.08)]"
      >
        {words}w
      </button>
    </Filled>
  )
}

/* --------------------------------------------------------------- narration */

/** Its duration, and a link to hear it. */
export function NarrationCell({
  cell,
  assetId,
  seconds,
  onOpen,
}: {
  cell: Cell
  assetId: string | undefined
  seconds: number
  onOpen: () => void
}) {
  return (
    <Filled cell={cell}>
      <button
        type="button"
        onClick={onOpen}
        disabled={!assetId}
        aria-label="Play the narration"
        className="flex items-center gap-1.5 rounded-[var(--radius-xs)] px-1 py-0.5 text-[11.5px] text-fg hover:bg-[hsl(var(--fg)/0.08)]"
      >
        <Play className="h-3 w-3 text-subtle" />
        <span className="tabular">{seconds > 0 ? formatClock(seconds) : '—'}</span>
      </button>
    </Filled>
  )
}

/* ------------------------------------------------------------------ slides */

/**
 * The pictures, at whatever size the column can afford. Unfilled slots are drawn
 * at their eventual count, so the cell has its final shape from the moment the
 * blueprint lands and nothing reflows as images arrive.
 */
export function SlidesCell({
  chapter,
  cells,
  thumbWidth,
  onOpenSlide,
}: {
  chapter: Chapter
  cells: Cell[]
  thumbWidth: number | null
  onOpenSlide: (slot: number) => void
}) {
  // Below the legible floor a thumbnail stops being a picture, and the cell
  // becomes a strip. Widening the column brings the pictures back.
  if (thumbWidth === null) {
    return (
      <div className="flex flex-wrap items-center gap-[3px]">
        {cells.map((cell, slot) => (
          <button
            key={slot}
            type="button"
            aria-label={`Slide ${slot + 1}`}
            title={chapter.slidePrompts[slot] ?? `Slide ${slot + 1}`}
            onClick={() => onOpenSlide(slot)}
            className={cn(
              'h-3 w-3 rounded-[2px]',
              cell.state === 'done' && !cell.stale && 'bg-[hsl(var(--success))]',
              cell.state === 'done' && cell.stale && 'bg-[hsl(var(--warning))]',
              cell.state === 'running' && 'bg-[hsl(var(--accent))]',
              cell.state === 'queued' && 'bg-[hsl(var(--info))]',
              cell.state === 'failed' && 'bg-[hsl(var(--danger))]',
              cell.state === 'pending' && 'bg-[hsl(var(--fg)/0.12)]',
            )}
          />
        ))}
      </div>
    )
  }

  const height = Math.round((thumbWidth * 9) / 16)

  return (
    <div className="flex items-center gap-1">
      {cells.map((cell, slot) => {
        const assetId = chapter.slideAssetIds[slot]
        return (
          <button
            key={slot}
            type="button"
            aria-label={`Slide ${slot + 1}`}
            title={chapter.slidePrompts[slot] ?? `Slide ${slot + 1}`}
            onClick={() => onOpenSlide(slot)}
            style={{ width: thumbWidth, height }}
            className={cn(
              'relative shrink-0 overflow-hidden rounded-[var(--radius-xs)]',
              cell.state === 'done'
                ? 'checker ring-1 ring-[hsl(var(--border))] hover:ring-[hsl(var(--accent))]'
                : 'border border-dashed border-[hsl(var(--border-strong))]',
            )}
          >
            {cell.state === 'done' && assetId && (
              <img
                src={assetUrl(assetId)}
                alt={`Slide ${slot + 1}`}
                loading="lazy"
                decoding="async"
                className="h-full w-full object-cover"
              />
            )}
            {cell.state === 'running' && <span className="sweep absolute inset-0" />}
            {cell.state === 'failed' && (
              <span className="absolute inset-0 flex items-center justify-center bg-[hsl(var(--danger-soft))]">
                <AlertTriangle className="h-3 w-3 text-[hsl(var(--danger))]" />
              </span>
            )}
            {cell.stale && (
              <span className="absolute right-0 top-0 h-0 w-0 border-l-[10px] border-t-[10px] border-l-transparent border-t-[hsl(var(--warning))]" />
            )}
          </button>
        )
      })}
    </div>
  )
}

/* -------------------------------------------------------------------- clip */

/**
 * The composed chapter. No duration and no poster frame: the duration is the
 * narration's, one column left, and the poster would be the first slide, one
 * column right.
 */
export function ClipCell({ cell, onOpen }: { cell: Cell; onOpen: () => void }) {
  return (
    <Filled cell={cell}>
      <button
        type="button"
        aria-label="Play the composed chapter"
        title="Play the composed chapter"
        onClick={onOpen}
        className="flex h-5 w-5 items-center justify-center rounded-[var(--radius-xs)] text-[hsl(var(--success))] hover:bg-[hsl(var(--fg)/0.08)]"
      >
        <Play className="h-3.5 w-3.5" />
      </button>
    </Filled>
  )
}
