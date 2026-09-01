import { useQuery } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { useDock } from '../editor/dock'
import { Mark } from '../editor/video/mark'
import { pipelineStages, type PipelineStage } from '../editor/video/stages'

/**
 * The inspector: the pipeline of whatever document is in front.
 *
 * It is the grid rotated ninety degrees. The table in the editor reads chapters
 * down and stages across, so how far *one stage* has got is a column you have
 * to count; here the stages are the rows and the counting is done. Same tasks,
 * different question — how is chapter 2 doing, against how far along is the
 * video — which is why this is not a second copy of the table.
 *
 * Ten rows, always the same ten, in the order the work happens. A panel whose
 * length grew as the run progressed could not be read at a glance: you would
 * have to work out what was missing before you could see where you were. Fixed,
 * an open ring *is* the information.
 *
 * It follows the front tab rather than the sidebar's selection. Single-clicking
 * a row already opens the document, so binding to the selection would be a
 * second, slower answer to the question the tab strip has already answered.
 */
export function Inspector() {
  const doc = useDock((s) => s.activeDoc)

  if (doc?.kind === 'video') return <VideoPipeline videoRef={doc.ref} />
  return <Note>{doc ? 'Nothing to inspect here yet.' : 'Open a video to see its pipeline.'}</Note>
}

function VideoPipeline({ videoRef }: { videoRef: string }) {
  // The same three keys the editor uses, deliberately. Two panes asking the
  // same questions of one cache is one fetch and one answer; a private key here
  // would be a second copy of the video that drifts from the one on screen.
  const video = useQuery({
    queryKey: qk.video(videoRef),
    queryFn: () => api.getVideo(videoRef),
    enabled: Boolean(videoRef),
  })
  const id = video.data?.id
  const chapters = useQuery({
    queryKey: qk.chapters(id ?? ''),
    queryFn: () => api.listChapters(videoRef),
    enabled: Boolean(id),
  })
  const tasks = useQuery({
    queryKey: qk.tasks(id ?? ''),
    queryFn: () => api.listTasks(videoRef),
    enabled: Boolean(id),
  })

  const data = video.data
  const stages = useMemo(
    () => (data ? pipelineStages(data, chapters.data ?? [], tasks.data ?? []) : []),
    [data, chapters.data, tasks.data],
  )

  if (video.error) return <Note>That video could not be loaded.</Note>
  if (!data) return <div className="min-h-0 flex-1" />

  return (
    <div className="min-h-0 flex-1 overflow-y-auto py-1.5">
      {stages.map((stage) => (
        <StageRow key={stage.id} stage={stage} />
      ))}
    </div>
  )
}

/**
 * One stage: the mark, its name, and how far it has got.
 *
 * Nothing here is a button. This pane reports; the document acts. There was an
 * Approve on the gate row, which meant one gate wore two buttons — one here and
 * one in the strip beside the Reject that goes with it — and a pane that can be
 * hidden with ⌘3 is the wrong of the two to make load-bearing. So the gate is
 * still *reported*, in the slot the button used to sit in.
 *
 * The trailing edge holds exactly one thing, in this order: the gate when there
 * is one, then the count, then a percentage. They never collide, because the
 * three sets do not overlap — the only stages that can hold a gate are Blueprint
 * and Upload, and they are two of the five that happen once and have nothing to
 * count; and the only stage that reports a percentage is Cut, which is another.
 *
 * The count comes before the percentage deliberately. `12/21` says how much work
 * there is as well as how much is done, which a percentage cannot; a percentage
 * earns its place only where there is a single task and nothing to count.
 */
function StageRow({ stage }: { stage: PipelineStage }) {
  const gate = stage.gate
  // Read only while the stage is running. A delta that carries no percent does
  // not clear the last one, so a finished task keeps the figure it stopped at —
  // and 100% next to a settled disc would be the row saying the same thing
  // twice, in the slot kept for whatever is still moving.
  const percent = stage.cell.state === 'running' ? stage.cell.task?.percent : undefined

  const trailing = ((): ReactNode => {
    if (gate) {
      return (
        <span className="shrink-0 text-[11px] font-medium" style={{ color: 'var(--accent)' }}>
          Needs approval
        </span>
      )
    }
    if (stage.count) return <Figure>{`${stage.count.done}/${stage.count.total}`}</Figure>
    if (percent !== undefined) return <Figure>{`${percent}%`}</Figure>
    return null
  })()

  return (
    <div className="flex h-[27px] items-center gap-2.5 px-3">
      <Mark cell={stage.cell} />
      <span className="min-w-0 flex-1 truncate text-[12px] text-primary">{stage.label}</span>
      {trailing}
    </div>
  )
}

/** The figure on the trailing edge: tabular, so it does not jitter as it counts. */
function Figure({ children }: { children: ReactNode }) {
  return <span className="shrink-0 text-[11px] tabular-nums text-tertiary">{children}</span>
}

/**
 * What the pane says when there is nothing to inspect.
 *
 * One line of quiet text, not the editors' centred icon-over-two-lines. That
 * empty state is sized for a document; at sidebar width it reads as a screen
 * that failed to load rather than as a pane waiting for a selection.
 */
function Note({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-6">
      <p className="text-center text-[12px] text-tertiary">{children}</p>
    </div>
  )
}
