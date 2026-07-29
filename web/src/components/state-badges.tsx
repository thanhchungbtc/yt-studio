import { memo } from 'react'

import { Badge, type Tone } from '@/components/ui/badge'
import { taskStateLabel, videoStateLabel } from '@/lib/format'
import type { TaskState, VideoState } from '@/lib/types'

const VIDEO_TONES: Record<VideoState, Tone> = {
  draft: 'neutral',
  running: 'accent',
  awaiting_approval: 'warning',
  blocked: 'violet',
  completed: 'success',
  failed: 'danger',
  cancelled: 'neutral',
}

export const VideoStateBadge = memo(function VideoStateBadge({ state }: { state: VideoState }) {
  return (
    <Badge tone={VIDEO_TONES[state]} dot pulse={state === 'running'}>
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
