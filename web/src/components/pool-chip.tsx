import { memo } from 'react'

import { Tooltip } from '@/components/ui/primitives'
import { poolLabel } from '@/lib/format'
import type { PoolName, PoolStat } from '@/lib/types'
import { cn } from '@/lib/utils'

/**
 * One pool per hue, so the status bar reads as a picture rather than a list of
 * numbers: which resource is busy is a glance, not a parse.
 */
const POOL_HUE: Record<PoolName, string> = {
  llm: 'var(--violet)',
  tts: 'var(--info)',
  image: 'var(--accent)',
  compose: 'var(--success)',
  cache: 'var(--fg-subtle)',
  upload: 'var(--warning)',
}

const SHORT: Record<PoolName, string> = {
  llm: 'LLM',
  tts: 'TTS',
  image: 'IMG',
  compose: 'CMP',
  cache: 'CSH',
  upload: 'UPL',
}

/**
 * A pool's occupancy as filled slots plus a queue badge. Capacity is the
 * binding constraint of the whole system, so it lives in the status bar
 * where it is always visible.
 */
export const PoolChip = memo(function PoolChip({ stat }: { stat: PoolStat }) {
  const hue = POOL_HUE[stat.pool] ?? 'var(--accent)'
  const slots = Math.min(stat.limit, 8)
  const idle = stat.inFlight === 0
  const saturated = stat.limit > 0 && stat.inFlight >= stat.limit
  const bottleneck = saturated && stat.queued > 0

  return (
    <Tooltip
      label={
        <span className="block space-y-0.5">
          <span className="block font-medium">{poolLabel(stat.pool)} pool</span>
          <span className="block">
            {stat.inFlight} of {stat.limit} slots busy
            {stat.queued > 0 ? `, ${stat.queued} waiting` : ''}
          </span>
          {bottleneck && (
            <span className="block text-[hsl(var(--warning))]">
              Saturated — this is the current bottleneck.
            </span>
          )}
        </span>
      }
    >
      <span
        className={cn(
          'flex items-center gap-1.5 rounded-full border px-1.5 py-[1px] transition-colors',
          bottleneck
            ? 'border-[hsl(var(--warning)/0.5)] bg-[hsl(var(--warning)/0.12)]'
            : idle
              ? 'border-transparent'
              : 'border-[hsl(var(--border))] bg-[hsl(var(--bg-hover))]',
        )}
      >
        <span
          className="text-[10px] font-semibold tracking-wide"
          style={{ color: idle ? 'hsl(var(--fg-subtle))' : `hsl(${hue})` }}
        >
          {SHORT[stat.pool] ?? stat.pool.toUpperCase()}
        </span>

        <span className="flex items-center gap-[2px]" aria-hidden>
          {Array.from({ length: slots }, (_, i) => (
            <span
              key={i}
              className={cn('h-2.5 w-[3px] rounded-[1px]', i < stat.inFlight && 'pulse-live')}
              style={{
                background:
                  i < stat.inFlight
                    ? bottleneck
                      ? 'hsl(var(--warning))'
                      : `hsl(${hue})`
                    : 'hsl(var(--fg) / 0.14)',
              }}
            />
          ))}
        </span>

        <span className={cn('tabular text-[10.5px]', idle ? 'text-subtle' : 'text-muted')}>
          {stat.inFlight}/{stat.limit}
        </span>

        {stat.queued > 0 && (
          <span
            className="tabular rounded-full px-1 text-[9.5px] font-medium leading-[14px]"
            style={{
              background: bottleneck ? 'hsl(var(--warning) / 0.25)' : 'hsl(var(--fg) / 0.1)',
              color: bottleneck ? 'hsl(var(--warning))' : 'hsl(var(--fg-muted))',
            }}
          >
            +{stat.queued}
          </span>
        )}
      </span>
    </Tooltip>
  )
})

/** The hue a pool is drawn in, for other views that want to match. */
export function poolHue(pool: PoolName): string {
  return POOL_HUE[pool] ?? 'var(--accent)'
}
