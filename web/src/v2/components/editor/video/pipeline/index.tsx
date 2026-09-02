import { useMemo, useState } from 'react'

import { columnTotals } from '../stages'
import type { ViewProps } from '../view'
import { Legend } from './legend'
import { SummaryLine } from './summary'
import { ChapterTable } from './table'

/**
 * The video as a thing that either happened or did not.
 *
 * Chapters down, stages across, one mark per artifact. It answers *where did
 * this get to* and nothing else — read-only over everything the pipeline
 * produced, because the pipeline runs on its own and what a person needs first
 * is to see where it stopped.
 *
 * The *plan* is the exception, and it is not really one: the blueprint's titles,
 * briefs and word budgets are the pipeline's inputs rather than its output, and
 * they are what the gate is asking about. Edit puts the table's first three
 * columns into fields; nothing is re-run, because a chapter that has not been
 * written yet is written from the edit, and one that has been is the operator's
 * to reconsider.
 */
export function PipelineView({ video, chapters, tasks }: ViewProps) {
  // Here rather than on the editor because nothing outside this view can edit,
  // and a mode that is not on screen should not be holding state.
  const [editing, setEditing] = useState(false)

  const totals = useMemo(
    () => columnTotals(chapters, video.slidesPerChapter),
    [chapters, video.slidesPerChapter],
  )

  // There is nothing to edit until the blueprint has written the rows, and a
  // switch that turns an empty table into an empty table is a switch that
  // teaches the wrong thing about what it does.
  const editable = chapters.length > 0

  return (
    <>
      <SummaryLine
        video={video}
        totals={totals}
        editing={editing && editable}
        editable={editable}
        onToggleEditing={() => setEditing((on) => !on)}
      />

      <ChapterTable
        videoId={video.id}
        chapters={chapters}
        tasks={tasks}
        slidesPerChapter={video.slidesPerChapter}
        editing={editing && editable}
      />

      <Legend />
    </>
  )
}
