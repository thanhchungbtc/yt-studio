import { count, duration } from '../../../../core/format'
import type { Video } from '../../../../core/types'
import { Button } from '../../../ui/button'
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
 * It sits above the shape because it is what the video *is*, where the shape is
 * how big it is, and it is clamped to two lines because a brief can be five
 * thousand characters and this is a header rather than a document.
 */
export function SummaryLine({
  video,
  totals,
  editing,
  editable,
  onToggleEditing,
}: {
  video: Video
  totals: ReturnType<typeof columnTotals>
  editing: boolean
  editable: boolean
  onToggleEditing: () => void
}) {
  return (
    <div className="hairline-b shrink-0 px-4 py-2">
      <div className="flex items-center gap-2">
        <span
          className="min-w-0 flex-1 truncate text-[12px] text-secondary"
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
        {video.blueprintAssetId ? <BlueprintPopover assetId={video.blueprintAssetId} /> : null}
        {editable ? <Button onClick={onToggleEditing}>{editing ? 'Done' : 'Edit'}</Button> : null}
      </div>

      {/* Absent rather than empty when nothing was briefed: a label over blank
          space would be the header claiming a field the video does not have. */}
      {video.topic ? (
        <p className="mt-1 line-clamp-2 text-[12px] leading-snug text-tertiary" title={video.topic}>
          {video.topic}
        </p>
      ) : null}
    </div>
  )
}
