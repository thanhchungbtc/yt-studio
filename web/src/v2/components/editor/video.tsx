import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { IDockviewPanelProps } from 'dockview-react'
import { Clapperboard } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { count, duration } from '../../core/format'
import type { Video, VideoState } from '../../core/types'
import { Button } from '../ui/button'
import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'
import { ChapterTable } from './video/table'
import { FinalStrip } from './video/final'
import { Legend } from './video/mark'
import { GateStrip } from './video/gate'
import { columnTotals, projectedSeconds } from './video/stages'

/**
 * The video editor.
 *
 * One document, one object, no view tabs. Everything on screen is the same
 * video at a different altitude: what state it is in, what it is waiting for,
 * what each chapter has produced, and what happens once every chapter is done.
 *
 * Read-only, deliberately. The pipeline runs on its own and the first thing a
 * person actually needs is to see where it got to and to unblock it — which is
 * the gate, and only the gate. Opening an artifact, rewriting a script and
 * retrying a failure are each worth their own step, and each is easier to get
 * right once this has been watched running for real.
 */

const STATUS: Record<VideoState, { label: string; color: string }> = {
  draft: { label: 'Draft', color: 'var(--text-tertiary)' },
  running: { label: 'Running', color: 'var(--running)' },
  awaiting_approval: { label: 'Needs approval', color: 'var(--accent)' },
  blocked: { label: 'Blocked', color: 'var(--failed)' },
  completed: { label: 'Completed', color: 'var(--done)' },
  failed: { label: 'Failed', color: 'var(--failed)' },
  cancelled: { label: 'Cancelled', color: 'var(--text-tertiary)' },
}

export function VideoEditor({ params }: IDockviewPanelProps<DocPanelParams>) {
  const ref = params.doc?.kind === 'video' ? params.doc.ref : ''

  const video = useQuery({
    queryKey: qk.video(ref),
    queryFn: () => api.getVideo(ref),
    enabled: Boolean(ref),
  })
  const id = video.data?.id

  // Everything inside a video is keyed by its id, because that is what a delta
  // carries; see the note on `qk`. Both wait for the video for the same reason.
  const chapters = useQuery({
    queryKey: qk.chapters(id ?? ''),
    queryFn: () => api.listChapters(ref),
    enabled: Boolean(id),
  })
  const tasks = useQuery({
    queryKey: qk.tasks(id ?? ''),
    queryFn: () => api.listTasks(ref),
    enabled: Boolean(id),
  })

  const totals = useMemo(
    () => columnTotals(chapters.data ?? [], video.data?.slidesPerChapter ?? 0),
    [chapters.data, video.data?.slidesPerChapter],
  )

  const gate = (tasks.data ?? []).find((task) => task.state === 'awaiting_approval')
  const status = video.data ? STATUS[video.data.state] : undefined

  const shell = (children: ReactNode) => (
    <EditorShell
      title={video.data?.title || params.title}
      seed={params.seed}
      initial={params.initial}
      status={
        status ? (
          <>
            {status.label} · {ref}
            {video.data && video.data.counts.total > 0
              ? ` · ${video.data.counts.succeeded} of ${video.data.counts.total} done`
              : ''}
          </>
        ) : (
          ref
        )
      }
      statusColor={status?.color}
    >
      {children}
    </EditorShell>
  )

  if (video.error) {
    return shell(
      <Placeholder
        icon={Clapperboard}
        title="That video could not be loaded"
        detail={(video.error as Error).message}
      />,
    )
  }
  if (!video.data) {
    return shell(<div className="h-full" />)
  }

  return shell(
    <div className="flex h-full min-h-0 flex-col">
      {gate ? <GateStrip videoRef={ref} videoId={video.data.id} task={gate} /> : null}

      <SummaryLine video={video.data} totals={totals} />

      <ChapterTable
        chapters={chapters.data ?? []}
        tasks={tasks.data ?? []}
        slidesPerChapter={video.data.slidesPerChapter}
      />

      <Legend />
      <FinalStrip video={video.data} tasks={tasks.data ?? []} />
    </div>,
  )
}

/** The shape of the thing: what it is made of, and how long it runs. */
function shapeOf(video: Video, totals: ReturnType<typeof columnTotals>): string {
  const words = totals.words || totals.estimatedWords
  // Always a projection, never a measurement: the seconds are the sum of each
  // chapter's words at the narration speed the blueprint budgeted with.
  //
  // Before any script is written that sum is zero, and this used to fall back
  // to the *target* duration — which is what was asked for, not what was
  // planned. A blueprint that budgets nine hundred words against a two-minute
  // target then reported "~2m" for seven minutes of narration. Projecting the
  // budget says what the plan actually is, which is the number you approve on.
  const runtime = duration(totals.seconds > 0 ? totals.seconds : projectedSeconds(words))
  return `${video.chapterCount} chapters · ${count(words)} words · ~${runtime}`
}

function SummaryLine({ video, totals }: { video: Video; totals: ReturnType<typeof columnTotals> }) {
  const client = useQueryClient()
  const cancel = useMutation({
    mutationFn: () => api.cancelVideo(video.ref),
    onSuccess: () => client.invalidateQueries({ queryKey: qk.video(video.ref) }),
  })

  const running = video.state === 'running' || video.state === 'awaiting_approval'

  return (
    <div className="hairline-b flex shrink-0 items-center gap-2 px-4 py-2">
      <span className="min-w-0 flex-1 truncate text-[12px] text-secondary">
        {shapeOf(video, totals)} · {video.slidesPerChapter} slides each
      </span>
      {video.error ? (
        <span className="min-w-0 shrink truncate text-[12px] text-[var(--failed)]">
          {video.error}
        </span>
      ) : null}
      {running ? (
        <Button onClick={() => cancel.mutate()} disabled={cancel.isPending}>
          Cancel
        </Button>
      ) : null}
    </div>
  )
}
