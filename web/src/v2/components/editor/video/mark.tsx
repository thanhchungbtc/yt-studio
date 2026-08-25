import { X } from 'lucide-react'

import { cn } from '../../../core/utils'
import type { Cell, CellState } from './stages'

/**
 * The four shapes, and nothing else.
 *
 * Every one of them differs from the others in more than colour, which is the
 * whole design constraint: a grid read at a glance cannot ask anyone to
 * remember which shade of small circle means what, and roughly one man in
 * twelve would not be able to anyway.
 *
 *   done     a filled disc — solid, settled, there
 *   running  an arc, spinning; the only shape that moves
 *   waiting  an open ring — the outline of something not yet filled in
 *   failed   a disc with a cross struck through it
 *   stale    done, ringed in the colour of attention
 *
 * Filled means *there is something there*; open means not yet. Green is settled
 * and red is wrong, but neither carries the state on its own — the silhouette
 * does, and the colour only confirms it.
 *
 * Twelve pixels rather than seven. At seven a hollow ring and a filled disc are
 * the same grey speck, and the vocabulary might as well not exist.
 */
const SIZE = 'size-3'

const TITLE: Record<CellState, string> = {
  done: 'Done',
  running: 'Running',
  waiting: 'Waiting',
  failed: 'Failed',
}

function Shape({ cell }: { cell: Cell }) {
  // Stale is *done*, flagged — so it keeps the filled core and takes an amber
  // rim, rather than becoming a fifth shape that hides what it is underneath.
  if (cell.stale) {
    return (
      <span
        className={cn(SIZE, 'block rounded-full border-2 bg-clip-padding')}
        style={{ backgroundColor: 'var(--done)', borderColor: 'var(--running)' }}
      />
    )
  }

  switch (cell.state) {
    case 'running':
      return <span className={cn(SIZE, 'mark-spinner block rounded-full')} />
    case 'waiting':
      return (
        <span
          className={cn(SIZE, 'block rounded-full border-[1.5px]')}
          style={{ borderColor: 'var(--text-tertiary)' }}
        />
      )
    case 'failed':
      return (
        <span
          className={cn(SIZE, 'flex items-center justify-center rounded-full')}
          style={{ backgroundColor: 'var(--failed)' }}
        >
          <X className="size-2 text-white" strokeWidth={3.5} />
        </span>
      )
    default:
      return (
        <span
          className={cn(SIZE, 'block rounded-full')}
          style={{ backgroundColor: 'var(--done)' }}
        />
      )
  }
}

export function Mark({ cell, className }: { cell: Cell; className?: string }) {
  const label = cell.stale ? `${TITLE[cell.state]} · an input changed since` : TITLE[cell.state]
  return (
    <span
      title={cell.task?.error || label}
      aria-label={label}
      className={cn('inline-flex shrink-0', className)}
    >
      <Shape cell={cell} />
    </span>
  )
}

/** One mark per slot: a half-drawn chapter says *which* slide is missing. */
export function SlotRow({ cells, className }: { cells: Cell[]; className?: string }) {
  return (
    <span className={cn('flex flex-wrap items-center gap-1.5', className)}>
      {cells.map((cell, index) => (
        <Mark key={index} cell={cell} />
      ))}
    </span>
  )
}

/**
 * The key to the grid.
 *
 * A symbolic vocabulary that nobody can read is decoration, and a tooltip is
 * not an answer — it requires already suspecting the shape means something.
 * Five items, one line, permanently on screen, where anyone learning to read
 * the table is already looking.
 */
const LEGEND: { cell: Cell; label: string }[] = [
  { cell: { state: 'done', stale: false }, label: 'Done' },
  { cell: { state: 'running', stale: false }, label: 'Running' },
  { cell: { state: 'waiting', stale: false }, label: 'Waiting' },
  { cell: { state: 'failed', stale: false }, label: 'Failed' },
  { cell: { state: 'done', stale: true }, label: 'Stale — an input changed since' },
]

export function Legend() {
  return (
    <div className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-1 px-4 py-2">
      {LEGEND.map(({ cell, label }) => (
        <span key={label} className="flex items-center gap-1.5 text-[11px] text-tertiary">
          <Mark cell={cell} />
          {label}
        </span>
      ))}
    </div>
  )
}
