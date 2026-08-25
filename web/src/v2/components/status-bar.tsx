import { Settings } from 'lucide-react'

import { useScheduler } from '../core/events'
import type { PoolStat } from '../core/types'
import { openSettings } from './settings'
import { HeaderButton } from './ui/header-button'

/**
 * The status bar.
 *
 * Full-width and in the same material as every other pane, so it reads as the
 * floor the window stands on rather than as a strip bolted to the bottom.
 *
 * What lives here is ambient state — about the application, not about anything
 * you have open — which is why the pools are here and not in the video editor.
 * A pool is shared by every video at once; showing it beside one of them would
 * say it belonged to that one.
 */
export function StatusBar() {
  const scheduler = useScheduler()

  return (
    <footer className="surface-chrome hairline-t flex h-[24px] shrink-0 items-center gap-4 px-3">
      {scheduler?.pools.map((pool) => (
        <Meter key={pool.pool} pool={pool} />
      ))}
      <span className="min-w-0 flex-1" />
      <HeaderButton
        icon={Settings}
        label="Settings"
        onClick={openSettings}
        className="-mr-1 size-[18px]"
      />
    </footer>
  )
}

/**
 * One pool, as a capacity meter.
 *
 * Grey is idle, blue is working, amber is full — three states you read by
 * colour without counting anything, which is the entire job of a status bar.
 * Amber is not an alarm: a saturated pool is the pipeline using what it was
 * given. It is the row worth acting on, because it is the one whose limit is
 * the thing standing between you and a faster run.
 *
 * The queue is what the pool could not take. It sits in a fixed-width column
 * that is blank at zero rather than appearing and disappearing, because a
 * status bar that reflows every time work arrives is a status bar you stop
 * being able to read at a glance.
 */
function Meter({ pool }: { pool: PoolStat }) {
  const full = pool.limit > 0 && pool.inFlight >= pool.limit
  const share = pool.limit > 0 ? Math.min(1, pool.inFlight / pool.limit) : 0

  return (
    <span
      className="flex shrink-0 items-center gap-1.5"
      title={`${pool.pool} · ${pool.inFlight} of ${pool.limit} in flight · ${pool.queued} queued`}
    >
      <span className="text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase">
        {pool.pool}
      </span>
      <span
        className="block h-[4px] w-[26px] overflow-hidden rounded-full"
        style={{ backgroundColor: 'var(--idle-selection)' }}
      >
        <span
          className="block h-full rounded-full transition-[width,background-color] duration-200 ease-out"
          style={{
            width: `${share * 100}%`,
            backgroundColor: full ? 'var(--running)' : 'var(--accent)',
          }}
        />
      </span>
      <span className="w-[14px] text-[10px] tabular-nums text-tertiary">
        {pool.queued > 0 ? `+${pool.queued}` : ''}
      </span>
    </span>
  )
}
