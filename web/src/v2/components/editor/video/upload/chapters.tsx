import { useMemo } from 'react'

import type { Chapter } from '../../../../core/types'
import { cn } from '../../../../core/utils'
import { Caption } from './caption'

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
 * The times are in the *player's* units, not the plan's. `durationSeconds` is
 * what the blueprint budgeted, and the cut is whatever ffmpeg produced — here,
 * 7:08 of planned narration against a 2:47 render. Left alone, this would send
 * you to 3:34 of a video that ends at 2:47. So once the player reports its
 * duration each chapter's *share* of the plan is scaled onto the real runtime:
 * the order and the proportions are the blueprint's, the absolute times are the
 * file's, and a click always lands inside the video. Before the player has read
 * its header the plan is all there is, and the list uses it unscaled.
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
  seconds,
  runtime,
  seekable,
  onSeek,
}: {
  chapters: Chapter[]
  /** Where the player is. */
  seconds: number
  /** How long the cut actually runs, once the player knows. */
  runtime: number | undefined
  seekable: boolean
  onSeek: (at: number) => void
}) {
  const { rows, timed } = useMemo(() => {
    const planned = chapters.reduce((sum, chapter) => sum + chapter.durationSeconds, 0)
    // Everything below is in player seconds. See the note above on why.
    const scale = runtime && planned > 0 ? runtime / planned : 1
    let start = 0
    const built = chapters.map((chapter) => {
      const row: Row = {
        id: chapter.id,
        ordinal: chapter.ordinal,
        title: chapter.title,
        start,
        seconds: chapter.durationSeconds * scale,
      }
      start += row.seconds
      return row
    })
    // With nothing narrated yet every duration is zero, so every start is zero
    // and a column of identical `0:00` is worse than no column: it looks like an
    // answer. The order is known, the times are not.
    return { rows: built, timed: planned > 0 }
  }, [chapters, runtime])

  // Which one is playing. Null until there is a cut with a duration, because
  // before that nothing is playing and lighting the first row would say
  // otherwise.
  const playing = useMemo(() => {
    if (!runtime) return null
    // The player snaps a seek back to the nearest keyframe, so clicking a
    // chapter can land a fraction of a second *before* it starts. Without the
    // tolerance the row you just clicked is the one row that does not light up.
    const at = seconds + 0.5
    let found: string | null = null
    for (const row of rows) if (row.start <= at) found = row.id
    return found
  }, [rows, runtime, seconds])

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
