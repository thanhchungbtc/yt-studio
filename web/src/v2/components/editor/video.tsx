import { useQuery } from '@tanstack/react-query'
import type { IDockviewPanelProps } from 'dockview-react'
import { Clapperboard } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { count, duration } from '../../core/format'
import type { Video, VideoState } from '../../core/types'
import { Button } from '../ui/button'
import { Segmented, type Segment } from '../ui/segmented'
import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'
import { ChapterTable } from './video/table'
import { BlueprintPopover } from './video/blueprint'
import { FinalStrip } from './video/final'
import { Legend } from './video/mark'
import { ChapterReader } from './video/reader'
import { StoppedStrip } from './video/stopped'
import { columnTotals, projectedSeconds } from './video/stages'

/**
 * The video editor.
 *
 * One document, one object, no view tabs. Everything on screen is the same
 * video at a different altitude: what state it is in, what it is waiting for,
 * what each chapter has produced, and what happens once every chapter is done.
 *
 * Read-only over the artifacts, deliberately. The pipeline runs on its own, so
 * what a person needs first is to see where it got to and to get it moving
 * again — which is the strip at the top, and only the strip. Opening an
 * artifact, rewriting a script and retrying a single failed task are each worth
 * their own step, and each is easier to get right once this has been watched
 * running for real.
 *
 * The *plan* is the exception, and it is not really one: the blueprint's titles,
 * briefs and word budgets are the pipeline's inputs rather than its output, and
 * they are what the gate is asking about. Edit puts the table's first three
 * columns into fields; nothing is re-run, because a chapter that has not been
 * written yet is written from the edit, and one that has been is the operator's
 * to reconsider.
 */

/**
 * The two altitudes on one video, and the whole of the switch between them.
 *
 * Not view tabs by another name. Tabs would cut the object into *parts* —
 * chapters here, artifacts there — along a seam that does not exist. These two
 * are the same chapters in the same order either way; what changes is whether a
 * chapter is a row of marks or the words it is made of. A mark can never show
 * you a picture, and that is the one thing the table structurally cannot do.
 */
type Mode = 'pipeline' | 'script'

const MODES: readonly Segment<Mode>[] = [
  { value: 'pipeline', label: 'Pipeline' },
  { value: 'script', label: 'Script' },
]

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
  // Held here rather than in the table because the switch belongs on the line
  // that describes the plan, and the table is what the switch changes.
  const [editing, setEditing] = useState(false)
  // Per document and no further. Which altitude you were last at is not worth a
  // line in the store, and a tab that reopened in a mode you had forgotten
  // choosing would be answering a question nobody asked.
  const [mode, setMode] = useState<Mode>('pipeline')

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
      actions={<Segmented segments={MODES} value={mode} onChange={setMode} />}
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

  // There is nothing to edit until the blueprint has written the rows, and a
  // switch that turns an empty table into an empty table is a switch that
  // teaches the wrong thing about what it does.
  const editable = (chapters.data?.length ?? 0) > 0

  // The strip stays in both. It is the only thing on screen that can be waiting
  // for an answer, and a gate that vanished because you went to read the script
  // would be the document hiding the one thing it needs from you.
  return shell(
    <div className="flex h-full min-h-0 flex-col">
      <StoppedStrip video={video.data} tasks={tasks.data ?? []} />

      {mode === 'script' ? (
        <ChapterReader
          chapters={chapters.data ?? []}
          tasks={tasks.data ?? []}
          slidesPerChapter={video.data.slidesPerChapter}
        />
      ) : (
        <>
          <SummaryLine
            video={video.data}
            totals={totals}
            editing={editing && editable}
            editable={editable}
            onToggleEditing={() => setEditing((on) => !on)}
          />

          <ChapterTable
            videoId={video.data.id}
            chapters={chapters.data ?? []}
            tasks={tasks.data ?? []}
            slidesPerChapter={video.data.slidesPerChapter}
            editing={editing && editable}
          />

          <Legend />
          <FinalStrip video={video.data} tasks={tasks.data ?? []} />
        </>
      )}
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

function SummaryLine({
  video,
  totals,
  editing,
  editable,
  onToggleEditing,
}: {
  video: Video
  totals: ReturnType<typeof columnTotals>
  editing: boolean
  editable: boolean
  onToggleEditing: () => void
}) {
  return (
    <div className="hairline-b flex shrink-0 items-center gap-2 px-4 py-2">
      <span className="min-w-0 flex-1 truncate text-[12px] text-secondary">
        {shapeOf(video, totals)} · {video.slidesPerChapter} slides each
      </span>
      {/*
        This line describes the plan, so the way to the plan belongs on it —
        reading it, then changing it. Each hidden until there is one.

        Those two and nothing else. Cancel used to sit here as well, one tab stop
        from Edit and the same size, which put the verb that kills a render in
        progress beside a switch that restyles some text. It lives with the other
        lifecycle verbs now, in the strip above.
      */}
      {video.blueprintAssetId ? <BlueprintPopover assetId={video.blueprintAssetId} /> : null}
      {editable ? <Button onClick={onToggleEditing}>{editing ? 'Done' : 'Edit'}</Button> : null}
    </div>
  )
}
