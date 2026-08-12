import { useVirtualizer } from '@tanstack/react-virtual'
import { FileText, MoreHorizontal, RotateCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'

import { useAssetViewer } from '@/components/asset-viewer'
import { ContextMenu, MenuItem, MenuLabel } from '../../ui/menu'
import { EmptyState, Skeleton, Tooltip } from '../../ui/primitives'
import { api, qk } from '@/core/api'
import type { ViewerItem } from '@/core/assets'
import type { Chapter, Task, Video } from '@/core/types'
import { cn } from '@/core/utils'
import { ClipCell, NarrationCell, ScriptCell, SlidesCell } from './cells'
import { ScriptDialog } from './script-dialog'
import { SLIDES_COLUMN, columnTotals, slideThumbWidth, stagesByChapter, wordsIn } from './stages'

const ROW = 60
const HEAD = 38

/** The grid, in one place so the header and every row cannot disagree. */
const COLUMNS = `40px minmax(240px,1fr) 76px 92px ${SLIDES_COLUMN}px 56px 36px`
const MIN_WIDTH = 40 + 240 + 76 + 92 + SLIDES_COLUMN + 56 + 36

/**
 * The blueprint, as a table.
 *
 * The layout carries one idea: the Chapter column is the plan and everything to
 * its right is the reality. So the left of the row is frozen at approval — the
 * title, the summary and the word budget you signed off on never move — and the
 * columns beside it fill in as the pipeline runs.
 *
 * The payoff is vertical, not horizontal. Reading *down* the slides column tells
 * you image generation has reached chapter twelve; no single row can say that,
 * which is why this replaced an accordion.
 */
export function BlueprintTable({
  video,
  chapters,
  tasks,
  artifacts,
  loading,
}: {
  video: Video
  chapters: Chapter[]
  tasks: Task[]
  /** Every artifact the video owns, so the lightbox can walk all of them. */
  artifacts: ViewerItem[]
  loading: boolean
}) {
  const openViewer = useAssetViewer()
  const parent = useRef<HTMLDivElement>(null)
  const [scripting, setScripting] = useState<Chapter | null>(null)

  const stages = useMemo(
    () => stagesByChapter(chapters, tasks, video.slidesPerChapter),
    [chapters, tasks, video.slidesPerChapter],
  )
  const totals = useMemo(
    () => columnTotals(chapters, video.slidesPerChapter),
    [chapters, video.slidesPerChapter],
  )
  const thumbWidth = useMemo(
    () => slideThumbWidth(Math.max(1, video.slidesPerChapter)),
    [video.slidesPerChapter],
  )

  const rows = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW,
    overscan: 10,
  })

  /** Opens the lightbox on one artifact, able to walk every other one from there. */
  const open = (assetId: string | undefined) => {
    if (!assetId) return
    const index = artifacts.findIndex((item) => item.id === assetId)
    if (index >= 0) openViewer(artifacts, index)
  }

  if (loading) {
    return (
      <div className="space-y-1 p-3">
        {Array.from({ length: 10 }, (_, i) => (
          <Skeleton key={i} className="h-[60px] w-full" />
        ))}
      </div>
    )
  }
  if (chapters.length === 0) {
    return (
      <EmptyState
        icon={<FileText />}
        title="No chapters yet"
        description="The blueprint decides how many there are. Approve it and they appear here."
      />
    )
  }

  return (
    <>
      <div ref={parent} className="h-full overflow-auto">
        <div style={{ minWidth: MIN_WIDTH }}>
          {/* The header is the progress summary. `SLIDES 24/80` says how far one
              stage has got across the whole video, which is the question a table
              can answer and a list cannot. */}
          <div
            className="sticky top-0 z-20 grid items-center border-b border-[hsl(var(--border))] bg-subtle no-select"
            style={{ gridTemplateColumns: COLUMNS, height: HEAD }}
          >
            <HeadCell className="sticky left-0 z-10 bg-subtle text-right">#</HeadCell>
            <HeadCell className="sticky left-10 z-10 bg-subtle">
              Chapter
              <Count done={chapters.length} />
            </HeadCell>
            <HeadCell>
              Script
              <Count done={totals.script.done} total={totals.script.total} />
            </HeadCell>
            <HeadCell>
              Narration
              <Count done={totals.narration.done} total={totals.narration.total} />
            </HeadCell>
            <HeadCell>
              Slides
              <Count done={totals.slides.done} total={totals.slides.total} />
            </HeadCell>
            <HeadCell>
              Clip
              <Count done={totals.clip.done} total={totals.clip.total} />
            </HeadCell>
            <HeadCell />
          </div>

          <div className="relative" style={{ height: rows.getTotalSize() }}>
            {rows.getVirtualItems().map((item) => {
              const chapter = chapters[item.index]
              if (!chapter) return null
              const stage = stages.get(chapter.id)
              if (!stage) return null
              return (
                <Row
                  key={chapter.id}
                  chapter={chapter}
                  stage={stage}
                  video={video}
                  thumbWidth={thumbWidth}
                  top={item.start}
                  onOpenScript={() => setScripting(chapter)}
                  onOpenAsset={open}
                />
              )
            })}
          </div>
        </div>
      </div>

      {scripting && (
        <ScriptDialog
          key={scripting.id}
          // Read back out of the list so a save is reflected without reopening.
          chapter={chapters.find((c) => c.id === scripting.id) ?? scripting}
          videoRef={video.ref}
          videoId={video.id}
          estimatedWords={scripting.estimatedWords}
          onClose={() => setScripting(null)}
        />
      )}
    </>
  )
}

