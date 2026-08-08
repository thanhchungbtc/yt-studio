import { memo } from 'react'

import { Badge, TONE_FILL, type Tone } from '@/components/ui/badge'
import { taskStateLabel, videoStateLabel } from '@/core/format'
import type { TaskState, VideoState } from '@/core/types'
import { cn } from '@/core/utils'

export const VIDEO_TONES: Record<VideoState, Tone> = {
  draft: 'neutral',
  running: 'accent',
  awaiting_approval: 'warning',
  blocked: 'violet',
  completed: 'success',
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

export const TASK_TONES: Record<TaskState, Tone> = {
  blocked: 'neutral',
  ready: 'info',
  running: 'accent',
  awaiting_approval: 'warning',
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'neutral',
}

export const TaskStateBadge = memo(function TaskStateBadge({
  state,
  className,
}: {
  state: TaskState
  className?: string
}) {
  return (
    <Badge tone={TASK_TONES[state]} dot pulse={state === 'running'} className={className}>
      {taskStateLabel(state)}
    </Badge>
  )
})

/**
 * The task badge reduced to its dot. A table row already names the task in the
 * column beside it, so the colour is all the badge was adding.
 */
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

export function taskStateFill(state: TaskState): string {
  return TONE_FILL[TASK_TONES[state]]
}

/**
 * The badge reduced to its dot, for list rows where the label is already
 * carried by a neighbouring column and the colour is doing all the work.
 */
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

export function videoStateFill(state: VideoState): string {
  return TONE_FILL[VIDEO_TONES[state]]
}
