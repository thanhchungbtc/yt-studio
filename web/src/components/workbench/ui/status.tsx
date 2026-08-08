import { memo } from 'react'

import { Badge, TONE_FILL, type Tone } from './controls'
import { Tooltip } from './primitives'
import { poolLabel, taskStateLabel, videoStateLabel } from '@/core/format'
import type { PoolName, PoolStat, TaskState, VideoState } from '@/core/types'
import { cn } from '@/core/utils'

/** One tone per state, used by every place a state is drawn. */
export const VIDEO_TONES: Record<VideoState, Tone> = {
  draft: 'neutral',
  running: 'accent',
  awaiting_approval: 'warning',
  blocked: 'violet',
  completed: 'success',
  failed: 'danger',
  cancelled: 'neutral',
}

export const TASK_TONES: Record<TaskState, Tone> = {
  blocked: 'neutral',
  ready: 'info',
  running: 'accent',
  awaiting_approval: 'warning',
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'neutral',
}

export const VideoStateBadge = memo(function VideoStateBadge({
  state,
  className,
}: {
  state: VideoState
  className?: string
}) {
  return (
    <Badge tone={VIDEO_TONES[state]} dot pulse={state === 'running'} className={className}>
      {videoStateLabel(state)}
    </Badge>
  )
})

/** The badge reduced to its dot, where a neighbouring column carries the label. */
export const VideoStateDot = memo(function VideoStateDot({
  state,
  className,
}: {
  state: VideoState
  className?: string
}) {
  return (
    <span
      aria-hidden
      className={cn(
        'h-[7px] w-[7px] shrink-0 rounded-full',
        TONE_FILL[VIDEO_TONES[state]],
        state === 'running' && 'pulse-live',
        className,
      )}
    />
  )
})

export const TaskStateDot = memo(function TaskStateDot({
  state,
  stale,
  className,
}: {
  state: TaskState
  /** Outranks the state colour: a succeeded-but-stale row must not read green. */
  stale?: boolean
  className?: string
}) {
  return (
    <span
      aria-hidden
      className={cn(
        'h-[7px] w-[7px] shrink-0 rounded-full',
        stale ? TONE_FILL.warning : TONE_FILL[TASK_TONES[state]],
        state === 'running' && 'pulse-live',
        state === 'blocked' && !stale && 'opacity-60',
        className,
      )}
    />
  )
})

export function TaskStateBadge({ state, className }: { state: TaskState; className?: string }) {
  return (
    <Badge tone={TASK_TONES[state]} dot pulse={state === 'running'} className={className}>
      {taskStateLabel(state)}
    </Badge>
  )
}

/* ------------------------------------------------------------------ pools */

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
 * A pool's occupancy as filled slots plus a queue badge. Capacity is the binding
 * constraint of the whole system, so it lives in the status bar where it is
 * always visible.
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

/** The same occupancy at console scale, where there is room for every slot. */
export const PoolMeter = memo(function PoolMeter({ stat }: { stat: PoolStat }) {
  const hue = POOL_HUE[stat.pool] ?? 'var(--accent)'
  const slots = Math.min(stat.limit, 16)
  const saturated = stat.limit > 0 && stat.inFlight >= stat.limit
  const bottleneck = saturated && stat.queued > 0

  return (
    <div className="flex items-center gap-2.5">
      <span className="w-16 shrink-0 text-[11.5px] font-medium text-fg">
        {poolLabel(stat.pool)}
      </span>
      <span className="flex flex-1 items-center gap-[3px]" aria-hidden>
        {Array.from({ length: slots }, (_, i) => (
          <span
            key={i}
            className={cn('h-3.5 flex-1 rounded-[2px]', i < stat.inFlight && 'pulse-live')}
            style={{
              background:
                i < stat.inFlight
                  ? bottleneck
                    ? 'hsl(var(--warning))'
                    : `hsl(${hue})`
                  : 'hsl(var(--fg) / 0.1)',
            }}
          />
        ))}
      </span>
      <span className="tabular w-20 shrink-0 text-right text-[11px] text-muted">
        {stat.inFlight}/{stat.limit}
        {stat.queued > 0 && (
          <span className={bottleneck ? 'text-[hsl(var(--warning))]' : 'text-subtle'}>
            {' '}
            +{stat.queued}
          </span>
        )}
      </span>
    </div>
  )
})
