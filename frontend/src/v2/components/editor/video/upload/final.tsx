import { ArrowUpRight } from 'lucide-react'

import type { Task, Video } from '../../../../core/types'
import { cn } from '../../../../core/utils'
import { Mark } from '../mark'
import { videoStage, type Cell } from '../stages'

/**
 * The four stages that belong to the video rather than to any chapter.
 *
 * They read in the same vocabulary the grid in Pipeline does, which is the
 * point: the eye that has learned five shapes over there does not have to learn
 * a second notation here.
 *
 * This is the summary line of the upload page, in the position Pipeline's
 * summary line occupies — one line saying what the page under it is made of.
 * Which of the four have happened is the fastest read of that, and the page
 * itself is the slow one.
 *
 * The receipt is deliberately not here. It used to end this row as a link
 * reading "Dry run" or "Watch it", which had to say whether a publish was real
 * in two words at the end of a strip; the status line below has the room to say
 * it properly, and only one of the two versions can be the one that is right.
 *
 * The thumbnail is a control, not a status: it opens the builder. Which is a
 * dialog rather than a document, so it opens *over* this video rather than into
 * a tab that could outlive it -- composing a picture is separate work, but it is
 * never work about a different video than the one in front of you.
 */
export function FinalStrip({
  video,
  tasks,
  onBuild,
}: {
  video: Video
  tasks: Task[]
  /** Opens the builder. The strip does not own it; see `UploadView`. */
  onBuild: () => void
}) {
  const cut = videoStage(tasks, ['concat'], Boolean(video.finalAssetId))
  const metadata = videoStage(tasks, ['metadata'], Boolean(video.metadata?.title))
  const thumbnail = videoStage(
    tasks,
    ['thumbnail', 'thumbnail_plan', 'thumbnail_icon'],
    Boolean(video.effectiveThumbnailAssetId),
  )
  const upload = videoStage(tasks, ['upload'], Boolean(video.upload))

  return (
    <div className="hairline-b flex shrink-0 items-center gap-5 px-4 py-2">
      <span className="shrink-0 text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase">
        Final
      </span>
      <Stage cell={cut} label="Cut" />
      <Stage cell={metadata} label="Metadata" />
      <Stage cell={thumbnail} label="Thumbnail" onOpen={onBuild} />
      <Stage cell={upload} label="Upload" />
    </div>
  )
}

function Stage({ cell, label, onOpen }: { cell: Cell; label: string; onOpen?: () => void }) {
  const content = (
    <>
      <Mark cell={cell} />
      <span className="text-[12px] whitespace-nowrap">{label}</span>
      {onOpen ? <ArrowUpRight className="size-3 opacity-60" strokeWidth={2} /> : null}
    </>
  )

  if (!onOpen) {
    return <span className="flex items-center gap-1.5 text-secondary">{content}</span>
  }
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        'flex items-center gap-1.5 rounded-[5px] px-1.5 py-0.5 -mx-1.5',
        'text-secondary transition-colors hover:bg-[var(--hover)] hover:text-primary',
      )}
    >
      {content}
    </button>
  )
}
