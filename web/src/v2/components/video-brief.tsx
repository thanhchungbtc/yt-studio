import type { ReactNode } from 'react'

import type { VideoBrief } from '../core/api'
import { count } from '../core/format'
import type { Video } from '../core/types'
import { Field, FieldDivider, Input, NumberField, Textarea } from './ui/field'

/**
 * The six fields a video is briefed with, and the one place they are described.
 *
 * Two dialogs ask for them — the one that creates a video and the one that
 * corrects it — and they were two copies of the same six labels, the same four
 * ranges and the same two character counters. The copies were the problem: a
 * limit raised on one side was a form that only learned its new bound by being
 * refused on the other.
 *
 * What is *not* here is everything the two dialogs disagree about. Creating a
 * video picks a channel, projects a task count and can start the pipeline;
 * editing one can do none of the three. Those live with the dialog that has
 * them, and this file holds the part that is genuinely the same question.
 *
 * The draft is strings, not numbers, because a half-typed number field holds
 * `""` and a `0` there would be the form filling in an answer nobody gave.
 * `requestFrom` is where they become the numbers the endpoint takes.
 */

/** The draft, in the form the controls hold it. */
export interface Brief {
  title: string
  topic: string
  chapterCount: string
  durationMinutes: string
  slidesPerChapter: string
  thumbnailCells: string
}

// The server's caps, from the CreateVideoInput and UpdateVideoInput schemas in
// delivery/http/videos.go. Mirrored rather than discovered: they are part of
// the request contract, and a form that finds out its own limits by being
// rejected is the thing this mirrors them to avoid. Both count the trimmed
// value, because trimmed is what gets sent.
export const TITLE_MAX = 200
export const TOPIC_MAX = 5000

/** What a new video starts from, absent anything else to go on. */
export const BLANK_BRIEF: Brief = {
  title: '',
  topic: '',
  chapterCount: '50',
  durationMinutes: '180',
  slidesPerChapter: '2',
  thumbnailCells: '12',
}

/** The saved video, as a draft to edit. */
export function briefFrom(video: Video): Brief {
  return {
    title: video.title,
    topic: video.topic,
    chapterCount: String(video.chapterCount),
    durationMinutes: String(video.targetDurationMinutes),
    slidesPerChapter: String(video.slidesPerChapter),
    thumbnailCells: String(video.thumbnailCells),
  }
}

/** The draft, as the endpoints take it. */
export function requestFrom(brief: Brief): VideoBrief {
  return {
    title: brief.title.trim(),
    topic: brief.topic.trim(),
    chapterCount: Number(brief.chapterCount) || 0,
    slidesPerChapter: Number(brief.slidesPerChapter) || 0,
    thumbnailCells: Number(brief.thumbnailCells) || 0,
    targetDurationMinutes: Number(brief.durationMinutes) || 0,
  }
}

/**
 * Whether the draft can be sent.
 *
 * Only the two prose fields are checked. The four numbers are `<input
 * type="number">` with a min and a max, so the control already refuses what is
 * out of range — where a title is typed freely and a topic is pasted, and both
 * can be over a limit that nothing on the way in enforces.
 */
export function briefReady(brief: Brief): boolean {
  const title = brief.title.trim().length
  return title > 0 && title <= TITLE_MAX && brief.topic.trim().length <= TOPIC_MAX
}

/**
 * The character count, shown only once it is worth knowing.
 *
 * A counter under an empty field is noise on every field nobody was ever going
 * to overrun. This one appears in the last fifth — where "will this fit" starts
 * being a real question — and turns red once the answer is no.
 *
 * Neither field is given a `maxLength`. The browser enforces that by silently
 * discarding the overflow, including on paste, and a topic is pasted prose: the
 * operator would lose the tail of a brief without being told. Showing the
 * overrun and refusing to submit keeps the text where they can edit it.
 */
function counterFor(length: number, max: number): ReactNode {
  if (length < max * 0.8) return undefined
  return (
    <span
      className="shrink-0 tabular-nums"
      style={length > max ? { color: 'var(--failed)' } : undefined}
    >
      {count(length)}/{count(max)}
    </span>
  )
}

/**
 * The six fields: what the video is *about*, then how big it is.
 *
 * The rule between the two groups is the whole layout. Above it the fields are
 * prose and take the full width — the topic most of all, which is the string
 * that steers the blueprint, every script and every slide prompt, and is worth
 * more room than anything else in either dialog.
 *
 * Below it they are numbers, in two columns. Stacked, they were four rows of a
 * sixty-eight pixel box against a full-width dialog, which is three quarters of
 * four rows spent on nothing; paired, they cost two rows and the space goes to
 * the brief above instead.
 */
export function BriefFields({
  brief,
  onChange,
}: {
  brief: Brief
  onChange: (brief: Brief) => void
}) {
  const set = <K extends keyof Brief>(key: K, value: Brief[K]) =>
    onChange({ ...brief, [key]: value })

  const titleLength = brief.title.trim().length
  const topicLength = brief.topic.trim().length
  const minutes = Number(brief.durationMinutes) || 0

  return (
    <>
      <Field label="Title" hint={counterFor(titleLength, TITLE_MAX)}>
        {(id) => (
          <Input
            id={id}
            data-autofocus
            value={brief.title}
            onChange={(event) => set('title', event.target.value)}
            placeholder="The Long Winter of the Harbour"
          />
        )}
      </Field>

      <Field
        label="Topic"
        hint={
          <span className="flex justify-between gap-3">
            <span>Steers the blueprint, the scripts and the slide prompts.</span>
            {counterFor(topicLength, TOPIC_MAX)}
          </span>
        }
      >
        {(id) => (
          <Textarea
            id={id}
            rows={9}
            value={brief.topic}
            onChange={(event) => set('topic', event.target.value)}
            placeholder="A northern port town over one winter, told through its shipping ledgers."
          />
        )}
      </Field>

      <FieldDivider />

      <div className="grid grid-cols-2 gap-x-6">
        <NumberField
          label="Duration"
          // Zero is not a length, it is the absence of one, and the unit is
          // where that gets said. Two words rather than the sentence this used
          // to be — in two columns the unit has about a hundred pixels, and a
          // unit that truncates is worse than a short one.
          unit={minutes > 0 ? 'minutes' : 'automatic'}
          value={brief.durationMinutes}
          onChange={(value) => set('durationMinutes', value)}
          min={0}
          max={720}
        />
        <NumberField
          label="Chapters"
          unit="a target, 1–500"
          value={brief.chapterCount}
          onChange={(value) => set('chapterCount', value)}
          min={1}
          max={500}
        />
        <NumberField
          label="Slides"
          unit="per chapter, 1–20"
          value={brief.slidesPerChapter}
          onChange={(value) => set('slidesPerChapter', value)}
          min={1}
          max={20}
        />
        {/* On a new video this is fixed at creation: the DAG gets one icon task
            per tile and cannot change width afterwards. Editing it later
            changes what the record says was planned, not the graph. */}
        <NumberField
          label="Thumbnail"
          unit="tiles, 1–24"
          value={brief.thumbnailCells}
          onChange={(value) => set('thumbnailCells', value)}
          min={1}
          max={24}
        />
      </div>
    </>
  )
}
