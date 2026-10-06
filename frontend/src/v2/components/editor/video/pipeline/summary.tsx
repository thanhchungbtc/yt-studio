import { useState } from 'react'

import { count, duration } from '../../../../core/format'
import type { Video } from '../../../../core/types'
import { Button } from '../../../ui/button'
import { Clamped } from '../../../ui/clamped'
import { columnTotals, projectedSeconds } from '../stages'
import { BlueprintPopover } from './blueprint'

/** The shape of the thing: what it is made of, and how long it runs. */
function shapeOf(video: Video, totals: ReturnType<typeof columnTotals>): string {
  const words = totals.words || totals.estimatedWords
  // Always a projection, never a measurement: the seconds are the sum of each
  // chapter's words at the narration speed the blueprint budgeted with.
  //
  // Before any script is written that sum is zero, and this used to fall back
  // to the *target* duration — which is what was asked for, not what was
  // planned. A blueprint that budgets nine hundred words against a two-minute
  // target then reported "~2m" for seven minutes of narration. Projecting the
  // budget says what the plan actually is, which is the number you approve on.
  const runtime = duration(totals.seconds > 0 ? totals.seconds : projectedSeconds(words))
  const parts = [
    `${video.chapterCount} chapters`,
    `${video.slidesPerChapter} slides each`,
    `${video.thumbnailCells} thumbnail tiles`,
    `${count(words)} words`,
    `~${runtime}`,
  ]
  // The target only when there is one, and named, because it is the one figure
  // on this line that is a request rather than a projection: zero means the
  // chapter count decides, and printing "target 0m" beside a runtime would read
  // as a contradiction rather than an absence.
  if (video.targetDurationMinutes > 0) {
    parts.push(`target ${video.targetDurationMinutes}m`)
  }
  return parts.join(' · ')
}

/**
 * The brief and the shape, in that order of importance.
 *
 * The topic used to appear nowhere at all, which made the most load-bearing
 * string in a video the one thing on screen you could not read: it steers the
 * blueprint, and it is re-sent as the summary on every chapter's script call.
 * It sits under the shape because the shape is the line the controls are on,
 * and it is folded to two lines because a topic can be five thousand
 * characters and this is a header rather than a document. The rest is a press
 * away, which is what keeps the fold from being a sentence that stops
 * mid-word.
 */
export function SummaryLine({
  video,
  totals,
  editing,
  editable,
  onToggleEditing,
  staleCount,
  onlyStale,
  onToggleStale,
}: {
  video: Video
  totals: ReturnType<typeof columnTotals>
  editing: boolean
  editable: boolean
  onToggleEditing: () => void
  /** How many chapters hold something stale. Zero hides the control entirely. */
  staleCount: number
  onlyStale: boolean
  onToggleStale: () => void
}) {
  // Per document and no further, like the table's own rows: whether you had a
  // topic open is not worth a line in the store.
  const [topicOpen, setTopicOpen] = useState(false)

  return (
    <div className="hairline-b shrink-0 px-4 py-2">
      <div className="flex items-center gap-2">
        <span
          className="min-w-0 flex-1 truncate text-sm text-fg-muted"
          title={shapeOf(video, totals)}
        >
          {shapeOf(video, totals)}
        </span>
        {/*
        This line describes the plan, so the way to the plan belongs on it —
        reading it, then changing it. Each hidden until there is one.

        Those two and nothing else. Cancel used to sit here as well, one tab stop
        from Edit and the same size, which put the verb that kills a render in
        progress beside a switch that restyles some text. It lives with the other
        lifecycle verbs now, in the strip above.
      */}
        {/*
          Absent when nothing is stale, which is most of the time — a filter
          that can only ever find nothing is a control that teaches you it does
          nothing. Appearing is also how a video says something *went* stale,
          which until now was a thing you found by scrolling.
        */}
        {staleCount > 0 ? (
          <button
            type="button"
            aria-pressed={onlyStale}
            onClick={onToggleStale}
            title={
              onlyStale
                ? 'Showing only the chapters with stale artifacts'
                : 'Show only the chapters with stale artifacts'
            }
            className="flex shrink-0 items-center gap-1.5 rounded-[5px] px-2 py-[3px] text-xs transition-colors"
            style={
              onlyStale
                ? { backgroundColor: 'var(--running)', color: '#fff' }
                : { backgroundColor: 'var(--band)', color: 'var(--running)' }
            }
          >
            <span aria-hidden>⚠</span>
            <span className="tabular-nums">{count(staleCount)}</span> stale
          </button>
        ) : null}
        {video.blueprintAssetId ? <BlueprintPopover assetId={video.blueprintAssetId} /> : null}
        {editable ? <Button onClick={onToggleEditing}>{editing ? 'Done' : 'Edit'}</Button> : null}
      </div>

      {/* Absent rather than empty when nothing was briefed: a label over blank
          space would be the header claiming a field the video does not have.

          The same control the chapter briefs in the table below carry, which is
          what makes the two read as one page. What it replaced was a `title`
          attribute, and a five-thousand character topic is not a tooltip. */}
      {video.topic ? (
        <Clamped
          text={video.topic}
          open={topicOpen}
          onToggle={() => setTopicOpen((on) => !on)}
          label="Video topic"
          className="mt-1 text-sm leading-snug text-fg-subtle"
        />
      ) : null}
    </div>
  )
}
