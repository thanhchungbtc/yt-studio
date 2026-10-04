import type { LucideIcon } from 'lucide-react'
import { PanelBottom, PanelLeft, PanelRight, Settings } from 'lucide-react'

import { useScheduler } from '../core/events'
import type { PoolStat } from '../core/types'
import { cn } from '../core/utils'
import { useWorkbench } from '../store/workbench'
import { openSettings } from './settings'
import { DragRegion } from './ui/drag-region'
import { Tooltip } from './ui/tooltip'

/**
 * What lives here is ambient state — about the application, not about anything
 * you have open — which is why the pools are here and not in the video editor.
 * A pool is shared by every video at once; showing it beside one of them would
 * say it belonged to that one.
 */
export function StatusBar() {
  const scheduler = useScheduler()
  const primary = useWorkbench((s) => s.primaryVisible)
  const bottom = useWorkbench((s) => s.bottomVisible)
  const secondary = useWorkbench((s) => s.secondaryVisible)

  return (
    <DragRegion className="flex h-7 shrink-0 items-center gap-3 px-3 text-[11px] text-tertiary">
      {/* Clipped rather than crowding: six meters are wider than a narrow
          window, and the thing that must never be pushed off the edge is the
          control, not the readout. */}
      <div className="flex min-w-0 flex-1 items-center gap-4 overflow-hidden">
        {scheduler?.pools.map((pool) => (
          <Meter key={pool.pool} pool={pool} />
        ))}
      </div>
      <div className="-mr-1.5 flex shrink-0 items-center gap-0.5">
        <Toggle
          icon={PanelLeft}
          label="Toggle Library"
          shortcut="⌘1"
          active={primary}
          onClick={() => useWorkbench.getState().togglePrimary()}
        />
        <Toggle
          icon={PanelBottom}
          label="Toggle Console"
          shortcut="⌘2"
          active={bottom}
          onClick={() => useWorkbench.getState().toggleBottom()}
        />
        <Toggle
          icon={PanelRight}
          label="Toggle Inspector"
          shortcut="⌘3"
          active={secondary}
          onClick={() => useWorkbench.getState().toggleSecondary()}
        />
        <span className="mx-1 h-3 w-px bg-line-strong" />
        <Toggle icon={Settings} label="Settings" shortcut="⌘," onClick={openSettings} />
      </div>
    </DragRegion>
  )
}

function Toggle({
  icon: Icon,
  label,
  shortcut,
  active,
  onClick,
}: {
  icon: LucideIcon
  label: string
  shortcut: string
  active?: boolean
  onClick: () => void
}) {
  return (
    <Tooltip content={label} shortcut={shortcut} side="top">
      <button
        type="button"
        aria-label={label}
        aria-pressed={active}
        onClick={onClick}
        className={cn(
          'flex size-6 items-center justify-center rounded-md transition-colors duration-100 hover:bg-hover hover:text-primary',
          active === false ? 'text-fg-faint' : 'text-secondary',
        )}
      >
        <Icon className="size-3.5" strokeWidth={1.9} />
      </button>
    </Tooltip>
  )
}

/**
 * One pool: what it is, how full it is, and by how much.
 *
 * The bar and the number are not the same fact twice. The bar is proportion,
 * read without counting — four of thirty-two is a sliver whatever the digits
 * say. The number is the value, which is what you need the moment you go to
 * change a limit, and no bar has ever told anyone whether it meant two or three.
 *
 * Grey is idle, the accent is working, amber is full — three states you take in
 * before reading a digit. Amber is not an alarm: a saturated pool is the
 * pipeline using what it was given. It is the row worth acting on, because its
 * limit is the thing standing between you and a faster run.
 *
 * Only the numerator is coloured. It is the half that moves, and colouring the
 * whole fraction would make the capacity look like it were changing too.
 *
 * The queue is what the pool could not take. It sits in a fixed-width column
 * that is blank at zero rather than appearing and disappearing, because a
 * status bar that reflows every time work arrives is one you stop being able to
 * read at a glance.
 */
function Meter({ pool }: { pool: PoolStat }) {
  const full = pool.limit > 0 && pool.inFlight >= pool.limit
  const share = pool.limit > 0 ? Math.min(1, pool.inFlight / pool.limit) : 0
  const tone =
    pool.inFlight === 0 ? 'var(--text-tertiary)' : full ? 'var(--running)' : 'var(--accent)'

  return (
    <span
      className="flex shrink-0 items-center gap-1.5"
      title={`${pool.pool} · ${pool.inFlight} of ${pool.limit} in flight · ${pool.queued} queued`}
    >
      <span className="text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase">
        {pool.pool}
      </span>

      <span
        className="block h-[4px] w-[18px] overflow-hidden rounded-full"
        style={{ backgroundColor: 'var(--active)' }}
      >
        <span
          className="block h-full rounded-full transition-[width,background-color] duration-200 ease-out"
          style={{ width: `${share * 100}%`, backgroundColor: tone }}
        />
      </span>

      <span className="text-[11px] tabular-nums">
        <span style={{ color: tone }}>{pool.inFlight}</span>
        <span className="text-tertiary">/{pool.limit}</span>
      </span>

      <span className="w-[16px] text-[10px] tabular-nums text-tertiary">
        {pool.queued > 0 ? `+${pool.queued}` : ''}
      </span>
    </span>
  )
}
