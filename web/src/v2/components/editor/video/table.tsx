import { useMemo } from 'react'

import { count, duration } from '../../../core/format'
import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Mark, SlotRow } from './mark'
import { columnTotals, projectedSeconds, stagesByChapter } from './stages'

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
 * The table is also where a blueprint is reviewed, and that is what the first
 * three columns are for. At the gate nothing has been generated, so every stage
 * cell is an empty ring — the grid is at its least informative at exactly the
 * moment the decision is hardest. What is decidable then is the *plan*: what
 * each chapter covers, and how long it is meant to run.
 *
 * So the summary is printed in full, wrapped, never clamped. A one-line ellipsis
 * of the sentence you are being asked to approve is worse than not showing it,
 * because it looks like you have read it.
 *
 * The stage cells stay marks and nothing else. Figures there — a word count, a
 * runtime — were the same numbers the summary line gives as totals, reprinted
 * once per row where they competed with the one thing those columns exist to
 * show.
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
const COLUMNS = '2.25rem minmax(13rem, 34rem) 7.5rem 4.5rem 5.5rem minmax(6rem, 12rem) 3.5rem 1fr'

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
        <span>Estimated length</span>
        <Head label="Script" done={totals.script.done} total={totals.script.total} />
        <Head label="Narration" done={totals.narration.done} total={totals.narration.total} />
        <Head label="Slides" done={totals.slides.done} total={totals.slides.total} />
        <Head label="Clip" done={totals.clip.done} total={totals.clip.total} />
      </div>

      {chapters.map((chapter, index) => {
        const stage = stages.get(chapter.id)
        if (!stage) return null
        // The blueprint's budget, not what was written: it is the number the
        // plan was approved on, and it does not move once the scripts land.
        const seconds = projectedSeconds(chapter.estimatedWords)
        return (
          <div
            key={chapter.id}
            role="row"
            style={{ gridTemplateColumns: COLUMNS }}
            className={cn(
              // Aligned to the top, not the middle: a chapter is two lines tall
              // now, and a mark floating beside the second one belongs to
              // nothing the eye can name.
              'grid items-start gap-3 px-4 py-2.5 hover:bg-[var(--hover)]',
              index > 0 && 'hairline-t',
            )}
          >
            <span className="pt-px text-[12px] tabular-nums text-tertiary">{chapter.ordinal}</span>

            <div className="min-w-0">
              <div className="text-[13px] leading-snug text-primary">
                {chapter.title || 'Untitled'}
              </div>
              {chapter.summary ? (
                <p className="mt-1 text-[12px] leading-snug text-secondary">{chapter.summary}</p>
              ) : null}
            </div>

            <div className="pt-px text-[12px] tabular-nums text-secondary">
              {chapter.estimatedWords > 0 ? (
                <>
                  ~{count(chapter.estimatedWords)}w
                  <span className="block text-[11px] text-tertiary">~{duration(seconds)}</span>
                </>
              ) : null}
            </div>

            <Mark cell={stage.script} className="pt-1" />
            <Mark cell={stage.narration} className="pt-1" />
            <SlotRow cells={stage.slides} className="pt-1" />
            <Mark cell={stage.clip} className="pt-1" />
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
