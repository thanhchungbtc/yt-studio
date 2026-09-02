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
