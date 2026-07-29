import { memo } from 'react'

import { Badge, type Tone } from '@/components/ui/badge'
import { taskStateLabel, videoStateLabel } from '@/lib/format'
import type { TaskState, VideoState } from '@/lib/types'
import { cn } from '@/lib/utils'

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

const TASK_TONES: Record<TaskState, Tone> = {
  blocked: 'neutral',
  ready: 'info',
  running: 'accent',
  awaiting_approval: 'warning',
  succeeded: 'success',
  failed: 'danger',
  cancelled: 'neutral',
}

export const TaskStateBadge = memo(function TaskStateBadge({ state }: { state: TaskState }) {
  return (
    <Badge tone={TASK_TONES[state]} dot pulse={state === 'running'}>
      {taskStateLabel(state)}
    </Badge>
  )
})

const TONE_FILL: Record<Tone, string> = {
  neutral: 'bg-[hsl(var(--fg-subtle))]',
  accent: 'bg-[hsl(var(--accent))]',
  success: 'bg-[hsl(var(--success))]',
  warning: 'bg-[hsl(var(--warning))]',
  danger: 'bg-[hsl(var(--danger))]',
  info: 'bg-[hsl(var(--info))]',
  violet: 'bg-[hsl(var(--violet))]',
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
