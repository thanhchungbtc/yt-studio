import type { ReactNode } from 'react'

import { cn } from '../../../core/utils'
import type { Cell, CellState } from './stages'

/**
 * The five shapes, and nothing else.
 *
 * Colour is spent only on what wants attention. `done` is the expected state of
 * every cell in a finished video, so it is drawn quiet and solid rather than
 * green — eighty green dots say nothing that the header does not say better,
 * and they drown the one amber dot that matters. Amber is running, red is
 * failed, a hollow ring is stale, and everything else is grey.
 *
 * The shapes are distinguishable without colour too, which is not a courtesy:
 * `running` and `stale` are both amber, and the difference between them —
 * happening now, versus happened and may be wrong — is the whole point.
 */
const SHAPE: Record<CellState, string> = {
  done: 'bg-[var(--text-secondary)]',
  running: 'bg-[var(--running)]',
  queued: 'border border-[var(--text-tertiary)]',
  pending: 'bg-[var(--text-tertiary)] opacity-45 scale-50',
  failed: 'bg-[var(--failed)]',
}

const TITLE: Record<CellState, string> = {
  done: 'Done',
  running: 'Running',
  queued: 'Queued',
  pending: 'Not started',
  failed: 'Failed',
}

export function Mark({ cell, className }: { cell: Cell; className?: string }) {
  const label = cell.stale ? `${TITLE[cell.state]} · an input changed since` : TITLE[cell.state]
  return (
    <span
      title={cell.task?.error || label}
      aria-label={label}
      className={cn(
        'inline-block size-[7px] shrink-0 rounded-full transition-colors',
        cell.stale ? 'border-2 border-[var(--running)] bg-transparent' : SHAPE[cell.state],
        cell.state === 'running' && !cell.stale && 'animate-pulse',
        className,
      )}
    />
  )
}

/** A mark with the one fact that column is worth reading beside it. */
export function MarkCell({ cell, children }: { cell: Cell; children?: ReactNode }) {
  return (
    <span className="flex items-center gap-1.5">
      <Mark cell={cell} />
      <span
        className={cn(
          'truncate text-[12px] tabular-nums',
          cell.state === 'failed' ? 'text-[var(--failed)]' : 'text-secondary',
        )}
      >
        {cell.state === 'failed' ? 'failed' : children}
      </span>
    </span>
  )
}

/** One mark per slot: a half-drawn chapter says *which* slide is missing. */
export function SlotRow({ cells }: { cells: Cell[] }) {
  return (
    <span className="flex flex-wrap items-center gap-1">
      {cells.map((cell, index) => (
        <Mark key={index} cell={cell} />
      ))}
    </span>
  )
}
