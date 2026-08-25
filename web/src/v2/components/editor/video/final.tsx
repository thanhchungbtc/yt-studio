import { ArrowUpRight } from 'lucide-react'

import type { Task, Video } from '../../../core/types'
import { cn } from '../../../core/utils'
import { openDoc } from '../dock'
import { Mark } from './mark'
import { videoStage, type Cell } from './stages'

/**
 * The four stages that belong to the video rather than to any chapter.
 *
 * They read in the same vocabulary as the grid above, which is the point: the
 * eye that has just learned five shapes does not have to learn a progress bar
 * as well. And they are the last row of the story the table tells — every
 * chapter is done, and then these four happen once.
 *
 * The thumbnail is a link, not a status. It is the only route to the thumbnail
 * builder in the whole window, and the video editor is where you would go
 * looking: the builder is a separate document because composing a picture is
 * separate work, not because it belongs to something else.
 */
export function FinalStrip({ video, tasks }: { video: Video; tasks: Task[] }) {
  const cut = videoStage(tasks, ['concat'], Boolean(video.finalAssetId))
  const metadata = videoStage(tasks, ['metadata'], Boolean(video.metadata?.title))
  const thumbnail = videoStage(
    tasks,
    ['thumbnail', 'thumbnail_plan', 'thumbnail_icon'],
    Boolean(video.effectiveThumbnailAssetId),
  )
  const upload = videoStage(tasks, ['upload'], Boolean(video.upload))

  return (
    <div className="surface-chrome hairline-t flex shrink-0 items-center gap-5 px-4 py-2">
      <span className="shrink-0 text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase">
        Final
      </span>
      <Stage cell={cut} label="Cut" />
      <Stage cell={metadata} label="Metadata" />
      <Stage
        cell={thumbnail}
        label="Thumbnail"
        onOpen={() =>
          openDoc({ kind: 'thumbnail', ref: video.ref }, `${video.title || video.ref} — thumbnail`)
        }
      />
      <Stage cell={upload} label="Upload" />
      {video.upload?.url ? (
        <a
          href={video.upload.url}
          target="_blank"
          rel="noreferrer"
          className="ml-auto shrink-0 truncate text-[12px] text-[var(--accent)] hover:underline"
        >
          {video.upload.dryRun ? 'Dry run' : 'Watch it'}
        </a>
      ) : null}
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
