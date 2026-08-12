import { useVirtualizer } from '@tanstack/react-virtual'
import { FileText } from 'lucide-react'
import type { CSSProperties, ReactNode } from 'react'
import { useEffect, useMemo, useRef, useState } from 'react'

import { useAssetViewer } from '../../asset-viewer'
import { EmptyState, Skeleton } from '../../ui/primitives'
import type { ViewerItem } from '@/core/assets'
import type { Chapter, Task, Video } from '@/core/types'
import { cn } from '@/core/utils'
import { ClipCell, NarrationCell, ScriptCell, SlidesCell } from './cells'
import { ScriptDialog } from './script-dialog'
import { useWorkbenchStore } from '../../lib/store'
import {
  columnTotals,
  slideThumbWidth,
  stagesByChapter,
  wordsIn,
  type ColumnTotals,
} from './stages'

/** A typical row, used only until the real one has been measured. */
const ROW_ESTIMATE = 76
const HEAD = 38

/**
 * The columns, with the widths they start at.
 *
 * Chapter is a plain width rather than `1fr`, which is the change that made the
 * rest of the table usable: as a flex column it swallowed every spare pixel —
 * 710 of them on a wide window — and left the columns that actually hold the
 * artifacts pinned at their minimums.
 *
 * Every width here is a starting point. They are draggable and remembered.
 */
interface ColumnDef {
  id: string
  label: string
  width: number
  min: number
  max: number
}

const COLUMNS: ColumnDef[] = [
  { id: 'chapter', label: 'Chapter', width: 280, min: 180, max: 900 },
  { id: 'script', label: 'Script', width: 84, min: 64, max: 200 },
  { id: 'narration', label: 'Narration', width: 100, min: 80, max: 240 },
  { id: 'slides', label: 'Slides', width: 220, min: 120, max: 560 },
  { id: 'clip', label: 'Clip', width: 60, min: 48, max: 160 },
]

/** Fixed: an ordinal has one right size. */
const ORDINAL = 40

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

  const stored = useWorkbenchStore((s) => s.columnWidths)
  const widths = useMemo(
    () =>
      COLUMNS.map((column) => ({
        ...column,
        width: clamp(stored[column.id] ?? column.width, column.min, column.max),
      })),
    [stored],
  )
  // A trailing `1fr` soaks up whatever is left over, so the table fills a wide
  // window without any single column having to stretch to do it.
  const template = `${ORDINAL}px ${widths.map((c) => `${c.width}px`).join(' ')} 1fr`
  const totalWidth = ORDINAL + widths.reduce((sum, c) => sum + c.width, 0)
  const slidesWidth = widths.find((c) => c.id === 'slides')?.width ?? 220

  const stages = useMemo(
    () => stagesByChapter(chapters, tasks, video.slidesPerChapter),
    [chapters, tasks, video.slidesPerChapter],
  )
  const totals = useMemo(
    () => columnTotals(chapters, video.slidesPerChapter),
    [chapters, video.slidesPerChapter],
  )
  const thumbWidth = useMemo(
    () => slideThumbWidth(Math.max(1, video.slidesPerChapter), slidesWidth),
    [video.slidesPerChapter, slidesWidth],
  )

  const rows = useVirtualizer({
    count: chapters.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW_ESTIMATE,
    // A chapter summary is shown in full however long it runs, so rows are as
    // tall as their own content and the virtualizer has to measure rather than
    // assume.
    measureElement: (element) => element.getBoundingClientRect().height,
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
        <div style={{ minWidth: totalWidth }}>
          {/* The header is the progress summary. `SLIDES 24/80` says how far one
              stage has got across the whole video, which is the question a table
              can answer and a list cannot. */}
          <div
            className="sticky top-0 z-20 grid border-b border-[hsl(var(--border))] bg-subtle no-select"
            style={{ gridTemplateColumns: template, height: HEAD }}
          >
            <HeadCell className="sticky left-0 z-10 justify-end bg-subtle">#</HeadCell>
            {widths.map((column, index) => (
              <HeadCell
                key={column.id}
                className={cn(column.id === 'chapter' && 'sticky z-10 bg-subtle')}
                style={column.id === 'chapter' ? { left: ORDINAL } : undefined}
              >
                {column.label}
                {countFor(column.id, chapters.length, totals)}
                <ColumnResizer column={column} first={index === 0} />
              </HeadCell>
            ))}
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
                  template={template}
                  index={item.index}
                  measureRef={rows.measureElement}
                  chapter={chapter}
                  stage={stage}
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

function HeadCell({
  className,
  style,
  children,
}: {
  className?: string
  style?: CSSProperties
  children?: ReactNode
}) {
  return (
    <div
      style={style}
      className={cn(
        'relative flex h-full items-center gap-1.5 px-2 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-subtle',
        className,
      )}
    >
      {children}
    </div>
  )
}

/** Which count a column's head carries, if any. */
function countFor(id: string, chapters: number, totals: ColumnTotals): ReactNode {
  switch (id) {
    case 'chapter':
      return <Count done={chapters} />
    case 'script':
      return <Count done={totals.script.done} total={totals.script.total} />
    case 'narration':
      return <Count done={totals.narration.done} total={totals.narration.total} />
    case 'slides':
      return <Count done={totals.slides.done} total={totals.slides.total} />
    case 'clip':
      return <Count done={totals.clip.done} total={totals.clip.total} />
    default:
      return null
  }
}

/**
 * The grab area on a column's trailing edge. One pixel of rule, seven of target
 * — the same bargain the panel splitters make, for the same reason.
 *
 * Double-click restores the default, which is the only way back from a width
 * dragged to somewhere unusable.
 */
function ColumnResizer({ column, first }: { column: ColumnDef; first: boolean }) {
  const setColumnWidth = useWorkbenchStore((s) => s.setColumnWidth)
  const resetColumnWidth = useWorkbenchStore((s) => s.resetColumnWidth)
  const [dragging, setDragging] = useState(false)
  const origin = useRef({ x: 0, width: column.width })

  useEffect(() => {
    if (!dragging) return
    const onMove = (event: PointerEvent) => {
      const { x, width } = origin.current
      setColumnWidth(column.id, clamp(width + (event.clientX - x), column.min, column.max))
    }
    const stop = () => setDragging(false)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
    const previous = document.body.style.cursor
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
      document.body.style.cursor = previous
      document.body.style.userSelect = ''
    }
  }, [column.id, column.max, column.min, dragging, setColumnWidth])

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={`Resize the ${column.label} column`}
      aria-valuenow={column.width}
      aria-valuemin={column.min}
      aria-valuemax={column.max}
      tabIndex={0}
      onPointerDown={(event) => {
        if (event.button !== 0) return
        event.preventDefault()
        origin.current = { x: event.clientX, width: column.width }
        setDragging(true)
      }}
      onDoubleClick={() => resetColumnWidth(column.id)}
      onKeyDown={(event) => {
        const step = event.shiftKey ? 32 : 8
        if (event.key === 'ArrowLeft') {
          setColumnWidth(column.id, clamp(column.width - step, column.min, column.max))
        } else if (event.key === 'ArrowRight') {
          setColumnWidth(column.id, clamp(column.width + step, column.min, column.max))
        } else {
          return
        }
        event.preventDefault()
      }}
      className={cn(
        'group/resize absolute inset-y-0 right-0 z-20 w-[7px] translate-x-[3px] cursor-col-resize touch-none',
        first && 'right-[-1px]',
      )}
    >
      <span
        aria-hidden
        className={cn(
          'absolute inset-y-[9px] left-[3px] w-px transition-colors',
          dragging
            ? 'bg-[hsl(var(--accent))]'
            : 'bg-[hsl(var(--border))] group-hover/resize:bg-[hsl(var(--accent)/0.7)]',
        )}
      />
    </div>
  )
}

