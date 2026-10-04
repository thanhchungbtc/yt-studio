import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { memo, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import { api, qk } from '../../../../core/api'
import { count, duration } from '../../../../core/format'
import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'
import { Button } from '../../../ui/button'
import { NearbyProvider, useNear, useNearbyRoot } from '../../../ui/nearby'
import { Mark } from '../mark'
import { chapterSeconds, useChapterStages, wordsIn, type Cell } from '../stages'
import type { ViewProps } from '../view'
import { ClipViewer } from './clip-viewer'
import { ChapterOutline } from './outline'
import { SlideViewer } from './slide-viewer'

/**
 * Which slide is open — the slot, not the picture in it.
 *
 * A content address would have been the obvious thing to hold and is the wrong
 * one: redrawing a slide produces a *different* asset, so a viewer keyed on the
 * old id goes on showing the image you just replaced. Keyed on the slot, the
 * bytes, the prompt and the staleness are all read fresh on every render, and
 * the new picture arrives on the event stream without the dialog being told.
 */
interface Viewing {
  chapterId: string
  slot: number
}

/**
 * Which of a chapter's four artifacts are drawn.
 *
 * A filter rather than a mode list, because the useful views are combinations:
 * script alone is a proof-reading pass, slides alone is a contact sheet, script
 * and narration together is checking that the voice says what the page does.
 * Naming those would be guessing at which three of sixteen anyone wants.
 */
interface Shown {
  script: boolean
  narration: boolean
  slides: boolean
  clip: boolean
}

const ALL_SHOWN: Shown = { script: true, narration: true, slides: true, clip: true }

/** One empty list, so a chapter without stages does not defeat the memo. */
const NO_CELLS: Cell[] = []

const SECTIONS: readonly { key: keyof Shown; label: string }[] = [
  { key: 'script', label: 'Script' },
  { key: 'narration', label: 'Narration' },
  { key: 'slides', label: 'Slides' },
  { key: 'clip', label: 'Clip' },
]

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
export function ChaptersView({ video, chapters, tasks }: ViewProps) {
  const slidesPerChapter = video.slidesPerChapter

  // Only for the empty slots: a missing picture is either one that has not been
  // drawn or one that failed, and a blank box that cannot tell you which is a
  // blank box you have to go to the other view to understand.
  const stages = useChapterStages(chapters, tasks, slidesPerChapter)

  // Not persisted, for the reason the mode above it is not: a tab that reopened
  // with the slides hidden, by a choice you had forgotten making, reads as a
  // video whose slides are missing.
  const [shown, setShown] = useState<Shown>(ALL_SHOWN)

  // Which slide is open, and what to call it. Held here rather than per chapter
  // so there is one viewer for the whole scroll and no way to end up with two.
  const [viewing, setViewing] = useState<Viewing | null>(null)

  const scroller = useRef<HTMLDivElement>(null)
  const { root: nearbyRoot, watch } = useNearbyRoot()
  // The reader's scroller is read directly (the jump, the scroll-spy) and is
  // also the root the chapters measure their distance from.
  const setScroller = useCallback(
    (element: HTMLDivElement | null) => {
      scroller.current = element
      nearbyRoot(element)
    },
    [nearbyRoot],
  )
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

  /*
    The open slide, resolved from the current chapters and tasks rather than
    from what was true when it was clicked.

    This is what makes a redraw visible without closing anything: the asset id
    is read here, so the moment the new one lands in the cache the dialog's
    `src` changes. It also closes itself if the slide disappears underneath it,
    which is the only sane answer to a chapter that has gone away.
  */
  const open = useMemo(() => {
    if (!viewing) return null
    const chapter = chapters.find((row) => row.id === viewing.chapterId)
    const assetId = chapter?.slideAssetIds[viewing.slot]
    if (!chapter || !assetId) return null
    return {
      chapter,
      slot: viewing.slot,
      assetId,
      cell: stages.get(chapter.id)?.slides[viewing.slot],
    }
  }, [viewing, chapters, stages])

  const jump = (id: string) => {
    const root = scroller.current
    const target = root?.querySelector<HTMLElement>(`[data-chapter="${id}"]`)
    if (!root || !target) return
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
    // Chapters off screen are not laid out until they come into view, so the
    // distance to a far one is an estimate until the scroll gets there. Once it
    // settles, the landing is checked and corrected without animation.
    const settle = () => {
      root.removeEventListener('scrollend', settle)
      window.clearTimeout(timer)
      const off = target.getBoundingClientRect().top - root.getBoundingClientRect().top
      if (Math.abs(off) > 2) target.scrollIntoView({ block: 'start' })
    }
    root.addEventListener('scrollend', settle)
    const timer = window.setTimeout(settle, 900)
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
    // `@container` so the outline can take itself away when the window is too
    // narrow for a column of it beside the reader.
    <div className="@container flex min-h-0 flex-1">
      <ChapterOutline chapters={chapters} activeId={active} onJump={jump} />
      <div className="flex min-w-0 flex-1 flex-col">
        <FilterBar shown={shown} onToggle={(key) => setShown((s) => ({ ...s, [key]: !s[key] }))} />
        {/* A container of its own, so a chapter lays its script and its media
            side by side against the reader's width rather than the window's. */}
        <div ref={setScroller} className="@container/reader min-h-0 flex-1 overflow-y-auto">
          <NearbyProvider value={watch}>
            {chapters.map((chapter, index) => (
              <ChapterBlock
                key={chapter.id}
                chapter={chapter}
                eager={index < EAGER}
                slides={stages.get(chapter.id)?.slides ?? NO_CELLS}
                shown={shown}
                onView={setViewing}
              />
            ))}
          </NearbyProvider>
        </div>
      </div>
      {open ? (
        <SlideViewer
          chapterId={open.chapter.id}
          slot={open.slot}
          videoId={video.id}
          src={`/assets/${open.assetId}`}
          title={`${open.chapter.title} · Slide ${open.slot + 1}`}
          prompt={open.chapter.slidePrompts[open.slot]}
          stale={open.cell?.stale ?? false}
          drawing={open.cell?.state === 'running'}
          onClose={() => setViewing(null)}
        />
      ) : null}
    </div>
  )
}

/**
 * The column everything in a chapter lines up in.
 *
 * It used to be a 40rem measure centred in the reader, which kept the script
 * readable and left two thirds of a wide window empty. The measure belongs to
 * the *script* and nothing else, so it is the script that carries it now: on a
 * wide reader a chapter is two columns, the script at a readable width and the
 * narration, slides and clip beside it, and this cap only stops the pair from
 * drifting apart on a very large display.
 */
const COLUMN = 'mx-auto w-full max-w-[84rem]'

/** The script's own measure, for when it has the chapter to itself. */
const MEASURE = 'max-w-[52rem]'

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
/**
 * What to draw, as four switches.
 *
 * Above the scroll rather than in the window's mode bar: these belong to this
 * view and nothing else, and the mode bar is shared chrome. Indented to the
 * same column the chapter bands use, so the row reads as the head of the list
 * rather than as a strip laid over it.
 */
function FilterBar({ shown, onToggle }: { shown: Shown; onToggle: (key: keyof Shown) => void }) {
  return (
    <div className="hairline-b shrink-0">
      <div className="px-6 py-2">
        <div className={cn(COLUMN, 'flex items-center gap-1.5')}>
          <span className="mr-1 text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase">
            Show
          </span>
          {SECTIONS.map((section) => {
            const on = shown[section.key]
            return (
              <button
                key={section.key}
                type="button"
                aria-pressed={on}
                onClick={() => onToggle(section.key)}
                className={cn(
                  'rounded-[5px] px-2 py-[3px] text-[11px] transition-colors',
                  on ? 'font-medium' : 'text-tertiary hover:bg-[var(--hover)] hover:text-secondary',
                )}
                style={
                  on ? { backgroundColor: 'var(--accent-wash)', color: 'var(--accent)' } : undefined
                }
              >
                {section.label}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

const SCRIPT = [
  'max-h-[24rem] overflow-y-auto',
  'rounded-[7px] px-3.5 py-3',
  'font-mono text-[12px] leading-[1.65] whitespace-pre-wrap',
  'text-primary',
].join(' ')

const PANEL = {
  backgroundColor: 'var(--band)',
  boxShadow: '0 0 0 0.5px var(--separator)',
}

/** How many chapters draw in full before the observer has said anything. */
const EAGER = 4

/** The stand-in height for a body that has never been drawn. */
const ESTIMATE = 520

/**
 * One chapter of the reader.
 *
 * Memoised: its chapter and its slide cells keep their identity until something
 * in this chapter moves, so a frame that lands a slide in chapter 12 redraws
 * chapter 12 rather than every script on the page.
 */
const ChapterBlock = memo(function ChapterBlock({
  chapter,
  eager,
  slides,
  shown,
  onView,
}: {
  chapter: Chapter
  /** Drawn in full from the first frame: the chapters the reader opens on. */
  eager: boolean
  slides: Cell[]
  shown: Shown
  onView: (viewing: Viewing) => void
}) {
  const words = wordsIn(chapter.script)
  const section = useRef<HTMLElement>(null)
  const inner = useRef<HTMLDivElement>(null)
  const near = useNear(section, eager, () => {
    const measured = inner.current?.offsetHeight ?? 0
    if (measured > 0) height.current = measured
  })
  // The height the body last drew at, held for the spacer that stands in for
  // it while it is away, so the scroll does not jump when it comes back.
  const height = useRef<number | null>(null)
  // A body that has been pressed or focused stays drawn: it may be playing its
  // narration or holding an unsaved edit, and taking it away would end both.
  const [engaged, setEngaged] = useState(false)
  const drawn = near || engaged
  // Held here rather than up in the reader: the button that opens it is in this
  // component, so there is nothing to thread, and a chapter has one clip.
  const [playing, setPlaying] = useState(false)

  // The script part draws either way — it says so when a chapter has none — so
  // it alone is enough to keep the body. The other three have nothing to say
  // about an artifact that does not exist.
  const narration = shown.narration && !!chapter.audioAssetId
  const pictures = shown.slides && slides.length > 0
  const clip = shown.clip && !!chapter.clipAssetId
  const media = narration || pictures || clip
  const body = shown.script || media
  // Two columns only when there is something for each of them; a script alone
  // keeps its measure, and media alone takes the width a contact sheet wants.
  const split = shown.script && media

  return (
    // `chapter-section` lets the browser skip laying out and painting the
    // chapters that are off screen — a long video is mostly off screen.
    <section ref={section} data-chapter={chapter.id} className="chapter-section">
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
              ? `${count(words)} words · ${duration(chapterSeconds(chapter))}`
              : 'not written yet'}
          </span>
        </div>
      </header>

      {/* Padding outside the column and not on it, exactly as the header does
          it: with `px-6` on the capped element itself the two would be measured
          from different edges and the ordinal would sit 24px off the first word. */}
      {/* Hidden whole rather than left empty: with every section switched off a
          chapter is its header alone, and thirty-two rems of padding under each
          one would turn a table of contents into a column of gaps. */}
      {body && !drawn ? <div aria-hidden style={{ height: height.current ?? ESTIMATE }} /> : null}
      {body && drawn ? (
        <div
          ref={inner}
          className="px-6 pt-5 pb-10"
          onPointerDownCapture={() => setEngaged(true)}
          onFocusCapture={() => setEngaged(true)}
        >
          <div
            className={cn(
              COLUMN,
              'grid gap-x-8 gap-y-7',
              split && '@[60rem]/reader:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)]',
            )}
          >
            {shown.script ? (
              <div className={cn('min-w-0', !split && MEASURE)}>
                <ScriptPart chapter={chapter} />
              </div>
            ) : null}

            {media ? (
              <div className="flex min-w-0 flex-col gap-7">
                {narration ? (
                  <Part label="Narration">
                    <Narration
                      assetId={chapter.audioAssetId ?? ''}
                      seconds={chapter.audioDurationSeconds}
                    />
                  </Part>
                ) : null}

                {pictures ? (
                  <Part label="Slides">
                    <div className="grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-2.5">
                      {slides.map((cell, slot) => (
                        <Slide
                          key={slot}
                          cell={cell}
                          id={chapter.slideAssetIds[slot]}
                          slot={slot}
                          onView={() => onView({ chapterId: chapter.id, slot })}
                        />
                      ))}
                    </div>
                  </Part>
                ) : null}

                {clip ? (
                  <Part label="Clip">
                    <Button className="self-start" onClick={() => setPlaying(true)}>
                      Play clip
                    </Button>
                  </Part>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      {playing && chapter.clipAssetId ? (
        <ClipViewer
          src={`/assets/${chapter.clipAssetId}`}
          title={`${chapter.title} · Clip`}
          onClose={() => setPlaying(false)}
        />
      ) : null}
    </section>
  )
})

/**
 * One of the three things a chapter is made of, with its name over it.
 *
 * The names are the whole fix for "it is hard to tell that is a script". A wall
 * of prose with nothing above it is ambiguous — it could be a summary, a
 * transcript, a note — and the chapter title two lines up does not answer the
 * question. One word does. They are the same 10px uppercase caption FINAL and
 * INSPECTOR already use, so this reads as the same application.
 */
/**
 * The script, and the one thing anyone wants to do to it.
 *
 * Reading is the default posture of this page, so editing is a mode you enter
 * rather than a textarea sitting open: the panel keeps the same width, the same
 * face and the same size in both, so entering the mode moves no text.
 *
 * Nothing guards the button. A script task still to run will overwrite whatever
 * is typed here when it lands, and a script gate is exactly the moment an edit
 * is wanted — the two are indistinguishable from a button's point of view, and
 * the operator knows which one they are looking at. The server takes the same
 * position: it writes the edit, flags the narration and the clip derived from
 * the replaced text as stale, and re-runs nothing.
 */
function ScriptPart({ chapter }: { chapter: Chapter }) {
  const client = useQueryClient()
  // null is "not editing". A separate boolean would let the two disagree about
  // whether there is a draft.
  const [draft, setDraft] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: (script: string) => api.updateChapterScript(chapter.id, script),
    // The response is the whole row, so this patches the cache rather than
    // refetching — and the word count in the header moves with the patch.
    onSuccess: (updated) => {
      client.setQueryData<Chapter[]>(qk.chapters(chapter.videoId), (prev) =>
        prev?.map((row) => (row.id === updated.id ? updated : row)),
      )
      setDraft(null)
    },
  })

  if (draft === null) {
    return (
      <Part
        label="Script"
        action={
          <Button className="h-[22px] px-2.5 text-[11px]" onClick={() => setDraft(chapter.script)}>
            Edit
          </Button>
        }
      >
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
    )
  }

  const trimmed = draft.trim()
  // An empty script is refused by the server, and an unchanged one is a request
  // that would flag the narration stale for nothing.
  const ready = trimmed !== '' && trimmed !== chapter.script && !save.isPending

  return (
    <Part
      label="Script"
      action={
        <>
          <Button
            className="h-[22px] px-2.5 text-[11px]"
            onClick={() => {
              save.reset()
              setDraft(null)
            }}
            disabled={save.isPending}
          >
            Cancel
          </Button>
          <Button
            primary
            className="h-[22px] px-2.5 text-[11px]"
            onClick={() => save.mutate(trimmed)}
            disabled={!ready}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </Button>
        </>
      }
    >
      {/* Same metrics as the `pre` it replaces, so the words do not move when
          the mode changes. A minimum height rather than the panel's cap: an
          edit is worth more room than a read, and a box that scrolls at twenty-four
          rem while you are writing in it is the wrong trade. */}
      <textarea
        autoFocus
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        className={cn(SCRIPT, 'max-h-none min-h-[24rem] w-full resize-y outline-none')}
        style={{ ...PANEL, boxShadow: '0 0 0 1px var(--accent)' }}
      />
      {save.error ? (
        <p className="text-[12px]" style={{ color: 'var(--failed)' }}>
          {(save.error as Error).message}
        </p>
      ) : null}
    </Part>
  )
}

/**
 * A chapter's narration: a stand-in until it is pressed, the platform player
 * after.
 *
 * The player is the platform's own — it knows how to seek, how to answer the
 * keyboard and where the system volume goes, and the asset handler serves
 * ranges, so scrubbing works. But WebKit builds every player's controls in
 * script, and a reader of eighty narrated chapters built eighty of them on the
 * click that opened it: measured, two seconds of a frozen window. So nothing is
 * built until someone asks to listen, and the press that asks is the one that
 * starts it — the stand-in is the same height, so nothing moves when it swaps.
 *
 * Fetched on the press and not before. Narration is served as WAV, ten
 * megabytes a minute, and a page of players that fetched on sight would pull
 * the better part of a hundred megabytes for a page nobody has played.
 */
function Narration({ assetId, seconds }: { assetId: string; seconds: number }) {
  const [live, setLive] = useState(false)
  if (live) {
    return (
      <audio
        controls
        autoPlay
        preload="auto"
        src={`/assets/${assetId}`}
        className="h-[32px] w-full"
      />
    )
  }
  return (
    <button
      type="button"
      // On the press, not the release, so the player is there and starting by
      // the time the hand lifts.
      onPointerDown={(event) => {
        if (event.button === 0) setLive(true)
      }}
      onClick={() => setLive(true)}
      aria-label="Play narration"
      className="group/play flex h-[32px] w-full items-center gap-2.5 rounded-full px-1 text-left"
      style={PANEL}
    >
      <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-[var(--accent)] text-white transition-transform duration-100 group-hover/play:scale-105">
        <Play className="ml-px size-3 fill-current" strokeWidth={0} />
      </span>
      <span className="text-[12px] font-medium text-secondary group-hover/play:text-primary">
        Play
      </span>
      {seconds > 0 ? (
        <span className="ml-auto pr-3 text-[11.5px] tabular-nums text-tertiary">
          {duration(seconds)}
        </span>
      ) : null}
    </button>
  )
}

function Part({
  label,
  action,
  children,
}: {
  label: string
  /** Buttons for this part, on the label's line. */
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex min-h-[20px] items-center gap-3">
        <span className="text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase">
          {label}
        </span>
        {action ? <div className="ml-auto flex items-center gap-1.5">{action}</div> : null}
      </div>
      {children}
    </div>
  )
}

/**
 * 1344×768 is what the composer frames a slide at. The width is the grid's, so
 * the tiles grow with the column instead of leaving a ragged gap at its end.
 */
const TILE = 'aspect-[1344/768] w-full rounded-[6px]'

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
  onView: () => void
}) {
  if (id) {
    return (
      <button
        type="button"
        onClick={onView}
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
