import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import { count, duration } from '../../../core/format'
import type { Chapter, Task } from '../../../core/types'
import { cn } from '../../../core/utils'
import { Button } from '../../ui/button'
import { ClipViewer } from './clip-viewer'
import { Mark } from './mark'
import { ChapterOutline } from './outline'
import { SlideViewer } from './slide-viewer'
import { stagesByChapter, wordsIn, type Cell } from './stages'

/** A slide the viewer can open: where its bytes are, and what to call it. */
interface Slide {
  id: string
  title: string
}

/**
 * The video as something to read, rather than something to watch finish.
 *
 * The table says whether each chapter happened. This says what each chapter
 * *is*: the narration in full, the voice that reads it, and the pictures that
 * go under it — in that order, chapter after chapter, down one scroll.
 *
 * It is not an artifact browser, and the difference matters. Grouping these by
 * kind, or listing them with their sizes and content addresses, would cut the
 * video along a seam that does not exist. The chapter is the spine here exactly
 * as it is in the table; only the altitude changes, from *did this happen* to
 * *what does it say*.
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

  // Which slide is open, and what to call it. Held here rather than per chapter
  // so there is one viewer for the whole scroll and no way to end up with two.
  const [viewing, setViewing] = useState<Slide | null>(null)

  const scroller = useRef<HTMLDivElement>(null)
  const [active, setActive] = useState<string | null>(null)

  /*
    Which chapter is in view, for the bar in the outline.

    An observer rather than arithmetic on `scrollTop`: the browser already knows
    where every section is, and asking it costs nothing per frame where measuring
    would cost a layout on every scroll event.

    The top strip is what "in view" means here. `rootMargin` crops the observed
    area to a band just under the pinned header, so the active chapter is the one
    whose content is at the top of the reader rather than whichever happens to
    cover the most pixels — the second rule makes the bar sit on the wrong
    chapter for the whole of a long one.
  */
  useEffect(() => {
    const root = scroller.current
    if (!root) return
    const sections = root.querySelectorAll<HTMLElement>('[data-chapter]')
    if (sections.length === 0) return

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          const id = entry.target.getAttribute('data-chapter')
          if (id) setActive(id)
        }
      },
      { root, rootMargin: '-38px 0px -85% 0px', threshold: 0 },
    )
    for (const section of sections) observer.observe(section)
    return () => observer.disconnect()
  }, [chapters])

  const jump = (id: string) => {
    scroller.current
      ?.querySelector<HTMLElement>(`[data-chapter="${id}"]`)
      ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

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
    // `@container` so the card can take itself away when the window is too
    // narrow to have a margin for it to sit in.
    <div className="@container relative flex min-h-0 flex-1">
      <ChapterOutline chapters={chapters} activeId={active} onJump={jump} />
      {/* The card's footprint, reserved at exactly the width the card appears
          at, so the two can never disagree about whether there is room. */}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto @[54rem]:pl-[216px]">
        {chapters.map((chapter) => (
          <ChapterBlock
            key={chapter.id}
            chapter={chapter}
            slides={stages.get(chapter.id)?.slides ?? []}
            onView={setViewing}
          />
        ))}
      </div>
      {viewing ? (
        <SlideViewer
          src={`/assets/${viewing.id}`}
          title={viewing.title}
          onClose={() => setViewing(null)}
        />
      ) : null}
    </div>
  )
}

/**
 * The column everything in a chapter lines up in, and why it is not the width
 * of the window.
 *
 * A line stops being readable somewhere past ninety characters — the eye loses
 * the return sweep and starts re-reading lines it has already had — and this
 * window is twice that wide. So the column is capped, and *centred*, with the
 * band headers running full width across it.
 *
 * The centring is the part that was wrong before. Capped and left-aligned puts
 * the whole remainder on one side, which reads as something having failed to
 * fill it; the same cap centred reads as deliberate. Nothing about the measure
 * changed, only which side the leftover is on.
 */
const COLUMN = 'mx-auto w-full max-w-[40rem]'

/**
 * The script as a bounded panel of machine text.
 *
 * A `pre` in SF Mono at twelve pixels, recessed behind a hairline. Narration is
 * a generated artifact — the exact bytes a voice will read, straight out of a
 * model — and the reference for that is a pro app's code panel rather than a
 * page from a book: dense, precise, obviously *output*. It also puts the script
 * in the same register as the rest of the window, which a reading face did not.
 *
 * `pre-wrap` rather than `pre`. Real code does not wrap and neither should this,
 * except a sentence of narration is two hundred characters long, and a panel a
 * mile wide with a horizontal scrollbar is not a panel anyone reads.
 *
 * And bounded, which is the point of it. A chapter's script is a few thousand
 * characters; eight of them at full height is a view you scroll for a minute
 * without passing a second chapter. Capped, every chapter is roughly one screen
 * and the structure is visible again — the long ones scroll in place.
 *
 * The one cost: the pointer inside a panel scrolls the panel. Reaching its end
 * hands the scroll back to the page, which is the browser's default and the
 * behaviour worth keeping.
 */
const SCRIPT = [
  'max-h-[19rem] overflow-y-auto',
  'rounded-[7px] px-3.5 py-3',
  'font-mono text-[12px] leading-[1.65] whitespace-pre-wrap',
  'text-primary',
].join(' ')

const PANEL = {
  backgroundColor: 'var(--band)',
  boxShadow: '0 0 0 0.5px var(--separator)',
}

