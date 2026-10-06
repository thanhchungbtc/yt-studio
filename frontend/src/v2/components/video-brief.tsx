import type { ReactNode } from 'react'

import type { VideoBrief } from '../core/api'
import { count } from '../core/format'
import type { Video } from '../core/types'
import { NARRATION_WPM } from './editor/video/stages'
import { gridShape } from './thumbnail/compose'
import { defaultStyle } from './thumbnail/style'
import { Field, FieldDivider, INDENT, Input, RangeField, Textarea } from './ui/field'

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

// The backend's caps, from checkBrief in internal/services/videos.go. Mirrored rather than discovered: they are part of
// the request contract, and a form that finds out its own limits by being
// rejected is the thing this mirrors them to avoid. Both count the trimmed
// value, because trimmed is what gets sent.
export const TITLE_MAX = 200
export const TOPIC_MAX = 5000

/**
 * The four numeric bounds, once.
 *
 * The rows below read their track from here and `briefReady` checks against the
 * same rows, so a field cannot be draggable to a value the button then refuses
 * to send. The step is a drag granularity and not a constraint — the box beside
 * each track still takes any figure in range, which is what the two controls
 * are for.
 */
const LIMITS = {
  durationMinutes: { min: 0, max: 720, step: 5 },
  chapterCount: { min: 1, max: 500, step: 1 },
  slidesPerChapter: { min: 1, max: 20, step: 1 },
  thumbnailCells: { min: 1, max: 24, step: 1 },
} as const

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

/** A video idea in JSON, as the brainstorm skill writes it. */
export interface Idea {
  brief: Partial<Brief>
  channel?: string
  start?: boolean
}

