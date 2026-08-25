import { useMemo } from 'react'

import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Mark, SlotRow } from './mark'
import { columnTotals, stagesByChapter } from './stages'

/**
 * The blueprint as a grid: chapters down, pipeline stages across.
 *
 * It used to be three tabs in v1 — Chapters, Artifacts, Info — which split one
 * object along a seam that does not exist. An artifact *belongs to* a chapter,
 * so every artifact the pipeline produces has a fixed position in the row of
 * the chapter that owns it, and reaching one is pointing at a known place
 * rather than navigating to it.
 *
 * A grid rather than a `<table>` because the header has to stick while the body
 * scrolls, and one `grid-template-columns` shared by the head and every row is
 * a simpler way to keep them aligned than two elements agreeing to.
 *
 * A cell is a mark and nothing else. Every figure that used to sit beside one —
 * a word count, a projected runtime — was the same number the summary line
 * already gives as a total, printed once per row where it competed with the one
 * thing the grid exists to show. Forty rows of numbers nobody adds up is forty
 * rows of noise over the one amber arc that matters.
 */

/*
  The trailing `1fr` is a spacer, and it is the only reason the grid is legible
  in a wide window.

  Without it the chapter column takes every spare pixel, and on a wide screen
  the marks end up half a metre from the title they belong to — the eye has to
  travel a blank gap to pair them, which is exactly the failure a table is
  supposed to prevent. Capping the title and parking the slack at the far end
  keeps the stages beside their chapter at any width.
*/
const COLUMNS = '2.5rem minmax(10rem, 26rem) 4.75rem 5.75rem minmax(6rem, 12rem) 3.75rem 1fr'

interface ChapterTableProps {
  chapters: Chapter[]
  tasks: Task[]
  slidesPerChapter: number
}

export function ChapterTable({ chapters, tasks, slidesPerChapter }: ChapterTableProps) {
  const stages = useMemo(
    () => stagesByChapter(chapters, tasks, slidesPerChapter),
    [chapters, tasks, slidesPerChapter],
  )
  const totals = useMemo(
    () => columnTotals(chapters, slidesPerChapter),
    [chapters, slidesPerChapter],
  )

  if (chapters.length === 0) {
    return (
      <p className="px-4 py-6 text-[12px] text-tertiary">
        No chapters yet — the blueprint writes them.
      </p>
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <div
        role="row"
        style={{ gridTemplateColumns: COLUMNS }}
        className="surface-band hairline-b sticky top-0 z-10 grid items-center gap-3 px-4 py-1.5 text-[10px] font-semibold tracking-[0.06em] text-tertiary uppercase"
      >
        <span>#</span>
        <span>Chapter</span>
        <Head label="Script" done={totals.script.done} total={totals.script.total} />
        <Head label="Narration" done={totals.narration.done} total={totals.narration.total} />
        <Head label="Slides" done={totals.slides.done} total={totals.slides.total} />
        <Head label="Clip" done={totals.clip.done} total={totals.clip.total} />
      </div>

      {chapters.map((chapter) => {
        const stage = stages.get(chapter.id)
        if (!stage) return null
        return (
          <div
            key={chapter.id}
            role="row"
            style={{ gridTemplateColumns: COLUMNS }}
            className="grid items-center gap-3 px-4 py-2 hover:bg-[var(--hover)]"
          >
            <span className="text-[12px] tabular-nums text-tertiary">{chapter.ordinal}</span>
            <span className="truncate text-[13px] text-primary" title={chapter.title}>
              {chapter.title || 'Untitled'}
            </span>
            <Mark cell={stage.script} />
            <Mark cell={stage.narration} />
            <SlotRow cells={stage.slides} />
            <Mark cell={stage.clip} />
          </div>
        )
      })}
    </div>
  )
}

/**
 * A column head carrying how far its stage has got across the whole video.
 *
 * This is the arithmetic no individual row can do, and it is most of why the
 * grid is worth its density: `SLIDES 24/80` answers a question that scanning
 * eighty marks would only approximate.
 */
function Head({ label, done, total }: { label: string; done: number; total: number }) {
  const complete = total > 0 && done === total
  return (
    <span className="flex items-baseline gap-1.5">
      <span>{label}</span>
      {total > 0 ? (
        <span className={cn('tabular-nums', complete ? 'opacity-0' : 'opacity-100')}>
          {done}/{total}
        </span>
      ) : null}
    </span>
  )
}
