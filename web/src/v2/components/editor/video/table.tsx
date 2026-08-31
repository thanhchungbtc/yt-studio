import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'

import { api, qk, type ChapterPlan } from '../../../core/api'
import { count, duration } from '../../../core/format'
import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Mark, SlotRow } from './mark'
import { columnTotals, projectedSeconds, stagesByChapter } from './stages'

/**
 * The blueprint as a grid: chapters down, pipeline stages across.
 *
 * Not three tabs — Chapters, Artifacts, Info — which would split one object
 * along a seam that does not exist. An artifact *belongs to* a chapter, so
 * every artifact the pipeline produces has a fixed position in the row of the
 * chapter that owns it, and reaching one is pointing at a known place rather
 * than navigating to it.
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
 *
 * And because those first three columns *are* the plan, they are the three the
 * table can edit. Nothing else here is editable: an artifact is the output of a
 * task and belongs to the task table, but a title, a brief and a word budget are
 * inputs — the ones every script is written from — and the moment they are worth
 * changing is while looking at the grid that shows what they add up to.
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
  /** The cache key an edit patches; a delta only ever carries the id. */
  videoId: string
  chapters: Chapter[]
  tasks: Task[]
  slidesPerChapter: number
  editing: boolean
}

export function ChapterTable({
  videoId,
  chapters,
  tasks,
  slidesPerChapter,
  editing,
}: ChapterTableProps) {
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
              'grid items-start gap-3 px-4 py-2.5',
              // A row highlighting under the cursor while you type in it is
              // motion answering nothing.
              !editing && 'hover:bg-[var(--hover)]',
              index > 0 && 'hairline-t',
            )}
          >
            <span className="pt-px text-[12px] tabular-nums text-tertiary">{chapter.ordinal}</span>

            {editing ? (
              <PlanFields chapter={chapter} videoId={videoId} />
            ) : (
              <>
                <div className="min-w-0">
                  <div className="text-[13px] leading-snug text-primary">
                    {chapter.title || 'Untitled'}
                  </div>
                  {chapter.summary ? (
                    <p className="mt-1 text-[12px] leading-snug text-secondary">
                      {chapter.summary}
                    </p>
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
              </>
            )}

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

/** The budget as a field: 0 means unset, and unset shows as empty, not as `0`. */
function wordsDraft(estimatedWords: number): string {
  return estimatedWords > 0 ? String(estimatedWords) : ''
}

/**
 * The plan, as fields: the two cells that stand in for the title and the budget
 * while the table is in edit mode.
 *
 * A fragment of two grid children rather than a wrapper around them, because the
 * columns belong to the table and a box across a pair of them would break the
 * alignment the whole grid is built on.
 *
 * Mounted only while editing, which is what seeds the drafts — they are the
 * chapter as it stood when Edit was pressed rather than when the table first
 * rendered.
 *
 * Saved on blur, not behind a Save button. A blueprint is thirty rows of three
 * fields, and one button at the end of that is ninety edits held in a browser
 * tab waiting for a click that may not come; blur is the moment the operator has
 * finished with the field, and it is how renaming already works everywhere else
 * on the system. Escape puts the row back.
 *
 * Every save sends the whole plan, because that is what the PUT takes. The
 * second field to blur finds nothing changed and sends nothing.
 */
function PlanFields({ chapter, videoId }: { chapter: Chapter; videoId: string }) {
  const client = useQueryClient()
  const [title, setTitle] = useState(chapter.title)
  const [summary, setSummary] = useState(chapter.summary)
  const [words, setWords] = useState(wordsDraft(chapter.estimatedWords))

  const budget = Math.max(0, Math.trunc(Number(words) || 0))

  const save = useMutation({
    mutationFn: (plan: ChapterPlan) => api.updateChapterPlan(chapter.id, plan),
    // The response is the whole row, so this patches the cache rather than
    // refetching it — and the totals in the summary line move with the patch.
    onSuccess: (updated) =>
      client.setQueryData<Chapter[]>(qk.chapters(videoId), (prev) =>
        prev?.map((row) => (row.id === updated.id ? updated : row)),
      ),
  })

  const revert = () => {
    setTitle(chapter.title)
    setSummary(chapter.summary)
    setWords(wordsDraft(chapter.estimatedWords))
  }

  const commit = () => {
    const plan: ChapterPlan = {
      title: title.trim(),
      summary: summary.trim(),
      estimatedWords: budget,
    }
    // A cleared title is a slip rather than an instruction: the server rejects
    // it, and a row with nothing to name it by is not what anyone was reaching
    // for. Put back, the way the Finder puts back a filename you emptied.
    if (!plan.title) {
      setTitle(chapter.title)
      return
    }
    if (
      plan.title === chapter.title &&
      plan.summary === chapter.summary &&
      plan.estimatedWords === chapter.estimatedWords
    ) {
      return
    }
    save.mutate(plan)
  }

  const onEscape = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== 'Escape') return
    event.preventDefault()
    revert()
  }

  // Return commits by leaving the field, so a save has one path through this
  // component and not two. Not bound in the summary, where Return is a
  // paragraph break and nothing else.
  const onReturn = (event: KeyboardEvent<HTMLInputElement>) => {
    onEscape(event)
    if (event.key !== 'Enter') return
    event.preventDefault()
    event.currentTarget.blur()
  }

  const box = useRef<HTMLTextAreaElement>(null)
  // Grown to its content, because the summary is never clamped in read mode
  // either. A three-line well with a scrollbar in it would hide the sentence
  // being approved, which is the one thing this column exists to show.
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [summary])

  return (
    <>
      <div className="min-w-0">
        <input
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          onBlur={commit}
          onKeyDown={onReturn}
          aria-label={`Chapter ${chapter.ordinal} title`}
          className="control w-full"
          // `.control` carries its own type size and it is not this row's. Set
          // inline because the class is unlayered, so a utility would lose to it
          // — and a row that changed height on entering edit mode would move
          // every row below it.
          style={{ fontSize: 13, lineHeight: 1.375 }}
        />
        <textarea
          ref={box}
          rows={1}
          value={summary}
          onChange={(event) => setSummary(event.target.value)}
          onBlur={commit}
          onKeyDown={onEscape}
          placeholder="What this chapter covers"
          aria-label={`Chapter ${chapter.ordinal} summary`}
          className="control mt-1 w-full resize-none overflow-hidden"
          style={{ fontSize: 12, lineHeight: 1.375 }}
        />
        {save.error ? (
          <p className="mt-1 text-[11px] leading-snug text-[var(--failed)]">
            {(save.error as Error).message}
          </p>
        ) : null}
      </div>

      <div>
        <div className="flex items-center gap-1">
          <input
            type="number"
            min={0}
            step={10}
            value={words}
            onChange={(event) => setWords(event.target.value)}
            onBlur={commit}
            onKeyDown={onReturn}
            aria-label={`Chapter ${chapter.ordinal} word budget`}
            className="control w-[58px] text-right tabular-nums"
          />
          <span className="text-[11px] text-tertiary">w</span>
        </div>
        {/* Projected from the draft rather than from what is saved: a budget is
            rebalanced against the runtime it buys, and a figure that only caught
            up after the save would be answering the previous question.

            Sized and aligned to the field above it, so the runtime sits under
            the number it comes from rather than under the left edge of a column
            the number is not against. */}
        <span className="mt-1 block w-[58px] text-right text-[11px] tabular-nums text-tertiary">
          {budget > 0 ? `~${duration(projectedSeconds(budget))}` : '—'}
        </span>
      </div>
    </>
  )
}
