import { useQuery } from '@tanstack/react-query'
import { useMemo, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import { useDock } from '../editor/dock'
import { Mark } from '../editor/video/mark'
import { useStageMenu } from '../editor/video/pipeline/regenerate'
import { pipelineStages, type PipelineStage } from '../editor/video/stages'
import type { MenuItem } from '../ui/menu'

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
  // Hooks run before the early returns below, so the empty id is the one render
  // where there is no video yet — and `menuFor` is never called in it, because
  // there are no stages to call it for.
  const { menuFor, error } = useStageMenu(data?.id ?? '', tasks.data ?? [])

  if (video.error) return <Note>That video could not be loaded.</Note>
  if (!data) return <div className="min-h-0 flex-1" />

  return (
    <div className="min-h-0 flex-1 overflow-y-auto py-1.5">
      {/* Above the rows rather than beside the dot that caused it: the menu has
          closed by the time this exists, and the pane is too narrow to hang a
          message off a twelve-pixel target. One at a time, as there is one
          press at a time. */}
      {error ? (
        <p className="px-3 pb-1.5 text-[11px] text-[var(--failed)]">{error.message}</p>
      ) : null}

      {stages.map((stage) => (
        <StageRow key={stage.id} stage={stage} menu={menuFor(stage)} />
      ))}
    </div>
  )
}

/**
 * One stage: the mark, its name, and how far it has got.
 *
 * The mark is the one control, and only on four of the ten rows. This pane used
 * to hold no buttons at all, and the rule it was keeping is narrower than it
 * read: there was an Approve on the gate row, which meant one gate wore two
 * buttons — one here and one in the strip beside the Reject that goes with it —
 * and a pane that can be hidden with ⌘3 is the wrong of the two to make
 * load-bearing. Both halves of that were about Approve. It duplicated a verb
 * the document already had, and it was the only way out of a blocked video.
 *
 * Regenerating a whole stage is neither. It exists nowhere else — the table
 * offers one dot at a time, and only four of these ten stages are columns in
 * it at all — and hiding this pane costs a shortcut rather than an exit, since
 * every one of those presses can still be made cell by cell. A stage is also
 * the only object either pane has that *is* the whole stage, which is what the
 * verb is about. So the gate stays reported and the four settled stages act.
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
function StageRow({ stage, menu }: { stage: PipelineStage; menu: MenuItem[] }) {
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
      <Mark cell={stage.cell} menu={menu} />
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