function ChapterBlock({
  chapter,
  slides,
  onView,
}: {
  chapter: Chapter
  slides: Cell[]
  onView: (slide: Slide) => void
}) {
  const words = wordsIn(chapter.script)
  // Held here rather than up in the reader: the button that opens it is in this
  // component, so there is nothing to thread, and a chapter has one clip.
  const [playing, setPlaying] = useState(false)

  return (
    <section data-chapter={chapter.id}>
      {/*
        Sticky, and the only thing in this view that is. Eight chapters down a
        scroll the question is always which chapter this is, and a header that
        has left the screen answers it for whoever is at the top instead.
      */}
      <header className="surface-band hairline-b sticky top-0 z-10 px-6 py-2">
        {/* The band spans the window; its contents line up with the column
            underneath it, so the ordinal sits over the first word rather than
            somewhere off to the left of everything. */}
        <div className={cn(COLUMN, 'flex items-baseline gap-3')}>
          <span className="shrink-0 text-[11px] tabular-nums text-tertiary">{chapter.ordinal}</span>
          <span className="min-w-0 flex-1 truncate text-[11px] font-semibold tracking-[0.05em] text-secondary uppercase">
            {chapter.title}
          </span>
          <span className="shrink-0 text-[11px] tabular-nums text-tertiary">
            {words > 0
              ? `${count(words)} words · ${duration(chapter.durationSeconds)}`
              : 'not written yet'}
          </span>
        </div>
      </header>

      {/* Padding outside the column and not on it, exactly as the header does
          it: with `px-6` on the capped element itself the two would be measured
          from different edges and the ordinal would sit 24px off the first word. */}
      <div className="px-6 pt-5 pb-12">
        <div className={cn(COLUMN, 'flex flex-col gap-7')}>
          <Part label="Script">
            {chapter.script ? (
              <pre className={SCRIPT} style={PANEL}>
                {chapter.script}
              </pre>
            ) : (
              <p className="text-[12px] text-tertiary">
                The script for this chapter has not been written yet.
              </p>
            )}
          </Part>

          {chapter.audioAssetId ? (
            <Part label="Narration">
              {/*
                The platform control, not one built here. It arrives knowing how
                to seek, how to answer the keyboard and where the system volume
                goes, and the asset handler serves ranges, so scrubbing works.

                `preload="none"` is load-bearing rather than tidy. Narration is
                served as WAV, which is uncompressed — call it ten megabytes a
                minute — so seven chapters of players that fetch on sight would
                pull the better part of a hundred megabytes for a page nobody
                has pressed play on yet.
              */}
              <audio
                controls
                preload="none"
                src={`/assets/${chapter.audioAssetId}`}
                className="h-[32px] w-full"
              />
            </Part>
          ) : null}

          {slides.length > 0 ? (
            <Part label="Slides">
              <div className="flex flex-wrap gap-2.5">
                {slides.map((cell, slot) => (
                  <Slide
                    key={slot}
                    cell={cell}
                    id={chapter.slideAssetIds[slot]}
                    slot={slot}
                    onView={(slide) =>
                      onView({ ...slide, title: `${chapter.title} · ${slide.title}` })
                    }
                  />
                ))}
              </div>
            </Part>
          ) : null}

          {chapter.clipAssetId ? (
            <Part label="Clip">
              <Button className="self-start" onClick={() => setPlaying(true)}>
                Play clip
              </Button>
            </Part>
          ) : null}
        </div>
      </div>

      {playing && chapter.clipAssetId ? (
        <ClipViewer
          src={`/assets/${chapter.clipAssetId}`}
          title={`${chapter.title} · Clip`}
          onClose={() => setPlaying(false)}
        />
      ) : null}
    </section>
  )
}

/**
 * One of the three things a chapter is made of, with its name over it.
 *
 * The names are the whole fix for "it is hard to tell that is a script". A wall
 * of prose with nothing above it is ambiguous — it could be a summary, a
 * transcript, a note — and the chapter title two lines up does not answer the
 * question. One word does. They are the same 10px uppercase caption FINAL and
 * INSPECTOR already use, so this reads as the same application.
 */
function Part({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <span className="text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase">
        {label}
      </span>
      {children}
    </div>
  )
}

/** 1344×768 is what the composer frames a slide at; 1.75 is that, small. */
const TILE = 'h-[112px] w-[196px] shrink-0 rounded-[6px]'

/**
 * One slot, whether or not it has a picture in it.
 *
 * The empty ones keep their place in the row rather than closing the gap, so a
 * half-drawn chapter says *which* slide is missing — the same reason the table
 * draws a mark per slot instead of a count.
 *
 * Only a drawn slot is a button. An empty one has no picture to enlarge, and a
 * viewer that opened to say so would be repeating what the tile already says.
 */
function Slide({
  id,
  cell,
  slot,
  onView,
}: {
  id: string | undefined
  cell: Cell
  slot: number
  onView: (slide: Slide) => void
}) {
  if (id) {
    return (
      <button
        type="button"
        onClick={() => onView({ id, title: `Slide ${slot + 1}` })}
        // The ring rather than a brightness lift: macOS shows a picture is
        // pickable by outlining it, and the picture itself should not change
        // colour under the pointer.
        className={cn(TILE, 'overflow-hidden hover:ring-2 hover:ring-[var(--accent)]')}
      >
        <img
          src={`/assets/${id}`}
          alt={`Slide ${slot + 1}`}
          loading="lazy"
          decoding="async"
          className="size-full object-cover"
          style={{ boxShadow: '0 0 0 0.5px var(--separator-strong)' }}
        />
      </button>
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