function clamp(value: number, min: number, max: number): number {
  return Math.round(Math.min(max, Math.max(min, value)))
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
  template,
  index,
  measureRef,
  chapter,
  stage,
  thumbWidth,
  top,
  onOpenScript,
  onOpenAsset,
}: {
  template: string
  index: number
  measureRef: (node: Element | null) => void
  chapter: Chapter
  stage: ReturnType<typeof stagesByChapter> extends Map<string, infer S> ? S : never
  thumbWidth: number | null
  top: number
  onOpenScript: () => void
  onOpenAsset: (assetId: string | undefined) => void
}) {
  return (
    <div
      ref={measureRef}
      data-index={index}
      data-chapter-row={chapter.ordinal}
      // No colour transition anywhere on the row. The frozen cells paint their
      // own background — they have to, or the columns behind them show through —
      // and a transition on the row alone made the two halves arrive a frame
      // apart, which is what read as a flash when the pointer crossed a row.
      className="group absolute inset-x-0 grid min-h-[60px] border-b border-[hsl(var(--border))] bg-app hover:bg-[hsl(var(--bg-hover))]"
      style={{ gridTemplateColumns: template, transform: `translateY(${top}px)` }}
    >
      <div className="sticky left-0 z-10 flex items-center justify-end bg-app px-2 pt-[9px] group-hover:bg-[hsl(var(--bg-hover))]">
        <span className="tabular text-[11px] font-semibold text-subtle">{chapter.ordinal}</span>
      </div>

      <div
        style={{ left: ORDINAL }}
        className="sticky z-10 flex min-w-0 flex-col justify-center gap-0.5 bg-app px-2 py-2 group-hover:bg-[hsl(var(--bg-hover))]"
      >
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
        {/* In full, however long it runs, and with the blueprint's own line
            breaks kept — the plan is the thing being reviewed here. */}
        <p className="whitespace-pre-wrap text-[11px] leading-[15px] text-muted">
          {chapter.summary}
        </p>
      </div>

      <div className="flex items-start px-2 pt-2">
        <ScriptCell cell={stage.script} words={wordsIn(chapter.script)} onOpen={onOpenScript} />
      </div>

      <div className="flex items-start px-2 pt-2">
        <NarrationCell
          cell={stage.narration}
          assetId={chapter.audioAssetId}
          seconds={chapter.durationSeconds}
          onOpen={() => onOpenAsset(chapter.audioAssetId)}
        />
      </div>

      <div className="flex items-start px-2 pt-2">
        <SlidesCell
          chapter={chapter}
          cells={stage.slides}
          thumbWidth={thumbWidth}
          onOpenSlide={(slot) => onOpenAsset(chapter.slideAssetIds[slot])}
        />
      </div>

      <div className="flex items-start px-2 pt-2">
        <ClipCell cell={stage.clip} onOpen={() => onOpenAsset(chapter.clipAssetId)} />
      </div>

      {/* Soaks up leftover width so the row's hover runs the full table. */}
      <div />
    </div>
  )
}
