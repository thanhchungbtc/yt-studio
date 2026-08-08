import { memo } from 'react'

import { Tooltip } from '@/components/ui/primitives'
import { poolLabel } from '@/core/format'
import type { PoolStat } from '@/core/types'
import { cn } from '@/core/utils'

/**
 * One pool's occupancy as discrete slots, so "2 of 2 busy with 40 queued" reads
 * at a glance — capacity is the binding constraint, and the operator console
 * exists to show where it is going.
 */
export const PoolMeter = memo(function PoolMeter({
  stat,
  compact,
}: {
  stat: PoolStat
  compact?: boolean
}) {
  const slots = Math.min(stat.limit, 16)
  const saturated = stat.inFlight >= stat.limit && stat.limit > 0
  const bottleneck = saturated && stat.queued > 0

  return (
    <Tooltip
      label={
        <span>
          <strong>{poolLabel(stat.pool)}</strong> — {stat.inFlight} of {stat.limit} slots busy
          {stat.queued > 0 ? `, ${stat.queued} queued` : ''}
          {bottleneck ? ' · this pool is the bottleneck' : ''}
        </span>
      }
    >
      <div className={cn('flex items-center gap-2', compact ? 'text-[11px]' : 'text-[12px]')}>
        <span
          className={cn(
            'w-16 shrink-0 font-medium',
            bottleneck ? 'text-[hsl(var(--warning))]' : 'text-muted',
          )}
        >
          {poolLabel(stat.pool)}
        </span>
        <span className="flex items-center gap-[3px]" aria-hidden>
          {Array.from({ length: slots }, (_, i) => (
            <span
              key={i}
              className={cn(
                'h-3 w-[5px] rounded-[1px] transition-colors',
                i < stat.inFlight
                  ? bottleneck
                    ? 'bg-[hsl(var(--warning))]'
                    : 'bg-[hsl(var(--accent))]'
                  : 'bg-[hsl(var(--bg-hover))]',
              )}
            />
          ))}
          {stat.limit > slots && <span className="text-subtle">+{stat.limit - slots}</span>}
        </span>
        <span className="tabular text-subtle">
          {stat.inFlight}/{stat.limit}
        </span>
        {stat.queued > 0 && (
          <span className="tabular rounded-full bg-[hsl(var(--bg-hover))] px-1.5 text-[10.5px] text-muted">
            {stat.queued} queued
          </span>
        )}
      </div>
    </Tooltip>
  )
})
