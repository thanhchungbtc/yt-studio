import { useMemo } from 'react'

import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'
import { Caption } from '../../../ui/caption'
import { chapterSeconds } from '../stages'

/**
 * The chapters, as a list you click.
 *
 * A list view in the app's own idiom: inset rounded rows, the ordinal, the
 * title, and where it starts. The row the player is inside is the selected one,
 * filled in accent — which is the same thing the source list does, and the same
 * thing Music does with the track that is playing.
 *
 * It was a proportional rail before, with a tick per chapter and a playhead
 * sliding down it. That drew the *shape* of the video, which is a thing worth
 * knowing and not a thing worth putting a bespoke widget in a sidebar for. A
 * list is what a list of chapters is, and it is one row per chapter at one
 * height whether there are two of them or fifty.
 *
 * The times come from the render itself. `video.chapterOffsets` is the array the
 * concat handed ffmpeg to place its crossfades at, so a row's time is not an
 * estimate of where a chapter starts — it is where the cut was told to put it.
 * No scaling, and no waiting for the player to report a duration.
 *
 * Before there is a render there are no offsets, and the list falls back to
 * projecting each chapter from its narration and spreading that across whatever
 * the player says it is playing. That fallback is worth understanding as the
 * weaker thing it is: a chapter's clip carries a fixed pad of a few seconds
 * regardless of length, and scaling by *share* spreads that pad in proportion
 * instead of evenly, which walks the later rows off by seconds on a long video.
 * It is close enough to read a plan by and not close enough to seek by, which is
 * exactly why the offsets are stored rather than recomputed here.
 */
interface Row {
  id: string
  ordinal: number
  title: string
  /** Seconds into the cut. */
  start: number
  seconds: number
}

/**
 * An offset into the video, where zero is a real answer.
 *
 * Not `duration()` from core: that formats a *length*, and a length of zero is
 * nothing at all, so it prints an em dash. The first chapter starts at zero and
 * "—" is the wrong thing to say about it.
 */
function offset(seconds: number): string {
  const whole = Math.max(0, Math.round(seconds))
  const s = String(whole % 60).padStart(2, '0')
  const m = Math.floor(whole / 60) % 60
  const h = Math.floor(whole / 3600)
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
}

export function ChapterList({
  chapters,
  offsets,
  seconds,
  runtime,
  seekable,
  onSeek,
}: {
  chapters: Chapter[]
  /** The render's own chapter timeline; empty before there is a render. */
  offsets: number[]
  /** Where the player is. */
  seconds: number
  /** How long the cut runs, once the player knows. Only the fallback needs it. */
  runtime: number | undefined
  seekable: boolean
  onSeek: (at: number) => void
}) {
  const { rows, timed } = useMemo(() => {
    // One offset per chapter or none at all: a shorter array is a timeline from
    // a render with a different number of chapters, and lining it up by index
    // would point every row at the wrong place rather than at nothing.
    const exact = offsets.length === chapters.length ? offsets : null

    const planned = chapters.reduce((sum, chapter) => sum + chapterSeconds(chapter), 0)
    const scale = runtime && planned > 0 ? runtime / planned : 1
    let start = 0
    const built = chapters.map((chapter, index) => {
      const seconds = chapterSeconds(chapter) * scale
      const row: Row = {
        id: chapter.id,
        ordinal: chapter.ordinal,
        title: chapter.title,
        // `?? start` is unreachable — the length check above is what makes the
        // index safe — and it is the honest fallback rather than a non-null
        // assertion, which would trade a wrong time for a blank row.
        start: exact?.[index] ?? start,
        seconds,
      }
      start += seconds
      return row
    })
    // With nothing narrated yet every duration is zero, so every start is zero
    // and a column of identical `0:00` is worse than no column: it looks like an
    // answer. The order is known, the times are not.
    return { rows: built, timed: exact !== null || planned > 0 }
  }, [chapters, offsets, runtime])

  // Which one is playing. Null until there is a cut with a duration, because
  // before that nothing is playing and lighting the first row would say
  // otherwise.
  const playing = useMemo(() => {
    if (!runtime && !timed) return null
    // The player snaps a seek back to the nearest keyframe, so clicking a
    // chapter can land a fraction of a second *before* it starts. Without the
    // tolerance the row you just clicked is the one row that does not light up.
    const at = seconds + 0.5
    let found: string | null = null
    for (const row of rows) if (row.start <= at) found = row.id
    return found
  }, [rows, runtime, timed, seconds])

  return (
    <div className="px-2 py-3">
      <Caption className="px-2">Chapters</Caption>

      {rows.length === 0 ? (
        <p className="mt-2 px-2 text-[12px] text-tertiary">
          The blueprint writes the chapters first.
        </p>
      ) : (
        <div className="mt-1.5 flex flex-col">
          {rows.map((row) => (
            <ChapterRow
              key={row.id}
              row={row}
              selected={playing === row.id}
              timed={timed}
              seekable={seekable}
              onSeek={onSeek}
            />
          ))}
        </div>
      )}
    </div>
  )
}

/**
 * One row: the ordinal, the title, the moment it starts.
 *
 * The inset rounded pill rather than a full-bleed band, which is most of why
 * the source list reads as macOS and not as a table — same reason here.
 */
function ChapterRow({
  row,
  selected,
  timed,
  seekable,
  onSeek,
}: {
  row: Row
  selected: boolean
  /** Whether the start time is known at all; see the note in `ChapterList`. */
  timed: boolean
  seekable: boolean
  onSeek: (at: number) => void
}) {
  return (
    <button
      type="button"
      // Nothing to seek in until there is a cut. The row still draws — the plan
      // is worth reading before the video exists — it just is not a control.
      disabled={!seekable}
      onClick={() => onSeek(row.start)}
      aria-current={selected}
      // The full text, because a narrow pane truncates most of these.
      title={row.title}
      className={cn(
        'group flex w-full items-baseline gap-2 rounded-[7px] px-2 py-[5px] text-left',
        'transition-colors duration-75',
        !selected && seekable && 'hover:bg-[var(--hover)]',
      )}
      style={selected ? { backgroundColor: 'var(--accent)' } : undefined}
    >
      <span
        className={cn(
          'w-[13px] shrink-0 text-right text-[11px] tabular-nums',
          selected ? 'text-white/70' : 'text-tertiary',
        )}
      >
        {row.ordinal}
      </span>
      <span
        className={cn(
          'min-w-0 flex-1 truncate text-[12px]',
          selected ? 'text-white' : 'text-secondary group-hover:text-primary',
        )}
      >
        {row.title}
      </span>
      {timed ? (
        <span
          className={cn(
            'shrink-0 text-[11px] tabular-nums',
            selected ? 'text-white/80' : 'text-tertiary',
          )}
        >
          {offset(row.start)}
        </span>
      ) : null}
    </button>
  )
}