function HeadCell({ className, children }: { className?: string; children?: ReactNode }) {
  return (
    <div
      className={cn(
        'flex h-full items-center gap-1.5 px-2 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-subtle',
        className,
      )}
    >
      {children}
    </div>
  )
}

function Count({ done, total }: { done: number; total?: number }) {
  const complete = total !== undefined && done >= total && total > 0
  return (
    <span
      className={cn(
        'tabular rounded-full px-1 text-[10px] font-medium leading-[15px]',
        complete
          ? 'bg-[hsl(var(--success)/0.16)] text-[hsl(var(--success))]'
          : 'bg-[hsl(var(--fg)/0.07)] text-subtle',
      )}
    >
      {total === undefined ? done : `${done}/${total}`}
    </span>
  )
}

function Row({
  chapter,
  stage,
  video,
  thumbWidth,
  top,
  onOpenScript,
  onOpenAsset,
}: {
  chapter: Chapter
  stage: ReturnType<typeof stagesByChapter> extends Map<string, infer S> ? S : never
  video: Video
  thumbWidth: number | null
  top: number
  onOpenScript: () => void
  onOpenAsset: (assetId: string | undefined) => void
}) {
  const queryClient = useQueryClient()
  const retryChapter = useMutation({
    mutationFn: () => api.retryChapter(video.ref, chapter.ordinal),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: qk.videoTasks(video.id) })
      void queryClient.invalidateQueries({ queryKey: qk.video(video.ref) })
    },
  })

  const words = wordsIn(chapter.script)

  return (
    <ContextMenu
      items={
        <>
          <MenuLabel>{`${chapter.ordinal}. ${chapter.title}`}</MenuLabel>
          <MenuItem onSelect={onOpenScript}>Open the script</MenuItem>
          <MenuItem onSelect={() => retryChapter.mutate()}>Re-run this chapter</MenuItem>
        </>
      }
    >
      <div
        data-chapter-row={chapter.ordinal}
        className="group absolute inset-x-0 grid items-center border-b border-[hsl(var(--border))] bg-app transition-colors hover:bg-[hsl(var(--bg-hover))]"
        style={{ gridTemplateColumns: COLUMNS, height: ROW, transform: `translateY(${top}px)` }}
      >
        {/* Frozen while the rest scrolls sideways, so a narrow window never
            loses which chapter a cell belongs to. */}
        <div className="sticky left-0 z-10 flex h-full items-center justify-end bg-app px-2 group-hover:bg-[hsl(var(--bg-hover))]">
          <span className="tabular text-[11px] font-semibold text-subtle">{chapter.ordinal}</span>
        </div>

        <Tooltip label={chapter.summary || 'No summary'} side="right">
          <div className="sticky left-10 z-10 flex h-full min-w-0 flex-col justify-center gap-0.5 bg-app px-2 group-hover:bg-[hsl(var(--bg-hover))]">
            <div className="flex items-baseline gap-2">
              <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-fg">
                {chapter.title || <span className="text-subtle">Untitled</span>}
              </span>
              {chapter.estimatedWords > 0 && (
                <span className="tabular shrink-0 text-[10.5px] text-subtle">
                  ~{chapter.estimatedWords}w
                </span>
              )}
            </div>
            <p className="line-clamp-2 text-[11px] leading-[14px] text-muted">{chapter.summary}</p>
          </div>
        </Tooltip>

        <div className="flex h-full items-center px-2">
          <ScriptCell
            cell={stage.script}
            words={words}
            videoRef={video.ref}
            videoId={video.id}
            onOpen={onOpenScript}
          />
        </div>

        <div className="flex h-full items-center px-2">
          <NarrationCell
            cell={stage.narration}
            assetId={chapter.audioAssetId}
            seconds={chapter.durationSeconds}
            videoRef={video.ref}
            videoId={video.id}
          />
        </div>

        <div className="flex h-full items-center px-2">
          <SlidesCell
            chapter={chapter}
            cells={stage.slides}
            thumbWidth={thumbWidth}
            onOpenSlide={(slot) => onOpenAsset(chapter.slideAssetIds[slot])}
          />
        </div>

        <div className="flex h-full items-center px-2">
          <ClipCell
            cell={stage.clip}
            videoRef={video.ref}
            videoId={video.id}
            onOpen={() => onOpenAsset(chapter.clipAssetId)}
          />
        </div>

        <div className="flex h-full items-center justify-center">
          <Tooltip label="Re-run this chapter">
            <button
              type="button"
              aria-label="Re-run this chapter"
              disabled={retryChapter.isPending}
              onClick={() => retryChapter.mutate()}
              className="flex h-6 w-6 items-center justify-center rounded-[var(--radius-xs)] text-subtle opacity-0 transition-opacity hover:bg-[hsl(var(--bg-hover))] hover:text-fg focus-visible:opacity-100 group-hover:opacity-100"
            >
              {retryChapter.isPending ? (
                <RotateCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <MoreHorizontal className="h-3.5 w-3.5" />
              )}
            </button>
          </Tooltip>
        </div>
      </div>
    </ContextMenu>
  )
}
