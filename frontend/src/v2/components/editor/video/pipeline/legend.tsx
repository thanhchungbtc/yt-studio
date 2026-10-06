import { Mark } from '../mark'
import type { Cell } from '../stages'

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
