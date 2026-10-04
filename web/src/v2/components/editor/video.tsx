import type { IDockviewPanelProps } from 'dockview-react'
import { Clapperboard } from 'lucide-react'
import {
  memo,
  useDeferredValue,
  useMemo,
  useState,
  type ComponentType,
  type ReactNode,
} from 'react'

import { useVideoDoc } from '../../core/queries'
import type { Chapter, Task, VideoState } from '../../core/types'
import { Segmented, type Segment } from '../ui/segmented'
import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'
import { PipelineView } from './video/pipeline'
import { ChaptersView } from './video/chapters'
import { StoppedStrip } from './video/stopped'
import { UploadView } from './video/upload'
import type { ViewProps } from './video/view'

/**
 * The video editor.
 *
 * One document, one object. Everything on screen is the same video at a
 * different altitude, and the mode switch is the whole of the way between
 * them: did this happen, what does it say, what does it become.
 *
 * Not view tabs by another name. Tabs would cut the object into *parts* —
 * chapters here, artifacts there — and reaching one would be navigation. These
 * are three readings of the whole thing, and each is complete on its own.
 *
 * This file owns what all three share and nothing else: the three queries, the
 * frame, the strip that can be waiting for an answer, and the list of modes.
 * Everything a single mode needs lives in that mode's folder, which is what
 * makes a fourth one a new folder and a new row rather than an edit here.
 */

/**
 * The modes, in the order they are read in.
 *
 * A table rather than a branch. Every view takes `ViewProps` and nothing else,
 * so adding one is a folder and a line, and no mode can grow a prop that the
 * editor has to learn about.
 */
type Mode = 'pipeline' | 'chapters' | 'upload'

interface ModeEntry extends Segment<Mode> {
  View: ComponentType<ViewProps>
}

const MODES: readonly ModeEntry[] = [
  { value: 'pipeline', label: 'Pipeline', View: PipelineView },
  { value: 'chapters', label: 'Chapters', View: ChaptersView },
  { value: 'upload', label: 'Upload', View: UploadView },
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
  // Per document and no further. Which altitude you were last at is not worth a
  // line in the store, and a tab that reopened in a mode you had forgotten
  // choosing would be answering a question nobody asked.
  const [mode, setMode] = useState<Mode>('pipeline')

  const { video, chapters, tasks, ready } = useVideoDoc(ref)

  // The chrome answers at once; the body renders just behind it. A mode, or a
  // document, with fifty chapters in it is a big tree to build, and built in
  // the same pass as the click it held the click hostage — the segmented
  // control, the tab and the selected row all waited on it. Deferred, the
  // control moves and the tab appears on the next frame, and React builds the
  // body in the background, dropping the work if another click supersedes it.
  const shownMode = useDeferredValue(mode)
  // The body waits for all three answers, not just the record. A table drawn
  // before its chapters have arrived says "No chapters yet" about a video that
  // has eighty — a flash of something false, which is worse than a blank.
  // A failed query counts as answered, so its error has somewhere to show.
  const loaded = ready && !chapters.isPending && !tasks.isPending
  const showBody = useDeferredValue(loaded, false)

  // Every mode that has been on screen, so it can stay built behind the others.
  // Grown during render rather than in an effect: the mode has to be in the set
  // on the same pass it is shown, or it would flash empty for a frame.
  const [visited, setVisited] = useState<ReadonlySet<Mode>>(() => new Set([shownMode]))
  if (!visited.has(shownMode)) setVisited(new Set([...visited, shownMode]))

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
  // The body waits for the record proper. A library row is enough to name the
  // document, but not necessarily everything a mode reads off it.
  const record = video.data
  if (!record || !showBody) {
    return shell(<Pending />)
  }

  // The strip stays in every mode. It is the only thing on screen that can be
  // waiting for an answer, and a gate that vanished because you went to read
  // the script would be the document hiding the one thing it needs from you.
  //
  // A mode, once visited, stays built. Coming back to it is a visibility flip
  // that keeps its scroll position, rather than a fifty-chapter document laid
  // out again from nothing inside the click; see `.mode-pane`.
  return shell(
    <div className="flex h-full min-h-0 flex-col">
      <StoppedStrip video={record} tasks={tasks.data ?? []} />
      <div className="relative min-h-0 flex-1">
        {MODES.filter((entry) => visited.has(entry.value)).map(({ value, View }) => (
          <ModePane
            key={value}
            View={View}
            hidden={value !== shownMode}
            video={record}
            chapters={chapters.data ?? NO_CHAPTERS}
            tasks={tasks.data ?? NO_TASKS}
          />
        ))}
      </div>
    </div>,
  )
}

const NO_CHAPTERS: Chapter[] = []
const NO_TASKS: Task[] = []

/**
 * One mode, built once and kept.
 *
 * Memoised, and that is the point of it: switching modes re-renders the editor,
 * and without this every mode kept behind the shown one would be rebuilt along
 * with it — three documents' worth of work for a click that changes which one
 * is visible. Its props are cache entries, which keep their identity until the
 * stream actually changes them.
 */
const ModePane = memo(function ModePane({
  View,
  hidden,
  ...props
}: ViewProps & { View: ComponentType<ViewProps>; hidden: boolean }) {
  const { video, chapters, tasks } = props
  // Showing or hiding a mode is an attribute on this wrapper and nothing more.
  // Without this the whole view re-rendered on every switch — both the one
  // going away and the one coming forward — for a change neither of them can
  // see.
  const view = useMemo(
    () => <View video={video} chapters={chapters} tasks={tasks} />,
    [View, video, chapters, tasks],
  )
  return (
    <div className="mode-pane absolute inset-0 flex flex-col" data-hidden={hidden ? '' : undefined}>
      {view}
    </div>
  )
})

/**
 * The body while its data is on the way.
 *
 * Nothing at first: against a local server the wait is a frame or two, and a
 * spinner that blinks on and off for that long is noise. If it runs long, a
 * small one fades in — the macOS rule of showing progress only once there is
 * progress worth showing.
 */
function Pending() {
  return (
    <div className="flex h-full items-center justify-center">
      <span className="pending-in mark-spinner block size-4 rounded-full" aria-label="Loading" />
    </div>
  )
}
