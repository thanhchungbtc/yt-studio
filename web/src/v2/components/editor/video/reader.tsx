import { useMemo } from 'react'

import { count, duration } from '../../../core/format'
import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Mark } from './mark'
import { stagesByChapter, wordsIn, type Cell } from './stages'

/**
 * The video as something to read, rather than something to watch finish.
 *
 * The table says whether each chapter happened. This says what each chapter
 * *is*: the narration in full, the voice that reads it, and the pictures that
 * go under it — in that order, chapter after chapter, down one scroll.
 *
 * It is not an artifact browser, and the difference matters. Grouping these by
 * kind, or listing them with their sizes and content addresses, would be the
 * Artifacts tab v1 had and v2 deleted on purpose. The chapter is the spine here
 * exactly as it is in the table; only the altitude changes, from *did this
 * happen* to *what does it say*.
 *
 * Every field it draws is already in the chapters cache the editor fetched —
 * `script` is the whole body on the wire, not a flag — so this view costs no
 * request of its own, and the refetch `events.ts` fires when a script lands
 * fills it in while it is open.
 */
export function ChapterReader({
  chapters,
  tasks,
  slidesPerChapter,
}: {
  chapters: Chapter[]
  tasks: Task[]
  slidesPerChapter: number
}) {
  // Only for the empty slots: a missing picture is either one that has not been
  // drawn or one that failed, and a blank box that cannot tell you which is a
  // blank box you have to go to the other view to understand.
  const stages = useMemo(
    () => stagesByChapter(chapters, tasks, slidesPerChapter),
    [chapters, tasks, slidesPerChapter],
  )

  if (chapters.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center px-8">
        <p className="text-[13px] text-tertiary">
          Nothing to read yet. The blueprint writes the chapters first.
        </p>
      </div>
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {chapters.map((chapter) => (
        <ChapterBlock
          key={chapter.id}
          chapter={chapter}
          slides={stages.get(chapter.id)?.slides ?? []}
        />
      ))}
    </div>
  )
}

function ChapterBlock({ chapter, slides }: { chapter: Chapter; slides: Cell[] }) {
  const words = wordsIn(chapter.script)

  return (
    <section>
      {/*
        Sticky, and the only thing in this view that is. Eight chapters down a
        scroll the question is always which chapter this is, and a header that
        has left the screen answers it for whoever is at the top instead.
      */}
      <header className="surface-band hairline-b sticky top-0 z-10 flex items-baseline gap-3 px-5 py-1.5">
        <span className="shrink-0 text-[11px] tabular-nums text-tertiary">{chapter.ordinal}</span>
        <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.05em] text-secondary uppercase">
          {chapter.title}
        </span>
        <span className="shrink-0 text-[11px] tabular-nums text-tertiary">
          {words > 0
            ? `${count(words)} words · ${duration(chapter.durationSeconds)}`
            : 'not written yet'}
        </span>
      </header>

      <div className="px-5 pt-3.5 pb-6">
        {chapter.script ? (
          /*
            Whole, wrapped, never clamped — the same argument the table makes
            about a summary at the gate. And capped at a readable measure: the
            editor is as wide as the window, and a line of narration that runs
            the full width of a wide one is a line nobody finishes.
          */
          <p className="max-w-[68ch] text-[13px] leading-[1.65] whitespace-pre-wrap text-primary">
            {chapter.script}
          </p>
        ) : (
          <p className="text-[12px] text-tertiary">
            The script for this chapter has not been written yet.
          </p>
        )}

        {chapter.audioAssetId ? (
          /*
            The platform control, not one built here. It arrives knowing how to
            seek, how to answer the keyboard and where the system volume goes,
            and the asset handler serves ranges, so scrubbing works.

            `preload="none"` is load-bearing rather than tidy. Narration is
            served as WAV, which is uncompressed — call it ten megabytes a
            minute — so seven chapters of players that fetch on sight would pull
            the better part of a hundred megabytes for a page nobody has pressed
            play on yet.
          */
          <audio
            controls
            preload="none"
            src={`/assets/${chapter.audioAssetId}`}
            className="mt-4 h-[32px] w-full max-w-[420px]"
          />
        ) : null}

        {slides.length > 0 ? (
          <div className="mt-4 flex flex-wrap gap-2.5">
            {slides.map((cell, slot) => (
              <Slide key={slot} cell={cell} id={chapter.slideAssetIds[slot]} slot={slot} />
            ))}
          </div>
        ) : null}
      </div>
    </section>
  )
}

/** 1344×768 is what the composer frames a slide at; 1.75 is that, small. */
const TILE = 'h-[88px] w-[154px] shrink-0 rounded-[6px]'

/**
 * One slot, whether or not it has a picture in it.
 *
 * The empty ones keep their place in the row rather than closing the gap, so a
 * half-drawn chapter says *which* slide is missing — the same reason the table
 * draws a mark per slot instead of a count.
 */
function Slide({ id, cell, slot }: { id: string | undefined; cell: Cell; slot: number }) {
  if (id) {
    return (
      <img
        src={`/assets/${id}`}
        alt={`Slide ${slot + 1}`}
        loading="lazy"
        decoding="async"
        className={cn(TILE, 'object-cover')}
        style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
      />
    )
  }
  return (
    <div
      className={cn(TILE, 'flex items-center justify-center border border-dashed')}
      style={{ borderColor: 'var(--separator-strong)' }}
    >
      {cell.state === 'waiting' ? (
        <span className="text-[11px] tabular-nums text-tertiary">{slot + 1}</span>
      ) : (
        <Mark cell={cell} />
      )}
    </div>
  )
}
