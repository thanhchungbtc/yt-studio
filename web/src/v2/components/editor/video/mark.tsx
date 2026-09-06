import { X } from 'lucide-react'

import { cn } from '../../../core/utils'
import { Menu, type MenuItem } from '../../ui/menu'
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

/**
 * The hit target, which is not the dot.
 *
 * Twelve pixels is the right size to *read* a grid of these at and far too small
 * to ask anyone to hit. The padding grows the target to twenty-eight while the
 * negative margin takes it back out of the layout, so the button paints and
 * catches the pointer over a comfortable square and the dot stays exactly where
 * the column put it. Nothing above this has to know the dot became a control.
 */
const TARGET = '-m-2 p-2'

/**
 * A mark, and — where there is something to do about it — the menu to do it.
 *
 * The menu is a prop rather than something the mark works out for itself,
 * because the same four shapes are drawn in three places and only one of them
 * is a grid of live tasks. The legend draws marks that stand for a state rather
 * than being in one, and a legend you could right-click into re-running
 * somebody's video is a bug waiting for a slow afternoon.
 */
export function Mark({
  cell,
  className,
  menu,
}: {
  cell: Cell
  className?: string
  menu?: MenuItem[]
}) {
  const label = cell.stale ? `${TITLE[cell.state]} · an input changed since` : TITLE[cell.state]
  const tooltip = cell.task?.error || label

  if (!menu || menu.length === 0) {
    return (
      <span title={tooltip} aria-label={label} className={cn('inline-flex shrink-0', className)}>
        <Shape cell={cell} />
      </span>
    )
  }

  // The caller's className stays on the outer span in both branches, and the
  // target grows inside it. The column passes alignment through here (`pt-1`),
  // and letting that land on the same element as the target's own padding would
  // have tailwind-merge collapse the two — leaving actionable dots sitting four
  // pixels above the inert ones in the same row.
  return (
    <span className={cn('inline-flex shrink-0', className)}>
      {/* `align="start"` so the card hangs from the dot's own edge. A twelve-pixel
          trigger centred under a 176-pixel menu says nothing about which of eighty
          slides it belongs to, and that is the one thing it has to be clear about. */}
      <Menu items={menu} align="start">
        <button
          type="button"
          title={tooltip}
          aria-label={`${label} — actions`}
          className={cn('inline-flex cursor-default rounded-full', TARGET)}
        >
          <Shape cell={cell} />
        </button>
      </Menu>
    </span>
  )
}
