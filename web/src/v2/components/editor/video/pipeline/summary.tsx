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
  return `${video.chapterCount} chapters · ${count(words)} words · ~${runtime}`
}

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
    <div className="hairline-b flex shrink-0 items-center gap-2 px-4 py-2">
      <span className="min-w-0 flex-1 truncate text-[12px] text-secondary">
        {shapeOf(video, totals)} · {video.slidesPerChapter} slides each
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
  )
}
