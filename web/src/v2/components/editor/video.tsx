import { useQuery } from '@tanstack/react-query'
import type { IDockviewPanelProps } from 'dockview-react'
import { Clapperboard } from 'lucide-react'
import { useState, type ComponentType, type ReactNode } from 'react'

import { api, qk } from '../../core/api'
import type { VideoState } from '../../core/types'
import { Segmented, type Segment } from '../ui/segmented'
import type { DocPanelParams } from './dock'
import { Placeholder } from './placeholder'
import { EditorShell } from './shell'
import { PipelineView } from './video/pipeline'
import { ScriptView } from './video/script'
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
type Mode = 'pipeline' | 'script' | 'upload'

interface ModeEntry extends Segment<Mode> {
  View: ComponentType<ViewProps>
}

const MODES: readonly ModeEntry[] = [
  { value: 'pipeline', label: 'Pipeline', View: PipelineView },
  { value: 'script', label: 'Script', View: ScriptView },
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

  // The fallback is unreachable — `mode` is only ever set from this table —
  // and it is the mode the editor opens in, so nothing is hidden if it ever is.
  const View = MODES.find((entry) => entry.value === mode)?.View ?? PipelineView

  // The strip stays in every mode. It is the only thing on screen that can be
  // waiting for an answer, and a gate that vanished because you went to read
  // the script would be the document hiding the one thing it needs from you.
  return shell(
    <div className="flex h-full min-h-0 flex-col">
      <StoppedStrip video={video.data} tasks={tasks.data ?? []} />
      <View video={video.data} chapters={chapters.data ?? []} tasks={tasks.data ?? []} />
    </div>,
  )
}