export function ideaFrom(text: string): Idea | null {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return null
  }
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const o = raw as Record<string, unknown>
  if (typeof o.title !== 'string') return null
  const brief: Partial<Brief> = { title: o.title }
  if (typeof o.topic === 'string') brief.topic = o.topic
  const numbers: [keyof Brief, string][] = [
    ['chapterCount', 'chapterCount'],
    ['durationMinutes', 'targetDurationMinutes'],
    ['slidesPerChapter', 'slidesPerChapter'],
    ['thumbnailCells', 'thumbnailCells'],
  ]
  for (const [field, key] of numbers) {
    const v = o[key]
    if (typeof v === 'number' && Number.isFinite(v)) brief[field] = String(v)
  }
  return {
    brief,
    ...(typeof o.channel === 'string' ? { channel: o.channel } : {}),
    ...(typeof o.start === 'boolean' ? { start: o.start } : {}),
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
 * The numbers as well as the prose, because a figure can be typed into the box
 * beside a track: the button goes dead on a title over its limit and on a
 * chapter count of 900 alike, rather than letting the server be the thing that
 * says no.
 */
export function briefReady(brief: Brief): boolean {
  const title = brief.title.trim().length
  if (title === 0 || title > TITLE_MAX) return false
  if (brief.topic.trim().length > TOPIC_MAX) return false
  return Object.entries(LIMITS).every(([field, limit]) => {
    const value = Number(brief[field as keyof typeof LIMITS])
    return Number.isInteger(value) && value >= limit.min && value <= limit.max
  })
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
 * Mirrors `entity.DefaultWordsPerChapter`, and only for the projection below.
 *
 * Duplicated for the same reason `NARRATION_WPM` beside it is: it is a constant
 * of the plan rather than a setting, and the cost of it drifting is a readout
 * that reads a little long or a little short — which is a projection either
 * way. Nothing is decided from it.
 */
const WORDS_PER_CHAPTER = 450

/** A rough length, as anyone says it out loud: `3h`, `2h 53m`, `45m`. */
function coarse(minutes: number): string {
  const whole = Math.round(minutes)
  if (whole < 60) return `${whole}m`
  const h = Math.floor(whole / 60)
  const m = whole % 60
  return m === 0 ? `${h}h` : `${h}h ${m}m`
}

/**
 * What the four numbers come to, in the words the plan is made of.
 *
 * The same arithmetic the blueprint prompt does — a target duration wins when
 * set, and without one the length is whatever the chapters come to at the
 * default size — so this line is the budget the model will actually be given
 * rather than a second guess at it.
 *
 * Worth a line because the two figures it is derived from are the two an
 * operator cannot hold in their head at once. Fifty chapters against three
 * hours is a fine plan; five hundred against the same three hours is
 * twenty-three words a chapter, and the only place that was ever visible was in
 * the scripts, an hour of generation later.
 */
function budgetOf(brief: Brief): { runtime: string; wordsPerChapter: number } | null {
  const chapters = Number(brief.chapterCount) || 0
  const minutes = Number(brief.durationMinutes) || 0
  if (chapters <= 0) return null
  const words = minutes > 0 ? minutes * NARRATION_WPM : chapters * WORDS_PER_CHAPTER
  return {
    runtime: coarse(words / NARRATION_WPM),
    wordsPerChapter: Math.round(words / chapters),
  }
}

/** Under this, a chapter is a paragraph read aloud rather than a chapter. */
const THIN_CHAPTER_WORDS = 80

/**
 * The grid a thumbnail count lays out as.
 *
 * Drawn rather than described because the count on its own says nothing about
 * the picture: twelve is two rows of six and twenty-four is two rows of twelve,
 * and which of those a channel wants is a thing you can see and cannot read.
 *
 * The shape comes from the renderer's own `gridShape` at the default style, so
 * the preview cannot drift from what the composer draws. A channel whose style
 * sets a different row count gets a different picture — the dialog has no style
 * to consult, so this is the default's answer, which is also what most videos
 * get.
 */
function GridPreview({ cells }: { cells: number }) {
  if (cells < 1 || cells > LIMITS.thumbnailCells.max) return null
  const { rows, cols } = gridShape(cells, defaultStyle)

  // Row by row, the way the composer fills them: full rows first and the
  // remainder last, so a short bottom row is drawn short.
  const perRow: number[] = []
  let remaining = cells
  for (let r = 0; r < rows; r += 1) {
    const n = Math.min(cols, remaining)
    perRow.push(n)
    remaining -= n
  }

  return (
    <div className="flex items-center gap-3">
      <div className="flex flex-col gap-[3px]" aria-hidden>
        {perRow.map((n, row) => (
          <div key={row} className="flex gap-[3px]">
            {Array.from({ length: n }, (_, i) => (
              <span
                key={i}
                className="block h-[14px] w-[14px] rounded-[3px]"
                style={{ backgroundColor: 'var(--accent)', opacity: 0.55 }}
              />
            ))}
          </div>
        ))}
      </div>
      <span className="text-[11px] text-tertiary">
        {rows === 1 ? 'one row' : `${rows} rows`} of {cols}
      </span>
    </div>
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
 * Below it they are sizes, and every one is a track. That is the change worth
 * defending: these four were number boxes with their ranges printed beside them
 * as text, which made picking a length a matter of typing a figure and hoping.
 * They are all *rough* — a duration is a target, a chapter count is a target
 * with a tolerance band, and the tile count is a shape — so the control that
 * suits them is the one you sweep, with the box kept beside it for the times a
 * particular number is the point.
 *
 * Then the plan says what it comes to, and the grid shows what it looks like.
 * Neither is decoration: both are answers the operator otherwise waits an hour
 * of generation to find out.
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
  const budget = budgetOf(brief)
  const thin = budget !== null && budget.wordsPerChapter < THIN_CHAPTER_WORDS

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

      <RangeField
        label="Duration"
        // Zero is not a length, it is the absence of one, and the unit is where
        // that gets said: the chapter count decides instead.
        unit={minutes > 0 ? 'minutes' : 'automatic'}
        value={brief.durationMinutes}
        onChange={(value) => set('durationMinutes', value)}
        {...LIMITS.durationMinutes}
      />
      <RangeField
        label="Chapters"
        unit="a target"
        value={brief.chapterCount}
        onChange={(value) => set('chapterCount', value)}
        {...LIMITS.chapterCount}
      />
      <RangeField
        label="Slides"
        unit="per chapter"
        value={brief.slidesPerChapter}
        onChange={(value) => set('slidesPerChapter', value)}
        {...LIMITS.slidesPerChapter}
      />
      {/* On a new video the tile count is fixed at creation: the DAG gets one
          icon task per tile and cannot change width afterwards. Editing it
          later changes what the record says was planned, not the graph. */}
      <RangeField
        label="Thumbnail"
        unit="tiles"
        value={brief.thumbnailCells}
        onChange={(value) => set('thumbnailCells', value)}
        {...LIMITS.thumbnailCells}
      >
        <GridPreview cells={Number(brief.thumbnailCells) || 0} />
      </RangeField>

      {budget ? (
        <p className={`${INDENT} pt-3 text-[11px] text-secondary`}>
          <span className="tabular-nums">≈ {budget.runtime}</span> of narration ·{' '}
          <span className="tabular-nums">{count(budget.wordsPerChapter)}</span> words a chapter
          {thin ? <span style={{ color: 'var(--running)' }}> · thin for a chapter</span> : null}
        </p>
      ) : null}
    </>
  )
}
