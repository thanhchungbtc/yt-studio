import { useMemo } from 'react'

import { count, duration } from '../../../core/format'
import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { MarkCell, SlotRow } from './mark'
import { columnTotals, stagesByChapter, wordsIn } from './stages'

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
 */

const COLUMNS = '2.25rem minmax(8rem, 1fr) 6rem 6rem minmax(5rem, 12rem) 4rem'

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
        const written = wordsIn(chapter.script)
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
            <MarkCell cell={stage.script}>{written ? `${count(written)}w` : ''}</MarkCell>
            <MarkCell cell={stage.narration}>
              {chapter.durationSeconds > 0 ? duration(chapter.durationSeconds) : ''}
            </MarkCell>
            <SlotRow cells={stage.slides} />
            <MarkCell cell={stage.clip} />
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
